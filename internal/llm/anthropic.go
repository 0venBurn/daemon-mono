package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Anthropic provider — wraps the official anthropic-sdk-go
// ---------------------------------------------------------------------------

type AnthropicProvider struct {
	client       *http.Client
	providerName string
	apiKey       string
	model        string
	baseURL      string
	maxTokens    int64
	thinking     ThinkingLevel
}

// NewAnthropicProvider creates a provider that talks to the Anthropic Messages API.
// If baseURL is empty it defaults to https://api.anthropic.com.
func NewAnthropicProvider(cfg ProviderConfig) *AnthropicProvider {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	return &AnthropicProvider{
		client:       &http.Client{Timeout: 5 * time.Minute},
		providerName: cfg.Name,
		apiKey:       cfg.APIKey,
		model:        cfg.Model,
		baseURL:      baseURL,
		maxTokens:    4096,
		thinking:     cfg.Thinking,
	}
}

// SetMaxTokens allows overriding the default max_tokens (4096).
func (p *AnthropicProvider) SetMaxTokens(n int64) {
	p.maxTokens = n
}

func (p *AnthropicProvider) Close() error { return nil }

// ---------------------------------------------------------------------------
// Anthropic Messages API request/response types
// ---------------------------------------------------------------------------

type anthropicResponse struct {
	ID         string                  `json:"id"`
	Type       string                  `json:"type"`
	Role       string                  `json:"role"`
	Content    []anthropicContentBlock `json:"content"`
	Model      string                  `json:"model"`
	StopReason string                  `json:"stop_reason"`
}

type anthropicContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
}

// ---------------------------------------------------------------------------
// CreateMessage
// ---------------------------------------------------------------------------

func (p *AnthropicProvider) CreateMessage(ctx context.Context, messages []Message, systemPrompt string, tools []ToolDef) (*Message, error) {
	reqBody, err := p.buildRequest(messages, systemPrompt, tools, false)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	respBody, err := p.doRequest(ctx, reqBody)
	if err != nil {
		return nil, err
	}
	defer respBody.Close()

	var resp anthropicResponse
	if err := json.NewDecoder(respBody).Decode(&resp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	msg := &Message{Role: RoleAssistant}
	for _, block := range resp.Content {
		msg.Content = append(msg.Content, convertAnthropicBlock(block))
	}
	return msg, nil
}

// ---------------------------------------------------------------------------
// StreamMessages
// ---------------------------------------------------------------------------

func (p *AnthropicProvider) StreamMessages(ctx context.Context, messages []Message, systemPrompt string, tools []ToolDef) <-chan StreamEvent {
	ch := make(chan StreamEvent, 64)

	reqBody, err := p.buildRequest(messages, systemPrompt, tools, true)
	if err != nil {
		go func() {
			ch <- StreamEvent{Type: EventError, Error: fmt.Errorf("build request: %w", err)}
			close(ch)
		}()
		return ch
	}

	respBody, err := p.doRequest(ctx, reqBody)
	if err != nil {
		go func() {
			ch <- StreamEvent{Type: EventError, Error: err}
			close(ch)
		}()
		return ch
	}

	go p.streamSSE(ctx, ch, respBody)
	return ch
}

// streamSSE reads SSE events from respBody, accumulating tool_use blocks
// and forwarding text_delta events to ch.
func (p *AnthropicProvider) streamSSE(ctx context.Context, ch chan<- StreamEvent, body io.ReadCloser) {
	defer body.Close()
	defer close(ch)

	scanner := newSSEScanner(body)

	// Accumulate tool_use blocks that arrive as content_block_start + deltas.
	var toolID string
	var toolName string
	var toolBuf strings.Builder
	inToolUse := false

	for scanner.next() {
		select {
		case <-ctx.Done():
			ch <- StreamEvent{Type: EventError, Error: ctx.Err()}
			return
		default:
		}

		ev := scanner.event()
		switch ev.Type {
		case "content_block_start":
			var payload struct {
				ContentBlock struct {
					Type string `json:"type"`
					ID   string `json:"id"`
					Name string `json:"name"`
					Text string `json:"text"`
				} `json:"content_block"`
			}
			if err := json.Unmarshal(ev.Data, &payload); err == nil {
				if payload.ContentBlock.Type == "tool_use" {
					toolID = payload.ContentBlock.ID
					toolName = payload.ContentBlock.Name
					inToolUse = true
					toolBuf.Reset()
				} else if payload.ContentBlock.Text != "" {
					ch <- StreamEvent{Type: EventTextDelta, Text: payload.ContentBlock.Text}
				}
			}

		case "content_block_delta":
			var payload struct {
				Delta struct {
					Type        string `json:"type"`
					Text        string `json:"text"`
					PartialJSON string `json:"partial_json"`
				} `json:"delta"`
			}
			if err := json.Unmarshal(ev.Data, &payload); err == nil {
				if inToolUse {
					if payload.Delta.PartialJSON != "" {
						toolBuf.WriteString(payload.Delta.PartialJSON)
					} else if payload.Delta.Text != "" {
						toolBuf.WriteString(payload.Delta.Text)
					}
				} else {
					if payload.Delta.Text != "" {
						ch <- StreamEvent{Type: EventTextDelta, Text: payload.Delta.Text}
					}
				}
			}

		case "content_block_stop":
			if inToolUse {
				ch <- StreamEvent{
					Type:     EventToolUse,
					ToolID:   toolID,
					ToolName: toolName,
					Text:     toolBuf.String(),
				}
				inToolUse = false
			}

		case "message_stop":
			ch <- StreamEvent{Type: EventStop}

		case "message_delta":
			var payload struct {
				Delta struct {
					StopReason string `json:"stop_reason"`
				} `json:"delta"`
			}
			if err := json.Unmarshal(ev.Data, &payload); err == nil && payload.Delta.StopReason != "" {
				ch <- StreamEvent{Type: EventStop, StopReason: payload.Delta.StopReason}
			}

		case "error":
			ch <- StreamEvent{Type: EventError, Error: fmt.Errorf("stream error: %s", string(ev.Data))}
			return

		case "ping":
			// ignore
		}
	}
	if err := scanner.err(); err != nil {
		ch <- StreamEvent{Type: EventError, Error: err}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// isAdaptiveThinkingModel returns true for models that support adaptive
// thinking (Claude Opus 4.6+, Sonnet 4.6+).
func isAdaptiveThinkingModel(model string) bool {
	adaptiveModels := []string{
		"claude-opus-4-6", "claude-opus-4-7", "claude-opus-4-8",
		"claude-sonnet-4-6",
	}
	for _, m := range adaptiveModels {
		if model == m || strings.HasPrefix(model, m+"-") {
			return true
		}
	}
	if strings.HasPrefix(model, "claude-mythos") {
		return true
	}
	return false
}

// mapAnthropicEffort maps a daemon ThinkingLevel to Anthropic thinking params.
func mapAnthropicEffort(level ThinkingLevel, model string) (thinkingType string, effort string, budgetTokens int) {
	if level == "" || level == ThinkingOff {
		return "", "", 0
	}
	if isAdaptiveThinkingModel(model) {
		switch level {
		case ThinkingLow:
			return "adaptive", "low", 0
		case ThinkingMedium:
			return "adaptive", "medium", 0
		case ThinkingHigh:
			return "adaptive", "high", 0
		case ThinkingXHigh:
			return "adaptive", "xhigh", 0
		}
	}
	// Budget-based thinking for older models
	switch level {
	case ThinkingLow:
		return "enabled", "", 1024
	case ThinkingMedium:
		return "enabled", "", 4096
	case ThinkingHigh:
		return "enabled", "", 8192
	case ThinkingXHigh:
		return "enabled", "", 16384
	}
	return "", "", 0
}

func (p *AnthropicProvider) buildRequest(messages []Message, systemPrompt string, tools []ToolDef, stream bool) (json.RawMessage, error) {
	// Use map-based approach so we can conditionally include thinking fields.
	req := map[string]any{
		"model":      p.model,
		"max_tokens": p.maxTokens,
	}

	if stream {
		req["stream"] = true
	}

	if systemPrompt != "" {
		req["system"] = []map[string]any{{"type": "text", "text": systemPrompt}}
	}

	msgSlice, err := convertMessagesJSON(messages)
	if err != nil {
		return nil, err
	}
	req["messages"] = msgSlice

	if len(tools) > 0 {
		toolDefs, err := convertToolsJSON(tools)
		if err != nil {
			return nil, err
		}
		req["tools"] = toolDefs
	}

	// Thinking configuration
	thinkType, effort, budget := mapAnthropicEffort(p.thinking, p.model)
	if thinkType != "" {
		thinkingMap := map[string]any{"type": thinkType}
		if thinkType == "enabled" {
			thinkingMap["budget_tokens"] = budget
			thinkingMap["display"] = "summarized"
		} else if thinkType == "adaptive" {
			thinkingMap["display"] = "summarized"
		}
		req["thinking"] = thinkingMap
		if effort != "" {
			req["output_config"] = map[string]any{"effort": effort}
		}
	} else if p.thinking == ThinkingOff {
		req["thinking"] = map[string]any{"type": "disabled"}
	}

	// Interleaved thinking beta for budget-based models
	if thinkType == "enabled" {
		req["beta"] = []string{"interleaved-thinking-2025-05-14"}
	}

	return json.Marshal(req)
}

func convertMessagesJSON(messages []Message) (json.RawMessage, error) {
	var rawMsgs []json.RawMessage
	for _, m := range messages {
		content, err := json.Marshal(convertContentBlocks(m.Content))
		if err != nil {
			return nil, err
		}
		msg := map[string]any{"role": string(m.Role), "content": json.RawMessage(content)}
		b, err := json.Marshal(msg)
		if err != nil {
			return nil, err
		}
		rawMsgs = append(rawMsgs, b)
	}
	return json.Marshal(rawMsgs)
}

func convertToolsJSON(tools []ToolDef) (json.RawMessage, error) {
	var defs []map[string]any
	for _, t := range tools {
		defs = append(defs, map[string]any{
			"type":         "custom",
			"name":         t.Name,
			"description":  t.Description,
			"input_schema": t.InputSchema,
		})
	}
	return json.Marshal(defs)
}

func (p *AnthropicProvider) doRequest(ctx context.Context, body json.RawMessage) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	applyOpenCodeHeaders(req, ctx, p.providerName, p.baseURL)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("anthropic API error %d: %s", resp.StatusCode, string(b))
	}
	return resp.Body, nil
}

func convertContentBlocks(blocks []ContentBlock) []anthropicContentBlock {
	var out []anthropicContentBlock
	for _, b := range blocks {
		out = append(out, convertAnthropicBlockInverse(b))
	}
	return out
}

func convertAnthropicBlockInverse(b ContentBlock) anthropicContentBlock {
	switch b.Type {
	case "tool_result":
		return anthropicContentBlock{
			Type:      "tool_result",
			ToolUseID: b.ToolUseID,
			Text:      b.Text,
		}
	case "tool_use":
		return anthropicContentBlock{
			Type:  "tool_use",
			ID:    b.ID,
			Name:  b.Name,
			Input: b.Input,
		}
	default: // "text"
		return anthropicContentBlock{
			Type: "text",
			Text: b.Text,
		}
	}
}

func convertAnthropicBlock(b anthropicContentBlock) ContentBlock {
	switch b.Type {
	case "tool_use":
		return ToolUseBlock(b.ID, b.Name, b.Input)
	case "tool_result":
		return ToolResultBlock(b.ToolUseID, b.Text, false)
	default:
		return TextBlock(b.Text)
	}
}

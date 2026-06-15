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
// OpenAI-compatible provider — works with OpenAI, OpenRouter, and any
// endpoint that follows the OpenAI Chat Completions API format.
// Uses only net/http; no third-party SDK.
// ---------------------------------------------------------------------------

type OpenAIProvider struct {
	client       *http.Client
	providerName string
	apiKey       string
	baseURL      string
	model        string
	maxTokens    int64
	thinking     ThinkingLevel
	compat       openAICompat
}

type openAICompat struct {
	MaxTokensField                              string
	ThinkingFormat                              string
	SupportsReasoningEffort                     bool
	SupportsUsageInStreaming                    bool
	SupportsStrictMode                          bool
	RequiresReasoningContentOnAssistantMessages bool
	ZAIToolStream                               bool
}

// NewOpenAIProvider creates a provider for OpenAI-compatible endpoints.
// baseURL should be something like "https://api.openai.com" or
// "https://openrouter.ai/api". The /v1/chat/completions path is appended.
func NewOpenAIProvider(cfg ProviderConfig) *OpenAIProvider {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	return &OpenAIProvider{
		client:       &http.Client{Timeout: 5 * time.Minute},
		providerName: cfg.Name,
		apiKey:       cfg.APIKey,
		baseURL:      baseURL,
		model:        cfg.Model,
		maxTokens:    4096,
		thinking:     cfg.Thinking,
		compat:       openAICompatForConfig(cfg),
	}
}

// SetMaxTokens allows overriding the default max_tokens (4096).
func (p *OpenAIProvider) SetMaxTokens(n int64) {
	p.maxTokens = n
}

func (p *OpenAIProvider) Close() error { return nil }

// ---------------------------------------------------------------------------
// OpenAI Chat Completions request/response types
// ---------------------------------------------------------------------------

type oaiMessage struct {
	Role       string        `json:"role"`
	Content    interface{}   `json:"content"` // string or []oaiContentBlock
	ToolCalls  []oaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"` // for tool role
}

type oaiContentBlock struct {
	Type string `json:"type"` // "text" or "tool_use" (not used by OpenAI)
	Text string `json:"text,omitempty"`
}

type oaiTool struct {
	Type     string      `json:"type"` // "function"
	Function oaiFunction `json:"function"`
}

type oaiFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type oaiToolCall struct {
	ID       string        `json:"id,omitempty"`
	Index    *int          `json:"index,omitempty"`
	Type     string        `json:"type,omitempty"` // "function"
	Function oaiToolCallFn `json:"function"`
}

type oaiToolCallFn struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string
}

// --- Response types ---

type oaiResponse struct {
	Choices []oaiChoice `json:"choices"`
}

type oaiChoice struct {
	Message      oaiMessage `json:"message"`
	FinishReason string     `json:"finish_reason"`
}

// --- Streaming types (SSE chunk) ---

type oaiStreamChunk struct {
	Choices []oaiStreamChoice `json:"choices"`
}

type oaiStreamChoice struct {
	Delta oaiStreamDelta `json:"delta"`
	// FinishReason is non-empty on the final chunk.
	FinishReason string `json:"finish_reason,omitempty"`
}

type oaiStreamDelta struct {
	Role             string          `json:"role,omitempty"`
	Content          string          `json:"content,omitempty"`
	ReasoningContent string          `json:"reasoning_content,omitempty"`
	ReasoningText    string          `json:"reasoning_text,omitempty"`
	Reasoning        json.RawMessage `json:"reasoning,omitempty"`
	ToolCalls        []oaiToolCall   `json:"tool_calls,omitempty"`
}

// ---------------------------------------------------------------------------
// CreateMessage
// ---------------------------------------------------------------------------

func (p *OpenAIProvider) CreateMessage(ctx context.Context, messages []Message, systemPrompt string, tools []ToolDef) (*Message, error) {
	reqBody, err := p.buildRequest(messages, systemPrompt, tools, false)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	respBody, _, err := p.doRequest(ctx, reqBody)
	if err != nil {
		return nil, err
	}
	defer respBody.Close()

	var resp oaiResponse
	if err := json.NewDecoder(respBody).Decode(&resp); err != nil {
		return nil, fmt.Errorf("decode openai response: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("openai returned no choices")
	}

	return p.convertResponse(&resp), nil
}

// ---------------------------------------------------------------------------
// StreamMessages
// ---------------------------------------------------------------------------

func (p *OpenAIProvider) StreamMessages(ctx context.Context, messages []Message, systemPrompt string, tools []ToolDef) <-chan StreamEvent {
	ch := make(chan StreamEvent, 64)

	reqBody, err := p.buildRequest(messages, systemPrompt, tools, true)
	if err != nil {
		go func() {
			ch <- StreamEvent{Type: EventError, Error: fmt.Errorf("build request: %w", err)}
			close(ch)
		}()
		return ch
	}

	go func() {
		respBody, statusCode, err := p.doRequest(ctx, reqBody)
		if err != nil {
			ch <- StreamEvent{Type: EventError, Error: err}
			close(ch)
			return
		}
		ch <- StreamEvent{Type: EventResponseHeaders, StatusCode: statusCode}
		p.streamSSE(ctx, ch, respBody)
	}()
	return ch
}

func (p *OpenAIProvider) streamSSE(ctx context.Context, ch chan<- StreamEvent, body io.ReadCloser) {
	defer body.Close()
	defer close(ch)

	scanner := newSSEScanner(body)

	// OpenAI streams tool calls as deltas keyed by index. The ID commonly appears
	// only on the first chunk, so index must be the primary accumulator key.
	var toolAccum []*oaiToolCall

	for scanner.next() {
		select {
		case <-ctx.Done():
			ch <- StreamEvent{Type: EventError, Error: ctx.Err()}
			return
		default:
		}

		ev := scanner.event()
		ch <- StreamEvent{Type: EventRawEvent}
		// OpenAI uses event: (blank) with data lines for chat completion chunks
		var chunk oaiStreamChunk
		if err := json.Unmarshal(ev.Data, &chunk); err != nil {
			// Could be a "[DONE]" message
			if strings.TrimSpace(string(ev.Data)) == "[DONE]" {
				ch <- StreamEvent{Type: EventStop}
				return
			}
			continue
		}

		for _, choice := range chunk.Choices {
			delta := choice.Delta

			// Text delta
			if delta.Content != "" {
				ch <- StreamEvent{Type: EventTextDelta, Text: delta.Content}
			}
			if reasoning := openAIReasoningText(delta); reasoning != "" {
				ch <- StreamEvent{Type: EventThinkingDelta, Text: reasoning}
			}

			// Tool call deltas — accumulate by index, falling back to ID.
			for _, tc := range delta.ToolCalls {
				accum := findToolAccumulator(toolAccum, tc)
				if accum == nil {
					toolAccum = append(toolAccum, &oaiToolCall{
						ID:       tc.ID,
						Index:    tc.Index,
						Type:     tc.Type,
						Function: oaiToolCallFn{Name: tc.Function.Name, Arguments: tc.Function.Arguments},
					})
					continue
				}
				accum.Function.Arguments += tc.Function.Arguments
				if tc.Function.Name != "" {
					accum.Function.Name = tc.Function.Name
				}
				if tc.ID != "" {
					accum.ID = tc.ID
				}
				if tc.Type != "" {
					accum.Type = tc.Type
				}
			}

			// Finish
			if choice.FinishReason != "" {
				// Emit accumulated tool calls
				for _, tc := range toolAccum {
					ch <- StreamEvent{
						Type:     EventToolUse,
						ToolID:   tc.ID,
						ToolName: tc.Function.Name,
						Text:     tc.Function.Arguments, // JSON string of arguments
					}
				}
				ch <- StreamEvent{Type: EventStop, StopReason: choice.FinishReason}
				return
			}
		}
	}

	if err := scanner.err(); err != nil {
		ch <- StreamEvent{Type: EventError, Error: err}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// mapOAIReasoningEffort maps a daemon ThinkingLevel to an OpenAI reasoning_effort value.
func mapOAIReasoningEffort(level ThinkingLevel) string {
	switch level {
	case ThinkingLow:
		return "low"
	case ThinkingMedium:
		return "medium"
	case ThinkingHigh:
		return "high"
	case ThinkingXHigh:
		return "high" // OpenAI max is "high"
	}
	return ""
}

func (p *OpenAIProvider) buildRequest(messages []Message, systemPrompt string, tools []ToolDef, stream bool) (json.RawMessage, error) {
	messagesOut := make([]oaiMessage, 0, len(messages)+1)
	if systemPrompt != "" {
		messagesOut = append(messagesOut, oaiMessage{Role: "system", Content: systemPrompt})
	}
	messagesOut = append(messagesOut, convertToOAIMessages(messages)...)

	req := map[string]any{
		"model":    p.model,
		"messages": messagesOut,
	}
	if stream {
		req["stream"] = true
		if p.compat.SupportsUsageInStreaming {
			req["stream_options"] = map[string]any{"include_usage": true}
		}
	}

	maxTokensField := p.compat.MaxTokensField
	if maxTokensField == "" {
		maxTokensField = "max_tokens"
	}
	req[maxTokensField] = p.maxTokens

	if len(tools) > 0 {
		toolsOut := make([]oaiTool, 0, len(tools))
		for _, t := range tools {
			fn := oaiFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			}
			toolsOut = append(toolsOut, oaiTool{Type: "function", Function: fn})
		}
		req["tools"] = toolsOut
	}

	if p.compat.SupportsReasoningEffort {
		if effort := mapOAIReasoningEffort(p.thinking); effort != "" {
			req["reasoning_effort"] = effort
		}
	}

	return json.Marshal(req)
}

func (p *OpenAIProvider) doRequest(ctx context.Context, body json.RawMessage) (io.ReadCloser, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openAIChatCompletionsURL(p.baseURL), bytes.NewReader(body))
	if err != nil {
		return nil, 0, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	applyOpenCodeHeaders(req, ctx, p.providerName, p.baseURL)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("openai request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return nil, resp.StatusCode, fmt.Errorf("openai API error %d: %s", resp.StatusCode, string(b))
	}
	return resp.Body, resp.StatusCode, nil
}

func openAIChatCompletionsURL(baseURL string) string {
	baseURL = strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(baseURL, "/v1") {
		return baseURL + "/chat/completions"
	}
	return baseURL + "/v1/chat/completions"
}

func openAICompatForConfig(cfg ProviderConfig) openAICompat {
	compat := openAICompat{MaxTokensField: "max_tokens", SupportsReasoningEffort: true}
	if cfg.Name == "opencode" || cfg.Name == "opencode-go" || strings.Contains(cfg.BaseURL, "opencode.ai/zen") {
		compat.SupportsReasoningEffort = false
	}

	if cfg.Name == "opencode-go" {
		switch cfg.Model {
		case "glm-5", "glm-5.1", "zai/glm-5", "zai/glm-5.1":
			compat.MaxTokensField = "max_tokens"
			compat.ThinkingFormat = "zai"
			compat.ZAIToolStream = true
		case "deepseek-v4-flash", "deepseek-v4-pro":
			compat.ThinkingFormat = "deepseek"
		case "qwen3.6-plus", "qwen3.7-plus", "qwen3.7-max", "alibaba/qwen3-coder", "alibaba/qwen3-coder-next":
			compat.ThinkingFormat = "qwen"
		case "minimax-m3", "minimax-m2.7":
			compat.SupportsStrictMode = false
		}
	}
	if cfg.Name == "opencode" {
		switch cfg.Model {
		case "glm-5", "glm-5.1":
			compat.ThinkingFormat = "zai"
			compat.ZAIToolStream = true
		case "deepseek-v4-flash", "deepseek-v4-pro":
			compat.ThinkingFormat = "deepseek"
		case "qwen3.6-plus", "qwen3.7-plus", "qwen3.7-max":
			compat.ThinkingFormat = "qwen"
		}
	}
	return compat
}

func openAIReasoningText(delta oaiStreamDelta) string {
	if delta.ReasoningContent != "" {
		return delta.ReasoningContent
	}
	if delta.ReasoningText != "" {
		return delta.ReasoningText
	}
	if len(delta.Reasoning) == 0 || string(delta.Reasoning) == "null" {
		return ""
	}
	var text string
	if err := json.Unmarshal(delta.Reasoning, &text); err == nil {
		return text
	}
	var obj map[string]any
	if err := json.Unmarshal(delta.Reasoning, &obj); err != nil {
		return ""
	}
	for _, key := range []string{"content", "text", "summary"} {
		if value, ok := obj[key].(string); ok && value != "" {
			return value
		}
	}
	return ""
}

func findToolAccumulator(accum []*oaiToolCall, delta oaiToolCall) *oaiToolCall {
	for _, existing := range accum {
		if delta.Index != nil && existing.Index != nil && *delta.Index == *existing.Index {
			return existing
		}
		if delta.ID != "" && existing.ID == delta.ID {
			return existing
		}
	}
	return nil
}

func convertToOAIMessages(messages []Message) []oaiMessage {
	var converted []oaiMessage
	for _, m := range messages {
		converted = append(converted, convertOneToOAIMessages(m)...)
	}
	return converted
}

func convertToOAIMessage(m Message) oaiMessage {
	msgs := convertOneToOAIMessages(m)
	if len(msgs) == 0 {
		return oaiMessage{Role: string(m.Role), Content: ""}
	}
	return msgs[0]
}

func convertOneToOAIMessages(m Message) []oaiMessage {
	var toolResults []oaiMessage
	var textParts []oaiContentBlock

	for _, b := range m.Content {
		if b.Type == "tool_result" {
			toolResults = append(toolResults, oaiMessage{
				Role:       "tool",
				Content:    b.Text,
				ToolCallID: b.ToolUseID,
			})
			continue
		}
		if b.Type == "text" {
			textParts = append(textParts, oaiContentBlock{Type: "text", Text: b.Text})
		}
	}
	if len(toolResults) > 0 {
		if len(textParts) == 0 {
			return toolResults
		}
		return append([]oaiMessage{simpleOAITextMessage(m.Role, textParts)}, toolResults...)
	}

	// Check if any content is a tool_use — then it's an assistant message with tool_calls.
	hasToolUse := false
	for _, b := range m.Content {
		if b.Type == "tool_use" {
			hasToolUse = true
			break
		}
	}

	if m.Role == RoleAssistant && hasToolUse {
		var toolCalls []oaiToolCall
		var text []string
		for _, b := range m.Content {
			switch b.Type {
			case "tool_use":
				args := "{}"
				if len(b.Input) > 0 {
					args = string(b.Input)
				}
				toolCalls = append(toolCalls, oaiToolCall{
					ID:       b.ID,
					Type:     "function",
					Function: oaiToolCallFn{Name: b.Name, Arguments: args},
				})
			case "text":
				text = append(text, b.Text)
			}
		}
		var content any
		if joined := strings.Join(text, "\n"); joined != "" {
			content = joined
		}
		return []oaiMessage{{
			Role:      "assistant",
			Content:   content,
			ToolCalls: toolCalls,
		}}
	}

	return []oaiMessage{simpleOAITextMessage(m.Role, textParts)}
}

func simpleOAITextMessage(role Role, parts []oaiContentBlock) oaiMessage {
	if len(parts) == 0 {
		return oaiMessage{Role: string(role), Content: ""}
	}
	if len(parts) == 1 {
		return oaiMessage{Role: string(role), Content: parts[0].Text}
	}
	return oaiMessage{Role: string(role), Content: parts}
}

func (p *OpenAIProvider) convertResponse(resp *oaiResponse) *Message {
	msg := &Message{Role: RoleAssistant}
	choice := resp.Choices[0]

	// Text content — Content may be a string or nil
	switch v := choice.Message.Content.(type) {
	case string:
		if v != "" {
			msg.Content = append(msg.Content, TextBlock(v))
		}
	case []interface{}:
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				if t, _ := m["type"].(string); t == "text" {
					if text, _ := m["text"].(string); text != "" {
						msg.Content = append(msg.Content, TextBlock(text))
					}
				}
			}
		}
	}

	// Tool calls
	for _, tc := range choice.Message.ToolCalls {
		input := json.RawMessage(tc.Function.Arguments)
		if len(input) == 0 {
			input = json.RawMessage("{}")
		}
		msg.Content = append(msg.Content, ToolUseBlock(tc.ID, tc.Function.Name, input))
	}

	return msg
}

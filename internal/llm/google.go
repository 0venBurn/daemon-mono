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
// Google Gemini provider — uses the Generative AI REST API.
// No third-party SDK; only net/http.
// ---------------------------------------------------------------------------

type GoogleProvider struct {
	client       *http.Client
	providerName string
	apiKey       string
	model        string
	baseURL      string
	maxTokens    int64
	thinking     ThinkingLevel
}

// NewGoogleProvider creates a provider for Google Generative AI.
// If baseURL is empty it defaults to "https://generativelanguage.googleapis.com".
func NewGoogleProvider(cfg ProviderConfig) *GoogleProvider {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com"
	}
	return &GoogleProvider{
		client:       &http.Client{Timeout: 5 * time.Minute},
		providerName: cfg.Name,
		apiKey:       cfg.APIKey,
		model:        cfg.Model,
		baseURL:      baseURL,
		maxTokens:    8192,
		thinking:     cfg.Thinking,
	}
}

// SetMaxTokens allows overriding the default max output tokens (8192).
func (p *GoogleProvider) SetMaxTokens(n int64) {
	p.maxTokens = n
}

func (p *GoogleProvider) Close() error { return nil }

// ---------------------------------------------------------------------------
// Gemini API request/response types
// ---------------------------------------------------------------------------

type geminiRequest struct {
	Contents          []geminiContent  `json:"contents"`
	SystemInstruction *geminiContent   `json:"systemInstruction,omitempty"`
	Tools             []geminiToolDecl `json:"tools,omitempty"`
	GenerationConfig  *geminiGenConfig `json:"generationConfig,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"` // "user" or "model"
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text         string        `json:"text,omitempty"`
	FunctionCall *geminiFnCall `json:"functionCall,omitempty"`
	FunctionResp *geminiFnResp `json:"functionResponse,omitempty"`
}

type geminiFnCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
}

type geminiFnResp struct {
	Name     string          `json:"name"`
	Response json.RawMessage `json:"response,omitempty"`
}

type geminiToolDecl struct {
	FunctionDeclarations []geminiFnDecl `json:"functionDeclarations"`
}

type geminiFnDecl struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type geminiGenConfig struct {
	MaxOutputTokens int64              `json:"maxOutputTokens,omitempty"`
	Temperature     float64            `json:"temperature,omitempty"`
	ThinkingConfig  *geminiThinkingCfg `json:"thinkingConfig,omitempty"`
}

type geminiThinkingCfg struct {
	ThinkingLevel string `json:"thinkingLevel,omitempty"`
}

// --- Response types ---

type geminiResponse struct {
	Candidates []geminiCandidate `json:"candidates"`
}

type geminiCandidate struct {
	Content      geminiContent `json:"content"`
	FinishReason string        `json:"finishReason,omitempty"`
}

// --- Streaming types ---

type geminiStreamChunk struct {
	Candidates []geminiCandidate `json:"candidates"`
}

// ---------------------------------------------------------------------------
// CreateMessage
// ---------------------------------------------------------------------------

func (p *GoogleProvider) CreateMessage(ctx context.Context, messages []Message, systemPrompt string, tools []ToolDef) (*Message, error) {
	reqBody, err := p.buildRequest(messages, systemPrompt, tools, false)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	respBody, err := p.doRequest(ctx, reqBody, false)
	if err != nil {
		return nil, err
	}
	defer respBody.Close()

	var resp geminiResponse
	if err := json.NewDecoder(respBody).Decode(&resp); err != nil {
		return nil, fmt.Errorf("decode gemini response: %w", err)
	}

	if len(resp.Candidates) == 0 {
		return nil, fmt.Errorf("gemini returned no candidates")
	}

	return p.convertResponse(&resp), nil
}

// ---------------------------------------------------------------------------
// StreamMessages
// ---------------------------------------------------------------------------

func (p *GoogleProvider) StreamMessages(ctx context.Context, messages []Message, systemPrompt string, tools []ToolDef) <-chan StreamEvent {
	ch := make(chan StreamEvent, 64)

	reqBody, err := p.buildRequest(messages, systemPrompt, tools, true)
	if err != nil {
		go func() {
			ch <- StreamEvent{Type: EventError, Error: fmt.Errorf("build request: %w", err)}
			close(ch)
		}()
		return ch
	}

	respBody, err := p.doRequest(ctx, reqBody, true)
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

func (p *GoogleProvider) streamSSE(ctx context.Context, ch chan<- StreamEvent, body io.ReadCloser) {
	defer body.Close()
	defer close(ch)

	scanner := newSSEScanner(body)

	for scanner.next() {
		select {
		case <-ctx.Done():
			ch <- StreamEvent{Type: EventError, Error: ctx.Err()}
			return
		default:
		}

		ev := scanner.event()
		var chunk geminiStreamChunk
		if err := json.Unmarshal(ev.Data, &chunk); err != nil {
			continue
		}

		for _, cand := range chunk.Candidates {
			for _, part := range cand.Content.Parts {
				if part.Text != "" {
					ch <- StreamEvent{Type: EventTextDelta, Text: part.Text}
				}
				if part.FunctionCall != nil {
					argsJSON := part.FunctionCall.Args
					if len(argsJSON) == 0 {
						argsJSON = json.RawMessage("{}")
					}
					ch <- StreamEvent{
						Type:     EventToolUse,
						ToolID:   part.FunctionCall.Name, // Gemini doesn't have separate IDs
						ToolName: part.FunctionCall.Name,
						Text:     string(argsJSON),
					}
				}
			}
			if cand.FinishReason != "" {
				ch <- StreamEvent{Type: EventStop, StopReason: cand.FinishReason}
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

// mapGeminiThinkingLevel maps a daemon ThinkingLevel to a Gemini thinkingLevel enum value.
func mapGeminiThinkingLevel(level ThinkingLevel) string {
	switch level {
	case ThinkingLow:
		return "MINIMAL"
	case ThinkingMedium:
		return "MEDIUM"
	case ThinkingHigh:
		return "HIGH"
	case ThinkingXHigh:
		return "HIGH" // Gemini max is HIGH
	}
	return ""
}

func (p *GoogleProvider) buildRequest(messages []Message, systemPrompt string, tools []ToolDef, stream bool) (json.RawMessage, error) {
	genConfig := &geminiGenConfig{
		MaxOutputTokens: p.maxTokens,
	}

	if level := mapGeminiThinkingLevel(p.thinking); level != "" {
		genConfig.ThinkingConfig = &geminiThinkingCfg{ThinkingLevel: level}
	}

	req := geminiRequest{
		GenerationConfig: genConfig,
	}

	if systemPrompt != "" {
		req.SystemInstruction = &geminiContent{
			Parts: []geminiPart{{Text: systemPrompt}},
		}
	}

	for _, m := range messages {
		req.Contents = append(req.Contents, convertToGeminiContent(m))
	}

	if len(tools) > 0 {
		var decls []geminiFnDecl
		for _, t := range tools {
			decls = append(decls, geminiFnDecl{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			})
		}
		req.Tools = []geminiToolDecl{{FunctionDeclarations: decls}}
	}

	return json.Marshal(req)
}

func geminiEndpoint(model, action string) string {
	// action is "generateContent" or "streamGenerateContent"
	return fmt.Sprintf("/v1beta/models/%s:%s", model, action)
}

func (p *GoogleProvider) doRequest(ctx context.Context, body json.RawMessage, stream bool) (io.ReadCloser, error) {
	action := "generateContent"
	if stream {
		action = "streamGenerateContent"
	}
	url := p.baseURL + geminiEndpoint(p.model, action) + "?key=" + p.apiKey
	if stream {
		url += "&alt=sse"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	applyOpenCodeHeaders(req, ctx, p.providerName, p.baseURL)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gemini request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gemini API error %d: %s", resp.StatusCode, string(b))
	}
	return resp.Body, nil
}

func convertToGeminiContent(m Message) geminiContent {
	var parts []geminiPart

	for _, b := range m.Content {
		switch b.Type {
		case "text":
			parts = append(parts, geminiPart{Text: b.Text})
		case "tool_use":
			parts = append(parts, geminiPart{
				FunctionCall: &geminiFnCall{
					Name: b.Name,
					Args: b.Input,
				},
			})
		case "tool_result":
			// Gemini uses functionResponse for tool results
			respJSON := json.RawMessage(`{"result": ` + jsonQuote(b.Text) + `}`)
			if b.IsError {
				respJSON = json.RawMessage(`{"error": ` + jsonQuote(b.Text) + `}`)
			}
			parts = append(parts, geminiPart{
				FunctionResp: &geminiFnResp{
					Name:     b.ToolUseID,
					Response: respJSON,
				},
			})
		}
	}

	// If no parts were created, add an empty text part
	if len(parts) == 0 {
		parts = append(parts, geminiPart{Text: ""})
	}

	role := "user"
	if m.Role == RoleAssistant {
		role = "model"
	}

	return geminiContent{Role: role, Parts: parts}
}

func (p *GoogleProvider) convertResponse(resp *geminiResponse) *Message {
	msg := &Message{Role: RoleAssistant}
	if len(resp.Candidates) == 0 {
		return msg
	}
	cand := resp.Candidates[0]
	for _, part := range cand.Content.Parts {
		if part.Text != "" {
			msg.Content = append(msg.Content, TextBlock(part.Text))
		}
		if part.FunctionCall != nil {
			input := part.FunctionCall.Args
			if len(input) == 0 {
				input = json.RawMessage("{}")
			}
			msg.Content = append(msg.Content, ToolUseBlock(
				part.FunctionCall.Name, // Gemini lacks separate IDs
				part.FunctionCall.Name,
				input,
			))
		}
	}
	return msg
}

// jsonQuote wraps a string in JSON quotes, escaping as needed.
func jsonQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

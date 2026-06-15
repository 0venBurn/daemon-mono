package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestTextBlock(t *testing.T) {
	b := TextBlock("hello")
	if b.Type != "text" {
		t.Errorf("expected type text, got %s", b.Type)
	}
	if b.Text != "hello" {
		t.Errorf("expected text hello, got %s", b.Text)
	}
}

func TestToolUseBlock(t *testing.T) {
	input := json.RawMessage(`{"x": 1}`)
	b := ToolUseBlock("id-1", "my_tool", input)
	if b.Type != "tool_use" {
		t.Errorf("expected type tool_use, got %s", b.Type)
	}
	if b.ID != "id-1" {
		t.Errorf("expected id id-1, got %s", b.ID)
	}
	if b.Name != "my_tool" {
		t.Errorf("expected name my_tool, got %s", b.Name)
	}
}

func TestToolResultBlock(t *testing.T) {
	b := ToolResultBlock("tool-123", "result text", false)
	if b.Type != "tool_result" {
		t.Errorf("expected type tool_result, got %s", b.Type)
	}
	if b.ToolUseID != "tool-123" {
		t.Errorf("expected tool_use_id tool-123, got %s", b.ToolUseID)
	}
	if b.IsError {
		t.Error("expected IsError false")
	}
}

func TestToolResultBlockError(t *testing.T) {
	b := ToolResultBlock("tool-456", "something broke", true)
	if !b.IsError {
		t.Error("expected IsError true")
	}
}

func TestMessageJSON(t *testing.T) {
	msg := Message{
		Role: RoleUser,
		Content: []ContentBlock{
			TextBlock("hello world"),
		},
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(data) != `{"role":"user","content":[{"type":"text","text":"hello world"}]}` {
		t.Errorf("unexpected JSON: %s", data)
	}
}

func TestMessageWithToolUseAndResult(t *testing.T) {
	msg := Message{
		Role: RoleAssistant,
		Content: []ContentBlock{
			TextBlock("let me check"),
			ToolUseBlock("call-1", "get_weather", json.RawMessage(`{"city":"SF"}`)),
		},
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded Message
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Role != RoleAssistant {
		t.Errorf("expected role assistant, got %s", decoded.Role)
	}
	if len(decoded.Content) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(decoded.Content))
	}
	if decoded.Content[0].Type != "text" {
		t.Errorf("expected first block type text, got %s", decoded.Content[0].Type)
	}
	if decoded.Content[1].Type != "tool_use" {
		t.Errorf("expected second block type tool_use, got %s", decoded.Content[1].Type)
	}
	if decoded.Content[1].Name != "get_weather" {
		t.Errorf("expected tool name get_weather, got %s", decoded.Content[1].Name)
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	origProvider := os.Getenv(EnvProvider)
	origAnthropicKey := os.Getenv(EnvAnthropicKey)
	origModel := os.Getenv(EnvModel)
	origXDG := os.Getenv("XDG_CONFIG_HOME")
	defer func() {
		restoreEnv(EnvProvider, origProvider)
		restoreEnv(EnvAnthropicKey, origAnthropicKey)
		restoreEnv(EnvModel, origModel)
		restoreEnv("XDG_CONFIG_HOME", origXDG)
	}()

	// Use temp dir to avoid reading real auth.json
	os.Setenv("XDG_CONFIG_HOME", t.TempDir())
	os.Unsetenv(EnvProvider)
	os.Setenv(EnvAnthropicKey, "test-key")
	os.Unsetenv(EnvModel)

	cfg := LoadConfig()
	if cfg.Name != "anthropic" {
		t.Errorf("expected default provider anthropic, got %s", cfg.Name)
	}
	if cfg.Model != "claude-sonnet-4-5" {
		t.Errorf("expected default model claude-sonnet-4-5, got %s", cfg.Model)
	}
	if cfg.APIKey != "test-key" {
		t.Errorf("expected api key test-key, got %s", cfg.APIKey)
	}
}

func TestLoadConfigOpenAI(t *testing.T) {
	origProvider := os.Getenv(EnvProvider)
	origOpenAIKey := os.Getenv(EnvOpenAIKey)
	origModel := os.Getenv(EnvModel)
	origXDG := os.Getenv("XDG_CONFIG_HOME")
	defer func() {
		restoreEnv(EnvProvider, origProvider)
		restoreEnv(EnvOpenAIKey, origOpenAIKey)
		restoreEnv(EnvModel, origModel)
		restoreEnv("XDG_CONFIG_HOME", origXDG)
	}()

	os.Setenv("XDG_CONFIG_HOME", t.TempDir())
	os.Setenv(EnvProvider, "openai")
	os.Setenv(EnvOpenAIKey, "sk-123")
	os.Setenv(EnvModel, "gpt-4-turbo")

	cfg := LoadConfig()
	if cfg.Name != "openai" {
		t.Errorf("expected provider openai, got %s", cfg.Name)
	}
	if cfg.Model != "gpt-4-turbo" {
		t.Errorf("expected model gpt-4-turbo, got %s", cfg.Model)
	}
}

func TestNewProviderUnknown(t *testing.T) {
	_, err := NewProvider(ProviderConfig{Name: "unknown"})
	if err == nil {
		t.Error("expected error for unknown provider")
	}
}

func TestNewProviderMissingKey(t *testing.T) {
	_, err := NewProvider(ProviderConfig{Name: "anthropic", APIKey: ""})
	if err == nil {
		t.Error("expected error for missing api key")
	}
}

func TestNewProviderAnthropic(t *testing.T) {
	p, err := NewProvider(ProviderConfig{
		Name:   "anthropic",
		APIKey: "test-key",
		Model:  "claude-sonnet-4-5",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := p.(*AnthropicProvider); !ok {
		t.Errorf("expected AnthropicProvider, got %T", p)
	}
}

func TestNewProviderOpenAI(t *testing.T) {
	p, err := NewProvider(ProviderConfig{
		Name:   "openai",
		APIKey: "sk-test",
		Model:  "gpt-4o",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := p.(*OpenAIProvider); !ok {
		t.Errorf("expected OpenAIProvider, got %T", p)
	}
}

func TestNewProviderGoogle(t *testing.T) {
	p, err := NewProvider(ProviderConfig{
		Name:   "google",
		APIKey: "test-key",
		Model:  "gemini-2.0-flash",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := p.(*GoogleProvider); !ok {
		t.Errorf("expected GoogleProvider, got %T", p)
	}
}

func TestToolDefJSON(t *testing.T) {
	td := ToolDef{
		Name:        "get_weather",
		Description: "Get the weather",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"city": map[string]any{"type": "string"},
			},
			"required": []string{"city"},
		},
	}
	data, err := json.Marshal(td)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded ToolDef
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Name != "get_weather" {
		t.Errorf("expected name get_weather, got %s", decoded.Name)
	}
}

func TestConvertToGeminiContent(t *testing.T) {
	msg := Message{
		Role: RoleUser,
		Content: []ContentBlock{
			TextBlock("hello"),
		},
	}
	gc := convertToGeminiContent(msg)
	if gc.Role != "user" {
		t.Errorf("expected role user, got %s", gc.Role)
	}
	if len(gc.Parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(gc.Parts))
	}
	if gc.Parts[0].Text != "hello" {
		t.Errorf("expected text hello, got %s", gc.Parts[0].Text)
	}
}

func TestConvertToGeminiContentAssistant(t *testing.T) {
	msg := Message{
		Role: RoleAssistant,
		Content: []ContentBlock{
			TextBlock("response"),
		},
	}
	gc := convertToGeminiContent(msg)
	if gc.Role != "model" {
		t.Errorf("expected role model, got %s", gc.Role)
	}
}

func TestConvertToGeminiContentToolUse(t *testing.T) {
	msg := Message{
		Role: RoleAssistant,
		Content: []ContentBlock{
			ToolUseBlock("call-1", "search", json.RawMessage(`{"q":"test"}`)),
		},
	}
	gc := convertToGeminiContent(msg)
	if gc.Role != "model" {
		t.Errorf("expected role model, got %s", gc.Role)
	}
	if len(gc.Parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(gc.Parts))
	}
	if gc.Parts[0].FunctionCall == nil {
		t.Fatal("expected FunctionCall, got nil")
	}
	if gc.Parts[0].FunctionCall.Name != "search" {
		t.Errorf("expected function name search, got %s", gc.Parts[0].FunctionCall.Name)
	}
}

func TestConvertToGeminiContentToolResult(t *testing.T) {
	msg := Message{
		Role: RoleUser,
		Content: []ContentBlock{
			ToolResultBlock("call-1", "sunny, 72F", false),
		},
	}
	gc := convertToGeminiContent(msg)
	if len(gc.Parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(gc.Parts))
	}
	if gc.Parts[0].FunctionResp == nil {
		t.Fatal("expected FunctionResp, got nil")
	}
	if gc.Parts[0].FunctionResp.Name != "call-1" {
		t.Errorf("expected name call-1, got %s", gc.Parts[0].FunctionResp.Name)
	}
}

func TestConvertToOAIMessage(t *testing.T) {
	msg := Message{
		Role: RoleUser,
		Content: []ContentBlock{
			TextBlock("hello"),
		},
	}
	oai := convertToOAIMessage(msg)
	if oai.Role != "user" {
		t.Errorf("expected role user, got %s", oai.Role)
	}
	// Content should be a string when single text block
	if s, ok := oai.Content.(string); !ok || s != "hello" {
		t.Errorf("expected content 'hello', got %v", oai.Content)
	}
}

func TestConvertToOAIMessageToolResult(t *testing.T) {
	msg := Message{
		Role: RoleUser,
		Content: []ContentBlock{
			ToolResultBlock("call-1", "result text", false),
		},
	}
	oai := convertToOAIMessage(msg)
	if oai.Role != "tool" {
		t.Errorf("expected role tool, got %s", oai.Role)
	}
	if oai.ToolCallID != "call-1" {
		t.Errorf("expected tool_call_id call-1, got %s", oai.ToolCallID)
	}
}

func TestConvertToOAIMessagesMultipleToolResults(t *testing.T) {
	msgs := convertToOAIMessages([]Message{{
		Role: RoleUser,
		Content: []ContentBlock{
			ToolResultBlock("call-1", "first", false),
			ToolResultBlock("call-2", "second", false),
		},
	}})
	if len(msgs) != 2 {
		t.Fatalf("expected 2 OpenAI tool messages, got %d: %#v", len(msgs), msgs)
	}
	if msgs[0].Role != "tool" || msgs[0].ToolCallID != "call-1" || msgs[0].Content != "first" {
		t.Fatalf("bad first tool message: %#v", msgs[0])
	}
	if msgs[1].Role != "tool" || msgs[1].ToolCallID != "call-2" || msgs[1].Content != "second" {
		t.Fatalf("bad second tool message: %#v", msgs[1])
	}
}

func TestOpenAIBuildRequestMultipleToolResults(t *testing.T) {
	p := NewOpenAIProvider(ProviderConfig{Name: "openai", APIKey: "sk-test", Model: "gpt-4o"})
	data, err := p.buildRequest([]Message{{
		Role: RoleUser,
		Content: []ContentBlock{
			ToolResultBlock("call-1", "first", false),
			ToolResultBlock("call-2", "second", false),
		},
	}}, "", nil, false)
	if err != nil {
		t.Fatalf("buildRequest: %v", err)
	}
	var req struct {
		Messages []oaiMessage `json:"messages"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(req.Messages) != 2 || req.Messages[0].ToolCallID != "call-1" || req.Messages[1].ToolCallID != "call-2" {
		t.Fatalf("bad messages: %#v", req.Messages)
	}
}

func TestConvertToOAIMessageToolCalls(t *testing.T) {
	msg := Message{
		Role: RoleAssistant,
		Content: []ContentBlock{
			TextBlock("checking"),
			ToolUseBlock("call-1", "search", json.RawMessage(`{"q":"test"}`)),
		},
	}
	oai := convertToOAIMessage(msg)
	if oai.Role != "assistant" {
		t.Errorf("expected role assistant, got %s", oai.Role)
	}
	if len(oai.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(oai.ToolCalls))
	}
	if oai.ToolCalls[0].ID != "call-1" {
		t.Errorf("expected tool call id call-1, got %s", oai.ToolCalls[0].ID)
	}
	if oai.ToolCalls[0].Function.Name != "search" {
		t.Errorf("expected tool call name search, got %s", oai.ToolCalls[0].Function.Name)
	}
}

func TestAnthropicBuildRequest(t *testing.T) {
	p := NewAnthropicProvider(ProviderConfig{
		Name:   "anthropic",
		APIKey: "test-key",
		Model:  "claude-sonnet-4-5",
	})

	messages := []Message{
		{Role: RoleUser, Content: []ContentBlock{TextBlock("Hello")}},
	}
	tools := []ToolDef{
		{
			Name:        "get_weather",
			Description: "Get weather for a city",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"city": map[string]any{"type": "string"},
				},
				"required": []string{"city"},
			},
		},
	}

	data, err := p.buildRequest(messages, "You are helpful.", tools, false)
	if err != nil {
		t.Fatalf("buildRequest: %v", err)
	}

	var req map[string]any
	if err := json.Unmarshal(data, &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req["model"] != "claude-sonnet-4-5" {
		t.Errorf("expected model claude-sonnet-4-5, got %v", req["model"])
	}
	if _, ok := req["stream"]; ok {
		// stream field should be omitted when false
		t.Errorf("expected stream to be omitted when false, got %v", req["stream"])
	}
	sys, ok := req["system"].([]any)
	if !ok || len(sys) == 0 {
		t.Errorf("expected system blocks, got %v", req["system"])
	}
}

func TestOpenAIChatCompletionsURL(t *testing.T) {
	tests := []struct {
		base string
		want string
	}{
		{"https://api.openai.com", "https://api.openai.com/v1/chat/completions"},
		{"https://opencode.ai/zen/v1", "https://opencode.ai/zen/v1/chat/completions"},
		{"https://openrouter.ai/api/v1/", "https://openrouter.ai/api/v1/chat/completions"},
	}
	for _, tt := range tests {
		if got := openAIChatCompletionsURL(tt.base); got != tt.want {
			t.Errorf("openAIChatCompletionsURL(%q) = %q, want %q", tt.base, got, tt.want)
		}
	}
}

func TestOpenAIDoRequestDoesNotDoubleV1(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`)
	}))
	defer srv.Close()

	p := NewOpenAIProvider(ProviderConfig{Name: "openai", APIKey: "sk-test", Model: "gpt-4o", BaseURL: srv.URL + "/v1"})
	msg, err := p.CreateMessage(context.Background(), []Message{{Role: RoleUser, Content: []ContentBlock{TextBlock("hello")}}}, "", nil)
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	if len(msg.Content) != 1 || msg.Content[0].Text != "ok" {
		t.Fatalf("bad message: %#v", msg)
	}
}

func TestOpenAIBuildRequest(t *testing.T) {
	p := NewOpenAIProvider(ProviderConfig{
		Name:   "openai",
		APIKey: "sk-test",
		Model:  "gpt-4o",
	})

	messages := []Message{
		{Role: RoleUser, Content: []ContentBlock{TextBlock("Hello")}},
	}
	tools := []ToolDef{
		{
			Name:        "get_weather",
			Description: "Get weather for a city",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"city": map[string]any{"type": "string"},
				},
				"required": []string{"city"},
			},
		},
	}

	data, err := p.buildRequest(messages, "You are helpful.", tools, false)
	if err != nil {
		t.Fatalf("buildRequest: %v", err)
	}

	var req map[string]any
	if err := json.Unmarshal(data, &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req["model"] != "gpt-4o" {
		t.Errorf("expected model gpt-4o, got %v", req["model"])
	}
	msgs, ok := req["messages"].([]any)
	if !ok || len(msgs) < 2 {
		t.Errorf("expected at least 2 messages (system+user), got %v", req["messages"])
	}
}

func TestOpenAIStreamAccumulatesToolCallsByIndex(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call-1\",\"type\":\"function\",\"function\":{\"name\":\"replace_text\",\"arguments\":\"{\\\"old\\\":\\\"\"}}]}}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"x\\\",\\\"new\\\":\\\"y\\\"}\"}}]}}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n")
	}))
	defer srv.Close()

	p := NewOpenAIProvider(ProviderConfig{Name: "openai", APIKey: "sk-test", Model: "gpt-4o", BaseURL: srv.URL})
	ch := p.StreamMessages(context.Background(), []Message{{Role: RoleUser, Content: []ContentBlock{TextBlock("edit")}}}, "", nil)

	var toolEvents []StreamEvent
	for ev := range ch {
		if ev.Type == EventError {
			t.Fatalf("stream error: %v", ev.Error)
		}
		if ev.Type == EventToolUse {
			toolEvents = append(toolEvents, ev)
		}
	}
	if len(toolEvents) != 1 {
		t.Fatalf("expected 1 tool event, got %#v", toolEvents)
	}
	if toolEvents[0].ToolID != "call-1" || toolEvents[0].ToolName != "replace_text" {
		t.Fatalf("bad tool event metadata: %#v", toolEvents[0])
	}
	if !strings.Contains(toolEvents[0].Text, `"old":"x"`) || !strings.Contains(toolEvents[0].Text, `"new":"y"`) {
		t.Fatalf("bad accumulated args: %q", toolEvents[0].Text)
	}
}

func TestOpenAIStreamEmitsReasoningDelta(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"thinking\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"done\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
	}))
	defer srv.Close()

	p := NewOpenAIProvider(ProviderConfig{Name: "openai", APIKey: "sk-test", Model: "gpt-4o", BaseURL: srv.URL})
	ch := p.StreamMessages(context.Background(), []Message{{Role: RoleUser, Content: []ContentBlock{TextBlock("hello")}}}, "", nil)

	var sawHeaders, sawRaw, sawThinking bool
	for ev := range ch {
		if ev.Type == EventError {
			t.Fatalf("stream error: %v", ev.Error)
		}
		switch ev.Type {
		case EventResponseHeaders:
			sawHeaders = ev.StatusCode == http.StatusOK
		case EventRawEvent:
			sawRaw = true
		case EventThinkingDelta:
			sawThinking = ev.Text == "thinking"
		}
	}
	if !sawHeaders || !sawRaw || !sawThinking {
		t.Fatalf("expected headers/raw/thinking events, got headers=%v raw=%v thinking=%v", sawHeaders, sawRaw, sawThinking)
	}
}

func TestOpenCodeOpenAIHeadersAndCompat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-opencode-session"); got != "session-1" {
			t.Fatalf("expected x-opencode-session session-1, got %q", got)
		}
		if got := r.Header.Get("x-opencode-client"); got != "pi" {
			t.Fatalf("expected x-opencode-client pi, got %q", got)
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if _, ok := req["max_tokens"]; !ok {
			t.Fatalf("expected max_tokens in request: %#v", req)
		}
		if _, ok := req["max_completion_tokens"]; ok {
			t.Fatalf("did not expect max_completion_tokens in request: %#v", req)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\n")
	}))
	defer srv.Close()

	p := NewOpenAIProvider(ProviderConfig{Name: "opencode-go", APIKey: "test-key", Model: "glm-5.1", BaseURL: srv.URL})
	ctx := WithSessionID(context.Background(), "session-1")
	for ev := range p.StreamMessages(ctx, []Message{{Role: RoleUser, Content: []ContentBlock{TextBlock("hello")}}}, "", nil) {
		if ev.Type == EventError {
			t.Fatalf("stream error: %v", ev.Error)
		}
	}
}

func TestOpenCodeResponsesModelIsUnsupported(t *testing.T) {
	_, err := NewProvider(ProviderConfig{Name: "opencode", APIKey: "test-key", Model: "gpt-5.1"})
	if err == nil || !strings.Contains(err.Error(), "openai-responses") {
		t.Fatalf("expected openai-responses unsupported error, got %v", err)
	}
}

func TestGoogleBuildRequest(t *testing.T) {
	p := NewGoogleProvider(ProviderConfig{
		Name:   "google",
		APIKey: "test-key",
		Model:  "gemini-2.0-flash",
	})

	messages := []Message{
		{Role: RoleUser, Content: []ContentBlock{TextBlock("Hello")}},
	}
	tools := []ToolDef{
		{
			Name:        "get_weather",
			Description: "Get weather for a city",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"city": map[string]any{"type": "string"},
				},
				"required": []string{"city"},
			},
		},
	}

	data, err := p.buildRequest(messages, "You are helpful.", tools, false)
	if err != nil {
		t.Fatalf("buildRequest: %v", err)
	}

	var req map[string]any
	if err := json.Unmarshal(data, &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	contents, ok := req["contents"].([]any)
	if !ok || len(contents) < 1 {
		t.Errorf("expected contents, got %v", req["contents"])
	}
}

func TestIsAnthropicModel(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		{"claude-sonnet-4-5", true},
		{"claude-opus-4-7", true},
		{"claude-haiku-4-5", true},
		{"deepseek-v4-flash", false},
		{"gpt-4o", false},
		{"qwen3.7-max", true},
		{"qwen3.7-plus", true},
		{"qwen3.6-plus", false},
		{"minimax-m3", true},
		{"minimax-m2.7", false},
	}
	for _, tt := range tests {
		got := isAnthropicModel(tt.model)
		if got != tt.want {
			t.Errorf("isAnthropicModel(%q) = %v, want %v", tt.model, got, tt.want)
		}
	}
}

func TestOpenCodeProviderClaudeModel(t *testing.T) {
	p, err := NewProvider(ProviderConfig{
		Name:    "opencode",
		APIKey:  "test-key",
		Model:   "claude-sonnet-4-5",
		BaseURL: "https://opencode.ai/zen",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ap, ok := p.(*AnthropicProvider)
	if !ok {
		t.Fatalf("expected AnthropicProvider for opencode claude model, got %T", p)
	}
	if ap.baseURL != "https://opencode.ai/zen" {
		t.Errorf("expected base URL https://opencode.ai/zen, got %s", ap.baseURL)
	}
}

func TestOpenCodeProviderOpenAIModel(t *testing.T) {
	p, err := NewProvider(ProviderConfig{
		Name:    "opencode",
		APIKey:  "test-key",
		Model:   "deepseek-v4-flash",
		BaseURL: "https://opencode.ai/zen",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	op, ok := p.(*OpenAIProvider)
	if !ok {
		t.Fatalf("expected OpenAIProvider for opencode deepseek model, got %T", p)
	}
	if op.baseURL != "https://opencode.ai/zen/v1" {
		t.Errorf("expected base URL https://opencode.ai/zen/v1, got %s", op.baseURL)
	}
}

func TestOpenCodeGoProviderClaudeModel(t *testing.T) {
	p, err := NewProvider(ProviderConfig{
		Name:    "opencode-go",
		APIKey:  "test-key",
		Model:   "minimax-m3",
		BaseURL: "https://opencode.ai/zen/go/v1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ap, ok := p.(*AnthropicProvider)
	if !ok {
		t.Fatalf("expected AnthropicProvider for opencode-go anthropic model, got %T", p)
	}
	if ap.baseURL != "https://opencode.ai/zen/go" {
		t.Errorf("expected base URL https://opencode.ai/zen/go, got %s", ap.baseURL)
	}
}

func TestOpenCodeGoProviderOpenAIModel(t *testing.T) {
	p, err := NewProvider(ProviderConfig{
		Name:    "opencode-go",
		APIKey:  "test-key",
		Model:   "deepseek-v4-flash",
		BaseURL: "https://opencode.ai/zen/go/v1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	op, ok := p.(*OpenAIProvider)
	if !ok {
		t.Fatalf("expected OpenAIProvider for opencode-go deepseek model, got %T", p)
	}
	if op.baseURL != "https://opencode.ai/zen/go/v1" {
		t.Errorf("expected base URL https://opencode.ai/zen/go/v1, got %s", op.baseURL)
	}
}

func TestOpenCodeGoKnownPiRoutingRows(t *testing.T) {
	tests := []struct {
		model string
		want  string
	}{
		{"glm-5.1", "openai-completions"},
		{"deepseek-v4-flash", "openai-completions"},
		{"minimax-m3", "anthropic-messages"},
		{"qwen3.7-plus", "anthropic-messages"},
	}
	for _, tt := range tests {
		if got := OpenCodeModelAPIForInfo("opencode-go", tt.model); got != tt.want {
			t.Fatalf("opencode-go/%s routed to %s, want %s", tt.model, got, tt.want)
		}
	}
}

func TestLoadConfigOpenCode(t *testing.T) {
	origProvider := os.Getenv(EnvProvider)
	origOpenCodeKey := os.Getenv(EnvOpenCodeKey)
	origModel := os.Getenv(EnvModel)
	origHome := os.Getenv("HOME")
	defer func() {
		restoreEnv(EnvProvider, origProvider)
		restoreEnv(EnvOpenCodeKey, origOpenCodeKey)
		restoreEnv(EnvModel, origModel)
		os.Setenv("HOME", origHome)
	}()

	// Use temp dir to avoid touching real auth.json
	tmpDir := t.TempDir()
	os.Setenv("XDG_CONFIG_HOME", tmpDir)

	os.Setenv(EnvProvider, "opencode")
	os.Setenv(EnvOpenCodeKey, "zen-test-key")
	os.Setenv(EnvModel, "claude-sonnet-4-5")

	cfg := LoadConfig()
	if cfg.Name != "opencode" {
		t.Errorf("expected provider opencode, got %s", cfg.Name)
	}
	if cfg.APIKey != "zen-test-key" {
		t.Errorf("expected api key zen-test-key, got %s", cfg.APIKey)
	}
	if cfg.BaseURL != "https://opencode.ai/zen" {
		t.Errorf("expected base URL https://opencode.ai/zen, got %s", cfg.BaseURL)
	}
}

func TestLoadConfigOpenCodeBaseURLOverrideSurvivesProviderRouting(t *testing.T) {
	origProvider := os.Getenv(EnvProvider)
	origOpenCodeKey := os.Getenv(EnvOpenCodeKey)
	origModel := os.Getenv(EnvModel)
	origBaseURL := os.Getenv("OPENCODE_BASE_URL")
	origXDG := os.Getenv("XDG_CONFIG_HOME")
	defer func() {
		restoreEnv(EnvProvider, origProvider)
		restoreEnv(EnvOpenCodeKey, origOpenCodeKey)
		restoreEnv(EnvModel, origModel)
		restoreEnv("OPENCODE_BASE_URL", origBaseURL)
		restoreEnv("XDG_CONFIG_HOME", origXDG)
	}()

	os.Setenv("XDG_CONFIG_HOME", t.TempDir())
	os.Setenv(EnvProvider, "opencode")
	os.Setenv(EnvOpenCodeKey, "zen-test-key")
	os.Setenv(EnvModel, "deepseek-v4-flash")
	os.Setenv("OPENCODE_BASE_URL", "https://proxy.example/zen")

	cfg := LoadConfig()
	p, err := NewProvider(cfg)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	op, ok := p.(*OpenAIProvider)
	if !ok {
		t.Fatalf("expected OpenAIProvider, got %T", p)
	}
	if op.baseURL != "https://proxy.example/zen" {
		t.Errorf("expected env base URL override to survive, got %s", op.baseURL)
	}
}

func TestLoadConfigOpenCodeFallbackKey(t *testing.T) {
	origProvider := os.Getenv(EnvProvider)
	origOpenCodeKey := os.Getenv(EnvOpenCodeKey)
	origAnthropicKey := os.Getenv(EnvAnthropicKey)
	origModel := os.Getenv(EnvModel)
	defer func() {
		restoreEnv(EnvProvider, origProvider)
		restoreEnv(EnvOpenCodeKey, origOpenCodeKey)
		restoreEnv(EnvAnthropicKey, origAnthropicKey)
		restoreEnv(EnvModel, origModel)
	}()

	os.Setenv(EnvProvider, "opencode")
	os.Unsetenv(EnvOpenCodeKey)
	os.Setenv(EnvAnthropicKey, "anthropic-fallback-key")

	cfg := LoadConfig()
	if cfg.APIKey != "anthropic-fallback-key" {
		t.Errorf("expected fallback to anthropic key, got %s", cfg.APIKey)
	}
}

func TestAuthFileReadWrite(t *testing.T) {
	origHome := os.Getenv("HOME")
	os.Setenv("XDG_CONFIG_HOME", t.TempDir())
	defer os.Setenv("HOME", origHome)

	auth := NewAuthFile()
	auth.Set("anthropic", AuthFileEntry{APIKey: "sk-ant-test123"})
	auth.Set("opencode", AuthFileEntry{APIKey: "zen-key", Model: "claude-sonnet-4-5"})

	if err := WriteAuthFile(auth); err != nil {
		t.Fatalf("write auth file: %v", err)
	}

	loaded, err := ReadAuthFile()
	if err != nil {
		t.Fatalf("read auth file: %v", err)
	}

	if entry, ok := loaded.Get("anthropic"); !ok || entry.APIKey != "sk-ant-test123" {
		t.Errorf("expected anthropic key sk-ant-test123, got %#v", entry)
	}
	if entry, ok := loaded.Get("opencode"); !ok || entry.Model != "claude-sonnet-4-5" {
		t.Errorf("expected opencode model claude-sonnet-4-5, got %#v", entry)
	}
}

func TestLoadConfigFromAuthFile(t *testing.T) {
	origProvider := os.Getenv(EnvProvider)
	origAnthropicKey := os.Getenv(EnvAnthropicKey)
	origHome := os.Getenv("HOME")
	defer func() {
		restoreEnv(EnvProvider, origProvider)
		restoreEnv(EnvAnthropicKey, origAnthropicKey)
		os.Setenv("HOME", origHome)
	}()

	tmpDir := t.TempDir()
	os.Setenv("XDG_CONFIG_HOME", tmpDir)
	os.Unsetenv(EnvProvider)
	os.Unsetenv(EnvAnthropicKey)

	auth := NewAuthFile()
	auth.ActiveProvider = "opencode"
	auth.Set("opencode", AuthFileEntry{APIKey: "from-auth-file", Model: "claude-opus-4-5"})
	if err := WriteAuthFile(auth); err != nil {
		t.Fatalf("write: %v", err)
	}

	cfg := LoadConfig()
	if cfg.Name != "opencode" {
		t.Errorf("expected provider from auth.json _active_provider, got %s", cfg.Name)
	}
	if cfg.APIKey != "from-auth-file" {
		t.Errorf("expected key from auth.json, got %s", cfg.APIKey)
	}
	if cfg.Model != "claude-opus-4-5" {
		t.Errorf("expected model from auth.json, got %s", cfg.Model)
	}
}

func TestAuthFileOverridesEnvOverAuthFile(t *testing.T) {
	origProvider := os.Getenv(EnvProvider)
	origAnthropicKey := os.Getenv(EnvAnthropicKey)
	origHome := os.Getenv("HOME")
	defer func() {
		restoreEnv(EnvProvider, origProvider)
		restoreEnv(EnvAnthropicKey, origAnthropicKey)
		os.Setenv("HOME", origHome)
	}()

	tmpDir := t.TempDir()
	os.Setenv("XDG_CONFIG_HOME", tmpDir)
	os.Setenv(EnvProvider, "anthropic")
	os.Setenv(EnvAnthropicKey, "env-key-wins")

	auth := NewAuthFile()
	auth.Set("anthropic", AuthFileEntry{APIKey: "auth-file-key", Model: "old-model"})
	if err := WriteAuthFile(auth); err != nil {
		t.Fatalf("write: %v", err)
	}

	cfg := LoadConfig()
	if cfg.APIKey != "env-key-wins" {
		t.Errorf("env var should override auth.json, got %s", cfg.APIKey)
	}
}

func TestProviderDetails(t *testing.T) {
	details := ProviderDetails()
	if len(details) != 5 {
		t.Errorf("expected 5 providers, got %d", len(details))
	}
	providers := ListProviders()
	if len(providers) != 5 {
		t.Errorf("expected 5 provider names, got %d", len(providers))
	}
}

func TestProviderConfigAPIField(t *testing.T) {
	p, err := NewProvider(ProviderConfig{
		Name:    "opencode",
		APIKey:  "test-key",
		Model:   "big-pickle",
		BaseURL: "https://opencode.ai/zen",
		API:     "anthropic-messages",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ap, ok := p.(*AnthropicProvider)
	if !ok {
		t.Errorf("expected AnthropicProvider when API forced to anthropic-messages, got %T", p)
	}
	if ap.baseURL != "https://opencode.ai/zen" {
		t.Errorf("expected anthropic base URL from explicit API, got %s", ap.baseURL)
	}
}

func TestOpenCodePreservesExplicitBaseURLOverride(t *testing.T) {
	p, err := NewProvider(ProviderConfig{
		Name:    "opencode",
		APIKey:  "test-key",
		Model:   "deepseek-v4-flash",
		BaseURL: "https://proxy.example/opencode",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	op, ok := p.(*OpenAIProvider)
	if !ok {
		t.Fatalf("expected OpenAIProvider, got %T", p)
	}
	if op.baseURL != "https://proxy.example/opencode" {
		t.Errorf("expected explicit base URL to survive, got %s", op.baseURL)
	}
}

func TestOpenCodeExplicitAPIChoosesMatchingDefaultBaseURL(t *testing.T) {
	p, err := NewProvider(ProviderConfig{
		Name:    "opencode-go",
		APIKey:  "test-key",
		Model:   "minimax-m3",
		BaseURL: "https://opencode.ai/zen/go",
		API:     "openai-completions",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	op, ok := p.(*OpenAIProvider)
	if !ok {
		t.Fatalf("expected OpenAIProvider, got %T", p)
	}
	if op.baseURL != "https://opencode.ai/zen/go/v1" {
		t.Errorf("expected OpenAI base URL from explicit API, got %s", op.baseURL)
	}
}

func TestOpenCodeModelRoutingCoversGoogle(t *testing.T) {
	p, err := NewProvider(ProviderConfig{Name: "opencode", APIKey: "test-key", Model: "gemini-3-flash"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	gp, ok := p.(*GoogleProvider)
	if !ok {
		t.Fatalf("expected GoogleProvider, got %T", p)
	}
	if gp.baseURL != "https://opencode.ai/zen/v1" {
		t.Errorf("expected OpenCode /v1 base for google route, got %s", gp.baseURL)
	}
}

func TestGeminiEndpoint(t *testing.T) {
	ep := geminiEndpoint("gemini-2.0-flash", "generateContent")
	if ep != "/v1beta/models/gemini-2.0-flash:generateContent" {
		t.Errorf("unexpected endpoint: %s", ep)
	}
	ep2 := geminiEndpoint("gemini-2.0-flash", "streamGenerateContent")
	if ep2 != "/v1beta/models/gemini-2.0-flash:streamGenerateContent" {
		t.Errorf("unexpected endpoint: %s", ep2)
	}
}

func TestGeminiStreamRequestsSSE(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("alt"); got != "sse" {
			t.Fatalf("expected alt=sse, got %q in %s", got, r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"hi\"}]},\"finishReason\":\"STOP\"}]}\n\n")
	}))
	defer srv.Close()

	p := NewGoogleProvider(ProviderConfig{Name: "google", APIKey: "test-key", Model: "gemini-2.0-flash", BaseURL: srv.URL})
	ch := p.StreamMessages(context.Background(), []Message{{Role: RoleUser, Content: []ContentBlock{TextBlock("hello")}}}, "", nil)
	var sawText bool
	for ev := range ch {
		if ev.Type == EventError {
			t.Fatalf("stream error: %v", ev.Error)
		}
		if ev.Type == EventTextDelta && ev.Text == "hi" {
			sawText = true
		}
	}
	if !sawText {
		t.Fatal("expected streamed text")
	}
}

func restoreEnv(key, origValue string) {
	if origValue == "" {
		os.Unsetenv(key)
	} else {
		os.Setenv(key, origValue)
	}
}

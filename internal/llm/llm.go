// Package llm provides a multi-provider LLM abstraction for the daemon project.
// It defines SDK-agnostic types and a Provider interface so that agent loop code
// can work with Anthropic, OpenAI-compatible, and Google Gemini providers
// without depending on any particular SDK.
package llm

import (
	"context"
	"encoding/json"
)

type requestContextKey string

const sessionIDContextKey requestContextKey = "session_id"

// WithSessionID annotates provider requests with the daemon session ID.
// Providers that need routing/session affinity headers can read it without
// storing per-request state globally.
func WithSessionID(ctx context.Context, sessionID string) context.Context {
	if sessionID == "" {
		return ctx
	}
	return context.WithValue(ctx, sessionIDContextKey, sessionID)
}

func sessionIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(sessionIDContextKey).(string); ok {
		return v
	}
	return ""
}

// ---------------------------------------------------------------------------
// Core message types
// ---------------------------------------------------------------------------

// Role identifies who authored a message.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Message is a single turn in a conversation.
type Message struct {
	Role    Role           `json:"role"`
	Content []ContentBlock `json:"content"`
}

// ContentBlock represents one block inside a message.
// The Type field determines which other fields are populated.
type ContentBlock struct {
	Type string `json:"type"` // "text", "tool_use", "tool_result"

	// Populated when Type == "text"
	Text string `json:"text,omitempty"`

	// Populated when Type == "tool_use"
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	// Populated when Type == "tool_result"
	ToolUseID string `json:"tool_use_id,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
}

// Helper constructors

func TextBlock(text string) ContentBlock {
	return ContentBlock{Type: "text", Text: text}
}

func ToolUseBlock(id, name string, input json.RawMessage) ContentBlock {
	return ContentBlock{Type: "tool_use", ID: id, Name: name, Input: input}
}

func ToolResultBlock(toolUseID, content string, isError bool) ContentBlock {
	return ContentBlock{Type: "tool_result", ToolUseID: toolUseID, Text: content, IsError: isError}
}

// ---------------------------------------------------------------------------
// Tool definition
// ---------------------------------------------------------------------------

// ToolDef describes a tool that can be invoked by the model.
type ToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema"`
}

// ---------------------------------------------------------------------------
// Streaming events
// ---------------------------------------------------------------------------

// StreamEvent is sent over the channel returned by StreamMessages.
type StreamEvent struct {
	// Type is one of: "response_headers", "raw_event", "text_delta",
	// "thinking_delta", "tool_use", "stop", "error".
	Type string `json:"type"`

	// Populated when Type == "text_delta" or "thinking_delta".
	Text string `json:"text,omitempty"`

	// Populated when Type == "tool_use" — streamed partial JSON for tool input.
	ToolID   string `json:"tool_id,omitempty"`
	ToolName string `json:"tool_name,omitempty"`

	// Populated when Type == "stop".
	StopReason string `json:"stop_reason,omitempty"`

	// Populated when Type == "response_headers".
	StatusCode int `json:"status_code,omitempty"`

	// Populated when Type == "error".
	Error error `json:"-"`
}

// StreamEvent types
const (
	EventResponseHeaders = "response_headers"
	EventRawEvent        = "raw_event"
	EventTextDelta       = "text_delta"
	EventThinkingDelta   = "thinking_delta"
	EventToolUse         = "tool_use"
	EventStop            = "stop"
	EventError           = "error"
)

// ---------------------------------------------------------------------------
// Provider interface
// ---------------------------------------------------------------------------

// Provider is the interface that all LLM backends implement.
type Provider interface {
	// StreamMessages sends messages to the model and returns a channel of
	// streaming events. The channel is closed when the stream ends.
	// The caller should drain the channel and check for error events.
	StreamMessages(ctx context.Context, messages []Message, systemPrompt string, tools []ToolDef) <-chan StreamEvent

	// CreateMessage sends messages and returns the complete response.
	CreateMessage(ctx context.Context, messages []Message, systemPrompt string, tools []ToolDef) (*Message, error)

	// Close releases any resources held by the provider.
	Close() error
}

// ---------------------------------------------------------------------------
// Config
// ---------------------------------------------------------------------------

// ThinkingLevel controls how much reasoning/extended thinking the model does.
// Maps to different provider-specific params:
//
//	Anthropic: thinking.enabled/adaptive + budget or effort
//	OpenAI:    reasoning_effort
//	Google:    thinkingConfig.thinkingLevel
type ThinkingLevel string

const (
	ThinkingOff    ThinkingLevel = "off"
	ThinkingLow    ThinkingLevel = "low"
	ThinkingMedium ThinkingLevel = "medium"
	ThinkingHigh   ThinkingLevel = "high"
	ThinkingXHigh  ThinkingLevel = "xhigh"
)

func ValidThinkingLevels() []ThinkingLevel {
	return []ThinkingLevel{ThinkingOff, ThinkingLow, ThinkingMedium, ThinkingHigh, ThinkingXHigh}
}

func ParseThinkingLevel(s string) ThinkingLevel {
	for _, l := range ValidThinkingLevels() {
		if string(l) == s {
			return l
		}
	}
	return ""
}

// ProviderConfig holds everything needed to instantiate a provider.
type ProviderConfig struct {
	Name            string        `json:"name"` // "anthropic", "openai", "google", "opencode", "opencode-go"
	APIKey          string        `json:"api_key"`
	BaseURL         string        `json:"base_url"` // optional override
	BaseURLExplicit bool          `json:"-"`        // true when BaseURL came from env/auth instead of defaults
	Model           string        `json:"model"`    // model identifier
	API             string        `json:"api"`      // optional: "anthropic-messages", "openai-completions" to force API type
	Thinking        ThinkingLevel `json:"thinking"` // reasoning effort level
}

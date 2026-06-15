package llm

// ---------------------------------------------------------------------------
// OpenCode Zen and OpenCode Go providers
//
// These are routing gateways (proxies) that serve multiple API shapes
// depending on model. The model name or explicit API field determines
// which underlying provider (Anthropic, OpenAI, Google) handles the request.
//
// Base URLs:
//
//	opencode:    https://opencode.ai/zen      (anthropic-messages)
//	             https://opencode.ai/zen/v1   (openai-completions/responses, google)
//	opencode-go: https://opencode.ai/zen/go   (anthropic-messages)
//	             https://opencode.ai/zen/go/v1 (openai-completions/responses, google)
// ---------------------------------------------------------------------------

import (
	"fmt"
	"strings"
)

// openCodeBaseURLs maps the default endpoints for each OpenCode provider.
var openCodeBaseURLs = map[string]openCodeBase{
	"opencode":    {anthropic: "https://opencode.ai/zen", openai: "https://opencode.ai/zen/v1"},
	"opencode-go": {anthropic: "https://opencode.ai/zen/go", openai: "https://opencode.ai/zen/go/v1"},
}

type openCodeBase struct {
	anthropic string
	openai    string
}

// openCodeAPITypes maps Pi's API type strings to daemon's provider type.
var openCodeAPITypes = map[string]string{
	"anthropic-messages":   "anthropic",
	"openai-completions":   "openai",
	"google-generative-ai": "google",
}

// modelAPIByProvider maps (provider, modelID) -> Pi API type.
// This is needed because the same model ID can map to different API types
// depending on provider (e.g., qwen3.6-plus is anthropic-messages on opencode
// but openai-completions on opencode-go).
var modelAPIByProvider = map[string]map[string]string{
	"opencode": {
		// Anthropic Messages
		"claude-haiku-4-5":  "anthropic-messages",
		"claude-opus-4-1":   "anthropic-messages",
		"claude-opus-4-5":   "anthropic-messages",
		"claude-opus-4-6":   "anthropic-messages",
		"claude-opus-4-7":   "anthropic-messages",
		"claude-opus-4-8":   "anthropic-messages",
		"claude-sonnet-4":   "anthropic-messages",
		"claude-sonnet-4-5": "anthropic-messages",
		"claude-sonnet-4-6": "anthropic-messages",
		"minimax-m3":        "anthropic-messages",
		"qwen3.5-plus":      "anthropic-messages",
		"qwen3.6-plus":      "anthropic-messages",
		"qwen3.7-max":       "anthropic-messages",
		"qwen3.7-plus":      "anthropic-messages",

		// OpenAI Completions
		"big-pickle":        "openai-completions",
		"deepseek-v4-flash": "openai-completions",
		"deepseek-v4-pro":   "openai-completions",
		"glm-5":             "openai-completions",
		"glm-5.1":           "openai-completions",
		"kimi-k2.5":         "openai-completions",
		"kimi-k2.6":         "openai-completions",
		"mimo-v2.5":         "openai-completions",
		"mimo-v2.5-pro":     "openai-completions",
		"minimax-m2.5":      "openai-completions",
		"minimax-m2.7":      "openai-completions",

		// OpenAI Responses (GPT-5+ Codex)
		"gpt-5":              "openai-responses",
		"gpt-5-codex":        "openai-responses",
		"gpt-5-nano":         "openai-responses",
		"gpt-5.1":            "openai-responses",
		"gpt-5.1-codex":      "openai-responses",
		"gpt-5.1-codex-max":  "openai-responses",
		"gpt-5.1-codex-mini": "openai-responses",
		"gpt-5.2":            "openai-responses",
		"gpt-5.2-codex":      "openai-responses",
		"gpt-5.3-codex":      "openai-responses",
		"gpt-5.4":            "openai-responses",
		"gpt-5.4-mini":       "openai-responses",
		"gpt-5.4-nano":       "openai-responses",
		"gpt-5.4-pro":        "openai-responses",
		"gpt-5.5":            "openai-responses",
		"gpt-5.5-pro":        "openai-responses",

		// Google Generative AI
		"gemini-3-flash":   "google-generative-ai",
		"gemini-3.1-pro":   "google-generative-ai",
		"gemini-3.5-flash": "google-generative-ai",
	},
	"opencode-go": {
		// Anthropic Messages — models that use /zen/go base URL
		"anthropic/claude-3.5-haiku":      "anthropic-messages",
		"anthropic/claude-3-haiku":        "anthropic-messages",
		"anthropic/claude-haiku-4.5":      "anthropic-messages",
		"anthropic/claude-opus-4.1":       "anthropic-messages",
		"anthropic/claude-opus-4.5":       "anthropic-messages",
		"anthropic/claude-opus-4.6":       "anthropic-messages",
		"anthropic/claude-opus-4.7":       "anthropic-messages",
		"anthropic/claude-opus-4.8":       "anthropic-messages",
		"anthropic/claude-opus-4":         "anthropic-messages",
		"anthropic/claude-sonnet-4.5":     "anthropic-messages",
		"anthropic/claude-sonnet-4.6":     "anthropic-messages",
		"anthropic/claude-sonnet-4":       "anthropic-messages",
		"alibaba/qwen3.7-max":             "anthropic-messages",
		"alibaba/qwen3.7-plus":            "anthropic-messages",
		"alibaba/qwen3.6-plus":            "anthropic-messages",
		"alibaba/qwen3.5-flash":           "anthropic-messages",
		"alibaba/qwen3.5-plus":            "anthropic-messages",
		"deepseek/deepseek-r1":            "anthropic-messages",
		"deepseek/deepseek-v3.1":          "anthropic-messages",
		"deepseek/deepseek-v4-flash":      "anthropic-messages",
		"deepseek/deepseek-v4-pro":        "anthropic-messages",
		"google/gemini-2.5-flash":         "anthropic-messages",
		"google/gemini-2.5-pro":           "anthropic-messages",
		"google/gemini-3-flash":           "anthropic-messages",
		"google/gemini-3.1-pro-preview":   "anthropic-messages",
		"google/gemini-3.5-flash":         "anthropic-messages",
		"kimi-k2.7-code":                  "anthropic-messages",
		"minimax-m3":                      "anthropic-messages",
		"minimax/minimax-m3":              "anthropic-messages",
		"qwen3.7-max":                     "anthropic-messages",
		"qwen3.7-plus":                    "anthropic-messages",
		"xai/grok-4.1-fast-non-reasoning": "anthropic-messages",
		"xai/grok-4.1-fast-reasoning":     "anthropic-messages",
		"xai/grok-4.20-multi-agent":       "anthropic-messages",
		"xai/grok-4.20-reasoning":         "anthropic-messages",
		"xai/grok-4.3":                    "anthropic-messages",
		"xiaomi/mimo-v2.5":                "anthropic-messages",
		"zai/glm-5":                       "anthropic-messages",
		"zai/glm-5.1":                     "anthropic-messages",

		// OpenAI Completions — models that use /zen/go/v1 base URL
		"deepseek-v4-flash":        "openai-completions",
		"deepseek-v4-pro":          "openai-completions",
		"glm-5":                    "openai-completions",
		"glm-5.1":                  "openai-completions",
		"kimi-k2.6":                "openai-completions",
		"mimo-v2.5":                "openai-completions",
		"mimo-v2.5-pro":            "openai-completions",
		"minimax-m2.7":             "openai-completions",
		"qwen3.6-plus":             "openai-completions",
		"alibaba/qwen3-coder":      "openai-completions",
		"alibaba/qwen3-coder-next": "openai-completions",
		"openai/gpt-5":             "openai-completions",
		"openai/gpt-5.1":           "openai-completions",
		"openai/o3":                "openai-completions",
	},
}

// OpenCodeModelAPIForInfo returns the API type for a model under an OpenCode provider.
// It is exported so surfaces can display the same routing decision as Go.
func OpenCodeModelAPIForInfo(provider, model string) string {
	return openCodeModelAPI(provider, model)
}

// openCodeModelAPI returns the API type for a given model under a provider.
// Falls back to prefix-based heuristic if the model isn't in the catalog.
func openCodeModelAPI(provider, model string) string {
	if catalog, ok := modelAPIByProvider[provider]; ok {
		if api, ok := catalog[model]; ok {
			return api
		}
	}
	// Fallback: prefix-based detection
	if isAnthropicModel(model) {
		return "anthropic-messages"
	}
	if strings.HasPrefix(model, "gemini-") || strings.HasPrefix(model, "google/gemini-") {
		return "google-generative-ai"
	}
	// Most models go through OpenAI completions
	return "openai-completions"
}

// openCodeBaseForAPI returns the default base URL for a resolved API type under
// an OpenCode provider. Anthropic-messages models use the bare endpoint;
// everything else uses the /v1 variant.
func openCodeBaseForAPI(provider, api string) string {
	bases, ok := openCodeBaseURLs[provider]
	if !ok {
		bases = openCodeBaseURLs["opencode"]
	}
	if api == "anthropic-messages" {
		return bases.anthropic
	}
	return bases.openai
}

type resolvedOpenCode struct {
	backend string
	baseURL string
}

func resolveOpenCode(provider string, cfg ProviderConfig) resolvedOpenCode {
	api := cfg.API
	if api == "" {
		api = openCodeModelAPI(provider, cfg.Model)
	}
	backend := openCodeAPITypes[api]
	if backend == "" {
		backend = "openai"
	}

	baseURL := cfg.BaseURL
	if !cfg.BaseURLExplicit && (baseURL == "" || isOpenCodeDefaultBase(provider, baseURL)) {
		baseURL = openCodeBaseForAPI(provider, api)
	}
	return resolvedOpenCode{backend: backend, baseURL: baseURL}
}

func isOpenCodeDefaultBase(provider, baseURL string) bool {
	baseURL = strings.TrimRight(baseURL, "/")
	bases, ok := openCodeBaseURLs[provider]
	if !ok {
		bases = openCodeBaseURLs["opencode"]
	}
	return baseURL == strings.TrimRight(bases.anthropic, "/") || baseURL == strings.TrimRight(bases.openai, "/")
}

// newOpenCodeProvider creates a provider for opencode.ai/zen.
// The model name or explicit API field determines which backend to use.
func newOpenCodeProvider(cfg ProviderConfig) (Provider, error) {
	api := cfg.API
	if api == "" {
		api = openCodeModelAPI("opencode", cfg.Model)
	}
	if api == "openai-responses" {
		return nil, fmt.Errorf("opencode model %q uses openai-responses, which daemon does not support via Chat Completions", cfg.Model)
	}

	resolved := resolveOpenCode("opencode", cfg)
	cfg.BaseURL = resolved.baseURL

	switch resolved.backend {
	case "anthropic":
		return NewAnthropicProvider(cfg), nil
	case "google":
		return NewGoogleProvider(cfg), nil
	default: // openai
		return NewOpenAIProvider(cfg), nil
	}
}

// newOpenCodeGoProvider creates a provider for opencode.ai/zen/go.
func newOpenCodeGoProvider(cfg ProviderConfig) (Provider, error) {
	api := cfg.API
	if api == "" {
		api = openCodeModelAPI("opencode-go", cfg.Model)
	}
	if api == "openai-responses" {
		return nil, fmt.Errorf("opencode-go model %q uses openai-responses, which daemon does not support via Chat Completions", cfg.Model)
	}

	resolved := resolveOpenCode("opencode-go", cfg)
	cfg.BaseURL = resolved.baseURL

	switch resolved.backend {
	case "anthropic":
		return NewAnthropicProvider(cfg), nil
	case "google":
		return NewGoogleProvider(cfg), nil
	default: // openai
		return NewOpenAIProvider(cfg), nil
	}
}

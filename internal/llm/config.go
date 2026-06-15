package llm

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ---------------------------------------------------------------------------
// Environment variable names
// ---------------------------------------------------------------------------

const (
	EnvProvider         = "DAEMON_PROVIDER"
	EnvAnthropicKey     = "ANTHROPIC_API_KEY"
	EnvOpenAIKey        = "OPENAI_API_KEY"
	EnvGoogleKey        = "GOOGLE_API_KEY"
	EnvOpenCodeKey      = "OPENCODE_API_KEY"
	EnvOpenAIBaseURL    = "OPENAI_BASE_URL"
	EnvAnthropicBaseURL = "ANTHROPIC_BASE_URL"
	EnvGoogleBaseURL    = "GOOGLE_BASE_URL"
	EnvModel            = "DAEMON_MODEL"
	EnvThinking         = "DAEMON_THINKING"

	// Auth file location: ~/.config/daemon/auth.json
	AuthFileDir  = "daemon"
	AuthFileName = "auth.json"
)

// AuthFileEntry represents one provider entry in auth.json.
type AuthFileEntry struct {
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url,omitempty"`
	Model   string `json:"model,omitempty"`
}

// AuthFile is the full auth.json structure. Provider entries remain top-level
// for hand-editability, while the active provider uses an explicit field.
type AuthFile struct {
	ActiveProvider string
	Providers      map[string]AuthFileEntry
}

func NewAuthFile() AuthFile {
	return AuthFile{Providers: map[string]AuthFileEntry{}}
}

func (a AuthFile) Get(provider string) (AuthFileEntry, bool) {
	entry, ok := a.Providers[provider]
	return entry, ok
}

func (a *AuthFile) Set(provider string, entry AuthFileEntry) {
	if a.Providers == nil {
		a.Providers = map[string]AuthFileEntry{}
	}
	a.Providers[provider] = entry
}

func (a AuthFile) MarshalJSON() ([]byte, error) {
	out := map[string]any{}
	if a.ActiveProvider != "" {
		out["_active_provider"] = a.ActiveProvider
	}
	for name, entry := range a.Providers {
		out[name] = entry
	}
	return json.Marshal(out)
}

func (a *AuthFile) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	*a = NewAuthFile()
	for key, value := range raw {
		switch key {
		case "_active_provider":
			if err := json.Unmarshal(value, &a.ActiveProvider); err != nil {
				return fmt.Errorf("parse _active_provider: %w", err)
			}
		case "_active":
			// Legacy prototype schema: { "_active": { "api_key": "anthropic" } }.
			// Read it for migration compatibility, but never write it back.
			var legacy AuthFileEntry
			if err := json.Unmarshal(value, &legacy); err != nil {
				return fmt.Errorf("parse legacy _active: %w", err)
			}
			if a.ActiveProvider == "" {
				a.ActiveProvider = legacy.APIKey
			}
		default:
			var entry AuthFileEntry
			if err := json.Unmarshal(value, &entry); err != nil {
				return fmt.Errorf("parse provider %q: %w", key, err)
			}
			a.Providers[key] = entry
		}
	}
	return nil
}

// Default models per provider.
var defaultModels = map[string]string{
	"anthropic":   "claude-sonnet-4-5",
	"openai":      "gpt-4o",
	"google":      "gemini-2.0-flash",
	"opencode":    "claude-sonnet-4-5",
	"opencode-go": "deepseek-v4-flash",
}

// defaultBaseURLs per provider. Used when no override is set.
var defaultBaseURLs = map[string]string{
	"opencode":    "https://opencode.ai/zen",
	"opencode-go": "https://opencode.ai/zen/go/v1",
}

// ---------------------------------------------------------------------------
// Auth file helpers
// ---------------------------------------------------------------------------

// AuthFilePath returns the path to ~/.config/daemon/auth.json.
func AuthFilePath() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(configDir, AuthFileDir, AuthFileName)
}

// ReadAuthFile reads ~/.config/daemon/auth.json. Returns empty config if missing.
func ReadAuthFile() (AuthFile, error) {
	path := AuthFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return NewAuthFile(), nil
		}
		return AuthFile{}, fmt.Errorf("read auth file: %w", err)
	}
	var auth AuthFile
	if err := json.Unmarshal(data, &auth); err != nil {
		return AuthFile{}, fmt.Errorf("parse auth file: %w", err)
	}
	if auth.Providers == nil {
		auth.Providers = map[string]AuthFileEntry{}
	}
	return auth, nil
}

// WriteAuthFile writes the auth file, creating parent directories with 0700
// and the file itself with 0600 permissions.
func WriteAuthFile(auth AuthFile) error {
	path := AuthFilePath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := json.MarshalIndent(auth, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal auth: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write auth file: %w", err)
	}
	return nil
}

// ListProviders returns the list of supported provider names.
func ListProviders() []string {
	return []string{"anthropic", "openai", "google", "opencode", "opencode-go"}
}

// ProviderInfo describes a provider for display.
type ProviderInfo struct {
	Name         string
	EnvKey       string
	DefaultModel string
	Description  string
}

// ProviderDetails returns descriptive info for all providers.
func ProviderDetails() []ProviderInfo {
	return []ProviderInfo{
		{Name: "anthropic", EnvKey: EnvAnthropicKey, DefaultModel: "claude-sonnet-4-5", Description: "Anthropic Claude (direct)"},
		{Name: "openai", EnvKey: EnvOpenAIKey, DefaultModel: "gpt-4o", Description: "OpenAI (direct)"},
		{Name: "google", EnvKey: EnvGoogleKey, DefaultModel: "gemini-2.0-flash", Description: "Google Gemini (direct)"},
		{Name: "opencode", EnvKey: EnvOpenCodeKey, DefaultModel: "claude-sonnet-4-5", Description: "OpenCode Zen (proxy: Claude, DeepSeek, Qwen...)"},
		{Name: "opencode-go", EnvKey: EnvOpenCodeKey, DefaultModel: "deepseek-v4-flash", Description: "OpenCode Go (proxy: DeepSeek, Kimi, MiMo...)"},
	}
}

// ---------------------------------------------------------------------------
// LoadConfig reads provider configuration with this resolution order:
//   1. Environment variables (highest priority)
//   2. auth.json file
//   3. .env file (loaded by loadDotEnv in main)
//   4. Built-in defaults
// ---------------------------------------------------------------------------

func LoadConfig() ProviderConfig {
	name := os.Getenv(EnvProvider)

	// Check auth.json for active provider hint if no env override
	if name == "" {
		auth, _ := ReadAuthFile()
		if auth.ActiveProvider != "" {
			name = auth.ActiveProvider
		}
	}
	if name == "" {
		name = "anthropic"
	}

	cfg := ProviderConfig{Name: name}

	// Load from auth.json first (lower priority than env vars)
	auth, _ := ReadAuthFile()
	if entry, ok := auth.Get(name); ok {
		cfg.APIKey = entry.APIKey
		cfg.BaseURL = entry.BaseURL
		cfg.BaseURLExplicit = entry.BaseURL != ""
		cfg.Model = entry.Model
	}

	// Env vars override auth.json
	switch name {
	case "anthropic":
		if v := os.Getenv(EnvAnthropicKey); v != "" {
			cfg.APIKey = v
		}
		if v := os.Getenv(EnvAnthropicBaseURL); v != "" {
			cfg.BaseURL = v
			cfg.BaseURLExplicit = true
		}
	case "openai":
		if v := os.Getenv(EnvOpenAIKey); v != "" {
			cfg.APIKey = v
		}
		if v := os.Getenv(EnvOpenAIBaseURL); v != "" {
			cfg.BaseURL = v
			cfg.BaseURLExplicit = true
		}
	case "google":
		if v := os.Getenv(EnvGoogleKey); v != "" {
			cfg.APIKey = v
		}
		if v := os.Getenv(EnvGoogleBaseURL); v != "" {
			cfg.BaseURL = v
			cfg.BaseURLExplicit = true
		}
	case "opencode":
		if v := os.Getenv(EnvOpenCodeKey); v != "" {
			cfg.APIKey = v
		} else if cfg.APIKey == "" {
			cfg.APIKey = os.Getenv(EnvAnthropicKey)
		}
		if v := os.Getenv("OPENCODE_BASE_URL"); v != "" {
			cfg.BaseURL = v
			cfg.BaseURLExplicit = true
		} else if cfg.BaseURL == "" {
			cfg.BaseURL = defaultBaseURLs["opencode"]
		}
	case "opencode-go":
		if v := os.Getenv(EnvOpenCodeKey); v != "" {
			cfg.APIKey = v
		} else if cfg.APIKey == "" {
			cfg.APIKey = os.Getenv(EnvOpenAIKey)
		}
		if v := os.Getenv("OPENCODE_GO_BASE_URL"); v != "" {
			cfg.BaseURL = v
			cfg.BaseURLExplicit = true
		} else if cfg.BaseURL == "" {
			cfg.BaseURL = defaultBaseURLs["opencode-go"]
		}
	}

	if v := os.Getenv(EnvModel); v != "" {
		cfg.Model = v
	} else if cfg.Model == "" {
		cfg.Model = defaultModels[name]
	}

	if v := os.Getenv(EnvThinking); v != "" {
		if lvl := ParseThinkingLevel(v); lvl != "" {
			cfg.Thinking = lvl
		}
	}

	return cfg
}

// LoadFromEnv is an alias for LoadConfig.
func LoadFromEnv() ProviderConfig {
	return LoadConfig()
}

// ---------------------------------------------------------------------------
// Provider factory
// ---------------------------------------------------------------------------

// ResolveProviderConfig normalizes provider-specific derived configuration for
// display and construction. Explicit custom BaseURL values are preserved.
func ResolveProviderConfig(cfg ProviderConfig) ProviderConfig {
	switch cfg.Name {
	case "opencode":
		resolved := resolveOpenCode("opencode", cfg)
		cfg.BaseURL = resolved.baseURL
	case "opencode-go":
		resolved := resolveOpenCode("opencode-go", cfg)
		cfg.BaseURL = resolved.baseURL
	}
	return cfg
}

// NewProvider creates the appropriate Provider based on config.
// For providers like "opencode" that serve multiple API types, the API field
// or model name determines whether Anthropic-messages or OpenAI-completions is used.
// Returns an error if required fields are missing.
func NewProvider(cfg ProviderConfig) (Provider, error) {
	cfg = ResolveProviderConfig(cfg)
	switch cfg.Name {
	case "anthropic":
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("anthropic provider requires %s", EnvAnthropicKey)
		}
		return NewAnthropicProvider(cfg), nil
	case "openai":
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("openai provider requires %s", EnvOpenAIKey)
		}
		return NewOpenAIProvider(cfg), nil
	case "google":
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("google provider requires %s", EnvGoogleKey)
		}
		return NewGoogleProvider(cfg), nil
	case "opencode":
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("opencode provider requires %s (or %s as fallback)", EnvOpenCodeKey, EnvAnthropicKey)
		}
		return newOpenCodeProvider(cfg)
	case "opencode-go":
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("opencode-go provider requires %s (or %s as fallback)", EnvOpenCodeKey, EnvOpenAIKey)
		}
		return newOpenCodeGoProvider(cfg)
	default:
		return nil, fmt.Errorf("unknown provider: %s (supported: anthropic, openai, google, opencode, opencode-go)", cfg.Name)
	}
}

// MustNewProvider is like NewProvider but panics on error.
func MustNewProvider(cfg ProviderConfig) Provider {
	p, err := NewProvider(cfg)
	if err != nil {
		panic(err)
	}
	return p
}

// isAnthropicModel returns true for models that should use the Anthropic Messages API.
// Only used as a heuristic fallback when a model isn't in the openCodeModelCatalog.
func isAnthropicModel(model string) bool {
	prefixes := []string{
		"claude-",
		"minimax-m3",
		"qwen3.7-max",
		"qwen3.7-plus",
	}
	for _, p := range prefixes {
		if len(model) >= len(p) && model[:len(p)] == p {
			return true
		}
	}
	return false
}

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func loadDotEnv() {
	envKeys := []string{
		"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GOOGLE_API_KEY", "OPENCODE_API_KEY",
		"DAEMON_PROVIDER", "DAEMON_MODEL", "DAEMON_THINKING", "DAEMON_ENABLE_FFF",
		"ANTHROPIC_BASE_URL", "OPENAI_BASE_URL", "GOOGLE_BASE_URL",
		"OPENCODE_BASE_URL", "OPENCODE_GO_BASE_URL",
	}
	candidates := []string{".env", filepath.Join(os.Getenv("HOME"), "workspaces", "daemon", ".env")}
	for _, filePath := range candidates {
		content, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(content), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			key, value, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			key = strings.TrimSpace(key)
			for _, envKey := range envKeys {
				if key == envKey && os.Getenv(key) == "" {
					value = strings.TrimSpace(value)
					value = strings.Trim(value, "\"'")
					_ = os.Setenv(key, value)
				}
			}
		}
	}
}

func shortTitle(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "agent edit"
	}
	if len(s) > 48 {
		return s[:48] + "…"
	}
	return s
}

func randomish(s string) string {
	var h uint32 = 2166136261
	for _, c := range []byte(s) {
		h ^= uint32(c)
		h *= 16777619
	}
	return fmt.Sprintf("%08x", h)
}

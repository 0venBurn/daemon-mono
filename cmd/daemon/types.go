package main

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/0venburn/daemon/internal/fff"
	"github.com/0venburn/daemon/internal/llm"
)

type Position []int

type Selection struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
	Text  string   `json:"text"`
}

type StartParams struct {
	Prompt      string     `json:"prompt"`
	CWD         string     `json:"cwd"`
	File        string     `json:"file"`
	Filetype    string     `json:"filetype"`
	Cursor      Position   `json:"cursor"`
	Selection   *Selection `json:"selection,omitempty"`
	Content     string     `json:"content"`
	ChangedTick int64      `json:"changedtick"`
}

type SessionInputContext struct {
	File        string           `json:"file"`
	Filetype    string           `json:"filetype"`
	Cursor      []int            `json:"cursor"`
	Selection   *Selection       `json:"selection,omitempty"`
	Content     string           `json:"content"`
	ChangedTick int64            `json:"changedtick"`
	Transcript  []TranscriptTurn `json:"transcript"`
}

type TranscriptTurn struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

type SessionInputParams struct {
	SessionID string              `json:"session_id"`
	Intent    string              `json:"intent"`
	Text      string              `json:"text"`
	CWD       string              `json:"cwd"`
	Context   SessionInputContext `json:"context"`
}

type PromptParams struct {
	SessionID string `json:"session_id"`
	Text      string `json:"text"`
}

type CancelParams struct {
	SessionID string `json:"session_id"`
}

type Incoming struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type Outgoing struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params any             `json:"params,omitempty"`
	Error  string          `json:"error,omitempty"`
}

type Daemon struct {
	provider llm.Provider
	config   llm.ProviderConfig
	out      *json.Encoder
	outMu    sync.Mutex
	sessions map[string]context.CancelFunc
	sessMu   sync.Mutex
	provMu   sync.Mutex  // protects provider recreation
	finder   *fff.Finder // long-lived FFF search instance
}

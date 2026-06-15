package main

import "github.com/0venburn/daemon/internal/llm"

const (
	methodDaemonError   = "daemon/error"
	methodDaemonInfo    = "daemon/info"
	methodSessionStart  = "session/start"
	methodSessionStatus = "session/status"
	methodSessionError  = "session/error"
	methodSessionDone   = "session/done"
	methodExplainChunk  = "explain/chunk"
	methodEditCreate    = "edit/create"
	methodEditInsert    = "edit/insert"
	methodEditReplace   = "edit/replace"
	methodToolStart     = "tool/start"
	methodToolDone      = "tool/done"
)

type daemonErrorEvent struct {
	Error string `json:"error"`
}

type daemonInfoEvent struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	APIType  string `json:"api_type"`
	BaseURL  string `json:"base_url"`
	HasKey   bool   `json:"has_key"`
	Thinking string `json:"thinking"`
}

type sessionStartEvent struct {
	SessionID string `json:"session_id"`
	File      string `json:"file"`
	Mode      string `json:"mode,omitempty"`
}

type sessionStatusEvent struct {
	SessionID string `json:"session_id"`
	State     string `json:"state"`
	Message   string `json:"message,omitempty"`
}

type sessionErrorEvent struct {
	SessionID string `json:"session_id"`
	Error     string `json:"error"`
	Details   string `json:"details,omitempty"`
}

type sessionDoneEvent struct {
	SessionID string `json:"session_id"`
	File      string `json:"file"`
	Mode      string `json:"mode,omitempty"`
}

type explainChunkEvent struct {
	SessionID string `json:"session_id"`
	File      string `json:"file"`
	Text      string `json:"text"`
}

type editCreateEvent struct {
	SessionID   string `json:"session_id"`
	File        string `json:"file"`
	Text        string `json:"text"`
	Description string `json:"description,omitempty"`
	Index       int    `json:"index"`
}

type editInsertEvent struct {
	SessionID       string `json:"session_id"`
	File            string `json:"file"`
	BaseChangedTick int64  `json:"base_changedtick"`
	Text            string `json:"text"`
	Row             int    `json:"row"`
	Col             int    `json:"col"`
	Description     string `json:"description,omitempty"`
	Index           int    `json:"index"`
}

type editReplaceEvent struct {
	SessionID       string `json:"session_id"`
	File            string `json:"file"`
	BaseChangedTick int64  `json:"base_changedtick"`
	Old             string `json:"old"`
	New             string `json:"new"`
	Description     string `json:"description,omitempty"`
	Index           int    `json:"index"`
}

type toolEvent struct {
	SessionID   string `json:"session_id"`
	Name        string `json:"name"`
	File        string `json:"file,omitempty"`
	Row         int    `json:"row,omitempty"`
	Col         int    `json:"col,omitempty"`
	Description string `json:"description,omitempty"`
	Arguments   any    `json:"arguments,omitempty"`
	Result      string `json:"result,omitempty"`
}

func (d *Daemon) sendDaemonError(err string) {
	d.send(methodDaemonError, daemonErrorEvent{Error: err})
}

func (d *Daemon) sendDaemonInfo() {
	d.send(methodDaemonInfo, daemonInfoFromConfig(d.config))
}

func daemonInfoFromConfig(cfg llm.ProviderConfig) daemonInfoEvent {
	apiType := cfg.API
	if apiType == "" {
		switch cfg.Name {
		case "anthropic":
			apiType = "anthropic-messages"
		case "openai":
			apiType = "openai-completions"
		case "google":
			apiType = "google-generative-ai"
		case "opencode", "opencode-go":
			apiType = llm.OpenCodeModelAPIForInfo(cfg.Name, cfg.Model)
		}
	}
	return daemonInfoEvent{
		Provider: cfg.Name,
		Model:    cfg.Model,
		APIType:  apiType,
		BaseURL:  cfg.BaseURL,
		HasKey:   cfg.APIKey != "",
		Thinking: string(cfg.Thinking),
	}
}

func (d *Daemon) sendSessionStart(sessionID, file, mode string) {
	d.send(methodSessionStart, sessionStartEvent{SessionID: sessionID, File: file, Mode: mode})
}

func (d *Daemon) sendSessionStatus(sessionID, state, message string) {
	d.send(methodSessionStatus, sessionStatusEvent{SessionID: sessionID, State: state, Message: message})
}

func (d *Daemon) sendSessionError(sessionID, err string) {
	d.send(methodSessionError, sessionErrorEvent{SessionID: sessionID, Error: err})
}

func (d *Daemon) sendSessionErrorDetails(sessionID, err, details string) {
	d.send(methodSessionError, sessionErrorEvent{SessionID: sessionID, Error: err, Details: details})
}

func (d *Daemon) sendSessionDone(sessionID, file, mode string) {
	d.send(methodSessionDone, sessionDoneEvent{SessionID: sessionID, File: file, Mode: mode})
}

func (d *Daemon) sendExplainChunk(sessionID, file, text string) {
	d.send(methodExplainChunk, explainChunkEvent{SessionID: sessionID, File: file, Text: text})
}

func (d *Daemon) sendEditCreate(sessionID, file, text, description string, index int) {
	d.send(methodEditCreate, editCreateEvent{SessionID: sessionID, File: file, Text: text, Description: description, Index: index})
}

func (d *Daemon) sendEditInsert(sessionID, file string, baseChangedTick int64, text string, row, col int, description string, index int) {
	d.send(methodEditInsert, editInsertEvent{SessionID: sessionID, File: file, BaseChangedTick: baseChangedTick, Text: text, Row: row, Col: col, Description: description, Index: index})
}

func (d *Daemon) sendEditReplace(sessionID, file string, baseChangedTick int64, oldText, newText, description string, index int) {
	d.send(methodEditReplace, editReplaceEvent{SessionID: sessionID, File: file, BaseChangedTick: baseChangedTick, Old: oldText, New: newText, Description: description, Index: index})
}

func (d *Daemon) sendToolStart(sessionID, name, file string, row, col int, description string) {
	d.sendToolStartArgs(sessionID, name, file, row, col, description, nil)
}

func (d *Daemon) sendToolStartArgs(sessionID, name, file string, row, col int, description string, args any) {
	d.send(methodToolStart, toolEvent{SessionID: sessionID, Name: name, File: file, Row: row, Col: col, Description: description, Arguments: args})
}

func (d *Daemon) sendToolDone(sessionID, name, file string, row, col int) {
	d.sendToolDoneResult(sessionID, name, file, row, col, "")
}

func (d *Daemon) sendToolDoneResult(sessionID, name, file string, row, col int, result string) {
	d.sendToolDoneResultArgs(sessionID, name, file, row, col, result, nil)
}

func (d *Daemon) sendToolDoneResultArgs(sessionID, name, file string, row, col int, result string, args any) {
	d.send(methodToolDone, toolEvent{SessionID: sessionID, Name: name, File: file, Row: row, Col: col, Result: result, Arguments: args})
}

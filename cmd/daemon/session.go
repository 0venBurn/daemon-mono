package main

import (
	"context"
	"fmt"
	"os"
	"strings"
)

func (d *Daemon) handleSessionInput(p SessionInputParams) {
	sessionID := p.SessionID
	if sessionID == "" {
		sessionID = newSessionID(p.Text, p.Context.File)
	}

	params := startParamsFromInput(p)
	switch strings.ToLower(strings.TrimSpace(p.Intent)) {
	case "explain":
		d.startExplainSessionWithID(sessionID, params)
	case "ask":
		d.startChatSession(sessionID, params, p.Context.Transcript)
	default:
		d.startEditableSession(sessionID, params)
	}
}

func (d *Daemon) startSession(p StartParams) {
	d.startEditableSession(newSessionID(p.Prompt, p.File), p)
}

func (d *Daemon) startEditableSession(sessionID string, p StartParams) {
	d.startManagedSession(sessionID, p.File, "", func(ctx context.Context) {
		if shouldUseFastPatch(p) {
			d.runPatchSession(ctx, sessionID, p)
			return
		}
		d.runEditSession(ctx, sessionID, p)
	})
}

func shouldUseFastPatch(p StartParams) bool {
	if p.Selection != nil && p.Selection.Text != "" {
		return true
	}
	if strings.TrimSpace(p.File) == "" || strings.TrimSpace(p.Content) == "" {
		return false
	}
	prompt := strings.ToLower(p.Prompt)
	multiFileHints := []string{" across ", " all files", " every file", " other file", " another file", " project", " repo", " repository", " find ", " search ", " grep ", " scan ", " inspect "}
	for _, hint := range multiFileHints {
		if strings.Contains(prompt, hint) {
			return false
		}
	}
	return true
}

func (d *Daemon) startExplainSession(p StartParams) {
	d.startExplainSessionWithID(newSessionID("explain", p.File+p.Selection.Text), p)
}

func (d *Daemon) startExplainSessionWithID(sessionID string, p StartParams) {
	if p.Selection == nil || strings.TrimSpace(p.Selection.Text) == "" {
		d.sendDaemonError("no selected code to explain")
		return
	}

	d.startManagedSession(sessionID, p.File, "explain", func(ctx context.Context) {
		d.runExplainSession(ctx, sessionID, p)
	})
}

func (d *Daemon) startChatSession(sessionID string, p StartParams, transcript []TranscriptTurn) {
	d.startManagedSession(sessionID, p.File, "ask", func(ctx context.Context) {
		d.runChatSession(ctx, sessionID, p, transcript)
	})
}

func (d *Daemon) startManagedSession(sessionID, file, mode string, run func(context.Context)) {
	ctx, cancel := context.WithCancel(context.Background())

	d.sessMu.Lock()
	d.sessions[sessionID] = cancel
	d.sessMu.Unlock()

	d.sendSessionStart(sessionID, file, mode)

	go func() {
		defer func() {
			d.sessMu.Lock()
			delete(d.sessions, sessionID)
			d.sessMu.Unlock()
		}()
		run(ctx)
	}()
}

func startParamsFromInput(p SessionInputParams) StartParams {
	var cursor Position
	if len(p.Context.Cursor) >= 2 {
		cursor = Position{p.Context.Cursor[0], p.Context.Cursor[1]}
	}
	return StartParams{
		Prompt:      p.Text,
		CWD:         p.CWD,
		File:        p.Context.File,
		Filetype:    p.Context.Filetype,
		Cursor:      cursor,
		Selection:   p.Context.Selection,
		Content:     p.Context.Content,
		ChangedTick: p.Context.ChangedTick,
	}
}

func newSessionID(parts ...string) string {
	return fmt.Sprintf("s-%d", os.Getpid()) + "-" + randomish(strings.Join(parts, "|"))
}

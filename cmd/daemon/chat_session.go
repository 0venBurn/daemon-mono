package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/0venburn/daemon/internal/llm"
)

func (d *Daemon) runChatSession(ctx context.Context, sessionID string, p StartParams, transcript []TranscriptTurn) {
	d.sendSessionStatus(sessionID, "thinking", "answering follow-up")

	messages := []llm.Message{
		{Role: llm.RoleUser, Content: []llm.ContentBlock{llm.TextBlock(buildChatPrompt(p, transcript))}},
	}
	ch := d.provider.StreamMessages(llm.WithSessionID(ctx, sessionID), messages, chatSystemPrompt(), nil)

	d.sendSessionStatus(sessionID, "writing", "streaming reply")

	var streamErr error
	for event := range ch {
		switch event.Type {
		case llm.EventTextDelta:
			if event.Text == "" {
				continue
			}
			d.sendExplainChunk(sessionID, p.File, event.Text)
		case llm.EventThinkingDelta:
			d.sendSessionStatus(sessionID, "thinking", "streaming reasoning")
		case llm.EventError:
			streamErr = event.Error
		}
	}
	if streamErr != nil {
		if ctx.Err() != nil {
			d.sendSessionStatus(sessionID, "cancelled", ctx.Err().Error())
			return
		}
		d.sendSessionError(sessionID, streamErr.Error())
		return
	}
	d.sendSessionDone(sessionID, p.File, "ask")
}

func chatSystemPrompt() string {
	return strings.Join([]string{
		"You answer follow-up questions in a Neovim transcript.",
		"Be concise, concrete, and helpful.",
		"Do not emit edit operations or JSON patches.",
		"Do not use markdown fences unless the user explicitly asks for a fenced block.",
	}, "\n")
}

func buildChatPrompt(p StartParams, transcript []TranscriptTurn) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Working directory: %s\n", p.CWD)
	fmt.Fprintf(&b, "File: %s\n", p.File)
	fmt.Fprintf(&b, "Filetype: %s\n", p.Filetype)
	fmt.Fprintf(&b, "Cursor row,col zero-based: %v\n\n", []int(p.Cursor))

	if len(transcript) > 0 {
		fmt.Fprintf(&b, "Conversation so far:\n")
		for i, turn := range transcript {
			role := strings.TrimSpace(turn.Role)
			text := strings.TrimSpace(turn.Text)
			if role == "" || text == "" {
				continue
			}
			if i == len(transcript)-1 && role == "user" && text == strings.TrimSpace(p.Prompt) {
				continue
			}
			fmt.Fprintf(&b, "%s: %s\n\n", role, text)
		}
	}

	fmt.Fprintf(&b, "Current user question:\n%s\n\n", p.Prompt)
	content := p.Content
	if len(content) > 80000 {
		content = content[:80000] + "\n...TRUNCATED..."
	}
	fmt.Fprintf(&b, "Current buffer content for context:\n---FILE---\n%s\n---END FILE---\n", content)
	return b.String()
}

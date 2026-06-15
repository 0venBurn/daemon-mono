package main

import (
	"context"

	"github.com/0venburn/daemon/internal/llm"
)

func (d *Daemon) runExplainSession(ctx context.Context, sessionID string, p StartParams) {
	d.sendSessionStatus(sessionID, "thinking", "explaining selection")

	messages := []llm.Message{
		{Role: llm.RoleUser, Content: []llm.ContentBlock{llm.TextBlock(buildExplainPrompt(p))}},
	}
	ch := d.provider.StreamMessages(llm.WithSessionID(ctx, sessionID), messages, explainSystemPrompt(), nil)

	d.sendSessionStatus(sessionID, "writing", "streaming explanation")

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
	d.sendSessionDone(sessionID, p.File, "explain")
}

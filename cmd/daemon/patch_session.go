package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/0venburn/daemon/internal/llm"
)

func (d *Daemon) runPatchSession(ctx context.Context, sessionID string, p StartParams) {
	sessionStart := time.Now()
	d.sendSessionStatus(sessionID, "planning", "building patch")

	var feedback string
	for attempt := 1; attempt <= 3; attempt++ {
		if attempt > 1 {
			d.sendSessionStatus(sessionID, "planning", fmt.Sprintf("retrying smaller patch (%d/3)", attempt))
		}

		raw, err := d.requestPatch(ctx, sessionID, p, feedback)
		if err != nil {
			if ctx.Err() != nil {
				d.sendSessionStatus(sessionID, "cancelled", ctx.Err().Error())
				return
			}
			d.sendSessionError(sessionID, err.Error())
			return
		}

		d.writeDebug("patch/response", map[string]any{"session_id": sessionID, "attempt": attempt, "raw_len": len(raw)})

		patch, err := parsePatchResponse(raw)
		if err != nil {
			feedback = "Previous response was rejected because it was not valid patch JSON. Return ONLY JSON in the requested shape. Error: " + err.Error()
			continue
		}

		var edits []PatchEdit
		if patchValidationMode() == "off" {
			edits, err = preparePatchResponse(patch)
		} else {
			edits, err = validatePatchResponse(p.Content, p.Prompt, patch)
		}
		if err != nil {
			feedback = "Previous patch was rejected. Try again with smaller exact old/new edits. Prefer one-line patches. Do not rewrite whole blocks unless explicitly asked. Rejection details:\n" + err.Error()
			d.sendSessionStatus(sessionID, "planning", "patch invalid: "+truncateForStatus(err.Error(), 160))
			continue
		}

		d.sendSessionStatus(sessionID, "writing", "applying patch")
		for i, edit := range edits {
			op := edit.Op
			if op == "" {
				op = "replace"
			}
			if op == "insert_at_selection_end" {
				if p.Selection == nil {
					d.sendSessionError(sessionID, "model returned insert_at_selection_end without a selection")
					return
				}
				d.sendEditInsert(sessionID, p.File, p.ChangedTick, edit.Text, p.Selection.End[0], p.Selection.End[1], edit.Description, i)
				continue
			}
			d.sendEditReplace(sessionID, p.File, p.ChangedTick, edit.Old, edit.New, edit.Description, i)
		}
		d.writeDebug("session/total", map[string]any{"session_id": sessionID, "file": p.File, "elapsed_ms": time.Since(sessionStart).Milliseconds()})
		d.sendSessionDone(sessionID, p.File, "")
		return
	}

	d.sendSessionErrorDetails(sessionID, "could not get a valid surgical patch after 3 attempts", feedback)
}

func (d *Daemon) requestPatch(ctx context.Context, sessionID string, p StartParams, feedback string) (string, error) {
	prompt := buildPrompt(p)
	if strings.TrimSpace(feedback) != "" {
		prompt += "\n\nPATCH VALIDATION FEEDBACK:\n" + feedback + "\n"
	}

	messages := []llm.Message{
		{Role: llm.RoleUser, Content: []llm.ContentBlock{llm.TextBlock(prompt)}},
	}
	d.sendSessionStatus(sessionID, "thinking", "calling model")
	apiStart := time.Now()
	system := systemPrompt(p)
	debug := map[string]any{"session_id": sessionID, "file": p.File, "mode": "patch", "provider": d.config.Name, "model": d.config.Model, "base_url": d.config.BaseURL, "prompt_bytes": len(prompt), "system_bytes": len(system), "content_bytes": len(p.Content), "messages": len(messages)}
	d.writeDebug("model/request", debug)
	message, err := d.streamMessage(ctx, sessionID, messages, system, nil, debug)
	if err != nil {
		return "", err
	}
	apiMS := time.Since(apiStart).Milliseconds()
	d.writeDebug("model/response", map[string]any{"session_id": sessionID, "file": p.File, "turn": -1, "mode": "patch", "api_ms": apiMS})
	d.sendSessionStatus(sessionID, "planning", fmt.Sprintf("model response received in %dms", apiMS))

	var b strings.Builder
	for _, content := range message.Content {
		if content.Type == "text" {
			b.WriteString(content.Text)
		}
	}
	return b.String(), nil
}

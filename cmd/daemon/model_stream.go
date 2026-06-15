package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/0venburn/daemon/internal/llm"
)

func (d *Daemon) streamMessage(ctx context.Context, sessionID string, messages []llm.Message, systemPrompt string, tools []llm.ToolDef, debug map[string]any) (*llm.Message, error) {
	callCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	callCtx = llm.WithSessionID(callCtx, sessionID)

	start := time.Now()
	var headerMS int64 = -1
	var firstRawMS int64 = -1
	var firstSemanticMS int64 = -1
	var content []llm.ContentBlock
	var text strings.Builder

	payload := func(extra map[string]any) map[string]any {
		out := map[string]any{"session_id": sessionID}
		for k, v := range debug {
			out[k] = v
		}
		for k, v := range extra {
			out[k] = v
		}
		return out
	}
	markSemantic := func(kind string) {
		if firstSemanticMS >= 0 {
			return
		}
		firstSemanticMS = time.Since(start).Milliseconds()
		d.writeDebug("model/first_semantic_event", payload(map[string]any{"first_semantic_ms": firstSemanticMS, "kind": kind}))
	}

	flushText := func() {
		if text.Len() == 0 {
			return
		}
		content = append(content, llm.TextBlock(text.String()))
		text.Reset()
	}

	const firstEventTimeout = 45 * time.Second
	const idleTimeout = 45 * time.Second
	timeoutStage := "response headers or first stream event"
	timeoutDuration := firstEventTimeout
	timeout := time.NewTimer(timeoutDuration)
	defer timeout.Stop()
	resetTimeout := func(stage string, duration time.Duration) {
		timeoutStage = stage
		timeoutDuration = duration
		if !timeout.Stop() {
			select {
			case <-timeout.C:
			default:
			}
		}
		timeout.Reset(duration)
	}

	ch := d.provider.StreamMessages(callCtx, messages, systemPrompt, tools)
	for {
		select {
		case <-callCtx.Done():
			return nil, callCtx.Err()
		case <-timeout.C:
			cancel()
			d.writeDebug("model/timeout", payload(map[string]any{"elapsed_ms": time.Since(start).Milliseconds(), "stage": timeoutStage, "timeout_ms": timeoutDuration.Milliseconds()}))
			return nil, fmt.Errorf("model %s timeout after %s", timeoutStage, timeoutDuration)
		case ev, ok := <-ch:
			if !ok {
				flushText()
				if len(content) == 0 {
					return nil, fmt.Errorf("model stream ended with no content")
				}
				return &llm.Message{Role: llm.RoleAssistant, Content: content}, nil
			}

			switch ev.Type {
			case llm.EventResponseHeaders:
				if headerMS < 0 {
					headerMS = time.Since(start).Milliseconds()
					d.writeDebug("model/response_headers", payload(map[string]any{"headers_ms": headerMS, "status_code": ev.StatusCode}))
					resetTimeout("first stream event", firstEventTimeout)
				}
			case llm.EventRawEvent:
				if firstRawMS < 0 {
					firstRawMS = time.Since(start).Milliseconds()
					d.writeDebug("model/first_raw_event", payload(map[string]any{"first_raw_ms": firstRawMS}))
				}
				resetTimeout("stream idle", idleTimeout)
			case llm.EventTextDelta:
				markSemantic("text")
				resetTimeout("stream idle", idleTimeout)
				text.WriteString(ev.Text)
			case llm.EventThinkingDelta:
				markSemantic("thinking")
				resetTimeout("stream idle", idleTimeout)
				d.sendSessionStatus(sessionID, "thinking", "streaming reasoning")
			case llm.EventToolUse:
				markSemantic("tool_use")
				resetTimeout("stream idle", idleTimeout)
				flushText()
				input := json.RawMessage(ev.Text)
				if !json.Valid(input) {
					input = json.RawMessage(`{}`)
				}
				content = append(content, llm.ToolUseBlock(ev.ToolID, ev.ToolName, input))
			case llm.EventError:
				d.writeDebug("model/error", payload(map[string]any{"elapsed_ms": time.Since(start).Milliseconds(), "error": ev.Error.Error()}))
				return nil, ev.Error
			case llm.EventStop:
				flushText()
				return &llm.Message{Role: llm.RoleAssistant, Content: content}, nil
			}
		}
	}
}

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

func (d *Daemon) serve(in io.Reader) {
	scanner := bufio.NewScanner(in)
	buf := make([]byte, 1024)
	scanner.Buffer(buf, 1024*1024*8)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var msg Incoming
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			d.sendDaemonError("decode: " + err.Error())
			continue
		}
		d.handle(msg)
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "stdin: %v\n", err)
	}
}

func (d *Daemon) handle(msg Incoming) {
	switch msg.Method {
	case "daemon/info":
		d.sendDaemonInfo()
	case "daemon/set_thinking":
		var p struct {
			Level string `json:"level"`
		}
		_ = json.Unmarshal(msg.Params, &p)
		d.setThinking(p.Level)
	case "session/input":
		var p SessionInputParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			d.sendDaemonError(err.Error())
			return
		}
		d.handleSessionInput(p)
	case "session/start":
		var p StartParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			d.sendDaemonError(err.Error())
			return
		}
		d.startSession(p)
	case "session/explain":
		var p StartParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			d.sendDaemonError(err.Error())
			return
		}
		d.startExplainSession(p)
	case "session/cancel":
		var p CancelParams
		_ = json.Unmarshal(msg.Params, &p)
		d.cancelSession(p.SessionID, "cancelled")
	case "session/prompt":
		var p PromptParams
		_ = json.Unmarshal(msg.Params, &p)
		// Prototype behavior: prompt update cancels current stream and asks user to restart with amended prompt.
		// Next pass keeps conversation state and replans from current buffer.
		d.cancelSession(p.SessionID, "prompt: "+p.Text)
		d.sendSessionStatus(p.SessionID, "prompted", "stopped; restart edit with prompt text included")
	default:
		d.sendDaemonError("unknown method: " + msg.Method)
	}
}

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
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

type SteerParams struct {
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
	client   *anthropic.Client
	out      *json.Encoder
	outMu    sync.Mutex
	sessions map[string]context.CancelFunc
	sessMu   sync.Mutex
}

func main() {
	loadAnthropicAPIKey()

	client := anthropic.NewClient()
	d := &Daemon{
		client:   &client,
		out:      json.NewEncoder(os.Stdout),
		sessions: map[string]context.CancelFunc{},
	}

	scanner := bufio.NewScanner(os.Stdin)
	buf := make([]byte, 1024)
	scanner.Buffer(buf, 1024*1024*8)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var msg Incoming
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			d.send("daemon/error", map[string]any{"error": "decode: " + err.Error()})
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
	case "session/start":
		var p StartParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			d.send("daemon/error", map[string]any{"error": err.Error()})
			return
		}
		d.startSession(p)
	case "session/explain":
		var p StartParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			d.send("daemon/error", map[string]any{"error": err.Error()})
			return
		}
		d.startExplainSession(p)
	case "session/cancel":
		var p CancelParams
		_ = json.Unmarshal(msg.Params, &p)
		d.cancelSession(p.SessionID, "cancelled")
	case "session/steer":
		var p SteerParams
		_ = json.Unmarshal(msg.Params, &p)
		// Prototype behavior: steering cancels current stream and asks user to restart with amended prompt.
		// Next pass keeps conversation state and replans from current buffer.
		d.cancelSession(p.SessionID, "steer: "+p.Text)
		d.send("session/status", map[string]any{"session_id": p.SessionID, "state": "steered", "message": "stopped; restart edit with steering text included"})
	default:
		d.send("daemon/error", map[string]any{"error": "unknown method: " + msg.Method})
	}
}

func (d *Daemon) startSession(p StartParams) {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		d.send("daemon/error", map[string]any{"error": "ANTHROPIC_API_KEY missing"})
		return
	}

	sessionID := fmt.Sprintf("s-%d", os.Getpid()) + "-" + randomish(p.Prompt+p.File)
	ctx, cancel := context.WithCancel(context.Background())

	d.sessMu.Lock()
	d.sessions[sessionID] = cancel
	d.sessMu.Unlock()

	d.send("session/start", map[string]any{"session_id": sessionID, "file": p.File})
	d.send("plan/update", map[string]any{
		"session_id": sessionID,
		"title":      shortTitle(p.Prompt),
		"items": []string{
			"Use current buffer, cursor, and selection as scope",
			"Stream generated edit into the real Neovim buffer",
			"Keep ghost cursor at agent insertion point",
			"Stop immediately on Esc/cancel/user edit",
		},
	})

	go func() {
		defer func() {
			d.sessMu.Lock()
			delete(d.sessions, sessionID)
			d.sessMu.Unlock()
		}()
		d.runSession(ctx, sessionID, p)
	}()
}

func (d *Daemon) runSession(ctx context.Context, sessionID string, p StartParams) {
	d.send("session/status", map[string]any{"session_id": sessionID, "state": "thinking", "message": "calling haiku"})

	if p.Selection != nil {
		d.runPatchSession(ctx, sessionID, p)
		return
	}

	stream := d.client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaudeHaiku4_5,
		MaxTokens: 2048,
		System:    []anthropic.TextBlockParam{{Text: systemPrompt(p)}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(buildPrompt(p))),
		},
	})

	d.send("session/status", map[string]any{"session_id": sessionID, "state": "writing", "message": "streaming edit"})

	sanitizer := NewMarkdownFenceSanitizer()
	delay := streamDelay()
	charsPerChunk := streamCharsPerChunk()
	sendEdit := func(text string) {
		if text == "" {
			return
		}
		for _, part := range splitForVisibleStreaming(text, charsPerChunk) {
			select {
			case <-ctx.Done():
				return
			default:
			}
			d.send("edit/chunk", map[string]any{
				"session_id":         sessionID,
				"file":               p.File,
				"base_changedtick":   p.ChangedTick,
				"text":               part,
				"label":              "writing",
				"target":             "cursor",
				"cursor_at_start":    p.Cursor,
				"replaces_selection": p.Selection != nil,
			})
			if delay > 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(delay):
				}
			}
		}
	}

	for stream.Next() {
		event := stream.Current()
		if event.Type != "content_block_delta" || event.Delta.Type != "text_delta" || event.Delta.Text == "" {
			continue
		}
		sendEdit(sanitizer.Add(event.Delta.Text))
	}
	sendEdit(sanitizer.Flush())
	if err := stream.Err(); err != nil {
		if ctx.Err() != nil {
			d.send("session/status", map[string]any{"session_id": sessionID, "state": "cancelled", "message": ctx.Err().Error()})
			return
		}
		d.send("session/error", map[string]any{"session_id": sessionID, "error": err.Error()})
		return
	}
	d.send("session/done", map[string]any{"session_id": sessionID, "file": p.File})
}

func (d *Daemon) startExplainSession(p StartParams) {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		d.send("daemon/error", map[string]any{"error": "ANTHROPIC_API_KEY missing"})
		return
	}
	if p.Selection == nil || strings.TrimSpace(p.Selection.Text) == "" {
		d.send("daemon/error", map[string]any{"error": "no selected code to explain"})
		return
	}

	sessionID := fmt.Sprintf("s-%d", os.Getpid()) + "-explain-" + randomish(p.File+p.Selection.Text)
	ctx, cancel := context.WithCancel(context.Background())

	d.sessMu.Lock()
	d.sessions[sessionID] = cancel
	d.sessMu.Unlock()

	d.send("session/start", map[string]any{"session_id": sessionID, "file": p.File, "mode": "explain"})

	go func() {
		defer func() {
			d.sessMu.Lock()
			delete(d.sessions, sessionID)
			d.sessMu.Unlock()
		}()
		d.runExplainSession(ctx, sessionID, p)
	}()
}

func (d *Daemon) runExplainSession(ctx context.Context, sessionID string, p StartParams) {
	d.send("session/status", map[string]any{"session_id": sessionID, "state": "thinking", "message": "explaining selection"})

	stream := d.client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaudeHaiku4_5,
		MaxTokens: 1024,
		System:    []anthropic.TextBlockParam{{Text: explainSystemPrompt()}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(buildExplainPrompt(p))),
		},
	})

	d.send("session/status", map[string]any{"session_id": sessionID, "state": "writing", "message": "streaming explanation"})

	for stream.Next() {
		event := stream.Current()
		if event.Type != "content_block_delta" || event.Delta.Type != "text_delta" || event.Delta.Text == "" {
			continue
		}
		d.send("explain/chunk", map[string]any{
			"session_id": sessionID,
			"file":       p.File,
			"text":       event.Delta.Text,
		})
	}
	if err := stream.Err(); err != nil {
		if ctx.Err() != nil {
			d.send("session/status", map[string]any{"session_id": sessionID, "state": "cancelled", "message": ctx.Err().Error()})
			return
		}
		d.send("session/error", map[string]any{"session_id": sessionID, "error": err.Error()})
		return
	}
	d.send("session/done", map[string]any{"session_id": sessionID, "file": p.File, "mode": "explain"})
}

func (d *Daemon) runPatchSession(ctx context.Context, sessionID string, p StartParams) {
	d.send("session/status", map[string]any{"session_id": sessionID, "state": "planning", "message": "building patch"})

	var feedback string
	for attempt := 1; attempt <= 3; attempt++ {
		if attempt > 1 {
			d.send("session/status", map[string]any{"session_id": sessionID, "state": "planning", "message": fmt.Sprintf("retrying smaller patch (%d/3)", attempt)})
		}

		raw, err := d.requestPatch(ctx, p, feedback)
		if err != nil {
			if ctx.Err() != nil {
				d.send("session/status", map[string]any{"session_id": sessionID, "state": "cancelled", "message": ctx.Err().Error()})
				return
			}
			d.send("session/error", map[string]any{"session_id": sessionID, "error": err.Error()})
			return
		}

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
			d.send("session/status", map[string]any{
				"session_id": sessionID,
				"state":      "planning",
				"message":    "patch invalid: " + truncateForStatus(err.Error(), 160),
			})
			continue
		}

		d.send("session/status", map[string]any{"session_id": sessionID, "state": "writing", "message": "applying patch"})
		for i, edit := range edits {
			op := edit.Op
			if op == "" {
				op = "replace"
			}
			if op == "insert_at_selection_end" {
				d.send("edit/insert", map[string]any{
					"session_id":       sessionID,
					"file":             p.File,
					"base_changedtick": p.ChangedTick,
					"text":             edit.Text,
					"row":              p.Selection.End[0],
					"col":              p.Selection.End[1],
					"description":      edit.Description,
					"index":            i,
				})
				continue
			}
			d.send("edit/replace", map[string]any{
				"session_id":       sessionID,
				"file":             p.File,
				"base_changedtick": p.ChangedTick,
				"old":              edit.Old,
				"new":              edit.New,
				"description":      edit.Description,
				"index":            i,
			})
		}
		d.send("session/done", map[string]any{"session_id": sessionID, "file": p.File})
		return
	}

	d.send("session/error", map[string]any{"session_id": sessionID, "error": "could not get a valid surgical patch after 3 attempts", "details": feedback})
}

func (d *Daemon) requestPatch(ctx context.Context, p StartParams, feedback string) (string, error) {
	prompt := buildPrompt(p)
	if strings.TrimSpace(feedback) != "" {
		prompt += "\n\nPATCH VALIDATION FEEDBACK:\n" + feedback + "\n"
	}

	message, err := d.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaudeHaiku4_5,
		MaxTokens: 4096,
		System:    []anthropic.TextBlockParam{{Text: systemPrompt(p)}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})
	if err != nil {
		return "", err
	}

	var b strings.Builder
	for _, content := range message.Content {
		if content.Type == "text" {
			b.WriteString(content.Text)
		}
	}
	return b.String(), nil
}

func (d *Daemon) cancelSession(sessionID, reason string) {
	d.sessMu.Lock()
	cancel := d.sessions[sessionID]
	if sessionID == "" {
		for _, c := range d.sessions {
			c()
		}
		d.sessions = map[string]context.CancelFunc{}
	} else if cancel != nil {
		cancel()
		delete(d.sessions, sessionID)
	}
	d.sessMu.Unlock()
	d.send("session/status", map[string]any{"session_id": sessionID, "state": "cancelled", "message": reason})
}

func (d *Daemon) send(method string, params any) {
	d.outMu.Lock()
	defer d.outMu.Unlock()
	if err := d.out.Encode(Outgoing{Method: method, Params: params}); err != nil {
		fmt.Fprintf(os.Stderr, "encode: %v\n", err)
	}
}

func systemPrompt(p StartParams) string {
	if p.Selection != nil {
		return strings.Join([]string{
			"You are a Neovim co-editor prototype.",
			"The selected text is scope/context, not an automatic replacement target.",
			"Return ONLY JSON with this shape: {\"edits\":[{\"op\":\"replace\",\"old\":\"exact existing text\",\"new\":\"replacement text\",\"description\":\"short reason\"}]}",
			"Supported ops:",
			"- replace: exact old/new replacement for surgical modifications.",
			"- delete: exact old text to remove; use {\"op\":\"delete\",\"old\":\"...\"}.",
			"- insert_before: exact old anchor plus text to insert before it; use {\"op\":\"insert_before\",\"old\":\"...\",\"text\":\"...\"}.",
			"- insert_after: exact old anchor plus text to insert after it; use {\"op\":\"insert_after\",\"old\":\"...\",\"text\":\"...\"}.",
			"- insert_at_selection_end: append text at the end of the selected scope; use {\"op\":\"insert_at_selection_end\",\"text\":\"...\",\"description\":\"...\"}.",
			"Use insert_at_selection_end for additive requests like adding Python class methods or appending methods to the selected scope.",
			"Use the smallest exact old/new replacements that satisfy the user intent.",
			"Never rewrite the whole selected function/block unless the user explicitly says rewrite, replace, or regenerate the whole block.",
			"For adding a parameter, edit ONLY the function signature line and necessary callsite lines. Do not include the function body in old/new.",
			"For adding a comment, edit only the line(s) where the comment is inserted, or use old as the exact following line and new as comment plus that line.",
			"Prefer each old value to be one line. Maximum old size: 3 lines or 240 characters unless explicit whole-block rewrite requested.",
			"Every old value must be copied exactly from the current buffer and should appear once.",
			"Example replace: {\"edits\":[{\"op\":\"replace\",\"old\":\"func add(a int, b int) int {\",\"new\":\"func add(a int, b int, debug bool) int {\",\"description\":\"add debug parameter\"}]}",
			"Example insert: {\"edits\":[{\"op\":\"insert_at_selection_end\",\"text\":\"\\n    def reset(self):\\n        self.value = 0\\n\",\"description\":\"add reset method\"}]}",
			"Example multi-line surgical change: {\"edits\":[{\"op\":\"replace\",\"old\":\"    def __init__(self, name, age, height):\",\"new\":\"    def __init__(self, name, dob, height):\"},{\"op\":\"replace\",\"old\":\"        self.age = age\",\"new\":\"        self.dob = dob\"}]}",
			"Do not use markdown. Do not output code fences. Do not explain.",
		}, "\n")
	}
	return strings.Join([]string{
		"You are a Neovim co-editor prototype.",
		"Return ONLY raw editor text that should be inserted at the cursor.",
		"Do not use markdown. Never output code fences. Never output language labels.",
		"No explanation. No preamble. No trailing commentary.",
		"Preserve indentation and local style.",
		"If the user asks for a code change, output code only.",
	}, "\n")
}

func explainSystemPrompt() string {
	return strings.Join([]string{
		"You explain selected code inside a small Neovim popup.",
		"Be concise and concrete.",
		"Explain what the code does, important control/data flow, and any notable side effects or risks.",
		"Do not suggest edits unless the user explicitly asked.",
	}, "\n")
}

func buildExplainPrompt(p StartParams) string {
	var b strings.Builder
	intent := strings.TrimSpace(p.Prompt)
	if intent == "" {
		intent = "Explain the selected code."
	}
	fmt.Fprintf(&b, "User intent:\n%s\n\n", intent)
	fmt.Fprintf(&b, "Working directory: %s\n", p.CWD)
	fmt.Fprintf(&b, "File: %s\n", p.File)
	fmt.Fprintf(&b, "Filetype: %s\n", p.Filetype)
	fmt.Fprintf(&b, "Cursor row,col zero-based: %v\n\n", []int(p.Cursor))
	if p.Selection != nil {
		fmt.Fprintf(&b, "Selected range start/end zero-based: %v -> %v\n", []int(p.Selection.Start), []int(p.Selection.End))
		fmt.Fprintf(&b, "Selected code:\n---SELECTION---\n%s\n---END SELECTION---\n\n", p.Selection.Text)
	}
	content := p.Content
	if len(content) > 80000 {
		content = content[:80000] + "\n...TRUNCATED..."
	}
	fmt.Fprintf(&b, "Current buffer content for context:\n---FILE---\n%s\n---END FILE---\n", content)
	return b.String()
}

func buildPrompt(p StartParams) string {
	var b strings.Builder
	fmt.Fprintf(&b, "User intent:\n%s\n\n", p.Prompt)
	fmt.Fprintf(&b, "Working directory: %s\n", p.CWD)
	fmt.Fprintf(&b, "File: %s\n", p.File)
	fmt.Fprintf(&b, "Filetype: %s\n", p.Filetype)
	fmt.Fprintf(&b, "Cursor row,col zero-based: %v\n\n", []int(p.Cursor))
	if p.Selection != nil {
		fmt.Fprintf(&b, "Selected range start/end zero-based: %v -> %v\n", []int(p.Selection.Start), []int(p.Selection.End))
		fmt.Fprintf(&b, "Selected text used as edit scope/context:\n---SELECTION---\n%s\n---END SELECTION---\n\n", p.Selection.Text)
		fmt.Fprintf(&b, "Output JSON patch edits. Do not rewrite the full selected region unless the user explicitly asked to rewrite/replace the whole region. Prefer surgical old/new replacements.\n\n")
	} else {
		fmt.Fprintf(&b, "No selection. Output text to insert at cursor.\n\n")
	}
	content := p.Content
	if len(content) > 80000 {
		content = content[:80000] + "\n...TRUNCATED..."
	}
	fmt.Fprintf(&b, "Current buffer content:\n---FILE---\n%s\n---END FILE---\n", content)
	return b.String()
}

type MarkdownFenceSanitizer struct {
	prefixDone bool
	prefixBuf  string
	lineBuf    string
}

func NewMarkdownFenceSanitizer() *MarkdownFenceSanitizer {
	return &MarkdownFenceSanitizer{}
}

func (s *MarkdownFenceSanitizer) Add(chunk string) string {
	if chunk == "" {
		return ""
	}

	text := chunk
	if !s.prefixDone {
		s.prefixBuf += chunk
		var ready bool
		text, ready = s.consumePrefix()
		if !ready {
			return ""
		}
	}

	return s.processCompleteLines(text)
}

func (s *MarkdownFenceSanitizer) Flush() string {
	if !s.prefixDone {
		text, _ := s.consumePrefixForce()
		return s.processCompleteLines(text) + s.flushLine()
	}
	return s.flushLine()
}

func (s *MarkdownFenceSanitizer) consumePrefix() (string, bool) {
	buf := s.prefixBuf
	leading := len(buf) - len(strings.TrimLeft(buf, "\ufeff\r\n\t "))
	trimmed := buf[leading:]

	if trimmed == "" || strings.HasPrefix("```", trimmed) {
		return "", false
	}

	if strings.HasPrefix(trimmed, "```") {
		newline := strings.IndexByte(trimmed, '\n')
		if newline < 0 {
			return "", false
		}
		s.prefixDone = true
		s.prefixBuf = ""
		return trimmed[newline+1:], true
	}

	s.prefixDone = true
	s.prefixBuf = ""
	return buf, true
}

func (s *MarkdownFenceSanitizer) consumePrefixForce() (string, bool) {
	buf := s.prefixBuf
	s.prefixDone = true
	s.prefixBuf = ""

	leading := len(buf) - len(strings.TrimLeft(buf, "\ufeff\r\n\t "))
	trimmed := buf[leading:]
	if strings.HasPrefix(trimmed, "```") {
		newline := strings.IndexByte(trimmed, '\n')
		if newline < 0 {
			return "", true
		}
		return trimmed[newline+1:], true
	}
	return buf, true
}

func (s *MarkdownFenceSanitizer) processCompleteLines(text string) string {
	if text == "" {
		return ""
	}
	s.lineBuf += text

	var out strings.Builder
	for {
		newline := strings.IndexByte(s.lineBuf, '\n')
		if newline < 0 {
			break
		}
		line := s.lineBuf[:newline+1]
		s.lineBuf = s.lineBuf[newline+1:]
		if isMarkdownFenceLine(line) {
			continue
		}
		out.WriteString(line)
	}
	return out.String()
}

func (s *MarkdownFenceSanitizer) flushLine() string {
	line := s.lineBuf
	s.lineBuf = ""
	if isMarkdownFenceLine(line) {
		return ""
	}
	return line
}

func isMarkdownFenceLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "```")
}

func patchValidationMode() string {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("STEER_PATCH_VALIDATION")))
	if mode == "off" || mode == "false" || mode == "0" || mode == "yolo" {
		return "off"
	}
	return "strict"
}

func streamDelay() time.Duration {
	raw := strings.TrimSpace(os.Getenv("STEER_STREAM_DELAY_MS"))
	if raw == "" {
		return 0
	}
	ms, err := strconv.Atoi(raw)
	if err != nil || ms <= 0 {
		return 0
	}
	if ms > 1000 {
		ms = 1000
	}
	return time.Duration(ms) * time.Millisecond
}

func streamCharsPerChunk() int {
	raw := strings.TrimSpace(os.Getenv("STEER_STREAM_CHARS"))
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0
	}
	if n > 200 {
		n = 200
	}
	return n
}

func splitForVisibleStreaming(text string, charsPerChunk int) []string {
	if charsPerChunk <= 0 {
		return []string{text}
	}

	var parts []string
	var b strings.Builder
	count := 0
	for _, r := range text {
		b.WriteRune(r)
		count++
		if r == '\n' || count >= charsPerChunk {
			parts = append(parts, b.String())
			b.Reset()
			count = 0
		}
	}
	if b.Len() > 0 {
		parts = append(parts, b.String())
	}
	return parts
}

func loadAnthropicAPIKey() {
	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		return
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
			if !ok || strings.TrimSpace(key) != "ANTHROPIC_API_KEY" {
				continue
			}
			value = strings.TrimSpace(value)
			value = strings.Trim(value, "\"'")
			_ = os.Setenv("ANTHROPIC_API_KEY", value)
			return
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

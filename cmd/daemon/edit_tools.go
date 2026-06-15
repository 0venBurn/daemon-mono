package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/0venburn/daemon/internal/fff"
	"github.com/0venburn/daemon/internal/llm"
)

// Safety limit: prevent true infinite loops. The loop exits naturally when
// the model returns no tool calls, so this is just a backstop.
const maxTurns = 50

func (d *Daemon) runEditSession(ctx context.Context, sessionID string, p StartParams) {
	sessionStart := time.Now()
	d.sendSessionStatus(sessionID, "planning", "editing "+p.File)
	d.writeDebug("session/prompt", map[string]any{"session_id": sessionID, "file": p.File, "prompt": p.Prompt})

	current := p
	if ok := d.runPatchForFile(ctx, sessionID, current); !ok {
		return
	}
	d.writeDebug("session/total", map[string]any{"session_id": sessionID, "file": current.File, "elapsed_ms": time.Since(sessionStart).Milliseconds()})
	d.sendSessionDone(sessionID, current.File, "")
}

func (d *Daemon) runPatchForFile(ctx context.Context, sessionID string, p StartParams) bool {
	edits, err := d.requestEditTools(ctx, sessionID, p)
	if err != nil {
		if ctx.Err() != nil {
			d.sendSessionStatus(sessionID, "cancelled", ctx.Err().Error())
			return false
		}
		d.sendSessionError(sessionID, err.Error())
		return false
	}
	if len(edits) == 0 {
		return true
	}

	d.sendSessionStatus(sessionID, "writing", "applying tool edits")
	for i, edit := range edits {
		targetFile := p.File
		if edit.File != "" {
			if !filepath.IsAbs(edit.File) {
				targetFile = filepath.Join(p.CWD, edit.File)
			} else {
				targetFile = edit.File
			}
		}
		if edit.Op == "create_file" {
			d.sendEditCreate(sessionID, edit.Path, edit.Text, edit.Description, i)
			continue
		}
		if edit.Op == "insert_at" {
			d.sendEditInsert(sessionID, targetFile, p.ChangedTick, edit.Text, edit.Row, edit.Col, edit.Description, i)
			continue
		}
		d.sendEditReplace(sessionID, targetFile, p.ChangedTick, edit.Old, edit.New, edit.Description, i)
	}
	return true
}

func (d *Daemon) handleExploreTool(name string, input json.RawMessage, cwd string) string {
	switch name {
	case "read_file":
		var in readFileInput
		if err := json.Unmarshal(input, &in); err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		path := in.Path
		if !filepath.IsAbs(path) {
			path = filepath.Join(cwd, path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Sprintf("Error reading %s: %v", in.Path, err)
		}
		content := string(data)
		if len(content) > 80000 {
			content = content[:80000] + "\n...TRUNCATED..."
		}
		return fmt.Sprintf("Contents of %s:\n---FILE---\n%s\n---END FILE---", in.Path, content)
	case "list_directory":
		var in listDirectoryInput
		if err := json.Unmarshal(input, &in); err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		path := in.Path
		if path == "" {
			path = cwd
		} else if !filepath.IsAbs(path) {
			path = filepath.Join(cwd, path)
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return fmt.Sprintf("Error listing %s: %v", in.Path, err)
		}
		var lines []string
		for _, e := range entries {
			if e.IsDir() {
				lines = append(lines, e.Name()+"/")
			} else {
				lines = append(lines, e.Name())
			}
		}
		return fmt.Sprintf("Contents of %s:\n%s", path, strings.Join(lines, "\n"))
	case "find_files":
		var in findFilesInput
		if err := json.Unmarshal(input, &in); err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		if d.finder == nil {
			return "Error: file search not available"
		}
		sr, err := d.finder.SearchFiles(in.Query, fff.WithCurrentFile(cwd))
		if err != nil {
			return fmt.Sprintf("Error searching files: %v", err)
		}
		return fff.FormatSearchResult(sr)

	case "grep":
		var in grepInput
		if err := json.Unmarshal(input, &in); err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		if d.finder == nil {
			return "Error: file search not available"
		}
		opts := []fff.GrepOpt{fff.WithGrepPageLimit(30), fff.WithGrepContext(1, 1)}
		switch in.Mode {
		case "regex":
			opts = append(opts, fff.WithGrepMode(fff.GrepRegex))
		case "fuzzy":
			opts = append(opts, fff.WithGrepMode(fff.GrepFuzzy))
		default:
			opts = append(opts, fff.WithGrepMode(fff.GrepPlain))
		}
		if in.MaxLines > 0 {
			opts = append(opts, fff.WithGrepPageLimit(in.MaxLines))
		}
		gr, err := d.finder.Grep(in.Query, opts...)
		if err != nil {
			return fmt.Sprintf("Error grepping: %v", err)
		}
		return fff.FormatGrepResult(gr)
	default:
		return fmt.Sprintf("Unknown exploration tool: %s", name)
	}
}

func (d *Daemon) buildEditSystemPrompt(p StartParams) string {
	var parts []string
	if d.finder != nil {
		parts = append(parts, "You are a Neovim co-editor with seven tools:")
	} else {
		parts = append(parts, "You are a Neovim co-editor with five tools:")
	}
	parts = append(parts, "- replace_text(old, new, file?): find and replace exact text. Use file param to target a different file.")
	parts = append(parts, "- insert_at(row, col, text, file?): insert text at position. Use file param to target a different file.")
	parts = append(parts, "- create_file(path, text): create a new file with the given content")
	parts = append(parts, "- read_file(path): read a file's contents to understand it before editing")
	parts = append(parts, "- list_directory(path): list files in a directory")
	if d.finder != nil {
		parts = append(parts, "- find_files(query): fuzzy search for files by name or path. Prefer over glob patterns.")
		parts = append(parts, "- grep(query, mode?): search file contents. Plain by default, auto-falls back to fuzzy on zero matches.")
		parts = append(parts, "When the user asks to edit a different file, use find_files or grep to locate it, then read_file to see its contents, then use replace_text or insert_at with the file parameter set to that file path.")
	} else {
		parts = append(parts, "When the user asks to edit a different file, use list_directory and read_file to inspect it, then use replace_text or insert_at with the file parameter set to that file path.")
	}
	parts = append(parts, "Do NOT write Python or shell code that creates files — use create_file for new files, replace_text/insert_at with file param for edits to other files.")
	parts = append(parts, "Use tools when the request requires changing files. Call zero tools only when no file changes are needed.")
	return strings.Join(parts, "\n")
}

func buildToolEditPrompt(p StartParams) string {
	var b strings.Builder
	fmt.Fprintf(&b, "User intent:\n%s\n\n", p.Prompt)
	fmt.Fprintf(&b, "Working directory: %s\n", p.CWD)
	fmt.Fprintf(&b, "File: %s\n", p.File)
	fmt.Fprintf(&b, "Filetype: %s\n", p.Filetype)
	fmt.Fprintf(&b, "Cursor row,col zero-based: %v\n\n", []int(p.Cursor))
	if p.Selection != nil {
		fmt.Fprintf(&b, "Selected range start/end zero-based: %v -> %v\n", []int(p.Selection.Start), []int(p.Selection.End))
		fmt.Fprintf(&b, "Selected text:\n---SELECTION---\n%s\n---END SELECTION---\n\n", p.Selection.Text)
	}
	content := p.Content
	if len(content) > 80000 {
		content = content[:80000] + "\n...TRUNCATED..."
	}
	fmt.Fprintf(&b, "Current buffer content:\n---FILE---\n%s\n---END FILE---\n", content)
	return b.String()
}

func (d *Daemon) requestEditTools(ctx context.Context, sessionID string, p StartParams) ([]toolEdit, error) {
	prompt := buildToolEditPrompt(p)
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: []llm.ContentBlock{llm.TextBlock(prompt)}},
	}

	var allEdits []toolEdit
	for turn := 0; turn < maxTurns; turn++ {
		// Check for cancellation
		if ctx.Err() != nil {
			return allEdits, ctx.Err()
		}

		d.sendSessionStatus(sessionID, "thinking", fmt.Sprintf("calling model (turn %d)", turn+1))
		apiStart := time.Now()
		system := d.buildEditSystemPrompt(p)
		tools := d.editTools()
		debug := map[string]any{"session_id": sessionID, "file": p.File, "turn": turn, "mode": "tools", "provider": d.config.Name, "model": d.config.Model, "base_url": d.config.BaseURL, "prompt_bytes": len(prompt), "system_bytes": len(system), "content_bytes": len(p.Content), "tools": len(tools), "messages": len(messages)}
		d.writeDebug("model/request", debug)
		message, err := d.streamMessage(ctx, sessionID, messages, system, tools, debug)
		if err != nil {
			return allEdits, err
		}
		apiMS := time.Since(apiStart).Milliseconds()
		d.writeDebug("model/response", map[string]any{"session_id": sessionID, "file": p.File, "turn": turn, "api_ms": apiMS, "content": message.Content})
		d.sendSessionStatus(sessionID, "planning", fmt.Sprintf("model response received in %dms", apiMS))

		var toolCalls []llm.ContentBlock
		var resultBlocks []llm.ContentBlock
		var textParts []string
		current := p.Content
		editCountBeforeTurn := len(allEdits)

		for _, block := range message.Content {
			if block.Type == "text" {
				if strings.TrimSpace(block.Text) != "" {
					textParts = append(textParts, block.Text)
				}
				continue
			}
			if block.Type != "tool_use" {
				continue
			}
			toolCalls = append(toolCalls, block)

			// Execute each tool call. Every branch MUST add a resultBlock
			// so that every tool_use has a matching tool_result — same guarantee
			// as Pi/OpenCode where no tool call is left orphaned.
			result, edit := d.executeToolCall(block, sessionID, &p, &current)
			resultBlocks = append(resultBlocks, result)
			if edit != nil {
				allEdits = append(allEdits, *edit)
			}
		}

		// No tool calls: model decided it's done. Return whatever edits we collected.
		if len(toolCalls) == 0 {
			d.writeDebug("session/edit_result", map[string]any{"file": p.File, "turn": turn, "edits": len(allEdits), "reason": "no_tool_calls"})
			return allEdits, nil
		}
		// If this turn produced concrete edits, skip the extra confirmation turn.
		// The editor UI already applies/animates the edits, and avoiding this turn
		// removes 1-12s of visible latency on OpenAI-compatible proxies.
		if len(allEdits) > editCountBeforeTurn {
			d.writeDebug("session/edit_result", map[string]any{"file": p.File, "turn": turn, "edits": len(allEdits), "reason": "edit_tools_complete"})
			return allEdits, nil
		}

		// Feed tool results back and loop
		messages = append(messages, llm.Message{Role: llm.RoleAssistant, Content: message.Content})
		messages = append(messages, llm.Message{Role: llm.RoleUser, Content: resultBlocks})
	}

	// Safety limit reached
	d.writeDebug("session/edit_result", map[string]any{"file": p.File, "turn": maxTurns, "edits": len(allEdits), "reason": "max_turns"})
	return allEdits, nil
}

// executeToolCall processes a single tool call block and returns a result block
// and an optional edit. Guarantees a result block is always returned for every
// tool_use, mirroring Pi/OpenCode's pattern of never leaving a tool call orphaned.
func (d *Daemon) executeToolCall(block llm.ContentBlock, sessionID string, p *StartParams, current *string) (llm.ContentBlock, *toolEdit) {
	switch block.Name {
	case "replace_text":
		var in replaceTextInput
		if err := json.Unmarshal(block.Input, &in); err != nil {
			return llm.ToolResultBlock(block.ID, fmt.Sprintf("Error: invalid replace_text arguments: %v", err), true), nil
		}
		if in.Old == "" || in.Old == in.New {
			return llm.ToolResultBlock(block.ID, "Error: replace_text requires non-empty, differing old and new text.", true), nil
		}
		toolFile := p.File
		if in.File != "" {
			toolFile = in.File
		}
		d.sendToolStartArgs(sessionID, "replace_text", toolFile, 0, 0, in.Description, block.Input)
		// Validate match count for current buffer and cross-file edits
		if in.File == "" {
			matches := countOccurrences(*current, in.Old)
			if matches != 1 {
				d.sendToolDone(sessionID, "replace_text", toolFile, 0, 0)
				return llm.ToolResultBlock(block.ID, fmt.Sprintf("Error: old text found %d times in current buffer (expected exactly 1). Re-read the file or adjust your old text.", matches), true), nil
			}
			*current = strings.Replace(*current, in.Old, in.New, 1)
		} else {
			// Cross-file edit: validate against target file contents
			targetPath := in.File
			if !filepath.IsAbs(targetPath) {
				targetPath = filepath.Join(p.CWD, targetPath)
			}
			data, err := os.ReadFile(targetPath)
			if err != nil {
				d.sendToolDone(sessionID, "replace_text", toolFile, 0, 0)
				return llm.ToolResultBlock(block.ID, fmt.Sprintf("Error: could not read %s: %v. Use read_file first.", in.File, err), true), nil
			}
			targetContent := string(data)
			matches := countOccurrences(targetContent, in.Old)
			if matches != 1 {
				d.sendToolDone(sessionID, "replace_text", toolFile, 0, 0)
				return llm.ToolResultBlock(block.ID, fmt.Sprintf("Error: old text found %d times in %s (expected exactly 1). Use read_file to check current contents.", matches, in.File), true), nil
			}
		}
		result := fmt.Sprintf("Replaced 1 occurrence of %d chars", len(in.Old))
		d.sendToolDoneResultArgs(sessionID, "replace_text", toolFile, 0, 0, result, block.Input)
		return llm.ToolResultBlock(block.ID, result, false), &toolEdit{Op: "replace_text", Old: in.Old, New: in.New, File: in.File, Description: in.Description}

	case "insert_at":
		var in insertAtInput
		if err := json.Unmarshal(block.Input, &in); err != nil {
			return llm.ToolResultBlock(block.ID, fmt.Sprintf("Error: invalid insert_at arguments: %v", err), true), nil
		}
		if in.Text == "" {
			return llm.ToolResultBlock(block.ID, "Error: insert_at requires non-empty text.", true), nil
		}
		toolFile := p.File
		if in.File != "" {
			toolFile = in.File
		}
		d.sendToolStartArgs(sessionID, "insert_at", toolFile, in.Row, in.Col, in.Description, block.Input)
		result := fmt.Sprintf("Inserted %d chars at row %d col %d", len(in.Text), in.Row, in.Col)
		d.sendToolDoneResultArgs(sessionID, "insert_at", toolFile, in.Row, in.Col, result, block.Input)
		return llm.ToolResultBlock(block.ID, result, false), &toolEdit{Op: "insert_at", Row: in.Row, Col: in.Col, Text: in.Text, File: in.File, Description: in.Description}

	case "create_file":
		var in createFileInput
		if err := json.Unmarshal(block.Input, &in); err != nil {
			return llm.ToolResultBlock(block.ID, fmt.Sprintf("Error: invalid create_file arguments: %v", err), true), nil
		}
		if in.Path == "" {
			return llm.ToolResultBlock(block.ID, "Error: create_file requires a file path.", true), nil
		}
		d.sendToolStartArgs(sessionID, "create_file", in.Path, 0, 0, in.Description, block.Input)
		result := fmt.Sprintf("Created %s", in.Path)
		d.sendToolDoneResultArgs(sessionID, "create_file", in.Path, 0, 0, result, block.Input)
		return llm.ToolResultBlock(block.ID, result, false), &toolEdit{Op: "create_file", Path: in.Path, Text: in.Text, Description: in.Description}

	case "read_file":
		var in readFileInput
		_ = json.Unmarshal(block.Input, &in)
		d.sendToolStartArgs(sessionID, "read_file", in.Path, 0, 0, "", block.Input)
		result := d.handleExploreTool(block.Name, block.Input, p.CWD)
		d.sendToolDoneResultArgs(sessionID, "read_file", in.Path, 0, 0, result, block.Input)
		return llm.ToolResultBlock(block.ID, result, false), nil

	case "list_directory":
		var in listDirectoryInput
		_ = json.Unmarshal(block.Input, &in)
		d.sendToolStartArgs(sessionID, "list_directory", in.Path, 0, 0, "", block.Input)
		result := d.handleExploreTool(block.Name, block.Input, p.CWD)
		d.sendToolDoneResultArgs(sessionID, "list_directory", in.Path, 0, 0, result, block.Input)
		return llm.ToolResultBlock(block.ID, result, false), nil

	case "find_files":
		var in findFilesInput
		_ = json.Unmarshal(block.Input, &in)
		d.sendToolStartArgs(sessionID, "find_files", "", 0, 0, "", block.Input)
		result := d.handleExploreTool(block.Name, block.Input, p.CWD)
		d.sendToolDoneResultArgs(sessionID, "find_files", "", 0, 0, result, block.Input)
		return llm.ToolResultBlock(block.ID, result, false), nil

	case "grep":
		var in grepInput
		_ = json.Unmarshal(block.Input, &in)
		file := in.File
		d.sendToolStartArgs(sessionID, "grep", file, 0, 0, "", block.Input)
		result := d.handleExploreTool(block.Name, block.Input, p.CWD)
		d.sendToolDoneResultArgs(sessionID, "grep", file, 0, 0, result, block.Input)
		return llm.ToolResultBlock(block.ID, result, false), nil

	default:
		return llm.ToolResultBlock(block.ID, fmt.Sprintf("Error: unknown tool %q", block.Name), true), nil
	}
}

func (d *Daemon) writeEventDebug(method string, params any) {
	p := d.debugPath()
	if p == "" {
		return
	}
	payload := map[string]any{
		"kind":   "event",
		"method": method,
		"params": params,
		"cwd":    mustGetwd(),
	}
	d.writeDebugPayload(p, payload)
}

func (d *Daemon) writeDebug(kind string, payload map[string]any) {
	p := d.debugPath()
	if p == "" {
		return
	}
	payload["kind"] = kind
	payload["cwd"] = mustGetwd()
	d.writeDebugPayload(p, payload)
}

func (d *Daemon) writeDebugPayload(path string, payload map[string]any) {
	now := time.Now()
	payload["ts"] = now.Format("15:04:05.000")
	b, err := json.Marshal(payload)
	if err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(b)
	_, _ = f.Write([]byte("\n"))
}

func (d *Daemon) debugPath() string {
	if p := os.Getenv("DAEMON_DEBUG_FILE"); p != "" {
		return p
	}
	return filepath.Join(os.TempDir(), "daemon-debug.jsonl")
}

func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return "?"
	}
	return wd
}

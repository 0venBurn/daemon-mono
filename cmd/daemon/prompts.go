package main

import (
	"fmt"
	"strings"
)

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
		"Return ONLY JSON with this shape: {\"edits\":[{\"op\":\"replace\",\"old\":\"exact existing text\",\"new\":\"replacement text\",\"description\":\"short reason\"}]}",
		"Supported ops:",
		"- replace: exact old/new replacement for surgical modifications.",
		"- delete: exact old text to remove; use {\"op\":\"delete\",\"old\":\"...\"}.",
		"- insert_before: exact old anchor plus text to insert before it; use {\"op\":\"insert_before\",\"old\":\"...\",\"text\":\"...\"}.",
		"- insert_after: exact old anchor plus text to insert after it; use {\"op\":\"insert_after\",\"old\":\"...\",\"text\":\"...\"}.",
		"Use the smallest exact old/new replacements that satisfy the user intent.",
		"Every old value must be copied exactly from the current buffer and should appear once.",
		"Do not use markdown. Do not output code fences. Do not explain.",
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
		fmt.Fprintf(&b, "No selection. Edit only this current buffer using JSON patch edits. Use exact old/new replacements or anchored inserts. Do not request other files.\n\n")
	}
	content := p.Content
	if len(content) > 80000 {
		content = content[:80000] + "\n...TRUNCATED..."
	}
	fmt.Fprintf(&b, "Current buffer content:\n---FILE---\n%s\n---END FILE---\n", content)
	return b.String()
}

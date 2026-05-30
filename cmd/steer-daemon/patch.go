package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// PatchResponse is the model's JSON patch envelope.
// It stays small because patch policy lives in the functions below.
type PatchResponse struct {
	Edits []PatchEdit `json:"edits"`
}

// PatchEdit describes one editor-owned edit operation.
// The daemon validates and normalizes this shape; the surface applies it.
type PatchEdit struct {
	Op          string `json:"op,omitempty"`
	Old         string `json:"old,omitempty"`
	New         string `json:"new,omitempty"`
	Text        string `json:"text,omitempty"`
	Description string `json:"description,omitempty"`
}

func preparePatchResponse(patch PatchResponse) ([]PatchEdit, error) {
	if len(patch.Edits) == 0 {
		return nil, fmt.Errorf("patch response had no edits")
	}

	prepared := make([]PatchEdit, 0, len(patch.Edits))
	for i, edit := range patch.Edits {
		normalized, ok, err := normalizePatchEdit(i, edit)
		if err != nil {
			return nil, err
		}
		if ok {
			prepared = append(prepared, normalized)
		}
	}
	if len(prepared) == 0 {
		return nil, fmt.Errorf("no non-empty edits remained after preparation")
	}
	return prepared, nil
}

func normalizePatchEdit(i int, edit PatchEdit) (PatchEdit, bool, error) {
	op := edit.Op
	if op == "" {
		op = "replace"
	}
	edit.Op = op

	switch op {
	case "insert_at_selection_end":
		if edit.Text == "" {
			return edit, false, fmt.Errorf("edit %d insert_at_selection_end has empty text", i)
		}
		return edit, true, nil
	case "replace":
		if edit.Old == "" {
			return edit, false, fmt.Errorf("edit %d has empty old text", i)
		}
		if edit.Old == edit.New {
			return edit, false, nil
		}
		return edit, true, nil
	case "delete":
		if edit.Old == "" {
			return edit, false, fmt.Errorf("edit %d delete has empty old text", i)
		}
		edit.Op = "replace"
		edit.New = ""
		return edit, true, nil
	case "insert_before":
		if edit.Old == "" || edit.Text == "" {
			return edit, false, fmt.Errorf("edit %d insert_before needs old anchor and text", i)
		}
		edit.Op = "replace"
		edit.New = edit.Text + edit.Old
		return edit, true, nil
	case "insert_after":
		if edit.Old == "" || edit.Text == "" {
			return edit, false, fmt.Errorf("edit %d insert_after needs old anchor and text", i)
		}
		edit.Op = "replace"
		edit.New = edit.Old + edit.Text
		return edit, true, nil
	default:
		return edit, false, fmt.Errorf("edit %d has unsupported op %q", i, edit.Op)
	}
}

func validatePatchResponse(content, prompt string, patch PatchResponse) ([]PatchEdit, error) {
	if len(patch.Edits) == 0 {
		return nil, fmt.Errorf("patch response had no edits")
	}

	current := content
	valid := make([]PatchEdit, 0, len(patch.Edits))
	var problems []string
	for i, edit := range patch.Edits {
		op := edit.Op
		if op == "" {
			op = "replace"
		}
		switch op {
		case "insert_at_selection_end":
			if edit.Text == "" {
				problems = append(problems, fmt.Sprintf("edit %d insert_at_selection_end has empty text", i))
				continue
			}
			if !isExplicitRewriteIntent(prompt) && isBroadInsert(edit.Text) {
				problems = append(problems, fmt.Sprintf("edit %d insert text is too broad; add fewer methods or explicitly ask to rewrite/regenerate", i))
				continue
			}
			valid = append(valid, edit)
			current += edit.Text
			continue
		case "replace":
			// validate below
		case "delete":
			edit.Op = "replace"
			edit.New = ""
		case "insert_before":
			if edit.Text == "" {
				problems = append(problems, fmt.Sprintf("edit %d insert_before has empty text", i))
				continue
			}
			edit.Op = "replace"
			edit.New = edit.Text + edit.Old
		case "insert_after":
			if edit.Text == "" {
				problems = append(problems, fmt.Sprintf("edit %d insert_after has empty text", i))
				continue
			}
			edit.Op = "replace"
			edit.New = edit.Old + edit.Text
		default:
			problems = append(problems, fmt.Sprintf("edit %d has unsupported op %q", i, edit.Op))
			continue
		}

		if edit.Old == "" {
			problems = append(problems, fmt.Sprintf("edit %d has empty old text", i))
			continue
		}
		if edit.Old == edit.New {
			continue
		}

		matches := countOccurrences(current, edit.Old)
		switch matches {
		case 0:
			problems = append(problems, fmt.Sprintf("edit %d old text was not found", i))
			continue
		case 1:
			// ok
		default:
			expanded, nextCurrent, ok := expandAmbiguousInlineReplace(current, edit)
			if ok {
				valid = append(valid, expanded...)
				current = nextCurrent
				continue
			}
			problems = append(problems, fmt.Sprintf("edit %d old text matched %d times; choose a more specific one-line old value", i, matches))
			continue
		}

		if !isExplicitRewriteIntent(prompt) && isBroadPatch(edit.Old, edit.New) {
			oldMidLen, newMidLen, oldMidLines, newMidLines := replacementSize(edit.Old, edit.New)
			problems = append(problems, fmt.Sprintf(
				"edit %d is too broad after minimal diff (old_mid_len=%d new_mid_len=%d old_mid_lines=%d new_mid_lines=%d); use smaller one-line old/new edits",
				i, oldMidLen, newMidLen, oldMidLines, newMidLines,
			))
			continue
		}

		current = strings.Replace(current, edit.Old, edit.New, 1)
		valid = append(valid, edit)
	}

	if len(problems) > 0 {
		return nil, errors.New(strings.Join(problems, "\n"))
	}
	if len(valid) == 0 {
		return nil, fmt.Errorf("no non-empty edits remained after validation")
	}
	return valid, nil
}

func countOccurrences(s, substr string) int {
	if substr == "" {
		return 0
	}
	count := 0
	start := 0
	for {
		idx := strings.Index(s[start:], substr)
		if idx < 0 {
			return count
		}
		count++
		start += idx + len(substr)
	}
}

func expandAmbiguousInlineReplace(content string, edit PatchEdit) ([]PatchEdit, string, bool) {
	if edit.Old == "" || strings.Count(edit.Old, "\n") > 1 || strings.Count(edit.New, "\n") > 1 || len(edit.Old) > 120 || len(edit.New) > 120 {
		return nil, content, false
	}

	current := content
	lines := strings.SplitAfter(content, "\n")
	var expanded []PatchEdit
	for i, line := range lines {
		if !strings.Contains(line, edit.Old) {
			continue
		}
		newLine := strings.ReplaceAll(line, edit.Old, edit.New)
		if newLine == line {
			continue
		}

		oldText := line
		newText := newLine
		if countOccurrences(current, oldText) != 1 {
			var ok bool
			oldText, newText, ok = contextualizeLinePatch(current, lines, i, newLine)
			if !ok {
				return nil, content, false
			}
		}

		lineEdit := PatchEdit{
			Op:          "replace",
			Old:         oldText,
			New:         newText,
			Description: edit.Description,
		}
		if isBroadPatch(lineEdit.Old, lineEdit.New) {
			return nil, content, false
		}
		expanded = append(expanded, lineEdit)
		current = strings.Replace(current, oldText, newText, 1)
	}

	if len(expanded) == 0 {
		return nil, content, false
	}
	return expanded, current, true
}

func contextualizeLinePatch(current string, lines []string, i int, newLine string) (string, string, bool) {
	line := lines[i]
	candidates := [][2]string{}
	if i > 0 {
		candidates = append(candidates, [2]string{lines[i-1] + line, lines[i-1] + newLine})
	}
	if i+1 < len(lines) {
		candidates = append(candidates, [2]string{line + lines[i+1], newLine + lines[i+1]})
	}
	if i > 0 && i+1 < len(lines) {
		candidates = append(candidates, [2]string{lines[i-1] + line + lines[i+1], lines[i-1] + newLine + lines[i+1]})
	}

	for _, candidate := range candidates {
		if countOccurrences(current, candidate[0]) == 1 {
			return candidate[0], candidate[1], true
		}
	}
	return "", "", false
}

func truncateForStatus(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func isExplicitRewriteIntent(prompt string) bool {
	p := strings.ToLower(prompt)
	return strings.Contains(p, "rewrite") || strings.Contains(p, "replace whole") || strings.Contains(p, "replace the whole") || strings.Contains(p, "regenerate")
}

func isBroadPatch(old, new string) bool {
	oldMidLen, newMidLen, oldMidLines, newMidLines := replacementSize(old, new)
	if oldMidLen > 240 || newMidLen > 240 {
		return true
	}
	return oldMidLines >= 3 || newMidLines >= 3
}

func isBroadInsert(text string) bool {
	return len(text) > 1200 || strings.Count(text, "\n") > 40
}

func replacementSize(old, new string) (oldMidLen, newMidLen, oldMidLines, newMidLines int) {
	prefix := 0
	maxPrefix := min(len(old), len(new))
	for prefix < maxPrefix && old[prefix] == new[prefix] {
		prefix++
	}

	suffix := 0
	maxSuffix := min(len(old)-prefix, len(new)-prefix)
	for suffix < maxSuffix && old[len(old)-1-suffix] == new[len(new)-1-suffix] {
		suffix++
	}

	oldMid := old[prefix : len(old)-suffix]
	newMid := new[prefix : len(new)-suffix]
	return len(oldMid), len(newMid), strings.Count(oldMid, "\n"), strings.Count(newMid, "\n")
}

func parsePatchResponse(raw string) (PatchResponse, error) {
	var patch PatchResponse
	text := strings.TrimSpace(raw)
	if text == "" {
		return patch, fmt.Errorf("empty patch response")
	}
	start := strings.IndexByte(text, '{')
	end := strings.LastIndexByte(text, '}')
	if start < 0 || end < start {
		return patch, fmt.Errorf("patch response was not JSON")
	}
	text = text[start : end+1]
	if err := json.Unmarshal([]byte(text), &patch); err != nil {
		return patch, fmt.Errorf("invalid patch JSON: %w", err)
	}
	return patch, nil
}

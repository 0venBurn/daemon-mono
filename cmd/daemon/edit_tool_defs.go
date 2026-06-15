package main

import "github.com/0venburn/daemon/internal/llm"

type toolEdit struct {
	Op          string
	Old         string
	New         string
	Text        string
	Row         int
	Col         int
	Path        string
	File        string // target file, empty means current buffer
	Description string
}

type replaceTextInput struct {
	Old         string `json:"old"`
	New         string `json:"new"`
	File        string `json:"file"` // target file path, empty means current buffer
	Description string `json:"description"`
}

type insertAtInput struct {
	Row         int    `json:"row"`
	Col         int    `json:"col"`
	Text        string `json:"text"`
	File        string `json:"file"` // target file path, empty means current buffer
	Description string `json:"description"`
}

type createFileInput struct {
	Path        string `json:"path"`
	Text        string `json:"text"`
	Description string `json:"description"`
}

type readFileInput struct {
	Path string `json:"path"`
}

type listDirectoryInput struct {
	Path string `json:"path"`
}

type findFilesInput struct {
	Query string `json:"query"` // fuzzy file name or path search
}

type grepInput struct {
	Query    string `json:"query"`     // content search term
	Mode     string `json:"mode"`      // plain (default), regex, fuzzy
	File     string `json:"file"`      // optional: restrict to a single file path
	MaxLines uint32 `json:"max_lines"` // optional: max result lines (default 30)
}

func (d *Daemon) editTools() []llm.ToolDef {
	tools := []llm.ToolDef{
		{
			Name:        "replace_text",
			Description: "Find and replace exact text. Use the file parameter to target a different file.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"old":         map[string]any{"type": "string", "description": "Exact text to find"},
					"new":         map[string]any{"type": "string", "description": "Replacement text (must differ from old)"},
					"file":        map[string]any{"type": "string", "description": "Target file path. Use to edit a different file."},
					"description": map[string]any{"type": "string", "description": "Short description of the change"},
				},
				"required": []string{"old", "new"},
			},
		},
		{
			Name:        "insert_at",
			Description: "Insert text at a specific row,col position in a file.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"row":         map[string]any{"type": "integer", "description": "Row number (0-based)"},
					"col":         map[string]any{"type": "integer", "description": "Column number (0-based)"},
					"text":        map[string]any{"type": "string", "description": "Text to insert"},
					"file":        map[string]any{"type": "string", "description": "Target file path. Use to insert into a different file."},
					"description": map[string]any{"type": "string", "description": "Short description of the change"},
				},
				"required": []string{"row", "col", "text"},
			},
		},
		{
			Name:        "create_file",
			Description: "Create a new file with the given content. Use only for new files.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":        map[string]any{"type": "string", "description": "Path for the new file"},
					"text":        map[string]any{"type": "string", "description": "Complete file contents to write"},
					"description": map[string]any{"type": "string", "description": "Short description"},
				},
				"required": []string{"path", "text"},
			},
		},
		{
			Name:        "read_file",
			Description: "Read the contents of a file. Use before editing unfamiliar files.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string", "description": "Path to the file to read"},
				},
				"required": []string{"path"},
			},
		},
		{
			Name:        "list_directory",
			Description: "List files and directories at a path.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string", "description": "Directory path. Defaults to CWD."},
				},
			},
		},
	}
	if d.finder != nil {
		tools = append(tools,
			llm.ToolDef{
				Name:        "find_files",
				Description: "Search files by name or path fragment. Typo-tolerant fuzzy matching with frecency ranking. Prefer this over glob patterns.",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query": map[string]any{"type": "string", "description": "File name or path fragment (e.g. 'file_ops', 'main.go', 'internal/edit')"},
					},
					"required": []string{"query"},
				},
			},
			llm.ToolDef{
				Name:        "grep",
				Description: "Search file contents. Smart-case with auto fuzzy fallback. Use to find where a function is defined or a string appears.",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query":     map[string]any{"type": "string", "description": "Search term or regex pattern"},
						"mode":      map[string]any{"type": "string", "description": "plain (default), regex, or fuzzy"},
						"max_lines": map[string]any{"type": "integer", "description": "Max result lines (default 30)"},
					},
					"required": []string{"query"},
				},
			},
		)
	}
	return tools
}

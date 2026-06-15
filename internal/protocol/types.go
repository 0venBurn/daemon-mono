package protocol

type Event struct {
	Type string `json:"type"`
	Data any    `json:"data,omitempty"`
}

type EditReplacement struct {
	OldText string `json:"old_text"`
	NewText string `json:"new_text"`
}

type EditOp struct {
	Path  string            `json:"path"`
	Edits []EditReplacement `json:"edits"`
}

type EditResult struct {
	Path    string `json:"path"`
	Changed bool   `json:"changed"`
}

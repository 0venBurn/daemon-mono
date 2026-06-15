package surface

import (
	"context"
	"fmt"

	"github.com/0venburn/daemon/internal/edit"
	"github.com/0venburn/daemon/internal/protocol"
)

// FakeSurface implements Surface for tests.
//
// EXERCISE 1: Complete the Edit and Publish methods below.
// Read explainers/001-go-interfaces-for-daemon.html for reference implementations.
type FakeSurface struct {
	Files  map[string]string
	Edits  []protocol.EditOp
	Events []protocol.Event
}

// NewFakeSurface creates a FakeSurface seeded with file contents.
func NewFakeSurface(files map[string]string) *FakeSurface {
	return &FakeSurface{
		Files:  files,
		Edits:  make([]protocol.EditOp, 0),
		Events: make([]protocol.Event, 0),
	}
}

// Compile-time check: if FakeSurface doesn't satisfy Surface, this won't compile.
var _ Surface = (*FakeSurface)(nil)

func (f *FakeSurface) Read(_ context.Context, path string) (string, error) {
	content, ok := f.Files[path]
	if !ok {
		return "", fmt.Errorf("file not found: %s", path)
	}
	return content, nil
}

func (f *FakeSurface) Edit(ctx context.Context, op protocol.EditOp) (protocol.EditResult, error) {
	f.Edits = append(f.Edits, op)

	original, ok := f.Files[op.Path]

	if !ok {
		return protocol.EditResult{Path: op.Path}, fmt.Errorf("file not found: %s", op.Path)
	}

	result, err := edit.ApplyExact(original, op.Edits)
	if err != nil {
		return protocol.EditResult{Path: op.Path}, err
	}
	event := protocol.Event{
		Type: "edit/applied",
		Data: result,
	}
	f.Publish(ctx, event)

	f.Files[op.Path] = result

	return protocol.EditResult{Path: op.Path, Changed: true}, nil
}

func (f *FakeSurface) Publish(ctx context.Context, event protocol.Event) error {
	f.Events = append(f.Events, event)
	return nil
}

func (f *FakeSurface) HasPublishedEvent(eventType string) bool {
	for _, event := range f.Events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}

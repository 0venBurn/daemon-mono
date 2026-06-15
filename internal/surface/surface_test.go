package surface

import (
	"context"
	"testing"

	"github.com/0venburn/daemon/internal/protocol"
)

// EXERCISE 1: Make this test pass by completing FakeSurface.
func TestEditThroughFakeSurface(t *testing.T) {
	surf := NewFakeSurface(map[string]string{
		"main.go": "func hello() {\n    fmt.Println(\"hi\")\n}\n",
	})

	op := protocol.EditOp{
		Path: "main.go",
		Edits: []protocol.EditReplacement{
			{OldText: `"hi"`, NewText: `"hello, world"`},
		},
	}

	result, err := surf.Edit(context.Background(), op)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed {
		t.Fatal("expected change")
	}

	want := "func hello() {\n    fmt.Println(\"hello, world\")\n}\n"
	got := surf.Files["main.go"]
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}

	if len(surf.Edits) != 1 {
		t.Fatalf("expected 1 recorded edit, got %d", len(surf.Edits))
	}
}

func TestEditAddsEvent(t *testing.T) {
	surf := NewFakeSurface(map[string]string{
		"main.go": "func hello() {\n    fmt.Println(\"hi\")\n}\n",
	})

	op := protocol.EditOp{
		Path: "main.go",
		Edits: []protocol.EditReplacement{
			{OldText: `"hi"`, NewText: `"hello, world"`},
		},
	}

	result, err := surf.Edit(context.Background(), op)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed {
		t.Fatal("expected change")
	}
}

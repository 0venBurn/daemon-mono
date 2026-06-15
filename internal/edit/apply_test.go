package edit

import (
	"testing"

	"github.com/0venburn/daemon/internal/protocol"
)

func TestApplyExactReplacesUniqueText(t *testing.T) {
	got, err := ApplyExact("hello world", []protocol.EditReplacement{
		{OldText: "hello", NewText: "goodbye"},
	})
	if err != nil {
		t.Fatal(err)
	}

	want := "goodbye world"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

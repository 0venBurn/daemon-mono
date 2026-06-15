package main

import "testing"

func TestMarkdownFenceSanitizerStripsOpeningAndClosingFence(t *testing.T) {
	s := NewMarkdownFenceSanitizer()
	got := s.Add("```ja")
	got += s.Add("va\npublic int x() {\n")
	got += s.Add("  return 1;\n```\n")
	got += s.Flush()

	want := "public int x() {\n  return 1;\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestMarkdownFenceSanitizerKeepsPlainIndentedCode(t *testing.T) {
	s := NewMarkdownFenceSanitizer()
	got := s.Add("    public int x() {\n")
	got += s.Add("      return 1;\n")
	got += s.Flush()

	want := "    public int x() {\n      return 1;\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestMarkdownFenceSanitizerStripsBareFence(t *testing.T) {
	s := NewMarkdownFenceSanitizer()
	got := s.Add("```")
	got += s.Add("\nhello\n")
	got += s.Add("```")
	got += s.Flush()

	want := "hello\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestSplitForVisibleStreaming(t *testing.T) {
	got := splitForVisibleStreaming("abcdef\nghi", 3)
	want := []string{"abc", "def", "\n", "ghi"}
	if len(got) != len(want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %#v want %#v", got, want)
		}
	}
}

func TestShouldUseFastPatchForCurrentBufferEdit(t *testing.T) {
	p := StartParams{File: "/tmp/a.py", Content: "def f():\n    pass\n", Prompt: "implement the funcs in this file"}
	if !shouldUseFastPatch(p) {
		t.Fatal("expected current-buffer edit to use fast patch")
	}
}

func TestShouldUseFastPatchSkipsProjectSearch(t *testing.T) {
	p := StartParams{File: "/tmp/a.py", Content: "def f():\n    pass\n", Prompt: "search the repo and update callers"}
	if shouldUseFastPatch(p) {
		t.Fatal("expected project/search prompt to use tool agent")
	}
}

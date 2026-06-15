package fff

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFinderSmoke(t *testing.T) {
	SetLibraryPath()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// Walk up to repo root
	repoRoot := filepath.Join(cwd, "..", "..")
	abs, err := filepath.Abs(repoRoot)
	if err != nil {
		t.Fatal(err)
	}

	f, err := NewFinder(abs)
	if err != nil {
		t.Fatalf("NewFinder: %v", err)
	}
	defer f.Destroy()

	ok := f.WaitForScan(5000)
	if !ok {
		t.Log("scan did not complete within 5s, results may be partial")
	}

	// Test file search
	sr, err := f.SearchFiles("main.go")
	if err != nil {
		t.Fatalf("SearchFiles: %v", err)
	}
	if len(sr.Items) == 0 {
		t.Error("SearchFiles returned no results for main.go")
	}
	t.Logf("SearchFiles 'main.go': %d results (total matched: %d)", len(sr.Items), sr.TotalMatched)
	for i, f := range sr.Items {
		if i >= 5 {
			break
		}
		t.Logf("  %s (%s) frecency=%d", f.RelativePath, f.FileName, 0)
	}

	// Test grep
	gr, err := f.Grep("func main", WithGrepContext(1, 1))
	if err != nil {
		t.Fatalf("Grep: %v", err)
	}
	if len(gr.Items) == 0 {
		t.Error("Grep returned no results for 'func main'")
	}
	t.Logf("Grep 'func main': %d matches in %d files", gr.TotalMatched, gr.TotalFilesSearched)
	for i, m := range gr.Items {
		if i >= 3 {
			break
		}
		t.Logf("  %s:%d %s", m.RelativePath, m.LineNumber, m.LineContent)
	}
}
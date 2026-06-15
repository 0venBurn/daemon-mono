// Package fff provides Go bindings for the FFF (Fast File Finder) C library.
// It enables fuzzy file search, glob search, and content grep — all frecency-ranked
// and session-persistent, replacing slow per-call glob/grep CLI spawns.
package fff

/*
#cgo CFLAGS: -I${SRCDIR}
#cgo LDFLAGS: -L${SRCDIR}/../../lib -lfff_c -lm -ldl -lpthread -Wl,-rpath,${SRCDIR}/../../lib
#include <fff.h>
#include <stdlib.h>
*/
import "C"
import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"unsafe"
)

// Finder holds a long-lived FFF instance scoped to a project directory.
type Finder struct {
	handle unsafe.Pointer
	mu     sync.Mutex
	base   string
	closed bool
}

// NewFinder creates an FFF instance indexing baseDir.
// Enables AI mode, background watcher, and content indexing.
// Call Destroy when done.
func NewFinder(baseDir string) (*Finder, error) {
	abs, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, fmt.Errorf("fff: resolve path: %w", err)
	}

	cBase := C.CString(abs)
	defer C.free(unsafe.Pointer(cBase))

	opts := C.FffCreateOptions{
		version:                  C.FFF_CREATE_OPTIONS_VERSION,
		base_path:                cBase,
		frecency_db_path:         nil,
		history_db_path:          nil,
		enable_mmap_cache:        true,
		enable_content_indexing:  true,
		watch:                    true,
		ai_mode:                  true,
		enable_home_dir_scanning: false,
		enable_fs_root_scanning:  false,
	}

	res := C.fff_create_instance_with(&opts)
	if res == nil {
		return nil, errors.New("fff: create instance returned nil")
	}
	defer C.fff_free_result(res)

	if !res.success {
		var errMsg string
		if res.error != nil {
			errMsg = C.GoString(res.error)
		}
		return nil, fmt.Errorf("fff: create instance: %s", errMsg)
	}

	return &Finder{
		handle: res.handle,
		base:   abs,
	}, nil
}

// Destroy releases the FFF instance and all its resources.
func (f *Finder) Destroy() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.closed {
		C.fff_destroy(f.handle)
		f.closed = true
	}
}

// WaitForScan waits up to timeoutMs for the initial file scan to complete.
func (f *Finder) WaitForScan(timeoutMs uint64) bool {
	f.mu.Lock()
	handle := f.handle
	f.mu.Unlock()
	if handle == nil {
		return false
	}
	res := C.fff_wait_for_scan(handle, C.uint64_t(timeoutMs))
	if res == nil {
		return false
	}
	defer C.fff_free_result(res)
	return res.int_value == 1
}

// --- FileItem ---

type FileItem struct {
	RelativePath string
	FileName     string
	GitStatus    string
	Size         uint64
	Modified     uint64
	IsBinary     bool
}

func goFileItem(item *C.FffFileItem) FileItem {
	fi := FileItem{
		Size:     uint64(item.size),
		Modified: uint64(item.modified),
		IsBinary: bool(item.is_binary),
	}
	if item.relative_path != nil {
		fi.RelativePath = C.GoString(item.relative_path)
	}
	if item.file_name != nil {
		fi.FileName = C.GoString(item.file_name)
	}
	if item.git_status != nil {
		fi.GitStatus = C.GoString(item.git_status)
	}
	return fi
}

// --- SearchResult ---

type SearchResult struct {
	Items        []FileItem
	TotalMatched uint32
	TotalFiles   uint32
}

func goSearchResult(sr *C.FffSearchResult) SearchResult {
	r := SearchResult{
		TotalMatched: uint32(sr.total_matched),
		TotalFiles:   uint32(sr.total_files),
	}
	count := int(uint32(sr.count))
	if count > 0 && sr.items != nil {
		r.Items = make([]FileItem, count)
		for i := 0; i < count; i++ {
			item := C.fff_search_result_get_item(sr, C.uint32_t(i))
			if item != nil {
				r.Items[i] = goFileItem(item)
			}
		}
	}
	return r
}

// --- GrepMatch ---

type GrepMatch struct {
	RelativePath     string
	FileName         string
	GitStatus        string
	LineContent      string
	LineNumber       uint64
	Col              uint32
	IsDefinition     bool
	IsBinary         bool
	TotalFrecencyScore int64
}

func goGrepMatch(m *C.FffGrepMatch) GrepMatch {
	gm := GrepMatch{
		LineNumber:       uint64(m.line_number),
		Col:              uint32(m.col),
		IsDefinition:     bool(m.is_definition),
		IsBinary:         bool(m.is_binary),
		TotalFrecencyScore: int64(m.total_frecency_score),
	}
	if m.relative_path != nil {
		gm.RelativePath = C.GoString(m.relative_path)
	}
	if m.file_name != nil {
		gm.FileName = C.GoString(m.file_name)
	}
	if m.git_status != nil {
		gm.GitStatus = C.GoString(m.git_status)
	}
	if m.line_content != nil {
		gm.LineContent = C.GoString(m.line_content)
	}
	// context lines
	beforeCount := uint32(m.context_before_count)
	if beforeCount > 0 && m.context_before != nil {
		lines := make([]string, beforeCount)
		for i := uint32(0); i < beforeCount; i++ {
			line := C.fff_grep_match_get_context_before(m, C.uint32_t(i))
			if line != nil {
				lines[i] = C.GoString(line)
			}
		}
		// Store first context-before line for snippeting if needed
		if len(lines) > 0 {
			gm.LineContent = lines[0] + "\n" + gm.LineContent
		}
	}
	afterCount := uint32(m.context_after_count)
	if afterCount > 0 && m.context_after != nil {
		var after []string
		for i := uint32(0); i < afterCount; i++ {
			line := C.fff_grep_match_get_context_after(m, C.uint32_t(i))
			if line != nil {
				after = append(after, C.GoString(line))
			}
		}
		if len(after) > 0 {
			gm.LineContent += "\n" + joinStrings(after, "\n")
		}
	}
	return gm
}

func joinStrings(ss []string, sep string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}

// --- GrepResult ---

type GrepResult struct {
	Items            []GrepMatch
	TotalMatched     uint32
	TotalFilesSearched uint32
	TotalFiles       uint32
	FilteredFileCount uint32
}

func goGrepResult(gr *C.FffGrepResult) GrepResult {
	r := GrepResult{
		TotalMatched:       uint32(gr.total_matched),
		TotalFilesSearched: uint32(gr.total_files_searched),
		TotalFiles:         uint32(gr.total_files),
		FilteredFileCount:  uint32(gr.filtered_file_count),
	}
	count := int(uint32(gr.count))
	if count > 0 && gr.items != nil {
		r.Items = make([]GrepMatch, count)
		for i := 0; i < count; i++ {
			m := C.fff_grep_result_get_match(gr, C.uint32_t(i))
			if m != nil {
				r.Items[i] = goGrepMatch(m)
			}
		}
	}
	return r
}

// --- Search methods ---

// SearchFiles performs fuzzy file search. query can be a partial filename,
// path fragment, or constraint expression like "*.rs !test/".
func (f *Finder) SearchFiles(query string, opts ...SearchOpt) (*SearchResult, error) {
	f.mu.Lock()
	handle := f.handle
	f.mu.Unlock()
	if handle == nil {
		return nil, errors.New("fff: finder destroyed")
	}

	cfg := applySearchOpts(opts...)
	cQuery := C.CString(query)
	cCurrent := C.CString(cfg.CurrentFile)
	defer C.free(unsafe.Pointer(cQuery))
	defer C.free(unsafe.Pointer(cCurrent))

	res := C.fff_search(handle, cQuery, cCurrent, C.uint32_t(cfg.MaxThreads), C.uint32_t(cfg.Page), C.uint32_t(cfg.PageSize), C.int32_t(cfg.ComboBoost), C.uint32_t(cfg.MinComboCount))
	if res == nil {
		return nil, errors.New("fff: search returned nil")
	}
	defer C.fff_free_result(res)
	if !res.success {
		var errMsg string
		if res.error != nil {
			errMsg = C.GoString(res.error)
		}
		return nil, fmt.Errorf("fff: search: %s", errMsg)
	}
	sr := (*C.FffSearchResult)(res.handle)
	if sr == nil {
		return &SearchResult{}, nil
	}
	result := goSearchResult(sr)
	return &result, nil
}

// Glob performs literal glob search, frecency-ranked.
func (f *Finder) Glob(pattern string, opts ...SearchOpt) (*SearchResult, error) {
	f.mu.Lock()
	handle := f.handle
	f.mu.Unlock()
	if handle == nil {
		return nil, errors.New("fff: finder destroyed")
	}

	cfg := applySearchOpts(opts...)
	cPattern := C.CString(pattern)
	cCurrent := C.CString(cfg.CurrentFile)
	defer C.free(unsafe.Pointer(cPattern))
	defer C.free(unsafe.Pointer(cCurrent))

	res := C.fff_glob(handle, cPattern, cCurrent, C.uint32_t(cfg.MaxThreads), C.uint32_t(cfg.Page), C.uint32_t(cfg.PageSize))
	if res == nil {
		return nil, errors.New("fff: glob returned nil")
	}
	defer C.fff_free_result(res)
	if !res.success {
		var errMsg string
		if res.error != nil {
			errMsg = C.GoString(res.error)
		}
		return nil, fmt.Errorf("fff: glob: %s", errMsg)
	}
	sr := (*C.FffSearchResult)(res.handle)
	if sr == nil {
		return &SearchResult{}, nil
	}
	result := goSearchResult(sr)
	return &result, nil
}

// Grep performs content search across indexed files.
func (f *Finder) Grep(query string, opts ...GrepOpt) (*GrepResult, error) {
	f.mu.Lock()
	handle := f.handle
	f.mu.Unlock()
	if handle == nil {
		return nil, errors.New("fff: finder destroyed")
	}

	cfg := applyGrepOpts(opts...)
	cQuery := C.CString(query)
	defer C.free(unsafe.Pointer(cQuery))

	res := C.fff_live_grep(handle,
		cQuery,
		C.uint8_t(cfg.Mode),
		C.uint64_t(cfg.MaxFileSize),
		C.uint32_t(cfg.MaxMatchesPerFile),
		C.bool(cfg.SmartCase),
		C.uint32_t(cfg.FileOffset),
		C.uint32_t(cfg.PageLimit),
		C.uint64_t(cfg.TimeBudgetMs),
		C.uint32_t(cfg.BeforeContext),
		C.uint32_t(cfg.AfterContext),
		C.bool(cfg.ClassifyDefinitions),
	)
	if res == nil {
		return nil, errors.New("fff: grep returned nil")
	}
	defer C.fff_free_result(res)
	if !res.success {
		var errMsg string
		if res.error != nil {
			errMsg = C.GoString(res.error)
		}
		return nil, fmt.Errorf("fff: grep: %s", errMsg)
	}
	gr := (*C.FffGrepResult)(res.handle)
	if gr == nil {
		return &GrepResult{}, nil
	}
	result := goGrepResult(gr)
	return &result, nil
}

// --- Option types ---

type SearchOpt struct {
	apply func(*searchConfig)
}

type searchConfig struct {
	CurrentFile    string
	MaxThreads     uint32
	Page           uint32
	PageSize       uint32
	ComboBoost     int32
	MinComboCount  uint32
}

func applySearchOpts(opts ...SearchOpt) searchConfig {
	cfg := searchConfig{
		MaxThreads:    0, // auto
		PageSize:     30,
		ComboBoost:   0, // default
		MinComboCount: 0,
	}
	for _, o := range opts {
		o.apply(&cfg)
	}
	return cfg
}

func WithCurrentFile(path string) SearchOpt {
	return SearchOpt{apply: func(c *searchConfig) { c.CurrentFile = path }}
}

func WithPageSize(n uint32) SearchOpt {
	return SearchOpt{apply: func(c *searchConfig) { c.PageSize = n }}
}

// --- Grep option types ---

type GrepMode uint8

const (
	GrepPlain  GrepMode = 0
	GrepRegex  GrepMode = 1
	GrepFuzzy  GrepMode = 2
)

type GrepOpt struct {
	apply func(*grepConfig)
}

type grepConfig struct {
	Mode                 GrepMode
	MaxFileSize          uint64
	MaxMatchesPerFile    uint32
	SmartCase            bool
	FileOffset           uint32
	PageLimit            uint32
	TimeBudgetMs         uint64
	BeforeContext        uint32
	AfterContext         uint32
	ClassifyDefinitions  bool
}

func applyGrepOpts(opts ...GrepOpt) grepConfig {
	cfg := grepConfig{
		Mode:                GrepPlain,
		MaxFileSize:         10 * 1024 * 1024,
		MaxMatchesPerFile:   100,
		SmartCase:           true,
		PageLimit:           30,
		TimeBudgetMs:        500,
		ClassifyDefinitions: true,
	}
	for _, o := range opts {
		o.apply(&cfg)
	}
	return cfg
}

func WithGrepMode(m GrepMode) GrepOpt {
	return GrepOpt{apply: func(c *grepConfig) { c.Mode = m }}
}

func WithGrepContext(before, after uint32) GrepOpt {
	return GrepOpt{apply: func(c *grepConfig) { c.BeforeContext = before; c.AfterContext = after }}
}

func WithGrepPageLimit(n uint32) GrepOpt {
	return GrepOpt{apply: func(c *grepConfig) { c.PageLimit = n }}
}

// FormatSearchResult formats file search results for LLM consumption.
func FormatSearchResult(r *SearchResult) string {
	if r == nil || len(r.Items) == 0 {
		return "No files found."
	}
	var s string
	for _, f := range r.Items {
		s += f.RelativePath + "\n"
	}
	header := fmt.Sprintf("%d files found", r.TotalMatched)
	if r.TotalMatched > uint32(len(r.Items)) {
		header += fmt.Sprintf(" (showing %d)", len(r.Items))
	}
	return header + ":\n" + s
}

// FormatGrepResult formats grep results for LLM consumption.
func FormatGrepResult(r *GrepResult) string {
	if r == nil || len(r.Items) == 0 {
		return "No matches found."
	}
	var s string
	for _, m := range r.Items {
		def := ""
		if m.IsDefinition {
			def = " [def]"
		}
		s += fmt.Sprintf("%s:%d:%d%s %s\n", m.RelativePath, m.LineNumber, m.Col, def, m.LineContent)
	}
	header := fmt.Sprintf("%d matches in %d files", r.TotalMatched, r.TotalFilesSearched)
	return header + ":\n" + s
}

// SetLibraryPath sets LD_LIBRARY_PATH so the dynamic linker finds libfff_c.so.
// Must be called before any FFF function. Safe to call multiple times.
func SetLibraryPath() {
	// Try relative to executable
	if exe, err := os.Executable(); err == nil {
		libDir := filepath.Join(filepath.Dir(exe), "..", "lib")
		if fi, err := os.Stat(libDir); err == nil && fi.IsDir() {
			_ = os.Setenv("LD_LIBRARY_PATH", libDir+":"+os.Getenv("LD_LIBRARY_PATH"))
			return
		}
	}
	// Try development path relative to this source file
	_, thisFile, _, ok := runtime.Caller(0)
	if ok {
		libDir := filepath.Join(filepath.Dir(thisFile), "..", "..", "lib")
		if fi, err := os.Stat(libDir); err == nil && fi.IsDir() {
			_ = os.Setenv("LD_LIBRARY_PATH", libDir+":"+os.Getenv("LD_LIBRARY_PATH"))
		}
	}
}
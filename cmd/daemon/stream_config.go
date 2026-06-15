package main

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type MarkdownFenceSanitizer struct {
	prefixDone bool
	prefixBuf  string
	lineBuf    string
}

func NewMarkdownFenceSanitizer() *MarkdownFenceSanitizer {
	return &MarkdownFenceSanitizer{}
}

func (s *MarkdownFenceSanitizer) Add(chunk string) string {
	if chunk == "" {
		return ""
	}

	text := chunk
	if !s.prefixDone {
		s.prefixBuf += chunk
		var ready bool
		text, ready = s.consumePrefix()
		if !ready {
			return ""
		}
	}

	return s.processCompleteLines(text)
}

func (s *MarkdownFenceSanitizer) Flush() string {
	if !s.prefixDone {
		text, _ := s.consumePrefixForce()
		return s.processCompleteLines(text) + s.flushLine()
	}
	return s.flushLine()
}

func (s *MarkdownFenceSanitizer) consumePrefix() (string, bool) {
	buf := s.prefixBuf
	leading := len(buf) - len(strings.TrimLeft(buf, "\ufeff\r\n\t "))
	trimmed := buf[leading:]

	if trimmed == "" || strings.HasPrefix("```", trimmed) {
		return "", false
	}

	if strings.HasPrefix(trimmed, "```") {
		newline := strings.IndexByte(trimmed, '\n')
		if newline < 0 {
			return "", false
		}
		s.prefixDone = true
		s.prefixBuf = ""
		return trimmed[newline+1:], true
	}

	s.prefixDone = true
	s.prefixBuf = ""
	return buf, true
}

func (s *MarkdownFenceSanitizer) consumePrefixForce() (string, bool) {
	buf := s.prefixBuf
	s.prefixDone = true
	s.prefixBuf = ""

	leading := len(buf) - len(strings.TrimLeft(buf, "\ufeff\r\n\t "))
	trimmed := buf[leading:]
	if strings.HasPrefix(trimmed, "```") {
		newline := strings.IndexByte(trimmed, '\n')
		if newline < 0 {
			return "", true
		}
		return trimmed[newline+1:], true
	}
	return buf, true
}

func (s *MarkdownFenceSanitizer) processCompleteLines(text string) string {
	if text == "" {
		return ""
	}
	s.lineBuf += text

	var out strings.Builder
	for {
		newline := strings.IndexByte(s.lineBuf, '\n')
		if newline < 0 {
			break
		}
		line := s.lineBuf[:newline+1]
		s.lineBuf = s.lineBuf[newline+1:]
		if isMarkdownFenceLine(line) {
			continue
		}
		out.WriteString(line)
	}
	return out.String()
}

func (s *MarkdownFenceSanitizer) flushLine() string {
	line := s.lineBuf
	s.lineBuf = ""
	if isMarkdownFenceLine(line) {
		return ""
	}
	return line
}

func isMarkdownFenceLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "```")
}

func patchValidationMode() string {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("DAEMON_PATCH_VALIDATION")))
	if mode == "off" || mode == "false" || mode == "0" || mode == "yolo" {
		return "off"
	}
	return "strict"
}

func streamDelay() time.Duration {
	raw := strings.TrimSpace(os.Getenv("DAEMON_STREAM_DELAY_MS"))
	if raw == "" {
		return 0
	}
	ms, err := strconv.Atoi(raw)
	if err != nil || ms <= 0 {
		return 0
	}
	if ms > 1000 {
		ms = 1000
	}
	return time.Duration(ms) * time.Millisecond
}

func streamCharsPerChunk() int {
	raw := strings.TrimSpace(os.Getenv("DAEMON_STREAM_CHARS"))
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0
	}
	if n > 200 {
		n = 200
	}
	return n
}

func splitForVisibleStreaming(text string, charsPerChunk int) []string {
	if charsPerChunk <= 0 {
		return []string{text}
	}

	var parts []string
	var b strings.Builder
	count := 0
	for _, r := range text {
		b.WriteRune(r)
		count++
		if r == '\n' || count >= charsPerChunk {
			parts = append(parts, b.String())
			b.Reset()
			count = 0
		}
	}
	if b.Len() > 0 {
		parts = append(parts, b.String())
	}
	return parts
}

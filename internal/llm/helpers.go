package llm

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
)

// sseEvent represents a single Server-Sent Event.
type sseEvent struct {
	Type string
	Data []byte
}

// sseScanner reads SSE events from an io.ReadCloser using a bufio.Scanner.
type sseScanner struct {
	sc     *bufio.Scanner
	evt    sseEvent
	errVal error
	inited bool
}

func newSSEScanner(r io.Reader) *sseScanner {
	sc := bufio.NewScanner(r)
	return &sseScanner{sc: sc}
}

func (s *sseScanner) next() bool {
	if s.errVal != nil || s.sc == nil {
		return false
	}
	if !s.inited {
		s.sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		s.inited = true
	}

	eventType := ""
	dataBuf := &bytes.Buffer{}

	for s.sc.Scan() {
		line := s.sc.Text()

		// An empty line signals the end of an event
		if line == "" {
			if dataBuf.Len() > 0 || eventType != "" {
				s.evt = sseEvent{
					Type: eventType,
					Data: dataBuf.Bytes(),
				}
				return true
			}
			continue
		}

		if strings.HasPrefix(line, ":") {
			// comment, ignore
			continue
		}

		field, value, ok := strings.Cut(line, ":")
		if !ok {
			field = line
			value = ""
		}
		// Remove a single leading space from value per SSE spec
		if len(value) > 0 && value[0] == ' ' {
			value = value[1:]
		}

		switch field {
		case "event":
			eventType = value
		case "data":
			dataBuf.WriteString(value)
			dataBuf.WriteByte('\n')
		}
	}

	if s.sc.Err() != nil {
		s.errVal = s.sc.Err()
		return false
	}

	// EOF — emit final event if we have accumulated data
	if dataBuf.Len() > 0 || eventType != "" {
		s.evt = sseEvent{
			Type: eventType,
			Data: dataBuf.Bytes(),
		}
		return true
	}

	return false
}

func (s *sseScanner) event() sseEvent { return s.evt }
func (s *sseScanner) err() error      { return s.errVal }

func applyOpenCodeHeaders(req *http.Request, ctx context.Context, providerName, baseURL string) {
	if !isOpenCodeRequest(providerName, baseURL) {
		return
	}
	if sessionID := sessionIDFromContext(ctx); sessionID != "" {
		req.Header.Set("x-opencode-session", sessionID)
	}
	// Match Pi's OpenCode attribution/routing header.
	req.Header.Set("x-opencode-client", "pi")
}

func isOpenCodeRequest(providerName, baseURL string) bool {
	if providerName == "opencode" || providerName == "opencode-go" {
		return true
	}
	return strings.Contains(strings.TrimSpace(baseURL), "opencode.ai/zen")
}

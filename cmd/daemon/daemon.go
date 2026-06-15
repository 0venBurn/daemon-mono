package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/0venburn/daemon/internal/fff"
	"github.com/0venburn/daemon/internal/llm"
)

func newDaemon(out io.Writer) (*Daemon, error) {
	loadDotEnv()

	cfg := llm.ResolveProviderConfig(llm.LoadFromEnv())
	provider, err := llm.NewProvider(cfg)
	if err != nil {
		return nil, fmt.Errorf("provider init: %w", err)
	}

	fmt.Fprintf(os.Stderr, "daemon: provider=%s model=%s base_url=%s\n", cfg.Name, cfg.Model, cfg.BaseURL)

	var finder *fff.Finder
	if os.Getenv("DAEMON_ENABLE_FFF") == "1" || strings.EqualFold(os.Getenv("DAEMON_ENABLE_FFF"), "true") {
		fff.SetLibraryPath()
		cwd, _ := os.Getwd()
		var err error
		finder, err = fff.NewFinder(cwd)
		if err != nil {
			fmt.Fprintf(os.Stderr, "daemon: warning: FFF init failed (%v), search tools will be limited\n", err)
		} else {
			finder.WaitForScan(5000)
		}
	} else {
		fmt.Fprintln(os.Stderr, "daemon: FFF disabled (set DAEMON_ENABLE_FFF=1 to enable experimental search)")
	}

	return &Daemon{
		provider: provider,
		config:   cfg,
		out:      json.NewEncoder(out),
		sessions: map[string]context.CancelFunc{},
		finder:   finder,
	}, nil
}

func (d *Daemon) setThinking(level string) {
	parsed := llm.ParseThinkingLevel(level)
	if parsed == "" && level != "" && level != "off" {
		d.sendDaemonError("invalid thinking level: " + level + ". Use: off, low, medium, high, xhigh")
		return
	}
	if parsed == "" && level == "off" {
		parsed = llm.ThinkingOff
	}
	d.config.Thinking = parsed

	// Recreate provider with new thinking level
	d.provMu.Lock()
	old := d.provider
	d.provider, _ = llm.NewProvider(d.config)
	if old != nil {
		old.Close()
	}
	d.provMu.Unlock()

	d.writeDebug("thinking/set", map[string]any{"level": string(parsed)})
	d.sendDaemonInfo()
}

func (d *Daemon) cancelSession(sessionID, reason string) {
	d.sessMu.Lock()
	cancel := d.sessions[sessionID]
	if sessionID == "" {
		for _, c := range d.sessions {
			c()
		}
		d.sessions = map[string]context.CancelFunc{}
	} else if cancel != nil {
		cancel()
		delete(d.sessions, sessionID)
	}
	d.sessMu.Unlock()
	d.sendSessionStatus(sessionID, "cancelled", reason)
}

func (d *Daemon) send(method string, params any) {
	d.writeEventDebug(method, params)
	d.outMu.Lock()
	defer d.outMu.Unlock()
	if err := d.out.Encode(Outgoing{Method: method, Params: params}); err != nil {
		fmt.Fprintf(os.Stderr, "encode: %v\n", err)
	}
}

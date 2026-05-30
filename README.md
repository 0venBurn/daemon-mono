# daemon

Small, hackable coding-agent harness.

Daemon is not meant to be one more chat UI. Goal: one shared Go core for model loops, tools, context, sessions, profiles, validation, events, and extensions; multiple replaceable surfaces over it.

## Current state

Working proof-of-concept:

- `cmd/steer-daemon/` — Go JSONL daemon for Neovim steer mode.
- `~/.config/nvim/plugin/steer.lua` and `~/.config/nvim/lua/steer/init.lua` — local Neovim plugin surface.
- Anthropic Haiku streaming.
- Cursor/no-selection mode streams raw insertion text at cursor.
- Visual-selection mode asks for surgical JSON `old`/`new` patches.
- Patch engine validates exact, unique, small edits and retries invalid broad patches up to 3 times.
- Lua applies edits through Neovim buffer APIs, so edits are visible, undoable, cancellable, and editor-native.
- Ghost cursor tracks the agent insertion point.
- `:SteerExplain` explains selected code in a popup.

Recent architecture pass:

- Extracted patch parsing, normalization, preparation, and validation into `cmd/steer-daemon/patch.go`.
- Moved patch tests into `cmd/steer-daemon/patch_test.go`.
- `cmd/steer-daemon/main.go` now calls the patch engine instead of owning patch policy.

## Build

```sh
go build -o ~/.local/bin/steer-daemon ./cmd/steer-daemon
```

The local Neovim plugin expects:

```text
~/.local/bin/steer-daemon
```

## Neovim usage

```vim
<leader>ai        start edit at cursor / visual selection
<leader>ax        explain selected code in popup
<leader>aq        cancel
<Esc>             cancel active stream
:SteerEdit ...    start with inline prompt
:SteerPrompt ...  steer/cancel active prototype session
:SteerExplain     explain selected code (select code first)
:SteerStatus      status
:SteerThrottle N [chars]  add N ms delay per streamed chunk, optionally split chunks to chars
```

## Architecture direction

Keep one core, many surfaces.

```text
Daemon core
├── agent session loop
├── model provider abstraction
├── tool registry
├── context builder
├── validation and edit engines
├── event log
├── profiles and skills
└── extension manager

Surfaces
├── Neovim steer surface
├── TUI harness surface
├── headless JSONL surface
└── tmux/zellij/instance adapters
```

Important rule:

> The core decides what should happen. The surface decides how it is presented and, in editor mode, how edits are applied.

For Neovim, Go should not directly write active editor buffers. Go emits edit operations; Lua applies them with Neovim APIs.

## Near-term plan

1. Keep the current Neovim prototype working.
2. Keep extracting deep modules from `cmd/steer-daemon/main.go`.
3. Add a provider interface so Anthropic SDK types stay at the edge.
4. Add a typed event/surface seam instead of `send(method string, params any)` everywhere.
5. Extract session lifecycle and cancellation into a session module.
6. Load env/config once and pass plain runtime config inward.
7. Grow toward shared core packages under `internal/`.

Likely package shape:

```text
cmd/
  daemon/              # future TUI entrypoint
  daemon-agent/        # future headless/jsonl entrypoint
  steer-daemon/        # current Neovim prototype daemon
internal/
  agent/
  llm/
  tool/
  context/
  edit/
  validation/
  protocol/
  surface/
  session/
  profile/
  skill/
  extension/
nvim/
  lua/daemon/          # future repo-owned Neovim surface
```

## Later

- TUI harness with plans, tools, diffs, approvals, context toggles, sessions, and extension management.
- Headless JSONL/RPC surface for tests and automation.
- Event log and resumable sessions.
- Capability-filtered tools from profile + project config + surface capabilities + approval policy.
- Starlark and external JSONL extensions.
- Optional tmux/zellij/instance orchestration.

# daemon

Small, hackable coding-agent harness.

> **Status:** Work in progress. This is currently a personal project that I use myself. You are welcome to clone it and use it too, but expect rough edges, breaking changes, and a likely complete rewrite once the base feature set is proven.

Licensed under the MIT License. See [LICENSE](LICENSE).

Daemon is not meant to be one more chat UI. Goal: one shared Go core for model loops, tools, context, sessions, profiles, validation, events, and extensions; multiple replaceable surfaces over it.

## Current state

Working proof-of-concept:

- `cmd/daemon/` — Go JSONL daemon used by the Neovim surface.
- `nvim/` — repo-owned Neovim plugin surface, loaded from local config.
- Multi-provider LLM abstraction (Anthropic, OpenAI, Google, OpenCode Zen, OpenCode Go).
- Cursor/no-selection mode streams raw insertion text at cursor.
- Visual-selection mode asks for surgical JSON `old`/`new` patches.
- Patch engine validates exact, unique, small edits and retries invalid broad patches up to 3 times.
- Lua applies edits through Neovim buffer APIs, so edits are visible, undoable, cancellable, and editor-native.
- Ghost cursor tracks the agent insertion point.
- `:DaemonExplain` explains selected code in a popup.

Recent architecture pass:

- Extracted patch parsing, normalization, preparation, and validation into `cmd/daemon/patch.go`.
- Moved patch tests into `cmd/daemon/patch_test.go`.
- `cmd/daemon/main.go` now calls the patch engine instead of owning patch policy.

## Installation

Clone the repo, build the daemon binary, then load the Neovim plugin from the checkout.

Prerequisites:

- Go 1.24+
- Neovim 0.9+
- An API key for one supported provider
- Optional: `lib/libfff_c.so` for FFF-backed file search

Build and install the daemon binary:

```sh
make build
# equivalent: go build -o ~/.local/bin/daemon ./cmd/daemon
```

The Neovim plugin currently expects the daemon at:

```text
~/.local/bin/daemon
```

`~/.local/bin` must be on your `PATH` if you also want to run `daemon` directly from a shell.

Load the repo-owned plugin from your checkout while it is a prototype:

```lua
vim.opt.runtimepath:prepend("/path/to/daemon/nvim")
require("daemon").setup()
```

Run tests:

```sh
make test
# equivalent: go test ./...
```

## Release and install model

Current prototype install target:

```text
~/.local/bin/daemon
```

The Neovim plugin launches that path by default. Until packaged releases exist, build from a checkout and load `nvim/` through your Neovim runtime path.

Planned release shape:

- Versioned Go daemon binaries attached to GitHub releases.
- Plugin manager setup that points Neovim at this repository.
- Optional override for the daemon binary path in `require("daemon").setup({ ... })`.
- Semantic versions once the JSONL protocol and auth file schema stabilize.

## Providers and auth

Daemon supports multiple LLM providers, selected by `DAEMON_PROVIDER` or `~/.config/daemon/auth.json`.

### Quick start: `:DaemonAuth`

```vim
:DaemonAuth          Select and authenticate a provider interactively
:DaemonAuthStatus    Show current auth configuration
:DaemonAuthSwitch    Switch active provider (must already be configured)
:DaemonAuthLogout    Remove a provider's credentials
```

`:DaemonAuth` presents a picker:

``
► ✓ anthropic        claude-sonnet-4-5  Anthropic API key
  ✗ openai           gpt-4o             OpenAI API key
  ✗ google           gemini-2.0-flash    Google API key
  ✗ opencode         claude-sonnet-4-5  OpenCode Zen proxy
  ✗ opencode-go      deepseek-v4-flash  OpenCode Go proxy
```

Select one → enter API key → optionally override model → done. Credentials are saved to `~/.config/daemon/auth.json` (mode 0600), and the daemon restarts with the new provider.

### Environment variables

| Variable | Provider | Description |
|---|---|---|
| `DAEMON_PROVIDER` | all | `anthropic`, `openai`, `google`, `opencode`, `opencode-go` |
| `ANTHROPIC_API_KEY` | anthropic | Direct Anthropic API key |
| `OPENAI_API_KEY` | openai | Direct OpenAI API key |
| `GOOGLE_API_KEY` | google | Direct Google API key |
| `OPENCODE_API_KEY` | opencode, opencode-go | OpenCode Zen/Go API key |
| `DAEMON_MODEL` | all | Override model name |
| `OPENCODE_BASE_URL` | opencode | Override base URL |
| `OPENCODE_GO_BASE_URL` | opencode-go | Override base URL |

Env vars take priority over auth.json. Auth file resolution: `~/.config/daemon/auth.json`.

### Auth file format

```json
{
  "_active_provider": "anthropic",
  "anthropic": { "api_key": "sk-ant-...", "model": "claude-sonnet-4-5" },
  "opencode": { "api_key": "zen-...", "model": "claude-sonnet-4-5" },
  "openai": { "api_key": "sk-..." }
}
```

Security notes:

- Auth files are written to `~/.config/daemon/auth.json` with mode `0600`.
- Environment variables take precedence over auth file values.
- Do not commit `.env`, auth files, or raw provider keys.

Legacy auth files using `{ "_active": { "api_key": "anthropic" } }` are still read, then rewritten to `_active_provider` when auth settings change.

### OpenCode Zen

[OpenCode Zen](https://opencode.ai) is a proxy that routes to multiple providers. The `opencode` provider automatically selects the right API (Anthropic Messages for `claude-*` models, OpenAI Completions for everything else) based on the model name.

| Provider | Default model | Base URL | API type |
|---|---|---|---|
| `opencode` | `claude-sonnet-4-5` | `https://opencode.ai/zen` | Auto (Anthropic/OpenAI) |
| `opencode-go` | `deepseek-v4-flash` | `https://opencode.ai/zen/go/v1` | Auto (Anthropic/OpenAI) |

## Neovim usage

```vim
<leader>ai        start edit at cursor / visual selection
<leader>ax        explain selected code in popup
<leader>aq        cancel
<Esc>             cancel active stream
:DaemonAuth       select and authenticate a provider
:DaemonAuthStatus show auth status
:DaemonAuthSwitch switch active provider
:DaemonAuthLogout remove provider credentials
:DaemonEdit ...    start with inline prompt
:DaemonPrompt ...  prompt/cancel active prototype session
:DaemonExplain     explain selected code (select code first); focuses transcript popup
:DaemonStatus      status
:DaemonThrottle N [chars]  add N ms delay per streamed chunk, optionally split chunks to chars

Explain popup keys:

q / <Esc>         close transcript
j / k             move line-by-line
<C-j> / J         scroll down
<C-k> / K         scroll up
```

## FFF/C dependency

`internal/fff` binds to `lib/libfff_c.so` through cgo. The library is currently shipped in `lib/` so a clean checkout can build and test without downloading extra artifacts.

Runtime notes:

- Keep `lib/libfff_c.so` in the repo `lib/` directory for local development.
- If installing elsewhere, ensure the dynamic linker can find `libfff_c.so` via rpath or `LD_LIBRARY_PATH`.
- FFF-backed search is intended to become optional behind a package boundary.

## Architecture overview

Shipped today:

```text
Neovim plugin ──JSONL──> cmd/daemon
                         ├── internal/llm providers
                         ├── patch/edit validation
                         ├── optional internal/fff search bindings
                         └── protocol messages back to Lua
```

The current daemon owns more orchestration than desired. `cmd/daemon` is being reduced toward a thin JSONL adapter while reusable code moves under `internal/`.

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
├── Neovim daemon surface
├── TUI harness surface
├── headless JSONL surface
└── tmux/zellij/instance adapters
```

Important rule:

> The core decides what should happen. The surface decides how it is presented and, in editor mode, how edits are applied.

For Neovim, Go should not directly write active editor buffers. Go emits edit operations; Lua applies them with Neovim APIs.

## Near-term plan

1. Keep the current Neovim prototype working.
2. Keep extracting deep modules from `cmd/daemon/main.go`.
3. ~~Add a provider interface so Anthropic SDK types stay at the edge.~~ Done: `internal/llm/` with providers for Anthropic, OpenAI, Google, OpenCode Zen, OpenCode Go.
4. Add a typed event/surface seam instead of `send(method string, params any)` everywhere.
5. Extract session lifecycle and cancellation into a session module.
6. Load env/config once and pass plain runtime config inward.
7. Grow toward shared core packages under `internal/`.

Likely package shape:

```text
cmd/
  daemon/        # current Neovim JSONL daemon
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
  lua/daemon/          # repo-owned Neovim surface
  plugin/daemon.lua    # plugin entrypoint
```

## Later

- TUI harness with plans, tools, diffs, approvals, context toggles, sessions, and extension management.
- Headless JSONL/RPC surface for tests and automation.
- Event log and resumable sessions.
- Capability-filtered tools from profile + project config + surface capabilities + approval policy.
- Starlark and external JSONL extensions.
- Optional tmux/zellij/instance orchestration.

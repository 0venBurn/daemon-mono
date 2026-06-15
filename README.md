# daemon poc

**Proof of concept.** The core idea: the problem with LLM products today is that there's only one way to interact with them an abstracted tui box desktop surface. There should be multiple surfaces for different contexts. Daemon is one shared core for model loops, tools, context, sessions, and edits; multiple replaceable surfaces on top.

This is a very rough poc that was developed with the idea of verifying possibilities and smoothness rather complete correctness. Likely will be rewrote in a seperate repo with learnings.

Neovim is the first surface POC. You can abstract the core from the surface. The plan is to repeat this pattern for improved Neovim, desktop, web, and TUI surfaces.

![daemon demo](./daemon-demo.gif)

## Installation

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

The Neovim plugin expects the daemon at `~/.local/bin/daemon`. Ensure `~/.local/bin` is on your `PATH`.

Load the repo-owned plugin from your checkout:

```lua
vim.opt.runtimepath:prepend("/path/to/daemon/nvim")
require("daemon").setup()
```

## Providers and auth

Daemon supports multiple LLM providers, selected by `DAEMON_PROVIDER` or `~/.config/daemon/auth.json`.

### `:DaemonAuth`

```vim
:DaemonAuth          Select and authenticate a provider interactively
:DaemonAuthStatus    Show current auth configuration
:DaemonAuthSwitch    Switch active provider (must already be configured)
:DaemonAuthLogout    Remove a provider's credentials
```

### Environment variables

| Variable               | Provider              | Description                                                |
| ---------------------- | --------------------- | ---------------------------------------------------------- |
| `DAEMON_PROVIDER`      | all                   | `anthropic`, `openai`, `google`, `opencode`, `opencode-go` |
| `ANTHROPIC_API_KEY`    | anthropic             | Direct Anthropic API key                                   |
| `OPENAI_API_KEY`       | openai                | Direct OpenAI API key                                      |
| `GOOGLE_API_KEY`       | google                | Direct Google API key                                      |
| `OPENCODE_API_KEY`     | opencode, opencode-go | OpenCode Zen/Go API key                                    |
| `DAEMON_MODEL`         | all                   | Override model name                                        |
| `OPENCODE_BASE_URL`    | opencode              | Override base URL                                          |
| `OPENCODE_GO_BASE_URL` | opencode-go           | Override base URL                                          |

Env vars take priority over `auth.json`.

### Auth file format

`~/.config/daemon/auth.json` (mode 0600):

```json
{
  "_active_provider": "anthropic",
  "anthropic": { "api_key": "sk-ant-...", "model": "claude-sonnet-4-5" },
  "opencode": { "api_key": "zen-...", "model": "claude-sonnet-4-5" },
  "openai": { "api_key": "sk-..." }
}
```

### OpenCode Zen

| Provider      | Default model       | Base URL                        | API type                |
| ------------- | ------------------- | ------------------------------- | ----------------------- |
| `opencode`    | `claude-sonnet-4-5` | `https://opencode.ai/zen`       | Auto (Anthropic/OpenAI) |
| `opencode-go` | `deepseek-v4-flash` | `https://opencode.ai/zen/go/v1` | Auto (Anthropic/OpenAI) |

Licensed under the MIT License. See [LICENSE](LICENSE).

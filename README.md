# daemon

A Neovim AI editing agent. Daemon runs as a headless Go process that streams LLM responses and applies edits back to Neovim over JSON-RPC.

> **Early testing.** Expect bugs, breaking changes, and rough edges. Not production-ready.

## Demo

![daemon demo](./daemon-demo.gif)

## Features

### Edit modes

- **Open edit** — `:DaemonEdit` or `<leader>ai` in normal mode. The model reads your buffer and applies surgical find/replace or insert edits. Works on the current file or across files using built-in tools (`read_file`, `list_directory`, `find_files`, `grep`, `create_file`).
- **Selection edit** — `<leader>ai` in visual mode. Edits are targeted at the selected region only. Faster patch path; the model sees exactly what you highlighted.
- **Explain** — `<leader>ax` in visual mode. Streams an explanation of the selected code in a floating markdown window. No edits, just understanding.

### Activity tracker with jump-to-insertion

`:DaemonActivity` opens a live activity panel showing every operation (model calls, tool use, edits) as it happens. Each entry is jumpable — press `<CR>` on any row to jump the cursor to the row/col where that edit was applied.

### Multi-turn conversations

`:DaemonAsk` continues the conversation in-context. The model sees the full transcript of prior edits and explanations, so you can iterate without re-explaining. `:DaemonPrompt <text>` appends a follow-up instruction to the running session inline.

### Thinking levels

`:DaemonThinking [off|low|medium|high|xhigh]` controls how much reasoning budget the model uses. Higher levels give deeper reasoning at the cost of latency. `:DaemonThinking` with no argument shows the current level.

### Streaming edits

Edits stream in as the model generates them — not buffered until the end. You see text appear character-by-character in your buffer as the model writes, with a ghost cursor showing the agent's position.

### Multi-provider

Supports Anthropic Claude, OpenAI, Google Gemini, and OpenCode proxy. Configure via `~/.config/daemon/auth.json` or environment variables. Switch providers with `:DaemonAuthSwitch`. See current config with `:DaemonInfo`.

## Installation

1. **Build the daemon binary:**

   ```sh
   git clone https://github.com/0venBurn/daemon.git
   cd daemon
   make build
   ```

   This installs `daemon` to `~/.local/bin/daemon`.

2. **Add the Neovim plugin:**

   ```lua
   -- In your Neovim config:
   vim.opt.runtimepath:prepend("/path/to/daemon/nvim")
   require("daemon").setup()
   ```

   Or use `make install-nvim` to print the config lines.

3. **Authenticate:**

   Run `:DaemonAuth` inside Neovim, or create `~/.config/daemon/auth.json` manually:

   ```json
   {
     "_active_provider": "anthropic",
     "anthropic": { "api_key": "sk-ant-..." }
   }
   ```

   Environment variables also work: `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `GOOGLE_API_KEY`, or `DAEMON_PROVIDER` + model-specific keys.

## Commands

| Command                        | Description                                     |
| ------------------------------ | ----------------------------------------------- |
| `:DaemonEdit [args]`           | Start an edit session (prompts if no args)      |
| `:DaemonPrompt <text>`         | Send a prompt to start or continue a session    |
| `:DaemonAsk [text]`            | Ask a follow-up question in the current session |
| `:DaemonExplain`               | Explain the visual selection                    |
| `:DaemonExplainClose`          | Close the explanation popup                     |
| `:DaemonCancel`                | Cancel the running session                      |
| `:DaemonNewSession`            | Cancel and reset for a fresh session            |
| `:DaemonActivity`              | Open the activity tracker                       |
| `:DaemonStatus`                | Show active session info                        |
| `:DaemonThrottle <ms> [chars]` | Set streaming speed                             |
| `:DaemonThinking [level]`      | Set thinking level (off/low/medium/high/xhigh)  |
| `:DaemonAuth`                  | Interactive provider authentication             |
| `:DaemonAuthStatus`            | Show auth config                                |
| `:DaemonAuthSwitch`            | Switch active provider                          |
| `:DaemonAuthLogout`            | Remove stored credentials                       |
| `:DaemonInfo`                  | Show current provider/model info                |

## Keymaps

| Key          | Mode          | Action                            |
| ------------ | ------------- | --------------------------------- |
| `<leader>ai` | Normal        | Edit (prompts for input)          |
| `<leader>ai` | Visual        | Edit selection                    |
| `<leader>ax` | Normal/Visual | Explain selection                 |
| `<leader>aq` | Normal        | Cancel agent                      |
| `<Esc>`      | Normal        | Cancel agent or close explanation |

## Running tests

```sh
make lint   # gofmt + go vet
make test    # go test ./...
make build   # go build ./cmd/daemon
```

## License

MIT. See [LICENSE](LICENSE).


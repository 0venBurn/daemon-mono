# Contributing

This project is a proof-of-concept. Small, focused changes are easiest to review.

## Build

```sh
go build -o ~/.local/bin/daemon ./cmd/daemon
```

## Test

```sh
go test ./...
```

## Neovim local setup

```lua
vim.opt.runtimepath:prepend("/path/to/daemon/nvim")
require("daemon").setup()
```

The plugin expects the daemon binary at `~/.local/bin/daemon`.

## Code style

- Keep `cmd/daemon` thin where practical.
- Prefer typed protocol/event boundaries over ad hoc maps.
- Keep editor buffer writes in Lua; Go emits operations/events.
- Add tests for patch/edit/protocol changes.

## Secrets

Do not commit `.env`, provider API keys, or `~/.config/daemon/auth.json`.

.PHONY: build test lint install-nvim

build:
	go build -o ~/.local/bin/daemon ./cmd/daemon

test:
	go test ./...

lint:
	gofmt -w cmd internal
	go test ./...

install-nvim:
	@echo 'Add this to your Neovim config:'
	@echo 'vim.opt.runtimepath:prepend("$(CURDIR)/nvim")'
	@echo 'require("daemon").setup()'

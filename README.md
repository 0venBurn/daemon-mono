# daemon poc

**Proof of concept.** The core idea: the problem with LLM products today is that there's only one way to interact with them an abstracted tui box desktop surface. There should be multiple surfaces for different contexts. Daemon is one shared core for model loops, tools, context, sessions, and edits; multiple replaceable surfaces on top.

This is a very rough poc that was developed with the idea of verifying possibilities and smoothness rather complete correctness. Likely will be rewrote in a separate repo with learnings.

Neovim is the first surface POC. You can abstract the core from the surface. The plan is to repeat this pattern for improved Neovim, desktop, web, and TUI surfaces.

![daemon demo](./daemon-demo.gif)

Licensed under the MIT License. See [LICENSE](LICENSE).

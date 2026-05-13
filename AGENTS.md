# AGENTS.md

## Response Style (Default)

Use a caveman style by default in every response.

Rules:

- Brief and neutral.
- Terse, high-signal, no filler.
- Keep full technical accuracy.
- Fragments OK.
- Keep exact technical terms, code, and error strings unchanged.

Disable caveman only when:

- Generating formal artifacts (reports, docs, PRDs, ADRs).
- Safety/irreversible warnings need full clarity.
- User explicitly says: "normal mode" or "stop caveman".
- Editing code files & writing comments

After exception section completes, resume caveman style automatically.

Examples:

- User: "Why React component re-render?"
  - Good: "Inline obj prop -> new ref -> re-render. `useMemo`."
- User: "Explain DB pooling"
  - Good: "Pool = reuse DB conn. Skip handshake -> faster under load."
- Destructive op warning (temporary clarity mode):
  - "**Warning:** This will permanently delete all rows in `users` and cannot be undone. Confirm backup first."

## Docs

Read docs when they help current intent. Docs are organised as follows.

/docs/code -> contains knowledge around system patterns, conventions and testing.

- system_patterns.md shows architectural overviews on what happens in each service.
- conventions.md considers the conventions for a project
- testing.md considers the testing strategy for the variety of changes/ implementation

/docs/adrs -> contain architectural design records for decisions made in the process

/docs/reports -> contains specific html reports generated about research.

/docs/prds -> contains product requirement docs around specific issues. Can be used in code reviews or when implementing.

## Project Context

This project is a Go project using starlark for the extension system. It is aiming to be a self extending coding harness.

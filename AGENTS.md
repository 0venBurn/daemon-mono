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

Read docs when they help current intent. Small/local work needs nearby files only. Broader/riskier work needs relevant docs and specs.

hello

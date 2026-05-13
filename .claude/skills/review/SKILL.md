---
name: review
description: Review code changes on two axes — code quality (bugs, security, performance, patterns) and requirements conformance (PRDs, issues, ADRs). Use this skill when the user wants a code review of a diff, branch, or set of changes. Triggers include "review this", "review the diff", "check my changes", "review this PR", or any request to evaluate code changes before merging. Always go deep into surrounding context, not just the diff itself. Do NOT use for general code exploration or implementation tasks.
---

# Review

A two-axis code review process. Axis 1 checks code quality. Axis 2 checks whether the change satisfies its requirements. Style preferences and non-critical nitpicks are out of scope on both axes.

## Process

### 1. Get Changes — Read the diff

Choose review target from user intent:

- Branch review: `git diff main...HEAD` (substitute base ref if not `main`)
- Staged review: `git diff --staged`
- Unstaged review: `git diff`
- File review: diff/read the specified files

If unclear, default to branch review when on a feature branch; otherwise ask which target to review.

### 2. Understand Context — Go beyond the diff

Do not review the diff in isolation. Read the surrounding code to understand:

- What does the changed code interact with?
- What invariants or assumptions does the broader system rely on?
- How does data flow through the affected area?

Read `@docs/code` to understand established patterns and conventions. If `docs/code` does not exist, note it and review against standard conventions for the detected stack.

### 3. Load Requirements — Find the source of truth

Identify the requirement(s) this change is meant to satisfy. Check in order:

1. **Linked issue or task** — if the branch name or commit messages reference an issue, read it.
2. **PRDs** — check `@docs/prds` for a matching product requirement doc.
3. **ADRs** — check `@docs/adrs` for architectural decisions relevant to the changed area.
4. **Task list** — if a task list exists for this feature, read it to understand scope and completion criteria.

If no requirements doc can be found, note it in the report and review on Axis 1 only.

### 4. Axis 1 — Code Quality

Flag issues in these categories only:

| Category           | What to look for                                                |
| ------------------ | --------------------------------------------------------------- |
| **Bugs**           | Logic errors, null/undefined handling, edge cases               |
| **Performance**    | N+1 queries, inefficient algorithms, memory leaks               |
| **Security**       | SQL injection, XSS, auth bypasses, data exposure                |
| **Correctness**    | Business logic errors, data integrity violations                |
| **Pattern Breaks** | Violations of established architectural patterns or conventions |

If an issue doesn't fall into one of these categories, do not raise it.

### 5. Axis 2 — Requirements Conformance

Compare the diff against the loaded requirements. Flag issues in these categories only:

| Category          | What to look for                                                     |
| ----------------- | -------------------------------------------------------------------- |
| **Missing**       | Requirements specified but not implemented in this change            |
| **Divergent**     | Implementation differs from what the PRD/issue/ADR specified         |
| **ADR Violation** | Change contradicts an architectural decision recorded in `docs/adrs` |
| **Scope Creep**   | Implementation includes work not covered by any requirement          |
| **Incomplete**    | Task list items marked done but implementation is partial or stubbed |

If no requirements were found in step 3, skip this axis and note it in the report.

## Output Format

For each issue found:

```
[SEVERITY] [AXIS] [CATEGORY] Brief title
Location: file:line (or "Requirements gap" for Axis 2 items with no code location)
Issue: What is wrong and why it matters.
```

Severity: `HIGH`, `MEDIUM`, or `LOW`. Only include `LOW` when still critical enough to block or repair.

Group findings by axis. Axis 1 first, then Axis 2.

If no issues are found on either axis, say so plainly. Do not pad the review.

Write the full report into an HTML file and place in `@docs/reports` with a meaningful name.

## Constraints

- **Critical issues only.** No style comments, no suggestions, no "consider doing X instead."
- **Context is mandatory.** Do not flag something as a pattern break without first verifying what the pattern actually is.
- **Requirements are mandatory.** Do not flag a requirements gap without first reading the actual requirement doc.
- **Be specific.** Vague concerns ("this might be slow") are not reviews — point to the exact code and explain why.

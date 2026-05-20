---
name: to-prd
description: Generate a Product Requirements Document from the current conversation context and push it to Linear. Ask which project to add it to. Use when a feature spec has emerged that should be captured as a permanent artifact before implementation.
---

# To PRD

Generate a PRD from the current conversation context and save it to linear. Ask which project to add it to.

Do NOT interview the user — just synthesize what you already know from the conversation.

## Process

### 1. Explore the codebase (if needed)

Understand the current state of the code, if you haven't already. Use the project's domain glossary vocabulary throughout the PRD, and respect any ADRs in the area you're touching.

### 2. Gather the spec from context

Extract these from the current conversation:

- **What problem are we solving?** From the user's perspective. What's the pain?
- **What's the proposed solution?** The user-facing outcome, not the implementation.
- **What user stories emerged?** Even rough ones — "as a X, I want Y, so that Z."
- **What implementation decisions were discussed?** Modules, schemas, API contracts, architectural notes.
- **What's explicitly out of scope?** Things discussed and deliberately deferred.

If any of these are fuzzy, synthesize what you can and flag gaps.

### 3. Draft the modules

Sketch the major modules to build or modify. Look for opportunities to extract deep modules that can be tested in isolation.

A deep module (as opposed to a shallow module) is one which encapsulates a lot of functionality in a simple, testable interface which rarely changes.

Ask the user: _Do these modules match your expectations? Which modules do you want tests written for?_

### 4. Determine the title and project

Ask the user: _What should the title be? (3-6 words, e.g. "Customer Portal" or "Payment Retry Logic")_

Ask the user: _Which Linear project should this PRD go into? (e.g. "Platform", "Web", "Mobile")_

Use the title to derive a slug: lowercase, hyphens for spaces, strip punctuation.

### 5. Propose the PRD outline to the user

Show a preview:

```
## Proposed PRD: NNNN-slug.md

**Problem:** {1-2 sentences}
**Solution:** {1-2 sentences}
**Modules:** {list of modules to build/modify}
**User stories:** {count} stories identified
**Out of scope:** {what's not included}
```

Ask: _Does this scope look right? Anything missing or over-scoped?_

Iterate until the user approves.

### 6. Push the PRD to Linear

Use the Linear skill to create a new document in the specified project with the PRD content.

Set the title to: `PRD: {Feature name}`

Add labels: `prd`, `spec`

Link to any relevant issues or existing ADRs.

<prd-template>
```md
# {Feature name}

## Problem Statement

The problem that the user is facing, from the user's perspective.

## Solution

The solution to the problem, from the user's perspective.

## User Stories

A LONG, numbered list of user stories in the format:

1. As an <actor>, I want a <feature>, so that <benefit>

Cover all aspects of the feature exhaustively.

## Implementation Decisions

A list of implementation decisions that were made. Include:

- The modules that will be built/modified
- The interfaces of those modules
- Technical clarifications from the developer
- Architectural decisions
- Schema changes
- API contracts
- Specific interactions

Do NOT include specific file paths or code snippets — they go stale fast.

Exception: if a prototype produced a snippet that encodes a decision more precisely than prose can (state machine, reducer, schema, type shape), inline it within the relevant decision and note briefly that it came from a prototype. Trim to the decision-rich parts — not a working demo, just the important bits.

## Testing Decisions

- What makes a good test (only test external behavior, not implementation details)
- Which modules will be tested
- Prior art for the tests (i.e. similar types of tests in the codebase)

## Out of Scope

Things that are explicitly out of scope for this PRD.

## Further Notes

Any further notes about the feature.

```
</prd-template>

### 7. Confirm

```

Pushed PRD to Linear: {project}/{slug}

```

```

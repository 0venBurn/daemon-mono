---
name: linear
description: Work with Linear issues. Use when user says "linear", "to issue", "to issues", "create issues", "pull issue", "fetch issue", "read issue", "update issue", "move issue", "comment on issue", or references a Linear identifier like "RAN-118". Supports creating implementation issues from local planning artifacts, fetching existing Linear issues, updating issue fields/status, and adding comments.
---

# Linear

Create, fetch, inspect, update, and comment on Linear issues from the coding agent.

## Prerequisites

This skill reads `LINEAR_API_KEY` from the environment. If it is not set, tell the user:

```bash
LINEAR_API_KEY is not set. Export it or add it to a .env file:

    export LINEAR_API_KEY=lin_api_...

Create a key at https://linear.app/settings/api
```

All GraphQL requests authenticate with:

```bash
-H "Authorization: $LINEAR_API_KEY"
```

Use `curl` against `https://api.linear.app/graphql` unless a project-specific Linear MCP/tool is available.

## Common helpers

### Check auth

```bash
test -n "$LINEAR_API_KEY" || echo "LINEAR_API_KEY is not set"
```

### GraphQL request shape

```bash
curl -s -X POST https://api.linear.app/graphql \
  -H "Authorization: $LINEAR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"query":"query { viewer { id name email } }"}'
```

Prefer GraphQL variables for user-provided strings instead of interpolating raw text into query strings.

## Modes

## 1. Fetch/read a Linear issue

Use when the user references an issue identifier like `RAN-118` or asks to pull/read/fetch an issue.

Query by identifier:

```bash
curl -s -X POST https://api.linear.app/graphql \
  -H "Authorization: $LINEAR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "query": "query Issue($id: String!) { issue(id: $id) { id identifier title description url branchName state { id name type } team { id key name } project { id name } assignee { id name } labels { nodes { id name } } comments { nodes { id body user { name } createdAt } } } }",
    "variables": { "id": "RAN-118" }
  }'
```

After fetching:

- Summarise the issue briefly.
- Identify the concrete implementation target in this repo.
- Read relevant code/docs before editing.
- If issue details are missing or ambiguous, ask for clarification before implementation.

## 2. Create implementation issues from local context

Use when the user wants to convert a plan/PRD/ADR/prototype into Linear issues, says `to issue`, `to issues`, `create issues`, or asks to break work into tickets.

### 2a. Ask for the Linear project

Ask: _Which Linear project should these issues go under?_

Look up project IDs:

```bash
curl -s -X POST https://api.linear.app/graphql \
  -H "Authorization: $LINEAR_API_KEY" \
  -H "Content-Type: application/json" \
  -d "{\"query\": \"{ projects { nodes { id name teams { nodes { id name key } } } } }\"}"
```

Match the user's project name using case-insensitive substring matching. Note the project ID and associated team IDs. If no match, list available projects and ask the user to pick one.

### 2b. Gather context

Work from conversation context plus local artifacts:

- ADRs in `docs/adrs/`
- PRDs in `docs/prds/`
- prototype code, if any
- current codebase state, if needed

If key artifacts are missing, ask whether to proceed with available context or pause while they create missing artifacts.

### 2c. Draft vertical slices

Break work into tracer-bullet issues:

- Each issue delivers a narrow complete path through all relevant layers.
- A completed slice is independently demoable/verifiable.
- Prefer many thin slices over few thick ones.

Present proposed issues with:

- Title
- Blocked by
- User stories / source requirements covered
- AFK/HITL expectation if relevant

Ask the user to approve granularity/dependencies before publishing.

### 2d. Create issues

Determine workflow state:

```bash
curl -s -X POST https://api.linear.app/graphql \
  -H "Authorization: $LINEAR_API_KEY" \
  -H "Content-Type: application/json" \
  -d "{\"query\": \"{ team(id: \\\"TEAM_ID\\\") { states { nodes { id name type } } } }\"}"
```

Use state with `type: "unstarted"` or name `Todo`.

Create issue:

```bash
curl -s -X POST https://api.linear.app/graphql \
  -H "Authorization: $LINEAR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "query": "mutation IssueCreate($input: IssueCreateInput!) { issueCreate(input: $input) { issue { id identifier title url } success } }",
    "variables": {
      "input": {
        "teamId": "TEAM_ID",
        "projectId": "PROJECT_ID",
        "title": "Issue title here",
        "description": "Issue body in markdown here",
        "stateId": "STATE_ID"
      }
    }
  }'
```

Report created `identifier` and `url`.

### 2e. Link blockers

After all issues are created, create relations:

```bash
curl -s -X POST https://api.linear.app/graphql \
  -H "Authorization: $LINEAR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "query": "mutation RelationCreate($input: RelationCreateInput!) { relationCreate(input: $input) { success } }",
    "variables": {
      "input": {
        "issueId": "BLOCKED_ISSUE_ID",
        "relatedIssueId": "BLOCKER_ISSUE_ID",
        "type": "blocks"
      }
    }
  }'
```

`issueId` is the blocked issue; `relatedIssueId` is the blocker.

## 3. Update a Linear issue

Use when the user asks to update status/title/description/assignee/labels/project or otherwise modify an existing issue.

Fetch the issue first unless already fetched in this conversation. Confirm destructive or broad changes before applying.

Update issue fields:

```bash
curl -s -X POST https://api.linear.app/graphql \
  -H "Authorization: $LINEAR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "query": "mutation IssueUpdate($id: String!, $input: IssueUpdateInput!) { issueUpdate(id: $id, input: $input) { success issue { id identifier title url state { name } } } }",
    "variables": {
      "id": "RAN-118",
      "input": {
        "title": "Updated title"
      }
    }
  }'
```

To move status, first query team states and set `stateId`.

## 4. Comment on a Linear issue

Use when the user asks to add a comment, post findings, link a branch/PR, or record implementation notes.

```bash
curl -s -X POST https://api.linear.app/graphql \
  -H "Authorization: $LINEAR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "query": "mutation CommentCreate($input: CommentCreateInput!) { commentCreate(input: $input) { success comment { id url } } }",
    "variables": {
      "input": {
        "issueId": "LINEAR_ISSUE_UUID",
        "body": "Comment body in markdown"
      }
    }
  }'
```

## Issue body template for created tickets

```markdown
## Parent

Reference the parent PRD/ADR/Linear issue by stable title or identifier. Do not use local file paths unless the repository path is meaningful to the team.

## What to build

A concise description of the vertical slice. Describe end-to-end behavior, not layer-by-layer implementation.

## Decisions this slice depends on

Reference relevant ADRs, PRDs, prototypes, or existing Linear issues.

## Acceptance criteria

- [ ] Criterion 1
- [ ] Criterion 2
- [ ] Criterion 3

## Blocked by

- Linear issue identifier, or "None — can start immediately"
```

## Constraints

- Do not close or modify parent issues unless explicitly asked.
- Do not update Linear until the user approves generated issue breakdowns.
- For existing issue implementation, fetch issue details first, then inspect local code before editing.
- Prefer terse summaries and exact issue identifiers/URLs in final responses.

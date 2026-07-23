# Phase 09: PR Review Tools - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-07-23
**Phase:** 09-pr-review-tools
**Areas discussed:** Degradation contract, Optional project_id, open_review result

---

## Degradation contract

### Q1: When the GET list returns 200 with state != "ok", how should the list tools surface that?

| Option | Description | Selected |
|--------|-------------|----------|
| IsError hybrid (rec) | Pass Kamacu's JSON through verbatim (state field included) AND set CallToolResult.IsError=true when state != "ok". Agent gets structured JSON + MCP error flag. Minimal targeted inspection (first such inspection in the milestone). | ✓ |
| Raw passthrough | Return JSON verbatim with IsError=false always; agent reads state like the browser. Maximally consistent with 06/07/08 pattern + SC1, but state:"no_gh" comes back as a success, not an MCP error. | |
| Full conversion | Bridge parses state, returns a plain error string (no JSON) for non-ok. Cleanest message but loses structured JSON and is the most bridge-side logic. | |

**User's choice:** IsError hybrid
**Notes:** The GET list endpoint always returns 200 with a `state` field (GHSET-03/GHCOL-05) — the locked bridge.call pattern (2xx→passthrough, non-2xx→error) does not fit. `open_review` is unaffected (its gates return real 409/502/400 → clean D-05 errors).

### Q2: What should the TextContent body be when IsError=true?

| Option | Description | Selected |
|--------|-------------|----------|
| Pure JSON (rec) | Content = raw Kamacu Result JSON verbatim; the state field IS the message. Purest passthrough; agent parses JSON fine. | ✓ |
| Prose + JSON | A short prose line mapped from state followed by the raw JSON. Skimmable, at the cost of a 4-string state→prose map in the bridge. | |

**User's choice:** Pure JSON
**Notes:** The `state` value names the problem (disabled/no_gh/auth_required/error). Honesty-over-hand-holding posture (cf. 08 D-05 / D-M001-2).

### Q3: What should each list tool return from the shared Result payload?

| Option | Description | Selected |
|--------|-------------|----------|
| Per-tool array (rec) | Success → just the tool's array (prs / reviewed); degradation → full Result JSON with IsError=true. Distinct per-tool results, no redundancy. | ✓ |
| Always full Result | Both tools return full Result JSON; IsError from state. Simplest + uniform, but two tools return byte-identical payloads (design smell). | |
| Per-tool envelope | Each returns {state, stale, fetchedAt, <queue>:[...]} with only its queue. Uniform + distinct, but most rebuild logic. | |

**User's choice:** Per-tool array

---

## Optional project_id

### Q1: How should the optional project_id behave on the two list tools?

| Option | Description | Selected |
|--------|-------------|----------|
| Require it (rec) | project_id REQUIRED in InputSchema; omitting → clear MCP error. Every tool stays a single bridge.call. Diverges from the '?' in REQUIREMENTS but matches per-repo reality + agent scope. | ✓ |
| Aggregate all | Keep optional; when omitted, enumerate linked projects + merge cached calls. Satisfies global triage. First multi-call aggregation; PRSummary has no repo field → loses provenance. | |
| Aggregate + annotate | Aggregate but augment each PR with project_id/repo. Richest global triage, most bridge logic. | |

**User's choice:** Require it
**Notes:** Deliberate refinement of REQUIREMENTS MCPREV-01/02 — the `?` is dropped. An agent runs in one project+task PTY; PRs are per-repo; the endpoint is per-project. Agent wanting global view calls list_projects then loops.

---

## open_review result

### Q1: What should open_review return on success (200)?

| Option | Description | Selected |
|--------|-------------|----------|
| Raw passthrough (rec) | Return Kamacu's {task, pr} JSON verbatim (IsError=false). Locked 06/07/08 default. Agent gets task.id + worktree_path + live PR meta. | ✓ |
| Trimmed result | Return just {task_id, worktree_path, pr_title, pr_url}. Less noise but first bridge-side shaping on a 200, no clear win. | |

**User's choice:** Raw passthrough
**Notes:** Uses bridge.call verbatim — open_review's degradation is non-2xx (409/502/400) so no IsError handling needed. No agent auto-start; task.status='todo' + worktree provisioned.

### Q2: Should the list tools expose a cache-bypass refresh param?

| Option | Description | Selected |
|--------|-------------|----------|
| No refresh (rec) | Rely on 60s TTL + 10s attempt floor. Tools stay minimal. Browser keeps its refresh button; MCP tools don't need one for v1.11. | ✓ |
| Expose refresh? | Optional refresh boolean → ?refresh=1. Parity with browser refresh; useful for force-fresh fetch. | |

**User's choice:** No refresh

---

## the agent's Discretion

- `internal/mcp/reviews.go` handler + test layout (mirror 07 per-resource shape; `registerReviewTools` + `bridge.*Review*` methods + `reviews_test.go`).
- How `state` is parsed: unmarshal into `github.Result` (type-safe, couples mcp→github leaf package) vs a local minimal struct / generic map (no coupling).
- The review-specific list helper signature/name (likely one shared helper taking the queue name; only "prs" vs "reviewed" differs).
- Exact InputSchema map shape per tool (07 flat `map[string]any` template; project_id required on all 3, pr_number required on open_review).
- Error-case test coverage (one representative error per tool per 07 D-08; list-tool test must exercise state→IsError branch with an HTTP-200 fake handler).

## Deferred Ideas

- Global / cross-project PR triage (list tools with no project_id aggregating) — D-04 requires project_id; agents loop over list_projects. Revisit if real need.
- `refresh?` cache-bypass param on list tools — D-06 ships without it.
- `get_pr` / `get_review_diff` MCP tool (expose GET .../pull-requests/{n}) — not in MCPREV-01..03.
- Trimmed open_review result — rejected (raw passthrough).
- MCPHARD-01..03 (typed errors, stdout guards, e2e harness) — v1.12, inherited.
- Per-task auto-scoping (MCPAUTO `get_my_project`) — would soften D-04's required project_id; v1.12.

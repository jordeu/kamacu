# Phase 09: PR Review Tools - Research

**Researched:** 2026-07-23
**Domain:** MCP tool bridging (Go) — translating an agent-facing tool surface onto Kamacu's existing v1.3/v1.5 `internal/github` HTTP endpoints
**Confidence:** HIGH

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**D-01 — IsError hybrid for the list tools.** The two list tools do NOT use `bridge.call` verbatim. A review-specific bridge helper does `b.do` → read body → parse the top-level `state` field → branch: when `state == "ok"` return `CallToolResult{Content:[TextContent{...}], IsError:false}`; when `state != "ok"` return `CallToolResult{Content:[TextContent{...}], IsError:true}`. Transport errors and any genuine non-2xx still wrap via the D-05 shape (`kamacu %s %s: ...`). `open_review` is UNAFFECTED — its gates return real non-2xx (409/502/400) so it uses `bridge.call` verbatim.

**D-02 — Pure JSON body on error.** When `IsError=true`, the TextContent payload is the raw Kamacu `Result` JSON verbatim (e.g. `{"state":"no_gh","prs":null,"reviewed":null,...}`) — NO prose prefix, NO state→message map. The `state` value itself names the problem; the IsError flag is the actionable signal.

**D-03 — Per-tool array on success; full Result on degradation.** `list_pending_reviews` success → the `prs` array (re-marshalled). `list_recently_reviewed` success → the `reviewed` array (re-marshalled). Degradation (`state != "ok"`) for EITHER tool → `IsError=true` + the full `Result` JSON. The agent checks `IsError` first, so the success-array-vs-error-object shape difference is unambiguous.

**D-04 — `project_id` is REQUIRED on both list tools** (and on `open_review`, which already requires it via the POST path). The REQUIREMENTS `?` is dropped. Rationale: PRs are per-repo, repos are per-project, the endpoint is per-project, and an agent runs inside one project+task PTY so it always knows its `project_id`. Omitting it returns a clear MCP error before any HTTP call.

**D-05 — `open_review` returns the raw `{task, pr}` envelope as TextContent** (IsError=false) via `bridge.call` verbatim. No trimming, no bridge-side shaping. No agent auto-start — `open_review` provisions the workspace only.

**D-06 — No `refresh` / force-fetch param on any tool.** The list tools rely on the existing per-repo cache (60s success TTL, 10s attempt floor).

**SC3 satisfaction** — the v1.3 two-gate ladder (toggle → link) is enforced server-side in `pullrequests.go` on every call; the bridge adds NO gate logic and NO bypass. List tools surface a blocked gate as `IsError=true` + `state:"disabled"`; `open_review` surfaces it as a D-05 wrapped `HTTP 409` error.

### the agent's Discretion

- **`internal/mcp/reviews.go` handler + test layout** — mirror Phase 07's per-resource shape (D-07): `registerReviewTools(s, b)` called from `registerTools` in `server.go`; `bridge.listPendingReviews` / `listRecentlyReviewed` / `openReview` methods; one `reviews_test.go` with happy + error cases per tool.
- **How `state` is parsed** — unmarshal into `github.Result` (couples `internal/mcp` to `internal/github`) OR a tiny local struct / `map[string]any` reading only the `state` + `prs`/`reviewed` keys.
- **The review-specific list helper signature/name** — e.g. a shared `func (b *bridge) listReviews(ctx, projectID int64, queue string)` (only `queue` differs), vs two separate methods.
- **Exact InputSchema map shape per tool** — Phase 07's flat `map[string]any` template applies. `project_id` required on all 3; `pr_number` (integer) required on `open_review`.
- **Error-case test coverage** — one representative error per tool (list tool with disabled/unlinked → assert `IsError=true` + state field; `open_review` with integration off → assert D-05 wrapped `HTTP 409`).

### Deferred Ideas (OUT OF SCOPE)

- Global / cross-project PR triage (`list_pending_reviews()` with no `project_id`).
- `refresh?` cache-bypass param on the list tools.
- `get_pr` / `get_review_diff` MCP tool (exposing `GET /api/projects/{id}/pull-requests/{n}`).
- Trimmed `open_review` result envelope.
- MCPHARD-01..03 (typed bridge error taxonomy, stdout guards, real-binary e2e harness) — v1.12.
- Per-task auto-scoping (MCPAUTO) — v1.12.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| MCPREV-01 | `list_pending_reviews(project_id?)` lists PRs awaiting the user's review (`user-review-requested:@me draft:false`) | Bridges `GET /api/projects/{id}/pull-requests`; projects the `prs` array of `github.Result` on `state:"ok"`, else IsError+full-Result (D-01..D-03). D-04 makes `project_id` required. |
| MCPREV-02 | `list_recently_reviewed(project_id?)` lists open PRs the user already reviewed (`reviewed-by:@me draft:false`) | Same endpoint + same helper as MCPREV-01; projects the `reviewed` array instead. One cached `gh` cycle serves both (no N+1). |
| MCPREV-03 | `open_review(project_id, pr_number)` opens a PR as a review workspace (reuses v1.3 Phase 12 find-or-create `source='github_pr'` task) | Bridges `POST /api/projects/{id}/pull-requests/{n}/review` verbatim via `bridge.call`; returns `{task, pr}` TextContent (D-05). |
</phase_requirements>

## Summary

Phase 09 is the **thinnest, most mechanical phase of the v1.11 milestone**: it adds exactly ONE new Go file (`internal/mcp/reviews.go`), ONE new test file (`internal/mcp/reviews_test.go`), and ONE new line in `internal/mcp/server.go`'s `registerTools`. It performs **zero** changes to `internal/api/`, `internal/github/`, the database, migrations, or the frontend. Every ounce of real work — the `gh` calls, the per-repo TTL cache, the two-gate ladder (toggle → link), the find-or-create `source='github_pr'` task path, the detached PR-head worktree provisioning, the board-leak filter — already exists in Kamacu's v1.3/v1.5 `internal/github` + `internal/api/pullrequests.go` surface. Phase 09 is pure translation: three MCP tools that route to three already-healthy HTTP endpoints. `[VERIFIED: codebase — internal/api/pullrequests.go, internal/github/service.go read in full]`

The pattern is locked by Phases 06/07/08: each tool is a thin handler that builds a path (+ optional body) and delegates the response half to the bridge. Two of the three tools (`list_pending_reviews`, `list_recently_reviewed`) hit the **same** cached `GET` endpoint and share a single state-inspecting helper; the third (`open_review`) is a one-line `bridge.call` wrapper around the POST. The one genuine design divergence — the **IsError hybrid** (D-01) — is forced by a load-bearing wire-contract fact: `GET /api/projects/{id}/pull-requests` **always returns HTTP 200**, carrying degradation in the `state` field of `github.Result` (`ok`|`disabled`|`no_gh`|`auth_required`|`error`), never in the status code. The locked `bridge.call` (2xx→passthrough, non-2xx→error) therefore cannot fit; the list helper reads ONE top-level string field and sets ONE bool. `[VERIFIED: internal/api/pullrequests.go:40-81 always writeJSON(200,...); internal/github/service.go:19-25 Result.State]`

**Primary recommendation:** Implement `reviews.go` as a near-clone of `workspaces.go`/`tasks.go`, with a shared `listReviews(ctx, projectID, queue)` helper for the two list tools (the state→IsError logic lives in exactly one place) and a thin `openReview` that delegates to `bridge.call`. Use a **local struct** (not `github.Result`) for the state-decode to preserve the milestone's "internal/mcp does not import other internal/* domain packages" streak — the decode only needs `State string` + `PRs`/`Reviewed` slices. Verify the IsError branch fires on `state != "ok"` **regardless of array nullness** (the stale-cache edge: a transient `no_gh`/`error` with a prior successful cache carries non-null arrays + `stale:true`).

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| MCP tool surface (`list_pending_reviews`, `list_recently_reviewed`, `open_review`) | MCP bridge (`internal/mcp/reviews.go`) | — | Translation only — parse args, build path, hand response back. Owns the IsError-flag decision (D-01). |
| `gh` execution + per-repo TTL cache | Kamacu API + `internal/github` (existing) | — | Already implemented; Phase 09 does NOT touch. One `gh` cycle serves both queues (no N+1). |
| Two-gate ladder (settings toggle → project link) | Kamacu API (`internal/api/pullrequests.go`) | — | Enforced server-side on EVERY call; bridge adds zero gate logic and cannot bypass. |
| Find-or-create review task + worktree provisioning | Kamacu API (`internal/api/pullrequests.go` POST handler) | `internal/worktree` | Runs unchanged on the POST; `open_review` is a verbatim bridge to it. |
| Agent-facing error signaling | MCP bridge (IsError flag) | — | Tool-level conditions (degraded state) → IsError+Content per SDK spec; transport/HTTP errors → protocol-level error per `bridge.call`. |

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/modelcontextprotocol/go-sdk/mcp` | v1.6.1 | MCP server + tool types (`Server`, `Tool`, `CallToolResult`, `CallToolRequest`, `TextContent`) | Already pinned in `go.mod`; every Phase 06/07/08 tool uses it. **No version change.** `[VERIFIED: go.mod]` |
| Go stdlib (`net/http`, `encoding/json`, `fmt`, `context`) | Go 1.26 | HTTP bridging, JSON decode of `state`, arg parsing | Already imported by every `internal/mcp/*.go`. `[VERIFIED: bridge.go, tasks.go imports]` |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `kamacu/internal/github` (optional) | in-repo | The `github.Result` / `PRSummary` wire types | ONLY if the implementer chooses to decode the state body into the real wire types instead of a local struct (see Discretion recommendation below). |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Local struct for `state` decode | `github.Result` (couples `internal/mcp` → `internal/github`) | `github.Result`/`PRSummary` are stable wire types and the decode is type-safe; but it breaks the milestone's clean streak that `internal/mcp` imports only stdlib + SDK. Local struct wins on coupling; `github.Result` wins on type safety. Either is defensible. |

**Installation:**
```bash
# NO INSTALL. This phase adds ZERO new dependencies.
# All imports are already in go.mod (go-sdk v1.6.1) or stdlib.
```

**Version verification:** Confirmed `github.com/modelcontextprotocol/go-sdk v1.6.1` and `github.com/google/subcommands v1.2.0` already present in `go.mod`. No `go get` required.

## Package Legitimacy Audit

> **No new packages installed in this phase.** Phase 09 adds only Go source files (`internal/mcp/reviews.go`, `internal/mcp/reviews_test.go`) and one registration line in `internal/mcp/server.go`. Every import is already present in `go.mod` from Phases 06–08 (`github.com/modelcontextprotocol/go-sdk/mcp` v1.6.1, stdlib). The optional `kamacu/internal/github` import is an in-repo package, not an external registry dependency.

| Package | Registry | Age | Downloads | Source Repo | Verdict | Disposition |
|---------|----------|-----|-----------|-------------|---------|-------------|
| (none — no new external packages) | — | — | — | — | — | N/A |

**Packages removed due to [SLOP] verdict:** none
**Packages flagged as suspicious [SUS]:** none

## Architecture Patterns

### System Architecture Diagram

```
  agent CLI (claude/opencode) inside a Kamacu task PTY
        │  JSON-RPC over stdio (tools/call)
        ▼
  ┌─────────────────────────────────────────────────────────┐
  │ kamacu mcp serve  (internal/mcp/)                        │
  │                                                          │
  │   registerReviewTools(s, b)   ◄── NEW (one line in       │
  │      │                            server.go registerTools)│
  │      ├── list_pending_reviews ─► b.listReviews(..,"prs") │
  │      ├── list_recently_reviewed► b.listReviews(..,"rev") │
  │      │        shared helper:                              │
  │      │          b.do(GET /api/projects/{id}/pull-requests)│
  │      │          → read body → parse top-level "state"     │
  │      │          → if state=="ok":  remarshal queue array, │
  │      │                              IsError=false          │
  │      │          → else:           full Result JSON,       │
  │      │                              IsError=true   (D-01) │
  │      │                                                   │
  │      └── open_review ─────────► b.call(                  │
  │              POST /api/projects/{id}/pull-requests/{n}/   │
  │              review)  ◄── verbatim bridge.call (D-05)     │
  └────────────────────┬────────────────────────────────────┘
                       │ HTTP, X-Kamacu-Token, loopback only
                       ▼
  ┌─────────────────────────────────────────────────────────┐
  │ Kamacu HTTP API (internal/api/pullrequests.go) UNCHANGED │
  │                                                          │
  │   GET .../pull-requests  (always 200)                    │
  │     GATE1: settings toggle off ─► state:"disabled"       │
  │     GATE2: project unlinked  ─► state:"disabled"         │
  │     else: svc.Get() → github.Result{State,PRs,Reviewed}  │
  │           (per-repo cache: 60s TTL, 10s floor)           │
  │                                                          │
  │   POST .../pull-requests/{n}/review                      │
  │     GATE1/GATE2 fail ─► 409                              │
  │     gh failure / worktree failure ─► 502                 │
  │     find-or-create source='github_pr' task + CheckoutPR  │
  │     → {task, pr} (200)                                   │
  └────────────────────┬────────────────────────────────────┘
                       │ (gh CLI only when both gates open)
                       ▼
                    GitHub
```

A reader can trace the primary use case (agent opens a review) top-to-bottom: `tools/call open_review` → `bridge.call` (POST) → server-side gate check → find-or-create task + worktree → `{task, pr}` → TextContent → agent. The list tools share an identical left-hand path through the IsError-hybrid helper.

### Recommended Project Structure
```
internal/mcp/
├── server.go          # +1 line: registerReviewTools(s, b) in registerTools
├── bridge.go          # UNCHANGED (bridge.call reused verbatim by open_review)
├── reviews.go         # NEW — 3 tools: registrar + 3 bridge methods (+ shared listReviews helper)
└── reviews_test.go    # NEW — happy + error per tool (incl. state→IsError + open_review 409)
```

### Pattern 1: Per-resource file split (Phase 07 D-07)
**What:** One `registerXTools(s, b)` per resource owns its `s.AddTool` calls + the `bridge.*X*` handler methods; one `x_test.go` mirrors it. `registerTools` in `server.go` is a flat list of registrar calls.
**When to use:** Always — this is the locked milestone pattern.
**Example:**
```go
// server.go — the single integration point (ONE new line):
func registerTools(s *mcp.Server, b *bridge) {
	registerTaskTools(s, b)
	registerProjectTools(s, b)
	registerWorkspaceTools(s, b)
	registerSessionTools(s, b)
	registerReviewTools(s, b) // NEW — Phase 09
}
```

### Pattern 2: The IsError hybrid (D-01) — scoped divergence from bridge.call
**What:** When an endpoint's success/degradation contract rides in the body (not the status), a resource-specific helper reads ONE top-level field and sets ONE bool. This is the **second** scoped divergence in the milestone (Phase 08's `subscribe` diverged for streaming; Phase 09's list tools diverge for the state-field model). The divergence is narrowly scoped and documented; transport errors and genuine non-2xx still wrap via the D-05 shape.
**When to use:** Only here — every other tool uses `bridge.call`.
**Example:**
```go
// Source: SDK protocol.go:92-108 (IsError semantics) + D-01/D-02/D-03
// listReviews is the shared body of both list tools. `queue` selects which
// array to project on success: "prs" (awaiting) or "reviewed".
func (b *bridge) listReviews(ctx context.Context, projectID int64, queue string) (*mcp.CallToolResult, error) {
	path := fmt.Sprintf("/api/projects/%d/pull-requests", projectID)
	resp, err := b.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("kamacu bridge: %w", err) // transport error → protocol-level
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("kamacu GET %s: read body: %w", path, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Defensive: the endpoint always returns 200, but a genuine non-2xx
		// (e.g. a future 500) still wraps as a protocol-level error.
		return nil, fmt.Errorf("kamacu GET %s: HTTP %d: %s", path, resp.StatusCode, string(body))
	}
	// Decode ONLY what we need. Local struct keeps internal/mcp decoupled
	// from internal/github (the milestone's clean-import streak).
	var res struct {
		State    string `json:"state"`
		Stale    bool   `json:"stale"`
		PRs      json.RawMessage `json:"prs"`
		Reviewed json.RawMessage `json:"reviewed"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		// Shape surprise: surface as a protocol-level error (faithful, not faked).
		return nil, fmt.Errorf("kamacu GET %s: parse result: %w", path, err)
	}
	if res.State != "ok" {
		// D-02: pass the FULL Result JSON verbatim (state names the problem).
		// IsError=true is the actionable signal. Do NOT gate on array nullness
		// — a stale-cache degrade (transient error + prior success) carries
		// non-null arrays + stale:true; the agent checks IsError first.
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: string(body)}},
		}, nil
	}
	// D-03: project the requested queue array on success.
	raw := res.PRs
	if queue == "reviewed" {
		raw = res.Reviewed
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}},
	}, nil
}
```

### Anti-Patterns to Avoid
- **Do NOT gate IsError on array nullness.** A transient `no_gh`/`error` state with a prior successful cache carries NON-null `prs`/`reviewed` arrays (stale data) + `stale:true`. Set `IsError=true` purely on `state != "ok"`. (See Common Pitfalls.)
- **Do NOT add gate logic to the bridge.** The two-gate ladder runs server-side on every call; mirroring it in the bridge would duplicate/bypass it. SC3 is satisfied by *not* bypassing.
- **Do NOT use `SetError(err)` for the degrade path.** `SetError` overwrites `Content` with the error string — D-02 wants the raw Result JSON. Set `IsError: true` directly and provide your own `Content`.
- **Do NOT make `project_id` optional** to "match the REQUIREMENTS `?`". D-04 deliberately drops the `?`; an agent always knows its `project_id`, and requiring it keeps every tool a single HTTP call.
- **Do NOT expose a `refresh` param.** D-06 ships without it; the 60s TTL + 10s floor govern freshness.
- **Do NOT import `internal/store`** (or any DB package) from `internal/mcp`. The bridge speaks HTTP only.
- **Do NOT write to stdout.** `slog` is pinned to stderr (Phase 06 D-12); the JSON-RPC stream must stay clean.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| `gh` invocation, caching, draft filtering, checks reduction | A second `gh` call path from the bridge | The existing `GET /api/projects/{id}/pull-requests` + `internal/github.Service` | Already battle-tested (v1.3/v1.5); one cached cycle serves both queues. Re-inventing it would violate the milestone's "no N+1 gh calls" and re-introduce every edge case (draft dedup, checks rollup, host/account selection). |
| Find-or-create review task + worktree on PR head | New provisioning code | `POST /api/projects/{id}/pull-requests/{n}/review` | The v1.3 Phase 12 path handles reattach, retry, board-leak exclusion, detached-head worktree. `open_review` is a one-line bridge to it. |
| Two-gate enforcement (toggle → link) | Gate checks in the bridge | Server-side gates in `pullrequests.go` | Gates run unchanged on every call; the bridge cannot bypass them and must not duplicate them. |
| Error envelope shaping for the list tools | A `state`→message map or prose prefix | The raw `Result` JSON + the `IsError` flag | The `state` value names the problem faithfully (honesty-over-hand-holding, per project posture). |

**Key insight:** This phase is the purest "don't hand-roll" phase in the milestone. If the implementer is writing more than ~120 lines of `reviews.go` + ~150 lines of test, they are almost certainly re-implementing something Kamacu already does — stop and route to the existing endpoint instead.

## Common Pitfalls

### Pitfall 1: Gating IsError on array nullness (the stale-cache edge)
**What goes wrong:** An implementer reads D-03's parenthetical ("both arrays null") and writes `if res.PRs == nil { IsError=true }`, or assumes a non-ok state always has empty arrays.
**Why it happens:** `github.Result.resultLocked()` (`service.go:165-178`) sets `PRs`/`Reviewed` from cache whenever `e.hasCache` is true — INDEPENDENT of `state`. So a transient `no_gh`/`auth_required`/`error` (lastErr set) that occurs AFTER a prior successful fetch returns `state != "ok"` AND `stale:true` AND **non-null arrays** carrying the last-good lists.
**How to avoid:** Set `IsError=true` **purely** on `state != "ok"`. Never inspect array nullness to decide IsError. The agent checks `IsError` first (D-03), so it will not consume stale data as fresh — but the stale data is legitimately in the payload for context.
**Warning signs:** A test that asserts `IsError=true` for `state:"no_gh"` AND simultaneously asserts the payload has `prs:null` — that assertion only holds for the endpoint-gate `disabled` case (where the handler constructs `Result{State:"disabled"}` with no arrays), not for the Service-driven degrade states.
**Scope note:** For the three SC3 scenarios specifically (integration off → `disabled`; unlinked → `disabled`; gh absent → `no_gh` with no prior cache), the arrays ARE null in practice. The nuance matters for the `auth_required`/`error`/`no_gh-after-success` cases and for writing robust tests.

### Pitfall 2: Forgetting `json:"isError,omitempty"` is the SDK's job (not yours)
**What goes wrong:** Hand-marshaling the IsError flag into the TextContent, or wrapping the result in a custom envelope.
**Why it happens:** Misunderstanding the SDK contract.
**How to avoid:** Set `CallToolResult.IsError = true` directly; the SDK serializes it as `"isError":true` on the wire (verified `protocol.go:108`). Your `Content` is just the TextContent payload. `false` omits the field entirely (omitempty) — correct, since success is the default.

### Pitfall 3: Treating `bridge.call` as the only response path
**What goes wrong:** Forcing the list tools through `bridge.call` and then post-hoc inspecting the returned TextContent to set IsError — convoluted, and `bridge.call` returns a fully-built `CallToolResult` you'd have to mutate.
**Why it happens:** Over-applying the "every tool is the same shape" invariant.
**How to avoid:** The list tools use a sibling helper (`listReviews`) that calls `b.do` directly and builds the `CallToolResult` with the IsError branch. Phase 08 already established the precedent that a tool MAY diverge from `bridge.call` when the endpoint demands it (subscribe streamed). `open_review` still uses `bridge.call` verbatim — its gates return real non-2xx.

### Pitfall 4: Re-adding the `?` to `project_id` "because REQUIREMENTS says so"
**What goes wrong:** Making `project_id` optional to match the roadmap-time signature sketch, then having to invent an aggregate-all path.
**Why it happens:** Treating REQUIREMENTS as more authoritative than the phase CONTEXT.
**How to avoid:** D-04 is the locked refinement — `project_id` is REQUIRED. Document the refinement in each tool's description so agents know PRs are per-repo. An agent wanting a global view calls `list_projects` then loops.

### Pitfall 5: Importing `internal/github` and coupling the bridge to a domain package
**What goes wrong:** Decoding into `github.Result` works, but it ends the milestone's clean streak that `internal/mcp` imports only stdlib + SDK, and it pulls `internal/github`'s transitive deps into the MCP subcommand's binary.
**Why it happens:** Reaching for the "real" type for type safety.
**How to avoid:** Decode into a tiny local struct (only `State`, and `PRs`/`Reviewed` as `json.RawMessage` so success can re-marshal the chosen queue without even defining `PRSummary`). This keeps `internal/mcp` self-contained and the success-path projection is a one-field pick. (This is a recommendation, not a lock — the discretion is the agent's.)

### Pitfall 6: Forgetting the `withRecover` wrapper (or not)
**What goes wrong:** Either omitting panic recovery (Phase 06 Open Q1 / Pitfall 2) OR wrapping review handlers unnecessarily.
**Why it happens:** Cargo-culting from `sessions.go` which wraps every handler in `withRecover`.
**How to avoid:** `sessions.go` uses `withRecover` because session handlers touch streaming/concurrent session state where a panic would desync the JSON-RPC stream. The review tools are simple synchronous HTTP bridges (like `tasks.go`/`workspaces.go`, which do NOT use `withRecover`). Match the task/workspace pattern — no `withRecover` needed unless a profile shows a panic risk. (If the team prefers defense-in-depth, wrapping is harmless; but it is not required for parity with the closest analogues.)

## Code Examples

Verified patterns from official sources and the in-repo codebase.

### IsError semantics (the load-bearing SDK fact for D-01)
```go
// Source: go-sdk@v1.6.1/mcp/protocol.go:92-108 (read from module cache)
//
//   // IsError reports whether the tool call ended in an error.
//   // If not set, this is assumed to be false (the call was successful).
//   // Any errors that originate from the tool should be reported inside the
//   // Content field, with IsError set to true, not as an MCP protocol-level
//   // error response. Otherwise, the LLM would not be able to see that an
//   // error occurred and self-correct.
//   // However, any errors in finding the tool, ... or any other exceptional
//   // conditions, should be reported as an MCP error response.
//   IsError bool `json:"isError,omitempty"`
//
// IMPLICATION for D-01: a degraded list (state != "ok") is a TOOL-LEVEL
// condition the agent must SEE to self-correct → IsError=true + Content.
// A transport failure / genuine non-2xx is an EXCEPTIONAL condition →
// protocol-level error (non-nil error return, SDK surfaces JSON-RPC -32603).
// The two list tools split exactly along this line.
```

### InputSchema template (required project_id; required pr_number on open_review)
```go
// Source: tasks.go / workspaces.go flat map[string]any template (Phase 07 lock).
// list_pending_reviews / list_recently_reviewed share this shape.
s.AddTool(
	&mcp.Tool{
		Name:        "list_pending_reviews",
		Description: "List PRs awaiting your review for a project. Bridges GET /api/projects/{id}/pull-requests and returns the 'prs' array (the user-review-requested:@me draft:false queue). PRs are per-repo and repos are per-project, so project_id is required. When GitHub integration is off, the project is unlinked, or gh is absent, returns isError=true with the raw Kamacu Result JSON (the 'state' field names the problem); the agent never bypasses the two-gate ladder.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id": map[string]any{
					"type":        "integer",
					"description": "The project id. Required — PRs are per-repo and the endpoint is per-project.",
				},
			},
			"required": []string{"project_id"},
		},
	},
	func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return b.listPendingReviews(ctx, req)
	},
)

// open_review adds pr_number as a second required integer property.
```

### open_review — a one-line bridge.call wrapper (D-05)
```go
// Source: D-05 + bridge.go:88 + moveTask (tasks.go:267) as the POST template.
func (b *bridge) openReview(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		ProjectID int64 `json:"project_id"`
		PRNumber  int64 `json:"pr_number"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("open_review: invalid arguments: %w", err)
	}
	path := fmt.Sprintf("/api/projects/%d/pull-requests/%d/review", args.ProjectID, args.PRNumber)
	return b.call(ctx, http.MethodPost, path, nil) // {task, pr} passthrough; 409/502/400 → D-05 wrap
}
```

### Test: the state→IsError branch (the one new assertion shape)
```go
// Source: workspaces_test.go httptest pattern + D-01/D-02. The fake handler
// returns HTTP 200 with a degraded state — the load-bearing case the standard
// bridge.call assertion CANNOT cover (it would treat 200 as success).
func TestBridge_ListPendingReviews_GhAbsent_IsErrorWithState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK) // ALWAYS 200 — degradation rides in body
		_, _ = w.Write([]byte(`{"state":"no_gh","stale":false,"prs":null,"reviewed":null}`))
	}))
	t.Cleanup(srv.Close)
	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"project_id":1}`))
	res, err := b.listPendingReviews(context.Background(), req)
	if err != nil {
		t.Fatalf("listPendingReviews: %v (degrade is a tool result, not a transport error)", err)
	}
	if !res.IsError {
		t.Fatal("IsError: want true for state:no_gh, got false")
	}
	tc, _ := res.Content[0].(*mcpsdk.TextContent)
	if !strings.Contains(tc.Text, `"state":"no_gh"`) {
		t.Errorf("degrade payload: want raw Result JSON with state, got %q", tc.Text)
	}
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Phase 07: every tool via `bridge.call` (2xx→passthrough, non-2xx→error) | Phase 08: `subscribe` diverges for streaming; Phase 09: list tools diverge for the state-field model (IsError hybrid) | v1.11 (Phases 08/09) | A scoped, documented divergence from `bridge.call` is now an established milestone pattern, not a violation. The invariant is "every tool starts from `b.do`", not "every tool ends at `bridge.call`". |
| MCP SDK `SetError(err)` (overwrites Content with error text) | Direct `IsError=true` + custom Content (D-02 raw JSON) | SDK v1.6.0+ | `SetError` is for the typed-handler path; the low-level `ToolHandler` path (which Phase 06 locked in) can set the field directly. Do NOT use `SetError` for the degrade path — it would clobber the raw Result JSON. |

**Deprecated/outdated:**
- `seterroroverwrite` debug flag (SDK `protocol.go:117-122`): restores pre-1.6.0 behavior where `SetError` always overwrote Content. Will be removed in SDK 1.8.0. Not relevant here (we don't use `SetError`).

## Assumptions Log

> All claims in this research were verified against in-repo source or the SDK module cache this session. No `[ASSUMED]` claims remain — no user confirmation needed before execution.

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| — | (none) | — | — |

**If this table is empty:** All claims were verified or cited — no user confirmation needed.

## Open Questions

1. **`withRecover` on review handlers — yes or no?**
   - What we know: `sessions.go` wraps every handler in `withRecover`; `tasks.go`/`workspaces.go` do not. The review tools are synchronous HTTP bridges (closest analogue: tasks/workspaces), so panic risk is low.
   - What's unclear: Whether the team wants defense-in-depth parity with sessions.
   - Recommendation: Do NOT add `withRecover` (match tasks/workspaces). It's the agent's discretion; adding it is harmless but inconsistent with the closest analogues. (See Pitfall 6.)

2. **Shared `listReviews` helper vs two separate methods?**
   - What we know: Both list tools hit the identical URL and differ only in which queue array to project.
   - Recommendation: Shared helper `listReviews(ctx, projectID, queue)` — the state→IsError logic lives in exactly one place. This is the agent's discretion (D-07); the natural factoring.

## Environment Availability

> This phase has no NEW external dependencies. It reuses the already-running Kamacu HTTP API (loopback) and the already-installed Go toolchain. The MCP subcommand and `internal/github` gh integration are unchanged from v1.3/v1.5.

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Kamacu HTTP API (loopback `127.0.0.1:7333`) | All 3 tools (bridge target) | ✓ (runtime) | v1.11 dev | — (the bridge fails fast with a transport error if down) |
| `gh` CLI | The PR endpoints (transitively) | ✓ (host) | — | Degraded states (`no_gh`) surface as `IsError=true` — graceful by design |
| Go toolchain | Build | ✓ | 1.26 | — |
| `github.com/modelcontextprotocol/go-sdk` | MCP types | ✓ | v1.6.1 (go.mod) | — |

**Missing dependencies with no fallback:** none
**Missing dependencies with fallback:** none

## Security Domain

> This phase adds no new trust boundary, no new auth, no new crypto, and no new untrusted-input parsing beyond what Phases 06/07/08 already established. It is a thin HTTP bridge over already-gated endpoints.

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | Inherits Phase 06 D-06/D-07 (`X-Kamacu-Token` on every call, loopback binding). No new auth. |
| V3 Session Management | no | Not in scope. |
| V4 Access Control | yes (inherited) | The v1.3 two-gate ladder (settings toggle → project link) enforces server-side on every call; the bridge adds NO gate logic and cannot bypass it (SC3). |
| V5 Input Validation | yes | `project_id`/`pr_number` parsed as `int64` via `json.Unmarshal` into a typed struct; `fmt.Sprintf("%d", ...)` into the path cannot inject path segments. No string-to-path interpolation of user input. |
| V6 Cryptography | no | No crypto. |

### Known Threat Patterns for the MCP bridge stack

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Path injection via `project_id`/`pr_number` | Tampering | Args are `int64`; `fmt.Sprintf("%d")` renders canonical decimal — no path-segment injection possible. |
| Bypassing the GitHub gate ladder | Elevation of privilege | Impossible by construction — the gates run server-side in `pullrequests.go` BEFORE any `gh` spawn; the bridge never reaches `gh` when a gate is closed. SC3. |
| Unbounded response body | DoS | `io.LimitReader(resp.Body, maxBodyBytes)` (1 MiB, `bridge.go:23`) caps every read. PR lists are tiny. |
| stdout pollution desyncing JSON-RPC | DoS | `slog` pinned to stderr (Phase 06 D-12); review handlers add no stdout writes. |

## Sources

### Primary (HIGH confidence)
- `internal/api/pullrequests.go` (read in full, 321 lines) — confirmed: GET always returns 200 with degradation in `state`; POST returns 409 (gates) / 502 (gh/worktree) / 200 (`{task, pr}`); `prWire` shape; find-or-create `source='github_pr'` path.
- `internal/github/service.go` (read in full, 178 lines) — confirmed: `Result{State,Stale,FetchedAt,PRs,Reviewed}` wire contract; `resultLocked()` sets arrays from cache independent of state (Pitfall 1 root cause); 60s TTL / 10s floor / maxFailures=3.
- `internal/github/prlist.go` (read in full, 324 lines) — confirmed: both queues from ONE `gh` cycle (`fetchLists`); `searchAwaiting`/`searchReviewed` constants; `PRSummary` has no repo/project field (D-04 rationale).
- `internal/mcp/bridge.go` (read in full, 106 lines) — confirmed: `bridge.do` (X-Kamacu-Token on every call), `bridge.call` (2xx→TextContent, non-2xx→D-05 wrap), `maxBodyBytes = 1<<20`.
- `internal/mcp/server.go` (read in full, 88 lines) — confirmed: `registerTools` is the one-line integration point; `slog`→stderr; stdin-EOF→ExitSuccess.
- `internal/mcp/tasks.go`, `workspaces.go`, `sessions.go` (read in full) — confirmed: per-resource split pattern, flat `map[string]any` InputSchema, `bridge.*` method shape, `withRecover` (sessions only), `subscribe` divergence precedent.
- `internal/mcp/bridge_test.go`, `workspaces_test.go` (read in full) — confirmed: `httptest` + `newCallToolRequest` test pattern, assert on `HTTP NNN` substring + Kamacu message substring.
- `go-sdk@v1.6.1/mcp/protocol.go:92-108` (read from module cache) — **the load-bearing IsError semantics**: tool-level conditions → IsError+Content; exceptional conditions → protocol error. `json:"isError,omitempty"` wire serialization. `SetError` helper overwrites Content (do NOT use for D-02).
- `go-sdk@v1.6.1/mcp/tool.go:17-30` — confirmed: low-level `ToolHandler` returns `(*CallToolResult, error)`; error return = protocol error; result return = tool result. Phase 06 locked the low-level path.
- `go.mod` — confirmed `github.com/modelcontextprotocol/go-sdk v1.6.1`, Go 1.26.
- `.planning/REQUIREMENTS.md` — MCPREV-01/02/03 signatures + Out-of-Scope boundary.
- `.planning/phases/09-pr-review-tools/09-CONTEXT.md` — locked decisions D-01..D-06, discretion areas, canonical refs.

### Secondary (MEDIUM confidence)
- `.planning/STATE.md` — Phase 06/07/08 decision history (bridge.call extraction, full-2xx widening, pointer-field PATCH semantics, subscribe divergence, withRecover placement).

### Tertiary (LOW confidence)
- (none — no WebSearch-only claims)

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all dependencies already in go.mod, verified by reading source files in full.
- Architecture: HIGH — exact in-repo line numbers verified; the pattern is cloned from workspaces.go/tasks.go.
- IsError semantics (the one load-bearing external fact): HIGH — read directly from go-sdk@v1.6.1/mcp/protocol.go in the module cache; aligns with the SDK doc comments and the MCP spec they reference.
- Pitfalls: HIGH — Pitfall 1 (stale-cache) derived from reading `service.go:165-178`; the rest from the established Phase 06/07/08 pattern.

**Research date:** 2026-07-23
**Valid until:** 2026-08-23 (30 days — stable; the SDK and endpoints are unchanged and the phase depends only on them)

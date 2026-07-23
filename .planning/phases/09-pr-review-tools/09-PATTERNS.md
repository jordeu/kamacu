# Phase 09: PR Review Tools - Pattern Map

**Mapped:** 2026-07-23
**Files analyzed:** 3 (1 NEW handler, 1 NEW test, 1 MODIFIED one-line registration)
**Analogs found:** 3 / 3 (every new file has a strong in-repo analog; the one genuinely novel piece — the IsError hybrid — has a partial precedent in Phase 08's `subscribeSessionOutput` divergence)

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `internal/mcp/reviews.go` (NEW) | MCP tool handler (per-resource split) | request-response (HTTP bridge) | `internal/mcp/workspaces.go` + `internal/mcp/tasks.go` + `internal/mcp/sessions.go` (divergence precedent) | exact (openReview) + role-match with scoped-divergence (listReviews) |
| `internal/mcp/reviews_test.go` (NEW) | test (httptest integration) | request-response | `internal/mcp/workspaces_test.go` + `internal/mcp/bridge_test.go` | exact (scaffold) + novel assertion (state→IsError) |
| `internal/mcp/server.go` (MODIFIED — one line) | route registration | n/a | `internal/mcp/server.go:82-87` `registerTools` | exact (the existing function being extended) |

**Scope confirmation:** Phase 09 touches ONLY `internal/mcp/`. ZERO changes to `internal/api/`, `internal/github/`, DB, migrations, or frontend (per CONTEXT `<domain>` and RESEARCH Summary). The endpoints and wire types referenced below are READ-ONLY bridge targets, not files to modify.

## Pattern Assignments

### `internal/mcp/reviews.go` (NEW — MCP tool handler, request-response HTTP bridge)

**Primary analogs:** `internal/mcp/workspaces.go` (per-resource shape + AddTool + InputSchema template), `internal/mcp/tasks.go` (moveTask = POST-with-required-int-args template for `openReview`; listTasks = path-with-project-id for the list tools), `internal/mcp/sessions.go` (the divergence-from-`bridge.call` precedent for `listReviews`).

**File layout (mirror Phase 07 D-07):** one `registerReviewTools(s, b)` function owning 3 `s.AddTool` calls + the 3 `bridge.*` handler methods on `*bridge`. The two list tools share one `listReviews(ctx, projectID, queue)` helper (only `queue` differs: `"prs"` vs `"reviewed"`); `openReview` is a one-line `bridge.call` wrapper.

#### Imports pattern — copy from `workspaces.go:1-11`

```go
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)
```

`listReviews` additionally needs `"io"` (for `io.LimitReader` + `io.ReadAll` — copy from `bridge.go:6`). It does NOT import `kamacu/internal/github` — use a local decode struct (RESEARCH Pitfall 5 / discretion recommendation).

#### `registerReviewTools` registrar — copy the AddTool shape from `workspaces.go:51-68`

```go
// Source: workspaces.go:51-68 (list_workspaces — the simplest registrar entry).
// Each tool is one s.AddTool(&mcp.Tool{Name, Description, InputSchema}, closure).
func registerReviewTools(s *mcp.Server, b *bridge) {
	s.AddTool(
		&mcp.Tool{
			Name:        "list_pending_reviews",
			Description: "...", // lead with what it does; note the bridged endpoint
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id": map[string]any{
						"type":        "integer",
						"description": "The project id. Required — PRs are per-repo and the endpoint is per-project.",
					},
				},
				"required": []string{"project_id"}, // D-04: REQUIRED, not optional
			},
		},
		func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return b.listPendingReviews(ctx, req)
		},
	)
	// ...list_recently_reviewed (identical shape) and open_review (adds pr_number)...
}
```

`open_review` InputSchema adds a second required integer (`pr_number`) — model it on `move_task`'s two-required-args schema at `tasks.go:134-148`.

#### `openReview` handler — the verbatim `bridge.call` POST pattern (D-05) — copy from `tasks.go:267-280` (moveTask)

```go
// Source: tasks.go:267-280 (moveTask — POST with required int args + path
// interpolation; the ONLY adaptation is the path and adding pr_number).
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

**Target endpoint (read-only reference — NOT modified):** `internal/api/pullrequests.go:148-268` POST handler. Returns 200 `{task, pr}` (`writeReview`, `pullrequests.go:308`); 409 on either gate (GATE 1 `:166`, GATE 2 `:175`/`:183`); 502 on `gh`/worktree failure (`:207`, `:246-258`); 400 on invalid PR number. The `{task, pr}` shape is `prWire` at `pullrequests.go:277-287`.

#### `listReviews` shared helper — the IsError hybrid (D-01..D-03)

This is the ONE genuinely novel piece in Phase 09. The closest precedent is `subscribeSessionOutput` in `sessions.go:273+` — it is the **only other handler in the milestone that does NOT delegate the response half to `bridge.call`**. Copy its shape: call `b.do` directly, read the body, build the `*mcp.CallToolResult` yourself instead of delegating.

**Scaffold (b.do + LimitReader + transport-error wrap) — copy verbatim from `bridge.go:88-100`:**

```go
// Source: bridge.go:88-100 (the b.do → defer Close → io.ReadAll(LimitReader) →
// transport-error wrap ("kamacu bridge: %w") → non-2xx wrap ("kamacu %s %s: HTTP %d: %s")
// ladder). listReviews repeats steps 1-3 and then branches instead of returning
// a TextContent passthrough.
func (b *bridge) listReviews(ctx context.Context, projectID int64, queue string) (*mcp.CallToolResult, error) {
	path := fmt.Sprintf("/api/projects/%d/pull-requests", projectID)
	resp, err := b.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("kamacu bridge: %w", err) // transport error → protocol-level
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes)) // maxBodyBytes = 1<<20, bridge.go:23
	if err != nil {
		return nil, fmt.Errorf("kamacu GET %s: read body: %w", path, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Defensive: endpoint always returns 200, but a future 500 still wraps.
		return nil, fmt.Errorf("kamacu GET %s: HTTP %d: %s", path, resp.StatusCode, string(body))
	}
	// === NOVEL STEP (D-01..D-03): parse top-level state, branch on it ===
	// (see "IsError hybrid" under No Analog Found / Shared Patterns below)
}
```

**The branch body is documented in CONTEXT D-01..D-03 and RESEARCH Pattern 2 (lines 192-239).** Planner should reference the full `listReviews` skeleton there — it is already extracted with line-citeable precision and need not be re-derived.

**Target endpoint (read-only reference — NOT modified):** `internal/api/pullrequests.go:40-81` GET list handler. **Always writes HTTP 200** (`writeJSON(w, http.StatusOK, ...)` at `:55`, `:67`, `:71`, `:75`, `:80`); degradation rides the `state` field of `github.Result`. This is the load-bearing fact that forces the divergence.

**Wire type to decode (reference — do NOT import):** `github.Result` at `service.go:19-25`:

```go
// Source: internal/github/service.go:19-25 (READ-ONLY — the contract listReviews inspects)
type Result struct {
	State     string      `json:"state"`     // "ok"|"disabled"|"no_gh"|"auth_required"|"error"
	Stale     bool        `json:"stale"`
	FetchedAt *time.Time  `json:"fetchedAt"`
	PRs       []PRSummary `json:"prs"`       // awaiting-review queue; null when no data
	Reviewed  []PRSummary `json:"reviewed"`  // reviewed-by:@me queue; null when no data
}
```

Use a local struct with `PRs`/`Reviewed` as `json.RawMessage` (RESEARCH Pitfall 5 — preserves the milestone's `internal/mcp` imports-only-stdlib+SDK streak). `PRSummary` shape (`prlist.go:22-34`) is irrelevant to the bridge — the success path re-marshals the chosen queue's RawMessage verbatim.

**CRITICAL pitfall (Pitfall 1):** set `IsError=true` **purely** on `state != "ok"`. Do NOT gate on array nullness — `service.go:165-178` `resultLocked()` sets `PRs`/`Reviewed` from cache independent of state, so a transient `no_gh`/`auth_required`/`error` AFTER a prior success returns non-null arrays + `stale:true`. The `state != "ok"` check is the only correct predicate.

#### `listPendingReviews` / `listRecentlyReviewed` wrappers — copy from `tasks.go:203-211` (getTask arg-parse shape)

```go
// Source: tasks.go:203-211 (getTask — unmarshal int64 arg, delegate to a path-building helper).
func (b *bridge) listPendingReviews(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		ProjectID int64 `json:"project_id"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("list_pending_reviews: invalid arguments: %w", err)
	}
	return b.listReviews(ctx, args.ProjectID, "prs")
}
// listRecentlyReviewed is identical except queue == "reviewed".
```

---

### `internal/mcp/reviews_test.go` (NEW — httptest integration test)

**Primary analogs:** `internal/mcp/workspaces_test.go` (per-resource test layout: one happy + one error per tool), `internal/mcp/bridge_test.go:19-21` (`newCallToolRequest` helper — reuse, do NOT redefine).

#### Test scaffold — copy from `workspaces_test.go:19-53`

```go
// Source: workspaces_test.go:1-13 (imports) + :19-53 (TestBridge_ListWorkspaces_Happy).
package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestBridge_OpenReview_Happy_PostsReviewPath(t *testing.T) {
	var (
		gotPath   string
		gotMethod string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"task":{"id":7},"pr":{"number":42,"title":"fix"}}`))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"project_id":3,"pr_number":42}`))
	// ... assertions on path "/api/projects/3/pull-requests/42/review", method POST,
	// passthrough of the {task, pr} body ...
}
```

**Reuse `newCallToolRequest`** from `bridge_test.go:19-21` — do NOT redefine it (same package, already in scope).

#### Error-case assertion — copy the "HTTP NNN substring" pattern from `workspaces_test.go:95-111` / `:221-243`

```go
// Source: workspaces_test.go:95-111 (CreateWorkspace_DuplicateName_Kamacu409) —
// assert on the status substring ("HTTP 409") AND the Kamacu message substring.
// Apply verbatim to open_review's gate-failure path.
func TestBridge_OpenReview_GateBlocked_Kamacu409(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"GitHub integration is off"}`)) // pullrequests.go:166 shape
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"project_id":3,"pr_number":42}`))
	_, err := b.openReview(context.Background(), req)
	if err == nil {
		t.Fatal("openReview: expected error for 409, got nil")
	}
	if !strings.Contains(err.Error(), "HTTP 409") {
		t.Errorf("openReview error: want substring \"HTTP 409\", got %q", err.Error())
	}
}
```

#### NOVEL assertion — the state→IsError branch (no existing analog; RESEARCH lines 364-389)

This is the one test shape with NO existing analog — `bridge.call` always treats 2xx as success, so no prior test asserts `IsError=true` on a 200 response. The scaffold (httptest server, `newCallToolRequest`, `*bridge` construction) copies `workspaces_test.go`; the assertion body copies RESEARCH's extracted example verbatim:

```go
// Source: 09-RESEARCH.md "Test: the state→IsError branch" (lines 364-389) —
// the load-bearing case the standard bridge.call assertion CANNOT cover.
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

**Per-tool coverage target (07 D-08):** one happy + one representative error per tool = 6 tests minimum. For the list tools, add a success test asserting the projected array (`prs` for pending, `reviewed` for recently-reviewed) — model on `TestBridge_ListWorkspaces_Happy` (`workspaces_test.go:19-53`).

---

### `internal/mcp/server.go` (MODIFIED — one new line in `registerTools`)

**Analog:** the file itself — extend `registerTools` at lines 82-87.

**Edit pattern (copy the existing registrar-call shape):**

```go
// Source: server.go:82-87 (registerTools — flat list of per-resource registrars).
func registerTools(s *mcp.Server, b *bridge) {
	registerTaskTools(s, b)
	registerProjectTools(s, b)
	registerWorkspaceTools(s, b)
	registerSessionTools(s, b)
	registerReviewTools(s, b) // NEW — Phase 09
}
```

That is the ENTIRE change to `server.go`. No other modifications. `ServeCommand.Execute` (`server.go:46-70`) is untouched — Phase 06 already wires `newBridgeFromEnv` + `registerTools` + `srv.Run`.

---

## Shared Patterns

### Bridge call helper (`bridge.call`) — D-05 wrap shape
**Source:** `internal/mcp/bridge.go:88-106`
**Apply to:** `openReview` (and ONLY `openReview` — the two list tools diverge per D-01)

```go
// Source: bridge.go:88-106 — the locked 2xx→passthrough / non-2xx→D-05-wrap helper.
// openReview delegates to this verbatim. The wrap format is:
//   "kamacu %s %s: HTTP %d: %s"  (method, path, status, body-as-string)
// so test assertions match on "HTTP NNN" substrings (workspaces_test.go:108).
func (b *bridge) call(ctx context.Context, method, path string, body io.Reader) (*mcp.CallToolResult, error) {
	resp, err := b.do(ctx, method, path, body)
	if err != nil {
		return nil, fmt.Errorf("kamacu bridge: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("kamacu %s %s: read body: %w", method, path, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("kamacu %s %s: HTTP %d: %s", method, path, resp.StatusCode, string(respBody))
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(respBody)},
		},
	}, nil
}
```

### `b.do` primitive + `X-Kamacu-Token` + `maxBodyBytes`
**Source:** `internal/mcp/bridge.go:23, 60-67`
**Apply to:** `listReviews` (the divergence helper calls `b.do` directly, then re-implements the response half)

```go
// Source: bridge.go:23 (cap) + :60-67 (do). Every bridge HTTP call goes through
// b.do; listReviews calls it directly instead of through bridge.call.
const maxBodyBytes = 1 << 20 // 1 MiB — bridge.go:23

func (b *bridge) do(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, b.base+path, body)
	if err != nil {
		return nil, fmt.Errorf("kamacu bridge: build %s %s: %w", method, path, err)
	}
	req.Header.Set("X-Kamacu-Token", b.token) // D-06/D-07 wire contract
	return b.client.Do(req)
}
```

### InputSchema template — flat `map[string]any`
**Source:** `internal/mcp/workspaces.go:60-63, 75-84, 96-109` and `tasks.go:35-43, 134-148`
**Apply to:** all 3 review tools (project_id required on all; pr_number added on open_review)

```go
// Source: tasks.go:134-148 (move_task — two required args, one with enum).
// Phase 06 lock: use the low-level Server.AddTool with an explicit map[string]any
// InputSchema, NOT the typed generic mcp.AddTool[In,Out] helper.
InputSchema: map[string]any{
	"type": "object",
	"properties": map[string]any{
		"project_id": map[string]any{
			"type":        "integer",
			"description": "...",
		},
		// open_review adds:
		"pr_number": map[string]any{
			"type":        "integer",
			"description": "The PR number.",
		},
	},
	"required": []string{"project_id"},              // list tools
	// "required": []string{"project_id", "pr_number"}, // open_review
},
```

**MUST include `"type": "object"` and a `"properties"` map** (even when empty — `workspaces.go:57-63` `list_workspaces` proves the empty-properties case). AddTool panics otherwise (Phase 06 Pitfall 2).

### Test scaffold — httptest + `newCallToolRequest` + `*bridge` literal
**Source:** `internal/mcp/bridge_test.go:19-21` (helper) + `workspaces_test.go:34-36, 74-75` (bridge literal)
**Apply to:** every test in `reviews_test.go`

```go
// Source: bridge_test.go:19-21 — REUSE this; do not redefine (same package).
func newCallToolRequest(args json.RawMessage) *mcpsdk.CallToolRequest {
	return &mcpsdk.CallToolRequest{Params: &mcpsdk.CallToolParamsRaw{Arguments: args}}
}

// Source: workspaces_test.go:34-36 — the canonical *bridge literal for tests.
b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
```

### `withRecover` — DO NOT apply (open question 1, resolved)
**Source:** `internal/mcp/sessions.go:62-72`
**Apply to:** NONE of the review tools.

`sessions.go` wraps every handler in `withRecover` because session handlers touch streaming/concurrent session state where a panic would desync the JSON-RPC stream. The review tools are **synchronous HTTP bridges** — exact analogues are `tasks.go` and `workspaces.go`, which do NOT use `withRecover`. Match the task/workspace pattern (RESEARCH Open Question 1 / Pitfall 6). Adding it is harmless but inconsistent with the closest analogues.

### Path interpolation safety (V5 input validation)
**Source:** `tasks.go:195, 210, 231, 259, 279, 292` and `workspaces.go:228, 244, 270`
**Apply to:** all 3 review tools

Args are decoded as `int64` via `json.Unmarshal` into a typed struct; the path uses `fmt.Sprintf("%d", ...)`, which renders canonical decimal — no path-segment injection possible (RESEARCH Security Domain V5). Never interpolate raw strings into the path.

---

## No Analog Found

| Component | Role | Reason | Closest Partial Precedent |
|-----------|------|--------|---------------------------|
| `listReviews` state→IsError branch body (D-01..D-03) | bridge response handler | The milestone has NO prior handler that inspects a JSON body field to set `CallToolResult.IsError`. Every prior tool either delegates to `bridge.call` (2xx→success, non-2xx→protocol error) or, in `subscribeSessionOutput`'s case, diverges for **streaming** (a different reason). | `internal/mcp/sessions.go:273+` `subscribeSessionOutput` — same *shape* of divergence (calls `b.do` directly, builds the `*mcp.CallToolResult` itself instead of delegating to `bridge.call`), different *reason* (streaming vs state-field inspection). The b.do + LimitReader + transport-error-wrap scaffold copies from `bridge.go:88-100`; only the post-read branch is novel. |
| Test asserting `IsError=true` on HTTP 200 | test assertion | No existing test asserts IsError on a 2xx response — `bridge.call` would never produce one. | The httptest scaffold + `newCallToolRequest` + substring-match patterns copy from `workspaces_test.go`; the new assertion body is fully extracted in `09-RESEARCH.md` lines 364-389. |

**Planner guidance for the novel piece:** the `listReviews` helper body is already specified line-by-line in `09-RESEARCH.md` Pattern 2 (lines 184-239) — the planner should reference that excerpt directly in the plan action rather than re-deriving it. The Anti-Patterns (`09-RESEARCH.md:241-248`) and Pitfall 1 (stale-cache edge, `:263-268`) are load-bearing implementation constraints — cite them in the plan's verification section.

## Metadata

**Analog search scope:** `/home/jordi/workspace/github/kangent/internal/mcp/` (full directory read: `server.go`, `bridge.go`, `workspaces.go`, `tasks.go`, `sessions.go:45-119`, `workspaces_test.go`, `bridge_test.go`), `/home/jordi/workspace/github/kangent/internal/api/pullrequests.go` (read-only bridge target), `/home/jordi/workspace/github/kangent/internal/github/{service.go, prlist.go:1-50}` (read-only wire types). No `AGENTS.md` present. Skills directory not consulted (no skill matches MCP bridge translation in Go).

**Files scanned:** 9 source files + 2 phase artifacts (CONTEXT, RESEARCH)
**Pattern extraction date:** 2026-07-23
**Confidence:** HIGH — every analog is an in-repo file read in full or in targeted section; the one novel piece (IsError hybrid) is fully specified in RESEARCH with line-citeable excerpts and load-bearing pitfalls.

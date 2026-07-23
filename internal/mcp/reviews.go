// Package mcp — see server.go for the package overview.
//
// reviews.go owns the three PR-review MCP tools (Phase 09 D-07 per-resource
// split): list_pending_reviews, list_recently_reviewed, open_review. These are
// the LAST tools of the v1.11 milestone — pure translation over Kamacu's
// existing v1.3/v1.5 internal/github HTTP surface. ZERO new endpoints, ZERO
// new gh calls, ZERO new gates, ZERO new DB/migration/frontend changes.
//
// Why this file is a sibling of workspaces.go (not a method on bridge.go):
// the per-resource split (07 D-07) keeps each resource's tool registrations
// and handler bodies in one file. reviews.go owns its three AddTool calls +
// the four bridge methods they delegate to (listReviews, listPendingReviews,
// listRecentlyReviewed, openReview).
//
// Design notes (full rationale in 09-CONTEXT.md / 09-RESEARCH.md):
//   - listPendingReviews and listRecentlyReviewed share ONE cached GET call
//     site (listReviews) — the v1.5 no-N+1 invariant is satisfied by
//     construction: one gh cycle per repo serves BOTH review queues.
//   - listReviews is the ONLY handler in the milestone that inspects a JSON
//     body field to set CallToolResult.IsError (the "IsError hybrid",
//     D-01/D-02/D-03). It diverges from bridge.call — modeled on
//     subscribeSessionOutput's scoped divergence (sessions.go) — because the
//     GET endpoint ALWAYS writes HTTP 200 and carries degradation in the
//     `state` field of github.Result. bridge.call would treat that 200 as
//     unconditional success and lose the degrade signal.
//   - openReview is a one-line bridge.call wrapper (D-05). The POST endpoint
//     returns real HTTP error codes (409 gate, 502 gh/worktree failure, 400
//     malformed PR), so bridge.call's non-2xx wrap is the correct degrade
//     surface. The {task, pr} envelope is passed through verbatim.
//
// Clean-import streak (Pitfall 5): this file imports ONLY stdlib + the MCP
// SDK. It does NOT import kamacu/internal/github, kamacu/internal/api, or
// kamacu/internal/store — the state-field decode uses a LOCAL anonymous
// struct (not github.Result) so the milestone's internal/mcp decoupling from
// the domain packages is preserved at the type level.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerReviewTools registers the three PR-review MCP tools (Phase 09 D-07
// per-resource split):
//
//   - list_pending_reviews (MCPREV-01): GET /api/projects/{id}/pull-requests.
//     Returns ONLY the prs array (the user-review-requested:@me draft:false
//     queue) re-marshalled as a single TextContent block on state:ok; on
//     state != ok returns the raw Kamacu Result JSON verbatim with IsError=true.
//   - list_recently_reviewed (MCPREV-02): SAME GET endpoint as above. Returns
//     ONLY the reviewed array (the reviewed-by:@me draft:false queue). Shares
//     the single cached GET call site with list_pending_reviews (one gh cycle
//     serves both queues per repo — the v1.5 no-N+1 invariant).
//   - open_review (MCPREV-03): POST /api/projects/{id}/pull-requests/{n}/review.
//     Returns the raw {task, pr} envelope verbatim. Provisions the review
//     workspace ONLY (no agent auto-start). Degrades via real HTTP error codes
//     (409 gate-blocked, 502 gh/worktree failure, 400 malformed PR number)
//     surfaced through the D-05 bridge.call wrap.
//
// Each tool is a thin HTTP-bridge handler. The list tools delegate to the
// shared listReviews helper (D-01/D-02/D-03 — the state-inspecting branch);
// openReview delegates the response-handling half to bridge.call (Plan 01's
// shared helper, the locked 2xx→passthrough / non-2xx→D-05-wrap contract).
// The bridge performs NO validation — Kamacu's existing handlers and the v1.3
// two-gate ladder (settings toggle → project link) run server-side on every
// call BEFORE any gh spawn. SC3 is satisfied by NOT bypassing those gates:
// the bridge adds NO gate logic of its own.
//
// Input contracts (D-04): project_id (integer) is REQUIRED on all three tools
// (PRs are per-repo and the endpoint is per-project). open_review additionally
// requires pr_number (integer). Omitting a required arg yields the SDK's
// standard validation error BEFORE any HTTP call.
//
// Freshness (D-06): NO tool exposes a refresh/force-fetch parameter. Freshness
// rides the existing per-repo 60s success TTL + 10s attempt floor in
// internal/github.Service (unchanged).
//
// The low-level Server.AddTool is used with an explicit map[string]any
// InputSchema — NOT the typed generic mcp.AddTool[In, Out] helper (Phase 06
// lock; SDK v1.6.1's Tool.InputSchema field is `any`). Every InputSchema MUST
// include "type":"object" and a "properties" map — AddTool panics otherwise
// (Phase 06 Pitfall 2).
func registerReviewTools(s *mcp.Server, b *bridge) {
	// 1. list_pending_reviews (MCPREV-01).
	s.AddTool(
		&mcp.Tool{
			Name:        "list_pending_reviews",
			Description: "List PRs awaiting your review for a project (the review-requested:@me draft:false queue). Bridges GET /api/projects/{id}/pull-requests and returns ONLY the `prs` array as JSON. On degrade (state != ok — GitHub integration off, project unlinked, gh absent, auth required, or transient error) returns the raw Kamacu Result JSON verbatim with IsError=true so the agent can self-correct; never bypasses Kamacu's two-gate ladder (settings toggle → project link). Freshness rides the existing per-repo 60s cache TTL — there is no refresh parameter.",
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

	// 2. list_recently_reviewed (MCPREV-02) — same GET endpoint, projects the
	// `reviewed` array instead of `prs`. Shares ONE cached GET call site with
	// list_pending_reviews via the shared listReviews helper (the v1.5 no-N+1
	// invariant: one gh cycle serves both queues per repo).
	s.AddTool(
		&mcp.Tool{
			Name:        "list_recently_reviewed",
			Description: "List PRs you have already reviewed for a project (the reviewed-by:@me draft:false queue). Bridges the SAME GET /api/projects/{id}/pull-requests endpoint as list_pending_reviews and returns ONLY the `reviewed` array as JSON. Same degrade contract (raw Result JSON + IsError=true on state != ok) and same freshness model (60s per-repo cache TTL, no refresh parameter).",
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
			return b.listRecentlyReviewed(ctx, req)
		},
	)

	// 3. open_review (MCPREV-03) — POST /api/projects/{id}/pull-requests/{n}/review.
	// Provisions a detached PR-head worktree for the review and returns the raw
	// {task, pr} envelope (the agent is NOT auto-started; the workspace is
	// provisioned only). Degrades via real HTTP error codes through the D-05
	// bridge.call wrap: 409 gate-blocked (GitHub integration off OR project
	// unlinked), 502 gh/worktree failure, 400 malformed PR number.
	s.AddTool(
		&mcp.Tool{
			Name:        "open_review",
			Description: "Open a PR as a review workspace for a project. Bridges POST /api/projects/{id}/pull-requests/{pr_number}/review and returns the raw {task, pr} JSON envelope verbatim (the live gh pr view detail, so the header/body never drift from GitHub). Provisions a detached PR-head worktree ONLY — the agent is NOT auto-started. Degrades via real HTTP error codes: 409 when Kamacu's two-gate ladder blocks (GitHub integration off OR project not linked to a repo), 502 on gh/worktree failure, 400 on a malformed PR number.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id": map[string]any{
						"type":        "integer",
						"description": "The project id. Required — PRs are per-repo and the endpoint is per-project.",
					},
					"pr_number": map[string]any{
						"type":        "integer",
						"description": "The PR number.",
					},
				},
				"required": []string{"project_id", "pr_number"}, // D-04: BOTH required
			},
		},
		func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return b.openReview(ctx, req)
		},
	)
}

// listReviews is the shared state-inspecting helper that contains ALL of the
// D-01/D-02/D-03 logic for both list_pending_reviews and list_recently_reviewed.
// The only difference between those two tools is which queue is projected on
// success (queue == "reviewed" → res.Reviewed; else res.PRs).
//
// This is the ONLY handler in the milestone that diverges from bridge.call to
// inspect a JSON body field — modeled on subscribeSessionOutput's scoped
// divergence (sessions.go). The GET endpoint (internal/api/pullrequests.go:40-81)
// ALWAYS writes HTTP 200 and carries degradation in the `state` field of
// github.Result; bridge.call would treat that 200 as unconditional success and
// lose the degrade signal (state:"disabled" / "no_gh" / "auth_required" /
// "error" would be passed through as if healthy).
//
// D-01 (IsError hybrid) / D-02 (raw Result JSON on degrade) / D-03 (per-tool
// array projection):
//   - On state == "ok": return the chosen queue's array re-marshalled as a
//     single TextContent block, IsError=false (the SDK serializes the omitted
//     zero value per protocol.go omitempty).
//   - On state != "ok": return the raw Kamacu Result JSON verbatim as a single
//     TextContent block with IsError=true. The Content is the raw body — NO
//     prose prefix, NO state→message map. The state field names the problem so
//     the agent can self-correct; IsError is the actionable signal.
//
// Pitfall 1 (stale-cache edge, load-bearing): IsError is set PURELY on
// state != "ok" — NEVER gated on prs/reviewed array nullness. internal/github
// service.go resultLocked() sets PRs/Reviewed from cache INDEPENDENTLY of
// state, so a transient no_gh/auth_required/error AFTER a prior success
// returns NON-NULL arrays + stale:true. A `if prs == nil { IsError=true }`
// predicate would let that degrade slip through as IsError=false. The
// state-field check is the only correct predicate. The dedicated regression
// test TestBridge_ListPendingReviews_StateErrorWithNonNullArrays_IsErrorStillTrue
// pins this invariant.
//
// Body-read ladder (copied verbatim from bridge.go:88-100): b.do → defer Close
// → io.ReadAll(LimitReader(maxBodyBytes)) → transport-error wrap
// ("kamacu bridge: %w") → non-2xx wrap ("kamacu GET %s: HTTP %d: %s"). The
// non-2xx branch is defensive — the endpoint always returns 200 today, but a
// future 500 still surfaces as a wrapped transport error rather than a silent
// misparse.
//
// Pitfall 5 (clean-import streak): the state decode uses a LOCAL anonymous
// struct, NOT github.Result. PRs/Reviewed are decoded as json.RawMessage so
// the chosen queue can be re-marshalled verbatim without a round-trip through
// a typed PRSummary (whose shape is irrelevant to the bridge).
func (b *bridge) listReviews(ctx context.Context, projectID int64, queue string) (*mcp.CallToolResult, error) {
	path := fmt.Sprintf("/api/projects/%d/pull-requests", projectID)
	resp, err := b.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		// Transport error (Kamacu down, connection refused) → wrap as a
		// protocol-level error; the SDK surfaces a JSON-RPC -32603. Never a
		// silent success.
		return nil, fmt.Errorf("kamacu bridge: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes)) // maxBodyBytes = 1<<20, bridge.go:23
	if err != nil {
		return nil, fmt.Errorf("kamacu GET %s: read body: %w", path, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Defensive — endpoint always returns 200 today, but a future 500
		// still wraps as a transport error rather than misparses.
		return nil, fmt.Errorf("kamacu GET %s: HTTP %d: %s", path, resp.StatusCode, string(body))
	}
	// === NOVEL STEP (D-01/D-02/D-03): decode the top-level state field and
	// branch on it. github.Result decoded into a LOCAL struct (Pitfall 5 — NOT
	// github.Result, so internal/mcp stays decoupled from internal/github). ===
	var res struct {
		State    string          `json:"state"`              // "ok"|"disabled"|"no_gh"|"auth_required"|"error"
		Stale    bool            `json:"stale"`              // true iff a prior-success cache is being served under a transient error
		PRs      json.RawMessage `json:"prs"`                // awaiting-review queue; null/absent when no data
		Reviewed json.RawMessage `json:"reviewed"`           // reviewed-by:@me queue; null/absent when no data
	}
	if err := json.Unmarshal(body, &res); err != nil {
		// Shape surprise (Kamacu returned a non-Result body under a 200 — e.g.
		// a future HTML error page from a misconfigured proxy). Surface it
		// faithfully as a wrapped error so the agent sees something rather
		// than a silent misparse; never fake success.
		return nil, fmt.Errorf("kamacu GET %s: parse result: %w", path, err)
	}
	// D-01/D-02: state != "ok" → IsError=true with the raw Result JSON verbatim.
	// Pitfall 1: IsError is set PURELY on state != "ok" — do NOT gate on array
	// nullness here. A stale-cache degrade (transient error AFTER a prior
	// success) carries non-null arrays + stale:true but is still an error.
	if res.State != "ok" {
		return &mcp.CallToolResult{
			// D-01: IsError is set as a struct field directly. D-02 requires the
			// raw Kamacu Result JSON to be preserved verbatim in Content; no
			// result-mutator helper is used (it would clobber the raw body).
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: string(body)}, // raw Kamacu Result JSON verbatim (D-02)
			},
		}, nil
	}
	// D-03 success branch: pick the queue and return it re-marshalled as a
	// single TextContent block. IsError stays false (zero value, omitted by
	// the SDK's protocol.go omitempty serialization).
	raw := res.PRs
	if queue == "reviewed" {
		raw = res.Reviewed
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(raw)},
		},
	}, nil
}

// listPendingReviews is the body of the list_pending_reviews tool handler
// (MCPREV-01). It parses the required project_id argument and delegates to
// the shared listReviews helper with queue="prs" (the awaiting-review queue).
//
// Argument shape mirrors tasks.go:203-211 (getTask — unmarshal int64 arg,
// delegate to a path-building helper). project_id is REQUIRED per the
// InputSchema; the SDK validates required-arg presence BEFORE this handler
// runs, so a missing project_id surfaces as the SDK's standard validation
// error, not a handler-level fmt.Errorf.
func (b *bridge) listPendingReviews(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		ProjectID int64 `json:"project_id"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("list_pending_reviews: invalid arguments: %w", err)
	}
	return b.listReviews(ctx, args.ProjectID, "prs")
}

// listRecentlyReviewed is the body of the list_recently_reviewed tool handler
// (MCPREV-02). Identical to listPendingReviews except queue="reviewed" (the
// reviewed-by:@me queue). Shares the single cached GET call site — the v1.5
// no-N+1 invariant: one gh cycle serves both queues per repo.
func (b *bridge) listRecentlyReviewed(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		ProjectID int64 `json:"project_id"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("list_recently_reviewed: invalid arguments: %w", err)
	}
	return b.listReviews(ctx, args.ProjectID, "reviewed")
}

// openReview is the body of the open_review tool handler (MCPREV-03). It
// POSTs to /api/projects/{id}/pull-requests/{n}/review and returns the raw
// {task, pr} envelope verbatim via bridge.call (D-05 — the locked 2xx→
// passthrough / non-2xx→wrap contract).
//
// Argument shape mirrors tasks.go:267-280 (moveTask — POST with required int
// args + path interpolation). The int64 args + fmt.Sprintf("%d", ...) path
// rendering is the V5 path-injection mitigation: canonical decimal, so no
// path-segment injection is possible regardless of agent-CLI input.
//
// Degrade surface (D-05): Kamacu's POST handler returns real HTTP error codes
// that bridge.call wraps faithfully:
//   - 409 gate-blocked (GitHub integration off OR project not linked —
//     pullrequests.go:166/:175/:183).
//   - 502 on gh/worktree failure (pullrequests.go:207/:246-258).
//   - 400 on a malformed PR number (pullrequests.go:155).
// Each surfaces as a non-nil error wrapping
// `kamacu POST /api/projects/{id}/pull-requests/{n}/review: HTTP NNN: <body>`,
// which the SDK turns into a JSON-RPC -32603. The bridge adds NO gate logic of
// its own and CANNOT reach gh when a gate is closed (the two-gate ladder runs
// server-side BEFORE any gh spawn — SC3 satisfied by NOT bypassing).
func (b *bridge) openReview(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		ProjectID int64 `json:"project_id"`
		PRNumber  int64 `json:"pr_number"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("open_review: invalid arguments: %w", err)
	}
	path := fmt.Sprintf("/api/projects/%d/pull-requests/%d/review", args.ProjectID, args.PRNumber)
	return b.call(ctx, http.MethodPost, path, nil) // D-05: verbatim bridge.call — {task,pr} passthrough; 409/502/400 → wrap
}

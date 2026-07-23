---
phase: 09-pr-review-tools
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/mcp/reviews.go
  - internal/mcp/reviews_test.go
  - internal/mcp/server.go
autonomous: true
requirements:
  - MCPREV-01
  - MCPREV-02
  - MCPREV-03
user_setup: []

must_haves:
  truths:
    # Success-path behaviors (SC1/SC2)
    - "Calling list_pending_reviews(project_id) hits GET /api/projects/{id}/pull-requests and, on state:ok, returns ONLY the prs array (re-marshalled) as a single TextContent block with IsError=false (D-03). [MCPREV-01, D-01, D-03]"
    - "Calling list_recently_reviewed(project_id) hits the SAME GET endpoint and, on state:ok, returns ONLY the reviewed array (re-marshalled) as a single TextContent block with IsError=false (D-03). [MCPREV-02, D-01, D-03]"
    - "Calling open_review(project_id, pr_number) hits POST /api/projects/{id}/pull-requests/{n}/review and returns the raw {task, pr} envelope as TextContent with IsError=false (D-05 — bridge.call passthrough). [MCPREV-03]"
    - "Both list tools share ONE cached GET call site — a single listReviews(ctx, projectID, queue) helper — so the v1.5 no-N+1 invariant is satisfied by construction (one gh cycle serves both queues per repo). [SC1]"
    # Degradation-path behaviors (SC3)
    - "When Kamacu's GET returns HTTP 200 with state != ok (disabled / no_gh / auth_required / error), both list tools return CallToolResult{IsError:true, Content:[TextContent{raw Kamacu Result JSON verbatim}]}, NOT a protocol-level error and NOT a stripped success (D-01/D-02). [SC3]"
    - "IsError is set PURELY on state != ok — never gated on prs/reviewed array nullness. A stale-cache degrade (transient no_gh/error AFTER a prior success carrying non-null arrays + stale:true) still surfaces IsError=true. [Pitfall 1]"
    - "When open_review hits Kamacu's POST and the two-gate ladder blocks (409), or gh/worktree fails (502), or PR number is malformed (400), the bridge surfaces a D-05 wrapped error string of the form `kamacu POST /api/projects/{id}/pull-requests/{n}/review: HTTP NNN: <body>` (non-nil err return → JSON-RPC -32603). The bridge adds NO gate logic of its own and cannot reach gh when a gate is closed. [SC3, D-05]"
    - "A transport failure (Kamacu down, connection refused) on any tool returns a non-nil error wrapping `kamacu bridge: ...` so the SDK surfaces a JSON-RPC error, never a silent success."
    # Input contract (D-04 / D-06)
    - "All three tools declare project_id (integer) in their InputSchema's required array; open_review additionally declares pr_number (integer) as required. Omitting them yields the SDK's standard validation error BEFORE any HTTP call (D-04). The REQUIREMENTS.md `?` on the list signatures is intentionally dropped. [D-04]"
    - "No tool exposes a refresh / force-fetch parameter (D-06) — freshness rides the existing per-repo 60s success TTL + 10s attempt floor in internal/github.Service."
    # Integrity invariants
    - "internal/mcp/reviews.go imports ONLY stdlib (context, encoding/json, fmt, io, net/http) + github.com/modelcontextprotocol/go-sdk/mcp. It does NOT import kamacu/internal/github, kamacu/internal/store, kamacu/internal/api, or any other internal/* package (Pitfall 5 — preserves the milestone's clean-import streak)."
    - "internal/mcp/reviews.go writes NOTHING to os.Stdout — slog stays on stderr (Phase 06 D-12 invariant intact); no fmt.Print* calls."
    - "registerTools in internal/mcp/server.go contains exactly one new line `registerReviewTools(s, b)` alongside the four existing registrars (07 D-07 per-resource split)."
  artifacts:
    - "internal/mcp/reviews.go exists with: registerReviewTools(s *mcp.Server, b *bridge); 4 methods on *bridge: listReviews(ctx, projectID int64, queue string), listPendingReviews(ctx, req), listRecentlyReviewed(ctx, req), openReview(ctx, req); 3 s.AddTool calls for list_pending_reviews / list_recently_reviewed / open_review."
    - "internal/mcp/reviews_test.go exists with at least 6 tests covering: happy + error per tool (07 D-08), including the state→IsError branch (a fake handler returning HTTP 200 with state:no_gh body) and the open_review 409 D-05 wrap."
    - "internal/mcp/server.go contains registerReviewTools(s, b) as the last call inside registerTools (after registerSessionTools)."
  key_links:
    - "registerReviewTools(s, b) is called from registerTools in server.go — the one-line integration point. Without this line, the 3 tools are invisible to tools/list."
    - "Both list tools delegate to listReviews(ctx, projectID, queue) — the single state-inspecting helper that contains ALL of D-01/D-02/D-03 logic. If the helper is wrong, both tools are wrong."
    - "openReview delegates to bridge.call (the locked Phase 06/07 helper) — it must NOT reimplement the response half."
  prohibitions:
    - statement: "Do NOT gate IsError on prs/reviewed array nullness (Pitfall 1 stale-cache edge)."
      status: resolved
      verification: "TestBridge_ListPendingReviews_StateErrorWithNonNullArrays_IsErrorStillTrue covers the stale-cache shape (state:no_gh + non-null prs/reviewed + stale:true) and asserts IsError=true."
    - statement: "Do NOT add a refresh/force-fetch parameter to any tool (D-06)."
      status: resolved
      verification: "grep on reviews.go InputSchema maps shows zero 'refresh' keys."
    - statement: "Do NOT import internal/github, internal/api, or internal/store from reviews.go (Pitfall 5)."
      status: resolved
      verification: "go list -deps or grep of reviews.go import block shows only stdlib + the MCP SDK package."
    - statement: "Do NOT use SetError for the list-tool degrade path — it would clobber the raw Result JSON D-02 mandates."
      status: resolved
      verification: "grep on reviews.go for SetError returns zero matches; IsError is set as a struct field directly."
---

<objective>
Ship the last 3 MCP tools of v1.11 — list_pending_reviews, list_recently_reviewed,
open_review — by bridging Kamacu's existing v1.3/v1.5 internal/github HTTP surface.
An agent inside a Kamacu task PTY can triage the user's PR review load (PRs awaiting
review + PRs already reviewed) and open a review workspace, reusing the existing
gh integration with NO new gh calls, NO new gates, NO new endpoints. This is the
milestone's mechanical coda: pure translation over endpoints that already exist.

Purpose: Close MCPREV-01/02/03. With this plan, every Kamacu surface the milestone
targets (tasks / projects / workspaces / sessions / reviews) is reachable from an
agent CLI via a tool call.

Output:
  - internal/mcp/reviews.go        (NEW — 3 tools, registrar, listReviews helper, openReview)
  - internal/mcp/reviews_test.go   (NEW — happy + error per tool, incl. state→IsError + 409)
  - internal/mcp/server.go         (MODIFIED — one new line: registerReviewTools(s, b))
</objective>

execution_context: |
  @/home/jordi/.config/opencode/gsd-core/workflows/execute-plan.md
  @/home/jordi/.config/opencode/gsd-core/templates/summary.md

context: |
  @.planning/PROJECT.md
  @.planning/ROADMAP.md
  @.planning/STATE.md
  @.planning/phases/09-pr-review-tools/09-CONTEXT.md
  @.planning/phases/09-pr-review-tools/09-RESEARCH.md
  @.planning/phases/09-pr-review-tools/09-PATTERNS.md

  # Phase 06/07/08 patterns Phase 09 clones — read for the exact shapes
  @internal/mcp/server.go
  @internal/mcp/bridge.go
  @internal/mcp/workspaces.go
  @internal/mcp/workspaces_test.go
  @internal/mcp/bridge_test.go

  # Bridge targets (READ-ONLY — NOT modified):
  @internal/api/pullrequests.go
  @internal/github/service.go

tasks:

<task type="auto" tdd="false">
  <name>Task 1: Create internal/mcp/reviews.go with 3 review tools handlers + registerReviewTools wiring</name>
  <files>internal/mcp/reviews.go, internal/mcp/server.go</files>

  <read_first>
    - internal/mcp/server.go (current registerTools body — confirms the 4 existing registrars and the integration point at line 82-87)
    - internal/mcp/bridge.go (confirms bridge.do at :60, bridge.call at :88, maxBodyBytes=1<<20 at :23, the X-Kamacu-Token header at :65 — listReviews calls b.do directly and reuses maxBodyBytes)
    - internal/mcp/workspaces.go (per-resource shape template: registerXTools(s, b) with s.AddTool(&mcp.Tool{Name,Description,InputSchema}, closure) at :51-166, plus (*bridge).method bodies at :168-271; openReview's structure mirrors updateWorkspace/deleteWorkspace's int64-arg + path + delegate-to-bridge.call shape)
    - internal/mcp/sessions.go (the divergence-from-bridge.call precedent — subscribeSessionOutput at :273 calls b.do via subscribeClient and builds its own *mcp.CallToolResult instead of using bridge.call; listReviews follows the same shape of scoped divergence for a different reason — the state-field model)
    - internal/api/pullrequests.go (READ-ONLY bridge target — confirms GET .../pull-requests at :40-81 ALWAYS writes 200 with github.Result{State,...} regardless of gate state; POST .../review at :148-268 returns 200 {task, pr} via writeReview :308, 409 on either gate at :166/:175/:183, 502 on gh/worktree failure at :207/:246-258, 400 on malformed PR number at :155)
    - internal/github/service.go (READ-ONLY wire contract — Result struct at :19-25 with State string `json:"state"` + PRs/Reviewed slices; the State enum "ok"|"disabled"|"no_gh"|"auth_required"|"error" documented in the struct comment; resultLocked at :165-178 is the Pitfall 1 root cause — arrays set from cache INDEPENDENT of state)
    - .planning/phases/09-pr-review-tools/09-RESEARCH.md (Pattern 2 lines 184-239 — the FULL listReviews skeleton with line-citeable precision; Anti-Patterns at :241-248; Pitfall 1 at :263-268; Code Examples at :299-360)
    - .planning/phases/09-pr-review-tools/09-PATTERNS.md (file classification table; reviews.go layout at :19-160; the listReviews scaffold excerpt at :100-123; openReview excerpt at :76-90; the list wrapper shape at :148-160)
  </read_first>

  <behavior>
    - list_pending_reviews happy path: project_id=1, fake GET returns 200 {"state":"ok","stale":false,"prs":[{...}],"reviewed":[{...}]} → result.Content[0] is the prs array re-marshalled, IsError=false
    - list_pending_reviews degrade: project_id=1, fake GET returns 200 {"state":"no_gh","stale":false,"prs":null,"reviewed":null} → IsError=true, Content is the raw body verbatim
    - list_pending_reviews stale-cache degrade: project_id=1, fake GET returns 200 {"state":"no_gh","stale":true,"prs":[{...}],"reviewed":[{...}]} → IsError=true (NOT false), Content is the raw body verbatim (Pitfall 1)
    - list_recently_reviewed happy path: identical to list_pending_reviews but Content is the reviewed array
    - open_review happy path: project_id=3, pr_number=42, fake POST returns 200 {"task":{"id":7},"pr":{"number":42}} → Content is the raw body verbatim, IsError=false, request method is POST, request path is /api/projects/3/pull-requests/42/review
    - open_review gate-blocked: fake POST returns 409 {"error":"GitHub integration is off"} → returns non-nil error containing the substring "HTTP 409"
  </behavior>

  <action>
    Create internal/mcp/reviews.go as a new file in package mcp, mirroring the per-resource split shape of workspaces.go (07 D-07). The file contains exactly:

    (a) A package-level doc comment explaining the file's role: 3 MCP tools bridging Kamacu's existing v1.3/v1.5 internal/github HTTP surface, ZERO endpoint/DB/migration changes, listPendingReviews/listRecentlyReviewed share one listReviews helper, openReview is a one-line bridge.call wrapper.

    (b) Imports: copy the workspaces.go:1-11 set — "context", "encoding/json", "fmt", "io", "net/http", "github.com/modelcontextprotocol/go-sdk/mcp". ADD "io" (needed for io.LimitReader + io.ReadAll in listReviews — copy from bridge.go:6). Do NOT import kamacu/internal/github (Pitfall 5 — decode into a local struct), kamacu/internal/api, or kamacu/internal/store.

    (c) registerReviewTools(s *mcp.Server, b *bridge) owning 3 s.AddTool calls. Copy the AddTool shape verbatim from workspaces.go:51-68. The three tools:

      1. list_pending_reviews — Description (per CONTEXT specifics): lead with what it does, note the bridged endpoint (GET /api/projects/{id}/pull-requests), note the projected array (the user-review-requested:@me draft:false queue), note that project_id is required because PRs are per-repo and repos are per-project, note the degrade contract (IsError=true + raw Result JSON when state != ok, never bypasses the two-gate ladder). InputSchema is map[string]any with "type":"object", a "properties" map containing project_id:{type:"integer", description:"The project id. Required — PRs are per-repo and the endpoint is per-project."}, and "required":[]string{"project_id"} (D-04 — REQUIRED, not optional). The handler closure delegates to b.listPendingReviews(ctx, req).

      2. list_recently_reviewed — identical shape except Name="list_recently_reviewed", Description notes the reviewed array (the reviewed-by:@me draft:false queue), same InputSchema, handler delegates to b.listRecentlyReviewed(ctx, req).

      3. open_review — Description notes it opens a PR as a review workspace by bridging POST /api/projects/{id}/pull-requests/{n}/review, returns the raw {task, pr} envelope, provisions the workspace only (no agent auto-start), degrades via real HTTP error codes (409 gate, 502 gh/worktree failure, 400 malformed PR). InputSchema adds a second required property pr_number:{type:"integer", description:"The PR number."}, so "required":[]string{"project_id","pr_number"}. Handler delegates to b.openReview(ctx, req).

    Every InputSchema MUST include "type":"object" and a "properties" map — AddTool panics otherwise (Phase 06 Pitfall 2). Use the low-level Server.AddTool with an explicit map[string]any — NOT the typed generic mcp.AddTool helper.

    (d) listReviews is the shared helper containing ALL of D-01/D-02/D-03. Signature: func (b *bridge) listReviews(ctx context.Context, projectID int64, queue string) (*mcp.CallToolResult, error). Body — copy the b.do + LimitReader + transport-error-wrap ladder verbatim from bridge.go:88-100 (the same scaffold bridge.call uses), then add the novel branch documented in 09-RESEARCH.md Pattern 2 lines 192-239:

      - path := fmt.Sprintf("/api/projects/%d/pull-requests", projectID)
      - resp, err := b.do(ctx, http.MethodGet, path, nil); on err return nil, fmt.Errorf("kamacu bridge: %w", err)
      - defer resp.Body.Close()
      - body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes)); on err return nil, fmt.Errorf("kamacu GET %s: read body: %w", path, err)
      - if resp.StatusCode < 200 || resp.StatusCode >= 300 → return nil, fmt.Errorf("kamacu GET %s: HTTP %d: %s", path, resp.StatusCode, string(body)) (defensive — endpoint always returns 200, but a future 500 still wraps)
      - Decode into a LOCAL anonymous struct (NOT github.Result — Pitfall 5):
          var res struct {
              State    string          `json:"state"`
              Stale    bool            `json:"stale"`
              PRs      json.RawMessage `json:"prs"`
              Reviewed json.RawMessage `json:"reviewed"`
          }
        On json.Unmarshal error → return nil, fmt.Errorf("kamacu GET %s: parse result: %w", path, err) (shape surprise surfaces faithfully, not faked)
      - D-01/D-02 branch: if res.State != "ok" → return &mcp.CallToolResult{IsError:true, Content:[]mcp.Content{&mcp.TextContent{Text:string(body)}}}, nil. DO NOT gate on array nullness (Pitfall 1) — set IsError=true PURELY on state != ok. The Content is the raw Kamacu Result JSON verbatim (D-02 — no prose prefix, no state→message map).
      - D-03 success branch: pick the queue — raw := res.PRs; if queue == "reviewed" { raw = res.Reviewed }. Return &mcp.CallToolResult{Content:[]mcp.Content{&mcp.TextContent{Text:string(raw)}}}, nil (IsError stays false by zero value — SDK serializes the field as omitted per protocol.go:108 omitempty).

    (e) listPendingReviews and listRecentlyReviewed wrappers — copy the getTask arg-parse shape from tasks.go:203-211. Each is a 5-line method: unmarshal req.Params.Arguments into a struct{ProjectID int64 `json:"project_id"`}, on error return nil, fmt.Errorf("<tool_name>: invalid arguments: %w", err), then return b.listReviews(ctx, args.ProjectID, "prs") for pending and "reviewed" for recently-reviewed.

    (f) openReview — copy the moveTask POST-with-required-int-args shape from tasks.go:267-280. Signature: func (b *bridge) openReview(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error). Body:
      - var args struct { ProjectID int64 `json:"project_id"`; PRNumber int64 `json:"pr_number"` }
      - json.Unmarshal(req.Params.Arguments, &args); on err return nil, fmt.Errorf("open_review: invalid arguments: %w", err)
      - path := fmt.Sprintf("/api/projects/%d/pull-requests/%d/review", args.ProjectID, args.PRNumber)
      - return b.call(ctx, http.MethodPost, path, nil) (D-05 — verbatim bridge.call; 200→{task,pr} passthrough; 409/502/400 → D-05 wrap)
    The int64 args + fmt.Sprintf("%d", ...) rendering is the V5 path-injection mitigation (RESEARCH Security Domain — canonical decimal, no path-segment injection possible).

    Do NOT wrap any review handler in withRecover (RESEARCH Open Question 1 / Pitfall 6 — review tools are synchronous HTTP bridges; withRecover is for sessions.go's streaming/concurrent session state where a panic would desync the JSON-RPC stream. tasks.go and workspaces.go do NOT use withRecover; match them).

    Then EDIT internal/mcp/server.go: add exactly one new line inside registerTools (the function at lines 82-87): `registerReviewTools(s, b)` as the LAST call after registerSessionTools(s, b), with a trailing `// NEW — Phase 09` comment. Do NOT modify any other line in server.go.

    Verify the file compiles and the new tools register: run `go build ./internal/mcp/...` and `go vet ./internal/mcp/...`.
  </action>

  <verify>
    <automated>cd /home/jordi/workspace/github/kangent && go build ./internal/mcp/... && go vet ./internal/mcp/...</automated>
  </verify>

  <acceptance_criteria>
    - File internal/mcp/reviews.go exists in package mcp.
    - go build ./internal/mcp/... exits 0.
    - go vet ./internal/mcp/... exits 0.
    - reviews.go contains the symbols: func registerReviewTools(s *mcp.Server, b *bridge); func (b *bridge) listReviews(ctx context.Context, projectID int64, queue string) (*mcp.CallToolResult, error); func (b *bridge) listPendingReviews(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error); func (b *bridge) listRecentlyReviewed(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error); func (b *bridge) openReview(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error).
    - reviews.go contains exactly three s.AddTool calls with Names "list_pending_reviews", "list_recently_reviewed", "open_review".
    - The InputSchema for both list tools has "required":[]string{"project_id"} (D-04 — REQUIRED).
    - The InputSchema for open_review has "required":[]string{"project_id","pr_number"}.
    - The listReviews body contains the literal branch `res.State != "ok"` returning IsError:true (D-01).
    - The listReviews body does NOT contain any array-nullness check gating IsError (Pitfall 1 — e.g. no `res.PRs == nil` predicate near the IsError decision).
    - The listReviews success branch picks res.Reviewed when queue == "reviewed", else res.PRs (D-03).
    - openReview's final line is `return b.call(ctx, http.MethodPost, path, nil)` (D-05 — verbatim bridge.call).
    - server.go's registerTools now contains registerReviewTools(s, b) as its final call.
    - `go list -f '{{ join .Imports "\n" }}' ./internal/mcp 2>/dev/null | grep -c 'kamacu/internal/github'` returns 0 (Pitfall 5 — no coupling to the domain package; same for internal/api and internal/store).
    - `grep -c 'SetError' internal/mcp/reviews.go` returns 0 (the IsError field is set as a struct field, never via SetError).
    - `grep -cE 'fmt\.Print(ln|f)?\(' internal/mcp/reviews.go` returns 0 (Phase 06 D-12 — no stdout pollution).
    - No review handler is wrapped in withRecover.
  </acceptance_criteria>

  <done>
    reviews.go compiles, vets clean, exposes registerReviewTools + 4 bridge methods + 3 AddTool registrations; server.go has the one-line integration; the listReviews helper sets IsError purely on state != "ok" and projects the chosen queue on success; openReview delegates verbatim to bridge.call; reviews.go imports only stdlib + the SDK; the package's clean-import streak (no internal/github, internal/api, internal/store) is intact.
  </done>
</task>

<task type="auto" tdd="false">
  <name>Task 2: Create internal/mcp/reviews_test.go with happy + error coverage per tool (incl. state→IsError and 409 wrap)</name>
  <files>internal/mcp/reviews_test.go</files>

  <read_first>
    - internal/mcp/reviews.go (the file Task 1 produced — confirms exact method names and the IsError-branch shape the tests must exercise)
    - internal/mcp/workspaces_test.go (the per-resource test template — TestBridge_ListWorkspaces_Happy at :19-53, TestBridge_CreateWorkspace_DuplicateName_Kamacu409 at :95-111 — copy the httptest.NewServer + *bridge literal + newCallToolRequest pattern verbatim)
    - internal/mcp/bridge_test.go:19-21 (the newCallToolRequest helper — REUSE it, do NOT redefine; same package so it is in scope)
    - .planning/phases/09-pr-review-tools/09-RESEARCH.md (the state→IsError test excerpt at lines 364-389 — the ONE novel assertion shape with no existing analog; the per-tool coverage target at :264)
    - .planning/phases/09-pr-review-tools/09-PATTERNS.md (the test scaffold + novel-assertion excerpts at :164-263; the open_review happy + 409 scaffolds at :186-231)
  </read_first>

  <behavior>
    - TestBridge_ListPendingReviews_Happy: fake GET returns 200 state:ok with both queues populated; assert IsError=false and Content is the prs array only (D-03 success projection).
    - TestBridge_ListPendingReviews_StateNoGh_IsErrorWithRawResult: fake GET returns 200 with state:no_gh body; assert IsError=true and Content contains "state":"no_gh" (D-01/D-02).
    - TestBridge_ListPendingReviews_StateErrorWithNonNullArrays_IsErrorStillTrue (PITFALL 1 STALE-CACHE EDGE — load-bearing): fake GET returns 200 with state:error AND non-null prs/reviewed AND stale:true; assert IsError=true (NOT false — never gate on array nullness).
    - TestBridge_ListRecentlyReviewed_Happy: fake GET returns 200 state:ok with both queues populated; assert IsError=false and Content is the reviewed array only (proves the queue parameter routes to the right RawMessage).
    - TestBridge_OpenReview_Happy_PostsReviewPath: fake POST returns 200 {task,pr}; assert request method POST, request path /api/projects/{id}/pull-requests/{n}/review, Content is the raw body passthrough, IsError=false.
    - TestBridge_OpenReview_GateBlocked_Kamacu409: fake POST returns 409 {"error":"GitHub integration is off"}; assert non-nil error containing substring "HTTP 409".
  </behavior>

  <action>
    Create internal/mcp/reviews_test.go in package mcp (NOT _test — same package so it can call the unexported bridge methods and reuse newCallToolRequest from bridge_test.go:19-21). Copy the imports block from workspaces_test.go:1-13: "context", "encoding/json", "net/http", "net/http/httptest", "strings", "testing", "time", and the SDK alias `mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"`.

    REUSE newCallToolRequest — do NOT redefine it (it lives in bridge_test.go in the same package). Construct every *bridge with the canonical literal: `b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}` (copy from workspaces_test.go:34).

    Write SIX tests, one happy + one error per tool (07 D-08 coverage target). Each test follows the workspaces_test.go scaffold: declare any canned body and capture vars (gotPath, gotMethod); spin up httptest.NewServer with a HandlerFunc that writes the canned body; t.Cleanup(srv.Close); construct *bridge; build a *mcp.CallToolRequest via newCallToolRequest(json.RawMessage(`{...}`)); call the bridge method under test; assert.

    The six tests:

    1. TestBridge_ListPendingReviews_Happy_StateOk_ProjectsPrsArray — fake GET returns 200 with body `{"state":"ok","stale":false,"prs":[{"number":101,"title":"fix a"},{"number":102,"title":"fix b"}],"reviewed":[{"number":201,"title":"already reviewed"}]}`. Call b.listPendingReviews(ctx, newCallToolRequest(json.RawMessage(`{"project_id":1}`))). Assert err==nil; res.IsError==false; the TextContent text marshals/contains the prs entries (number 101 and 102) and does NOT contain number 201 (D-03 — only the prs queue projected). Optionally assert gotPath == "/api/projects/1/pull-requests" and gotMethod == "GET".

    2. TestBridge_ListPendingReviews_StateNoGh_IsErrorWithRawResult (the RESEARCH novel assertion, lines 364-389) — fake GET returns 200 with body `{"state":"no_gh","stale":false,"prs":null,"reviewed":null}` (a real endpoint-gate / gh-absent degrade). Call b.listPendingReviews(...). Assert err==nil (degrade is a tool result, NOT a transport error); res.IsError==true; the TextContent text contains the substring `"state":"no_gh"` (D-02 — raw Result JSON verbatim).

    3. TestBridge_ListPendingReviews_StateErrorWithNonNullArrays_IsErrorStillTrue (PITFALL 1 STALE-CACHE EDGE — load-bearing; without this test a regression to `if prs == nil { IsError=true }` would slip through). Fake GET returns 200 with body `{"state":"error","stale":true,"prs":[{"number":101}],"reviewed":[{"number":201}]}` (transient error AFTER a prior successful fetch — non-null arrays carried from cache). Call b.listPendingReviews(...). Assert res.IsError==true (NOT false — IsError is set PURELY on state != ok, never on array nullness). This is the single most important test in the file.

    4. TestBridge_ListRecentlyReviewed_Happy_StateOk_ProjectsReviewedArray — same fake GET body as test 1; call b.listRecentlyReviewed(...). Assert err==nil; res.IsError==false; the TextContent text contains number 201 (the reviewed entry) and does NOT contain number 101 or 102 (proves the queue parameter routes the success projection to res.Reviewed, not res.PRs).

    5. TestBridge_OpenReview_Happy_PostsReviewPath (copy the PATTERNS scaffold at :186-205) — fake POST handler captures gotPath + gotMethod, returns 200 with body `{"task":{"id":7},"pr":{"number":42,"title":"fix"}}`. Call b.openReview(ctx, newCallToolRequest(json.RawMessage(`{"project_id":3,"pr_number":42}`))). Assert err==nil; gotPath == "/api/projects/3/pull-requests/42/review"; gotMethod == http.MethodPost; res.IsError==false; tc.Text == the canned body verbatim (D-05 passthrough).

    6. TestBridge_OpenReview_GateBlocked_Kamacu409 (copy the PATTERNS scaffold at :215-231, modeled on workspaces_test.go:95-111) — fake POST returns 409 with body `{"error":"GitHub integration is off"}` (pullrequests.go:166 shape). Call b.openReview(...). Assert err != nil (gate failure is a transport-level error per D-05, NOT a tool result); err.Error() contains substring "HTTP 409". Optionally also assert it contains "GitHub integration is off" (the Kamacu message survives the D-05 wrap — Gap 2 pattern from workspaces_test.go:240).

    Add a brief doc comment above each test naming the requirement ID (MCPREV-01/02/03) and the decision/pitfall being exercised, mirroring the workspaces_test.go doc-comment style.

    Verify the tests run and pass against the Task 1 implementation: `go test ./internal/mcp/ -run 'TestBridge_(ListPendingReviews|ListRecentlyReviewed|OpenReview)' -v`.
  </action>

  <verify>
    <automated>cd /home/jordi/workspace/github/kangent && go test ./internal/mcp/ -run 'TestBridge_(ListPendingReviews|ListRecentlyReviewed|OpenReview)' -v -count=1</automated>
  </verify>

  <acceptance_criteria>
    - File internal/mcp/reviews_test.go exists in package mcp (NOT package mcp_test — same-package so it can call unexported bridge methods and reuse newCallToolRequest).
    - `go test ./internal/mcp/ -run 'TestBridge_(ListPendingReviews|ListRecentlyReviewed|OpenReview)' -count=1` exits 0 (all six tests pass).
    - The test file defines exactly these six test functions: TestBridge_ListPendingReviews_Happy_StateOk_ProjectsPrsArray, TestBridge_ListPendingReviews_StateNoGh_IsErrorWithRawResult, TestBridge_ListPendingReviews_StateErrorWithNonNullArrays_IsErrorStillTrue, TestBridge_ListRecentlyReviewed_Happy_StateOk_ProjectsReviewedArray, TestBridge_OpenReview_Happy_PostsReviewPath, TestBridge_OpenReview_GateBlocked_Kamacu409.
    - TestBridge_ListPendingReviews_StateErrorWithNonNullArrays_IsErrorStillTrue asserts IsError==true on a body whose state is "error" AND whose prs/reviewed arrays are NON-null (Pitfall 1 stale-cache edge — load-bearing).
    - TestBridge_ListPendingReviews_Happy asserts the TextContent contains a prs entry and does NOT contain a reviewed entry (D-03 success projection).
    - TestBridge_ListRecentlyReviewed_Happy asserts the TextContent contains a reviewed entry and does NOT contain a prs entry (proves queue routing).
    - TestBridge_OpenReview_Happy_PostsReviewPath asserts request method POST and request path matching /api/projects/3/pull-requests/42/review.
    - TestBridge_OpenReview_GateBlocked_Kamacu409 asserts err != nil and err.Error() contains "HTTP 409".
    - The file does NOT redefine newCallToolRequest (it lives in bridge_test.go and is in scope).
    - `go vet ./internal/mcp/...` exits 0.
  </acceptance_criteria>

  <done>
    Six tests pass: happy + error per tool (07 D-08), including the load-bearing Pitfall 1 stale-cache assertion (state:error + non-null arrays → IsError=true), the D-02 raw-Result-JSON assertion, the D-03 per-tool queue projection, and the open_review D-05 409 wrap. Tests reuse the workspaces_test.go / bridge_test.go scaffold and newCallToolRequest — no redefinition, no new test infrastructure.
  </done>
</task>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| agent CLI → kamacu mcp serve (stdio JSON-RPC) | Untrusted tool-call arguments cross here (project_id, pr_number). Decoded as int64 via json.Unmarshal into a typed struct; fmt.Sprintf("%d",...) renders canonical decimal — no path-segment injection possible. |
| kamacu mcp serve → Kamacu HTTP API (loopback) | The bridge crosses this with X-Kamacu-Token on every call. Loopback binding (Phase 06 D-06) is the actual auth boundary; the token header is sent for forward-compat. Inherits unchanged. |
| Kamacu HTTP API → gh CLI → GitHub | The v1.3 two-gate ladder (settings toggle → project link) runs server-side on every call BEFORE any gh spawn. The bridge adds no gate logic and cannot bypass it. Inherits unchanged. |

## STRIDE Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation Plan |
|-----------|----------|-----------|----------|-------------|-----------------|
| T-09-01 | Tampering | listReviews / openReview path interpolation (project_id, pr_number) | low | mitigate | Args decoded as int64 via json.Unmarshal into a typed struct; path built with fmt.Sprintf("%d", ...) which renders canonical decimal — no path-segment injection possible (RESEARCH Security Domain V5). |
| T-09-02 | Elevation of privilege | Bypassing the v1.3 two-gate ladder via the bridge | low | accept | Impossible by construction — the gates run server-side in internal/api/pullrequests.go on EVERY call BEFORE any gh spawn (list GET returns state:"disabled"; POST returns 409). The bridge adds NO gate logic (SC3 satisfied by NOT bypassing). |
| T-09-03 | Denial of Service | Unbounded response body from a runaway Kamacu server | low | mitigate | listReviews and bridge.call both wrap the body read in io.LimitReader(resp.Body, maxBodyBytes) where maxBodyBytes = 1<<20 (1 MiB, bridge.go:23). PR lists are tiny in practice. |
| T-09-04 | Denial of Service | stdout pollution desyncing the JSON-RPC stream | low | accept | Inherited from Phase 06 D-12 — slog is pinned to stderr at server.go:49. reviews.go writes NOTHING to stdout (acceptance criterion enforces zero fmt.Print* matches). |
| T-09-05 | Information Disclosure | D-02 returns raw Kamacu Result JSON on degrade (state:no_gh, auth_required, etc.) | low | accept | Deliberate design (D-02 / CONTEXT specifics / project posture: honesty over hand-holding). The state field names the problem so the agent can self-correct; the IsError flag is the actionable signal. No secrets cross — Kamacu's Result carries only PR metadata and the gh state enum. |

No HIGH or critical threats → no blocking human checkpoint required (security_block_on: high).
</threat_model>

<verification>
End-to-end phase verification:

1. Build + vet: `go build ./internal/mcp/... && go vet ./internal/mcp/...` — both exit 0.
2. Tests: `go test ./internal/mcp/ -count=1` — exit 0 (the 6 new tests plus the existing Phase 06/07/08 tests all green; no regression).
3. The full v1.11 milestone binary builds: `go build ./...` — exit 0 (proves no other package was broken by the new file or the server.go edit).
4. The new tools register: a quick smoke check via `grep -c 'registerReviewTools' internal/mcp/server.go` returns 1 (the call site) plus 1 (the function definition in reviews.go) — actually `grep -rn 'registerReviewTools' internal/mcp/` shows exactly 2 lines: the definition in reviews.go and the call in server.go.
5. SC3 grep gate: `grep -c 'internal/github' internal/mcp/reviews.go` returns 0 — the bridge is decoupled from the domain package (Pitfall 5).
6. No-write-to-stdout gate: `grep -cE 'fmt\.Print(ln|f)?\(' internal/mcp/reviews.go` returns 0.
</verification>

<success_criteria>
Phase 09 complete when:

- [ ] internal/mcp/reviews.go compiles and vets clean.
- [ ] internal/mcp/reviews_test.go: 6 tests pass (happy + error per tool).
- [ ] registerReviewTools(s, b) is wired into server.go's registerTools.
- [ ] MCPREV-01 (list_pending_reviews): covered by tests 1-3.
- [ ] MCPREV-02 (list_recently_reviewed): covered by test 4.
- [ ] MCPREV-03 (open_review): covered by tests 5-6.
- [ ] D-01 (IsError hybrid): listReviews sets IsError purely on state != ok (verified by the stale-cache test 3).
- [ ] D-02 (raw Result JSON on error): verified by test 2's "state":"no_gh" substring assertion.
- [ ] D-03 (per-tool array projection): verified by tests 1 and 4 (queue routing).
- [ ] D-04 (project_id required): InputSchema "required" arrays enforce it; SDK validation precedes any HTTP call.
- [ ] D-05 (open_review via bridge.call, raw {task,pr}): verified by test 5's verbatim passthrough assertion.
- [ ] D-06 (no refresh param): no InputSchema declares refresh.
- [ ] SC3 (degrade is actionable, never bypasses two-gate ladder): list tools surface state != ok as IsError=true; open_review surfaces gates as wrapped HTTP 409; the bridge adds no gate logic.
- [ ] Pitfall 1 (stale-cache edge) defended: test 3 asserts IsError=true on state:error + non-null arrays.
- [ ] Pitfall 5 (no internal/github coupling) defended: grep gate returns 0.
- [ ] Phase 06 D-12 (stdout cleanliness) defended: reviews.go has zero fmt.Print* calls.
- [ ] Full milestone binary `go build ./...` exits 0.
- [ ] SUMMARY.md written at .planning/phases/09-pr-review-tools/09-01-PR-REVIEW-TOOLS-SUMMARY.md.
</success_criteria>

<output>
Create `.planning/phases/09-pr-review-tools/09-01-PR-REVIEW-TOOLS-SUMMARY.md` when done.
</output>

<artifacts_this_phase_produces>
## Artifacts this phase produces

New Go symbols (all in package `mcp`):

- **Functions:**
  - `func registerReviewTools(s *mcp.Server, b *bridge)` — the per-resource registrar (07 D-07 split).
  - `func (b *bridge) listReviews(ctx context.Context, projectID int64, queue string) (*mcp.CallToolResult, error)` — the shared state-inspecting helper (D-01/D-02/D-03).
  - `func (b *bridge) listPendingReviews(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error)` — the list_pending_reviews handler.
  - `func (b *bridge) listRecentlyReviewed(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error)` — the list_recently_reviewed handler.
  - `func (b *bridge) openReview(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error)` — the open_review handler.

- **Anonymous types (compile-time only):**
  - Inside `listReviews`: a local struct `{ State string; Stale bool; PRs json.RawMessage; Reviewed json.RawMessage }` for the state decode (Pitfall 5 — NOT github.Result).
  - Inside `listPendingReviews` / `listRecentlyReviewed`: `struct { ProjectID int64 }`.
  - Inside `openReview`: `struct { ProjectID int64; PRNumber int64 }`.

- **MCP tool registrations (visible via `tools/list`):**
  - `list_pending_reviews` (InputSchema: project_id required).
  - `list_recently_reviewed` (InputSchema: project_id required).
  - `open_review` (InputSchema: project_id + pr_number required).

- **Test functions (in `internal/mcp/reviews_test.go`):**
  - `TestBridge_ListPendingReviews_Happy_StateOk_ProjectsPrsArray`
  - `TestBridge_ListPendingReviews_StateNoGh_IsErrorWithRawResult`
  - `TestBridge_ListPendingReviews_StateErrorWithNonNullArrays_IsErrorStillTrue` (Pitfall 1)
  - `TestBridge_ListRecentlyReviewed_Happy_StateOk_ProjectsReviewedArray`
  - `TestBridge_OpenReview_Happy_PostsReviewPath`
  - `TestBridge_OpenReview_GateBlocked_Kamacu409`

- **New files:**
  - `internal/mcp/reviews.go`
  - `internal/mcp/reviews_test.go`

- **Modified files (one line each):**
  - `internal/mcp/server.go` — `registerReviewTools(s, b)` added as the final call in `registerTools` (line 87 becomes 88 with the new line inserted).

- **No new packages, no new env vars, no new endpoints, no DB/migration/frontend changes.**
</artifacts_this_phase_produces>

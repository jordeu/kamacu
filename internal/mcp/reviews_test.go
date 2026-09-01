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

// TestBridge_ListPendingReviews_Happy_StateOk_ProjectsPrsArray (MCPREV-01,
// D-03 success projection): when Kamacu's GET returns 200 with state:ok and
// BOTH queues populated, list_pending_reviews returns IsError=false and the
// TextContent is the `prs` array ONLY — the `reviewed` entries must NOT
// appear (the queue parameter routes the success projection to res.PRs, not
// res.Reviewed). Also proves the bridge hits the right path and method.
func TestBridge_ListPendingReviews_Happy_StateOk_ProjectsPrsArray(t *testing.T) {
	const canned = `{"state":"ok","stale":false,"prs":[{"number":101,"title":"fix a"},{"number":102,"title":"fix b"}],"reviewed":[{"number":201,"title":"already reviewed"}]}`
	var (
		gotPath   string
		gotMethod string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(canned))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"project_id":1}`))
	res, err := b.listPendingReviews(context.Background(), req)
	if err != nil {
		t.Fatalf("listPendingReviews: %v", err)
	}
	if res.IsError {
		t.Errorf("IsError: want false on state:ok, got true")
	}
	if gotPath != "/api/projects/1/pull-requests" {
		t.Errorf("request URL.Path: want %q, got %q", "/api/projects/1/pull-requests", gotPath)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("request method: want %q, got %q", http.MethodGet, gotMethod)
	}
	tc, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("result.Content[0]: want *TextContent, got %T", res.Content[0])
	}
	// D-03: the prs queue is projected — both prs entries present, reviewed absent.
	if !strings.Contains(tc.Text, `"number":101`) || !strings.Contains(tc.Text, `"number":102`) {
		t.Errorf("prs projection: want both prs entries (101, 102) in %q", tc.Text)
	}
	if strings.Contains(tc.Text, `"number":201`) {
		t.Errorf("prs projection: reviewed entry 201 must NOT appear in %q (D-03 — only the prs queue projected)", tc.Text)
	}
}

// TestBridge_ListPendingReviews_StateNoGh_IsErrorWithRawResult (MCPREV-01,
// D-01/D-02 degrade): when Kamacu's GET returns 200 with state:no_gh (GitHub
// absent / endpoint-gate closed), list_pending_reviews returns IsError=true
// and the TextContent is the raw Kamacu Result JSON verbatim — including the
// `state` field so the agent can self-correct. This is the RESEARCH novel
// assertion (lines 364-389): bridge.call would treat a 200 as unconditional
// success and lose the degrade signal; listReviews diverges precisely so
// IsError can be set on a 2xx response. err MUST be nil — a degrade is a tool
// result, NOT a transport error.
func TestBridge_ListPendingReviews_StateNoGh_IsErrorWithRawResult(t *testing.T) {
	const canned = `{"state":"no_gh","stale":false,"prs":null,"reviewed":null}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK) // ALWAYS 200 — degradation rides in the body
		_, _ = w.Write([]byte(canned))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"project_id":1}`))
	res, err := b.listPendingReviews(context.Background(), req)
	if err != nil {
		t.Fatalf("listPendingReviews: degrade is a tool result, not a transport error; got %v", err)
	}
	if !res.IsError {
		t.Fatal("IsError: want true for state:no_gh, got false (D-01)")
	}
	tc, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("result.Content[0]: want *TextContent, got %T", res.Content[0])
	}
	// D-02: the raw Kamacu Result JSON is preserved verbatim — the state field
	// survives so the agent can read the degrade reason and self-correct.
	if !strings.Contains(tc.Text, `"state":"no_gh"`) {
		t.Errorf("degrade payload: want raw Result JSON containing \"state\":\"no_gh\", got %q (D-02)", tc.Text)
	}
}

// TestBridge_ListPendingReviews_StateErrorWithNonNullArrays_IsErrorStillTrue
// (MCPREV-01, PITFALL 1 STALE-CACHE EDGE — load-bearing): when Kamacu's GET
// returns 200 with state:error AND non-null prs/reviewed arrays AND stale:true
// (the shape produced by internal/github service.go resultLocked() when a
// transient error occurs AFTER a prior successful fetch — cached arrays are
// carried independently of state), list_pending_reviews MUST still return
// IsError=true. A regression to `if prs == nil { IsError=true }` would let
// this degrade slip through as IsError=false because the arrays are non-null.
// The state-field check is the only correct predicate. This is the single
// most important test in the file — it pins the invariant the plan's Pitfall 1
// documents.
func TestBridge_ListPendingReviews_StateErrorWithNonNullArrays_IsErrorStillTrue(t *testing.T) {
	const canned = `{"state":"error","stale":true,"prs":[{"number":101}],"reviewed":[{"number":201}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(canned))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"project_id":1}`))
	res, err := b.listPendingReviews(context.Background(), req)
	if err != nil {
		t.Fatalf("listPendingReviews: degrade is a tool result; got %v", err)
	}
	// THE load-bearing assertion: IsError==true even though the arrays are
	// non-null. IsError is set PURELY on state != "ok", never on array nullness.
	if !res.IsError {
		t.Fatal("IsError: want true (state:error + non-null arrays + stale:true is STILL a degrade — Pitfall 1), got false")
	}
}

// TestBridge_ListRecentlyReviewed_Happy_StateOk_ProjectsReviewedArray
// (MCPREV-02, D-03 queue routing): same fake GET body as the pending happy
// path, but list_recently_reviewed projects the `reviewed` queue — the prs
// entries must NOT appear. Proves the queue parameter routes the success
// projection to res.Reviewed, not res.PRs.
func TestBridge_ListRecentlyReviewed_Happy_StateOk_ProjectsReviewedArray(t *testing.T) {
	const canned = `{"state":"ok","stale":false,"prs":[{"number":101,"title":"fix a"},{"number":102,"title":"fix b"}],"reviewed":[{"number":201,"title":"already reviewed"}]}`
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(canned))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"project_id":1}`))
	res, err := b.listRecentlyReviewed(context.Background(), req)
	if err != nil {
		t.Fatalf("listRecentlyReviewed: %v", err)
	}
	if res.IsError {
		t.Errorf("IsError: want false on state:ok, got true")
	}
	// Same endpoint as list_pending_reviews (the v1.5 no-N+1 invariant: one gh
	// cycle serves both queues per repo).
	if gotPath != "/api/projects/1/pull-requests" {
		t.Errorf("request URL.Path: want %q, got %q", "/api/projects/1/pull-requests", gotPath)
	}
	tc, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("result.Content[0]: want *TextContent, got %T", res.Content[0])
	}
	// D-03: the reviewed queue is projected — reviewed entry present, prs absent.
	if !strings.Contains(tc.Text, `"number":201`) {
		t.Errorf("reviewed projection: want reviewed entry 201 in %q", tc.Text)
	}
	if strings.Contains(tc.Text, `"number":101`) || strings.Contains(tc.Text, `"number":102`) {
		t.Errorf("reviewed projection: prs entries (101, 102) must NOT appear in %q (D-03 — only the reviewed queue projected)", tc.Text)
	}
}

// TestBridge_OpenReview_Happy_PostsReviewPath (MCPREV-03, D-05 passthrough):
// when Kamacu's POST returns 200 with the {task, pr} envelope, open_review
// returns IsError=false and the TextContent is the raw body verbatim. Also
// proves the bridge POSTs to /api/projects/{id}/pull-requests/{n}/review with
// the right path and method (the int64 args render canonical decimal — V5
// path-injection mitigation).
func TestBridge_OpenReview_Happy_PostsReviewPath(t *testing.T) {
	const canned = `{"task":{"id":7},"pr":{"number":42,"title":"fix"}}`
	var (
		gotPath   string
		gotMethod string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(canned))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"project_id":3,"pr_number":42}`))
	res, err := b.openReview(context.Background(), req)
	if err != nil {
		t.Fatalf("openReview: %v", err)
	}
	if res.IsError {
		t.Errorf("IsError: want false on 200, got true")
	}
	if gotPath != "/api/projects/3/pull-requests/42/review" {
		t.Errorf("request URL.Path: want %q, got %q", "/api/projects/3/pull-requests/42/review", gotPath)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("request method: want %q, got %q", http.MethodPost, gotMethod)
	}
	tc, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("result.Content[0]: want *TextContent, got %T", res.Content[0])
	}
	// D-05: verbatim passthrough — the raw {task, pr} envelope survives.
	if tc.Text != canned {
		t.Errorf("passthrough text: want %q, got %q (D-05 verbatim)", canned, tc.Text)
	}
}

// TestBridge_OpenReview_GateBlocked_Kamacu409 (MCPREV-03, D-05 wrap): when
// Kamacu's POST returns 409 (the two-gate ladder blocks — GitHub integration
// off OR project not linked, pullrequests.go:166/175/183), open_review
// returns a non-nil error wrapping `kamacu POST ...: HTTP 409: <body>`. Gate
// failure is a transport-level error per D-05 (the bridge.call non-2xx wrap),
// NOT a tool result — the SDK surfaces it as JSON-RPC -32603. The Kamacu
// message survives the D-05 wrap (Gap 2 pattern from workspaces_test.go:240).
func TestBridge_OpenReview_GateBlocked_Kamacu409(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		// Real Kamacu shape (pullrequests.go:166) — keyed on "error".
		_, _ = w.Write([]byte(`{"error":"GitHub integration is off"}`))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"project_id":3,"pr_number":42}`))
	_, err := b.openReview(context.Background(), req)
	if err == nil {
		t.Fatal("openReview: expected error for gate-blocked 409, got nil")
	}
	if !strings.Contains(err.Error(), "HTTP 409") {
		t.Errorf("openReview error: want substring \"HTTP 409\", got %q", err.Error())
	}
	// Gap 2: assert on the Kamacu message substring, NOT on a JSON key name.
	// The raw body is inlined in the D-05 wrap, so the message text survives.
	if !strings.Contains(err.Error(), "GitHub integration is off") {
		t.Errorf("openReview error: want substring \"GitHub integration is off\" (Kamacu message survives D-05 wrap), got %q", err.Error())
	}
}

// TestBridge_PostPRReview_Happy_PostsReviewsPath (MCPREV-04, D-05 passthrough):
// when Kamacu's POST returns 201 with the gh JSON response, post_pr_review
// returns IsError=false and the TextContent is the raw body verbatim. Also
// proves the bridge POSTs to /api/projects/{id}/pull-requests/{n}/reviews
// (PLURAL — distinct from the singular /review open-workspace route) with the
// marshaled JSON body carrying verdict/body/inline_comments.
func TestBridge_PostPRReview_Happy_PostsReviewsPath(t *testing.T) {
	const canned = `{"id":99,"state":"APPROVED"}`
	var (
		gotPath   string
		gotMethod string
		gotBody   map[string]any
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated) // 201 Created — full 2xx range accepted by bridge.call
		_, _ = w.Write([]byte(canned))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"project_id":3,"pr_number":42,"verdict":"approve","body":"LGTM","inline_comments":[{"path":"a.go","line":1,"body":"x"}]}`))
	res, err := b.postPRReview(context.Background(), req)
	if err != nil {
		t.Fatalf("postPRReview: %v", err)
	}
	if res.IsError {
		t.Errorf("IsError: want false on 201, got true")
	}
	if gotPath != "/api/projects/3/pull-requests/42/reviews" {
		t.Errorf("request URL.Path: want %q, got %q", "/api/projects/3/pull-requests/42/reviews", gotPath)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("request method: want %q, got %q", http.MethodPost, gotMethod)
	}
	// The marshaled body carries the verdict + body + inline_comments through
	// to Kamacu verbatim (D-04 verbatim passthrough).
	if gotBody["verdict"] != "approve" {
		t.Errorf("body.verdict = %v, want approve", gotBody["verdict"])
	}
	if gotBody["body"] != "LGTM" {
		t.Errorf("body.body = %v, want LGTM", gotBody["body"])
	}
	ics, ok := gotBody["inline_comments"].([]any)
	if !ok {
		t.Fatalf("body.inline_comments not an array: %T", gotBody["inline_comments"])
	}
	if len(ics) != 1 {
		t.Fatalf("len(inline_comments) = %d, want 1", len(ics))
	}
	c0, _ := ics[0].(map[string]any)
	if c0["path"] != "a.go" || c0["line"] != float64(1) || c0["body"] != "x" {
		t.Errorf("inline_comments[0] = %v, want {path:a.go line:1 body:x}", c0)
	}
	tc, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("result.Content[0]: want *TextContent, got %T", res.Content[0])
	}
	// D-05: verbatim passthrough — the raw gh JSON response survives.
	if tc.Text != canned {
		t.Errorf("passthrough text: want %q, got %q (D-05 verbatim)", canned, tc.Text)
	}
}

// TestBridge_PostPRReview_GateBlocked_Kamacu409 (MCPREV-04, D-05 wrap): when
// Kamacu's POST returns 409 (the two-gate ladder blocks), post_pr_review
// returns a non-nil error wrapping `kamacu POST ...: HTTP 409: <body>`.
// Identical degrade contract to openReview's gate test.
func TestBridge_PostPRReview_GateBlocked_Kamacu409(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"GitHub integration is off"}`))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"project_id":3,"pr_number":42,"verdict":"approve"}`))
	_, err := b.postPRReview(context.Background(), req)
	if err == nil {
		t.Fatal("postPRReview: expected error for gate-blocked 409, got nil")
	}
	if !strings.Contains(err.Error(), "HTTP 409") {
		t.Errorf("postPRReview error: want substring \"HTTP 409\", got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "GitHub integration is off") {
		t.Errorf("postPRReview error: want substring \"GitHub integration is off\" (Kamacu message survives D-05 wrap), got %q", err.Error())
	}
}

// TestBridge_PostPRReview_OptionalFieldsOmitted_SendsVerdictOnly (MCPREV-04,
// omitempty): when the agent supplies ONLY the required verdict (no body, no
// inline_comments), the marshaled body carries ONLY verdict — proves the bridge
// does not require/synthesize the optional fields. The body/inline_comments
// keys MUST be absent (not just empty), matching the API layer's own omitempty.
func TestBridge_PostPRReview_OptionalFieldsOmitted_SendsVerdictOnly(t *testing.T) {
	const canned = `{"id":7,"state":"COMMENTED"}`
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(canned))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"project_id":1,"pr_number":2,"verdict":"comment"}`))
	res, err := b.postPRReview(context.Background(), req)
	if err != nil {
		t.Fatalf("postPRReview: %v", err)
	}
	if res.IsError {
		t.Errorf("IsError: want false on 201, got true")
	}
	if gotBody["verdict"] != "comment" {
		t.Errorf("body.verdict = %v, want comment", gotBody["verdict"])
	}
	if _, present := gotBody["body"]; present {
		t.Errorf("body.body present; want omitted (no body arg → not sent)")
	}
	if _, present := gotBody["inline_comments"]; present {
		t.Errorf("body.inline_comments present; want omitted (no inline_comments arg → not sent)")
	}
}

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

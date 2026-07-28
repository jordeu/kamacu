package github

// PostReview has NO local equivalent of "post a review" — posting needs live
// GitHub, unlike Clone which has the file:// git fallback. So the SUCCESS path
// is ALSO exercised via the stubbed postReviewRunner seam (the stub returns
// canned stdout + captures the marshaled body for assertion). The swap idiom
// mirrors clone_test.go.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// stubPostReview swaps the package-level postReviewRunner with fn and restores
// it on cleanup. The capture argument is whatever the test wants to inspect
// (repo, prNumber, marshaled body).
func swapPostReviewRunner(t *testing.T, fn func(ctx context.Context, repo string, prNumber int, body []byte) ([]byte, error)) {
	t.Helper()
	orig := postReviewRunner
	postReviewRunner = fn
	t.Cleanup(func() { postReviewRunner = orig })
}

// TestPostReview_ApproveWithInlineComments_MapsToApproveEvent captures the
// (repo, prNumber, body) the runner saw and asserts the verdict→event mapping,
// the comments array shape, the canned stdout passthrough, and a nil error.
func TestPostReview_ApproveWithInlineComments_MapsToApproveEvent(t *testing.T) {
	var gotRepo string
	var gotPR int
	var gotBody []byte
	canned := []byte(`{"id":99,"state":"APPROVED"}`)
	swapPostReviewRunner(t, func(ctx context.Context, repo string, prNumber int, body []byte) ([]byte, error) {
		gotRepo, gotPR, gotBody = repo, prNumber, body
		return canned, nil
	})

	out, err := PostReview(context.Background(), "owner/name", 42, "approve", "LGTM",
		[]InlineComment{{Path: "a.go", Line: 10, Body: "nit"}})
	if err != nil {
		t.Fatalf("PostReview: unexpected error: %v", err)
	}
	if string(out) != string(canned) {
		t.Fatalf("out = %q, want %q", out, canned)
	}
	if gotRepo != "owner/name" {
		t.Fatalf("repo = %q, want %q", gotRepo, "owner/name")
	}
	if gotPR != 42 {
		t.Fatalf("prNumber = %d, want 42", gotPR)
	}

	var decoded map[string]any
	if err := json.Unmarshal(gotBody, &decoded); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if decoded["event"] != "APPROVE" {
		t.Fatalf("event = %v, want APPROVE", decoded["event"])
	}
	if decoded["body"] != "LGTM" {
		t.Fatalf("body = %v, want LGTM", decoded["body"])
	}
	cs, ok := decoded["comments"].([]any)
	if !ok {
		t.Fatalf("comments not an array: %T", decoded["comments"])
	}
	if len(cs) != 1 {
		t.Fatalf("len(comments) = %d, want 1", len(cs))
	}
	c0, _ := cs[0].(map[string]any)
	if c0["path"] != "a.go" || c0["line"] != float64(10) || c0["body"] != "nit" {
		t.Fatalf("comments[0] = %v, want {path:a.go line:10 body:nit}", c0)
	}
}

// TestPostReview_RequestChanges_MapsToRequestChangesEvent is the minimal
// verdict→event mapping check for the request_changes verdict.
func TestPostReview_RequestChanges_MapsToRequestChangesEvent(t *testing.T) {
	var gotBody []byte
	swapPostReviewRunner(t, func(ctx context.Context, _ string, _ int, body []byte) ([]byte, error) {
		gotBody = body
		return []byte(`{}`), nil
	})

	if _, err := PostReview(context.Background(), "owner/name", 7, "request_changes", "", nil); err != nil {
		t.Fatalf("PostReview: unexpected error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(gotBody, &decoded); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if decoded["event"] != "REQUEST_CHANGES" {
		t.Fatalf("event = %v, want REQUEST_CHANGES", decoded["event"])
	}
	if _, present := decoded["body"]; present {
		t.Fatalf("body key present; want omitted (empty body should not be sent)")
	}
}

// TestPostReview_Comment_OmitsCommentsWhenEmpty proves omitempty: a "comment"
// verdict with nil comments produces a body WITHOUT a "comments" key.
func TestPostReview_Comment_OmitsCommentsWhenEmpty(t *testing.T) {
	var gotBody []byte
	swapPostReviewRunner(t, func(ctx context.Context, _ string, _ int, body []byte) ([]byte, error) {
		gotBody = body
		return []byte(`{}`), nil
	})

	if _, err := PostReview(context.Background(), "owner/name", 1, "comment", "note", nil); err != nil {
		t.Fatalf("PostReview: unexpected error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(gotBody, &decoded); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if decoded["event"] != "COMMENT" {
		t.Fatalf("event = %v, want COMMENT", decoded["event"])
	}
	if decoded["body"] != "note" {
		t.Fatalf("body = %v, want note", decoded["body"])
	}
	if _, present := decoded["comments"]; present {
		t.Fatalf("comments key present; want omitted (nil comments should not be sent)")
	}
}

// TestPostReview_UnknownVerdict_NeverCallsRunner asserts that an unknown
// verdict returns (nil, err) WITHOUT ever invoking the runner — the taxonomy
// check happens BEFORE the Available check and BEFORE the spawn.
func TestPostReview_UnknownVerdict_NeverCallsRunner(t *testing.T) {
	swapPostReviewRunner(t, func(ctx context.Context, _ string, _ int, _ []byte) ([]byte, error) {
		t.Fatal("runner called for an unknown verdict; PostReview must validate first")
		return nil, nil
	})

	out, err := PostReview(context.Background(), "owner/name", 1, "ship_it", "", nil)
	if err == nil {
		t.Fatal("PostReview: expected an error for an unknown verdict, got nil")
	}
	if out != nil {
		t.Fatalf("out = %v, want nil", out)
	}
	if !strings.Contains(err.Error(), "unknown verdict") {
		t.Fatalf("err = %q, want it to contain %q", err.Error(), "unknown verdict")
	}
}

// TestPostReview_GhFailure_ReturnsErrorWithStderr exercises the gh-failure
// passthrough: the runner returns (nil, err) and PostReview surfaces it
// verbatim. (The trimmed-stderr formatting inside realPostReview is not
// re-tested here because it would require a real gh failure; the API layer's
// 502 degrade test exercises the seam end-to-end.)
func TestPostReview_GhFailure_ReturnsErrorWithStderr(t *testing.T) {
	swapPostReviewRunner(t, func(ctx context.Context, _ string, _ int, _ []byte) ([]byte, error) {
		return nil, errors.New("boom")
	})

	out, err := PostReview(context.Background(), "owner/name", 1, "comment", "x", nil)
	if err == nil {
		t.Fatal("PostReview: expected an error from a failing runner, got nil")
	}
	if out != nil {
		t.Fatalf("out = %v, want nil", out)
	}
	// PostReview delegates the error shape to the runner; the production
	// realPostReview wraps it as "couldn't post review to PR #N: <msg>", but
	// the stub here returns the bare "boom" so just assert the substring.
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %q, want it to contain %q", err.Error(), "boom")
	}
}

package api

import (
	"net/http"
	"testing"

	"kamacu/internal/github"
)

// TestGithubStatus pins the GET /api/github/status contract: an always-200
// read of whether the host `gh` CLI is installed. It asserts the endpoint's
// answer equals github.Available() in this process, so the test is green on
// any host (gh present or absent) — it verifies the contract (status 200,
// boolean field named exactly `gh_available`, value = the real LookPath
// result), not a hardcoded true/false.
func TestGithubStatus(t *testing.T) {
	srv, _, _ := newTestServer(t)

	status, body := doJSON(t, http.MethodGet, srv.URL+"/api/github/status", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%v", status, body)
	}

	raw, ok := body["gh_available"]
	if !ok {
		t.Fatalf("response missing 'gh_available' field: %v", body)
	}
	avail, ok := raw.(bool)
	if !ok {
		t.Fatalf("gh_available = %v (%T), want a bool", raw, raw)
	}
	if want := github.Available(); avail != want {
		t.Errorf("gh_available = %v, want %v (github.Available())", avail, want)
	}
}

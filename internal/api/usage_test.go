package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"kamacu/internal/quota"
)

// usageFakeToken is generated, never pasted — real tokens must never enter
// fixtures.
func usageFakeToken() string {
	return "sk-ant-oat01-" + strings.Repeat("x", 95)
}

// usageLiveJSON mirrors the upstream shape captured live 2026-06-12.
const usageLiveJSON = `{
  "five_hour":        { "utilization": 15, "resets_at": "2026-06-12T07:49:59Z" },
  "seven_day":        { "utilization": 73, "resets_at": "2026-06-13T20:59:59Z" },
  "seven_day_opus":   null,
  "seven_day_sonnet": { "utilization": 0, "resets_at": null }
}`

// newUsageService builds a real quota.Service (Config is the public seam, no
// mocks) against a temp creds file and an httptest upstream. credsJSON==""
// leaves the credentials file absent. now may be nil for the real clock.
func newUsageService(t *testing.T, credsJSON string, handler http.HandlerFunc, now func() time.Time) *quota.Service {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".credentials.json")
	if credsJSON != "" {
		if err := os.WriteFile(path, []byte(credsJSON), 0o600); err != nil {
			t.Fatalf("write creds: %v", err)
		}
	}
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return quota.New(quota.Config{
		CredentialsPath: path,
		BaseURL:         srv.URL,
		UserAgent:       "claude-code/test",
		Now:             now,
	})
}

// usageGet drives GET path through a fresh mux with UsageRoutes registered.
func usageGet(t *testing.T, svc *quota.Service, path string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	UsageRoutes(mux, svc)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestUsageNoCredentialsAlways200(t *testing.T) {
	svc := newUsageService(t, "", func(w http.ResponseWriter, r *http.Request) {
		t.Error("upstream must not be hit without credentials")
	}, nil)
	rec := usageGet(t, svc, "/api/usage")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (degrade, never error)", rec.Code)
	}
	var res quota.Result
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if res.State != "no_credentials" {
		t.Errorf("state = %q, want no_credentials", res.State)
	}
}

func TestUsageHappyPath(t *testing.T) {
	exp := strconv.FormatInt(time.Now().Add(time.Hour).UnixMilli(), 10)
	creds := `{"claudeAiOauth":{"accessToken":"` + usageFakeToken() + `","expiresAt":` + exp + `}}`
	svc := newUsageService(t, creds, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(usageLiveJSON))
	}, nil)

	rec := usageGet(t, svc, "/api/usage")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var res quota.Result
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if res.State != "ok" || len(res.Windows) != 3 {
		t.Errorf("got {state %q, %d windows}, want ok with 3 windows", res.State, len(res.Windows))
	}
	if len(res.Windows) > 0 && (res.Windows[0].Key != "five_hour" || res.Windows[0].Label != "5h") {
		t.Errorf("windows[0] = {%q %q}, want {five_hour 5h}", res.Windows[0].Key, res.Windows[0].Label)
	}
	// Token-leak guard (roadmap-mandated): digested numbers only.
	if strings.Contains(rec.Body.String(), "sk-ant-oat") {
		t.Error("response body contains token material")
	}
}

func TestUsageRefreshParamForcesFetch(t *testing.T) {
	var hits atomic.Int64
	cur := time.Now()
	exp := strconv.FormatInt(cur.Add(2*time.Hour).UnixMilli(), 10)
	creds := `{"claudeAiOauth":{"accessToken":"` + usageFakeToken() + `","expiresAt":` + exp + `}}`
	svc := newUsageService(t, creds, func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(usageLiveJSON))
	}, func() time.Time { return cur })

	usageGet(t, svc, "/api/usage")
	if hits.Load() != 1 {
		t.Fatalf("hits after first GET = %d, want 1", hits.Load())
	}

	cur = cur.Add(30 * time.Second) // within 60s TTL, past the 10s floor

	// Plain GET inside the TTL serves the cache — no upstream hit.
	usageGet(t, svc, "/api/usage")
	if hits.Load() != 1 {
		t.Errorf("hits after plain GET within TTL = %d, want 1", hits.Load())
	}
	// ?refresh=1 maps to Get(ctx, force=true) — bypasses the TTL.
	usageGet(t, svc, "/api/usage?refresh=1")
	if hits.Load() != 2 {
		t.Errorf("hits after ?refresh=1 past floor = %d, want 2", hits.Load())
	}
}

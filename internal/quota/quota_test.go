package quota

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeToken is generated, never pasted — greppable-safe (108 chars, the
// observed sk-ant-oat01 length class). Real tokens must never enter fixtures.
func fakeToken() string {
	return "sk-ant-oat01-" + strings.Repeat("x", 95)
}

// writeCreds writes a credentials fixture into a temp dir and returns its path.
func writeCreds(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".credentials.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write creds fixture: %v", err)
	}
	return path
}

// liveUsageJSON mirrors the response captured live 2026-06-12: populated
// five_hour/seven_day, whole-null seven_day_opus, sonnet with null resets_at,
// plus the deferred extra_usage block (QUOTA-FUT-01) whose shape lacks
// utilization.
const liveUsageJSON = `{
  "five_hour":        { "utilization": 15, "resets_at": "2026-06-12T07:49:59Z" },
  "seven_day":        { "utilization": 73, "resets_at": "2026-06-13T20:59:59Z" },
  "seven_day_opus":   null,
  "seven_day_sonnet": { "utilization": 0, "resets_at": null },
  "extra_usage":      { "is_enabled": false }
}`

func TestReadCredentialsLiveShape(t *testing.T) {
	tok := fakeToken()
	// Exact live file shape: claudeAiOauth alongside unrelated mcpOAuth
	// secrets that must be ignored (decode only the two needed fields).
	path := writeCreds(t, `{
	  "claudeAiOauth": {
	    "accessToken": "`+tok+`",
	    "refreshToken": "fake-refresh-never-used",
	    "expiresAt": 1781238586291,
	    "scopes": ["user:inference"],
	    "subscriptionType": "team",
	    "rateLimitTier": "default"
	  },
	  "mcpOAuth": { "someServer": { "accessToken": "third-party-secret" } }
	}`)

	gotTok, gotExp, err := readCredentials(path)
	if err != nil {
		t.Fatalf("readCredentials: %v", err)
	}
	if gotTok != tok {
		t.Errorf("token = %q, want the fixture token", gotTok)
	}
	// expiresAt is epoch MILLISECONDS — must round-trip via time.UnixMilli.
	if want := time.UnixMilli(1781238586291); !gotExp.Equal(want) {
		t.Errorf("expiresAt = %v, want %v (UnixMilli of 1781238586291)", gotExp, want)
	}
}

func TestReadCredentialsNoCredentials(t *testing.T) {
	t.Run("file absent", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), ".credentials.json") // never written
		tok, exp, err := readCredentials(path)
		if !errors.Is(err, errNoCredentials) {
			t.Fatalf("err = %v, want errNoCredentials", err)
		}
		if tok != "" || !exp.IsZero() {
			t.Errorf("got (%q, %v), want zero values", tok, exp)
		}
	})

	t.Run("claudeAiOauth key absent", func(t *testing.T) {
		path := writeCreds(t, `{"mcpOAuth":{}}`)
		if _, _, err := readCredentials(path); !errors.Is(err, errNoCredentials) {
			t.Fatalf("err = %v, want errNoCredentials", err)
		}
	})

	t.Run("empty accessToken", func(t *testing.T) {
		path := writeCreds(t, `{"claudeAiOauth":{"accessToken":"","expiresAt":1781238586291}}`)
		if _, _, err := readCredentials(path); !errors.Is(err, errNoCredentials) {
			t.Fatalf("err = %v, want errNoCredentials", err)
		}
	})
}

func TestNormalizeWindowsLive(t *testing.T) {
	got, err := normalizeWindows([]byte(liveUsageJSON))
	if err != nil {
		t.Fatalf("normalizeWindows: %v", err)
	}

	// Exactly three windows, in the fixed preference order (D-75):
	// seven_day_opus (whole-null) skipped, extra_usage skipped.
	want := []struct {
		key   string
		label string
		util  float64
	}{
		{"five_hour", "5h", 15},
		{"seven_day", "7d", 73},
		{"seven_day_sonnet", "Sonnet", 0},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d windows %+v, want %d", len(got), got, len(want))
	}
	for i, w := range want {
		if got[i].Key != w.key || got[i].Label != w.label || got[i].Utilization != w.util {
			t.Errorf("windows[%d] = {%q %q %v}, want {%q %q %v}",
				i, got[i].Key, got[i].Label, got[i].Utilization, w.key, w.label, w.util)
		}
	}

	// resets_at passthrough: populated for five_hour, nil carried for sonnet.
	if got[0].ResetsAt == nil || *got[0].ResetsAt != "2026-06-12T07:49:59Z" {
		t.Errorf("five_hour resetsAt = %v, want 2026-06-12T07:49:59Z", got[0].ResetsAt)
	}
	if got[2].ResetsAt != nil {
		t.Errorf("seven_day_sonnet resetsAt = %q, want nil", *got[2].ResetsAt)
	}
}

func TestNormalizeWindowsUnknownKeyPassthrough(t *testing.T) {
	// Unknown future keys come AFTER the known four, label = raw key (D-74).
	// extra_usage is skipped explicitly even if it grows a utilization field.
	body := `{
	  "weekly_haiku":     { "utilization": 12, "resets_at": null },
	  "five_hour":        { "utilization": 15, "resets_at": "2026-06-12T07:49:59Z" },
	  "seven_day":        { "utilization": 73, "resets_at": "2026-06-13T20:59:59Z" },
	  "seven_day_opus":   null,
	  "seven_day_sonnet": { "utilization": 0, "resets_at": null },
	  "extra_usage":      { "utilization": 50, "resets_at": null }
	}`
	got, err := normalizeWindows([]byte(body))
	if err != nil {
		t.Fatalf("normalizeWindows: %v", err)
	}
	wantKeys := []string{"five_hour", "seven_day", "seven_day_sonnet", "weekly_haiku"}
	if len(got) != len(wantKeys) {
		t.Fatalf("got %d windows %+v, want keys %v", len(got), got, wantKeys)
	}
	for i, k := range wantKeys {
		if got[i].Key != k {
			t.Errorf("windows[%d].Key = %q, want %q", i, got[i].Key, k)
		}
	}
	last := got[len(got)-1]
	if last.Label != "weekly_haiku" {
		t.Errorf("unknown-key label = %q, want raw key %q", last.Label, "weekly_haiku")
	}
	if last.Utilization != 12 {
		t.Errorf("weekly_haiku utilization = %v, want 12", last.Utilization)
	}
}

func TestNormalizeWindowsMalformed(t *testing.T) {
	if _, err := normalizeWindows([]byte("not json")); err == nil {
		t.Fatal("normalizeWindows(malformed) = nil error, want error")
	}
}

// ---- Task 2: Service six-state matrix ----------------------------------

// newTestService builds a Service against a temp creds file and an httptest
// upstream (the public Config seam — no mocks). credsJSON=="" leaves the
// file absent (state 1).
func newTestService(t *testing.T, credsJSON string, handler http.HandlerFunc) *Service {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".credentials.json")
	if credsJSON != "" {
		if err := os.WriteFile(path, []byte(credsJSON), 0o600); err != nil {
			t.Fatalf("write creds: %v", err)
		}
	}
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return New(Config{CredentialsPath: path, BaseURL: srv.URL, UserAgent: "claude-code/test"})
}

// validCreds returns a creds fixture with the given token and a one-hour
// future expiry.
func validCreds(token string) string {
	exp := strconv.FormatInt(time.Now().Add(time.Hour).UnixMilli(), 10)
	return `{"claudeAiOauth":{"accessToken":"` + token + `","expiresAt":` + exp + `}}`
}

// countingHandler wraps handler with an atomic hit counter.
func countingHandler(hits *atomic.Int64, handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		handler(w, r)
	}
}

// fakeClock installs a controllable now() on s and returns an advance func.
func fakeClock(s *Service) func(d time.Duration) {
	cur := time.Now()
	s.now = func() time.Time { return cur }
	return func(d time.Duration) { cur = cur.Add(d) }
}

func serve200(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(body))
	}
}

func TestServiceState1FileAbsent(t *testing.T) {
	var hits atomic.Int64
	s := newTestService(t, "", countingHandler(&hits, serve200(liveUsageJSON)))
	res := s.Get(context.Background(), false)
	if res.State != "no_credentials" {
		t.Errorf("State = %q, want no_credentials", res.State)
	}
	if res.Windows != nil {
		t.Errorf("Windows = %+v, want nil", res.Windows)
	}
	if hits.Load() != 0 {
		t.Errorf("upstream hits = %d, want 0", hits.Load())
	}
}

func TestServiceState2KeyAbsent(t *testing.T) {
	var hits atomic.Int64
	s := newTestService(t, `{"mcpOAuth":{}}`, countingHandler(&hits, serve200(liveUsageJSON)))
	res := s.Get(context.Background(), false)
	if res.State != "no_credentials" {
		t.Errorf("State = %q, want no_credentials", res.State)
	}
	if hits.Load() != 0 {
		t.Errorf("upstream hits = %d, want 0", hits.Load())
	}
}

func TestServiceState3LocallyExpired(t *testing.T) {
	var hits atomic.Int64
	creds := `{"claudeAiOauth":{"accessToken":"` + fakeToken() + `","expiresAt":1}}`
	s := newTestService(t, creds, countingHandler(&hits, serve200(liveUsageJSON)))
	res := s.Get(context.Background(), false)
	if res.State != "auth_expired" {
		t.Errorf("State = %q, want auth_expired", res.State)
	}
	if res.Windows != nil {
		t.Errorf("Windows = %+v, want nil", res.Windows)
	}
	if hits.Load() != 0 {
		t.Errorf("upstream hits = %d, want 0 (never burn a call on a guaranteed 401)", hits.Load())
	}
}

func TestServiceState4Backoff429(t *testing.T) {
	var hits atomic.Int64
	s := newTestService(t, validCreds(fakeToken()), countingHandler(&hits, func(w http.ResponseWriter, r *http.Request) {
		if hits.Load() == 1 {
			serve200(liveUsageJSON)(w, r)
			return
		}
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	advance := fakeClock(s)

	if res := s.Get(context.Background(), false); res.State != "ok" {
		t.Fatalf("Get1 State = %q, want ok", res.State)
	}
	advance(61 * time.Second)
	res := s.Get(context.Background(), false) // hits upstream → 429
	if !res.Stale || len(res.Windows) != 3 {
		t.Errorf("Get2 after 429 = {State %q Stale %v %d windows}, want stale cached windows", res.State, res.Stale, len(res.Windows))
	}
	if hits.Load() != 2 {
		t.Fatalf("upstream hits = %d, want 2", hits.Load())
	}
	advance(31 * time.Second) // still inside the 60s Retry-After backoff
	res = s.Get(context.Background(), false)
	if hits.Load() != 2 {
		t.Errorf("upstream hits during backoff = %d, want 2 (no re-request before backoffUntil)", hits.Load())
	}
	if !res.Stale || len(res.Windows) != 3 {
		t.Errorf("Get3 during backoff = {Stale %v %d windows}, want stale cached", res.Stale, len(res.Windows))
	}
}

func TestServiceState5ServerErrorsStaleThenDrop(t *testing.T) {
	var hits atomic.Int64
	s := newTestService(t, validCreds(fakeToken()), countingHandler(&hits, func(w http.ResponseWriter, r *http.Request) {
		if hits.Load() == 1 {
			serve200(liveUsageJSON)(w, r)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	advance := fakeClock(s)

	if res := s.Get(context.Background(), false); res.State != "ok" {
		t.Fatalf("Get1 State = %q, want ok", res.State)
	}
	for i, wantStale := range []bool{true, true} { // failures 1 and 2 → stale
		advance(61 * time.Second)
		res := s.Get(context.Background(), false)
		if res.State != "error" || res.Stale != wantStale || len(res.Windows) != 3 {
			t.Errorf("Get%d = {State %q Stale %v %d windows}, want {error true 3}", i+2, res.State, res.Stale, len(res.Windows))
		}
	}
	advance(61 * time.Second)
	res := s.Get(context.Background(), false) // third consecutive failure → drop
	if res.State != "error" || res.Windows != nil {
		t.Errorf("Get4 = {State %q Windows %+v}, want {error nil}", res.State, res.Windows)
	}
}

func TestServiceState6MalformedBody(t *testing.T) {
	var hits atomic.Int64
	s := newTestService(t, validCreds(fakeToken()), countingHandler(&hits, func(w http.ResponseWriter, r *http.Request) {
		if hits.Load() == 1 {
			serve200(liveUsageJSON)(w, r)
			return
		}
		serve200("not json")(w, r)
	}))
	advance := fakeClock(s)

	if res := s.Get(context.Background(), false); res.State != "ok" {
		t.Fatalf("Get1 State = %q, want ok", res.State)
	}
	advance(61 * time.Second)
	if res := s.Get(context.Background(), false); res.State != "error" || !res.Stale || len(res.Windows) != 3 {
		t.Errorf("Get2 = {State %q Stale %v %d windows}, want stale error with cache", res.State, res.Stale, len(res.Windows))
	}
	advance(61 * time.Second)
	s.Get(context.Background(), false)
	advance(61 * time.Second)
	if res := s.Get(context.Background(), false); res.State != "error" || res.Windows != nil {
		t.Errorf("Get4 = {State %q Windows %+v}, want dropped {error nil}", res.State, res.Windows)
	}
}

func TestServiceUpstream401(t *testing.T) {
	t.Run("no prior cache", func(t *testing.T) {
		s := newTestService(t, validCreds(fakeToken()), func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		})
		res := s.Get(context.Background(), false)
		if res.State != "auth_expired" || res.Windows != nil {
			t.Errorf("Get = {State %q Windows %+v}, want {auth_expired nil}", res.State, res.Windows)
		}
	})

	t.Run("with prior cache: stale twice then dropped", func(t *testing.T) {
		var hits atomic.Int64
		s := newTestService(t, validCreds(fakeToken()), countingHandler(&hits, func(w http.ResponseWriter, r *http.Request) {
			if hits.Load() == 1 {
				serve200(liveUsageJSON)(w, r)
				return
			}
			w.WriteHeader(http.StatusUnauthorized)
		}))
		advance := fakeClock(s)
		if res := s.Get(context.Background(), false); res.State != "ok" {
			t.Fatalf("Get1 State = %q, want ok", res.State)
		}
		for i := 0; i < 2; i++ {
			advance(61 * time.Second)
			res := s.Get(context.Background(), false)
			if res.State != "auth_expired" || !res.Stale || len(res.Windows) != 3 {
				t.Errorf("Get%d = {State %q Stale %v %d windows}, want stale auth_expired with cache", i+2, res.State, res.Stale, len(res.Windows))
			}
		}
		advance(61 * time.Second)
		if res := s.Get(context.Background(), false); res.State != "auth_expired" || res.Windows != nil {
			t.Errorf("Get4 = {State %q Windows %+v}, want dropped {auth_expired nil}", res.State, res.Windows)
		}
	})
}

func TestServiceTokenChangeDropsCache(t *testing.T) {
	var hits atomic.Int64
	tokenA := fakeToken()
	tokenB := "sk-ant-oat01-" + strings.Repeat("y", 95)
	s := newTestService(t, validCreds(tokenA), countingHandler(&hits, func(w http.ResponseWriter, r *http.Request) {
		switch hits.Load() {
		case 1:
			serve200(liveUsageJSON)(w, r)
		case 2:
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			serve200(liveUsageJSON)(w, r)
		}
	}))
	advance := fakeClock(s)

	if res := s.Get(context.Background(), false); res.State != "ok" {
		t.Fatalf("Get1 State = %q, want ok", res.State)
	}

	// Switch accounts: rewrite the creds file with a different token.
	if err := os.WriteFile(s.credsPath, []byte(validCreds(tokenB)), 0o600); err != nil {
		t.Fatalf("rewrite creds: %v", err)
	}
	advance(61 * time.Second)
	// Refetch fails (503) — but token A's windows must NOT be served (QUOTA-08).
	res := s.Get(context.Background(), false)
	if res.Windows != nil {
		t.Errorf("Get2 after token change served old windows %+v, want nil", res.Windows)
	}
	if res.State != "error" {
		t.Errorf("Get2 State = %q, want error (fresh failure under new token)", res.State)
	}
	advance(61 * time.Second)
	// Next fetch succeeds — failures were reset by the token change.
	if res := s.Get(context.Background(), false); res.State != "ok" || res.Stale {
		t.Errorf("Get3 = {State %q Stale %v}, want fresh ok", res.State, res.Stale)
	}
}

func TestServiceHappyPathHeaders(t *testing.T) {
	token := fakeToken()
	var gotUA, gotBeta, gotAuth string
	s := newTestService(t, validCreds(token), func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		gotBeta = r.Header.Get("anthropic-beta")
		gotAuth = r.Header.Get("Authorization")
		serve200(liveUsageJSON)(w, r)
	})
	res := s.Get(context.Background(), false)
	if res.State != "ok" || res.Stale || len(res.Windows) != 3 || res.FetchedAt == nil {
		t.Errorf("Get = {State %q Stale %v %d windows FetchedAt %v}, want fresh ok with 3 windows", res.State, res.Stale, len(res.Windows), res.FetchedAt)
	}
	if gotUA != "claude-code/test" {
		t.Errorf("User-Agent = %q, want claude-code/test", gotUA)
	}
	if gotBeta != "oauth-2025-04-20" {
		t.Errorf("anthropic-beta = %q, want oauth-2025-04-20", gotBeta)
	}
	if gotAuth != "Bearer "+token {
		t.Errorf("Authorization = %q, want Bearer <fake token>", gotAuth)
	}
}

func TestServiceTTLAndFloor(t *testing.T) {
	var hits atomic.Int64
	s := newTestService(t, validCreds(fakeToken()), countingHandler(&hits, serve200(liveUsageJSON)))
	advance := fakeClock(s)

	s.Get(context.Background(), false)
	if hits.Load() != 1 {
		t.Fatalf("hits after Get1 = %d, want 1", hits.Load())
	}
	advance(30 * time.Second)
	s.Get(context.Background(), false) // within 60s TTL → cache
	if hits.Load() != 1 {
		t.Errorf("hits after Get2 (within TTL) = %d, want 1", hits.Load())
	}
	s.Get(context.Background(), true) // force, 30s past last attempt → fetch
	if hits.Load() != 2 {
		t.Errorf("hits after Get3 (force past floor) = %d, want 2", hits.Load())
	}
	advance(5 * time.Second)
	s.Get(context.Background(), true) // force WITHIN the 10s floor → no fetch
	if hits.Load() != 2 {
		t.Errorf("hits after Get4 (force within floor) = %d, want 2 (floor binds attempts)", hits.Load())
	}
}

func TestServiceRecoveryAfterAuthExpired(t *testing.T) {
	creds := `{"claudeAiOauth":{"accessToken":"` + fakeToken() + `","expiresAt":1}}`
	s := newTestService(t, creds, serve200(liveUsageJSON))
	if res := s.Get(context.Background(), false); res.State != "auth_expired" {
		t.Fatalf("Get1 State = %q, want auth_expired", res.State)
	}
	// A running claude session refreshed the file underneath (Pitfall 6).
	if err := os.WriteFile(s.credsPath, []byte(validCreds(fakeToken())), 0o600); err != nil {
		t.Fatalf("rewrite creds: %v", err)
	}
	if res := s.Get(context.Background(), false); res.State != "ok" || len(res.Windows) != 3 {
		t.Errorf("Get2 = {State %q %d windows}, want recovered ok (no special casing)", res.State, len(res.Windows))
	}
}

func TestDetectVersionFallback(t *testing.T) {
	// A binary that does not exist → pinned fallback.
	got := DetectVersion(filepath.Join(t.TempDir(), "no-such-claude"))
	if got != fallbackClaudeVersion {
		t.Errorf("DetectVersion(missing) = %q, want %q", got, fallbackClaudeVersion)
	}
}

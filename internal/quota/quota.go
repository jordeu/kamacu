// Package quota proxies Anthropic's undocumented OAuth usage endpoint as a
// strictly best-effort, demand-driven quota indicator. Kangent is a read-only
// passenger on ~/.claude/.credentials.json: the file is re-read on every poll,
// never written, and the token never outlives a single request, never appears
// in logs, and never crosses the Kangent API (the browser receives digested
// numbers only).
package quota

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Window is one normalized usage window. The JSON tags ARE the /api/usage
// response contract — the frontend renders these fields verbatim.
type Window struct {
	Key         string  `json:"key"`
	Label       string  `json:"label"`
	Utilization float64 `json:"utilization"`
	ResetsAt    *string `json:"resetsAt"` // ISO 8601 passthrough, CAN be null
}

// Result is the full /api/usage payload: a state enum plus last-good windows.
type Result struct {
	State     string     `json:"state"` // "ok" | "no_credentials" | "auth_expired" | "error"
	Stale     bool       `json:"stale"`
	FetchedAt *time.Time `json:"fetchedAt"` // last SUCCESSFUL fetch; null until first success
	Windows   []Window   `json:"windows"`   // null when no data to show
}

// credsFile decodes ONLY the two fields Kangent needs. The file also carries
// unrelated mcpOAuth third-party secrets — never decode, hold, or log the rest.
type credsFile struct {
	ClaudeAiOauth struct {
		AccessToken string `json:"accessToken"`
		ExpiresAt   int64  `json:"expiresAt"` // epoch MILLISECONDS (verified: 1781238586291)
	} `json:"claudeAiOauth"`
}

// errNoCredentials means there is nothing to poll with: file absent,
// unreadable, malformed, claudeAiOauth key absent, or empty token. This is a
// first-class state (API-key-only users), not an error to surface.
var errNoCredentials = errors.New("no oauth credentials")

// readCredentials extracts the access token and its expiry from the claude
// credentials file at path. Called fresh on every poll — claude rewrites the
// file constantly and rotates the token (~8h), so caching it would go stale.
func readCredentials(path string) (token string, expiresAt time.Time, err error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", time.Time{}, errNoCredentials
	}
	var c credsFile
	if err := json.Unmarshal(raw, &c); err != nil {
		return "", time.Time{}, errNoCredentials
	}
	if c.ClaudeAiOauth.AccessToken == "" {
		return "", time.Time{}, errNoCredentials
	}
	return c.ClaudeAiOauth.AccessToken, time.UnixMilli(c.ClaudeAiOauth.ExpiresAt), nil
}

// usageWindow is the lenient per-window upstream shape. Pointer fields:
// resets_at CAN be null and whole windows CAN be null (both observed live).
type usageWindow struct {
	Utilization *float64 `json:"utilization"`
	ResetsAt    *string  `json:"resets_at"`
}

// windowOrder is the fixed preference order (D-75 — rows must not jump
// between polls); windowLabels carries the full short names (D-74).
var windowOrder = [...]string{"five_hour", "seven_day", "seven_day_opus", "seven_day_sonnet"}

var windowLabels = map[string]string{
	"five_hour":        "5h",
	"seven_day":        "7d",
	"seven_day_opus":   "Opus",
	"seven_day_sonnet": "Sonnet",
}

// normalizeWindows decodes the upstream usage body into ordered windows:
// known keys first in preference order, then remaining keys alphabetically
// with raw-key labels (D-74 passthrough). Whole-null windows, keys whose
// shape lacks utilization, and the literal "extra_usage" block (deferred
// QUOTA-FUT-01 — its shape may grow a utilization field someday) are skipped.
func normalizeWindows(body []byte) ([]Window, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, err
	}
	delete(m, "extra_usage")

	var out []Window
	emit := func(key string) {
		raw, ok := m[key]
		if !ok {
			return
		}
		delete(m, key)
		var uw usageWindow
		if err := json.Unmarshal(raw, &uw); err != nil || uw.Utilization == nil {
			return // null window, noise key, or non-window shape
		}
		label, ok := windowLabels[key]
		if !ok {
			label = key // raw-key passthrough for future windows
		}
		out = append(out, Window{
			Key:         key,
			Label:       label,
			Utilization: *uw.Utilization,
			ResetsAt:    uw.ResetsAt,
		})
	}

	for _, k := range windowOrder {
		emit(k)
	}
	rest := make([]string, 0, len(m))
	for k := range m {
		rest = append(rest, k)
	}
	sort.Strings(rest)
	for _, k := range rest {
		emit(k)
	}
	return out, nil
}

// Config is the public construction seam. Zero values select production
// defaults; tests inject a temp credentials path, an httptest base URL, and
// a fixed UserAgent (never spawning claude).
type Config struct {
	CredentialsPath string           // default: ~/.claude/.credentials.json
	BaseURL         string           // default: "https://api.anthropic.com"
	UserAgent       string           // if "" → probe via DetectVersion(ClaudeBin)
	ClaudeBin       string           // -claude-bin flag value; "" → exec.LookPath("claude")
	Now             func() time.Time // default: time.Now; test seam for TTL/floor control
}

// Service is the demand-driven, token-keyed quota cache. There is no ticker
// goroutine: the browser's poll is the only trigger, so an idle server makes
// zero upstream requests.
type Service struct {
	mu        sync.Mutex
	credsPath string
	baseURL   string
	userAgent string
	client    *http.Client
	now       func() time.Time // time.Now; override in tests

	// cache state (all guarded by mu)
	cached       []Window
	tokenFP      string    // hex sha256 of the current token (QUOTA-08)
	fetchedAt    time.Time // zero until first success; drives 60s TTL and FetchedAt
	lastAttempt  time.Time // set on EVERY upstream attempt; drives the 10s hard floor
	backoffUntil time.Time // 429: now + max(Retry-After, 30s)
	failures     int       // consecutive; >= 3 ⇒ drop cached (QUOTA-06)
	lastErr      string    // "" | "auth_expired" | "error" — picks the dropped/stale state
	inflight     bool      // in-flight dedup
}

// New builds a Service from cfg, applying production defaults for zero
// values. The UA probe runs at most once per process, here.
func New(cfg Config) *Service {
	credsPath := cfg.CredentialsPath
	if credsPath == "" {
		if home, err := os.UserHomeDir(); err == nil {
			credsPath = filepath.Join(home, ".claude", ".credentials.json")
		}
		// Unresolvable home leaves credsPath "" — os.ReadFile("") always
		// errors, degrading every Get to no_credentials by design.
	}
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	userAgent := cfg.UserAgent
	if userAgent == "" {
		userAgent = "claude-code/" + DetectVersion(cfg.ClaudeBin)
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Service{
		credsPath: credsPath,
		baseURL:   baseURL,
		userAgent: userAgent,
		client:    &http.Client{Timeout: 10 * time.Second},
		now:       now,
	}
}

const (
	cacheTTL      = 60 * time.Second // gates re-fetch after a SUCCESS
	attemptFloor  = 10 * time.Second // gates every attempt, even ?refresh=1
	minBackoff429 = 30 * time.Second
	maxFailures   = 3 // consecutive failures before cached windows drop
)

// Get returns the current quota Result, fetching upstream only when the
// demand-driven cache rules allow it. force corresponds to ?refresh=1.
func (s *Service) Get(ctx context.Context, force bool) Result {
	s.mu.Lock()

	// 1. Re-read credentials EVERY call — claude rewrites the file
	// constantly and rotates the token (~8h). Never cached, never written.
	token, expiresAt, err := readCredentials(s.credsPath)
	if err != nil {
		// errNoCredentials (and nothing else can come back): first-class
		// state, no upstream call ever (D-71).
		s.mu.Unlock()
		return Result{State: "no_credentials"}
	}

	now := s.now()

	// 2. Token change (QUOTA-08): never serve one account's windows under
	// another account's token.
	fp := sha256Hex(token)
	if fp != s.tokenFP {
		s.cached = nil
		s.fetchedAt = time.Time{}
		s.failures = 0
		s.lastErr = ""
		s.tokenFP = fp
	}

	// 3. Locally expired token: behave as a 401 WITHOUT burning an upstream
	// request on a guaranteed failure (Pitfall 6).
	if expiresAt.Before(now) {
		s.recordFailureLocked("auth_expired")
		res := s.resultLocked()
		s.mu.Unlock()
		return res
	}

	// 4. Serve the cache without fetching when any gate holds. The 10s
	// floor binds lastAttempt (attempts); the 60s TTL binds fetchedAt
	// (successes) — separate fields, or a failing upstream gets hammered
	// by the 60s poll (Pitfall 3).
	if (!force && s.cached != nil && now.Sub(s.fetchedAt) < cacheTTL) ||
		now.Before(s.backoffUntil) ||
		now.Sub(s.lastAttempt) < attemptFloor ||
		s.inflight {
		res := s.resultLocked()
		s.mu.Unlock()
		return res
	}

	// 5. Fetch. Release mu during the HTTP call; inflight dedups concurrent
	// Gets onto the cache meanwhile.
	s.inflight = true
	s.lastAttempt = now
	s.mu.Unlock()

	status, body, retryAfter, fetchErr := s.fetchUpstream(ctx, token)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.inflight = false

	switch {
	case fetchErr != nil:
		s.recordFailureLocked("error")
	case status == http.StatusOK:
		windows, err := normalizeWindows(body)
		if err != nil {
			s.recordFailureLocked("error")
			break
		}
		s.cached = windows
		s.fetchedAt = s.now()
		s.failures = 0
		s.lastErr = ""
	case status == http.StatusUnauthorized:
		s.recordFailureLocked("auth_expired")
	case status == http.StatusTooManyRequests:
		s.backoffUntil = s.now().Add(retryAfter)
		s.recordFailureLocked("error")
	default:
		s.recordFailureLocked("error")
	}

	return s.resultLocked()
}

// fetchUpstream performs the single authenticated GET. It returns the status
// code, the body (200 only), and the 429 backoff duration (429 only). Error
// values never contain the token or any request material — status codes only
// (Pitfall 4).
func (s *Service) fetchUpstream(ctx context.Context, token string) (int, []byte, time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+"/api/oauth/usage", nil)
	if err != nil {
		return 0, nil, 0, errors.New("building usage request failed")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", s.userAgent)

	resp, err := s.client.Do(req)
	if err != nil {
		slog.Debug("quota upstream request failed", "kind", "network")
		return 0, nil, 0, errors.New("usage request failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Truncate-and-discard: never read error bodies into memory beyond
		// drain, never log them.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		slog.Debug("quota upstream non-200", "status", resp.StatusCode)
		var retryAfter time.Duration
		if resp.StatusCode == http.StatusTooManyRequests {
			retryAfter = parseRetryAfter(resp.Header.Get("Retry-After"))
		}
		return resp.StatusCode, nil, retryAfter, nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return 0, nil, 0, fmt.Errorf("reading usage response: status %d", resp.StatusCode)
	}
	return http.StatusOK, body, 0, nil
}

// parseRetryAfter maps a Retry-After header to a backoff duration of at
// least 30s; missing/unparseable values fall back to the 30s minimum.
func parseRetryAfter(h string) time.Duration {
	secs, err := strconv.Atoi(strings.TrimSpace(h))
	if err != nil || time.Duration(secs)*time.Second < minBackoff429 {
		return minBackoff429
	}
	return time.Duration(secs) * time.Second
}

// recordFailureLocked increments the consecutive-failure counter, remembers
// which degraded state to report, and drops the cached windows once
// maxFailures is reached (QUOTA-06). Caller holds mu.
func (s *Service) recordFailureLocked(state string) {
	s.failures++
	s.lastErr = state
	slog.Debug("quota fetch degraded", "state", state, "failures", s.failures)
	if s.failures >= maxFailures {
		s.cached = nil
	}
}

// resultLocked materializes the Result for the current cache state. Caller
// holds mu.
func (s *Service) resultLocked() Result {
	state := "ok"
	if s.lastErr != "" {
		state = s.lastErr
	}
	res := Result{State: state, Stale: s.lastErr != "" && s.cached != nil}
	if s.cached != nil {
		res.Windows = s.cached
		ft := s.fetchedAt
		res.FetchedAt = &ft
	}
	return res
}

// sha256Hex fingerprints a token for cache keying without retaining it.
func sha256Hex(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// fallbackClaudeVersion pins a known-good claude version for the User-Agent
// when probing fails. claude --version output 2026-06-12; bump
// opportunistically — the UA bucket is load-bearing for rate limits.
const fallbackClaudeVersion = "2.1.174"

var versionRe = regexp.MustCompile(`^\d+\.\d+\.\d+`)

// DetectVersion probes `claude --version` (the -claude-bin flag value, or
// "claude" on PATH) with a 5s timeout and returns the leading semver token
// of its stdout. Any failure — missing binary, timeout, parse miss — yields
// the pinned fallback. Called once per process from New.
func DetectVersion(claudeBin string) string {
	bin := claudeBin
	if bin == "" {
		p, err := exec.LookPath("claude")
		if err != nil {
			return fallbackClaudeVersion
		}
		bin = p
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "--version").Output()
	if err != nil {
		return fallbackClaudeVersion
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 || !versionRe.MatchString(fields[0]) {
		return fallbackClaudeVersion
	}
	return fields[0]
}

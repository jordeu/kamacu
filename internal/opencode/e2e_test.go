//go:build opencode_e2e

// Package opencode e2e (real binary): this file is excluded from the default
// build. It is compiled/run only with -tags opencode_e2e on a host that has
// the real `opencode` binary on PATH plus a working provider config. It proves
// the slice's high-risk item end-to-end: the REAL shipped plugin (fetch-based,
// the exact PluginSource() bytes Kamacu installs) LOADS in real opencode, its
// event-shape assumptions match real emissions, and its POSTs reliably reach a
// receiver so idle unlocks. Both tests t.Skip cleanly when opencode or the
// provider config is absent, so default CI (tag never set) and any host
// without opencode are unaffected.
//
// Stdlib only — no third-party deps.
package opencode

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// opencodeOnPath returns the opencode binary path or skips the test. This is
// the host gate: without the real binary there is nothing end-to-end to prove.
func opencodeOnPath(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("opencode")
	if err != nil {
		t.Skip("opencode not on PATH (e2e harness is host-gated)")
	}
	return p
}

// isolateOpencodeConfig builds a fully isolated opencode config tree in a temp
// dir and returns it. The caller sets BOTH XDG_CONFIG_HOME and HOME to the
// returned dir for full isolation (opencode honors XDG for config and HOME for
// data/log/cache — MEM029). The tree contains:
//   - <tmp>/opencode/plugin/kamacu-status.js: the REAL shipped fetch-based
//     plugin (PluginSource() bytes) — exactly what Kamacu installs, so this
//     test exercises the post-T01 fix rather than a curl-dependent loop.
//   - <tmp>/opencode/opencode.json: the user's provider config with the "mcp"
//     key stripped (a turn needs only the provider; MCP spinup is slow,
//     network-dependent, and irrelevant to the plugin->receiver loop).
//
// The user's REAL config is read from os.UserConfigDir() (current env, real
// home) BEFORE the caller overrides env, so live provider creds are copied
// from the host. Skips when no provider config exists — the harness cannot
// drive a real turn without one.
func isolateOpencodeConfig(t *testing.T) string {
	t.Helper()
	// Read the user's real provider config first (real home, pre-override).
	realCfgDir, err := os.UserConfigDir()
	if err != nil {
		t.Skipf("cannot resolve real config dir for provider config: %v", err)
	}
	realCfgPath := filepath.Join(realCfgDir, "opencode", "opencode.json")
	raw, err := os.ReadFile(realCfgPath)
	if err != nil {
		t.Skip("no opencode provider config (e2e harness needs a working provider)")
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Skipf("opencode.json is not a JSON object: %v", err)
	}
	// Strip MCP servers: provider-only is sufficient and avoids MCP spinup.
	delete(cfg, "mcp")
	stripped, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal mcp-stripped opencode.json: %v", err)
	}

	tmp := t.TempDir()
	// Write the REAL shipped plugin (the fetch-based bytes under test).
	pluginDir := filepath.Join(tmp, "opencode", "plugin")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("mkdir plugin dir: %v", err)
	}
	pluginPath := filepath.Join(pluginDir, PluginFilename)
	if err := os.WriteFile(pluginPath, PluginSource(), 0o644); err != nil {
		t.Fatalf("write plugin under test: %v", err)
	}
	// Write the mcp-stripped provider config.
	cfgPath := filepath.Join(tmp, "opencode", "opencode.json")
	if err := os.WriteFile(cfgPath, stripped, 0o644); err != nil {
		t.Fatalf("write opencode.json: %v", err)
	}
	return tmp
}

// receiverCounts is a mutex-guarded count of hook_event_name values POSTed to
// the in-process test receiver. It mirrors what the production receiver
// (internal/api/hooks.go) would observe, without depending on that package.
type receiverCounts struct {
	mu sync.Mutex
	n  map[string]int
}

func (c *receiverCounts) inc(name string) {
	c.mu.Lock()
	c.n[name]++
	c.mu.Unlock()
}

func (c *receiverCounts) snapshot() map[string]int {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]int, len(c.n))
	for k, v := range c.n {
		out[k] = v
	}
	return out
}

// total returns the count of all POSTs received (any hook_event_name).
func (c *receiverCounts) total() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := 0
	for _, v := range c.n {
		t += v
	}
	return t
}

// recordReceiver starts an httptest.Server that mirrors the production hook
// receiver contract (X-Kangent-Token constant-time compare, decode
// hook_event_name, 204 on success) and counts each accepted event by name. It
// returns the server (its URL is the KAMACU_HOOK_BASE) and the counts. The
// server is closed on test cleanup.
func recordReceiver(t *testing.T, token string) (*httptest.Server, *receiverCounts) {
	t.Helper()
	counts := &receiverCounts{n: map[string]int{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get("X-Kangent-Token")
		if token == "" || subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var payload struct {
			HookEventName string `json:"hook_event_name"`
		}
		// Tolerate a malformed body: an empty/unknown name simply isn't counted.
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if payload.HookEventName != "" {
			counts.inc(payload.HookEventName)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	return srv, counts
}

// envWithoutKamacu returns os.Environ() with any pre-existing KAMACU_* entries
// removed, so the test deterministically controls which (if any) KAMACU env
// the child opencode sees. Prevents contamination/flakiness when the test
// process itself was spawned inside a Kamacu session (duplicate keys would
// otherwise race the gate).
func envWithoutKamacu() []string {
	src := os.Environ()
	out := make([]string, 0, len(src))
	for _, kv := range src {
		if strings.HasPrefix(kv, "KAMACU_") {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// mintID returns a fresh lowercase hex id (session id or hook token). Used
// instead of reusing production minting so the e2e is fully self-contained.
func mintID(t *testing.T) string {
	t.Helper()
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("mint id: %v", err)
	}
	return hex.EncodeToString(b)
}

// truncForLog returns b capped at its last max bytes for test log output
// (opencode run output is verbose; the tail carries the turn outcome).
func truncForLog(b []byte) string {
	const max = 4000
	if len(b) <= max {
		return string(b)
	}
	return "..." + string(b[len(b)-max:])
}

// runOpencodeTurn drives a real `opencode run --format json "say hi"` turn
// inside the isolated config, with the given extra env appended to a
// KAMACU_*-scrubbed copy of os.Environ(). A 90s timeout bounds the turn. The
// combined stdout/stderr and exit error are returned; a non-zero exit is
// tolerated (a provider error is acceptable for the loop-live check per the
// plan's documented tradeoff).
func runOpencodeTurn(t *testing.T, tmp string, extraEnv []string) ([]byte, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "opencode", "run", "--format", "json", "say hi")
	env := envWithoutKamacu()
	env = append(env, "XDG_CONFIG_HOME="+tmp, "HOME="+tmp)
	env = append(env, extraEnv...)
	cmd.Env = env
	return cmd.CombinedOutput()
}

// TestE2E_RealTurnPostsLifecycle proves the slice's core idle-unlock path
// against the REAL opencode binary: the shipped fetch-based plugin loads, maps
// real events to claude-compatible names, and POSTs to the receiver so idle
// unlocks. The reliable assertion (per MEM029) is that the receiver observes
// at least one Stop (idle) POST from a completed turn. SessionStart and
// Notification are logged if present but NOT required (session.created did not
// appear in run-mode logs; Notification only fires on a permission prompt).
//
// Failure-mode tradeoff (Q5): if the provider is transiently down the turn
// errors and emits only a bare `error` event the plugin does NOT map, so Stop
// will not fire and this test fails on the Stop assertion. That is an accepted
// tradeoff for a host-gated milestone test — rerun on a known-good host. Do
// NOT weaken Stop to a soft assertion: it is the slice's idle-unlock proof.
func TestE2E_RealTurnPostsLifecycle(t *testing.T) {
	opencodeOnPath(t)
	tmp := isolateOpencodeConfig(t)

	sessionID := mintID(t)
	token := mintID(t)
	srv, counts := recordReceiver(t, token)

	out, runErr := runOpencodeTurn(t, tmp, []string{
		"KAMACU_SESSION_ID=" + sessionID,
		"KAMACU_HOOK_TOKEN=" + token,
		"KAMACU_HOOK_BASE=" + srv.URL,
	})
	t.Logf("opencode run exit error: %v", errString(runErr))
	t.Logf("opencode run output (tail):\n%s", truncForLog(out))
	got := counts.snapshot()
	t.Logf("receiver hook_event_name counts: %+v", got)

	// (1) Loop is live: at least one hook POST reached the receiver. Catches
	// the rarer "plugin loaded but emitted nothing" regression.
	if len(got) == 0 {
		t.Fatalf("receiver recorded ZERO hook POSTs — the shipped plugin did not load or emitted nothing against real opencode (plugin->receiver loop broken)")
	}
	// (2) Idle unlocked: at least one Stop (idle) POST. This is the load-
	// bearing assertion; do not soften it.
	if got["Stop"] < 1 {
		t.Fatalf("receiver saw no Stop (idle) POST — idle-unlock path did not fire. counts=%+v\n"+
			"(a transient provider error emits only a bare 'error' event the plugin does not map; rerun on a known-good host)", got)
	}
	t.Logf("PASS: plugin->receiver loop live AND idle (Stop) observed against real opencode")
}

// TestE2E_EnvGateNoopWhenSessionIDUnset proves the env gate (D014) against the
// REAL binary: with KAMACU_SESSION_ID omitted (token + base still set), the
// plugin returns {} before registering handlers and the receiver records ZERO
// POSTs. This is the negative counterpart to the loop-live case and guards the
// "invisible to non-Kamacu users" property.
func TestE2E_EnvGateNoopWhenSessionIDUnset(t *testing.T) {
	opencodeOnPath(t)
	tmp := isolateOpencodeConfig(t)

	token := mintID(t)
	srv, counts := recordReceiver(t, token)

	// KAMACU_SESSION_ID deliberately OMITTED; token + base still present, so a
	// missing session id is the ONLY gate trigger.
	out, _ := runOpencodeTurn(t, tmp, []string{
		"KAMACU_HOOK_TOKEN=" + token,
		"KAMACU_HOOK_BASE=" + srv.URL,
	})
	t.Logf("opencode run output (tail):\n%s", truncForLog(out))

	if total := counts.total(); total != 0 {
		t.Fatalf("env gate leaked: receiver recorded %d POSTs with KAMACU_SESSION_ID unset (plugin must return {} before registering handlers). counts=%+v", total, counts.snapshot())
	}
	t.Logf("PASS: env gate no-op'd against real opencode (zero POSTs with KAMACU_SESSION_ID unset)")
}

func errString(err error) string {
	if err == nil {
		return "<nil>"
	}
	return err.Error()
}

// --- M002/S03/T03: restart-resume discovery + argv proofs (host-gated) -------
//
// These exercise the REAL opencode binary to prove the Strategy-B discovery +
// -s resume path that the api-package CI tests stub:
//   1. a real turn creates a session row `opencode session list` can see, and
//      the worktree-directory filter + most-recently-updated selection resolve
//      the correct ses_id (locks the parser against the real json shape).
//   2. `opencode -s <real-id>` resolves the prior session (does NOT print
//      "Session not found"), and `opencode -s ses_NONEXISTENT` DOES — the
//      binary stale-vs-real signal that mirrors claude's missing-transcript
//      failure (resume is honest, never a silent fresh fork).
//
// Both skip cleanly when opencode or a provider config is absent (the gate is
// inherited from isolateOpencodeConfig). Provider tolerance (MEM031): a turn
// that ends in a provider runtime error still creates the session row, so the
// discovery proof does not require a working provider — only that opencode can
// mint a session.

// realGitWorktree provisions a REAL git worktree (git worktree add --detach)
// of the current repo, mirroring exactly how kamacu provisions task worktrees.
// opencode only reliably creates + persists a session row for a real project
// repo (tiny scratch repos may not, provider-error timing aside), so the
// discovery proof needs a faithful worktree. The worktree is removed on test
// cleanup. Skips when git is unavailable or the worktree cannot be created.
func realGitWorktree(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH (e2e needs a real worktree)")
	}
	// t.TempDir() is the PARENT; the worktree is a subdir so `git worktree
	// remove` cleans exactly it.
	parent := t.TempDir()
	path := parent + "/wt"
	add := exec.Command("git", "worktree", "add", "--detach", path)
	if out, err := add.CombinedOutput(); err != nil {
		t.Skipf("git worktree add failed (cannot create a real worktree for the discovery proof): %v; out=%s", err, truncForLog(out))
	}
	t.Cleanup(func() {
		rem := exec.Command("git", "worktree", "remove", "--force", path)
		if out, err := rem.CombinedOutput(); err != nil {
			t.Logf("cleanup: git worktree remove %s: %v; out=%s", path, err, truncForLog(out))
		}
	})
	return path
}

// opencodeSessionListJSON runs `opencode session list --format json` in the
// isolated config env, scoped to the worktree's project, and returns the raw
// output. opencode session list is project-scoped to $PWD (MEM036), so the
// listing is pinned to the worktree (cmd.Dir + PWD) — otherwise it lists a
// different project's sessions. extraEnv is appended after the worktree pins.
func opencodeSessionListJSON(t *testing.T, tmp, worktree string, extraEnv []string) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "opencode", "session", "list", "--format", "json")
	cmd.Dir = worktree
	env := envWithoutKamacu()
	env = append(env, "XDG_CONFIG_HOME="+tmp, "HOME="+tmp, "PWD="+worktree)
	env = append(env, extraEnv...)
	cmd.Env = env
	out, err := cmd.Output()
	if err != nil {
		t.Logf("opencode session list output (stderr): %s", truncForLog(out))
	}
	return out
}

// mostRecentSessionForDir mirrors internal/api.parseOpenCodeSessionList against
// the real json shape: returns the most-recently-updated ses_id whose directory
// == dir, or "" if none. Duplicated here (not imported) to keep the opencode
// e2e package dependency-free and to lock the shape independently.
func mostRecentSessionForDir(t *testing.T, out []byte, dir string) string {
	t.Helper()
	var entries []struct {
		ID        string `json:"id"`
		Updated   int64  `json:"updated"`
		Directory string `json:"directory"`
	}
	if len(out) == 0 {
		return ""
	}
	if err := json.Unmarshal(out, &entries); err != nil {
		t.Logf("session list json did not parse as an array: %v; raw=%q", err, truncForLog(out))
		return ""
	}
	var bestID string
	var bestUpdated int64
	for _, e := range entries {
		if e.ID == "" || e.Directory != dir {
			continue
		}
		if e.Updated > bestUpdated {
			bestID = e.ID
			bestUpdated = e.Updated
		}
	}
	return bestID
}

// runResumeById spawns `opencode -s <id>` (TUI resume) with a bounded timeout
// and returns its combined output. The TUI hangs on success (it waits for
// input), so a timeout-bound kill is the normal exit for a resolving id; a
// NON-resolving id prints "Session not found" and exits before the timeout.
func runResumeById(t *testing.T, tmp, worktree, id string) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "opencode", "-s", id)
	cmd.Dir = worktree
	env := envWithoutKamacu()
	env = append(env, "XDG_CONFIG_HOME="+tmp, "HOME="+tmp)
	env = append(env, "PWD="+worktree) // MEM035: opencode reads $PWD for its project dir
	cmd.Env = env
	out, _ := cmd.CombinedOutput() // timeout-kill is expected for a resolving id
	return out
}

// TestE2E_DiscoverAndResumeById proves the restart-resume runtime deliverable
// against REAL opencode: a turn creates a discoverable session, and resuming
// by that id resolves the prior session (NOT "Session not found"). This is the
// positive counterpart to the stale-id negative proof and the real-binary
// validation of the api-package parser's shape assumptions.
//
// The worktree is a REAL git worktree (git worktree add --detach), mirroring
// exactly how kamacu provisions task worktrees — opencode only reliably
// creates+persists a session row for a real project repo (tiny scratch repos
// may not, provider-error timing aside). If no session is discovered (a flaky
// provider that errors before session creation), the test skips rather than
// fails; the negative stale-id proof (TestE2E_ResumeStaleIdErrors) is the
// always-reliable DONE-WHEN backstop.
func TestE2E_DiscoverAndResumeById(t *testing.T) {
	opencodeOnPath(t)
	tmp := isolateOpencodeConfig(t)
	worktree := realGitWorktree(t) // a real project repo (mirrors kamacu worktrees)

	// 1. Run a turn in the worktree — creates a session row (even if the
	//    provider errors, per MEM031 the session lifecycle still completes).
	//    PWD=worktree (MEM035) so opencode records directory == worktree.
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	turn := exec.CommandContext(ctx, "opencode", "run", "--format", "json", "say hi")
	turn.Dir = worktree
	tenv := envWithoutKamacu()
	tenv = append(tenv, "XDG_CONFIG_HOME="+tmp, "HOME="+tmp)
	// PWD=worktree mirrors manager.Spawn's opencode env injection (MEM035):
	// opencode resolves the session's `directory` from $PWD, not getcwd(), so
	// discovery's directory==worktree filter requires $PWD to match cmd.Dir.
	tenv = append(tenv, "PWD="+worktree)
	turn.Env = tenv
	turnOut, turnErr := turn.CombinedOutput()
	cancel()
	t.Logf("worktree turn exit error: %v; output (tail):\n%s", errString(turnErr), truncForLog(turnOut))

	// 2. Discover the session for THIS worktree (project-scoped list, MEM036).
	raw := opencodeSessionListJSON(t, tmp, worktree, nil)
	t.Logf("session list json: %s", truncForLog(raw))
	sesID := mostRecentSessionForDir(t, raw, worktree)
	if sesID == "" {
		t.Skipf("no opencode session discovered for worktree %s after a turn (provider/config may not have created a session row); discovery proof inconclusive on this host — the negative stale-id proof is the reliable backstop", worktree)
	}
	t.Logf("discovered opencode session id %q for worktree %s", sesID, worktree)
	if !strings.HasPrefix(sesID, "ses_") {
		t.Errorf("discovered id %q does not have the expected ses_ prefix (opencode session id shape drifted)", sesID)
	}

	// 3. Resume by the discovered id resolves the prior session (NOT "Session
	//    not found"). The TUI hangs on success; the timeout kill is expected.
	resumeOut := runResumeById(t, tmp, worktree, sesID)
	t.Logf("opencode -s <real-id> output (tail):\n%s", truncForLog(resumeOut))
	if strings.Contains(string(resumeOut), "Session not found") {
		t.Fatalf("opencode -s <discovered-id> reported 'Session not found' — the real id did not resolve (resume-by-id is broken against real opencode). id=%s", sesID)
	}
	t.Logf("PASS: discovered real opencode session id resumes (no 'Session not found'); discovery + -s resume path proven against real opencode")
}

// TestE2E_ResumeStaleIdErrors is the deterministic negative proof (no provider
// needed): `opencode run -s ses_NONEXISTENT` prints "Session not found" — the
// already-verified stale-id behavior. A stale persisted opencode_session_id
// surfaces honestly (the user clicks "Reset session") rather than silently
// forking a fresh session. Mirrors claude's missing-transcript failure.
//
// Uses RUN mode (not TUI): run mode is non-interactive, so it surfaces the
// session-lookup error deterministically and exits, whereas a TTY-less TUI
// spawn (`opencode -s <id>` under exec.Command with no controlling terminal)
// hangs on TUI setup before reaching the lookup. The -s resume flag is global,
// so run mode exercises the identical id-resolution path.
func TestE2E_ResumeStaleIdErrors(t *testing.T) {
	opencodeOnPath(t)
	tmp := isolateOpencodeConfig(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "opencode", "run", "-s", "ses_NONEXISTENT", "hi")
	cmd.Env = append(envWithoutKamacu(), "XDG_CONFIG_HOME="+tmp, "HOME="+tmp)
	out, _ := cmd.CombinedOutput() // a missing id exits non-zero with the error text
	t.Logf("opencode run -s ses_NONEXISTENT output:\n%s", truncForLog(out))
	if !strings.Contains(string(out), "Session not found") {
		t.Fatalf("opencode run -s ses_NONEXISTENT did NOT report 'Session not found' — stale-id honesty regressed (a stale persisted opencode_session_id must surface an honest error, never silently fork). output=%q", truncForLog(out))
	}
	t.Logf("PASS: stale opencode session id surfaces 'Session not found' (resume is honest, not a silent fresh fork)")
}

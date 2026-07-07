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

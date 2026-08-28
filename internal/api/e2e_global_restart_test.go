package api

// e2e_global_restart_test.go — the repo's first REAL-binary lifecycle E2E
// (Phase 17, D-47..D-54): build cmd/kamacu in-test, spawn it with a fully
// isolated sandbox env, drive the SC1 reconfigure-gate narrative (D-48 /
// GCONF-04) and — in the restart leg below — the SC2 restart-resume
// narrative (D-49/D-51 / GSESS-02/03) over real HTTP against
// 127.0.0.1:<allocated-port>, SIGTERM the process (serve registers NO
// signal handler — hard death IS the restart semantic), and boot a second
// process against the SAME --db file.
//
// Host gates (D-50): tmux + git must be on PATH or the family skips (the
// house exec.LookPath convention). A build failure is FATAL, never a skip.
//
// Isolation (T-17-01..T-17-04): the spawned server's HOME, TMUX_TMPDIR and
// XDG_CONFIG_HOME all live inside a per-test sandbox, so every ~-relative
// resolution (transcript glob, opencode plugin write, tmux conf next to
// the DB, managed-clone namespace) stays inside t.TempDir. The tmux socket
// NAME stays the hardcoded serve default; only the socket DIRECTORY
// separates the test's tmux server from the user's real one — in BOTH
// directions (the spawned server's startup orphan sweep can never see the
// user's socket, and the harness's probes can never see it either, because
// both sides resolve the socket through the same sandboxed directory).
// KAMACU_SESSION_ID, KAMACU_HOOK_TOKEN and KAMACU_HOOK_BASE are filtered
// OUT of the inherited env: a Kamacu terminal exports all three, and the
// hook token is a secret (17-RESEARCH Pitfall 2).
//
// One production seam defeats plain TMUX_TMPDIR isolation and is handled
// here: the session manager's tmux spawn arm carries an explicit env
// allow-list (the TMUX/TMUX_PANE leak scrub, manager.go) that DROPS
// TMUX_TMPDIR — the spawned attach client would resolve the socket at the
// DEFAULT directory (the user's REAL tmux server) while every server-side
// probe honors the sandbox dir. The harness therefore prepends a sandbox
// bin dir to the child PATH holding a tiny `tmux` wrapper that re-exports
// the sandbox socket dir and execs the real binary — every tmux invocation
// from inside the server (spawn arm AND probes) then agrees on the socket.
// (The production-side inconsistency — probes honor TMUX_TMPDIR, spawns
// scrub it — is logged for the D-63 env-hygiene plan.)

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/google/uuid"

	"kamacu/internal/tmux"
)

// e2eProbeClient bounds the healthz readiness probe (a probe against a
// half-booted or dying server must never hang the test — http.DefaultClient
// has no timeout).
var e2eProbeClient = &http.Client{Timeout: 2 * time.Second}

// e2eServer is the real-binary SUT handle: one sandbox (HOME + tmux socket
// dir + DB + fake-claude recorder files), the built binary, an allocated
// loopback port, and the currently-running serve process (nil between
// restarts). start/stop implement the restart loop — a restart is a fresh
// process with IDENTICAL args and env against the same --db file.
type e2eServer struct {
	t          *testing.T
	addr       string
	baseURL    string
	dbPath     string
	sandbox    string
	tmuxDir    string
	binDir     string // sandbox PATH dir holding the socket-dir tmux wrapper
	binPath    string
	fakeClaude string
	argsFile   string
	pwdFile    string

	cmd      *exec.Cmd
	logFiles []string // one combined-output log file per server generation
}

// newE2EServer host-gates (D-50), builds the SUT, allocates a free loopback
// port, and registers LIFO cleanups so teardown order is: stop any running
// server, then KillServer the sandbox tmux socket, then t.TempDir's own
// removal (the socket dir must outlive the tmux server).
func newE2EServer(t *testing.T) *e2eServer {
	t.Helper()
	realTmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux not on PATH")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}

	sandbox := t.TempDir()
	tmuxDir := filepath.Join(sandbox, "tmux")
	if err := os.MkdirAll(tmuxDir, 0o755); err != nil {
		t.Fatalf("mkdir tmux socket dir: %v", err)
	}
	// The socket-dir tmux wrapper (see the file doc comment): production's
	// tmux spawn arm scrubs TMUX_TMPDIR, so a PATH shim is the only
	// harness-side way to keep the spawned tabs on the sandbox socket.
	binDir := filepath.Join(sandbox, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir sandbox bin dir: %v", err)
	}
	wrapper := "#!/bin/sh\n" +
		"# e2e socket-dir shim: re-arm the sandbox tmux socket dir for every\n" +
		"# tmux invocation from inside the spawned server (the spawn arm's env\n" +
		"# allow-list drops it; without this the tabs land on the real socket).\n" +
		"TMUX_TMPDIR='" + tmuxDir + "'\n" +
		"export TMUX_TMPDIR\n" +
		"exec '" + realTmux + "' \"$@\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "tmux"), []byte(wrapper), 0o755); err != nil {
		t.Fatalf("write tmux socket-dir wrapper: %v", err)
	}
	// Harness-side socket isolation (T-17-01): every tmux.Client invocation
	// from THIS test process (the SC2 liveness probe, the KillServer
	// cleanup) execs tmux without an explicit Env, so it inherits this
	// value and resolves the sandbox socket — the exact directory the
	// spawned servers use. The variable name spelled above is the only one
	// tmux 3.4 recognizes; any underscore-separated variant is silently
	// ignored and gives zero isolation.
	t.Setenv("TMUX_TMPDIR", tmuxDir)

	// Build the SUT in-test (D-47): deterministic on fresh checkouts (the
	// committed web/dist placeholder keeps the embed working) — never a
	// stale or missing bin/ artifact. A build failure is fatal.
	moduleRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve module root: %v", err)
	}
	binPath := filepath.Join(sandbox, "kamacu")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/kamacu")
	build.Dir = moduleRoot
	if out, berr := build.CombinedOutput(); berr != nil {
		t.Fatalf("go build ./cmd/kamacu (dir %s): %v\n%s", moduleRoot, berr, out)
	}

	// Free-port allocation — never a fixed port (go test runs packages in
	// parallel; serve's ensureLoopback would reject a wildcard anyway).
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate loopback port: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	e := &e2eServer{
		t:          t,
		addr:       addr,
		baseURL:    "http://" + addr,
		dbPath:     filepath.Join(sandbox, "kamacu.db"),
		sandbox:    sandbox,
		tmuxDir:    tmuxDir,
		binDir:     binDir,
		binPath:    binPath,
		fakeClaude: testdataFakeClaude(t), // chmod-reapplied absolute testdata path
		argsFile:   filepath.Join(sandbox, "fake-claude-args"),
		pwdFile:    filepath.Join(sandbox, "fake-claude-pwd"),
	}

	// LIFO cleanup: the tmux-server kill is registered FIRST so it runs
	// LAST — after the stop-server cleanup below has reaped the serve
	// process (which holds the tab open via its attach client), and after
	// t.TempDir's own registration so the socket dir outlives the server.
	sandboxTmux := e.sandboxTmux()
	t.Cleanup(func() {
		if kerr := sandboxTmux.KillServer(context.Background()); kerr != nil {
			t.Logf("cleanup: kill sandbox tmux server: %v", kerr)
		}
	})
	t.Cleanup(func() {
		if serr := e.stop(); serr != nil {
			t.Logf("cleanup: stop server: %v\nserver logs:\n%s", serr, e.dumpLogs())
		}
	})
	return e
}

// sandboxTmux is the harness-side client against the sandbox tmux server:
// the DEFAULT socket name (the one serve hardcodes) with ConfPath silenced;
// probes resolve the sandbox socket DIRECTORY via the inherited value set
// in newE2EServer (T-17-01 — both directions isolated).
func (e *e2eServer) sandboxTmux() tmux.Client {
	return tmux.Client{Socket: tmux.DefaultSocket, ConfPath: "/dev/null"}
}

// waitGlobalTmuxName polls the sandbox socket until exactly one
// kamacu-global-* session is live and returns its name — the authoritative
// live-tab name source. The engine's Info wire deliberately omits TmuxName
// for LIVE sessions (only orphaned/reconciled rows carry it), so the
// socket, not the REST response, is where a live tab's name is observed.
func (e *e2eServer) waitGlobalTmuxName() string {
	e.t.Helper()
	var names []string
	var err error
	deadline := time.Now().Add(3 * time.Second)
	for {
		names, err = e.sandboxTmux().ListSessions(context.Background())
		if err == nil {
			found := 0
			name := ""
			for _, n := range names {
				if strings.HasPrefix(n, "kamacu-global-") {
					found++
					name = n
				}
			}
			if found == 1 {
				return name
			}
		}
		if time.Now().After(deadline) {
			e.t.Fatalf("never found exactly one kamacu-global-* session on the sandbox socket (names=%v err=%v)", names, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// childEnv builds the spawned server's environment: the sandbox trio
// (HOME / TMUX_TMPDIR / XDG_CONFIG_HOME), the fake-claude recorder pair
// (t.Setenv cannot reach a spawned process — the recorder vars MUST ride
// the child env so the PTY-spawned fake-claude inherits them), PATH
// INHERITED but with the sandbox bin dir prepended so the socket-dir tmux
// wrapper wins every PATH resolution inside the server (see newE2EServer),
// and the KAMACU_ trio (SESSION_ID / HOOK_TOKEN / HOOK_BASE) scrubbed — a
// Kamacu terminal exports all three and the hook token is a secret
// (T-17-04, Pitfall 2).
func (e *e2eServer) childEnv() []string {
	env := []string{
		"HOME=" + e.sandbox,
		"TMUX_TMPDIR=" + e.tmuxDir,
		"XDG_CONFIG_HOME=" + filepath.Join(e.sandbox, ".config"),
		"FAKE_CLAUDE_ARGS_FILE=" + e.argsFile,
		"FAKE_CLAUDE_PWD_FILE=" + e.pwdFile,
	}
	for _, kv := range os.Environ() {
		key := kv
		val := ""
		if i := strings.IndexByte(kv, '='); i >= 0 {
			key, val = kv[:i], kv[i+1:]
		}
		switch key {
		case "HOME", "TMUX_TMPDIR", "XDG_CONFIG_HOME", "FAKE_CLAUDE_ARGS_FILE", "FAKE_CLAUDE_PWD_FILE":
			continue // overridden by the sandbox entries above
		case "KAMACU_SESSION_ID", "KAMACU_HOOK_TOKEN", "KAMACU_HOOK_BASE":
			continue // never inherited — secret/nesting hygiene (Pitfall 2)
		case "PATH":
			// Prepend the sandbox bin dir: the socket-dir tmux wrapper must
			// win LookPath inside the server.
			env = append(env, "PATH="+e.binDir+string(os.PathListSeparator)+val)
			continue
		}
		env = append(env, kv)
	}
	return env
}

// start spawns (or re-spawns, after stop) the real binary and polls
// GET /api/healthz until it answers 200 (smoke.sh's budget: 5s / 100ms).
// Both server A and server B run the identical command line.
func (e *e2eServer) start() {
	e.t.Helper()
	cmd := exec.Command(e.binPath, "serve",
		"--addr", e.addr,
		"--db", e.dbPath,
		"--claude-bin", e.fakeClaude,
	)
	cmd.Env = e.childEnv()
	// FILE-backed output, never pipes/buffers: the tmux server daemonizes
	// out of the serve process's reach and inherits its stdio. With a
	// buffer (or any non-*os.File writer) os/exec pipes the output through
	// copying goroutines that wait for EOF — an EOF the orphaned tmux
	// server never sends — so cmd.Wait() would never return and stop()
	// would hang the whole test on an already-dead process. File
	// descriptors have no EOF semantics; Wait returns as soon as the
	// process is reaped.
	logPath := filepath.Join(e.sandbox, fmt.Sprintf("serve-%d.log", len(e.logFiles)+1))
	lf, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		e.t.Fatalf("open server log file: %v", err)
	}
	cmd.Stdout, cmd.Stderr = lf, lf
	e.logFiles = append(e.logFiles, logPath)
	if err := cmd.Start(); err != nil {
		e.t.Fatalf("spawn kamacu serve: %v", err)
	}
	e.cmd = cmd
	deadline := time.Now().Add(5 * time.Second)
	for {
		resp, err := e2eProbeClient.Get(e.baseURL + "/api/healthz")
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		if time.Now().After(deadline) {
			e.t.Fatalf("server never answered /api/healthz within 5s at %s\nserver logs:\n%s", e.baseURL, e.dumpLogs())
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// stop SIGTERMs the running process and waits with a 10s deadline (serve
// registers no signal handler — default SIGTERM death IS the restart
// semantic), with Kill as the deadline backstop. Idempotent (no-op between
// restarts). The Wait error is deliberately ignored: death-by-signal is
// the expected exit shape here.
func (e *e2eServer) stop() error {
	if e.cmd == nil || e.cmd.Process == nil {
		return nil
	}
	proc := e.cmd.Process
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("signal server: %w", err)
	}
	done := make(chan error, 1)
	go func() { done <- e.cmd.Wait() }()
	select {
	case <-done:
		e.cmd = nil
		return nil
	case <-time.After(10 * time.Second):
		_ = proc.Kill()
		<-done
		e.cmd = nil
		return fmt.Errorf("server ignored SIGTERM for 10s (killed)")
	}
}

// dumpLogs returns every server generation's combined output.
func (e *e2eServer) dumpLogs() string {
	var b strings.Builder
	for _, p := range e.logFiles {
		if data, err := os.ReadFile(p); err == nil {
			b.Write(data)
		}
	}
	return b.String()
}

// --- small argv / fixture / poll helpers (house poll-with-deadline style) --

// e2eReadArgv reads the fake-claude argv recorder (nil while empty/absent).
func e2eReadArgv(argsFile string) []string {
	b, err := os.ReadFile(argsFile)
	if err != nil || len(b) == 0 {
		return nil
	}
	return strings.Split(strings.TrimRight(string(b), "\n"), "\n")
}

// e2eWaitArgvFlag polls the recorder until it carries the flag token
// (≤3s — the spawn → record window) and returns the recorded argv.
func e2eWaitArgvFlag(t *testing.T, argsFile, flag string) []string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if args := e2eReadArgv(argsFile); args != nil {
			for _, a := range args {
				if a == flag {
					return args
				}
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("fake-claude never recorded argv carrying %q at %s", flag, argsFile)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// e2eWaitArgvPair polls until the recorder carries the consecutive token
// pair. The resume spawn OVERWRITES the recorder file, so waiting for the
// pair (not merely a non-empty file) is what distinguishes a new spawn's
// argv from the previous one's.
func e2eWaitArgvPair(t *testing.T, argsFile string, pair ...string) []string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if args := e2eReadArgv(argsFile); args != nil && argvHasPair(args, pair...) {
			return args
		}
		if time.Now().After(deadline) {
			t.Fatalf("fake-claude argv never carried the consecutive pair %v at %s", pair, argsFile)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// e2eArgvValue returns the token immediately after flag ("" when absent).
func e2eArgvValue(args []string, flag string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag {
			return args[i+1]
		}
	}
	return ""
}

// e2eSeedTranscript writes the claude transcript fixture under the SANDBOX
// HOME — <sbx>/.claude/projects/e2e/<csid>.jsonl (any single directory
// level matches the transcriptExists glob, resume.go) — and returns its
// path so the D-54 negative edge can delete it. Must run BEFORE any
// resumable assertion (Pitfall 5: without the fixture, resumable is false
// and resume 409s — indistinguishable from a restart bug).
func e2eSeedTranscript(t *testing.T, sandboxHome, csid string) string {
	t.Helper()
	dir := filepath.Join(sandboxHome, ".claude", "projects", "e2e")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir transcript fixture dir: %v", err)
	}
	p := filepath.Join(dir, csid+".jsonl")
	if err := os.WriteFile(p, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write transcript fixture: %v", err)
	}
	return p
}

// e2eWaitGlobalSessionStatus polls the scoped list until the session id
// reports the wanted status — the over-HTTP exit observer for API stops
// (the harness has no manager handle into the real process). 8s budgets
// the D-14 stop grace, the waitGlobalSessionExited posture.
func e2eWaitGlobalSessionStatus(t *testing.T, baseURL, sid, want string) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for {
		_, rows := doJSONList(t, baseURL+"/api/sessions?scope=global")
		for _, row := range rows {
			if row["id"] == sid && row["status"] == want {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("session %q never reached status %q via GET /api/sessions?scope=global; rows=%v", sid, want, rows)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// e2eGlobalStatusEntries returns the /api/agents/status entries whose
// source is "global" (the Phase-16 discriminator).
func e2eGlobalStatusEntries(t *testing.T, baseURL string) []map[string]any {
	t.Helper()
	status, list := doJSONList(t, baseURL+"/api/agents/status")
	if status != http.StatusOK {
		t.Fatalf("GET /api/agents/status = %d, want 200", status)
	}
	var out []map[string]any
	for _, entry := range list {
		if entry["source"] == "global" {
			out = append(out, entry)
		}
	}
	return out
}

// e2eScopedTmuxRow returns the ?scope=global row carrying the tmux name —
// the live engine row pre-restart, the orphaned survivor post-restart.
func e2eScopedTmuxRow(t *testing.T, baseURL, tmuxName string) map[string]any {
	t.Helper()
	_, rows := doJSONList(t, baseURL+"/api/sessions?scope=global")
	for _, row := range rows {
		if row["tmuxName"] == tmuxName {
			return row
		}
	}
	t.Fatalf("no /api/sessions?scope=global row with tmuxName %q; rows=%v", tmuxName, rows)
	return nil
}

// e2eAssertGate409 asserts the D-15 409 grammar: non-empty error plus a
// non-empty reasons array whose first element is a {kind, target} object
// (the global_test.go assertion shape).
func e2eAssertGate409(t *testing.T, body map[string]any) {
	t.Helper()
	if got, _ := body["error"].(string); got == "" {
		t.Errorf("gate 409 error = %v, want non-empty", body["error"])
	}
	reasons, ok := body["reasons"].([]any)
	if !ok || len(reasons) == 0 {
		t.Fatalf("gate 409 reasons = %v, want a non-empty array", body["reasons"])
	}
	first, ok := reasons[0].(map[string]any)
	if !ok {
		t.Fatalf("gate 409 reasons[0] = %v, want a {kind,target} map", reasons[0])
	}
	if got := first["kind"]; got != "sessions" {
		t.Errorf("gate 409 reasons[0].kind = %v, want %q", got, "sessions")
	}
	if got, _ := first["target"].(string); got == "" {
		t.Errorf("gate 409 reasons[0].target = %v, want a non-empty live-session target", first["target"])
	}
}

// TestE2EGlobalReconfigureGate (SC1, D-47/D-48 / GCONF-04): the full
// reconfigure cycle against a REAL serve process over real HTTP —
// configure → shell=tmux → spawn agent + tmux bash tab → concurrent-spawn
// 409 → root change 409 with reasons → root clear 409 with reasons → stop
// both → clear 200 → resume-refusal 409 proving the persisted csid was
// cleared (D-16) observably, without reading the DB (Pitfall 6 — the
// /api/global wire never exposes resume ids).
func TestE2EGlobalReconfigureGate(t *testing.T) {
	e := newE2EServer(t)
	e.start()

	// 1. Folder root: a temp git repo fixture.
	root := gitRepo(t)
	if status, body := doJSON(t, "PUT", e.baseURL+"/api/global", map[string]any{"root_path": root}); status != http.StatusOK {
		t.Fatalf("PUT /api/global folder root: status = %d, want 200; body=%v", status, body)
	}

	// 2. Global bash tabs become tmux tabs.
	if status, body := doJSON(t, "PUT", e.baseURL+"/api/settings/shell", map[string]any{"value": "tmux"}); status != http.StatusOK {
		t.Fatalf("PUT /api/settings/shell tmux: status = %d, want 200; body=%v", status, body)
	}

	// 3. Global agent spawn: fake-claude (injected via --claude-bin)
	// records its argv; the --session-id pair's uuid is the persisted csid.
	status, body := doJSON(t, "POST", e.baseURL+"/api/sessions", map[string]any{"scope": "global", "kind": "agent"})
	if status != http.StatusCreated {
		t.Fatalf("global agent spawn: status = %d, want 201; body=%v", status, body)
	}
	agentID, _ := body["id"].(string)
	if agentID == "" {
		t.Fatalf("global agent spawn returned no id: %v", body)
	}
	csid := e2eArgvValue(e2eWaitArgvFlag(t, e.argsFile, "--session-id"), "--session-id")
	if _, err := uuid.Parse(csid); err != nil {
		t.Fatalf("fresh --session-id value %q is not a uuid: %v", csid, err)
	}

	// 4. Global tmux bash tab — kamacu-global-N on the sandbox socket. The
	// live tab's name is deliberately absent from the engine's Info wire
	// (TmuxName rides only orphaned/reconciled rows), so the socket probe
	// is the authoritative name source.
	status, body = doJSON(t, "POST", e.baseURL+"/api/sessions", map[string]any{"scope": "global", "kind": "bash"})
	if status != http.StatusCreated {
		t.Fatalf("global bash spawn: status = %d, want 201; body=%v", status, body)
	}
	tmuxID, _ := body["id"].(string)
	if tmuxID == "" {
		t.Fatalf("global bash spawn returned no id: %v", body)
	}
	tmuxName := e.waitGlobalTmuxName()
	if !strings.HasPrefix(tmuxName, "kamacu-global-") {
		t.Fatalf("live global tab name = %q, want a kamacu-global-N name", tmuxName)
	}

	// 5. Concurrent-spawn guard (D-34): a second global agent while one is
	// live is the honest 409 — asserted over real HTTP.
	status, body = doJSON(t, "POST", e.baseURL+"/api/sessions", map[string]any{"scope": "global", "kind": "agent"})
	if status != http.StatusConflict {
		t.Fatalf("concurrent global agent spawn: status = %d, want 409; body=%v", status, body)
	}
	if got, _ := body["error"].(string); got != "global agent already running" {
		t.Errorf("concurrent spawn error = %q, want %q", got, "global agent already running")
	}

	// 6. Root CHANGE while live: 409 with the structured reasons grammar.
	status, body = doJSON(t, "PUT", e.baseURL+"/api/global", map[string]any{"root_path": gitRepo(t)})
	if status != http.StatusConflict {
		t.Fatalf("root change while live: status = %d, want 409; body=%v", status, body)
	}
	e2eAssertGate409(t, body)

	// 7. Root CLEAR while live: the live gate covers the clear too.
	status, body = doJSON(t, "PUT", e.baseURL+"/api/global", map[string]any{"root_path": ""})
	if status != http.StatusConflict {
		t.Fatalf("root clear while live: status = %d, want 409; body=%v", status, body)
	}
	e2eAssertGate409(t, body)

	// 8. Stop the agent and the tmux tab, then delete the tab's engine
	// session. Stop replies 202 and reaps asynchronously; poll the scoped
	// list for both exits before the delete (Remove refuses a runner).
	for _, sid := range []string{agentID, tmuxID} {
		if s, b := doJSON(t, "POST", e.baseURL+"/api/sessions/"+sid+"/stop", nil); s != http.StatusAccepted {
			t.Fatalf("stop %s: status = %d, want 202; body=%v", sid, s, b)
		}
		e2eWaitGlobalSessionStatus(t, e.baseURL, sid, "exited")
	}
	if s, b := doJSON(t, "DELETE", e.baseURL+"/api/sessions/"+tmuxID, nil); s != http.StatusNoContent {
		t.Fatalf("delete tmux tab %s: status = %d, want 204; body=%v", tmuxID, s, b)
	}

	// 9. With nothing live, the clear succeeds.
	if status, body := doJSON(t, "PUT", e.baseURL+"/api/global", map[string]any{"root_path": ""}); status != http.StatusOK {
		t.Fatalf("root clear after stops: status = %d, want 200; body=%v", status, body)
	}

	// 10. ID-clearing proof (D-16) WITHOUT reading the DB (Pitfall 6 — the
	// /api/global wire never exposes resume ids). The root gates fire
	// BEFORE resume validation (D-33), so a bare post-clear resume would
	// be the D-28 "root not configured" 409 — reconfigure the root first,
	// and seed a transcript for the freshly-minted csid so an UNCLEARED id
	// would resume 201. The honest D-31 refusal is then exactly the
	// observable that the successful root change wiped the persisted csid.
	if status, body := doJSON(t, "PUT", e.baseURL+"/api/global", map[string]any{"root_path": root}); status != http.StatusOK {
		t.Fatalf("re-PUT root after clear: status = %d, want 200; body=%v", status, body)
	}
	e2eSeedTranscript(t, e.sandbox, csid)
	status, body = doJSON(t, "POST", e.baseURL+"/api/sessions", map[string]any{"scope": "global", "kind": "agent", "resume": true})
	if status != http.StatusConflict {
		t.Fatalf("resume after clear: status = %d, want 409; body=%v", status, body)
	}
	if got, _ := body["error"].(string); got != "no global claude session to resume" {
		t.Errorf("resume-after-clear error = %q, want %q", got, "no global claude session to resume")
	}
}

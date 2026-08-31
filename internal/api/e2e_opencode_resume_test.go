package api

// e2e_opencode_resume_test.go — the D-52 opencode half of the global
// restart-resume E2E (Phase 17 Plan 04 / GSESS-02): the REAL opencode
// binary driven through a REAL serve-process restart, proving the
// engine-branched resume append (`-s <ses_ id>`, the opencode arm in
// sessions.go) with a wrapper-agent argv recorder while capture, status
// flips and the refusal edge are observed over HTTP. It EXTENDS the 17-01
// harness (newE2EServer: build, spawn, healthz poll, SIGTERM+Wait,
// restart) — zero production code changes, zero new dependencies.
//
// Host gates (D-50): opencode + tmux + git must be on PATH or the test
// skips — on hosts without opencode, GSESS-02's opencode half degrades to
// the API-level stubs (Phase 15) plus the UAT's human flows (D-53).
//
// Env posture — the ONE difference from the 17-01 claude legs (Pitfall 3):
// REAL HOME and inherited XDG (e.realHomeEnv). The real opencode binary
// resolves its real config/auth and session storage (~/.local/share/
// opencode/opencode.db, keyed by the session's directory), and THREE
// parties must agree on that resolution for the capture to work: the
// spawned agent (the serve child's os.Environ()), the capture poller's
// `opencode session list` (same), and the test's out-of-band first turn
// (the test process env). Everything the leg can dirty stays confined: a
// dedicated temp --db (the harness sandbox DB), a fresh temp git-repo
// root, and a sandboxed TMUX_TMPDIR (T-17-10) with the socket-dir PATH
// shim — the user's real tmux server is never touched in either
// direction.
//
// Honest consequences of the real-HOME posture, accepted by design: the
// serve boot's opencode plugin install targets the REAL config dir
// (idempotent, skip-on-match — exactly what any kamacu boot maintains
// there), and the out-of-band first turn writes one session row into the
// real opencode storage, keyed by the temp root directory (the same
// residue TestGlobalOpencodeCaptureHost leaves; harmless and invisible
// outside `opencode session list` for that directory).
//
// The wrapper agent (17-RESEARCH Pattern 3): the system OpenCode agent's
// command is PATCHed to a test-written script that records its arguments
// one-per-line to a baked-in absolute argv file and then execs the
// LookPath-resolved ABSOLUTE opencode path (T-17-12 — no PATH games, a
// shadowing fake cannot win). The engine stays locked 'opencode' (the
// system lock is engine-only), so the engine-branched resume path runs
// verbatim while the argv stays observable. The restore PATCH is
// registered in t.Cleanup IMMEDIATELY after the mutation succeeds
// (Pitfall 9): LIFO teardown runs restore → stop-server → kill-tmux, and
// even a missed restore is contained by the dedicated temp --db (T-17-11).

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kamacu/internal/store"
)

// e2eWaitOcArgv polls the wrapper's argv recorder until the fresh spawn
// has written it (the wrapper's first statement runs the moment the PTY
// child starts). A zero-argument fresh spawn records a single empty line —
// non-nil from e2eReadArgv — which is the spawn-happened signal; the
// consecutive-pair idiom (e2eWaitArgvPair) later distinguishes the resume
// spawn's content, since the wrapper OVERWRITES the file per spawn.
func e2eWaitOcArgv(t *testing.T, argvFile string) []string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if args := e2eReadArgv(argvFile); args != nil {
			return args
		}
		if time.Now().After(deadline) {
			t.Fatalf("opencode wrapper never recorded argv at %s", argvFile)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// e2eWaitGlobalResumable polls GET /api/agents/status until the global
// entry flips resumable:true — the capture poller's persist observed over
// HTTP. opencode resumability keys off the persisted
// global_task.opencode_session_id ALONE (no transcript glob; agents.go's
// globalResumeState.resumable), so this flip is proof the ses_ id landed.
// 60s @ 200ms budgets the 2s-interval capture poller behind a first turn
// that can legitimately take tens of seconds.
func e2eWaitGlobalResumable(t *testing.T, baseURL string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for {
		for _, entry := range e2eGlobalStatusEntries(t, baseURL) {
			if entry["resumable"] == true {
				return entry
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("global status entry never flipped resumable:true within 60s (capture poller never persisted the ses_ id?)")
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// TestE2EGlobalOpencodeRestartResume (D-52 / GSESS-02 opencode half): the
// restart narrative against the real binary — configure folder root +
// OpenCode default agent → fresh agent spawn through the wrapper (argv
// recorded, NO -s token) → out-of-band first turn (`opencode run` in the
// root; the row is keyed by the directory) → stop → status feed flips
// resumable:true (capture persisted) → SIGTERM the REAL process → second
// process on the SAME --db → exactly one resumable global entry → resume
// 201 with the wrapper argv carrying the consecutive pair
// `-s ses_<captured id>` → stop → clear the root (wipes the persisted ids,
// D-16) → re-arm the root (D-33: the unconfigured-root 409 fires before
// resume validation — 17-01's step-10 precedent) → resume 409
// `no global opencode session to resume` (the D-54 opencode edge).
func TestE2EGlobalOpencodeRestartResume(t *testing.T) {
	// D-50 host gates: tmux+git ride newE2EServer; opencode is this leg's
	// own gate (the claude legs skip it deliberately).
	if _, err := exec.LookPath("opencode"); err != nil {
		t.Skip("opencode not on PATH")
	}
	e := newE2EServer(t)
	// The posture difference MUST be set before the first start() so BOTH
	// generations run identical args+env (the restart contract).
	e.realHomeEnv = true
	e.start()

	// T-17-12: resolve the real opencode once; the wrapper bakes the
	// absolute path into its exec line and the out-of-band first turn
	// below uses the same absolute binary.
	ocBin, err := exec.LookPath("opencode")
	if err != nil {
		t.Fatalf("LookPath opencode: %v", err)
	}

	// The wrapper agent (Pattern 3): record argv, then BECOME the real
	// opencode. Lives in the sandbox (NOT the PATH shim dir — the command
	// is spawned by absolute path, no resolution, no shadowing surface).
	argvFile := filepath.Join(e.sandbox, "opencode-argv")
	wrapper := "#!/bin/sh\n" +
		"# e2e opencode wrapper (D-52): record argv, then become the real opencode.\n" +
		"printf '%s\\n' \"$@\" > '" + argvFile + "'\n" +
		"exec '" + ocBin + "' \"$@\"\n"
	wrapperPath := filepath.Join(e.sandbox, "opencode-wrapper")
	if err := os.WriteFile(wrapperPath, []byte(wrapper), 0o755); err != nil {
		t.Fatalf("write opencode wrapper: %v", err)
	}

	// Find the system OpenCode agent over the real wire (the 00015 seed:
	// engine 'opencode', is_system true).
	_, agents := doJSONList(t, e.baseURL+"/api/agents")
	var ocID int64 = -1
	seedCommand := ""
	for _, a := range agents {
		if a["engine"] == "opencode" && a["is_system"] == true {
			if f, ok := a["id"].(float64); ok {
				ocID = int64(f)
				seedCommand, _ = a["command"].(string)
			}
		}
	}
	if ocID < 0 || seedCommand == "" {
		t.Fatalf("no system opencode agent in GET /api/agents: %v", agents)
	}

	// PATCH the command to the wrapper (system agents lock the ENGINE, not
	// the command — agents_crud.go), and register the restore PATCH
	// IMMEDIATELY after the mutation succeeds (Pitfall 9 / T-17-11).
	status, body := doJSON(t, "PATCH", fmt.Sprintf("%s/api/agents/%d", e.baseURL, ocID),
		map[string]any{"command": wrapperPath})
	if status != http.StatusOK {
		t.Fatalf("PATCH wrapper command: status = %d, want 200; body=%v", status, body)
	}
	t.Cleanup(func() {
		if s, b := doJSON(t, "PATCH", fmt.Sprintf("%s/api/agents/%d", e.baseURL, ocID),
			map[string]any{"command": seedCommand}); s != http.StatusOK {
			t.Logf("cleanup: restore OpenCode agent command to %q: status=%d body=%v", seedCommand, s, b)
		}
	})

	// 1. Folder root: a fresh temp git repo (never touches ~/.kamacu paths).
	root := gitRepo(t)
	if status, body := doJSON(t, "PUT", e.baseURL+"/api/global", map[string]any{"root_path": root}); status != http.StatusOK {
		t.Fatalf("PUT /api/global folder root: status = %d, want 200; body=%v", status, body)
	}

	// 2. The D-24 default-agent setter: the Scratchpad runs OpenCode.
	if status, body := doJSON(t, "PUT", e.baseURL+"/api/global", map[string]any{"agent_id": ocID}); status != http.StatusOK {
		t.Fatalf("PUT /api/global agent_id: status = %d, want 200; body=%v", status, body)
	}

	// 3. tmux shell for bash tabs (parity with the real-user configure
	// flow; this leg spawns no tabs, but the setting is part of the story).
	if status, body := doJSON(t, "PUT", e.baseURL+"/api/settings/shell", map[string]any{"value": "tmux"}); status != http.StatusOK {
		t.Fatalf("PUT /api/settings/shell tmux: status = %d, want 200; body=%v", status, body)
	}

	// The engine read that keys the resume branch below (the status wire
	// has no engine field — 17-01 deviation #4b: GET /api/global's agent
	// summary is the same singleton read).
	_, gbody := doJSON(t, "GET", e.baseURL+"/api/global", nil)
	agent, _ := gbody["agent"].(map[string]any)
	if agent == nil || agent["engine"] != "opencode" {
		t.Fatalf("GET /api/global agent = %v, want engine \"opencode\"", gbody["agent"])
	}

	// 4. Fresh global agent spawn through the wrapper: 201, argv recorded,
	// and a FRESH spawn carries NO -s token (the append is resume-only).
	status, body = doJSON(t, "POST", e.baseURL+"/api/sessions", map[string]any{"scope": "global", "kind": "agent"})
	if status != http.StatusCreated {
		t.Fatalf("global opencode spawn: status = %d, want 201; body=%v", status, body)
	}
	agentID, _ := body["id"].(string)
	if agentID == "" {
		t.Fatalf("global opencode spawn returned no id: %v", body)
	}
	freshArgv := e2eWaitOcArgv(t, argvFile)
	for _, a := range freshArgv {
		if a == "-s" {
			t.Errorf("fresh opencode argv carries -s (resume append leaked into a fresh spawn): %v", freshArgv)
		}
	}
	t.Logf("fresh opencode wrapper argv: %v", freshArgv)

	// 5. Out-of-band first turn (TestGlobalOpencodeCaptureHost's block):
	// `opencode run` in the root is the manual-terminal-session analog —
	// the row is keyed by the directory, matching the capture filter. The
	// model call may error; the session row is created regardless
	// (probe-verified on opencode 1.18.22).
	runCtx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(runCtx, ocBin, "run", "Reply with just: ok")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "PWD="+root)
	out, rerr := cmd.CombinedOutput()
	t.Logf("opencode run: err=%v output=%s", rerr, out)

	// 6. Stop the agent (202 — the shipped wire; the plan text's 200 was
	// 17-01 deviation #4a), observe the exit, then poll the status feed
	// until the global entry flips resumable:true — the capture poller's
	// persist over HTTP (opencode keys off the persisted id ALONE).
	if s, b := doJSON(t, "POST", e.baseURL+"/api/sessions/"+agentID+"/stop", nil); s != http.StatusAccepted {
		t.Fatalf("stop agent: status = %d, want 202; body=%v", s, b)
	}
	e2eWaitGlobalSessionStatus(t, e.baseURL, agentID, "exited")
	live := e2eWaitGlobalResumable(t, e.baseURL)
	if live["source"] != "global" || live["status"] != "exited" {
		t.Errorf("pre-restart resumable entry source/status = %v/%v, want global/exited", live["source"], live["status"])
	}
	if live["taskTitle"] != "Scratchpad" || live["projectName"] != "Global" {
		t.Errorf("pre-restart resumable entry labels = %v/%v, want Scratchpad/Global", live["taskTitle"], live["projectName"])
	}

	// 7. Hard death (SIGTERM — serve registers no handler) and the
	// quiescent-window DB read (Pitfall 6's belt-and-braces window, no
	// writer): the persisted ocsid, the exact id the resume argv must
	// carry. WAL + busy_timeout make the second connection safe.
	if err := e.stop(); err != nil {
		t.Fatalf("SIGTERM server A: %v\nserver logs:\n%s", err, e.dumpLogs())
	}
	http.DefaultClient.CloseIdleConnections()
	db, err := store.Open(e.dbPath)
	if err != nil {
		t.Fatalf("open temp db %s: %v", e.dbPath, err)
	}
	var ocsid sql.NullString
	if err := db.QueryRow(`SELECT opencode_session_id FROM global_task WHERE id = 1`).Scan(&ocsid); err != nil {
		db.Close()
		t.Fatalf("query global_task.opencode_session_id: %v", err)
	}
	db.Close()
	if !ocsid.Valid || !strings.HasPrefix(ocsid.String, "ses_") {
		t.Fatalf("persisted opencode session id = %+v, want a valid ses_… id (capture poller)", ocsid)
	}
	t.Logf("captured global opencode session id: %s", ocsid.String)

	// Second REAL process: identical args/env, same --db.
	e.start()

	// 8. Post-restart offer (pass 2b — DB-derived): exactly ONE global
	// entry, exited + resumable, Scratchpad/Global labels, no live id.
	entries := e2eGlobalStatusEntries(t, e.baseURL)
	if len(entries) != 1 {
		t.Fatalf("global status entries after restart = %d, want exactly 1: %v", len(entries), entries)
	}
	entry := entries[0]
	if entry["status"] != "exited" || entry["resumable"] != true {
		t.Errorf("post-restart entry status/resumable = %v/%v, want exited/true", entry["status"], entry["resumable"])
	}
	if entry["taskTitle"] != "Scratchpad" || entry["projectName"] != "Global" {
		t.Errorf("post-restart labels = %v/%v, want Scratchpad/Global", entry["taskTitle"], entry["projectName"])
	}
	if got, _ := entry["sessionId"].(string); got != "" {
		t.Errorf("post-restart sessionId = %q, want \"\" (DB-derived pass)", got)
	}

	// The resume (GSESS-02 opencode half): 201, and the wrapper argv now
	// carries the CONSECUTIVE pair -s <the SAME captured id> — the
	// engine-branched append exercised through the real spawn arm.
	status, body = doJSON(t, "POST", e.baseURL+"/api/sessions", map[string]any{"scope": "global", "kind": "agent", "resume": true})
	if status != http.StatusCreated {
		t.Fatalf("global opencode resume: status = %d, want 201; body=%v", status, body)
	}
	argv := e2eWaitArgvPair(t, argvFile, "-s", ocsid.String)
	if got := e2eArgvValue(argv, "-s"); got != ocsid.String {
		t.Errorf("resume argv -s value = %q, want the SAME captured id %q", got, ocsid.String)
	}
	t.Logf("resume opencode wrapper argv: %v", argv)

	// 9. The D-54 opencode stale/absent-ocsid edge: stop the resumed agent
	// (else the live gate 409s the root clear), clear the root (the
	// successful clear wipes the persisted ids — D-16), re-arm the root
	// (D-33: the unconfigured-root 409 fires BEFORE resume validation, so
	// a bare post-clear resume would return 'global root not configured' —
	// 17-01's step-10 precedent), then the honest engine-naming refusal.
	resumedID, _ := body["id"].(string)
	if resumedID == "" {
		t.Fatalf("resume spawn returned no id: %v", body)
	}
	if s, b := doJSON(t, "POST", e.baseURL+"/api/sessions/"+resumedID+"/stop", nil); s != http.StatusAccepted {
		t.Fatalf("stop resumed agent: status = %d, want 202; body=%v", s, b)
	}
	e2eWaitGlobalSessionStatus(t, e.baseURL, resumedID, "exited")
	if status, body := doJSON(t, "PUT", e.baseURL+"/api/global", map[string]any{"root_path": ""}); status != http.StatusOK {
		t.Fatalf("root clear after stops: status = %d, want 200; body=%v", status, body)
	}
	if status, body := doJSON(t, "PUT", e.baseURL+"/api/global", map[string]any{"root_path": root}); status != http.StatusOK {
		t.Fatalf("re-PUT root after clear: status = %d, want 200; body=%v", status, body)
	}
	status, body = doJSON(t, "POST", e.baseURL+"/api/sessions", map[string]any{"scope": "global", "kind": "agent", "resume": true})
	if status != http.StatusConflict {
		t.Fatalf("resume after clear: status = %d, want 409; body=%v", status, body)
	}
	if got, _ := body["error"].(string); got != "no global opencode session to resume" {
		t.Errorf("resume-after-clear error = %q, want %q", got, "no global opencode session to resume")
	}
}

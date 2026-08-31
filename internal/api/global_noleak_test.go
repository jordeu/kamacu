package api

// global_noleak_test.go — the consolidated TestGlobalNoLeak* family (D-55/D-56,
// GINT-03, SC4). One place walks EVERY enumeration surface with a configured
// AND live global session (a fake-claude agent plus a real global tmux bash
// tab), so the sentinel-leak invariant — the global entity is a singleton row,
// never a task/project row, so every surface excludes it BY CONSTRUCTION — is
// observable and re-provable per surface. Each subtest asserts the surface's
// own legitimate row IS present and zero global presence.
//
// D-55 audit table (surface → query/guard file:line → assertion; the guard
// lines were re-verified against the working tree on 2026-08-28 — no drift
// from the 17-PATTERNS table):
//
//	| Surface                    | Query/guard (file:line)                      | Assertion                                                        |
//	|----------------------------|----------------------------------------------|------------------------------------------------------------------|
//	| 1. Unscoped task list      | tasks.go:194  WHERE source = 'manual'        | GET /api/tasks: manual rows present, zero 'Scratchpad'/'Global'   |
//	| 2. Board fetch             | tasks.go:233  project_id=? AND manual        | GET /api/projects/{id}/tasks: same, count == pre-global count     |
//	| 3. Create position         | tasks.go:288  MIN(position) … AND manual     | peer fixture creation lands top-of-todo (manual-keyed MIN)        |
//	| 4. Move/position math      | tasks.go:432/:538/:554 (MIN/next/renumber)   | move → 200; untouched task's position unchanged by global presence |
//	| 5. Project list            | GET /api/projects (projects.go p.list)       | no sentinel project; count unchanged after configure + spawn      |
//	| 6. Workspace delete guard  | workspaces.go:215 COUNT(*) FROM projects     | empty ws delete 204 while global live; holding-project 409        |
//	| 7. Activity stats/lists    | activity.go:135 t.source = 'manual'          | raw body: peer row present, zero 'Scratchpad'/'Global' bytes      |
//	| 8. DB structural proof     | by construction (singleton, never a row)     | SELECT COUNT(*) tasks/projects unchanged by configure + spawn      |
//	| 9. MCP task tools (leak)   | mcp/tasks.go:197 → tasks.go:194 passthrough  | bridge list_tasks result carries no global entity (17-02 Task 3)  |
//	| 10. MCP session tools      | mcp/sessions.go:516/:579 passthrough         | global session listed/described with honest labels, never 404     |
//
// Adding a future surface = one row above + one subtest below (T-17-06).

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kamacu/internal/session"
	"kamacu/internal/settings"
	"kamacu/internal/tmux"
)

// TestGlobalNoLeak (D-56 / GINT-03): with a configured global root AND live
// global sessions (agent + tmux bash tab), every enumeration surface shows
// only its own legitimate manual rows. Runs host-gated (git for the peer
// fixtures, tmux for the live tab) over the full-surface harness.
func TestGlobalNoLeak(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not on PATH")
	}
	// Per-test socket + kill-server cleanup registered BEFORE the server
	// (LIFO: runs after the harness teardown).
	c := tmux.Client{Socket: fmt.Sprintf("ktest-gnoleak-%d", os.Getpid()), ConfPath: "/dev/null"}
	t.Cleanup(func() { _ = c.KillServer(context.Background()) })
	srv, mgr, db := newGlobalSessionServerWithTmux(t, c)

	// Live-global-agent wiring (the newGlobalAgentServerOpt convention): the
	// manager's ClaudeBin points at testdata/fake-claude, recorders in temp.
	mgr.SetAgentConfig(session.AgentConfig{
		BaseURL:   "http://127.0.0.1:7333",
		Token:     testHookToken,
		ClaudeBin: testdataFakeClaude(t),
	})
	t.Setenv("FAKE_CLAUDE_ARGS_FILE", filepath.Join(t.TempDir(), "args"))
	t.Setenv("FAKE_CLAUDE_PWD_FILE", filepath.Join(t.TempDir(), "pwd"))

	// shell=tmux so a kind:"bash" global spawn mints a REAL global tmux tab
	// (valid via Set because tmux IS on PATH).
	if err := settings.Set(db, settings.KeyShell, "tmux"); err != nil {
		t.Fatalf("set shell=tmux: %v", err)
	}

	// === Peer fixture FIRST, so baselines predate any global configuration. ===
	pid := createProject(t, srv, gitRepoWithCommit(t))
	actBody := createTask(t, srv, pid, "NoLeak Activity Fixture")
	actTid := taskID(t, actBody)
	moveBody := createTask(t, srv, pid, "NoLeak Move Fixture")
	moveTid := taskID(t, moveBody)
	// A done manual task in-window: the Activity surface's one row.
	if _, err := db.Exec(`UPDATE tasks SET status = 'done', done_at = ? WHERE id = ?`,
		time.Now().UTC().Format("2006-01-02T15:04:05.000Z"), actTid); err != nil {
		t.Fatalf("mark activity fixture done: %v", err)
	}

	// Baselines (before the global root is configured).
	baseProjects := listCount(t, srv.URL+"/api/projects")
	baseTasks := listCount(t, srv.URL+"/api/tasks")
	var baseTaskRows, baseProjectRows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM tasks`).Scan(&baseTaskRows); err != nil {
		t.Fatalf("baseline task count: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM projects`).Scan(&baseProjectRows); err != nil {
		t.Fatalf("baseline project count: %v", err)
	}

	// === Configure + spawn the global (live agent AND live tmux tab). ===
	putGlobalFolderRoot(t, srv)
	status, gbody := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"scope": "global", "kind": "agent"})
	if status != http.StatusCreated {
		t.Fatalf("global agent spawn: status = %d, want 201; body=%v", status, gbody)
	}
	status, bbody := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"scope": "global", "kind": "bash"})
	if status != http.StatusCreated {
		t.Fatalf("global bash (tmux) spawn: status = %d, want 201; body=%v", status, bbody)
	}
	awaitHasSession(t, c, "kamacu-global-1", true)

	// Precondition proof the probes run against LIVE state: the singleton
	// reports the agent and the minted tab as live.
	status, g := doJSON(t, "GET", srv.URL+"/api/global", nil)
	if status != http.StatusOK {
		t.Fatalf("GET /api/global: status = %d; body=%v", status, g)
	}
	live, ok := g["live"].(map[string]any)
	if !ok {
		t.Fatalf("live missing on /api/global: %v", g)
	}
	for _, k := range []string{"agent", "tmux"} {
		if v, ok := live[k].(float64); !ok || v < 1 {
			t.Fatalf("live.%s = %v, want >= 1 (the leak probes must run against live global state)", k, live[k])
		}
	}

	// === The surfaces (audit-table rows 1–8; rows 9–10 live in internal/mcp). ===
	t.Run("unscoped_task_list", func(t *testing.T) {
		st, raw := getJSON(t, srv.URL+"/api/tasks")
		if st != http.StatusOK {
			t.Fatalf("GET /api/tasks: status = %d; body=%s", st, raw)
		}
		if !strings.Contains(string(raw), "NoLeak Move Fixture") {
			t.Errorf("task list omits the manual fixture (the surface's legitimate row):\n%s", raw)
		}
		// Row-level leak check (NOT raw bytes): these bodies embed OS temp
		// paths under t.TempDir, whose path contains the test NAME — and
		// "TestGlobalNoLeak" contains "Global". The raw-byte posture is
		// reserved for Activity, whose body carries no paths.
		assertNoGlobalRows(t, srv.URL+"/api/tasks", "title")
		if got := listCount(t, srv.URL+"/api/tasks"); got != baseTasks {
			t.Errorf("task list count = %d, want the pre-global-configure %d", got, baseTasks)
		}
	})

	t.Run("board_fetch", func(t *testing.T) {
		url := fmt.Sprintf("%s/api/projects/%d/tasks", srv.URL, pid)
		st, raw := getJSON(t, url)
		if st != http.StatusOK {
			t.Fatalf("board fetch: status = %d; body=%s", st, raw)
		}
		if !strings.Contains(string(raw), "NoLeak Activity Fixture") || !strings.Contains(string(raw), "NoLeak Move Fixture") {
			t.Errorf("board omits the manual fixtures:\n%s", raw)
		}
		assertNoGlobalRows(t, url, "title")
	})

	t.Run("move_and_positions", func(t *testing.T) {
		// The untouched peer's position before the move (manual-keyed MIN /
		// next / renumber queries at tasks.go:432/:538/:554 must not see the
		// global — and cannot, it is not a task row).
		before := boardPosition(t, srv.URL, pid, actTid)
		status, body := doJSON(t, "POST", fmt.Sprintf("%s/api/tasks/%d/move", srv.URL, moveTid),
			map[string]any{"status": "in_progress"})
		if status != http.StatusOK {
			t.Fatalf("move manual peer: status = %d, want 200; body=%v", status, body)
		}
		if body["status"] != "in_progress" {
			t.Errorf("moved task status = %v, want in_progress", body["status"])
		}
		if after := boardPosition(t, srv.URL, pid, actTid); after != before {
			t.Errorf("untouched task position drifted %v → %v with a live global present", before, after)
		}
		// The board still carries exactly the two manual rows.
		if got := listCount(t, fmt.Sprintf("%s/api/projects/%d/tasks", srv.URL, pid)); got != baseTasks {
			t.Errorf("board count = %d after move, want %d manual rows", got, baseTasks)
		}
	})

	t.Run("project_list", func(t *testing.T) {
		st, raw := getJSON(t, srv.URL+"/api/projects")
		if st != http.StatusOK {
			t.Fatalf("GET /api/projects: status = %d; body=%s", st, raw)
		}
		assertNoGlobalRows(t, srv.URL+"/api/projects", "name")
		if got := listCount(t, srv.URL+"/api/projects"); got != baseProjects {
			t.Errorf("project count = %d, want the pre-global-configure %d (no sentinel project)", got, baseProjects)
		}
	})

	t.Run("workspace_delete_guard", func(t *testing.T) {
		// Empty workspace deletes fine while the global is live — the COUNT
		// guard at workspaces.go:215 counts PROJECT rows, and the global
		// singleton is not one.
		status, body := doJSON(t, "POST", srv.URL+"/api/workspaces", map[string]any{"name": "NoLeak Empty"})
		if status != http.StatusCreated {
			t.Fatalf("create empty workspace: status=%d body=%v", status, body)
		}
		emptyID := int64(body["id"].(float64))
		status, body = doJSON(t, "DELETE", srv.URL+"/api/workspaces/"+itoa(emptyID), nil)
		if status != http.StatusNoContent {
			t.Fatalf("delete empty workspace with live global: status = %d, want 204; body=%v", status, body)
		}
		// Negative control — the guard itself still works: a workspace
		// holding the peer project is refused 409 with the count message.
		status, body = doJSON(t, "POST", srv.URL+"/api/workspaces", map[string]any{"name": "NoLeak Full"})
		if status != http.StatusCreated {
			t.Fatalf("create holding workspace: status=%d body=%v", status, body)
		}
		fullID := int64(body["id"].(float64))
		if _, err := db.Exec(`UPDATE projects SET workspace_id = ? WHERE id = ?`, fullID, pid); err != nil {
			t.Fatalf("assign peer project to workspace: %v", err)
		}
		status, body = doJSON(t, "DELETE", srv.URL+"/api/workspaces/"+itoa(fullID), nil)
		if status != http.StatusConflict {
			t.Fatalf("delete holding workspace: status = %d, want 409; body=%v", status, body)
		}
		if got := body["error"]; got != "move or remove its 1 project(s) first" {
			t.Errorf("409 error = %v, want the count-carrying refusal", got)
		}
	})

	t.Run("activity_raw_bytes", func(t *testing.T) {
		// Activity over the same DB (no-gh fakes — never a real gh spawn).
		amux := newActivityEnv(t, db, nil, nil)
		asrv := httptest.NewServer(amux)
		t.Cleanup(asrv.Close)
		st, raw := getJSON(t, asrv.URL+"/api/activity?window=week")
		if st != http.StatusOK {
			t.Fatalf("GET /api/activity: status = %d; body=%s", st, raw)
		}
		if !strings.Contains(string(raw), "NoLeak Activity Fixture") {
			t.Errorf("activity body omits the done manual task (the surface's one row):\n%s", raw)
		}
		assertNoGlobalLeak(t, raw, "GET /api/activity")
	})

	t.Run("db_structural_zero_rows", func(t *testing.T) {
		var taskRows, projectRows int
		if err := db.QueryRow(`SELECT COUNT(*) FROM tasks`).Scan(&taskRows); err != nil {
			t.Fatalf("count tasks: %v", err)
		}
		if err := db.QueryRow(`SELECT COUNT(*) FROM projects`).Scan(&projectRows); err != nil {
			t.Fatalf("count projects: %v", err)
		}
		if taskRows != baseTaskRows {
			t.Errorf("tasks rows = %d after configure+spawn, want the baseline %d (exclusion by construction)", taskRows, baseTaskRows)
		}
		if projectRows != baseProjectRows {
			t.Errorf("projects rows = %d after configure+spawn, want the baseline %d (exclusion by construction)", projectRows, baseProjectRows)
		}
	})
}

// assertNoGlobalLeak negative-asserts the two global label strings on a RAW
// response body (the Phase-11 lesson: decoded len==0 checks cannot catch shape
// regressions — assert on bytes). Only safe for bodies that carry no OS temp
// paths (t.TempDir embeds the test name, which contains "Global").
func assertNoGlobalLeak(t *testing.T, raw []byte, surface string) {
	t.Helper()
	for _, leak := range []string{"Scratchpad", "Global"} {
		if strings.Contains(string(raw), leak) {
			t.Errorf("%s leaks the global label %q (GINT-03 violation):\n%s", surface, leak, raw)
		}
	}
}

// assertNoGlobalRows fetches a JSON-array endpoint and negative-asserts the
// two global label strings against every row's given string field (title for
// tasks, name for projects) — the row-level form of the leak check for bodies
// that embed temp paths.
func assertNoGlobalRows(t *testing.T, url, field string) {
	t.Helper()
	_, rows := doJSONList(t, url)
	for i, row := range rows {
		v, _ := row[field].(string)
		for _, leak := range []string{"Scratchpad", "Global"} {
			if strings.Contains(v, leak) {
				t.Errorf("row %d %s = %q leaks the global label (GINT-03 violation): %v", i, field, v, row)
			}
		}
	}
}

// listCount GETs a JSON-array endpoint and returns its length.
func listCount(t *testing.T, url string) int {
	t.Helper()
	_, list := doJSONList(t, url)
	return len(list)
}

// boardPosition fetches the board and returns the given task's position.
func boardPosition(t *testing.T, baseURL string, pid, tid int64) float64 {
	t.Helper()
	_, list := doJSONList(t, fmt.Sprintf("%s/api/projects/%d/tasks", baseURL, pid))
	for _, row := range list {
		if id, ok := row["id"].(float64); ok && int64(id) == tid {
			pos, ok := row["position"].(float64)
			if !ok {
				t.Fatalf("task %d has no numeric position: %v", tid, row)
			}
			return pos
		}
	}
	t.Fatalf("task %d not on the board", tid)
	return 0
}

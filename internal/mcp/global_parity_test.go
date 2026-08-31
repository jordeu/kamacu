package mcp

// global_parity_test.go — the D-57 / GINT-02 both-direction parity spot-check
// through the REAL bridge (in-memory transport, the sessions_test.go house
// pattern): an httptest backend seeded with the REAL wire shape of Kamacu's
// responses while a global session is live, driving bridge{base, token,
// client} and unwrapping CallToolResult TextContent.
//
// Wire shapes are seeded from the verified /api/sessions handler output
// (internal/api/sessions.go list: global entries carry taskTitle "Scratchpad"
// / projectName "Global" / agentName from the singleton JOIN; session.Info
// JSON tags: global/orphaned omitempty, tmuxName on orphaned rows only) and
// the /api/tasks manual-only array (tasks.go:194 — the API-level exclusion is
// proven in plan 17-02 Task 1 against the real handlers; this leg locks the
// BRIDGE contract). Read-only: no write-path bridge calls anywhere here.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// kamacuTasksWithLiveGlobal is GET /api/tasks verbatim while a global agent
// AND a global tmux tab are live: manual rows ONLY (the tasks.go:194
// source='manual' guard excludes the global by construction — it is not a
// task row). No path in the fixture contains the leak strings.
const kamacuTasksWithLiveGlobal = `[
  {"id":7,"project_id":3,"title":"Ship the widget","description":"","status":"todo","position":1,"created_at":"2026-08-28T09:00:00Z","updated_at":"2026-08-28T09:00:00Z","branch":"task/ship-the-widget-7","worktree_path":"/home/dev/repos/octo/widgets-wt","worktree_error":null,"source":"manual","pr_number":null,"pr_base_ref":null},
  {"id":9,"project_id":4,"title":"Fix the flaky test","description":"","status":"in_progress","position":1,"created_at":"2026-08-28T09:30:00Z","updated_at":"2026-08-28T09:30:00Z","branch":"task/fix-the-flaky-test-9","worktree_path":"/home/dev/repos/octo/gadgets-wt","worktree_error":null,"source":"manual","pr_number":null,"pr_base_ref":null}
]`

// TestGlobalParityTaskToolsLeakFree (D-57 leak direction): list_tasks passes
// GET /api/tasks through verbatim — the manual identifiers surface, and ZERO
// global entity (no "Scratchpad", no "Global") reaches the agent.
func TestGlobalParityTaskToolsLeakFree(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(kamacuTasksWithLiveGlobal))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	res, err := b.listTasks(context.Background(), newCallToolRequest(nil))
	if err != nil {
		t.Fatalf("listTasks: %v", err)
	}
	tc, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("result.Content[0]: want *TextContent, got %T", res.Content[0])
	}
	if !strings.Contains(tc.Text, "Ship the widget") || !strings.Contains(tc.Text, "Fix the flaky test") {
		t.Errorf("list_tasks result omits the manual fixtures:\n%s", tc.Text)
	}
	for _, leak := range []string{"Scratchpad", "Global"} {
		if strings.Contains(tc.Text, leak) {
			t.Errorf("list_tasks result leaks the global label %q (D-57 violation):\n%s", leak, tc.Text)
		}
	}
}

// kamacuSessionsWithLiveGlobal is GET /api/sessions verbatim while a global
// agent is live AND a post-restart global tmux tab is orphaned: the live
// global agent row (honest labels, real operable id), a manual task-scoped
// row, and the orphaned survivor row (id "", orphaned true — the SPA's
// reattach affordance, the exact post-restart shape from plan 17-01).
const kamacuSessionsWithLiveGlobal = `[
  {"id":"5f0d9c2e-64b3-4d0a-9a1e-8b2c7d3e9a01","label":"Agent","status":"running","createdAt":"2026-08-28T09:00:00Z","kind":"agent","engine":"claude","global":true,"taskTitle":"Scratchpad","projectName":"Global","agentName":"Claude Code"},
  {"id":"7a1e4b8f-92c4-4f5b-b3d0-6c8e1f2a3b02","label":"bash #1","status":"running","createdAt":"2026-08-28T09:05:00Z","kind":"bash","taskId":7,"taskTitle":"Ship the widget","projectName":"Widgets","agentName":"Claude Code"},
  {"id":"","label":"Bash 1","status":"running","createdAt":"2026-08-28T08:00:00Z","kind":"bash","global":true,"orphaned":true,"tmuxName":"kamacu-global-1","taskTitle":"Scratchpad","projectName":"Global","agentName":"Claude Code"}
]`

// TestGlobalParitySessionToolsHonestLabels (D-57 honest direction / GINT-02):
// list_sessions surfaces the LIVE global session with its Scratchpad/Global
// labels intact — the D-13 orphan filter is keyed on the orphaned flag, so a
// live (orphaned:false) global row passes through untouched, never dropped,
// never garbled.
func TestGlobalParitySessionToolsHonestLabels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(kamacuSessionsWithLiveGlobal))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	res, err := b.listSessions(context.Background(), newCallToolRequest(nil))
	if err != nil {
		t.Fatalf("listSessions: %v", err)
	}
	tc, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("result.Content[0]: want *TextContent, got %T", res.Content[0])
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(tc.Text), &rows); err != nil {
		t.Fatalf("decoded list_sessions JSON: %v\nraw=%q", err, tc.Text)
	}

	const globalID = "5f0d9c2e-64b3-4d0a-9a1e-8b2c7d3e9a01"
	var global, manual map[string]any
	orphanedSeen := 0
	for _, row := range rows {
		if row["id"] == globalID {
			global = row
		}
		if row["id"] == "7a1e4b8f-92c4-4f5b-b3d0-6c8e1f2a3b02" {
			manual = row
		}
		if orb, _ := row["orphaned"].(bool); orb {
			orphanedSeen++
		}
	}
	if global == nil {
		t.Fatalf("live global session missing from list_sessions result (the filter must not drop it):\n%s", tc.Text)
	}
	// Honest labels intact through the filter's decode/re-marshal round-trip.
	if got := global["taskTitle"]; got != "Scratchpad" {
		t.Errorf("global taskTitle = %v, want %q", got, "Scratchpad")
	}
	if got := global["projectName"]; got != "Global" {
		t.Errorf("global projectName = %v, want %q", got, "Global")
	}
	if got, _ := global["global"].(bool); !got {
		t.Errorf("global flag lost on the passthrough: %v", global)
	}
	if got := global["kind"]; got != "agent" {
		t.Errorf("global kind = %v, want agent", got)
	}
	// The manual peer keeps its own labels — no cross-contamination.
	if manual == nil {
		t.Fatalf("manual session missing from list_sessions result:\n%s", tc.Text)
	}
	if got := manual["taskTitle"]; got != "Ship the widget" {
		t.Errorf("manual taskTitle = %v, want the task's own title", got)
	}
	if _, present := manual["global"]; present {
		t.Errorf("manual session wrongly carries the global flag: %v", manual)
	}
	// The orphaned (id="") survivor row is filtered by the D-13 operable-ids
	// contract — exactly like a task-scoped orphaned row. It carries NO
	// operable session id, so it cannot be dropped-or-kept on scope; the
	// honest global surface for it is the SPA's reattach affordance, not the
	// agent's id-keyed tools.
	if orphanedSeen != 0 {
		t.Errorf("orphaned rows leaked into the agent-facing list (D-13): %d seen", orphanedSeen)
	}
}

// TestGlobalParityGetSessionGlobalID (D-57 honest direction, get leg):
// get_session of the global session's id is a 200-shaped passthrough carrying
// the honest labels — the bridge surfaces it, never 404 (a 404 would surface
// as an error from bridge.call's non-2xx wrap).
func TestGlobalParityGetSessionGlobalID(t *testing.T) {
	const canned = `{"id":"5f0d9c2e-64b3-4d0a-9a1e-8b2c7d3e9a01","label":"Agent","status":"running","createdAt":"2026-08-28T09:00:00Z","kind":"agent","engine":"claude","global":true,"taskTitle":"Scratchpad","projectName":"Global","agentName":"Claude Code"}`
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(canned))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"session_id":"5f0d9c2e-64b3-4d0a-9a1e-8b2c7d3e9a01"}`))
	res, err := b.getSession(context.Background(), req)
	if err != nil {
		t.Fatalf("get_session of the global id: %v (never a 404-shaped error)", err)
	}
	if want := "/api/sessions/5f0d9c2e-64b3-4d0a-9a1e-8b2c7d3e9a01"; gotPath != want {
		t.Errorf("request path = %q, want %q", gotPath, want)
	}
	tc, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("result.Content[0]: want *TextContent, got %T", res.Content[0])
	}
	var row map[string]any
	if err := json.Unmarshal([]byte(tc.Text), &row); err != nil {
		t.Fatalf("decoded get_session JSON: %v\nraw=%q", err, tc.Text)
	}
	if got := row["taskTitle"]; got != "Scratchpad" {
		t.Errorf("taskTitle = %v, want %q", got, "Scratchpad")
	}
	if got := row["projectName"]; got != "Global" {
		t.Errorf("projectName = %v, want %q", got, "Global")
	}
	if got, _ := row["global"].(bool); !got {
		t.Errorf("global flag lost on the get_session passthrough: %v", row)
	}
}

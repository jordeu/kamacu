package api

import (
	"net/http"
	"path/filepath"
	"testing"

	"kamacu/internal/store"
)

// personalWorkspaceID returns the id of the default (is_default=true) workspace
// from GET /api/workspaces. On a fresh migrated DB that is Personal (id 1).
func personalWorkspaceID(t *testing.T, srvURL string) int64 {
	t.Helper()
	status, list := doJSONList(t, srvURL+"/api/workspaces")
	if status != http.StatusOK {
		t.Fatalf("GET /api/workspaces: status=%d", status)
	}
	for _, ws := range list {
		if def, _ := ws["is_default"].(bool); def {
			return int64(ws["id"].(float64))
		}
	}
	t.Fatalf("no default workspace in %v", list)
	return 0
}

// TestWorkspaceList proves GET /api/workspaces returns exactly the seeded
// Personal workspace with is_default true on a fresh migrated DB (WSNAV-01).
func TestWorkspaceList(t *testing.T) {
	srv, _, _ := newTestServer(t)

	status, list := doJSONList(t, srv.URL+"/api/workspaces")
	if status != http.StatusOK {
		t.Fatalf("status=%d, want 200", status)
	}
	if len(list) != 1 {
		t.Fatalf("workspaces = %d, want 1 (seeded Personal)", len(list))
	}
	if list[0]["name"] != "Personal" {
		t.Errorf("name = %v, want Personal", list[0]["name"])
	}
	if def, _ := list[0]["is_default"].(bool); !def {
		t.Errorf("is_default = %v, want true", list[0]["is_default"])
	}
}

// TestWorkspaceCreate proves POST creates a workspace (201) and a following GET
// then lists two (WSMGMT-01).
func TestWorkspaceCreate(t *testing.T) {
	srv, _, _ := newTestServer(t)

	status, body := doJSON(t, "POST", srv.URL+"/api/workspaces", map[string]any{"name": "Work"})
	if status != http.StatusCreated {
		t.Fatalf("create: status=%d, want 201; body=%v", status, body)
	}
	if body["name"] != "Work" {
		t.Errorf("name = %v, want Work", body["name"])
	}
	if def, _ := body["is_default"].(bool); def {
		t.Errorf("new workspace is_default = %v, want false", body["is_default"])
	}

	status, list := doJSONList(t, srv.URL+"/api/workspaces")
	if status != http.StatusOK || len(list) != 2 {
		t.Fatalf("after create: status=%d count=%d, want 200/2", status, len(list))
	}
}

// TestWorkspaceCreateDuplicate proves a case-insensitive duplicate name is
// rejected 409 (WSMGMT-01, T-26-03).
func TestWorkspaceCreateDuplicate(t *testing.T) {
	srv, _, _ := newTestServer(t)

	if status, body := doJSON(t, "POST", srv.URL+"/api/workspaces", map[string]any{"name": "Work"}); status != http.StatusCreated {
		t.Fatalf("seed create: status=%d body=%v", status, body)
	}
	status, body := doJSON(t, "POST", srv.URL+"/api/workspaces", map[string]any{"name": "work"})
	if status != http.StatusConflict {
		t.Fatalf("case-insensitive dup: status=%d, want 409; body=%v", status, body)
	}
	if body["error"] != "a workspace with that name already exists" {
		t.Errorf("error = %q, want the duplicate message", body["error"])
	}
}

// TestWorkspaceCreateEmpty proves a blank name is rejected 400 (WSMGMT-01).
func TestWorkspaceCreateEmpty(t *testing.T) {
	srv, _, _ := newTestServer(t)

	status, body := doJSON(t, "POST", srv.URL+"/api/workspaces", map[string]any{"name": "  "})
	if status != http.StatusBadRequest {
		t.Fatalf("empty name: status=%d, want 400; body=%v", status, body)
	}
	if body["error"] != "name is required" {
		t.Errorf("error = %q, want %q", body["error"], "name is required")
	}
}

// TestWorkspaceRenamePersonal proves the DEFAULT workspace is renamable — rename
// is never gated on is_default (WSMGMT-02, D-04).
func TestWorkspaceRenamePersonal(t *testing.T) {
	srv, _, _ := newTestServer(t)
	pid := personalWorkspaceID(t, srv.URL)

	status, body := doJSON(t, "PATCH", srv.URL+"/api/workspaces/"+itoa(pid), map[string]any{"name": "Home"})
	if status != http.StatusOK {
		t.Fatalf("rename Personal: status=%d, want 200; body=%v", status, body)
	}
	if body["name"] != "Home" {
		t.Errorf("name = %v, want Home", body["name"])
	}
	// The default flag is preserved by a rename.
	if def, _ := body["is_default"].(bool); !def {
		t.Errorf("is_default = %v after rename, want true (rename never clears the flag)", body["is_default"])
	}
}

// TestWorkspaceRenameCollision proves renaming to an existing (other) name is
// rejected 409 (WSMGMT-02, T-26-03).
func TestWorkspaceRenameCollision(t *testing.T) {
	srv, _, _ := newTestServer(t)

	status, body := doJSON(t, "POST", srv.URL+"/api/workspaces", map[string]any{"name": "Work"})
	if status != http.StatusCreated {
		t.Fatalf("seed Work: status=%d body=%v", status, body)
	}
	wsID := int64(body["id"].(float64))

	// Rename the new "Work" workspace to "Personal" (an existing other name) → 409.
	status, body = doJSON(t, "PATCH", srv.URL+"/api/workspaces/"+itoa(wsID), map[string]any{"name": "Personal"})
	if status != http.StatusConflict {
		t.Fatalf("rename collision: status=%d, want 409; body=%v", status, body)
	}
	if body["error"] != "a workspace with that name already exists" {
		t.Errorf("error = %q, want the duplicate message", body["error"])
	}
}

// TestWorkspaceDeleteEmpty proves an empty, non-default workspace is removed 204
// (WSMGMT-03).
func TestWorkspaceDeleteEmpty(t *testing.T) {
	srv, _, _ := newTestServer(t)

	status, body := doJSON(t, "POST", srv.URL+"/api/workspaces", map[string]any{"name": "Scratch"})
	if status != http.StatusCreated {
		t.Fatalf("seed Scratch: status=%d body=%v", status, body)
	}
	wsID := int64(body["id"].(float64))

	status, _ = doJSON(t, "DELETE", srv.URL+"/api/workspaces/"+itoa(wsID), nil)
	if status != http.StatusNoContent {
		t.Fatalf("delete empty: status=%d, want 204", status)
	}
	// Gone from the list.
	_, list := doJSONList(t, srv.URL+"/api/workspaces")
	if len(list) != 1 {
		t.Fatalf("after delete: workspaces = %d, want 1", len(list))
	}
}

// TestWorkspaceDeleteNonEmpty proves a workspace still owning a project is
// refused 409 (WSMGMT-03, D-05). The project is placed via a direct UPDATE
// (no /api/workspaces assignment endpoint) so the test stays independent of
// plan 02's transfer surface.
func TestWorkspaceDeleteNonEmpty(t *testing.T) {
	srv, db, _ := newTestServer(t)

	projID := createProject(t, srv, gitRepo(t))

	status, body := doJSON(t, "POST", srv.URL+"/api/workspaces", map[string]any{"name": "Work"})
	if status != http.StatusCreated {
		t.Fatalf("seed Work: status=%d body=%v", status, body)
	}
	wsID := int64(body["id"].(float64))

	// Direct assignment — stays independent of plan 02's transfer endpoint.
	if _, err := db.Exec(`UPDATE projects SET workspace_id = ? WHERE id = ?`, wsID, projID); err != nil {
		t.Fatalf("assign project to workspace: %v", err)
	}

	status, body = doJSON(t, "DELETE", srv.URL+"/api/workspaces/"+itoa(wsID), nil)
	if status != http.StatusConflict {
		t.Fatalf("delete non-empty: status=%d, want 409; body=%v", status, body)
	}
	if body["error"] != "move or remove its 1 project(s) first" {
		t.Errorf("error = %q, want a count-carrying refusal", body["error"])
	}
}

// TestWorkspaceDeleteDefault proves the default (Personal) workspace can never be
// deleted (WSMGMT-04, D-06 — keyed off is_default, not the name).
func TestWorkspaceDeleteDefault(t *testing.T) {
	srv, _, _ := newTestServer(t)
	pid := personalWorkspaceID(t, srv.URL)

	status, body := doJSON(t, "DELETE", srv.URL+"/api/workspaces/"+itoa(pid), nil)
	if status != http.StatusConflict {
		t.Fatalf("delete default: status=%d, want 409; body=%v", status, body)
	}
	if body["error"] != "the default workspace can't be deleted" {
		t.Errorf("error = %q, want the default-protected message", body["error"])
	}
}

// TestBackfillWorkspaces proves the three BackfillWorkspaces behaviors on a
// migrated DB (store.Open + store.Migrate harness):
//
//	(1) no-op on a healthy boot where migration 00012 already created the default
//	    Personal workspace (never a second Personal);
//	(2) re-create Personal when the is_default=1 row is missing;
//	(3) idempotency on a double run (still exactly one workspace).
//
// This is the LEAN invariant guard, not a project-reassign loop: migration 00012's
// NOT NULL DEFAULT 1 FK already assigns every project, so there is nothing to
// backfill on projects (see the D-07 reinterpretation note in 25-03-PLAN.md,
// user-confirmed 2026-07-05).
func TestBackfillWorkspaces(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()
	if err := store.Migrate(db); err != nil {
		t.Fatalf("store.Migrate: %v", err)
	}

	countWorkspaces := func() int {
		t.Helper()
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM workspaces`).Scan(&n); err != nil {
			t.Fatalf("count workspaces: %v", err)
		}
		return n
	}
	countDefaults := func() int {
		t.Helper()
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM workspaces WHERE is_default = 1`).Scan(&n); err != nil {
			t.Fatalf("count default workspaces: %v", err)
		}
		return n
	}

	// (1) Healthy boot: migration 00012 already created Personal, so
	// BackfillWorkspaces is a no-op and does NOT insert a second workspace.
	if got := countWorkspaces(); got != 1 {
		t.Fatalf("after migrate: workspaces = %d, want 1 (migration seeds Personal)", got)
	}
	if err := BackfillWorkspaces(db); err != nil {
		t.Fatalf("BackfillWorkspaces (healthy boot): %v", err)
	}
	if got := countWorkspaces(); got != 1 {
		t.Fatalf("after no-op backfill: workspaces = %d, want 1 (no second Personal)", got)
	}

	// (2) Missing default: with no referencing projects the ON DELETE RESTRICT FK
	// permits dropping the default row, leaving the table empty; BackfillWorkspaces
	// re-creates Personal (is_default=1).
	if _, err := db.Exec(`DELETE FROM workspaces WHERE is_default = 1`); err != nil {
		t.Fatalf("delete default workspace: %v", err)
	}
	if got := countWorkspaces(); got != 0 {
		t.Fatalf("after delete: workspaces = %d, want 0", got)
	}
	if err := BackfillWorkspaces(db); err != nil {
		t.Fatalf("BackfillWorkspaces (missing default): %v", err)
	}
	if got := countDefaults(); got != 1 {
		t.Fatalf("after re-create: default workspaces = %d, want 1", got)
	}
	if got := countWorkspaces(); got != 1 {
		t.Fatalf("after re-create: workspaces = %d, want 1", got)
	}

	// (3) Idempotent double run: calling again keeps exactly one workspace
	// (no second Personal).
	if err := BackfillWorkspaces(db); err != nil {
		t.Fatalf("BackfillWorkspaces (double run): %v", err)
	}
	if got := countWorkspaces(); got != 1 {
		t.Fatalf("after double run: workspaces = %d, want 1 (idempotent)", got)
	}
}

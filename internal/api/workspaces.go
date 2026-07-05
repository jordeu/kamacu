package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// BackfillWorkspaces guarantees the WSDATA-02 invariant at startup, mirroring
// api.BackfillProjectIcons: it runs ONCE right after store.Migrate(db) and is
// IDEMPOTENT -- a cheap no-op on healthy boots.
//
// Unlike the icon backfill (which FILLS blank columns via a SELECT->UPDATE loop),
// migration 00012 already creates the default Personal workspace AND assigns every
// project to it via the projects.workspace_id NOT NULL DEFAULT 1 FK column. So this
// hook's active job is only the safety net: guarantee a default workspace row
// exists (WSMGMT-04: at least one workspace always exists). Under NOT NULL + FK +
// DEFAULT no project can be workspace-less, so there is deliberately NO
// SELECT->UPDATE reassignment loop over projects -- nothing to reassign.
//
// This is the LEAN reinterpretation of D-07's "collect-then-update" mechanism,
// user-confirmed 2026-07-05 via /gsd:plan-phase (see the D-07 note in 25-03-PLAN.md):
// both forms meet D-07's stated job identically with no data risk. All SQL is
// parameterless/literal (only a SELECT and a literal 'Personal' INSERT) -- no
// string-concatenated input (T-25-01).
func BackfillWorkspaces(db *sql.DB) error {
	var id int64
	err := db.QueryRow(`SELECT id FROM workspaces WHERE is_default = 1`).Scan(&id)
	if err == nil {
		return nil // default already exists -> no-op (the healthy-boot path)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err // a real DB error is propagated, never swallowed
	}
	// No default workspace: re-create Personal (defensive; 00012 normally created it).
	_, err = db.Exec(`INSERT INTO workspaces (name, is_default) VALUES ('Personal', 1)`)
	return err
}

// Workspace is the JSON shape of a workspaces row (migration 00012), mirroring
// Project. IsDefault marks the protected default workspace (D-06: identified by
// the flag, rename-proof — never by the name "Personal"). SQLite stores
// is_default as INTEGER 0/1; scanWorkspace maps it to bool.
type Workspace struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	IsDefault bool   `json:"is_default"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// workspaceColumns is the canonical SELECT/RETURNING column list; its ORDER must
// match the scanWorkspace Scan order.
const workspaceColumns = `id, name, is_default, created_at, updated_at`

// scanWorkspace scans one workspaces row. is_default comes back as INTEGER 0/1;
// scan into an int and map to bool (the managedInt != 0 idiom in scanProject) to
// avoid any modernc bool-scan friction.
func scanWorkspace(row interface{ Scan(...any) error }) (Workspace, error) {
	var ws Workspace
	var isDefaultInt int
	err := row.Scan(&ws.ID, &ws.Name, &isDefaultInt, &ws.CreatedAt, &ws.UpdatedAt)
	ws.IsDefault = isDefaultInt != 0
	return ws, err
}

// workspaceHandlers is the /api/workspaces CRUD surface (D-16), mirroring the
// db-only settingsHandlers shape. Every query uses ? placeholders (T-26-01);
// store.go's SetMaxOpenConns(1) serializes the writes.
type workspaceHandlers struct{ db *sql.DB }

// list handles GET /api/workspaces — every workspace, name-sorted
// (COLLATE NOCASE), returned as a JSON array (never nil, WSNAV-01).
func (h *workspaceHandlers) list(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(`SELECT ` + workspaceColumns + ` FROM workspaces ORDER BY name COLLATE NOCASE`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	workspaces := []Workspace{}
	for rows.Next() {
		ws, err := scanWorkspace(rows)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		workspaces = append(workspaces, ws)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, workspaces)
}

// create handles POST /api/workspaces (WSMGMT-01). Creates a workspace by name;
// a case-insensitive duplicate is rejected 409. The COLLATE NOCASE unique index
// (00012) is the ultimate backstop; the pre-check gives a clean message (T-26-03).
func (h *workspaceHandlers) create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	// Case-insensitive duplicate pre-check (mirrors the folder-create dedup).
	var exists int
	if err := h.db.QueryRow(`SELECT 1 FROM workspaces WHERE name = ? COLLATE NOCASE`, name).Scan(&exists); err == nil {
		writeError(w, http.StatusConflict, "a workspace with that name already exists")
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	ws, err := scanWorkspace(h.db.QueryRow(
		`INSERT INTO workspaces (name) VALUES (?) RETURNING `+workspaceColumns, name))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, ws)
}

// update handles PATCH /api/workspaces/{id} — rename (WSMGMT-02). Rename is NEVER
// gated on is_default: the default Personal workspace is renamable (D-04). A
// case-insensitive collision with a DIFFERENT workspace is rejected 409.
func (h *workspaceHandlers) update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		Name *string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Name == nil {
		writeError(w, http.StatusBadRequest, "nothing to update")
		return
	}
	name := strings.TrimSpace(*req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	// Duplicate pre-check excluding self (a no-op rename to the same name is fine).
	var exists int
	if err := h.db.QueryRow(`SELECT 1 FROM workspaces WHERE name = ? COLLATE NOCASE AND id != ?`, name, id).Scan(&exists); err == nil {
		writeError(w, http.StatusConflict, "a workspace with that name already exists")
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	ws, err := scanWorkspace(h.db.QueryRow(
		`UPDATE workspaces SET name = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id = ? RETURNING `+workspaceColumns,
		name, id))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ws)
}

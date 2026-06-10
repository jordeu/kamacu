package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Project is the JSON shape of a project row (RESEARCH.md Pattern 4).
type Project struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	RepoPath  string `json:"repo_path"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type projectHandlers struct{ db *sql.DB }

// validateRepoPath validates that p is an absolute path to an existing
// directory containing a git repository (RESEARCH.md Pattern 5). A leading
// "~/" is expanded to the user's home directory before validation. Returns
// the cleaned absolute path.
func validateRepoPath(p string) (string, error) {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("path must be absolute")
		}
		p = filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
	if !filepath.IsAbs(p) {
		return "", fmt.Errorf("path must be absolute")
	}
	abs := filepath.Clean(p)
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("not a directory: %s", abs)
	}
	// handles .git-as-directory AND .git-as-file (worktrees/submodules);
	// arg array only — never sh -c
	cmd := exec.Command("git", "-C", abs, "rev-parse", "--git-dir")
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("not a git repository: %s", abs)
	}
	return abs, nil
}

const projectColumns = `id, name, repo_path, created_at, updated_at`

func scanProject(row interface{ Scan(...any) error }) (Project, error) {
	var p Project
	err := row.Scan(&p.ID, &p.Name, &p.RepoPath, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

// list handles GET /api/projects.
func (h *projectHandlers) list(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(`SELECT ` + projectColumns + ` FROM projects ORDER BY name COLLATE NOCASE`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	projects := []Project{}
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		projects = append(projects, p)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, projects)
}

// create handles POST /api/projects.
func (h *projectHandlers) create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		RepoPath string `json:"repo_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	abs, err := validateRepoPath(req.RepoPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Pre-check duplicates (fine at single-user scale).
	var exists int
	if err := h.db.QueryRow(`SELECT 1 FROM projects WHERE repo_path = ?`, abs).Scan(&exists); err == nil {
		writeError(w, http.StatusConflict, "this repository is already added")
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = filepath.Base(abs)
	}
	p, err := scanProject(h.db.QueryRow(
		`INSERT INTO projects (name, repo_path) VALUES (?, ?) RETURNING `+projectColumns, name, abs))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

// update handles PATCH /api/projects/{id} — rename only (repo_path immutable in v1).
func (h *projectHandlers) update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
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
	p, err := scanProject(h.db.QueryRow(
		`UPDATE projects SET name = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
		 WHERE id = ? RETURNING `+projectColumns, name, id))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// delete handles DELETE /api/projects/{id}. CASCADE removes the project's
// tasks; the repository on disk is never touched (PROJ-03).
func (h *projectHandlers) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	res, err := h.db.Exec(`DELETE FROM projects WHERE id = ?`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// pathID parses the {id} path value; writes a 400 and returns ok=false on failure.
func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}

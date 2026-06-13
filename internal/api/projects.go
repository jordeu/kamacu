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

	"kangent/internal/github"
)

// Project is the JSON shape of a project row (RESEARCH.md Pattern 4).
// Description is NOT NULL ("" when unset). GithubRepo is a pointer so an
// unlinked project serializes as JSON null (distinct from "").
type Project struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	RepoPath    string  `json:"repo_path"`
	Description string  `json:"description"`
	GithubRepo  *string `json:"github_repo"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
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

const projectColumns = `id, name, repo_path, description, github_repo, created_at, updated_at`

func scanProject(row interface{ Scan(...any) error }) (Project, error) {
	var p Project
	var repo sql.NullString
	err := row.Scan(&p.ID, &p.Name, &p.RepoPath, &p.Description, &repo, &p.CreatedAt, &p.UpdatedAt)
	if repo.Valid {
		p.GithubRepo = &repo.String
	}
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

// update handles PATCH /api/projects/{id} — a PARTIAL update (D-13) of name,
// description, and github_repo. repo_path stays immutable in v1. Fields decode
// into pointers so an OMITTED key is left untouched while an explicit ""
// clears (description) or unlinks (github_repo). The lone hard error is a
// syntactically invalid github_repo (github.ValidateRepo); a gh-unverifiable
// but syntactically valid ref still saves (degrade-don't-break, GHSET-03).
func (h *projectHandlers) update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
		GithubRepo  *string `json:"github_repo"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Name == nil && req.Description == nil && req.GithubRepo == nil {
		writeError(w, http.StatusBadRequest, "nothing to update")
		return
	}

	var sets []string
	var args []any
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			writeError(w, http.StatusBadRequest, "name is required")
			return
		}
		sets = append(sets, "name = ?")
		args = append(args, name)
	}
	if req.Description != nil {
		if len(*req.Description) > 280 {
			writeError(w, http.StatusBadRequest, "Description is too long.")
			return
		}
		sets = append(sets, "description = ?")
		args = append(args, *req.Description)
	}
	if req.GithubRepo != nil {
		if strings.TrimSpace(*req.GithubRepo) == "" {
			// Explicit "" unlinks → store NULL.
			sets = append(sets, "github_repo = NULL")
		} else {
			canonical, _, err := github.ValidateRepo(r.Context(), *req.GithubRepo)
			if err != nil {
				// The ONLY blocking case: a syntactically invalid ref. The row
				// is left untouched. A soft-unverifiable ref returns no error
				// here and saves the syntactic owner/name.
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			sets = append(sets, "github_repo = ?")
			args = append(args, canonical)
		}
	}

	sets = append(sets, "updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')")
	args = append(args, id)
	query := `UPDATE projects SET ` + strings.Join(sets, ", ") + ` WHERE id = ? RETURNING ` + projectColumns
	p, err := scanProject(h.db.QueryRow(query, args...))
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

// githubOrigin handles GET /api/projects/{id}/github-origin — the project
// settings dialog's on-open origin prefill (D-08). git is shelled ONLY here,
// when the dialog opens, never on the project list. It reads the repo's
// `origin` remote and canonicalizes it to owner/name; detection is a
// convenience, so a missing/non-GitHub origin yields an empty suggestion and
// NEVER an error (200 either way). Only an unknown project id 404s.
func (h *projectHandlers) githubOrigin(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var repoPath string
	err := h.db.QueryRow(`SELECT repo_path FROM projects WHERE id = ?`, id).Scan(&repoPath)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// arg array only — never sh -c. Nonzero exit means no origin remote.
	out, runErr := exec.CommandContext(r.Context(), "git", "-C", repoPath, "remote", "get-url", "origin").Output()
	if runErr != nil {
		writeJSON(w, http.StatusOK, map[string]string{"suggestion": ""})
		return
	}
	suggestion, perr := github.ParseRepoRef(strings.TrimSpace(string(out)))
	if perr != nil {
		// Non-GitHub origin (e.g. GitLab) → empty suggestion, not an error.
		suggestion = ""
	}
	writeJSON(w, http.StatusOK, map[string]string{"suggestion": suggestion})
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

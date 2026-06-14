package api

import (
	"context"
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
	"kangent/internal/session"
	"kangent/internal/settings"
	"kangent/internal/tmux"
	"kangent/internal/worktree"
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
	// Managed is the v1.4 marker (migration 00008, D-06): true when Kangent
	// cloned and OWNS the directory under ~/.kangent/repos/ (gated-remove on
	// delete, pre-task fetch); false for user-pointed folder projects (never
	// touch their dir, D-09). SQLite stores it as INTEGER 0/1; scanProject maps
	// it to bool.
	Managed   bool   `json:"managed"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// projectHandlers carries the gated-delete dependencies (mirrors taskHandlers /
// worktreeHandlers): wt removes the managed clone's linked worktrees, mgr stops
// their sessions, tmuxClient kills detached tmux survivors — all needed by the
// gated managed-clone delete wired in plan 04. The folder-delete path (D-09)
// uses none of them.
type projectHandlers struct {
	db         *sql.DB
	wt         *worktree.Service
	mgr        *session.Manager
	tmuxClient tmux.Client
}

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

const projectColumns = `id, name, repo_path, description, github_repo, managed, created_at, updated_at`

func scanProject(row interface{ Scan(...any) error }) (Project, error) {
	var p Project
	var repo sql.NullString
	// SQLite returns managed as INTEGER 0/1; scan into an int and map to bool
	// to avoid any modernc bool-scan friction (14-RESEARCH.md §"Wire/scan
	// changes"). Column ORDER must match projectColumns (managed before the
	// timestamps).
	var managedInt int
	err := row.Scan(&p.ID, &p.Name, &p.RepoPath, &p.Description, &repo, &managedInt, &p.CreatedAt, &p.UpdatedAt)
	if repo.Valid {
		p.GithubRepo = &repo.String
	}
	p.Managed = managedInt != 0
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

// create handles POST /api/projects. Two creation paths share this endpoint:
//
//   - Folder path (the original): `{ "repo_path": "/abs/path" }` points Kangent
//     at a user-owned checkout. managed defaults to 0 — Kangent never touches
//     the dir on delete (D-09). UNCHANGED by v1.4.
//   - Repo-first path (v1.4, CKOUT-01): `{ "repo": "owner/name" }` gh-validates
//     the ref (RPROJ-05/D-03), `gh repo clone`s it into
//     ~/.kangent/repos/<owner>/<name> (D-02), and records it with managed=1 +
//     github_repo=canonical — but ONLY after the clone returns exit 0, so a
//     failed clone leaves no row and no dir (atomic, D-01/CKOUT-04). If the dest
//     already exists, it reattaches on origin-match (CKOUT-05/D-10).
//
// The branch is chosen by whether `repo` is non-empty.
func (h *projectHandlers) create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		RepoPath string `json:"repo_path"`
		Repo     string `json:"repo"` // owner/name OR a GitHub URL → repo-first path
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if strings.TrimSpace(req.Repo) != "" {
		h.createByRepo(w, r, req.Repo, req.Name)
		return
	}

	// --- Folder path (unchanged) ---
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

// reposBase is the hardcoded managed-clone root (D-02 / Claude's discretion: no
// repos_base setting for v1.4). Clones nest under it as <owner>/<name>.
const reposBase = "~/.kangent/repos/"

// createByRepo is the repo-first creation path (CKOUT-01). The ordering is the
// research §"Clone-then-Create Ordering" 8-step sequence and is load-bearing for
// atomicity: validate BEFORE any clone, clone BEFORE any row, row only after the
// clone returns exit 0.
func (h *projectHandlers) createByRepo(w http.ResponseWriter, r *http.Request, repoInput, nameInput string) {
	// 1. Parse/canonicalize the ref. The ONLY hard, host-independent reject.
	if _, err := github.ParseRepoRef(repoInput); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// 2. gh-validate (RPROJ-05/D-03). Mirror the update handler's degrade block:
	//    a syntactic error → 400; not-verified → 400 (msgRepoNotFound, or
	//    msgGHUnavailable when gh is absent). No clone, no row on any reject.
	canonical, verified, err := github.ValidateRepo(r.Context(), repoInput)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !verified {
		msg := msgRepoNotFound
		if !github.Available() {
			msg = msgGHUnavailable
		}
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	// 3. Compute the managed dest: ~/.kangent/repos/<owner>/<name>. canonical is
	//    "owner/name", so filepath.Join nests it correctly.
	base, err := settings.ExpandHome(reposBase)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	dest := filepath.Join(base, canonical)

	// 4. Dest already on disk → reattach-or-refuse (CKOUT-05/D-10). Never clone
	//    over or clobber an existing dir; on mismatch/non-git it 409s.
	reattached := false
	if _, statErr := os.Stat(dest); statErr == nil {
		if rerr := reattachManaged(r.Context(), dest, canonical); rerr != nil {
			writeError(w, http.StatusConflict, rerr.Error())
			return
		}
		reattached = true
	}

	// 5. Dedup pre-check on dest (mirror the folder branch): an existing row → 409.
	var exists int
	if derr := h.db.QueryRow(`SELECT 1 FROM projects WHERE repo_path = ?`, dest).Scan(&exists); derr == nil {
		writeError(w, http.StatusConflict, "this repository is already added")
		return
	} else if !errors.Is(derr, sql.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, derr.Error())
		return
	}

	// 6. Clone — only when not reattaching. On failure: belt-and-braces remove
	//    (Clone already removed it), surface ONE inline error, NO row (atomicity).
	if !reattached {
		if cerr := github.Clone(r.Context(), canonical, dest); cerr != nil {
			_ = os.RemoveAll(dest)
			writeError(w, http.StatusInternalServerError, cerr.Error())
			return
		}
	}

	// 7. INSERT only now (managed=1, github_repo=canonical — Phase 14 sets it so
	//    Phase 15's form can rely on it; research Open Question 1).
	name := strings.TrimSpace(nameInput)
	if name == "" {
		name = filepath.Base(dest) // = repo name
	}
	p, err := scanProject(h.db.QueryRow(
		`INSERT INTO projects (name, repo_path, github_repo, managed) VALUES (?, ?, ?, 1) RETURNING `+projectColumns,
		name, dest, canonical))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// 8. Created.
	writeJSON(w, http.StatusCreated, p)
}

// reattachManaged decides whether an already-existing dest can be reused as the
// managed clone for canonical owner/name (CKOUT-05/D-10). It NEVER removes or
// resets the dir (D-11): returning nil means "reuse dest"; a non-nil error (with
// a user-facing message) means "refuse, do not clobber". It is reached only when
// os.Stat(dest) showed the directory already exists.
func reattachManaged(ctx context.Context, dest, canonical string) error {
	// 1. dest must be a git repo (the validateRepoPath check). A stray/user dir
	//    is never clobbered.
	if err := exec.CommandContext(ctx, "git", "-C", dest, "rev-parse", "--git-dir").Run(); err != nil {
		return fmt.Errorf("a directory already exists at %s but is not a git repository", dest)
	}
	// 2. Read origin (the githubOrigin pattern). No origin → can't confirm
	//    ownership, refuse.
	out, err := exec.CommandContext(ctx, "git", "-C", dest, "remote", "get-url", "origin").Output()
	if err != nil {
		return fmt.Errorf("a directory already exists at %s with no origin remote", dest)
	}
	// 3. Canonicalize origin (ParseRepoRef handles ssh + https) and compare
	//    case-insensitively (gh canonicalizes casing). A parse error or mismatch
	//    → refuse without clobbering.
	got, perr := github.ParseRepoRef(strings.TrimSpace(string(out)))
	if perr != nil || !strings.EqualFold(got, canonical) {
		return fmt.Errorf("a different repository is already checked out at %s", dest)
	}
	// 4. Match → reuse. No re-clone, no reset, no fetch (D-11: plan 03's per-task
	//    fetch already provides freshness; a blocking fetch here is forbidden).
	return nil
}

// Hard-block messages for a non-empty github_repo that gh cannot verify
// (GHPRJ-03). The not-found vs no-gh copy is chosen by github.Available().
const (
	msgRepoNotFound  = "Repository not found on GitHub — check the name or your access."
	msgGHUnavailable = "Couldn't verify the repository — the gh CLI isn't available."
)

// update handles PATCH /api/projects/{id} — a PARTIAL update (D-13) of name,
// description, and github_repo. repo_path stays immutable in v1. Fields decode
// into pointers so an OMITTED key is left untouched while an explicit ""
// clears (description) or unlinks (github_repo). A non-empty github_repo is now
// MANDATORY-verified (GHPRJ-03): a syntactically invalid ref OR one that gh
// cannot confirm is rejected (400) and the row is left untouched — only a
// gh-verified, canonicalized ref is persisted. (This reverses the prior
// soft-save: an unverifiable ref no longer saves.)
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
			// Explicit "" unlinks → store NULL (no validation).
			sets = append(sets, "github_repo = NULL")
		} else {
			canonical, verified, err := github.ValidateRepo(r.Context(), *req.GithubRepo)
			if err != nil {
				// Syntactically invalid ref → reject with the canonical copy;
				// the row is left untouched.
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			if !verified {
				// MANDATORY hard block (GHPRJ-03): gh could not confirm the
				// repo. Reject WITHOUT touching the row — do not append to the
				// sets/args, so the stored link (if any) is preserved.
				msg := msgRepoNotFound
				if !github.Available() {
					msg = msgGHUnavailable
				}
				writeError(w, http.StatusBadRequest, msg)
				return
			}
			// Verified → persist the gh-canonicalized owner/name.
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

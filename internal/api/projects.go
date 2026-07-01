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

	"kamacu/internal/github"
	"kamacu/internal/session"
	"kamacu/internal/settings"
	"kamacu/internal/tmux"
	"kamacu/internal/worktree"
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
	// Managed is the v1.4 marker (migration 00008, D-06): true when Kamacu
	// cloned and OWNS the directory under ~/.kamacu/repos/ (gated-remove on
	// delete, pre-task fetch); false for user-pointed folder projects (never
	// touch their dir, D-09). SQLite stores it as INTEGER 0/1; scanProject maps
	// it to bool.
	Managed bool `json:"managed"`
	// IconLetters + IconColor are the v1.7 avatar fields (migration 00009, D-12):
	// both plain non-null strings (never null on the wire). New projects get them
	// from deriveLetters(name)+pickColor() at create; pre-existing rows are filled
	// by the startup backfill. Column ORDER here MUST match projectColumns and the
	// scanProject Scan order (they sit between managed and the timestamps).
	IconLetters string `json:"icon_letters"`
	IconColor   string `json:"icon_color"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
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

const projectColumns = `id, name, repo_path, description, github_repo, managed, icon_letters, icon_color, created_at, updated_at`

func scanProject(row interface{ Scan(...any) error }) (Project, error) {
	var p Project
	var repo sql.NullString
	// SQLite returns managed as INTEGER 0/1; scan into an int and map to bool
	// to avoid any modernc bool-scan friction (14-RESEARCH.md §"Wire/scan
	// changes"). Column ORDER must match projectColumns (managed before the
	// timestamps).
	var managedInt int
	// icon_letters/icon_color are TEXT NOT NULL (migration 00009) — scan straight
	// into string (no sql.NullString needed, D-12: never null). Order MUST match
	// projectColumns: managed, icon_letters, icon_color, then the timestamps.
	err := row.Scan(&p.ID, &p.Name, &p.RepoPath, &p.Description, &repo, &managedInt, &p.IconLetters, &p.IconColor, &p.CreatedAt, &p.UpdatedAt)
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
//   - Folder path (the original): `{ "repo_path": "/abs/path" }` points Kamacu
//     at a user-owned checkout. managed defaults to 0 — Kamacu never touches
//     the dir on delete (D-09). UNCHANGED by v1.4.
//   - Repo-first path (v1.4, CKOUT-01): `{ "repo": "owner/name" }` gh-validates
//     the ref (RPROJ-05/D-03), `gh repo clone`s it into
//     ~/.kamacu/repos/<owner>/<name> (D-02), and records it with managed=1 +
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
		`INSERT INTO projects (name, repo_path, icon_letters, icon_color) VALUES (?, ?, ?, ?) RETURNING `+projectColumns,
		name, abs, deriveLetters(name), pickColor()))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

// reposBase is the hardcoded managed-clone root (D-02 / Claude's discretion: no
// repos_base setting for v1.4). Clones nest under it as <owner>/<name>.
const reposBase = "~/.kamacu/repos/"

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

	// 3. Compute the managed dest: ~/.kamacu/repos/<owner>/<name>. canonical is
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
	// Best-effort GitHub description capture (RPROJ-02 / D-05): persisted
	// server-side so it surfaces (editable) in Project settings later — NOT a
	// dialog field. Degrade-don't-break: a missing/empty/failed read yields ""
	// (the projects.description default) and NEVER blocks create. Applies to
	// both the clone and reattach branches (this is their single INSERT).
	desc := github.RepoDescription(r.Context(), canonical)
	p, err := scanProject(h.db.QueryRow(
		`INSERT INTO projects (name, repo_path, github_repo, managed, description, icon_letters, icon_color) VALUES (?, ?, ?, 1, ?, ?, ?) RETURNING `+projectColumns,
		name, dest, canonical, desc, deriveLetters(name), pickColor()))
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
		IconLetters *string `json:"icon_letters"`
		IconColor   *string `json:"icon_color"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Name == nil && req.Description == nil && req.GithubRepo == nil &&
		req.IconLetters == nil && req.IconColor == nil {
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
	if req.IconLetters != nil {
		// D-10: server is the enforcer. Validate (trim/upper/≤2/≥1); an
		// empty/whitespace-only monogram is the lone hard error — reject 400 and
		// leave the row untouched (mirror the github_repo reject idiom above).
		letters, verr := validateIconLetters(*req.IconLetters)
		if verr != nil {
			writeError(w, http.StatusBadRequest, verr.Error())
			return
		}
		sets = append(sets, "icon_letters = ?")
		args = append(args, letters)
	}
	if req.IconColor != nil {
		// D-11: icon_color MUST be a curated-palette member. Off-palette
		// (free-form hex, color name, garbage) is rejected 400 with the row
		// untouched; on a hit the canonical lowercase palette hex is stored.
		color, verr := validateIconColor(*req.IconColor)
		if verr != nil {
			writeError(w, http.StatusBadRequest, verr.Error())
			return
		}
		sets = append(sets, "icon_color = ?")
		args = append(args, color)
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

// deleteBlocker is one reason a managed-project delete is refused (CKOUT-03 /
// D-07). The 409 body is { "error": ..., "reasons": [ {kind, target}, ... ] }
// — a STRUCTURED list a Phase 15 cleanup dialog can enumerate (research Open
// Question 3, recommended). Kind is a stable machine token; Target is a
// human-readable subject ("task #N" or "the managed checkout").
type deleteBlocker struct {
	Kind   string `json:"kind"`   // "uncommitted" | "unpushed" | "stash" | "sessions"
	Target string `json:"target"` // "task #<id>" | "the managed checkout"
}

// managedTaskWorktree is a project's task (or PR-review) worktree to gate/remove.
type managedTaskWorktree struct {
	taskID int64
	path   string
}

// delete handles DELETE /api/projects/{id}.
//
//   - Folder project (managed=0): today's behavior EXACTLY — DELETE FROM
//     projects (CASCADE removes tasks); the repository on disk is NEVER touched
//     (PROJ-03 / D-09). No git, no disk ops.
//   - Managed project (managed=1, v1.4 CKOUT-03): a two-pass, all-or-nothing
//     gated removal. The GATE phase computes the four gates
//     (dirty/unpushed/stash/sessions) over EVERY task worktree AND the clone
//     root, mutating nothing; if ANY gate trips it returns 409 with the
//     aggregated reasons and removes NOTHING (D-07). On all-clear the REMOVE
//     phase tears each linked worktree down first, then os.RemoveAll's the clone
//     dir, then deletes the rows (D-08 ordering). The clone root is removed with
//     os.RemoveAll — never `git worktree remove`, which refuses the main
//     worktree (Pitfall 2).
func (h *projectHandlers) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	// Load the marker + repo root. The marker is the SINGLE source of truth for
	// "Kamacu owns this dir" — never derive it from the path (data-loss hazard).
	var managed int
	var clone string
	err := h.db.QueryRow(`SELECT managed, repo_path FROM projects WHERE id = ?`, id).Scan(&managed, &clone)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// --- Folder path (UNCHANGED, D-09): never touch the directory. ---
	if managed == 0 {
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
		return
	}

	// --- Managed path (CKOUT-03): gated, all-or-nothing removal. ---
	h.deleteManaged(w, r, id, clone)
}

// deleteManaged runs the two-pass gated removal of a managed clone (CKOUT-03 /
// D-07 / D-08). It is reached only after delete() confirmed managed=1.
func (h *projectHandlers) deleteManaged(w http.ResponseWriter, r *http.Request, id int64, clone string) {
	ctx := r.Context()

	// Resolve the clone's default branch ONCE (origin/<default>); it is the
	// unpushed gate base for the clone AND every task worktree (they branch off
	// it). A read failure is CONSERVATIVE — treat it as a blocker rather than
	// skipping the unpushed gate (research §"Gated Delete", D-08 safety net).
	defBranch, dberr := h.wt.DefaultBranch(ctx, clone)
	unpushedBase := "origin/" + defBranch

	// Enumerate the project's task/PR-review worktrees. PR-review worktrees
	// (source='github_pr') branch off the same clone and MUST be gated/removed
	// too — no source filter.
	worktrees, err := h.projectWorktrees(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// === GATE PHASE (compute fresh, mutate NOTHING; two-pass per Pitfall 8). ===
	var blockers []deleteBlocker

	// If origin/<default> could not be resolved we cannot run the unpushed gate
	// safely → conservative blocker; remove nothing.
	if dberr != nil {
		blockers = append(blockers, deleteBlocker{
			Kind: "unpushed", Target: "the managed checkout",
		})
	}

	for _, wt := range worktrees {
		target := fmt.Sprintf("task #%d", wt.taskID)
		// A manually-deleted worktree dir counts clean (it will simply be
		// pruned in the remove phase) — only gate dirs that still exist.
		if _, statErr := os.Stat(wt.path); statErr != nil {
			// Still gate sessions for the task (a live session can exist with a
			// vanished tree); skip the git gates for the missing dir.
			if h.cleanupSessionCount(ctx, wt.taskID) > 0 {
				blockers = append(blockers, deleteBlocker{Kind: "sessions", Target: target})
			}
			continue
		}
		if dirty, derr := h.wt.DirtyCount(ctx, wt.path); derr != nil {
			writeError(w, http.StatusInternalServerError, derr.Error())
			return
		} else if dirty > 0 {
			blockers = append(blockers, deleteBlocker{Kind: "uncommitted", Target: target})
		}
		// Unpushed gate: origin/<default>..HEAD, NO fetch (network-free,
		// conservative — research Open Question 2). dberr already blocked above
		// if the base is unknown; only run rev-list when we have a base.
		if dberr == nil {
			if unpushed, uerr := h.wt.UnpushedCount(ctx, wt.path, unpushedBase); uerr != nil {
				writeError(w, http.StatusInternalServerError, uerr.Error())
				return
			} else if unpushed > 0 {
				blockers = append(blockers, deleteBlocker{Kind: "unpushed", Target: target})
			}
		}
		if stash, serr := h.wt.StashCount(ctx, wt.path); serr != nil {
			writeError(w, http.StatusInternalServerError, serr.Error())
			return
		} else if stash > 0 {
			blockers = append(blockers, deleteBlocker{Kind: "stash", Target: target})
		}
		if h.cleanupSessionCount(ctx, wt.taskID) > 0 {
			blockers = append(blockers, deleteBlocker{Kind: "sessions", Target: target})
		}
	}

	// Clone-root gates (D-08 safety net): the clone IS a worktree for the gates.
	// A missing clone dir is unusual for a managed project; gate it only when
	// present (RemoveAll would be a no-op anyway).
	if _, statErr := os.Stat(clone); statErr == nil {
		if dirty, derr := h.wt.DirtyCount(ctx, clone); derr != nil {
			writeError(w, http.StatusInternalServerError, derr.Error())
			return
		} else if dirty > 0 {
			blockers = append(blockers, deleteBlocker{Kind: "uncommitted", Target: "the managed checkout"})
		}
		if dberr == nil {
			if unpushed, uerr := h.wt.UnpushedCount(ctx, clone, unpushedBase); uerr != nil {
				writeError(w, http.StatusInternalServerError, uerr.Error())
				return
			} else if unpushed > 0 {
				blockers = append(blockers, deleteBlocker{Kind: "unpushed", Target: "the managed checkout"})
			}
		}
		if stash, serr := h.wt.StashCount(ctx, clone); serr != nil {
			writeError(w, http.StatusInternalServerError, serr.Error())
			return
		} else if stash > 0 {
			blockers = append(blockers, deleteBlocker{Kind: "stash", Target: "the managed checkout"})
		}
	}

	// ANY blocker → 409 with the structured reason list. REMOVE NOTHING (D-07).
	if len(blockers) > 0 {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":   "the project can't be deleted yet",
			"reasons": blockers,
		})
		return
	}

	// === REMOVE PHASE (all clear). Order is load-bearing (Pitfall 1). ===
	h.removeManaged(w, ctx, id, clone, worktrees)
}

// projectWorktrees enumerates a project's task/PR-review worktrees (those with
// a non-NULL worktree_path). No source filter: PR-review worktrees branch off
// the same managed clone and are gated/removed alongside task worktrees.
func (h *projectHandlers) projectWorktrees(ctx context.Context, projectID int64) ([]managedTaskWorktree, error) {
	rows, err := h.db.QueryContext(ctx,
		`SELECT id, worktree_path FROM tasks WHERE project_id = ? AND worktree_path IS NOT NULL`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []managedTaskWorktree
	for rows.Next() {
		var w managedTaskWorktree
		if err := rows.Scan(&w.taskID, &w.path); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// removeManaged is the REMOVE phase of the managed delete — reached ONLY after
// every gate passed (deleteManaged). The ordering is load-bearing (research
// Pitfall 1): remove each LINKED worktree first (wt.Remove via the shared
// CleanupWorktreeGated), THEN os.RemoveAll the clone dir, THEN delete the rows
// FK-ordered. The clone root is NEVER passed to wt.Remove — git refuses the main
// worktree (Pitfall 2); it is removed with os.RemoveAll.
func (h *projectHandlers) removeManaged(w http.ResponseWriter, ctx context.Context, id int64, clone string, worktrees []managedTaskWorktree) {
	for _, wt := range worktrees {
		// stopSessions=true: the gate phase already verified idleness; any
		// session that appeared since is intentionally reaped during teardown
		// (the gate is the user-facing refuse; teardown stops what remains).
		count := h.cleanupSessionCount(ctx, wt.taskID)
		live := h.liveTmuxNames(ctx, wt.taskID)
		removed, reason, cerr := CleanupWorktreeGated(
			ctx, h.db, h.wt, h.mgr, h.tmuxClient, live,
			wt.taskID, clone, wt.path, count, true /*stopSessions*/, false /*force*/)
		if cerr != nil {
			writeError(w, http.StatusInternalServerError, cerr.Error())
			return
		}
		if !removed {
			// A race re-tripped a gate (rare/defensive). Refuse with the reason.
			writeJSON(w, http.StatusConflict, map[string]any{
				"error":   "the project can't be deleted yet",
				"reasons": []deleteBlocker{{Kind: gateReasonKind(reason), Target: fmt.Sprintf("task #%d", wt.taskID)}},
			})
			return
		}
	}

	// All linked worktrees gone → remove the clone root (main worktree + .git).
	// os.RemoveAll ONLY (git worktree remove refuses the main worktree).
	if rerr := os.RemoveAll(clone); rerr != nil {
		writeError(w, http.StatusInternalServerError, rerr.Error())
		return
	}

	// Delete rows FK-ordered (FKs ON, no CASCADE on tmux_sessions): clear
	// tmux_sessions of the project's tasks first, then the project (CASCADE
	// removes the task rows). Mirror reaper.go / tasks.go delete ordering.
	if _, derr := h.db.ExecContext(ctx,
		`DELETE FROM tmux_sessions WHERE task_id IN (SELECT id FROM tasks WHERE project_id = ?)`, id); derr != nil {
		writeError(w, http.StatusInternalServerError, derr.Error())
		return
	}
	if _, derr := h.db.ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, id); derr != nil {
		writeError(w, http.StatusInternalServerError, derr.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// gateReasonKind maps the shared CleanupWorktreeGated reason strings to a
// deleteBlocker kind (defensive remove-phase race path only).
func gateReasonKind(reason string) string {
	switch reason {
	case "sessions running":
		return "sessions"
	case "worktree has uncommitted changes":
		return "uncommitted"
	default:
		return "uncommitted"
	}
}

// --- session-count helpers (replicated from worktreeHandlers; ~30 lines, keeps
// the package boundary clean per the plan's interface note). ---

// runningSessions counts the task's in-memory running sessions.
func (h *projectHandlers) runningSessions(taskID int64) int {
	n := 0
	for _, info := range h.mgr.ListByTask(taskID) {
		if info.Status == session.StatusRunning {
			n++
		}
	}
	return n
}

// liveTmuxNames returns the task's tmux session names alive on the dedicated
// socket (probed via has-session; an inconclusive probe is never read as
// alive). Mirrors worktreeHandlers.liveTmuxNames.
func (h *projectHandlers) liveTmuxNames(ctx context.Context, taskID int64) []string {
	rows, err := h.db.QueryContext(ctx, `SELECT name FROM tmux_sessions WHERE task_id = ?`, taskID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			continue
		}
		names = append(names, name)
	}
	live := make([]string, 0, len(names))
	for _, name := range names {
		if alive, herr := h.tmuxClient.HasSession(ctx, name); alive && herr == nil {
			live = append(live, name)
		}
	}
	return live
}

// cleanupSessionCount folds live detached tmux survivors into the in-memory
// running-session count (one honest number, no double-count). Mirrors
// worktreeHandlers.cleanupSessionCount.
func (h *projectHandlers) cleanupSessionCount(ctx context.Context, taskID int64) int {
	count := h.runningSessions(taskID)
	for _, name := range h.liveTmuxNames(ctx, taskID) {
		if !h.mgr.HasLiveTmux(name) {
			count++
		}
	}
	return count
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

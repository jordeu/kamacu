package api

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"kamacu/internal/session"
	"kamacu/internal/tmux"
	"kamacu/internal/worktree"
)

// enumTimeout bounds the whole worktree enumeration + per-row flag computation
// (Pitfall 5: N projects × N worktrees × 3 git calls). Matches the 60s budget
// CleanupWorktreeGated uses so a hung git never wedges the GET indefinitely.
const enumTimeout = 60 * time.Second

// PRStateGetter is the slice of the github service the panel's PR-eligibility
// and PR-state-display logic needs. Defined LOCALLY (mirrors the reaper's own
// local PRStateGetter) so a test injects a spy and the real *github.Service
// satisfies it via PRState → "OPEN"|"CLOSED"|"MERGED". A gh failure is
// warn-and-skip (degrade-don't-break), never a crash or a 500.
type PRStateGetter interface {
	PRState(ctx context.Context, repo string, n int) (string, error)
}

// WorktreeCleanupRoutes registers the Settings worktree-cleanup panel endpoints
// (WTREE-01..04). The panel is the THIRD caller of CleanupWorktreeGated
// (handler → reaper → panel): it reuses the one audited removal path and adds
// only cross-project enumeration + classification + the eligibility predicate.
// pr is the shared *github.Service (same instance the reaper + PR routes use)
// for PR merged/closed eligibility + display, degrading cleanly when gh is
// absent.
func WorktreeCleanupRoutes(mux *http.ServeMux, db *sql.DB, wt *worktree.Service, mgr *session.Manager, tmuxClient tmux.Client, pr PRStateGetter) {
	h := &cleanupPanelHandlers{db: db, wt: wt, mgr: mgr, tmuxClient: tmuxClient, pr: pr}
	mux.HandleFunc("GET /api/worktrees", h.list)
}

type cleanupPanelHandlers struct {
	db         *sql.DB
	wt         *worktree.Service
	mgr        *session.Manager
	tmuxClient tmux.Client
	pr         PRStateGetter
}

// --- session-count helpers (replicated VERBATIM from worktreeHandlers; ~30
// lines, the deliberate package-boundary replication per projects.go:741). ---

// runningSessions counts the task's in-memory running sessions.
func (h *cleanupPanelHandlers) runningSessions(taskID int64) int {
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
func (h *cleanupPanelHandlers) liveTmuxNames(ctx context.Context, taskID int64) []string {
	rows, err := h.db.QueryContext(ctx, `SELECT name FROM tmux_sessions WHERE task_id = ?`, taskID)
	if err != nil {
		slog.Warn("listing tmux_sessions for task", "task", taskID, "error", err)
		return nil
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			slog.Warn("scanning tmux_sessions row", "task", taskID, "error", err)
			continue
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		slog.Warn("iterating tmux_sessions rows", "task", taskID, "error", err)
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
func (h *cleanupPanelHandlers) cleanupSessionCount(ctx context.Context, taskID int64) int {
	count := h.runningSessions(taskID)
	for _, name := range h.liveTmuxNames(ctx, taskID) {
		if !h.mgr.HasLiveTmux(name) {
			count++
		}
	}
	return count
}

// --- enumeration + classification (the union scan, D-06/D-07) ---

// projectMeta carries the fields the panel groups + labels rows by.
type projectMeta struct {
	id       int64
	name     string
	repoPath string
	letters  string
	color    string
}

// dbTask is the DB side of the union: one task row with a non-NULL
// worktree_path. cleanPath is filepath.Clean(worktree_path) for map keying.
type dbTask struct {
	id        int64
	projectID int64
	cleanPath string
	rawPath   string
	title     string
	status    string
	source    string
	prNumber  int
	prBaseRef string
	branch    string
}

// enumWorktree is one classified worktree in a project's group, BEFORE flag
// computation. It pairs the on-disk git entry (nil for a stale pointer whose
// dir is gone) with its owning task (nil for an orphan).
type enumWorktree struct {
	classification string // "referenced" | "orphan" | "stale"
	path           string // the worktree path (git Path, or the stale task path)
	entry          *worktree.Entry
	task           *dbTask
}

// enumProject is a project plus its classified worktrees (main excluded).
type enumProject struct {
	meta      projectMeta
	worktrees []enumWorktree
}

// enumerate performs the cross-project union scan (D-06): for every project it
// runs git worktree list (excluding the main worktree, Pitfall 2) and unions the
// result with the task rows whose worktree_path is non-NULL, classifying each as
// referenced / orphan / stale (D-07). It is the shared spine both list() and
// cleanEligible() call so the two never drift. A single project's git-list
// failure is logged and skipped (its stale task rows still surface), never a
// whole-GET 500.
func (h *cleanupPanelHandlers) enumerate(ctx context.Context) ([]enumProject, error) {
	projects, err := h.loadProjects(ctx)
	if err != nil {
		return nil, err
	}
	tasksByProject, err := h.loadDBTasks(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]enumProject, 0, len(projects))
	for _, p := range projects {
		grp := enumProject{meta: p}

		// git side: every NON-main, non-bare linked worktree, keyed by clean path.
		gitByPath := map[string]*worktree.Entry{}
		entries, lerr := h.wt.List(ctx, p.repoPath)
		if lerr != nil {
			// A repo we can't list (e.g. moved/broken) — log and continue with
			// only its DB side (stale pointers still surface). Never fail the GET.
			slog.Warn("worktree list failed for project", "project", p.id, "repo", p.repoPath, "error", lerr)
		}
		mainClean := filepath.Clean(p.repoPath)
		for i := range entries {
			e := entries[i]
			if e.Bare {
				continue
			}
			clean := filepath.Clean(e.Path)
			if clean == mainClean {
				continue // main worktree — never a removable row (Pitfall 2)
			}
			gitByPath[clean] = &entries[i]
		}

		dbTasks := tasksByProject[p.id]
		dbByPath := map[string]*dbTask{}
		for i := range dbTasks {
			dbByPath[dbTasks[i].cleanPath] = &dbTasks[i]
		}

		// Referenced + orphan: iterate the git list.
		for clean, entry := range gitByPath {
			if task := dbByPath[clean]; task != nil {
				grp.worktrees = append(grp.worktrees, enumWorktree{
					classification: "referenced", path: entry.Path, entry: entry, task: task,
				})
			} else {
				grp.worktrees = append(grp.worktrees, enumWorktree{
					classification: "orphan", path: entry.Path, entry: entry,
				})
			}
		}

		// Stale pointers: DB task rows whose path is NOT in git's list.
		for i := range dbTasks {
			t := &dbTasks[i]
			if _, inGit := gitByPath[t.cleanPath]; !inGit {
				grp.worktrees = append(grp.worktrees, enumWorktree{
					classification: "stale", path: t.rawPath, task: t,
				})
			}
		}

		out = append(out, grp)
	}
	return out, nil
}

// loadProjects reads every project's grouping metadata (D-06 scan roots).
func (h *cleanupPanelHandlers) loadProjects(ctx context.Context) ([]projectMeta, error) {
	rows, err := h.db.QueryContext(ctx,
		`SELECT id, name, repo_path, icon_letters, icon_color FROM projects ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []projectMeta
	for rows.Next() {
		var p projectMeta
		if err := rows.Scan(&p.id, &p.name, &p.repoPath, &p.letters, &p.color); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// loadDBTasks reads every task with a non-NULL worktree_path, grouped by project
// (the DB side of the union). Mirrors projectWorktrees' shape, widened to the
// classification/flag/association columns the panel needs.
func (h *cleanupPanelHandlers) loadDBTasks(ctx context.Context) (map[int64][]dbTask, error) {
	rows, err := h.db.QueryContext(ctx,
		`SELECT id, project_id, title, status, worktree_path, branch, source, pr_number, pr_base_ref
		 FROM tasks WHERE worktree_path IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64][]dbTask{}
	for rows.Next() {
		var (
			t         dbTask
			branch    sql.NullString
			source    sql.NullString
			prNumber  sql.NullInt64
			prBaseRef sql.NullString
			wtPath    string
		)
		if err := rows.Scan(&t.id, &t.projectID, &t.title, &t.status, &wtPath,
			&branch, &source, &prNumber, &prBaseRef); err != nil {
			return nil, err
		}
		t.rawPath = wtPath
		t.cleanPath = filepath.Clean(wtPath)
		t.branch = branch.String
		t.source = source.String
		t.prNumber = int(prNumber.Int64)
		t.prBaseRef = prBaseRef.String
		out[t.projectID] = append(out[t.projectID], t)
	}
	return out, rows.Err()
}

// --- HTTP: GET /api/worktrees (the annotated union scan, WTREE-01/04, D-07) ---

// worktreeRow is the JSON contract the frontend (23-04) consumes verbatim.
// pointers are the nullable fields: association/pr_state/unpushed/blocked_path.
type worktreeRow struct {
	Repo           string  `json:"repo"`
	Path           string  `json:"path"`
	TaskID         int64   `json:"task_id"` // 0 for an orphan
	Classification string  `json:"classification"`
	Association    *string `json:"association"`
	PRState        *string `json:"pr_state"`
	Branch         string  `json:"branch"`
	Dirty          int     `json:"dirty"`
	Unpushed       *int    `json:"unpushed"` // null when the base can't resolve
	Stash          int     `json:"stash"`
	Blocked        bool    `json:"blocked"`
	BlockedPath    *string `json:"blocked_path"`
	Sessions       int     `json:"sessions"`
}

type projectGroup struct {
	ProjectID   int64         `json:"project_id"`
	ProjectName string        `json:"project_name"`
	IconLetters string        `json:"icon_letters"`
	IconColor   string        `json:"icon_color"`
	Worktrees   []worktreeRow `json:"worktrees"`
}

type listResponse struct {
	Projects []projectGroup `json:"projects"`
	Counts   struct {
		Total    int `json:"total"`
		Orphaned int `json:"orphaned"`
	} `json:"counts"`
}

func (h *cleanupPanelHandlers) list(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), enumTimeout)
	defer cancel()

	groups, err := h.enumerate(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var resp listResponse
	resp.Projects = make([]projectGroup, 0, len(groups))
	for _, g := range groups {
		pg := projectGroup{
			ProjectID:   g.meta.id,
			ProjectName: g.meta.name,
			IconLetters: g.meta.letters,
			IconColor:   g.meta.color,
			Worktrees:   make([]worktreeRow, 0, len(g.worktrees)),
		}
		for _, ew := range g.worktrees {
			row := h.buildRow(ctx, g.meta, ew)
			pg.Worktrees = append(pg.Worktrees, row)
			resp.Counts.Total++
			if row.Classification == "orphan" {
				resp.Counts.Orphaned++
			}
		}
		resp.Projects = append(resp.Projects, pg)
	}
	writeJSON(w, http.StatusOK, resp)
}

// buildRow annotates one classified worktree with the display + flag fields the
// row carries. STALE rows compute NO git flags (the dir is gone). REFERENCED /
// ORPHAN rows that exist on disk get dirty/unpushed/stash + session counts.
// blocked is always false for a list GET (it is only known after a remove
// attempt returns the blocked outcome — plan 23-03 renders the banner from the
// remove response), but the field is emitted so the type stays stable.
func (h *cleanupPanelHandlers) buildRow(ctx context.Context, p projectMeta, ew enumWorktree) worktreeRow {
	row := worktreeRow{
		Repo:           p.repoPath,
		Path:           ew.path,
		Classification: ew.classification,
	}

	if ew.task != nil {
		row.TaskID = ew.task.id
		row.Branch = ew.task.branch
		row.Association = h.associationFor(ew.task)
	}
	// Orphan/stale/referenced branch fallback: for an orphan the git entry
	// carries the branch ref (refs/heads/<name>); surface its short name.
	if row.Branch == "" && ew.entry != nil && ew.entry.Branch != "" {
		row.Branch = strings.TrimPrefix(ew.entry.Branch, "refs/heads/")
	}

	// STALE: no on-disk dir — no flag computation (dirty=0, unpushed=null, etc.).
	if ew.classification == "stale" {
		return row
	}

	// Only compute flags when the dir exists on disk (a referenced task whose
	// dir vanished but still shows in git as prunable, or a moved dir, must not
	// wedge the whole GET on a git error).
	if _, statErr := os.Stat(ew.path); statErr != nil {
		return row
	}

	if n, derr := h.wt.DirtyCount(ctx, ew.path); derr == nil {
		row.Dirty = n
	} else {
		slog.Warn("dirty count failed", "path", ew.path, "error", derr)
	}
	if n, serr := h.wt.StashCount(ctx, ew.path); serr == nil {
		row.Stash = n
	} else {
		slog.Warn("stash count failed", "path", ew.path, "error", serr)
	}
	if ew.task != nil {
		row.Sessions = h.cleanupSessionCount(ctx, ew.task.id)
	}

	// Unpushed: resolve the base NETWORK-FREE per row (Pattern 3); a resolution
	// or count failure ⇒ leave Unpushed nil (the client renders "Unpushed?").
	if base, ok := h.unpushedBase(ctx, p.repoPath, ew); ok {
		if n, uerr := h.wt.UnpushedCount(ctx, ew.path, base); uerr == nil {
			row.Unpushed = &n
		} else {
			slog.Warn("unpushed count failed", "path", ew.path, "base", base, "error", uerr)
		}
	}

	// PR-state display suffix (github_pr referenced rows only), degrade-don't-break.
	if ew.task != nil && ew.task.source == "github_pr" && ew.task.prNumber > 0 && h.pr != nil {
		if state, perr := h.pr.PRState(ctx, p.repoPath, ew.task.prNumber); perr == nil && state != "" {
			// DISPLAY-ONLY lowercasing (contract with 23-04: "open"|"closed"|
			// "merged"). The D-04 eligibility comparison keeps the RAW uppercase.
			lower := strings.ToLower(state)
			row.PRState = &lower
		}
	}

	return row
}

// associationFor renders the row's human association label (23-04 contract):
// "Task: <title>" for a manual task, "PR #<n>" for a github_pr task, nil for an
// orphan (handled by the nil task in buildRow).
func (h *cleanupPanelHandlers) associationFor(t *dbTask) *string {
	if t == nil {
		return nil
	}
	var s string
	if t.source == "github_pr" && t.prNumber > 0 {
		s = "PR #" + strconv.Itoa(t.prNumber)
	} else {
		s = "Task: " + t.title
	}
	return &s
}

// unpushedBase resolves the NETWORK-FREE commit-ish the unpushed count is
// measured against for a REFERENCED/ORPHAN row (Pattern 3). Returns ok=false
// when no base can be resolved (⇒ unpushed:null / conservatively ineligible):
//   - referenced manual task → ResolveBase(repo) (the default-branch chain).
//   - referenced github_pr task with pr_base_ref → origin/<pr_base_ref>,
//     verified to resolve; else fall back to ResolveBase.
//   - orphan with a checked-out branch → ResolveBase(repo); else no base.
func (h *cleanupPanelHandlers) unpushedBase(ctx context.Context, repo string, ew enumWorktree) (string, bool) {
	if ew.task != nil && ew.task.source == "github_pr" && strings.TrimSpace(ew.task.prBaseRef) != "" {
		candidate := "origin/" + strings.TrimSpace(ew.task.prBaseRef)
		if h.refResolves(ctx, ew.path, candidate) {
			return candidate, true
		}
		// origin/<base> absent (network-free read) — fall through to ResolveBase.
	}

	// Orphan with no checked-out branch: nothing meaningful to diff against.
	if ew.task == nil && ew.entry != nil && ew.entry.Detached {
		return "", false
	}

	base, err := h.wt.ResolveBase(ctx, repo)
	if err != nil {
		return "", false
	}
	return base, true
}

// refResolves reports whether commit-ish resolves in the worktree at wt, using a
// network-free rev-parse --verify (mirrors resolvePRBase's local-read posture).
func (h *cleanupPanelHandlers) refResolves(ctx context.Context, wt, ref string) bool {
	return h.wt.RefResolves(ctx, wt, ref)
}

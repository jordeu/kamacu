package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
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
	mux.HandleFunc("POST /api/worktrees/remove", h.remove)
	mux.HandleFunc("POST /api/worktrees/clean-eligible", h.cleanEligible)
	mux.HandleFunc("POST /api/worktrees/clear-pointer", h.clearPointer)
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
			// The panel is a cleanup QUEUE, not a full inventory (WTREE-01
			// refinement): skip active-work rows BEFORE buildRow so hidden rows
			// do no needless git/gh work (and never double-call gh's PRState).
			if !h.isCleanupCandidate(ctx, g.meta.repoPath, ew) {
				continue
			}
			row := h.buildRow(ctx, g.meta, ew)
			pg.Worktrees = append(pg.Worktrees, row)
			resp.Counts.Total++
			if row.Classification == "orphan" {
				resp.Counts.Orphaned++
			}
		}
		// Omit a project group that has no shown worktrees (no empty groups).
		if len(pg.Worktrees) == 0 {
			continue
		}
		resp.Projects = append(resp.Projects, pg)
	}
	writeJSON(w, http.StatusOK, resp)
}

// isCleanupCandidate reports whether a worktree should appear in the cleanup
// panel (user decision: the panel is a cleanup queue, not a full inventory).
// Orphans and stale pointers are always housekeeping candidates. A REFERENCED
// worktree only appears once its work is finished — task Done, or PR merged/
// closed (gh-unconfirmed ⇒ treated as still-active ⇒ hidden, degrade-don't-break).
func (h *cleanupPanelHandlers) isCleanupCandidate(ctx context.Context, repo string, ew enumWorktree) bool {
	switch ew.classification {
	case "orphan", "stale":
		return true
	default: // "referenced"
		_, ok := h.eligibilityReason(ctx, repo, ew)
		return ok
	}
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

// --- HTTP: POST /api/worktrees/remove (per-item force-remove, WTREE-02/D-03) ---

// remove force-removes a single worktree through the ONE shared gated path
// (the panel is the 3rd caller). D-03: the client always sends force=true AND
// stop_sessions=true for the per-item override, but both are read from the body
// and honored. The gates are re-checked server-side inside CleanupWorktreeGated
// (Pitfall 4/8 — the client snapshot is advisory). Outcome mapping:
//   - removed          → 204
//   - *BlockedError    → 200 {outcome:"blocked", path} (D-01, NOT a 500)
//   - !removed+reason  → 409 reason (a raced gate; force normally clears it)
//   - other err        → 500
func (h *cleanupPanelHandlers) remove(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Repo         string `json:"repo"`
		Path         string `json:"path"`
		TaskID       int64  `json:"task_id"`
		Force        bool   `json:"force"`
		StopSessions bool   `json:"stop_sessions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(req.Repo) == "" || strings.TrimSpace(req.Path) == "" {
		writeError(w, http.StatusBadRequest, "repo and path are required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), enumTimeout)
	defer cancel()

	// task_id==0 (orphan) → cleanupSessionCount/liveTmuxNames are no-ops (tmux
	// rows are keyed by task_id; an orphan has none).
	count := h.cleanupSessionCount(ctx, req.TaskID)
	live := h.liveTmuxNames(ctx, req.TaskID)

	removed, reason, err := CleanupWorktreeGated(
		ctx, h.db, h.wt, h.mgr, h.tmuxClient, live,
		req.TaskID, req.Repo, req.Path, count, req.StopSessions, req.Force)

	// D-01: a permission block is a distinct 200 outcome, not a 500. The git
	// registration + DB row stay intact (CleanupWorktreeGated did not proceed).
	var be *BlockedError
	if errors.As(err, &be) {
		writeJSON(w, http.StatusOK, map[string]any{"outcome": "blocked", "path": be.Path})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !removed {
		// A raced gate (force should normally clear it, but honor a real reason).
		writeError(w, http.StatusConflict, reason)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- HTTP: POST /api/worktrees/clean-eligible (bulk, WTREE-03/D-04/D-05) ---

// eligibleItem is one entry in the bulk preview/execution set (23-04 contract).
type eligibleItem struct {
	Repo        string `json:"repo"`
	Path        string `json:"path"`
	TaskID      int64  `json:"task_id"`
	ProjectName string `json:"project_name"`
	Reason      string `json:"reason"` // "orphaned" | "done" | "pr_merged" | "pr_closed"
}

// cleanEligible recomputes the provably-safe set SERVER-SIDE (never trusts a
// client list) and either previews it (dry_run) or removes it best-effort per
// item. It NEVER forces (D-04/D-05): eligibility requires all four gates to
// already pass, and each removal calls CleanupWorktreeGated with
// stopSessions=false, force=false. A raced-tripped gate skips that item and
// continues (unlike removeManaged's all-or-nothing).
func (h *cleanupPanelHandlers) cleanEligible(w http.ResponseWriter, r *http.Request) {
	dryRun := r.URL.Query().Get("dry_run") == "1"

	ctx, cancel := context.WithTimeout(r.Context(), enumTimeout)
	defer cancel()

	items := h.computeEligible(ctx)

	if dryRun {
		if items == nil {
			items = []eligibleItem{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
		return
	}

	// Execute: best-effort per item. NEVER force (stopSessions=false, force=false).
	removed, skipped := 0, 0
	for _, it := range items {
		count := h.cleanupSessionCount(ctx, it.TaskID)
		live := h.liveTmuxNames(ctx, it.TaskID)
		ok, _, err := CleanupWorktreeGated(
			ctx, h.db, h.wt, h.mgr, h.tmuxClient, live,
			it.TaskID, it.Repo, it.Path, count, false /*stopSessions*/, false /*force*/)
		if err != nil || !ok {
			// A race re-tripped a gate, or a blocked shell — skip it, keep going.
			skipped++
			continue
		}
		removed++
	}
	writeJSON(w, http.StatusOK, map[string]any{"removed": removed, "skipped": skipped})
}

// computeEligible enumerates the same union as list() and returns the provably-
// safe removal set (D-04): orphans OR referenced-done/pr-merged-or-closed, that
// pass ALL four gates (no session, no dirty, no unpushed, no stash) and are not
// a stale pointer. It reuses the shared enumerate() spine so the set never
// drifts from what list() shows. A row whose unpushed base can't resolve is
// conservatively INELIGIBLE (null unpushed fails the unpushed==0 test). NEVER
// includes anything that would need force.
func (h *cleanupPanelHandlers) computeEligible(ctx context.Context) []eligibleItem {
	groups, err := h.enumerate(ctx)
	if err != nil {
		slog.Warn("clean-eligible enumeration failed", "error", err)
		return nil
	}
	var out []eligibleItem
	for _, g := range groups {
		for _, ew := range g.worktrees {
			// Stale pointers are never bulk-removed (no dir to remove — the D-07
			// "clear pointer" action handles them).
			if ew.classification == "stale" {
				continue
			}
			// The dir must exist on disk to be removable.
			if _, statErr := os.Stat(ew.path); statErr != nil {
				continue
			}

			reason, ok := h.eligibilityReason(ctx, g.meta.repoPath, ew)
			if !ok {
				continue
			}

			// All four gates must already pass (never force).
			if !h.passesAllGates(ctx, g.meta.repoPath, ew) {
				continue
			}

			var taskID int64
			if ew.task != nil {
				taskID = ew.task.id
			}
			out = append(out, eligibleItem{
				Repo:        g.meta.repoPath,
				Path:        ew.path,
				TaskID:      taskID,
				ProjectName: g.meta.name,
				Reason:      reason,
			})
		}
	}
	return out
}

// eligibilityReason reports whether a REFERENCED/ORPHAN worktree qualifies for
// bulk removal on the task-status/PR axis (D-04), and the reason string
// (23-04 contract: "orphaned"|"done"|"pr_merged"|"pr_closed"). Orphans always
// qualify on this axis (no task to protect). A referenced task qualifies when
// its task is Done OR its PR is MERGED/CLOSED. PR state uses the RAW UPPERCASE
// value (NOT the display-lowercased one); a gh error ⇒ not eligible
// (degrade-don't-break — that row is simply skipped, never forced).
func (h *cleanupPanelHandlers) eligibilityReason(ctx context.Context, repo string, ew enumWorktree) (string, bool) {
	if ew.classification == "orphan" || ew.task == nil {
		return "orphaned", true
	}
	t := ew.task
	if t.source == "github_pr" && t.prNumber > 0 {
		if h.pr == nil {
			return "", false
		}
		state, err := h.pr.PRState(ctx, repo, t.prNumber)
		if err != nil {
			return "", false // degrade-don't-break: not eligible, never forced
		}
		switch state {
		case "MERGED":
			return "pr_merged", true
		case "CLOSED":
			return "pr_closed", true
		default:
			return "", false
		}
	}
	if t.status == "done" {
		return "done", true
	}
	return "", false
}

// passesAllGates reports whether a worktree passes ALL four conservative gates
// (D-04): no live session, no dirty file, no unpushed commit, no stash. A row
// whose unpushed base can't resolve is conservatively ineligible (null unpushed
// fails unpushed==0). Any git error on a gate ⇒ ineligible (never remove on a
// gate we couldn't compute).
func (h *cleanupPanelHandlers) passesAllGates(ctx context.Context, repo string, ew enumWorktree) bool {
	// Sessions.
	if ew.task != nil && h.cleanupSessionCount(ctx, ew.task.id) > 0 {
		return false
	}
	// Dirty.
	dirty, derr := h.wt.DirtyCount(ctx, ew.path)
	if derr != nil || dirty > 0 {
		return false
	}
	// Stash.
	stash, serr := h.wt.StashCount(ctx, ew.path)
	if serr != nil || stash > 0 {
		return false
	}
	// Unpushed: base must resolve AND count must be 0 (unresolvable ⇒ ineligible).
	base, ok := h.unpushedBase(ctx, repo, ew)
	if !ok {
		return false
	}
	unpushed, uerr := h.wt.UnpushedCount(ctx, ew.path, base)
	if uerr != nil || unpushed > 0 {
		return false
	}
	return true
}

// --- HTTP: POST /api/worktrees/clear-pointer (stale-pointer reconcile, D-07) ---

// clearPointer nulls a stale task's worktree columns (keeping the row + branch
// history) and prunes the stale git registration at the task's project repo
// (D-07). Deletes NO files — the dir is already gone (that is what "stale"
// means). Returns 204.
func (h *cleanupPanelHandlers) clearPointer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TaskID int64 `json:"task_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.TaskID <= 0 {
		writeError(w, http.StatusBadRequest, "task_id is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), enumTimeout)
	defer cancel()

	// Look up the task's project repo BEFORE nulling the columns so we can prune
	// the stale git registration there.
	var repo string
	err := h.db.QueryRowContext(ctx,
		`SELECT (SELECT repo_path FROM projects WHERE projects.id = tasks.project_id)
		 FROM tasks WHERE tasks.id = ?`, req.TaskID).Scan(&repo)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Null the pointer, KEEP the row + branch history (D-07 / D-34).
	if _, uerr := h.db.ExecContext(ctx,
		`UPDATE tasks SET branch = NULL, worktree_path = NULL, worktree_error = NULL WHERE id = ?`,
		req.TaskID); uerr != nil {
		writeError(w, http.StatusInternalServerError, uerr.Error())
		return
	}

	// Prune the stale git registration so a subsequent scan is clean. Best-effort
	// — a prune failure must not fail the DB-side clear (the pointer is already
	// nulled, which is the load-bearing reconciliation).
	if perr := h.wt.Prune(ctx, repo); perr != nil {
		slog.Warn("worktree prune after clear-pointer failed", "task", req.TaskID, "repo", repo, "error", perr)
	}

	w.WriteHeader(http.StatusNoContent)
}

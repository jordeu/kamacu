package api

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"kamacu/internal/github"
	"kamacu/internal/settings"
	"kamacu/internal/worktree"
)

// viewPR is the PR-metadata read seam (defaults to github.ViewPR). It is a
// package var so the review-endpoint tests can stub the live gh read and stay
// hermetic — the git half (CheckoutPR) runs against a real local repo, the gh
// half is injected. Production wiring leaves it as github.ViewPR.
var viewPR = github.ViewPR

// PullRequestRoutes registers:
//   - GET  /api/projects/{id}/pull-requests[?refresh=1]   — the review-queue list
//   - GET  /api/projects/{id}/pull-requests/{n}            — live PR detail (re-hydrate)
//   - POST /api/projects/{id}/pull-requests/{n}/review     — open-or-reattach
//
// Always 200 on the GET — the Result.State field carries degradation (GHSET-03 /
// GHCOL-05); the frontend branches on state/stale, never on the HTTP status.
// This endpoint is the GHSET-02 backend enforcement point: it returns
// state=disabled when github_integration is off OR the project is unlinked,
// BEFORE ever touching gh. The frontend's useSettings() gate is convenience;
// this server gate is the one that actually prevents a gh spawn.
//
// Registered as a sibling of UsageRoutes (not inside Routes()) so the api
// package's newTestServer is unaffected and the PR endpoint test can register
// its own mux with a fake-runner Service. wtSvc provisions the detached PR-head
// worktree on the POST review path.
func PullRequestRoutes(mux *http.ServeMux, db *sql.DB, svc *github.Service, wtSvc *worktree.Service) {
	mux.HandleFunc("GET /api/projects/{id}/pull-requests", func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}

		// GATE 1 (GHSET-02): integration toggle off -> disabled, never spawn gh.
		// settings.Get returns the code default "on" for an absent row, so any
		// non-"on" value (including a future "off") disables.
		val, err := settings.Get(db, settings.KeyGithubIntegration)
		if err != nil {
			writeJSON(w, http.StatusOK, github.Result{State: "error"})
			return
		}
		if val != "on" {
			writeJSON(w, http.StatusOK, github.Result{State: "disabled"})
			return
		}

		// GATE 2: project not linked -> disabled, never spawn gh. Fetch repo_path
		// alongside github_repo for cmd.Dir (Pitfall 6: gh host/account select).
		var repo sql.NullString
		var repoPath string
		qerr := db.QueryRow(`SELECT github_repo, repo_path FROM projects WHERE id = ?`, id).Scan(&repo, &repoPath)
		if errors.Is(qerr, sql.ErrNoRows) {
			// Unknown project still returns disabled (200), NOT 404 — the column
			// never blocks the board on a project-lookup miss (degrade-don't-break).
			writeJSON(w, http.StatusOK, github.Result{State: "disabled"})
			return
		}
		if qerr != nil {
			writeJSON(w, http.StatusOK, github.Result{State: "error"})
			return
		}
		if !repo.Valid || strings.TrimSpace(repo.String) == "" {
			writeJSON(w, http.StatusOK, github.Result{State: "disabled"})
			return
		}

		force := r.URL.Query().Get("refresh") == "1"
		writeJSON(w, http.StatusOK, svc.Get(r.Context(), repo.String, repoPath, force))
	})

	// GET .../pull-requests/{n} — live PR detail for re-hydration (12-07).
	//
	// Pure read: a fresh `gh pr view` mapped to the prWire the POST /review
	// handler returns. NO worktree, NO DB write. The review view seeds this from
	// the open-time POST response, but a hard reload wipes the client cache; the
	// frontend re-fetches here so the header (link/author/from-branch) and the
	// seed's interpolated title come from live GitHub again (the "live, never
	// drift" design — D-08/D-11, Pitfall 6). Same toggle+link gating as the
	// list/review handlers; a gh failure degrades to 502 and the frontend simply
	// keeps the header on its task-field fallback.
	mux.HandleFunc("GET /api/projects/{id}/pull-requests/{n}", func(w http.ResponseWriter, r *http.Request) {
		pid, ok := pathID(w, r)
		if !ok {
			return
		}
		n, perr := strconv.ParseInt(r.PathValue("n"), 10, 64)
		if perr != nil {
			writeError(w, http.StatusBadRequest, "invalid PR number")
			return
		}

		// GATE 1 (GHSET-02): integration off -> 409, never spawn gh.
		val, err := settings.Get(db, settings.KeyGithubIntegration)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if val != "on" {
			writeError(w, http.StatusConflict, "GitHub integration is off")
			return
		}

		// GATE 2: project not linked / unknown -> 409, never spawn gh.
		var repo sql.NullString
		qerr := db.QueryRow(`SELECT github_repo FROM projects WHERE id = ?`, pid).Scan(&repo)
		if errors.Is(qerr, sql.ErrNoRows) {
			writeError(w, http.StatusConflict, "project is not linked to a GitHub repo")
			return
		}
		if qerr != nil {
			writeError(w, http.StatusInternalServerError, qerr.Error())
			return
		}
		if !repo.Valid || strings.TrimSpace(repo.String) == "" {
			writeError(w, http.StatusConflict, "project is not linked to a GitHub repo")
			return
		}

		detail, derr := viewPR(r.Context(), repo.String, int(n))
		if derr != nil {
			writeError(w, http.StatusBadGateway, "couldn't load this PR: "+derr.Error())
			return
		}
		writeJSON(w, http.StatusOK, prWireFrom(detail))
	})

	// POST .../pull-requests/{n}/review — open-or-reattach a PR review (GHREV-01/02).
	//
	// Find-or-create a source='github_pr' task keyed by (project_id, pr_number);
	// provision a DETACHED worktree on the PR head via wtSvc.CheckoutPR; return
	// {task, pr} where pr is the LIVE gh pr view detail (so the header/body never
	// drift from GitHub, RESEARCH Pitfall 6). Provisioning is synchronous,
	// mirroring the task-create path. The off/unlinked gates short-circuit with a
	// 409 BEFORE any gh/worktree work (the open action can't proceed without a
	// linked repo — unlike the GET list, which degrades to a state field).
	mux.HandleFunc("POST /api/projects/{id}/pull-requests/{n}/review", func(w http.ResponseWriter, r *http.Request) {
		pid, ok := pathID(w, r)
		if !ok {
			return
		}
		n, perr := strconv.ParseInt(r.PathValue("n"), 10, 64)
		if perr != nil {
			writeError(w, http.StatusBadRequest, "invalid PR number")
			return
		}

		// GATE 1 (GHSET-02): integration off -> 409, never spawn gh.
		val, err := settings.Get(db, settings.KeyGithubIntegration)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if val != "on" {
			writeError(w, http.StatusConflict, "GitHub integration is off")
			return
		}

		// GATE 2: project not linked / unknown -> 409, never spawn gh.
		var repo sql.NullString
		var repoPath string
		qerr := db.QueryRow(`SELECT github_repo, repo_path FROM projects WHERE id = ?`, pid).Scan(&repo, &repoPath)
		if errors.Is(qerr, sql.ErrNoRows) {
			writeError(w, http.StatusConflict, "project is not linked to a GitHub repo")
			return
		}
		if qerr != nil {
			writeError(w, http.StatusInternalServerError, qerr.Error())
			return
		}
		if !repo.Valid || strings.TrimSpace(repo.String) == "" {
			writeError(w, http.StatusConflict, "project is not linked to a GitHub repo")
			return
		}
		ghRepo := repo.String

		// FIND EXISTING (GHREV-02 reattach key: project_id + pr_number + source).
		// by-id/by-key lookup is deliberately unfiltered-from-board (12-02) — the
		// review view legitimately deep-links its own PR row.
		existing, ferr := scanTask(db.QueryRow(
			`SELECT `+taskColumns+` FROM tasks WHERE project_id = ? AND pr_number = ? AND source = 'github_pr'`,
			pid, n))
		found := ferr == nil
		if ferr != nil && !errors.Is(ferr, sql.ErrNoRows) {
			writeError(w, http.StatusInternalServerError, ferr.Error())
			return
		}

		// The live PR detail is the source of truth for the header/body (D-08/D-11,
		// Pitfall 6). Fetch it on EVERY open (create AND reattach) so a renamed PR
		// never shows a stale title. A gh failure degrades to 502 — and on the
		// create path we have not yet inserted a row, so there is no half-created
		// state to clean up.
		detail, derr := viewPR(r.Context(), ghRepo, int(n))
		if derr != nil {
			writeError(w, http.StatusBadGateway, "couldn't open this review: "+derr.Error())
			return
		}

		var t Task
		switch {
		case found && existing.WorktreePath != nil && strings.TrimSpace(*existing.WorktreePath) != "":
			// Fully provisioned review — instant reattach (GHREV-02). NO CheckoutPR.
			t = existing
			writeReview(w, t, detail)
			return
		case found:
			// A prior provision failed (worktree_path NULL) — re-provision in
			// place using the existing row id (the Retry path).
			t = existing
		default:
			// First open — INSERT a new source='github_pr' row. status='todo'; the
			// board filter (12-02) excludes it regardless of status.
			// position is NOT NULL in the schema but meaningless for a PR review:
			// the board-leak filters (12-02) exclude source='github_pr' rows from
			// EVERY board/position query, so the value never participates in
			// ordering. Insert a fixed 0 to satisfy the column (GHREV-04 keeps it
			// off the board regardless of status/position).
			title := fmt.Sprintf("#%d %s", n, detail.Title)
			t, err = scanTask(db.QueryRow(
				`INSERT INTO tasks (project_id, title, status, source, pr_number, pr_base_ref, position)
				 VALUES (?, ?, 'todo', 'github_pr', ?, ?, 0)
				 RETURNING `+taskColumns, pid, title, n, detail.BaseRefName))
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}

		// PROVISION the detached PR-head worktree (GHREV-01/05). Failure lands in
		// worktree_error (D-25) and returns 502 — the row persists for a Retry.
		baseRaw, err := settings.Get(db, settings.KeyWorktreeBase)
		if err != nil {
			recordWorktreeError(db, t.ID, err)
			writeError(w, http.StatusBadGateway, "couldn't set up the worktree: "+err.Error())
			return
		}
		baseDir, err := settings.ExpandHome(baseRaw)
		if err != nil {
			recordWorktreeError(db, t.ID, err)
			writeError(w, http.StatusBadGateway, "couldn't set up the worktree: "+err.Error())
			return
		}
		prWtPath := worktree.PathUnder(baseDir, repoPath, "pr-"+strconv.FormatInt(n, 10), t.ID)
		if cerr := wtSvc.CheckoutPR(r.Context(), repoPath, prWtPath, detail.HeadRefOid, detail.HeadRefName, int(n)); cerr != nil {
			recordWorktreeError(db, t.ID, cerr)
			writeError(w, http.StatusBadGateway, "couldn't set up the worktree: "+cerr.Error())
			return
		}
		if _, derr := db.Exec(`UPDATE tasks SET worktree_path = ?, worktree_error = NULL WHERE id = ?`, prWtPath, t.ID); derr != nil {
			slog.Error("recording PR worktree on task", "task", t.ID, "error", derr)
		}
		t.WorktreePath = &prWtPath
		t.WorktreeError = nil

		writeReview(w, t, detail)
	})
}

// prWire is the live PR detail returned alongside the task. The frontend reads
// task.source to branch and pr.{title,body,author,url,baseRefName,headRefName,
// commits} for the read-only header / PR meta line / Description / merge line
// (these MUST come from a live gh read, never the stored row, so they mirror
// GitHub — D-08/D-11, Pitfall 6). headRefName + commits feed the GitHub-style
// "<author> wants to merge <N> commits into <base> from <head>" line (12-07).
type prWire struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	Author      string `json:"author"`
	URL         string `json:"url"`
	BaseRefName string `json:"baseRefName"`
	HeadRefName string `json:"headRefName"`
	Commits     int    `json:"commits"`
	State       string `json:"state"` // "OPEN" | "CLOSED" | "MERGED" — drives the D-09 review banner
}

// prWireFrom maps a live github.PRDetail to the prWire the frontend reads. The
// single builder keeps the POST /review envelope and the GET .../{n} detail
// byte-identical, so a hard-reload re-hydration (GET) renders exactly what the
// open-time response (POST) seeded.
func prWireFrom(d github.PRDetail) prWire {
	return prWire{
		Number:      d.Number,
		Title:       d.Title,
		Body:        d.Body,
		Author:      d.AuthorLogin,
		URL:         d.URL,
		BaseRefName: d.BaseRefName,
		HeadRefName: d.HeadRefName,
		Commits:     d.Commits,
		State:       d.State,
	}
}

// writeReview returns the {task, pr} envelope (200).
func writeReview(w http.ResponseWriter, t Task, d github.PRDetail) {
	writeJSON(w, http.StatusOK, map[string]any{
		"task": t,
		"pr":   prWireFrom(d),
	})
}

// recordWorktreeError stamps a provisioning failure on the task (D-25 Retry
// path) without failing the surrounding handler on the UPDATE itself.
func recordWorktreeError(db *sql.DB, taskID int64, cause error) {
	if _, err := db.Exec(`UPDATE tasks SET worktree_error = ? WHERE id = ?`, cause.Error(), taskID); err != nil {
		slog.Error("recording PR worktree error on task", "task", taskID, "error", err)
	}
}

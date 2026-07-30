package api

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"kamacu/internal/github"
	"kamacu/internal/settings"
)

// aggregateReviewsResult mirrors github.MergedClosedResult's shape (State /
// Stale / FetchedAt / PRs) so the activity endpoint's `reviews` sub-object reads
// the SAME wire contract as the review column — just under a `reviews` key with
// ReviewDoneSummary elements. The whole response is ALWAYS HTTP 200; degradation
// rides in State (D-02, REVIEWS-04).
type aggregateReviewsResult struct {
	State     string                     `json:"state"`
	Stale     bool                       `json:"stale"`
	FetchedAt *time.Time                 `json:"fetchedAt"`
	PRs       []github.ReviewDoneSummary `json:"prs"`
}

// activityResponse is the combined GET /api/activity payload (D-01): the
// tasks-done list, the reviews-done sub-object, and the stats block. Phase 11
// renders all three from one TanStack query.
type activityResponse struct {
	Tasks   []activityTask         `json:"tasks"`
	Reviews aggregateReviewsResult `json:"reviews"`
	Stats   statsBlock             `json:"stats"`
}

// repoRow is the row shape the linked-projects query yields: the gh identity
// (github_repo) + cmd.Dir (repo_path) + the project provenance aggregateReviews
// annotates onto each ReviewDoneSummary (D-06).
type repoRow struct {
	githubRepo  string
	repoPath    string
	projectID   int64
	projectName string
}

// aggregateReviews fans out one GetMergedClosed call per in-scope linked repo,
// concurrently and bounded (D-07), then applies the WINDOW FILTER to each repo's
// PRs (REVIEWS-01 / STATS-01) and rolls the per-repo states up into the
// aggregate reviews.state (D-08). cutoff threads the window in as a time.Time
// so the completedAt-vs-cutoff comparison is time-compare, never a string
// compare across the ms/s precision mismatch.
//
// A failing/slow repo degrades WITHOUT blocking the others or canceling the
// group (Pitfall 4): each closure runs under a per-repo context.WithTimeout and
// returns nil on EVERY outcome — a degrade rides in the per-repo result's state,
// never as an errgroup error. When repos is empty, state="ok" with empty PRs.
func aggregateReviews(ctx context.Context, ghSvc *github.Service, repos []repoRow, force bool, cutoff time.Time) aggregateReviewsResult {
	if len(repos) == 0 {
		return aggregateReviewsResult{State: "ok"}
	}

	type perRepo struct {
		prs       []github.ReviewDoneSummary
		state     string
		fetchedAt *time.Time
		stale     bool
	}
	results := make([]perRepo, len(repos))

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(5)
	for i, rc := range repos {
		i, rc := i, rc
		g.Go(func() error {
			rctx, cancel := context.WithTimeout(gctx, 12*time.Second)
			defer cancel()
			res := ghSvc.GetMergedClosed(rctx, rc.githubRepo, rc.repoPath, force)
			pr := perRepo{state: res.State, fetchedAt: res.FetchedAt, stale: res.Stale}
			for _, p := range res.PRs {
				p.ProjectName = rc.projectName
				p.ProjectID = rc.projectID
				p.Repo = rc.githubRepo
				// WINDOW FILTER (REVIEWS-01 / STATS-01): drop a PR whose
				// completedAt (gh closedAt, second-precision) falls BEFORE the
				// window cutoff. parseActivityTime handles BOTH the ms layout
				// (cutoff-shaped) AND the bare-Z layout (gh closedAt shape), so
				// the comparison is time.Time-vs-time.Time — never a string
				// compare across the precision mismatch. An unparseable
				// completedAt is dropped defensively rather than panicked.
				if t, ok := parseActivityTime(p.CompletedAt); !ok || t.Before(cutoff) {
					continue
				}
				pr.prs = append(pr.prs, p)
			}
			results[i] = pr
			return nil // a degrade never cancels the group (Pitfall 4)
		})
	}
	_ = g.Wait()

	states := make([]string, 0, len(results))
	var allPRs []github.ReviewDoneSummary
	var latestFetched *time.Time
	anyStale := false
	for _, r := range results {
		states = append(states, r.state)
		allPRs = append(allPRs, r.prs...)
		if r.stale {
			anyStale = true
		}
		if r.fetchedAt != nil && (latestFetched == nil || r.fetchedAt.After(*latestFetched)) {
			ft := *r.fetchedAt
			latestFetched = &ft
		}
	}
	return aggregateReviewsResult{
		State:     rollupReviewState(states),
		Stale:     anyStale,
		FetchedAt: latestFetched,
		PRs:       allPRs,
	}
}

// fetchActivityTasks runs the tasks-done SQL (D-12): done manual tasks in-window,
// narrowed by scope, ordered done_at DESC. The scope id is a parameterized ?
// placeholder; the WHERE clause is selected structurally on the parsed kind —
// the raw scope param is NEVER string-concatenated (Pitfall 5, T-10-05).
func fetchActivityTasks(ctx context.Context, db *sql.DB, scopeKind string, scopeID int64, cutoffStr string) ([]activityTask, error) {
	const base = `SELECT t.id, t.title, t.done_at, t.in_progress_at, t.in_review_at, p.id, p.name
		FROM tasks t JOIN projects p ON p.id = t.project_id
		WHERE t.status = 'done' AND t.source = 'manual' AND t.done_at IS NOT NULL AND t.done_at >= ?`
	q := base
	var args []any
	args = append(args, cutoffStr)
	switch scopeKind {
	case "workspace":
		q += " AND p.workspace_id = ?"
		args = append(args, scopeID)
	case "project":
		q += " AND t.project_id = ?"
		args = append(args, scopeID)
	}
	q += " ORDER BY t.done_at DESC"

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []activityTask
	for rows.Next() {
		var t activityTask
		var doneAt, inProg, inRev sql.NullString
		if err := rows.Scan(&t.ID, &t.Title, &doneAt, &inProg, &inRev, &t.ProjectID, &t.ProjectName); err != nil {
			return nil, err
		}
		if doneAt.Valid {
			t.DoneAt = doneAt.String
		}
		if inProg.Valid {
			t.InProgressAt = inProg.String
		}
		if inRev.Valid {
			t.InReviewAt = inRev.String
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// fetchLinkedRepos runs the in-scope linked-projects query: the rows that feed
// aggregateReviews. Same structural scope-branching as fetchActivityTasks
// (parameterized ? placeholder, never raw interpolation).
func fetchLinkedRepos(ctx context.Context, db *sql.DB, scopeKind string, scopeID int64) ([]repoRow, error) {
	const base = `SELECT id, name, github_repo, repo_path FROM projects
		WHERE github_repo IS NOT NULL AND TRIM(github_repo) != ''`
	q := base
	var args []any
	switch scopeKind {
	case "workspace":
		q += " AND workspace_id = ?"
		args = append(args, scopeID)
	case "project":
		q += " AND id = ?"
		args = append(args, scopeID)
	}
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []repoRow
	for rows.Next() {
		var rc repoRow
		if err := rows.Scan(&rc.projectID, &rc.projectName, &rc.githubRepo, &rc.repoPath); err != nil {
			return nil, err
		}
		out = append(out, rc)
	}
	return out, rows.Err()
}

// ActivityRoutes registers GET /api/activity — the combined tasks-done /
// reviews-done / stats endpoint (D-01). It mirrors PullRequestRoutes: the SAME
// shared *github.Service (no new construction), GATE 1 short-circuits to
// reviews.state="disabled" before any gh spawn, and the response is ALWAYS HTTP
// 200 with degradation riding in reviews.state (REVIEWS-04). Read-only — no
// worktree service needed.
func ActivityRoutes(mux *http.ServeMux, db *sql.DB, ghSvc *github.Service) {
	mux.HandleFunc("GET /api/activity", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		scopeKind, scopeID, err := parseScope(q.Get("scope"))
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		window, err := parseWindow(q.Get("window"))
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		// The SAME instant, two representations (Pitfall: ms/s precision). The
		// ms-ISO cutoffStr feeds the lexical SQL compare (reaper done_at
		// format); the cutoffTime time.Time threads into aggregateReviews'
		// completedAt window filter (gh closedAt is second-precision).
		now := time.Now().UTC()
		cutoffTime := now.Add(-window)
		cutoffStr := cutoffTime.Format("2006-01-02T15:04:05.000Z")

		// Tasks-done + stats (always populate regardless of gh state).
		tasks, qerr := fetchActivityTasks(r.Context(), db, scopeKind, scopeID, cutoffStr)
		if qerr != nil {
			slog.Error("activity: tasks-done query", "error", qerr)
			writeJSON(w, http.StatusOK, activityResponse{
				Reviews: aggregateReviewsResult{State: "error"},
				Stats:   statsBlock{TaskCount: 0},
			})
			return
		}
		cycle, dip, dir := buildDurationSlices(tasks)
		stats := statsBlock{
			TaskCount:       len(tasks),
			Cycle:           computeTimeStat(cycle),
			DwellInProgress: computeTimeStat(dip),
			DwellInReview:   computeTimeStat(dir),
		}

		// GATE 1 (GHSET-02): integration toggle off -> disabled, NEVER spawn gh.
		// settings.Get returns the code default "on" for an absent row, so any
		// non-"on" value disables (mirrors pullrequests.go:56-64).
		var reviews aggregateReviewsResult
		val, gerr := settings.Get(db, settings.KeyGithubIntegration)
		if gerr != nil {
			reviews.State = "error"
		} else if val != "on" {
			reviews.State = "disabled"
		} else {
			repos, lerr := fetchLinkedRepos(r.Context(), db, scopeKind, scopeID)
			if lerr != nil {
				slog.Error("activity: linked-projects query", "error", lerr)
				reviews.State = "error"
			} else {
				force := strings.TrimSpace(q.Get("refresh")) == "1"
				reviews = aggregateReviews(r.Context(), ghSvc, repos, force, cutoffTime)
			}
		}

		stats.ReviewCount = len(reviews.PRs)
		writeJSON(w, http.StatusOK, activityResponse{Tasks: tasks, Reviews: reviews, Stats: stats})
	})
}

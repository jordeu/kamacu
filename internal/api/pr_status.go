package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"time"

	"kamacu/internal/github"
	"kamacu/internal/settings"
)

// Project-wide status polling keeps all card hooks on one shared query. Only
// explicit, non-rejected links belonging to this project's board are read.
func (h prContextHandlers) projectStatuses(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var exists int
	if err := h.db.QueryRowContext(r.Context(), `SELECT 1 FROM projects WHERE id=?`, id).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, 404, "project not found")
		} else {
			writeError(w, 500, err.Error())
		}
		return
	}
	rows, err := h.db.QueryContext(r.Context(), `SELECT DISTINCT lower(l.repository),l.number FROM task_pr_links l JOIN tasks t ON t.id=l.task_id WHERE t.project_id=? AND t.source='manual' AND l.role!='incidental' ORDER BY lower(l.repository),l.number`, id)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	type identity struct {
		repo   string
		number int64
	}
	links := []identity{}
	for rows.Next() {
		var l identity
		if err = rows.Scan(&l.repo, &l.number); err != nil {
			break
		}
		links = append(links, l)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	enabled, err := settings.Get(h.db, settings.KeyGithubIntegration)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	statuses := make([]github.PRStatus, len(links))
	if enabled != "on" {
		for i, l := range links {
			statuses[i] = github.UnknownPRStatus(l.repo, l.number, "GitHub integration disabled")
		}
	} else {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		// Rotate the first worker-sized batch, including after failed polls.
		// Results still occupy their original, stable output positions.
		workers := min(4, len(links))
		start := h.statuses.NextBatchStart(strconv.FormatInt(id, 10), len(links), workers)
		jobs := make(chan int)
		var wg sync.WaitGroup
		for worker := 0; worker < workers; worker++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := range jobs {
					l := links[i]
					statuses[i] = h.statuses.Get(ctx, l.repo, l.number)
				}
			}()
		}
		for i := range links {
			jobs <- (start + i) % len(links)
		}
		close(jobs)
		wg.Wait()
	}
	writeJSON(w, 200, map[string]any{"statuses": statuses})
}

// Catch-up can reference PRs before a manual task exists. Use the same status
// service/cache as task cards without creating tasks or PR associations.
func (h prContextHandlers) pullRequestStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	n, err := strconv.ParseInt(r.PathValue("n"), 10, 64)
	if err != nil || n <= 0 {
		writeError(w, 400, "invalid PR number")
		return
	}
	var repo sql.NullString
	err = h.db.QueryRowContext(r.Context(), `SELECT github_repo FROM projects WHERE id=?`, id).Scan(&repo)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "project not found")
		return
	}
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	if !repo.Valid || repo.String == "" {
		writeError(w, 409, "project is not linked to a GitHub repo")
		return
	}
	enabled, err := settings.Get(h.db, settings.KeyGithubIntegration)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	if enabled != "on" {
		writeJSON(w, 200, github.UnknownPRStatus(repo.String, n, "GitHub integration disabled"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	writeJSON(w, 200, h.statuses.Get(ctx, repo.String, n))
}

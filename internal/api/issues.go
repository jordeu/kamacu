package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"kamacu/internal/github"
	"kamacu/internal/settings"
)

var searchIssues = github.SearchIssues
var viewIssue = github.ViewIssue

type issueProject struct {
	ID         int64
	Repo, Path string
	Managed    bool
}

func (h *taskHandlers) issueProject(w http.ResponseWriter, r *http.Request) (issueProject, bool) {
	var p issueProject
	id, ok := pathID(w, r)
	if !ok {
		return p, false
	}
	enabled, err := settings.Get(h.db, settings.KeyGithubIntegration)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return p, false
	}
	if enabled != "on" {
		writeError(w, http.StatusForbidden, "GitHub integration is disabled.")
		return p, false
	}
	var repo sql.NullString
	var managed int
	err = h.db.QueryRowContext(r.Context(), "SELECT github_repo, repo_path, managed FROM projects WHERE id = ?", id).Scan(&repo, &p.Path, &managed)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "project not found")
		return p, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return p, false
	}
	if !repo.Valid || strings.TrimSpace(repo.String) == "" {
		writeError(w, http.StatusBadRequest, "Link a GitHub repository in project settings first.")
		return p, false
	}
	p.ID, p.Repo, p.Managed = id, repo.String, managed != 0
	return p, true
}

func issueNumber(w http.ResponseWriter, r *http.Request) (int, bool) {
	n, err := strconv.Atoi(r.PathValue("number"))
	if err != nil || n < 1 {
		writeError(w, http.StatusBadRequest, "invalid issue number")
		return 0, false
	}
	return n, true
}

func (h *taskHandlers) importedIssue(ctx context.Context, p issueProject, n int) (Task, error) {
	return scanTask(h.db.QueryRowContext(ctx, `SELECT `+taskColumns+` FROM tasks WHERE project_id = ? AND issue_repo = ? COLLATE NOCASE AND issue_number = ?`, p.ID, p.Repo, n))
}

func (h *taskHandlers) listIssues(w http.ResponseWriter, r *http.Request) {
	p, ok := h.issueProject(w, r)
	if !ok {
		return
	}
	page := 1
	if raw := r.URL.Query().Get("page"); raw != "" {
		var err error
		page, err = strconv.Atoi(raw)
		if err != nil || page < 1 || page > 33 {
			writeError(w, http.StatusBadRequest, "page must be between 1 and 33")
			return
		}
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(query) > 256 {
		writeError(w, http.StatusBadRequest, "Search must be 256 characters or fewer.")
		return
	}
	result, err := searchIssues(r.Context(), p.Repo, p.Path, query, r.URL.Query().Get("include_closed") == "true", r.URL.Query().Get("assigned_to_me") == "true", page)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	for i := range result.Issues {
		task, err := h.importedIssue(r.Context(), p, result.Issues[i].Number)
		if err == nil {
			result.Issues[i].TaskID = &task.ID
		} else if !errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *taskHandlers) getIssue(w http.ResponseWriter, r *http.Request) {
	p, ok := h.issueProject(w, r)
	if !ok {
		return
	}
	n, ok := issueNumber(w, r)
	if !ok {
		return
	}
	issue, err := viewIssue(r.Context(), p.Repo, p.Path, n)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	task, err := h.importedIssue(r.Context(), p, n)
	if err == nil {
		issue.TaskID = &task.ID
	} else if !errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, issue)
}

type issueImportResult struct {
	Task            Task `json:"task"`
	AlreadyImported bool `json:"already_imported"`
}

func (h *taskHandlers) importIssue(w http.ResponseWriter, r *http.Request) {
	p, ok := h.issueProject(w, r)
	if !ok {
		return
	}
	n, ok := issueNumber(w, r)
	if !ok {
		return
	}
	existing, err := h.importedIssue(r.Context(), p, n)
	if err == nil {
		writeJSON(w, http.StatusOK, issueImportResult{Task: existing, AlreadyImported: true})
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	issue, err := viewIssue(r.Context(), p.Repo, p.Path, n)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	description := issue.Body
	if description != "" {
		description += "\n\n"
	}
	description += fmt.Sprintf("Imported from GitHub: %s", issue.URL)
	// The unique index makes concurrent clicks/retries idempotent. Git work is
	// performed only by the request that inserted the task, outside a transaction.
	task, err := scanTask(h.db.QueryRowContext(r.Context(), `INSERT INTO tasks
 (project_id,title,description,status,position,issue_repo,issue_number,issue_url)
 VALUES (?,?,?,'todo',(SELECT COALESCE(MIN(position),2.0)-1.0 FROM tasks WHERE project_id=? AND status='todo' AND source='manual'),?,?,?)
 ON CONFLICT DO NOTHING RETURNING `+taskColumns, p.ID, strings.TrimSpace(issue.Title), description, p.ID, p.Repo, n, issue.URL))
	if errors.Is(err, sql.ErrNoRows) {
		existing, err = h.importedIssue(r.Context(), p, n)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, issueImportResult{Task: existing, AlreadyImported: true})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	branch, path, provErr := provisionWorktree(r.Context(), h.db, h.wt, task.ID, task.Title, p.Path, p.Managed)
	if provErr != nil {
		msg := provErr.Error()
		task.WorktreeError = &msg
	} else {
		task.Branch = &branch
		task.WorktreePath = &path
	}
	writeJSON(w, http.StatusCreated, issueImportResult{Task: task})
}

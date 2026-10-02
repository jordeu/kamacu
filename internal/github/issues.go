package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Issue is a read-only snapshot; importing never changes the GitHub issue.
type Issue struct {
	Assignees []string `json:"-"`
	Number    int      `json:"number"`
	Title     string   `json:"title"`
	Body      string   `json:"body"`
	URL       string   `json:"url"`
	Author    string   `json:"author"`
	State     string   `json:"state"`
	TaskID    *int64   `json:"task_id,omitempty"`
}

type IssuePage struct {
	Issues  []Issue `json:"issues"`
	HasMore bool    `json:"has_more"`
}

type issueWire struct {
	Number        int    `json:"number"`
	Title         string `json:"title"`
	Body          string `json:"body"`
	URL           string `json:"html_url"`
	RepositoryURL string `json:"repository_url"`
	State         string `json:"state"`
	User          struct {
		Login string `json:"login"`
	} `json:"user"`
	Assignees []struct {
		Login string `json:"login"`
	} `json:"assignees"`
	PullRequest json.RawMessage `json:"pull_request"`
}

func (i issueWire) issue() Issue {
	issue := Issue{Number: i.Number, Title: i.Title, Body: i.Body, URL: i.URL, Author: i.User.Login, State: i.State}
	for _, assignee := range i.Assignees {
		issue.Assignees = append(issue.Assignees, assignee.Login)
	}
	return issue
}

var issueRunner = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err == nil {
		return out, nil
	}
	if ctx.Err() != nil {
		return nil, errors.New("GitHub request timed out or was cancelled. Try again.")
	}
	if errors.Is(err, exec.ErrNotFound) {
		return nil, errors.New("GitHub CLI (gh) is not installed. Install it to import issues.")
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		stderr := strings.ToLower(string(exit.Stderr))
		switch {
		case strings.Contains(stderr, "rate limit"):
			return nil, errors.New("GitHub rate limit reached. Try again later.")
		case strings.Contains(stderr, "401"), strings.Contains(stderr, "auth login"), strings.Contains(stderr, "authentication"):
			return nil, errors.New("GitHub authentication is required. Run gh auth login on the server, then retry.")
		case strings.Contains(stderr, "404"):
			return nil, errors.New("GitHub issue or repository is unavailable. Check the repository link and your access.")
		}
	}
	return nil, errors.New("Could not read GitHub issues. Check your connection and repository access, then retry.")
}

func readIssueJSON(ctx context.Context, dir string, target any, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out, err := issueRunner(ctx, dir, args...)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(out, target); err != nil {
		return errors.New("GitHub returned an invalid issue response. Try again.")
	}
	return nil
}

func ViewIssue(ctx context.Context, repo, dir string, number int) (Issue, error) {
	canonical, err := ParseRepoRef(repo)
	if err != nil {
		return Issue{}, err
	}
	if number < 1 {
		return Issue{}, errors.New("invalid issue number")
	}
	var raw issueWire
	err = readIssueJSON(ctx, dir, &raw, "api", "--hostname", "github.com", fmt.Sprintf("repos/%s/issues/%d", canonical, number))
	if err != nil {
		return Issue{}, err
	}
	if len(raw.PullRequest) > 0 && string(raw.PullRequest) != "null" {
		return Issue{}, errors.New("Select a GitHub issue, not a pull request.")
	}
	if raw.Number != number || strings.TrimSpace(raw.Title) == "" {
		return Issue{}, errors.New("GitHub returned an invalid issue.")
	}
	return raw.issue(), nil
}

func SearchIssues(ctx context.Context, repo, dir, query string, includeClosed, assignedToMe bool, page int) (IssuePage, error) {
	result := IssuePage{Issues: []Issue{}}
	canonical, err := ParseRepoRef(repo)
	if err != nil {
		return result, err
	}
	query = strings.TrimSpace(query)
	if n, err := strconv.Atoi(strings.TrimPrefix(query, "#")); err == nil && n > 0 {
		issue, err := ViewIssue(ctx, canonical, dir, n)
		if err != nil {
			return result, err
		}
		if assignedToMe {
			var user struct {
				Login string `json:"login"`
			}
			if err := readIssueJSON(ctx, dir, &user, "api", "--hostname", "github.com", "user"); err != nil {
				return result, err
			}
			if user.Login == "" {
				return result, errors.New("Could not identify your GitHub account. Check gh authentication and retry.")
			}
			assigned := false
			for _, login := range issue.Assignees {
				if strings.EqualFold(login, user.Login) {
					assigned = true
					break
				}
			}
			if !assigned {
				return result, nil
			}
		}
		if page == 1 && (includeClosed || issue.State == "open") {
			result.Issues = append(result.Issues, issue)
		}
		return result, nil
	}
	// Quote each search term so input cannot inject repository/state qualifiers.
	terms := []string{"repo:" + canonical, "is:issue"}
	if assignedToMe {
		terms = append(terms, "assignee:@me")
	}
	if !includeClosed {
		terms = append(terms, "is:open")
	}
	for _, term := range strings.Fields(query) {
		term = strings.NewReplacer("\"", "", "\\", "").Replace(term)
		if term != "" {
			terms = append(terms, "\""+term+"\"")
		}
	}
	var raw struct {
		Items      []issueWire `json:"items"`
		Total      int         `json:"total_count"`
		Incomplete bool        `json:"incomplete_results"`
	}
	err = readIssueJSON(ctx, dir, &raw, "api", "--hostname", "github.com", "--method", "GET", "search/issues", "-f", "q="+strings.Join(terms, " "), "-f", "sort=updated", "-f", "order=desc", "-f", "per_page=30", "-f", "page="+strconv.Itoa(page))
	if err != nil {
		return result, err
	}
	if raw.Incomplete {
		return result, errors.New("GitHub search was incomplete. Narrow your search and retry.")
	}
	for _, item := range raw.Items {
		// Defense in depth: never expose issues outside the project's repository.
		if !strings.EqualFold(item.RepositoryURL, "https://api.github.com/repos/"+canonical) || (len(item.PullRequest) > 0 && string(item.PullRequest) != "null") {
			continue
		}
		result.Issues = append(result.Issues, item.issue())
	}
	result.HasMore = page*30 < raw.Total && page < 33
	return result, nil
}

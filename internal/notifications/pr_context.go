package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"kamacu/internal/store"
)

type ViewerReview struct {
	ID          int64  `json:"id"`
	State       string `json:"state"`
	CommitID    string `json:"commit_id"`
	SubmittedAt string `json:"submitted_at"`
	URL         string `json:"html_url"`
}

// PRCommit contains a bounded subject line, never the full commit body.
type PRCommit struct {
	SHA    string `json:"sha"`
	URL    string `json:"html_url"`
	Commit struct {
		Message   string `json:"message"`
		Committer struct {
			Date string `json:"date"`
		} `json:"committer"`
	} `json:"commit"`
}

// PRFacts keeps identity, status and approvals outside truncatable excerpts.
// Nil approval means unknown; approval need not match the current head.
type PRFacts struct {
	ViewerReviewCount      int            `json:"viewer_review_count,omitempty"`
	ViewerReviewsTruncated bool           `json:"viewer_reviews_truncated,omitempty"`
	RecentCommitsTruncated bool           `json:"recent_commits_truncated,omitempty"`
	RecentCommits          []PRCommit     `json:"recent_commits"`
	ChangesError           string         `json:"changes_error,omitempty"`
	Repository             string         `json:"repository"`
	Number                 int64          `json:"number"`
	URL                    string         `json:"url"`
	Source                 string         `json:"source"`
	ObservedAt             time.Time      `json:"observed_at"`
	Available              bool           `json:"available"`
	Stale                  bool           `json:"stale"`
	Error                  string         `json:"error,omitempty"`
	State                  string         `json:"state"`
	Draft                  *bool          `json:"draft"`
	HeadCommit             string         `json:"head_commit"`
	UpdatedAt              string         `json:"updated_at"`
	RequestedReviewers     []string       `json:"requested_reviewers"`
	Viewer                 string         `json:"viewer"`
	ReviewsComplete        bool           `json:"reviews_complete"`
	ViewerEverApproved     *bool          `json:"viewer_ever_approved"`
	ViewerReviews          []ViewerReview `json:"viewer_reviews"`
}

func (g GitHub) PRContext(ctx context.Context, source Source, repository string, number int64) PRFacts {
	now := time.Now()
	if g.Now != nil {
		now = g.Now()
	}
	f := PRFacts{Source: "github_api", ObservedAt: now, Stale: true, ViewerReviews: []ViewerReview{}, RequestedReviewers: []string{}}
	repo, n, err := store.PRIdentity(repository, number)
	if err != nil {
		f.Error = err.Error()
		return f
	}
	f.Repository = repo
	f.Number = n
	f.URL = fmt.Sprintf("https://github.com/%s/pull/%d", repo, n)
	viewer, err := g.account(ctx, source)
	if err != nil {
		f.Error = err.Error()
		return f
	}
	f.Viewer = viewer
	host := g.config(source).Hostname
	path := fmt.Sprintf("repos/%s/pulls/%d", repo, n)
	raw, _, err := g.api(ctx, host, path)
	if err != nil {
		f.Error = err.Error()
		return f
	}
	var pr struct {
		State     string `json:"state"`
		Draft     *bool  `json:"draft"`
		Merged    bool   `json:"merged"`
		UpdatedAt string `json:"updated_at"`
		Head      struct {
			SHA string `json:"sha"`
		} `json:"head"`
		RequestedReviewers []struct {
			Login string `json:"login"`
		} `json:"requested_reviewers"`
		RequestedTeams []struct {
			Slug string `json:"slug"`
		} `json:"requested_teams"`
	}
	if json.Unmarshal(raw, &pr) != nil || pr.State == "" || pr.Head.SHA == "" {
		f.Error = "invalid PR metadata"
		return f
	}
	f.State = pr.State
	if pr.Merged {
		f.State = "merged"
	}
	f.Draft = pr.Draft
	f.HeadCommit = pr.Head.SHA
	f.UpdatedAt = pr.UpdatedAt
	for _, r := range pr.RequestedReviewers {
		f.RequestedReviewers = append(f.RequestedReviewers, r.Login)
	}
	for _, r := range pr.RequestedTeams {
		f.RequestedReviewers = append(f.RequestedReviewers, "team:"+r.Slug)
	}
	f.Available = true
	f.Stale = false
	f.ChangesError = "Recent commits not loaded"
	approved := false
	// Paginate oldest through newest, retaining every viewer review independently
	// of bounded review bodies used for notification prose.
	next := path + "/reviews?per_page=100"
	for page := 0; next != ""; page++ {
		if page >= 100 {
			f.Error = "review pagination limit reached; review history incomplete"
			return f
		}
		raw, headers, e := g.api(ctx, host, next)
		if e != nil {
			f.Error = "review history unavailable: " + e.Error()
			return f
		}
		var reviews []struct {
			ViewerReview
			User struct {
				Login string `json:"login"`
			} `json:"user"`
		}
		if !strings.HasPrefix(strings.TrimSpace(string(raw)), "[") || json.Unmarshal(raw, &reviews) != nil {
			f.Error = "invalid review history"
			return f
		}
		for _, r := range reviews {
			if strings.EqualFold(r.User.Login, viewer) {
				f.ViewerReviews = append(f.ViewerReviews, r.ViewerReview)
				if r.State == "APPROVED" {
					approved = true
					yes := true
					f.ViewerEverApproved = &yes
				}
			}
		}
		next = ""
		for _, part := range strings.Split(headers.Get("Link"), ",") {
			if strings.Contains(part, `rel="next"`) {
				target, _, ok := strings.Cut(strings.TrimSpace(part), ">")
				if !ok {
					f.Error = "invalid review pagination"
					return f
				}
				next, e = githubPath(host, strings.TrimPrefix(target, "<"))
				if e != nil || !strings.HasPrefix(next, path+"/reviews?") {
					f.Error = "invalid review pagination"
					return f
				}
			}
		}
	}
	f.ReviewsComplete = true
	f.ViewerEverApproved = &approved
	f.RecentCommits, f.ChangesError = g.recentPRCommits(ctx, host, path)
	return f
}

// Recent commit messages are untrusted GitHub content, distinct from identity
// and review facts. Fetch the last page, avoiding an unbounded diff payload.
func (g GitHub) recentPRCommits(ctx context.Context, host, path string) ([]PRCommit, string) {
	raw, headers, err := g.api(ctx, host, path+"/commits?per_page=10")
	if err != nil {
		return nil, err.Error()
	}
	for _, part := range strings.Split(headers.Get("Link"), ",") {
		if strings.Contains(part, `rel="last"`) {
			target, _, ok := strings.Cut(strings.TrimSpace(part), ">")
			if !ok {
				return nil, "invalid commits pagination"
			}
			last, e := githubPath(host, strings.TrimPrefix(target, "<"))
			if e != nil || !strings.HasPrefix(last, path+"/commits?") {
				return nil, "invalid commits pagination"
			}
			raw, _, err = g.api(ctx, host, last)
			if err != nil {
				return nil, err.Error()
			}
		}
	}
	var commits []PRCommit
	if json.Unmarshal(raw, &commits) != nil {
		return nil, "invalid recent commits"
	}
	for i := range commits {
		if commits[i].SHA == "" {
			return nil, "invalid recent commits"
		}
		message, _, _ := strings.Cut(commits[i].Commit.Message, "\n")
		if len(message) > 500 {
			message = message[:500]
		}
		commits[i].Commit.Message = strings.ToValidUTF8(message, "")
	}
	return commits, ""
}

package github

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"time"
)

// ListOpenPRs lists every open PR, including drafts, independently of the review queue.
func ListOpenPRs(ctx context.Context, repo, repoDir string) ([]PRSummary, string) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if !Available() {
		return nil, "no_gh"
	}
	cmd := exec.CommandContext(ctx, "gh", "api", "--paginate", "--slurp", "repos/"+repo+"/pulls?state=open&sort=updated&direction=desc&per_page=100")
	cmd.Dir = repoDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, classifyGhListError(err, stderr.String())
	}
	var pages [][]struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		User   struct {
			Login string `json:"login"`
		} `json:"user"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &pages); err != nil {
		return nil, "error"
	}
	prs := make([]PRSummary, 0)
	for _, page := range pages {
		for _, pr := range page {
			prs = append(prs, PRSummary{Number: pr.Number, Title: pr.Title, Author: pr.User.Login})
		}
	}
	return prs, "ok"
}

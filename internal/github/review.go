// Package github: review.go is the GitHub WRITE seam for PR reviews — it shells
// out to `gh api repos/{owner}/{repo}/pulls/{n}/reviews` the same way Clone
// shells out to `gh repo clone` and ViewPR shells out to `gh pr view`. This is
// the v1.3 no-writes REVERSAL landing point (server-side); the MCP bridge
// reaches it through the new /api/projects/{id}/pull-requests/{n}/reviews
// (PLURAL) endpoint over HTTP, NOT by importing this package.
//
// The verdict→event taxonomy (approve/request_changes/comment → GitHub's
// APPROVE/REQUEST_CHANGES/COMMENT) lives in exactly one place: inside
// PostReview. The API layer + MCP bridge hand PostReview the lowercase verdict;
// the map never leaks.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// InlineComment is the per-line-comment shape carried in the comments array of
// a posted review. JSON tags ARE the GitHub REST contract (path/line/body).
type InlineComment struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Body string `json:"body"`
}

// verdictToEvent maps the Kamacu verdict taxonomy (the agent-facing lowercase
// strings) to GitHub's UPPERCASE review events. Defined in exactly one place
// (this package); the API layer + MCP bridge pass the lowercase verdict
// verbatim and PostReview maps it here.
var verdictToEvent = map[string]string{
	"approve":         "APPROVE",
	"request_changes": "REQUEST_CHANGES",
	"comment":         "COMMENT",
}

// PostReview posts a PR review (approve / request_changes / comment, optionally
// with inline line comments) to GitHub via `gh api repos/{repo}/pulls/{n}/reviews
// --input -` with the JSON body on stdin. The verdict is mapped to the GitHub
// event inside this function (the taxonomy lives in exactly one place); an
// unknown verdict returns a non-nil error BEFORE any gh spawn.
//
// On success it returns gh's stdout JSON response ([]byte) so the API endpoint
// can pass it through as the 201 body. On gh absent (Available()==false), gh
// nonzero exit, or a stub failure, it returns (nil, error) — degrade-don't-break,
// mirroring ViewPR/PRState. It NEVER shell-interpolates: repo/prNumber ride an
// exec arg array; only the marshaled JSON body crosses stdin.
func PostReview(ctx context.Context, repo string, prNumber int, verdict, body string, comments []InlineComment) ([]byte, error) {
	event, ok := verdictToEvent[verdict]
	if !ok {
		return nil, fmt.Errorf("unknown verdict %q (want approve, request_changes, or comment)", verdict)
	}

	// Build the request payload as a map so omitempty drops empty comments and
	// an absent body — GitHub accepts a review with no body and no comments.
	payload := map[string]any{"event": event}
	if body != "" {
		payload["body"] = body
	}
	if len(comments) > 0 {
		cs := make([]map[string]any, len(comments))
		for i, c := range comments {
			cs[i] = map[string]any{"path": c.Path, "line": c.Line, "body": c.Body}
		}
		payload["comments"] = cs
	}

	jb, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal review body: %w", err)
	}
	return postReviewRunner(ctx, repo, prNumber, jb)
}

// postReviewRunner is the overridable seam for the actual review-posting exec,
// mirroring clone.go's cloneRunner. Production points at realPostReview; tests
// in this package swap in a fake to exercise PostReview's verdict mapping +
// body shape + error paths without a live gh or network (posting needs live
// GitHub, so the SUCCESS path is ALSO stub-driven — see review_test.go).
var postReviewRunner = realPostReview

// realPostReview is the production postReviewRunner: it shells out to `gh api`
// with an arg array (NEVER `sh -c` — the package invariant; repo is attacker-
// adjacent owner/name input, so no shell). Available-guarded; nonzero exit
// surfaces as an error whose message carries the trimmed stderr (a leading
// "fatal: " stripped, mirroring Clone's message shape).
func realPostReview(ctx context.Context, repo string, prNumber int, body []byte) ([]byte, error) {
	if !Available() {
		return nil, errors.New("the GitHub CLI (gh) is not installed")
	}
	apiPath := fmt.Sprintf("repos/%s/pulls/%d/reviews", repo, prNumber)
	cmd := exec.CommandContext(ctx, "gh", "api", apiPath, "--input", "-",
		"-H", "Accept: application/vnd.github+json", "-X", "POST")
	cmd.Stdin = bytes.NewReader(body)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		msg = strings.TrimPrefix(msg, "fatal: ")
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("couldn't post review to PR #%d: %s", prNumber, msg)
	}
	return out, nil
}

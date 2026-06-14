package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
)

// PRState reads only the PR's lifecycle state via `gh pr view <n> -R <repo>
// --json state` -> "OPEN" | "CLOSED" | "MERGED" (UPPERCASE — verified; there is
// NO `merged` field, derive merged from state == "MERGED"). It is the cheap
// per-tick read the reaper's PRStateGetter calls (D-02) — one fewer JSON field
// than ViewPR, since the reaper needs only the string. Degrades like ViewPR:
// gh absent or a nonzero exit -> ("", err), so the reaper warn-and-skips (D-03)
// rather than crashing the tick.
//
// It is a method on *Service (not the package) so it satisfies the reaper's
// local PRStateGetter interface and reuses the same *github.Service already
// wired in main.go — no new construction.
func (s *Service) PRState(ctx context.Context, repo string, n int) (string, error) {
	if !Available() {
		return "", errors.New("the GitHub CLI (gh) is not installed")
	}
	out, err := exec.CommandContext(ctx, "gh", "pr", "view", strconv.Itoa(n),
		"-R", repo, "--json", "state").Output()
	if err != nil {
		return "", fmt.Errorf("couldn't read PR #%d state", n)
	}
	var resp struct {
		State string `json:"state"`
	}
	if jsonErr := json.Unmarshal(out, &resp); jsonErr != nil {
		return "", fmt.Errorf("couldn't parse PR #%d state", n)
	}
	return resp.State, nil
}

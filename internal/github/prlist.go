package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os/exec"
	"sort"
	"strings"
)

// PRSummary is the per-PR wire shape returned (via Service.Get) to the
// /api/projects/{id}/pull-requests endpoint. The JSON tags ARE the contract:
// the frontend renders these fields verbatim. The list call fetches a RICH set
// (D-01/D-02) but Phase 11 renders only number/title/author/updatedAt/url plus
// the reduced Checks pill; the HeadRef*/BaseRef*/IsCrossRepository fields are
// fetched-but-unrendered for Phase 12/13 consumption (one call, no re-fetch).
// The raw statusCheckRollup is NEVER carried here — it is reduced server-side
// to a single pass|fail|pending|none value (D-00d).
type PRSummary struct {
	Number    int    `json:"number"`
	Title     string `json:"title"`
	Author    string `json:"author"`    // author.login flattened
	UpdatedAt string `json:"updatedAt"` // ISO 8601 passthrough
	URL       string `json:"url"`
	Checks    string `json:"checks"` // "pass" | "fail" | "pending" | "none" (reduced server-side, D-00d)
	// Fetched for Phase 12/13, NOT rendered in Phase 11 (D-02):
	HeadRefName       string `json:"headRefName"`
	HeadRefOid        string `json:"headRefOid"`
	BaseRefName       string `json:"baseRefName"`
	IsCrossRepository bool   `json:"isCrossRepository"`
}

// checkEntry is the lenient decode shape for ONE statusCheckRollup element.
// The rollup mixes two __typename variants: CheckRun (GitHub Actions / Checks
// API — uses status+conclusion) and StatusContext (legacy commit statuses,
// e.g. Prow tide / EasyCLA — uses state, no conclusion). Both decode into this
// one struct; reduceChecks branches on Typename.
//
// The rollup contains EVERY historical run of a check — original runs,
// re-runs, AND concurrency-cancelled attempts — so identity (Name/Context) and
// run timestamps (StartedAt/CompletedAt/CreatedAt) are decoded too. GitHub's
// own rollup state keeps only the LATEST run per check name; latestPerCheck
// reproduces that before reduceChecks tallies, so a stale superseded
// FAILURE/CANCELLED entry does not paint the dot red. No change to the
// `gh … --json statusCheckRollup` flags is needed: these sub-fields already
// ride along (CheckRun carries name/startedAt/completedAt; StatusContext
// carries context/createdAt).
type checkEntry struct {
	Typename    string `json:"__typename"`
	Status      string `json:"status"`     // CheckRun
	Conclusion  string `json:"conclusion"` // CheckRun
	State       string `json:"state"`      // StatusContext
	Name        string `json:"name"`        // CheckRun identity
	Context     string `json:"context"`     // StatusContext identity
	StartedAt   string `json:"startedAt"`   // CheckRun run time
	CompletedAt string `json:"completedAt"` // CheckRun run time
	CreatedAt   string `json:"createdAt"`   // StatusContext run time
}

// identity collapses re-runs: gh returns every historical run of a check, but
// GitHub's rollup state keeps only the latest run per name. CheckRun keys on
// name, StatusContext on context (kept in separate namespaces).
func (c checkEntry) identity() string {
	if c.Typename == "StatusContext" {
		return "ctx:" + c.Context
	}
	return "run:" + c.Name
}

// ranAt is the run's ordering timestamp (ISO 8601 sorts lexically == chrono).
// A later run supersedes earlier ones of the same identity.
func (c checkEntry) ranAt() string {
	if c.StartedAt != "" {
		return c.StartedAt
	}
	if c.CompletedAt != "" {
		return c.CompletedAt
	}
	return c.CreatedAt // StatusContext
}

// latestPerCheck keeps only the most recent run of each check identity. Without
// it a stale superseded FAILURE/CANCELLED entry (from a re-run or a
// concurrency-cancelled attempt) paints the dot red even though CI is green —
// e.g. seqeralabs/fusion #1461 (old "Unit tests" FAILURE + re-run SUCCESS) and
// #1459 (every job CANCELLED by concurrency + a SUCCESS re-run) are both green
// on GitHub but reduced to "fail" before this fix.
//
// The returned slice order is nondeterministic (map iteration); that is fine
// because reduceChecks only ORs booleans over it — do NOT add anything that
// depends on the order here.
func latestPerCheck(rollup []checkEntry) []checkEntry {
	latest := make(map[string]checkEntry, len(rollup))
	for _, c := range rollup {
		if prev, ok := latest[c.identity()]; !ok || c.ranAt() > prev.ranAt() {
			latest[c.identity()] = c
		}
	}
	out := make([]checkEntry, 0, len(latest))
	for _, c := range latest {
		out = append(out, c)
	}
	return out
}

// prRaw is the lenient decode shape for ONE PR from `gh pr list --json …`.
// gh's --json output decodes into this (NOT into PRSummary directly): author
// is an object, the rollup is the raw array, and isDraft is fetched for a
// belt-and-suspenders server-side draft filter on top of the draft:false
// search term.
type prRaw struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Author struct {
		Login string `json:"login"`
	} `json:"author"`
	UpdatedAt         string       `json:"updatedAt"`
	URL               string       `json:"url"`
	IsDraft           bool         `json:"isDraft"`
	StatusCheckRollup []checkEntry `json:"statusCheckRollup"`
	HeadRefName       string       `json:"headRefName"`
	HeadRefOid        string       `json:"headRefOid"`
	BaseRefName       string       `json:"baseRefName"`
	IsCrossRepository bool         `json:"isCrossRepository"`
}

// reduceChecks collapses a statusCheckRollup array to one of
// "pass" | "fail" | "pending" | "none" (D-00d, D-03, D-04). The per-entry
// signal mapping is verified live against gh 2.82.0 (both __typename variants
// observed in cli/cli and kubernetes/kubernetes). The critical detail
// (Pitfall 1): SKIPPED/NEUTRAL/STALE map to NON-failing — real PRs are full
// of them, and treating them as failures would paint nearly every dot red.
// Aggregate precedence is fail > pending > pass.
//
// Before tallying, latestPerCheck collapses superseded re-runs: gh returns
// every historical run of a check (original, re-run, concurrency-cancelled),
// and GitHub's own rollup state keeps only the latest run per name. Skipping
// this dedup lets one stale superseded FAILURE/CANCELLED entry paint the dot
// red even when CI is green (seqeralabs/fusion #1461, #1459).
func reduceChecks(rollup []checkEntry) string {
	if len(rollup) == 0 {
		return "none" // render nothing (D-04)
	}
	rollup = latestPerCheck(rollup) // collapse superseded re-runs (latest run per name wins)
	anyFail, anyPending := false, false
	for _, c := range rollup {
		var sig string // "ok" | "pending" | "fail"
		if c.Typename == "StatusContext" {
			switch c.State {
			case "SUCCESS":
				sig = "ok"
			case "PENDING", "EXPECTED":
				sig = "pending"
			default: // FAILURE, ERROR
				sig = "fail"
			}
		} else { // CheckRun
			if c.Status != "COMPLETED" {
				sig = "pending" // QUEUED/IN_PROGRESS/WAITING/PENDING/REQUESTED
			} else {
				switch c.Conclusion {
				case "SUCCESS", "NEUTRAL", "SKIPPED", "STALE":
					sig = "ok" // non-failing — do NOT turn the dot red (Pitfall 1)
				case "":
					sig = "pending" // defensive: completed-but-no-conclusion
				default: // FAILURE, TIMED_OUT, CANCELLED, ACTION_REQUIRED, STARTUP_FAILURE
					sig = "fail"
				}
			}
		}
		switch sig {
		case "fail":
			anyFail = true
		case "pending":
			anyPending = true
		}
	}
	switch {
	case anyFail:
		return "fail"
	case anyPending:
		return "pending"
	default:
		return "pass"
	}
}

// runGH is the single I/O primitive of Phase 11's backend: it shells out to
// `gh pr list` for the review-requested queue and classifies the outcome into
// a typed state. It NEVER spawns when gh is absent (returns "no_gh") and NEVER
// shell-interpolates — the command is an arg array with cmd.Dir set so gh
// resolves the right host/account (Pitfall 6 / D-00b). The return contract mirrors the
// quota fetcher: (prs, state, err) where state is one of
// "ok"|"no_gh"|"auth_required"|"error" and err is always nil for classified
// degrades (the Service switches on state, never on err).
func runGH(ctx context.Context, repo, repoDir string) (prs []PRSummary, state string, _ error) {
	if !Available() {
		return nil, "no_gh", nil // never spawn (D-00c)
	}
	cmd := exec.CommandContext(ctx, "gh", "pr", "list",
		"-R", repo,
		"--search", "user-review-requested:@me draft:false",
		"--state", "open",
		"--json", "number,title,author,updatedAt,url,isDraft,statusCheckRollup,headRefName,headRefOid,baseRefName,isCrossRepository,additions,deletions",
	)
	cmd.Dir = repoDir // D-00b / Pitfall 6: selects the right gh host/account
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	runErr := cmd.Run()
	if runErr != nil {
		es := stderr.String()
		var exitErr *exec.ExitError
		code := -1
		if errors.As(runErr, &exitErr) {
			code = exitErr.ExitCode()
		}
		// Exit code 4 ALONE is unreliable (cli/cli#9338) — combine the code
		// AND stderr substring sniffing (Pitfall 4). Never log the stderr body.
		if code == 4 ||
			strings.Contains(es, "gh auth login") ||
			strings.Contains(es, "401") ||
			strings.Contains(es, "Bad credentials") {
			slog.Debug("github pr list degraded", "state", "auth_required")
			return nil, "auth_required", nil
		}
		slog.Debug("github pr list degraded", "state", "error")
		return nil, "error", nil
	}

	var raws []prRaw
	if err := json.Unmarshal(stdout.Bytes(), &raws); err != nil {
		slog.Debug("github pr list degraded", "state", "error")
		return nil, "error", nil
	}

	out := make([]PRSummary, 0, len(raws))
	for _, r := range raws {
		if r.IsDraft {
			continue // belt-and-suspenders vs the draft:false search term
		}
		out = append(out, PRSummary{
			Number:            r.Number,
			Title:             r.Title,
			Author:            r.Author.Login,
			UpdatedAt:         r.UpdatedAt,
			URL:               r.URL,
			Checks:            reduceChecks(r.StatusCheckRollup),
			HeadRefName:       r.HeadRefName,
			HeadRefOid:        r.HeadRefOid,
			BaseRefName:       r.BaseRefName,
			IsCrossRepository: r.IsCrossRepository,
		})
	}
	// Most-recently-updated first (D-14). ISO 8601 strings sort lexically the
	// same as chronologically, so a string compare is correct here.
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt > out[j].UpdatedAt
	})
	return out, "ok", nil
}

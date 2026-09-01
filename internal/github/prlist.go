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

// The two review-queue searches that drive the column's two sections (D-12).
// Both ride ONE cached Service cycle via fetchLists; the only thing that
// differs between them is this --search argument.
const (
	searchAwaiting = "user-review-requested:@me draft:false" // top section (existing)
	searchReviewed = "reviewed-by:@me draft:false"           // Recently reviewed (new, D-12)
)

// listPRs is the single I/O primitive of the backend: it shells out to
// `gh pr list` for ONE search and classifies the outcome into a typed state.
// It NEVER spawns when gh is absent (returns "no_gh") and NEVER
// shell-interpolates — the command is an arg array with cmd.Dir set so gh
// resolves the right host/account (Pitfall 6 / D-00b). The return contract
// mirrors the quota fetcher: (prs, state, err) where state is one of
// "ok"|"no_gh"|"auth_required"|"error" and err is always nil for classified
// degrades (the Service switches on state, never on err).
//
// search is the --search argument (searchAwaiting or searchReviewed). Every
// other concern — the --json field set, the belt-and-suspenders draft filter,
// reduceChecks, the UpdatedAt sort, and the auth/error classification — is
// identical for both queues, so it lives here exactly once.
func listPRs(ctx context.Context, repo, repoDir, search string) (prs []PRSummary, state string, _ error) {
	if !Available() {
		return nil, "no_gh", nil // never spawn (D-00c)
	}
	cmd := exec.CommandContext(ctx, "gh", "pr", "list",
		"-R", repo,
		"--search", search,
		"--state", "open",
		"--json", "number,title,author,updatedAt,url,isDraft,statusCheckRollup,headRefName,headRefOid,baseRefName,isCrossRepository,additions,deletions",
	)
	cmd.Dir = repoDir // D-00b / Pitfall 6: selects the right gh host/account
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	runErr := cmd.Run()
	if runErr != nil {
		// Shared classification (Pattern 3) — listCompletedReviews uses the
		// exact same sniff so the two fetchers never drift (Pitfall 4).
		// T-10-03: log the classified state only, NEVER the stderr body.
		state := classifyGhListError(runErr, stderr.String())
		slog.Debug("github pr list degraded", "state", state)
		return nil, state, nil
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
	// Most-recently-updated first (D-10/D-14). ISO 8601 strings sort lexically
	// the same as chronologically, so a string compare is correct here.
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt > out[j].UpdatedAt
	})
	return out, "ok", nil
}

// PRLists carries the two review queues fetched in one cache cycle (D-12):
// Awaiting (user-review-requested:@me) and Reviewed (reviewed-by:@me, already
// deduped against Awaiting). The Service caches BOTH under one repoEntry, one
// fetchedAt, one TTL/floor — so the column's two sections always reflect the
// same gh snapshot.
type PRLists struct {
	Awaiting []PRSummary // user-review-requested:@me (top section)
	Reviewed []PRSummary // reviewed-by:@me, deduped against Awaiting (D-14)
}

// fetchLists is the production runner the Service calls: it runs BOTH searches
// in one cycle and applies top-precedence dedup (D-14). If EITHER search
// degrades, the whole cycle reports that degraded state with both lists empty —
// never a partial result, so a single gh failure degrades both sections
// together (REVWD-04/D-12). err is always nil; the caller switches on state.
func fetchLists(ctx context.Context, repo, repoDir string) (PRLists, string, error) {
	awaiting, st, _ := listPRs(ctx, repo, repoDir, searchAwaiting)
	if st != "ok" {
		return PRLists{}, st, nil
	}
	reviewed, st2, _ := listPRs(ctx, repo, repoDir, searchReviewed)
	if st2 != "ok" {
		return PRLists{}, st2, nil
	}
	reviewed = dedupeReviewed(awaiting, reviewed)
	return PRLists{Awaiting: awaiting, Reviewed: reviewed}, "ok", nil
}

// dedupeReviewed drops any reviewed PR whose Number appears in awaiting — the
// top "awaiting your review" section takes precedence (REVWD-03/D-14), so a
// re-requested PR shows up there and never duplicates in Recently reviewed.
// The surviving reviewed order is preserved (it was already
// most-recently-updated-first from listPRs).
func dedupeReviewed(awaiting, reviewed []PRSummary) []PRSummary {
	seen := make(map[int]struct{}, len(awaiting))
	for _, p := range awaiting {
		seen[p.Number] = struct{}{}
	}
	out := make([]PRSummary, 0, len(reviewed))
	for _, p := range reviewed {
		if _, dup := seen[p.Number]; dup {
			continue
		}
		out = append(out, p)
	}
	return out
}

// ReviewDoneSummary is the per-completed-PR wire shape for the activity
// endpoint's reviews-done list (REVIEWS-01, D-06). It carries only what the
// list renders: the gh-sourced identity/timestamp/URL plus provenance fields
// the Plan 02 handler annotates at aggregation time (PRSummary has no
// repo/project field, so provenance is added by the loop that knows which
// project each repo belongs to). GetMergedClosed (Task 2) leaves the three
// provenance fields at their zero values. PRSummary — the review-column wire
// type — is intentionally untouched (D-06: a dedicated type keeps each wire
// focused).
type ReviewDoneSummary struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	CompletedAt string `json:"completedAt"` // ISO 8601; closedAt from gh, set for BOTH merged+closed (D-05)
	URL         string `json:"url"`
	ProjectName string `json:"projectName"` // annotated by the Plan 02 aggregation loop
	ProjectID   int64  `json:"projectId"`   // annotated by the Plan 02 aggregation loop
	Repo        string `json:"repo"`        // annotated by the Plan 02 aggregation loop
}

// searchCompletedReviews is the activity endpoint's reviews-done search arg
// (D-05). The is:closed SEARCH QUALIFIER is the AUTHORITATIVE merged+closed
// filter — it stably includes merged per GitHub's search docs (discussion
// #5599). --state closed is kept too (see listCompletedReviews) for
// belt-and-suspenders, but it is a filed unfixed bug (cli/cli #8102) that a
// future gh could "fix" to EXCLUDE merged; the qualifier is the stable
// contract. draft:false drops never-published drafts from the completed set.
const searchCompletedReviews = "reviewed-by:@me draft:false is:closed"

// completedRaw is the lenient decode shape for ONE entry from
// `gh pr list --json number,title,closedAt,url`. A MERGE IS a close, so
// closedAt is set for BOTH merged and closed PRs (D-05); mergedAt would be
// merged-only and is intentionally NOT fetched. The four fields are exactly
// what ReviewDoneSummary's gh-sourced half needs.
type completedRaw struct {
	Number   int    `json:"number"`
	Title    string `json:"title"`
	ClosedAt string `json:"closedAt"`
	URL      string `json:"url"`
}

// classifyGhListError maps a non-nil gh pr list failure (the runErr from
// cmd.Run plus the captured stderr) to the wire state "auth_required" or
// "error". Exit code 4 ALONE is unreliable (cli/cli#9338), so the call
// combines the code AND stderr-substring sniffing — identical sniffing for
// listPRs and listCompletedReviews, one path, no drift (Pitfall 4 / Pattern 3).
// Pure: no I/O, no logging — the caller slog.Debug's the returned state with a
// `state` attr only (T-10-03: never log the stderr body).
func classifyGhListError(runErr error, stderr string) string {
	var exitErr *exec.ExitError
	code := -1
	if errors.As(runErr, &exitErr) {
		code = exitErr.ExitCode()
	}
	// Exit code 4 ALONE is unreliable (cli/cli#9338) — combine the code AND
	// stderr substrings, matching the historically-inlined listPRs sniff.
	if code == 4 ||
		strings.Contains(stderr, "gh auth login") ||
		strings.Contains(stderr, "401") ||
		strings.Contains(stderr, "Bad credentials") {
		return "auth_required"
	}
	return "error"
}

// parseCompletedReviews decodes gh's `--json number,title,closedAt,url` output
// into ReviewDoneSummary values with CompletedAt mapped from closedAt, sorted
// most-recently-completed first. Pure (no I/O) so the ok-path transformation
// is unit-testable with a canned fixture. ISO 8601 strings sort lexically the
// same as chronologically, so a string compare is correct (same property
// listPRs relies on for UpdatedAt).
func parseCompletedReviews(stdout []byte) ([]ReviewDoneSummary, error) {
	var raws []completedRaw
	if err := json.Unmarshal(stdout, &raws); err != nil {
		return nil, err
	}
	out := make([]ReviewDoneSummary, 0, len(raws))
	for _, r := range raws {
		out = append(out, ReviewDoneSummary{
			Number:      r.Number,
			Title:       r.Title,
			CompletedAt: r.ClosedAt, // A merge IS a close, so closedAt is set for BOTH (D-05).
			URL:         r.URL,
			// Provenance fields stay zero — annotated by the Plan 02 aggregation loop (D-06).
		})
	}
	// Most-recently-completed first. ISO 8601 strings sort lexically the same
	// as chronologically (the property listPRs relies on for UpdatedAt).
	sort.Slice(out, func(i, j int) bool {
		return out[i].CompletedAt > out[j].CompletedAt
	})
	return out, nil
}

// listCompletedReviews is the I/O primitive for the activity endpoint's
// reviews-done list (REVIEWS-01, D-05). It mirrors listPRs structurally —
// arg-array spawn (never shell-interpolated), cmd.Dir selects the gh
// host/account, the outcome is classified into
// "ok"|"no_gh"|"auth_required"|"error" via the SHARED classifyGhListError
// helper — but targets the merged+closed set:
//
//   - --search is searchCompletedReviews (reviewed-by:@me + draft:false +
//     is:closed; the is:closed qualifier is the authoritative merged+closed
//     filter, NOT --state closed which is a filed unfixed bug).
//   - --state is the literal "closed" (belt-and-suspenders with is:closed,
//     consistent with listPRs's --state open pattern).
//   - --limit 100 avoids the default-30 truncation that would silently drop a
//     weeks/months review history (Pitfall 1).
//   - --json is number,title,closedAt,url (closedAt maps to CompletedAt; NO
//     statusCheckRollup/isDraft/ref fields — the completed set renders no
//     checks pill and needs no draft filter).
//
// err is always nil for classified degrades; the caller switches on state,
// never on err (the listPRs contract).
func listCompletedReviews(ctx context.Context, repo, repoDir string) (prs []ReviewDoneSummary, state string, _ error) {
	if !Available() {
		return nil, "no_gh", nil // never spawn (D-00c)
	}
	cmd := exec.CommandContext(ctx, "gh", "pr", "list",
		"-R", repo,
		"--search", searchCompletedReviews,
		"--state", "closed",
		"--limit", "100",
		"--json", "number,title,closedAt,url",
	)
	cmd.Dir = repoDir // D-00b / Pitfall 6: selects the right gh host/account
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	runErr := cmd.Run()
	if runErr != nil {
		state := classifyGhListError(runErr, stderr.String())
		// T-10-03: log the classified state only, NEVER the stderr body.
		slog.Debug("github pr list degraded", "state", state)
		return nil, state, nil
	}
	out, err := parseCompletedReviews(stdout.Bytes())
	if err != nil {
		slog.Debug("github pr list degraded", "state", "error")
		return nil, "error", nil
	}
	return out, "ok", nil
}

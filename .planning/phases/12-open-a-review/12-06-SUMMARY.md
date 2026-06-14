---
phase: 12-open-a-review
plan: 06
subsystem: backend
tags: [go, git-worktree, github-pr, gh-pr-view, settings-kv, gap-closure]

# Dependency graph
requires:
  - phase: 12-open-a-review
    provides: "12-04 open-or-reattach endpoint POST .../{n}/review with CheckoutPR + ViewPR + the {task, pr} envelope"
  - phase: 12-open-a-review
    provides: "12-05 frontend wiring whose human-verify surfaced the 5 follow-ups this plan closes (backend half)"
provides:
  - "CheckoutPR(ctx, repo, path, headOID, headRefName, prNumber) — provisions the PR worktree on a NAMED branch (headRefName) with a pr/<n> safe collision fallback (GHREV-01) that never reuses/moves an existing local ref (GHREV-05)"
  - "github.PRDetail.Commits (count of gh's commits array) + ViewPR fetching the commits field — the merge-line commit count"
  - "prWire.headRefName + prWire.commits in the open endpoint {task, pr} envelope — the GitHub-style merge line data for 12-07"
  - "settings.KeyPRReviewSeed (pr_review_seed) with the canonical default prompt, free-text length-capped validation, served through the existing GET getAll / PUT {key} surface — no new endpoint, no migration"
affects: [12-07-gap-frontend]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Named-branch PR checkout with a never-reuse selection rule: branchExistsLocally guards each candidate (headRefName -> pr/<n> -> pr/<n>-<shortOID> -> error), and worktree add -b creates+checks-out the chosen name in the NEW worktree only, so the primary checkout HEAD never moves (GHREV-05 preserved while DECISION-OVERRIDING D-01 detached)"
    - "Derived-count decode: PRDetail.Commits is json:\"-\" (gh ships commits as an array, not a scalar); ViewPR's raw struct adds Commits []struct{} json:\"commits\" and sets d.Commits = len(raw.Commits) — tolerant of an absent/empty array (0, no panic)"
    - "Settings key added with ZERO endpoint/migration change: an absent KV row reads as the code default, getAll iterates Defaults, PUT {key} validates via a new Validate case — the whole REST surface lights up for free"

key-files:
  created: []
  modified:
    - "internal/worktree/worktree.go - CheckoutPR signature gains headRefName; --detach replaced by named-branch selection (+ branchExistsLocally helper)"
    - "internal/worktree/worktree_test.go - TestCheckoutPRDetachedHead -> TestCheckoutPRNamedBranch; +TestCheckoutPRCollisionFallsBackToPRBranch; TestCheckoutPRCreatesNoBranch -> TestCheckoutPRBranchInNewWorktreeOnly; GHREV-05 source-HEAD test + path-collision test updated to the 6-arg signature"
    - "internal/github/github.go - PRDetail.Commits field; commits added to the gh pr view --json set; ViewPR counts the commits array"
    - "internal/github/github_test.go - ghViewFixture gains a 3-element commits array; viewPRRaw gains Commits; decode test asserts Commits==3 and HeadRefName==gh-pr"
    - "internal/api/pullrequests.go - CheckoutPR call site passes detail.HeadRefName; prWire gains headRefName + commits; writeReview populates both"
    - "internal/settings/settings.go - KeyPRReviewSeed const + Defaults entry (canonical prompt)"
    - "internal/settings/validate.go - KeyPRReviewSeed free-text length-capped Validate case"
    - "internal/settings/settings_test.go - KeyPRReviewSeed in TestDefaultsValues want map (keeps the count check correct)"
    - "internal/settings/validate_test.go - TestValidatePRReviewSeed (free-text/empty/too-long) + strings import"

key-decisions:
  - "DECISION OVERRIDE of D-01 (detached) -> named branch: CheckoutPR now creates the PR's REAL head branch (headRefName) via worktree add -b, realigning with the literal GHREV-01 wording flagged at the 12-05 human-verify. GHREV-05 is PRESERVED by the never-reuse selection rule (a name that already exists locally is never chosen) + the pr/<n> fallback, so the fork same-name 'master' trap can never move the project's primary checkout."
  - "Collision fallback order is headRefName -> pr/<n> -> pr/<n>-<short headOID> -> clear error; the short-OID leg gives double-collision resilience without a migration or a stateful counter."
  - "PRDetail.Commits is json:\"-\" and derived from len(raw.Commits) because gh returns commits as an array of objects, not a count scalar; the empty/absent array decodes to 0 with no panic (tolerant decode)."
  - "pr_review_seed is a free-text settings key (length-capped 2000) interpolated on the FRONTEND (12-07), stored literally with <n>/<title> placeholders; empty is a real value meaning 'no injection' downstream. No new endpoint, no migration (absent row = code default)."

patterns-established:
  - "When a checkout must occupy a named branch but must never disturb existing refs, pre-check each candidate with show-ref --verify --quiet and only ever pass a non-existent name to worktree add -b — the safety guarantee comes from the selection, not from --force or branch deletion."

requirements-completed: []  # GHREV-01/05 stay Pending until 12-07 lands and the orchestrator re-runs phase verification.

# Metrics
duration: 7min
completed: 2026-06-14
---

# Phase 12 Plan 06: Open-a-Review Gap Closure (Backend) Summary

**The PR review worktree now checks out the PR's REAL head branch (headRefName) with a safe pr/<n> collision fallback instead of a detached HEAD — realigning with GHREV-01 while preserving the GHREV-05 primary-checkout safety via a never-reuse selection rule — and the open endpoint now carries the head branch name + commit count plus a new pr_review_seed settings key, wiring the backend data the 12-07 merge line and configurable-seed field will consume.**

## Status: all three tasks done, build + tests green

The three backend follow-ups (named-branch checkout, commits/head in the open response, the pr_review_seed key) are implemented, individually committed, and pass `go build ./...` plus all four affected package suites. The phase stays **unverified** and GHREV-01..05 remain **Pending** until the frontend gap plan (12-07) lands and the orchestrator re-runs phase verification.

## Performance

- **Duration:** ~7 min
- **Completed:** 2026-06-14
- **Tasks:** 3 of 3
- **Files modified:** 9

## Accomplishments

- **Named-branch PR checkout (GHREV-01, DECISION OVERRIDE of D-01):** `CheckoutPR` gains a `headRefName` parameter and replaces `worktree add --detach` with named-branch selection. The branch name is chosen by an EXACT never-reuse rule — `headRefName` if free, else `pr/<n>`, else `pr/<n>-<short headOID>`, else a clear error — and `worktree add -b <chosen>` creates AND checks out that branch in the NEW worktree only, pinned to `headOID`. A new `branchExistsLocally` helper (`show-ref --verify --quiet refs/heads/<name>`) is the guard that keeps the selection from ever reusing or moving an existing local ref.
- **GHREV-05 safety preserved + proven:** `TestCheckoutPRLeavesSourceHeadUnchanged` (the source-HEAD-unchanged proof) is kept and passes; a NEW `TestCheckoutPRCollisionFallsBackToPRBranch` pre-creates a local `master`, opens a PR whose head is `master`, and asserts the worktree lands on `pr/8` while the local `master` ref is byte-for-byte untouched (the fork same-name trap).
- **Commits count + head branch in the open response (merge-line data):** `PRDetail.Commits` (derived from the length of gh's `commits` array, `json:"-"`), `commits` added to the `gh pr view --json` field set, and `prWire.headRefName` + `prWire.commits` populated in `writeReview` on BOTH the create and reattach paths — so 12-07 can render `<author> wants to merge <N> commits into <base> from <head>`.
- **Open endpoint provisions on the named branch:** the `CheckoutPR` call site now passes `detail.HeadRefName` (the key link), so opening a review actually checks out the PR's branch.
- **`pr_review_seed` settings key:** `KeyPRReviewSeed` const + Defaults entry with the canonical prompt (`Review PR #<n> "<title>". Summarize the changes, then flag bugs, risky changes, and missing tests.`), a free-text length-capped (2000) `Validate` case, served through the existing GET getAll / PUT {key} surface with no new endpoint and no migration.

## Task Commits

1. **Task 1: Named-branch PR checkout with safe collision fallback** — `a1ea153` (feat)
2. **Task 2: Commits count + head branch in PRDetail and the open response** — `c211bfb` (feat)
3. **Task 3: pr_review_seed settings key via the existing KV store** — `a08a468` (feat)

**Plan metadata:** this SUMMARY + STATE.md + ROADMAP.md (docs commit).

## Files Created/Modified

- `internal/worktree/worktree.go` — `CheckoutPR` 6-arg signature (+`headRefName`), `--detach` replaced by the named-branch selection rule, `branchExistsLocally` helper, doc comment updated (drops the DETACHED framing, keeps the GHREV-05 + scoped-fetch notes).
- `internal/worktree/worktree_test.go` — `TestCheckoutPRDetachedHead` → `TestCheckoutPRNamedBranch` (on the named branch, not detached); `TestCheckoutPRCreatesNoBranch` → `TestCheckoutPRBranchInNewWorktreeOnly`; new `TestCheckoutPRCollisionFallsBackToPRBranch`; all CheckoutPR call sites updated to the 6-arg signature.
- `internal/github/github.go` — `PRDetail.Commits` (`json:"-"`); `commits` in the `--json` set; `d.Commits = len(raw.Commits)`.
- `internal/github/github_test.go` — `ghViewFixture` 3-element commits array; `viewPRRaw.Commits`; decode test asserts `Commits==3` + `HeadRefName=="gh-pr"`.
- `internal/api/pullrequests.go` — `CheckoutPR(..., detail.HeadRefName, ...)`; `prWire.headRefName` + `prWire.commits`; populated in `writeReview`.
- `internal/settings/settings.go` — `KeyPRReviewSeed` const + Defaults entry.
- `internal/settings/validate.go` — `KeyPRReviewSeed` free-text length-capped Validate case.
- `internal/settings/settings_test.go` — `KeyPRReviewSeed` in `TestDefaultsValues` want map.
- `internal/settings/validate_test.go` — `TestValidatePRReviewSeed` + `strings` import.

## Decisions Made

- **DECISION OVERRIDE of D-01:** detached → named branch. The 12-05 human-verify flagged that the detached HEAD did not match the user's expectation of seeing the PR's named branch (the literal GHREV-01 wording: "the PR's branch checked out"). `CheckoutPR` now creates the named head branch. GHREV-05 is preserved because the selection NEVER chooses a name that already exists locally — the collision fallback (`pr/<n>`, then `pr/<n>-<shortOID>`) protects the fork same-name case, and `worktree add -b` only ever touches the new worktree.
- **Commits is a derived count** (`json:"-"` + `len(raw.Commits)`), not a scalar field, because gh returns `commits` as an array of objects; the empty/absent array yields 0 with no panic.
- **pr_review_seed is free-text, frontend-interpolated, migration-free** — an absent KV row reads as the code default, so the key flows through the existing settings REST surface with zero handler change.

## Deviations from Plan

None - plan executed exactly as written. No bugs, missing functionality, or blocking issues surfaced; no architectural decisions required.

## Issues Encountered

- None. The `validate_test.go` `strings` import was added as part of the planned `TestValidatePRReviewSeed` (the test uses `strings.Repeat`); this is expected test scaffolding, not a deviation.

## Next Phase Readiness

- **Backend data for the two remaining 12-07 features is in place:** the merge line (`prWire.headRefName` + `prWire.commits`) and the configurable seed (`pr_review_seed` settings key). 12-07 (frontend) must: fix the PR-view terminal fit (follow-up #2), make the seed inject once-per-review (follow-up #3), add a Settings field bound to `pr_review_seed` and interpolate `<n>`/`<title>` into the seed (follow-up #4), and render the merge line from the new `pr` wire fields (follow-up #5).
- **Do NOT** mark the phase complete or touch REQUIREMENTS.md GHREV statuses here — GHREV-01..05 remain Pending until 12-07 lands and the orchestrator re-runs phase verification.
- No new Go dependency, no new npm dependency, no new DB migration.

## Self-Check: PASSED

- All 9 modified files exist on disk (verified below).
- Commits `a1ea153` (Task 1), `c211bfb` (Task 2), `a08a468` (Task 3) present in git history.
- `go build ./...` passes; `go test ./internal/worktree/ ./internal/github/ ./internal/api/ ./internal/settings/` all pass.
- GHREV-05 preserved: `--detach` removed from CheckoutPR (grep count 0), `TestCheckoutPRLeavesSourceHeadUnchanged` still passes, and the new collision test proves the existing local ref is never reused/moved.

---
*Phase: 12-open-a-review*
*Completed (backend gap closure): 2026-06-14 — phase still unverified, frontend gap plan 12-07 pending*

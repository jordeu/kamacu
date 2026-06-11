---
phase: 05-recovery-review
plan: 02
subsystem: api
tags: [git, diff, merge-base, numstat, unified-diff, rest, go]

# Dependency graph
requires:
  - phase: 03-worktree-lifecycle
    provides: worktree.Service.ResolveBase (base re-resolution) + gitRun error-shaping pattern
  - phase: 03-worktree-lifecycle
    provides: loadTaskRepo single-query task+repo_path convention, writeError/writeJSON helpers, pathID
provides:
  - internal/diff package (Compute pipeline + numstat/unified-patch parsers + no-index exit-1 runner)
  - GET /api/tasks/{id}/diff endpoint returning structured JSON (Pattern 4 contract)
  - DiffRoutes registration in cmd/kangent/main.go
affects: [05-04 DiffTab frontend (renders this exact JSON shape), 05-recovery-review]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Diff package owns its OWN git runner (run + runNoIndex) — exit-0 contract for normal git, exit-1-as-success only for --no-index untracked files (Pitfall 1)"
    - "numstat -z parsed NUL-first then TAB-split; rename records consume two extra NUL tokens to avoid desync (Pitfall 2)"
    - "Unified-diff sections split line-anchored on 'diff --git '; paths seeded from the header so mode-only/binary sections still carry a path"
    - "Three-dot semantics via merge-base: tracked diff vs merge-base, untracked appended via no-index, all sorted by path"

key-files:
  created:
    - internal/diff/parse.go
    - internal/diff/parse_test.go
    - internal/diff/diff.go
    - internal/diff/diff_test.go
    - internal/api/diffs.go
    - internal/api/diffs_test.go
  modified:
    - cmd/kangent/main.go

key-decisions:
  - "Diff package keeps a private copy of the gitRun pattern rather than importing worktree's unexported one — the untracked path needs the exit-1 contract worktree's runner must never grant"
  - "numstat is the single source of truth for tracked-file additions/deletions; untracked file stats are counted from the parsed no-index patch (numstat doesn't cover untracked)"
  - "Patch-section path is seeded from the 'diff --git a/p b/p' header (b-side) so mode-only and binary sections — which carry no ---/+++ lines — still get a path"
  - "Handler stats the worktree dir before any git call and relays a clear 'worktree directory is missing' message, so a vanished worktree yields the UI error card, not a raw git 128 (Pitfall 7)"

patterns-established:
  - "Pattern: per-untracked-file no-index runner with explicit ExitCode()==1 success branch"
  - "Pattern: server pre-structures diffs into per-file hunks (frontend is a dumb map, no client-side diff parsing)"

requirements-completed: [REVW-01]

# Metrics
duration: 7 min
completed: 2026-06-11
---

# Phase 5 Plan 02: Diff Service & REST Endpoint Summary

**REVW-01 backend: GET /api/tasks/{id}/diff returns everything the task changed vs the merge-base of its base branch (committed + staged + unstaged + untracked-as-additions, base movement excluded) as structured per-file-hunk JSON, parsed from git's -z/unified machine output with rename/binary/no-newline/mode-only edges handled.**

## Performance

- **Duration:** 7 min
- **Started:** 2026-06-11T09:08:47Z
- **Completed:** 2026-06-11T09:15:54Z
- **Tasks:** 3 (all TDD: RED → GREEN, no refactor needed)
- **Files modified:** 7 (6 created, 1 modified)

## Accomplishments

- `internal/diff` package: pure `parseNumstatZ` + `parsePatch` parsers and a `Compute` pipeline that produces the full D-59 picture from a real worktree (verified against git 2.43.0 fixture repos).
- Dedicated `runNoIndex` runner honoring `git diff --no-index`'s exit-1-as-success contract, so untracked files render as full-addition patches without ever mutating the user's index.
- `GET /api/tasks/{id}/diff` endpoint with honest error relays (no-worktree 409, unknown-task 404, missing-dir/git 500 with the message surfaced into the UI error card), registered in production wiring.
- Three-dot (merge-base) semantics proven by test: lines `main` gained after branching appear nowhere in the diff.

## Task Commits

Each task ran the TDD RED → GREEN cycle (no refactor commits — implementations were clean on first pass):

1. **Task 1: Diff types + numstat/patch parsers** - `03d5ef9` (test) → `0fed080` (feat)
2. **Task 2: Git runners + Compute pipeline** - `c9589f1` (test) → `f74adda` (feat)
3. **Task 3: GET /api/tasks/{id}/diff endpoint + registration** - `8596957` (test) → `be2a713` (feat)

## Files Created/Modified

- `internal/diff/parse.go` - JSON-ready Diff/Totals/File/Hunk/Line types (exact frontend tags) + `parseNumstatZ` (NUL-first, rename old/new consumption, binary detect) + `parsePatch` (line-anchored split, header-seeded paths, hunk classification, no-newline/mode-only/binary tolerances).
- `internal/diff/parse_test.go` - Table tests over verbatim machine-format fixtures: rename desync, binary, no-newline, mode-only, new, deleted, multi-file.
- `internal/diff/diff.go` - `run` (exit-0 git runner mirroring worktree.gitRun) + `runNoIndex` (exit-1 success contract) + `Compute` (merge-base → numstat → patch → ls-files → per-untracked no-index, correlated by path, sorted, totalled).
- `internal/diff/diff_test.go` - Real-git fixture tests with `TestMain` config isolation: full picture, binary, pristine, untracked-only, bogus base.
- `internal/api/diffs.go` - `DiffRoutes` + `get` handler (single-query lookup, gates, ResolveBase re-resolution, Compute, base set on response).
- `internal/api/diffs_test.go` - httptest fixture tests: modified+untracked, pristine, no-worktree 409, unknown-task 404, missing-dir 500.
- `cmd/kangent/main.go` - `api.DiffRoutes(mux, db, wtSvc)` registered after WorktreeRoutes.

## Decisions Made

- Private git runner in the diff package (not importing worktree's unexported `gitRun`) — the exit-code contracts genuinely differ between the packages.
- numstat is authoritative for tracked-file stats; untracked stats are derived from the no-index patch since numstat never reports untracked files.
- Patch path seeded from the `diff --git` header b-side so binary and mode-only sections (which have no `---`/`+++` lines) still resolve a path; `+++ b/` / `--- a/` and rename headers override where present.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

- The shared `internal/api` test package does not currently compile to completion because a **parallel wave-1 plan (05-01)** committed its RED-phase test changes to `internal/api/sessions_test.go` (commit `7cfafef`, referencing `sessionHandlers.globRoot`, `transcriptExists`, and an `os` import) while its GREEN-phase production code (`sessions.go`) has not yet landed. This is expected during parallel execution and is **out of scope** for plan 05-02 (SCOPE BOUNDARY: only fix issues directly caused by this plan's changes).
  - **My plan's code is fully self-consistent:** `go build ./...` succeeds, `go build ./internal/api/` succeeds, `go vet ./internal/diff/` is clean, and `go test ./internal/diff/ -count=1` passes (all five Compute tests + all parser tests).
  - `diffs_test.go` contains zero references to `globRoot`/`transcriptExists` — it does not contribute to the compile failure.
  - The orchestrator's post-wave validation will run `go test ./internal/api/` once 05-01's production code lands; my `TestDiff*` cases will execute then.

## Next Phase Readiness

- The exact Pattern 4 JSON contract (`base`, `totals{files,additions,deletions}`, `files[]{path,oldPath,status,binary,additions,deletions,hunks[]{header,lines[]{kind,text}}}`) is live and ready for the 05-04 frontend DiffTab, which renders it directly.
- No new Go modules added (`git diff go.mod` empty) — stdlib + system git only, per CLAUDE.md.
- Blocker for the orchestrator: do not run `go test ./internal/api/` for 05-02 sign-off in isolation until 05-01's GREEN phase lands; run after all wave-1 agents complete.

## Self-Check: PASSED

All 6 created source/test files exist on disk; all 6 task commits (3 test + 3 feat) present in git history.

---
phase: 22-github-style-diff-review
plan: 01
subsystem: api
tags: [go, sha256, diff, hashing, internal-diff, viewed-state]

# Dependency graph
requires:
  - phase: 05-diff-tab
    provides: "internal/diff.Compute — the three-dot merge-base diff pipeline producing []File{Path,Status,Binary,OldPath,Hunks} rendered as JSON"
provides:
  - "diff.File.Hash — deterministic sha256 hex of the rendered per-file diff (Status/Binary/OldPath/Hunks in; Base/Path/counts out), set for every file by Compute"
  - "diff.File.Viewed — bool field (json:\"viewed\") the API handler populates from the diff_viewed store; zero value here (Compute is DB-free)"
  - "hashFile(File) string — pure, length-prefixed, collision-safe hashing helper"
  - "TestFileHash — the primary Go test seam pinning determinism, content-sensitivity, D-01 base-insensitivity, rename-vs-modify, and binary strict-D-01 stability"
affects: [22-02, 22-03, 22-04, 22-05]

# Tech tracking
tech-stack:
  added: [crypto/sha256, encoding/hex, io]
  patterns:
    - "Server-authoritative rendered-per-file content hash (structured-diff-on-server, dumb-map-on-client)"
    - "Length-prefixed serialization into a sha256 hasher for collision-safety over ordered slices"

key-files:
  created: []
  modified:
    - internal/diff/parse.go
    - internal/diff/diff.go
    - internal/diff/diff_test.go

key-decisions:
  - "Hash covers the RENDERED per-file content only (Status/Binary/OldPath/Hunks); Base/Path/Additions/Deletions excluded — gives D-01 'base movement doesn't reset' for free under three-dot semantics"
  - "Binary strict-D-01 locked: hash a content-invariant header, never blob OIDs; a binary whose bytes change without a status/path/header change keeps its hash (accepted, documented limitation — not a bug)"
  - "hashFile set in one post-sort pass in Compute so tracked and untracked files flow through a single hashing code path"
  - "rename-vs-modify distinction tested by calling hashFile directly on hand-built File structs (in-package test) rather than coaxing git's rename heuristics into producing identical hunks"

patterns-established:
  - "Pattern: pure per-file helper over File/[]Hunk mirroring countHunkLines' shape — no I/O, ordered slices only, deterministic across runs/machines"
  - "Pattern: hash-relationship test assertions (equal/not-equal), never hard-coded hex literals, so tests survive serialization reshaping"

requirements-completed: [DIFF-03, DIFF-04]

# Metrics
duration: 8min
completed: 2026-07-02
---

# Phase 22 Plan 01: Rendered-Per-File Diff Hash Summary

**Server-authoritative sha256 `Hash` (and a `Viewed` field) on `diff.File`, computed via a pure length-prefixed `hashFile` helper over the rendered diff (Status/Binary/OldPath/Hunks), keying the per-file Viewed persistence and driving DIFF-04 auto-reset.**

## Performance

- **Duration:** ~8 min
- **Started:** 2026-07-02T04:44:00Z
- **Completed:** 2026-07-02T04:52:00Z
- **Tasks:** 2
- **Files modified:** 3

## Accomplishments
- `diff.File` gains `Hash string` (`json:"hash"`) and `Viewed bool` (`json:"viewed"`), each with an intent doc-comment; the binary strict-D-01 limitation is documented on the `Hash` field.
- `hashFile(File) string` — a pure, deterministic, length-prefixed sha256 helper (`crypto/sha256` + `encoding/hex`) that hashes the rendered per-file content and excludes Base/Path/counts.
- `Compute` sets `File.Hash` for every file in one post-sort pass, giving both the tracked and untracked branches a single hashing code path; `Viewed` is left at its zero value (Compute stays DB-free).
- `TestFileHash` exercises all five required relationships (determinism, content-sensitivity, D-01 base-insensitivity, rename-vs-modify, binary stability + byte-invariance) via real git in `t.TempDir()` plus a direct in-package `hashFile` comparison.

## Task Commits

Each task was committed atomically:

1. **Task 1: Add Hash/Viewed fields and the hashFile helper** — `491d16e` (feat)
2. **Task 2: Table-driven hash-invariant tests** — `3477f64` (test)

_Task 1 carried a `tdd="true"` marker, but the plan explicitly decomposed implementation (Task 1, verified by build+vet) and tests (Task 2, verified by `go test`) into separate tasks; executed in that plan-specified order. The MVP+TDD runtime gate was not active (no MVP_MODE/TDD_MODE flags passed; project mode is `yolo`)._

## Files Created/Modified
- `internal/diff/parse.go` - Added `Hash` and `Viewed` fields to the `File` struct with doc-comments (Hash placed after Path; a blank line separates the review-state fields into their own gofmt alignment group).
- `internal/diff/diff.go` - Added `crypto/sha256`/`encoding/hex`/`io` imports, the `hashFile` helper, and a post-sort `f.Hash = hashFile(f)` pass in `Compute`.
- `internal/diff/diff_test.go` - Added `TestFileHash` (5 sub-tests) asserting hash relationships, never hex literals.

## Decisions Made
- Followed the plan's Area 1 serialization exactly (length-prefixed `<len>:<bytes>` writes over ordered slices) — collision-safe (mitigates threat T-22-06) and deterministic; SHA-256 is stdlib and correct here even though it is a content fingerprint, not an auth control.
- Placed `Hash` immediately after `Path` per the plan; for `Viewed`, added a blank line after it so gofmt keeps it in its own alignment group (single-space `Viewed bool`) rather than aligning it with the `OldPath…Hunks` block. This preserves the plan's exact acceptance grep (`grep 'Viewed bool'`) and reads more cleanly (review-state fields grouped, separate from rendered-content fields).
- Tested the rename-vs-modify distinction by calling the unexported `hashFile` directly on two hand-built `File` values with identical hunks (the test file is `package diff`). This is deterministic and directly proves Status/OldPath participate, avoiding brittle attempts to make git emit byte-identical hunks for a rename and a modify.

## Deviations from Plan

None - plan executed exactly as written.

_Note: The `Viewed bool` field placement (blank line to force its own gofmt group) is a formatting detail that satisfies the plan's literal acceptance grep; it is not a functional deviation._

## Issues Encountered
- The plan's Task 1 acceptance grep `grep -n 'Viewed bool'` initially failed because gofmt aligned `Viewed    bool` (multiple spaces) when the field was contiguous with the `OldPath…Hunks` block. Resolved by adding a blank line after `Viewed`, which puts it in its own alignment group (single space) — build, vet, gofmt, and all acceptance greps then passed.

## Verification
- `go build ./internal/diff/...` and `go vet ./internal/diff/...` exit 0 (Task 1 gate).
- `go test ./internal/diff/... -run TestFileHash -v` — all 5 sub-tests PASS.
- `go test ./internal/diff/...` — full package green.
- `go build ./... && go vet ./...` — whole tree green (phase verification block).
- `gofmt -l` — no formatting drift on any modified file.
- Acceptance greps confirmed: `Hash string`/`Viewed bool` with correct json tags, `func hashFile`, `crypto/sha256` present, `.Hash = hashFile` inside Compute after the sort, and NO new `index <oid>..<oid>` parsing in parse.go (strict D-01 honored).

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- The `hash` + `viewed` JSON contract on `diff.File` is now the authoritative shape Plans 02/03 consume: Plan 02 wires the `diff_viewed` store + `PUT .../diff/viewed` endpoint and populates `File.Viewed`; the client echoes `file.hash` back on toggle.
- No blockers. `Viewed` is intentionally the zero value here (Compute is DB-free) — the API handler layers it on top, exactly as the research/patterns prescribe.

## Self-Check: PASSED

- Files verified present: `internal/diff/parse.go`, `internal/diff/diff.go`, `internal/diff/diff_test.go`, `.planning/phases/22-github-style-diff-review/22-01-SUMMARY.md`.
- Commits verified in git log: `491d16e` (feat), `3477f64` (test), `c499ed7` (docs).

---
*Phase: 22-github-style-diff-review*
*Completed: 2026-07-02*

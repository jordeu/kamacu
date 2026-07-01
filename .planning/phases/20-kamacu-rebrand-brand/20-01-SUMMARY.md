---
phase: 20-kamacu-rebrand-brand
plan: 01
subsystem: infra
tags: [go-module, rename, rebrand, makefile, gofmt, backend]

# Dependency graph
requires:
  - phase: (none)
    provides: first phase of v1.8; depends on nothing
provides:
  - Go module renamed kangent -> kamacu (go.mod + all 41 internal import paths)
  - Entrypoint dir cmd/kangent -> cmd/kamacu; Makefile builds bin/kamacu
  - Cosmetic product-name prose + kangentSessionID identifier renamed to Kamacu
  - All runtime keep-out literals (~/.kangent paths, -L kangent socket, session
    prefix, kangent-tmux.conf, X-Kangent-Token header) left byte-for-byte intact
affects: [phase-21-data-directory-migration, brand/UI rename plans in phase 20]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Keep-out boundary rename: swap identity/prose tokens while a fenced set of runtime literals stays byte-for-byte (verified by go test ./... staying green)"
    - "Import rewrite anchored on quote+slash (\"kangent/) provably cannot match any keep-out literal"

key-files:
  created:
    - .planning/phases/20-kamacu-rebrand-brand/deferred-items.md
  modified:
    - go.mod
    - Makefile
    - cmd/kamacu/main.go (moved from cmd/kangent/, imports + prose)
    - cmd/kamacu/main_test.go (moved from cmd/kangent/)
    - internal/**/*.go (41 files import-rewritten; 17 also prose-swept)

key-decisions:
  - "Full make build (frontend npm install + backend) not run: no network + frontend rename is a separate plan; make backend proves the renamed target produces bin/kamacu, and go build embeds the existing web/dist"
  - "8 pre-existing gofmt-dirty files left untouched (out of scope; logged to deferred-items.md) — the prefix swap did not shift import ordering"

patterns-established:
  - "Rename via protect-header -> global capital/all-caps prose swap -> restore-header -> distinctive lowercase-phrase swaps, leaving lowercase keep-out literals safe"

requirements-completed: [REBRAND-02, REBRAND-03]

# Metrics
duration: ~10min
completed: 2026-07-01
---

# Phase 20 Plan 01: Kamacu Go Identity Rename Summary

**Go module, imports, entrypoint dir, and Makefile target renamed kangent -> kamacu; cosmetic prose + the kangentSessionID identifier say Kamacu — while every runtime path/socket/session/config/auth literal stays kangent for Phase 21's gated flip.**

## Performance

- **Duration:** ~10 min
- **Started:** 2026-07-01T11:22Z (approx)
- **Completed:** 2026-07-01T11:31Z
- **Tasks:** 2
- **Files modified:** 50 unique (`.go` + go.mod + Makefile)

## Accomplishments
- `go.mod` is `module kamacu`; all 41 internal import paths rewritten `"kangent/` -> `"kamacu/` (zero `"kangent/` imports remain).
- Entrypoint moved `cmd/kangent/` -> `cmd/kamacu/` (git rename, history preserved); `Makefile` builds `bin/kamacu ./cmd/kamacu` and `dev-backend` runs `./cmd/kamacu`; `make backend` produces an executable `bin/kamacu`.
- Cosmetic sweep complete: product-name prose in comments, log lines (`kamacu listening`, `kamacu is listening`, `kamacu serves a shell`), and error text renamed; non-runtime identifier `kangentSessionID` -> `kamacuSessionID`; stale `cmd/kangent/...` path-doc comments updated to `cmd/kamacu/...`. Completeness grep (kangent minus keep-out/sentinel exclusions) returns 0.
- Keep-out boundary held: `DefaultSocket = "kangent"` (1), `reposBase` `~/.kangent/repos/` (1), the `kangent-%d-%d` session prefix (1), the `~/.kangent/kangent.db` / `kangent-tmux.conf` / `~/.kangent/worktrees` / `HasPrefix(name, "kangent-")` literals in `cmd/kamacu/main.go` (1 each), and `X-Kangent-Token` across all 6 sites — all byte-for-byte unchanged.
- `go build ./...`, `go vet ./...`, and `go test ./...` (12 packages) all green after each task.

## Task Commits

Each task was committed atomically:

1. **Task 1: Rename Go module, imports, cmd directory, Makefile target** - `a7b2796` (refactor)
2. **Task 2: Sweep cosmetic product-name prose and non-runtime identifiers** - `808003e` (refactor)

## Files Created/Modified
- `go.mod` - `module kangent` -> `module kamacu`.
- `Makefile` - build target `-o bin/kamacu ./cmd/kamacu`; `dev-backend` -> `./cmd/kamacu`.
- `cmd/kamacu/main.go`, `cmd/kamacu/main_test.go` - moved from `cmd/kangent/`; imports rewritten; main.go log/error/comment prose renamed (keep-out DB path/tmux.conf/worktree root/orphan-sweep guard preserved).
- `internal/**/*.go` - 41 files import-rewritten to `kamacu/internal/*`; 17 of them additionally prose-swept (`agent.go` identifier, `tmux.go`/`quota.go`/`projects.go`/`sessions.go`/`tasks.go`/`hooks.go`/`manager.go`/`session.go`/`tokenize.go` comments, and test-harness `cmd/kamacu` path-doc comments).
- `.planning/phases/20-kamacu-rebrand-brand/deferred-items.md` - logs 8 pre-existing gofmt-dirty files (out of scope).

## Decisions Made
- **Full `make build` not executed; `make backend` used to prove the renamed target.** `make build` = `frontend backend`, and `frontend` runs `cd web && npm install` which needs network (no `web/node_modules` in this worktree). The REBRAND-02 deliverable — the binary builds as `bin/kamacu` — is proven by `make backend`; `go build` still embeds the pre-existing `web/dist`. The frontend brand/UI rename is a separate plan in this phase.
- **Import rewrite anchored on `"kangent/` (quote+slash).** Provably cannot match any keep-out literal (all of which are `"~/.kangent...`, `"kangent"`, `"kangent-...`, `kangent.db`, or `kangent-tmux.conf`), so the mechanical sweep is keep-out-safe.
- **Capital prose handled by protect-then-global swap.** Confirmed every capital `Kangent` is prose or the `X-Kangent-Token` header (no Go identifier); protected the header, globally swapped `Kangent`/`KANGENT` -> `Kamacu`/`KAMACU`, restored the header, then applied distinctive lowercase-phrase swaps — leaving lowercase keep-out literals untouched.

## Deviations from Plan

None functionally — the two tasks executed as written. One scope-boundary note (not a code deviation): 8 files were already `gofmt`-dirty at the base commit (`0672e3b`, verified via `git show HEAD:<file> | gofmt -l`). The `"kangent/` -> `"kamacu/` prefix swap did not shift import ordering, so it neither caused nor worsened them. Per the scope boundary they were left untouched and logged to `deferred-items.md`; `go build`/`vet`/`test ./...` do not depend on gofmt.

## Issues Encountered
None.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- The backend now builds and tests as `kamacu` with a `kamacu`-named module and binary; other Phase 20 plans (brand mark, favicon, `web/index.html` title, sidebar wordmark, UI copy, README) can rebrand on top of a `kamacu`-named build.
- Phase 21 (data-directory migration) inherits an intact keep-out boundary: every runtime literal it must flip atomically (`~/.kangent` paths, `-L kangent` socket + `kangent-*` session prefix, `kangent-tmux.conf`, `X-Kangent-Token`) is verified still present and unchanged.

## Self-Check: PASSED

- Created files verified present: `20-01-SUMMARY.md`, `deferred-items.md`, `cmd/kamacu/main.go`.
- Commits verified in git log: `a7b2796` (Task 1), `808003e` (Task 2), `406f8c1` (docs).
- Working tree clean; no uncommitted artifacts.

---
*Phase: 20-kamacu-rebrand-brand*
*Completed: 2026-07-01*

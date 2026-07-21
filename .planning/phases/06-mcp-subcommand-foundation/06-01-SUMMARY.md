---
phase: 06-mcp-subcommand-foundation
plan: 01
subsystem: cli
tags: [cli, subcommands, dispatcher, google-subcommands]

requires: []
provides:
  - "Thin `cmd/kamacu/main.go` dispatcher using `github.com/google/subcommands` — registers `serve` Command + library HelpCommand, calls `subcommands.Execute`"
  - "`cmd/kamacu/serve.go` holding the entire pre-Phase-06 main() body verbatim inside `serveCmd.Execute`, plus relocated helpers `ensureLoopback`/`hostCheck`/`hookBaseURL`/`sweepOrphanTmux`"
  - "`kamacu serve` invocation shape with identical `--addr`/`--db`/`--claude-bin`/`--dev-origin`/`--insecure-allow-remote` flags and defaults"
  - "D-04 break-clean posture: bare `kamacu` now prints `google/subcommands` help and exits non-zero (no transitional shim)"
affects: [06-02-mcp-serve-subcommand, mcp-bridge, future-subcommands]

tech-stack:
  added: [github.com/google/subcommands v1.2.0]
  patterns: ["Command interface dispatch via google/subcommands (Pattern 2 nested Commander lands in Plan 02)"]

key-files:
  created:
    - cmd/kamacu/serve.go
  modified:
    - cmd/kamacu/main.go
    - go.mod
    - go.sum
    - Makefile
    - scripts/smoke.sh
    - README.md

key-decisions:
  - "Registered `&serveCmd{}` (pointer) instead of `serveCmd{}` (value) because SetFlags mutates the struct — pointer-receiver methods + value-receiver Name/Synopsis/Usage still satisfy the `subcommands.Command` interface via `*serveCmd`'s method set. Plan text said `serveCmd{}` for readability; pointer is the Go-idiomatic actual shape."
  - "Mapped `os.Exit(1)` failure paths in the pre-refactor body to `return subcommands.ExitFailure` (not `os.Exit`) so the dispatcher's `os.Exit(int(subcommands.Execute(...)))` owns process exit. `http.ListenAndServe` error also returns `ExitFailure`; clean return on listen-server stop returns `ExitSuccess` (mirrors `os.Exit(0)` intent)."
  - "Used `f.Func(...)` (method on `*flag.FlagSet`, Go 1.21+) for the repeatable `--dev-origin` flag — identical semantics to the prior top-level `flag.Func(...)`."

patterns-established:
  - "Pattern: `subcommands.Register(&cmd{}, \"\")` for stateful Commands whose SetFlags mutates fields"
  - "Pattern: bare-binary invocation prints help + exits non-zero (D-04 break-clean — no shim)"

requirements-completed: [MCPPROC-01]

coverage:
  - id: D1
    description: "`cmd/kamacu/main.go` is a <15-line dispatcher registering HelpCommand + serveCmd and dispatching via `subcommands.Execute`; contains no `http.ListenAndServe`/`ensureLoopback`/`hostCheck`/`sweepOrphanTmux` definitions"
    requirement: MCPPROC-01
    verification:
      - kind: integration
        ref: "grep checks in plan verify block — confirmed main.go has no server body, serve.go has all four helpers"
        status: pass
      - kind: integration
        ref: "go build -o /tmp/kamacu-plan01 ./cmd/kamacu"
        status: pass
    human_judgment: false
  - id: D2
    description: "`go.mod` contains `github.com/google/subcommands v1.2.0` as a direct requirement"
    requirement: MCPPROC-01
    verification:
      - kind: integration
        ref: "grep -q 'github.com/google/subcommands v1.2.0' go.mod"
        status: pass
    human_judgment: false
  - id: D3
    description: "`cmd/kamacu/serve.go` defines `serveCmd` implementing all five `subcommands.Command` interface methods; today's entire main() body lives in `serveCmd.Execute` verbatim (helpers relocated)"
    requirement: MCPPROC-01
    verification:
      - kind: integration
        ref: "go build + ./scripts/smoke.sh (SMOKE OK) — full mux + SPA fallback + middleware stack survives the move"
        status: pass
    human_judgment: false
  - id: D4
    description: "Bare `./bin/kamacu` (no args) prints help containing `Usage:` and exits non-zero (D-04 break-clean)"
    requirement: MCPPROC-01
    verification:
      - kind: integration
        ref: "/tmp/kamacu-plan01 >/dev/null 2>&1; echo exit=$? → 2; /tmp/kamacu-plan01 2>&1 | grep -qi 'usage:'"
        status: pass
    human_judgment: false
  - id: D5
    description: "`./bin/kamacu serve --help` shows all 5 flags (`--addr`/`--db`/`--claude-bin`/`--dev-origin`/`--insecure-allow-remote`) with original defaults"
    requirement: MCPPROC-01
    verification:
      - kind: integration
        ref: "/tmp/kamacu-plan01 serve --help 2>&1 | grep -q -- '--<flag>' for each of the 5 flags"
        status: pass
    human_judgment: false
  - id: D6
    description: "`./bin/kamacu serve --addr 127.0.0.1:<port> --db <path>` answers `/api/healthz` with `{\"status\":\"ok\"}` and `/` returns HTML (full mux + SPA fallback + middleware stack survives the move)"
    requirement: MCPPROC-01
    verification:
      - kind: integration
        ref: "manual curl against running instance on port 7403 → PASS healthz, PASS spa"
        status: pass
    human_judgment: false
  - id: D7
    description: "`make build` (Vite + Go embed) succeeds; `./scripts/smoke.sh` prints `SMOKE OK`; `go test ./...` passes"
    requirement: MCPPROC-01
    verification:
      - kind: integration
        ref: "make build + ./scripts/smoke.sh + go test ./... (clean run)"
        status: pass
    human_judgment: false
  - id: D8
    description: "Makefile `dev-backend` uses `go run ./cmd/kamacu serve`; scripts/smoke.sh uses `./bin/kamacu serve --addr ...`; README run instructions use `./bin/kamacu serve`"
    requirement: MCPPROC-01
    verification:
      - kind: integration
        ref: "grep checks for all three files"
        status: pass
    human_judgment: false

duration: 12min
completed: 2026-07-21
status: complete
---

# Phase 06 Plan 01: CLI Dispatcher Refactor Summary

**`cmd/kamacu/main.go` slimmed to a 14-line `google/subcommands` dispatcher; today's entire server body moved verbatim into `cmd/kamacu/serve.go`'s `serveCmd`; bare `kamacu` now prints help and exits non-zero (D-04 break-clean).**

## Performance

- **Duration:** ~12 min
- **Started:** 2026-07-21T18:35:00Z
- **Completed:** 2026-07-21T18:47:00Z
- **Tasks:** 2
- **Files modified:** 7 (1 new + 6 edited)

## Accomplishments
- Adopted `github.com/google/subcommands v1.2.0` as a direct dependency (zero transitive deps).
- Created `cmd/kamacu/serve.go` (~330 lines) holding the entire pre-Phase-06 `main()` body verbatim inside `serveCmd.Execute`, plus the four relocated unexported helpers (`sweepOrphanTmux`, `ensureLoopback`, `hookBaseURL`, `hostCheck`).
- Refactored `cmd/kamacu/main.go` to a thin dispatcher: registers `subcommands.HelpCommand()` + `&serveCmd{}`, calls `flag.Parse()` + `os.Exit(int(subcommands.Execute(ctx)))`. 14 lines total.
- Updated Makefile (`dev-backend: go run ./cmd/kamacu serve`), `scripts/smoke.sh` (`./bin/kamacu serve --addr ...`), and `README.md` (`./bin/kamacu serve`).
- Verified byte-for-byte behavior preservation: `make build` succeeds, `./scripts/smoke.sh` prints `SMOKE OK`, full `go test ./...` passes (clean run), and a live `./bin/kamacu serve` instance answers `/api/healthz` with `{"status":"ok"}` and serves HTML at `/` (SPA fallback intact).
- Verified D-04 break-clean: bare `/tmp/kamacu-plan01` exits non-zero (status 2) and prints the `google/subcommands` help block containing `Usage:` and the `serve` subcommand.

## Task Commits

Each task was committed atomically:

1. **Task 1: Adopt google/subcommands, move today's main body into serveCmd, refactor main.go to thin dispatcher, update README/Makefile/smoke.sh** — `d74f1b4` (refactor)
2. **Task 2: Verify byte-for-byte behavior preservation via smoke.sh and go test** — verification-only, no production code changes (no commit; STATE/ROADMAP updates ride the plan-metadata commit below)

**Plan metadata:** this commit (docs: complete cli-dispatcher-refactor plan)

## Files Created/Modified
- `cmd/kamacu/serve.go` — NEW (~330 lines): `serveCmd` struct + all five `subcommands.Command` methods; the entire pre-refactor `main()` body lives in `serveCmd.Execute`. Helpers `sweepOrphanTmux`/`ensureLoopback`/`hookBaseURL`/`hostCheck` relocated here.
- `cmd/kamacu/main.go` — refactored from 439 lines to 14 lines: dispatcher only.
- `go.mod` / `go.sum` — adds `github.com/google/subcommands v1.2.0`.
- `Makefile` — `dev-backend` target uses `go run ./cmd/kamacu serve`.
- `scripts/smoke.sh` — `start_server` uses `./bin/kamacu serve --addr ... --db ...`.
- `README.md` — run instructions use `./bin/kamacu serve`.

## Decisions Made
- **Registered `&serveCmd{}` (pointer) instead of `serveCmd{}` (value).** Plan text said `serveCmd{}` for readability, but `SetFlags` mutates the struct's fields, so it must have a pointer receiver; `*serveCmd` satisfies `subcommands.Command` because its method set includes both the value-receiver `Name/Synopsis/Usage` (promoted) and its own pointer-receiver `SetFlags/Execute`. A value `serveCmd{}` fails the interface check at compile time — confirmed during the first build attempt.
- **Mapped `os.Exit(1)` paths to `return subcommands.ExitFailure`.** The pre-refactor body had six `os.Exit(1)` calls in failure paths (loopback check, db resolution, migration Prepare/Complete, store Open/Migrate, backfills, tmux config, token gen, worktree root, ListenAndServe). All now return `subcommands.ExitFailure` so the dispatcher's `os.Exit(int(subcommands.Execute(...)))` owns process exit. `http.ListenAndServe` returning nil on clean stop returns `ExitSuccess` (today's path never hit this, but it's the correct mapping).
- **Used `f.Func(...)` for the repeatable `--dev-origin` flag.** This is the method form (Go 1.21+) on `*flag.FlagSet`, semantically identical to the prior top-level `flag.Func(...)` call. First build attempt had a stray extra `f` parameter (a typo); fixed before committing.

## Deviations from Plan

None - plan executed exactly as written. The `&serveCmd{}` vs `serveCmd{}` adjustment is the standard Go interface-satisfaction requirement for stateful Commands (the plan's literal `serveCmd{}` was pseudocode that the compiler rejected); documented in Decisions Made rather than as a deviation.

## Issues Encountered
- **Pre-existing flake in `internal/session.TestAttachAfterExitReturnsReplay`.** During the first full `go test ./...` run, this PTY-timing test failed with `late attach replay missing marker; got "\x1b[?2004h"`. It passes consistently when run in isolation (`-count=10`, 10/10 pass) and did not reproduce on a second full-suite run. Plan 01 touches NO files in `internal/session/`, so this is a pre-existing PTY-timing flake unrelated to the refactor. No action required.

## User Setup Required
None - no external service configuration required. The new `kamacu serve` invocation is the only behavioral change users will notice; existing `make dev-backend` / `./scripts/smoke.sh` flows continue to work because they were updated in lockstep.

## Next Phase Readiness
- **Ready for Plan 02 (`mcp-serve-subcommand`).** The thin dispatcher is in place; Plan 02 adds exactly one new `subcommands.Register(&mcpCmd{}, "")` line to `cmd/kamacu/main.go` and creates `cmd/kamacu/mcp.go` using Pattern 2 (nested `subcommands.Commander`) to dispatch `kamacu mcp serve`.
- **Ready for Plan 03 (`token-header-rename`).** No file overlap with this plan — Plan 03 touches `internal/api/hooks.go`, `internal/session/agent.go`, and `internal/opencode/*` only.
- **D-01..D-05 fully delivered:** (D-01) `google/subcommands` adopted; (D-02) main.go thin / body in `serve.go`; (D-03) `kamacu serve` runs the server; (D-04) bare `kamacu` prints help; (D-05) only `serve` registered (no `mcp` Command — that's Plan 02's responsibility).

---
*Phase: 06-mcp-subcommand-foundation*
*Completed: 2026-07-21*

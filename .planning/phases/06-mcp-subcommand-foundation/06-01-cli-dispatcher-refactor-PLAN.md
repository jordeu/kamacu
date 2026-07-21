---
phase: 06-mcp-subcommand-foundation
plan: 01
slug: cli-dispatcher-refactor
type: execute
wave: 1
depends_on: []
files_modified:
  - cmd/kamacu/main.go
  - cmd/kamacu/serve.go
  - go.mod
  - go.sum
  - README.md
  - Makefile
  - scripts/smoke.sh
autonomous: true
requirements: [MCPPROC-01]
must_haves:
  truths:
    # D-03 / D-04 — break clean on `serve`
    - "Running `kamacu serve` from a shell starts the HTTP server with the same flags (`--addr`, `--db`, `--claude-bin`, `--dev-origin`, `--insecure-allow-remote`) and identical boot behavior as today's bare `kamacu` invocation — `scripts/smoke.sh` prints `SMOKE OK` against `./bin/kamacu serve --addr ... --db ...` (D-03/D-04)"
    - "Running `kamacu` with NO args prints the `google/subcommands` help output (listing `serve` and the library's `help`) and exits non-zero — the server does NOT start (D-03/D-04 break-clean)"
    # D-01 — adoption
    - "`go.mod` contains `github.com/google/subcommands v1.2.0` as a direct requirement (D-01)"
    # Byte-for-byte behavior preservation
    - "All existing one-shot backfills (`BackfillProjectIcons`, `BackfillWorkspaces`, `BackfillAgents`, `BackfillAgentExtraParams`, `BackfillOpenCodeAgent`), `opencode.InstallPlugin`, `tmux.WriteConfig`, hook-token generation (`crypto/rand` 32 bytes → `hex.EncodeToString`), `migrate.Prepare` / `migrate.Complete`, `sweepOrphanTmux`, the reaper goroutine (`reaper.NewWithPR(...).Run`), SPA fallback handler, and the `hostCheck` / `ensureLoopback` / `hookBaseURL` helpers all run byte-for-byte unchanged inside `serveCmd.Execute`"
    - "`./bin/kamacu serve --addr 127.0.0.1:<port> --db <path>` answers `curl -fsS http://127.0.0.1:<port>/api/healthz` with `{\"status\":\"ok\"}` and a 200 status (proves the full mux + SPA fallback + middleware stack survives the move)"
    # Build / compile
    - "`go build -o bin/kamacu ./cmd/kamacu` (the `make backend` target) succeeds with exit code 0"
    - "`go test ./...` passes with zero failures (no test depends on the now-removed bare-main shape)"
  artifacts:
    - cmd/kamacu/main.go (refactored to thin dispatcher)
    - cmd/kamacu/serve.go (NEW — serveCmd Command holding today's main body)
    - go.mod (adds github.com/google/subcommands v1.2.0)
    - go.sum
    - README.md
    - Makefile
    - scripts/smoke.sh
  key_links:
    - "subcommands.Register(serveCmd{}, \"\") in main.go → serveCmd.Execute in serve.go"
    - "subcommands.Register(subcommands.HelpCommand(), \"\") in main.go → library's help command (default for bare `kamacu`)"
    - "subcommands.Execute(context.Background()) in main.go → dispatches to the FIRST positional arg (verified behavior in 06-RESEARCH.md § Pattern 2)"
  prohibitions:
    - statement: "Register NO `mcp` Command in this plan — Plan 02 owns cmd/kamacu/mcp.go and the `mcp serve` registration. main.go in this plan registers ONLY `serveCmd{}` and the library's `HelpCommand()` (D-05)."
      status: resolved
      verification: "After this plan, `cmd/kamacu/mcp.go` does NOT exist; `grep -n 'Register(' cmd/kamacu/main.go` returns exactly two lines (serveCmd + HelpCommand)"
    - statement: "DO NOT add `github.com/modelcontextprotocol/go-sdk` to go.mod in this plan — Plan 02 owns that. This plan adds ONLY `github.com/google/subcommands v1.2.0`."
      status: resolved
      verification: "After this plan, `grep 'modelcontextprotocol/go-sdk' go.mod` returns zero matches; `grep 'google/subcommands' go.mod` returns one match"
    - statement: "DO NOT add any transitional shim that lets bare `kamacu` (no args) start the server — D-04 break clean. The library's default behavior (print help, exit non-zero) is the intended posture."
      status: resolved
      verification: "`./bin/kamacu` (no args) prints help containing `Usage:` and exits with non-zero status; it never calls `http.ListenAndServe`"
    - statement: "DO NOT change semantics of `--addr`, `--db`, `--claude-bin`, `--dev-origin`, `--insecure-allow-remote` — flag names, defaults, and `--insecure-allow-remote` warn-and-allow logic are byte-for-byte identical to today's behavior. They simply move from package-level `flag.X` calls into `serveCmd.SetFlags(f *flag.FlagSet)`."
      status: resolved
      verification: "`./bin/kamacu serve --help` prints descriptions for all 5 flags with the SAME defaults as today (`127.0.0.1:7333`, `~/.kamacu/kamacu.db`, `""`, repeatable, `false`)"
    - statement: "DO NOT touch `internal/api/hooks.go`, `internal/session/agent.go`, or any `internal/opencode/*` file in this plan — those are Plan 03's coordinated-rename surface (zero file overlap with this plan)."
      status: resolved
      verification: "`git diff --name-only` for this plan shows NO files under `internal/api/`, `internal/session/`, or `internal/opencode/`"
---

# Plan 01: CLI dispatcher refactor + google/subcommands adoption

<objective>
Refactor `cmd/kamacu/main.go` onto `github.com/google/subcommands` so `kamacu serve` becomes the explicit server subcommand and bare `kamacu` prints help. Today's flag-based server body (from `flag.String("addr", ...)` through `http.ListenAndServe(...)`) moves verbatim into a new `serveCmd` Command in `cmd/kamacu/serve.go`. This is D-01..D-05 of Phase 06: thin dispatch in main.go, body in versioned packages, break clean on `serve`, register only `serve` (no `mcp` group, no future-subcommand stubs — those land in Plan 02).

Purpose: Establishes the `subcommands.Commander` shape Plan 02 extends with the `mcp` command group. Without this refactor, there is no clean slot to drop `kamacu mcp serve` into. The break-clean posture (D-04) is intentional — single-user local app, fresh-per-start hook token, server+agents always same-release means transitional shims are debt.

Output: `cmd/kamacu/main.go` becomes a <30-line dispatcher; `cmd/kamacu/serve.go` holds the entire today-main body; `go.mod` gains `github.com/google/subcommands v1.2.0`; `README.md` / `Makefile` / `scripts/smoke.sh` use `kamacu serve` explicitly.
</objective>

<execution_context>
@/home/jordi/.config/opencode/gsd-core/workflows/execute-plan.md
@/home/jordi/.config/opencode/gsd-core/templates/summary.md
</execution_context>

<context>
@.planning/PROJECT.md
@.planning/ROADMAP.md
@.planning/STATE.md
@.planning/phases/06-mcp-subcommand-foundation/06-CONTEXT.md
@.planning/phases/06-mcp-subcommand-foundation/06-RESEARCH.md
@cmd/kamacu/main.go
@go.mod
@Makefile
@scripts/smoke.sh
@README.md
</context>

<tasks>

<task type="auto">
  <name>Task 1: Adopt google/subcommands, move today's main body into serveCmd, refactor main.go to thin dispatcher, update README/Makefile/smoke.sh</name>
  <files>cmd/kamacu/main.go, cmd/kamacu/serve.go, go.mod, go.sum, Makefile, scripts/smoke.sh, README.md</files>
  <read_first>
    - `cmd/kamacu/main.go` — the ENTIRE 439-line file is the source of truth for what moves into serveCmd.Execute. Every import, every helper (`hostCheck`, `ensureLoopback`, `hookBaseURL`, `sweepOrphanTmux`), every backfill call, the reaper goroutine, the SPA fallback, the http.ListenAndServe — all of it relocates verbatim.
    - `.planning/phases/06-mcp-subcommand-foundation/06-RESEARCH.md` § "Pattern 2: Nested `kamacu mcp serve` dispatch" and the `google/subcommands` minimal pattern (lines ~676-727) — verified library idiom: `subcommands.Register(cmd, group)`, `flag.Parse()`, `os.Exit(int(subcommands.Execute(context.Background())))`. Note especially that `Execute` looks only at the FIRST positional arg.
    - `.planning/phases/06-mcp-subcommand-foundation/06-CONTEXT.md` decisions D-01..D-05 — locks the library choice, the file split (main thin / body in serve.go), the subcommand name (`serve`), the break-clean posture (no shim), and the registration breadth (only `serve`; NO `mcp` registration in this plan).
    - `go.mod` — currently has zero subcommands dependency; this plan adds `github.com/google/subcommands v1.2.0`.
    - `Makefile` — line 9 (`go build -o bin/kamacu ./cmd/kamacu`) stays unchanged; line 15 (`go run ./cmd/kamacu`) needs `serve` appended.
    - `scripts/smoke.sh` — line 27 (`./bin/kamacu --addr "$ADDR" --db "$WORK/k.db" &`) needs `serve` inserted after `./bin/kamacu`.
    - `README.md` — lines 37-38 invoke bare `./bin/kamacu`; line 48 (`make dev-backend`) is fine as-is (it calls the Makefile target we're updating).
  </read_first>
  <action>
    1. Add the dependency: `go get github.com/google/subcommands@v1.2.0 && go mod tidy`. Verify with `grep 'github.com/google/subcommands v1.2.0' go.mod`.

    2. Create `cmd/kamacu/serve.go` (same `package main`) defining `serveCmd` as a struct that holds the five flag values:
       - Fields: `addr string`, `db string`, `claudeBin string`, `devOrigins []string`, `insecureAllowRemote bool`.
       - Implement all five `subcommands.Command` interface methods:
         - `Name() string` returns `"serve"`.
         - `Synopsis() string` returns a one-line summary, e.g. `"run the Kamacu HTTP server (today's default behavior)"`.
         - `Usage() string` returns a short multi-line usage block naming the flags.
         - `SetFlags(f *flag.FlagSet)` declares the five flags with IDENTICAL names, defaults, and descriptions to today's `cmd/kamacu/main.go` lines 36-44 (`addr` → `"127.0.0.1:7333"`, `db` → `"~/.kamacu/kamacu.db"`, `claude-bin` → `""` with the same docstring, `dev-origin` as a repeatable `f.Func` appending to `devOrigins`, `insecure-allow-remote` as a `BoolVar` default false).
         - `Execute(ctx context.Context, f *flag.FlagSet, _ ...any) subcommands.ExitStatus` contains the ENTIRE current `main()` body from the `ensureLoopback(*addr)` check (today's line 51) through the final `http.ListenAndServe(*addr, handler)` (today's line 310), VERBATIM. The body reads from `c.addr`, `c.db`, etc. (the struct fields populated by SetFlags) instead of dereferencing flag pointers. All helpers (`sweepOrphanTmux`, `ensureLoopback`, `hookBaseURL`, `hostCheck`) move into serve.go too — they are unexported helpers used only by the server path. On `http.ListenAndServe` error, return `subcommands.ExitFailure` (today's `os.Exit(1)` equivalent). On clean ctx-cancel, return `subcommands.ExitSuccess`.
       - Move all needed imports from main.go into serve.go (`crypto/rand`, `database/sql`, `encoding/hex`, `fmt`, `io/fs`, `log/slog`, `net`, `net/http`, `os`, `path`, `path/filepath`, `strings`, `time`, plus all `kamacu/internal/*` imports and `kamacu/web`).

    3. Refactor `cmd/kamacu/main.go` to be a thin dispatcher. The new `main()` body is:
       - `subcommands.Register(subcommands.HelpCommand(), "")` — library's built-in help; gives bare `kamacu` a sensible default.
       - `subcommands.Register(serveCmd{}, "")` — the server.
       - `flag.Parse()` — library owns top-level flag parsing.
       - `os.Exit(int(subcommands.Execute(context.Background())))` — dispatches and propagates exit status.
       - Imports slim to: `context`, `flag`, `os`, `github.com/google/subcommands`. Nothing else.

    4. Update `Makefile` line 15: `dev-backend:` target's body changes from `go run ./cmd/kamacu` to `go run ./cmd/kamacu serve`. Build target on line 9 (`go build -o bin/kamacu ./cmd/kamacu`) is UNCHANGED.

    5. Update `scripts/smoke.sh` line 27: `./bin/kamacu --addr "$ADDR" --db "$WORK/k.db" &` becomes `./bin/kamacu serve --addr "$ADDR" --db "$WORK/k.db" &`.

    6. Update `README.md` lines 37-38: the run instructions change from bare `./bin/kamacu` to `./bin/kamacu serve`. Specifically line 38 (`./bin/kamacu    # starts the server; open the printed localhost URL in a browser`) becomes `./bin/kamacu serve    # starts the server; open the printed localhost URL in a browser`. Line 42's prose mentioning "`bin/kamacu` binary" stays — that's describing the binary file, not the invocation; no edit needed unless the surrounding sentence reads awkwardly.

    7. Do NOT create `cmd/kamacu/mcp.go` — that is Plan 02's file. main.go registers ONLY `serveCmd{}` and `subcommands.HelpCommand()` in this plan.
  </action>
  <verify>
    <automated>
      set -e
      grep -q 'github.com/google/subcommands v1.2.0' go.mod
      test -f cmd/kamacu/serve.go
      # main.go is now thin: no ListenAndServe, no ensureLoopback, no hostCheck in main.go
      ! grep -q 'http.ListenAndServe' cmd/kamacu/main.go
      ! grep -q 'func ensureLoopback' cmd/kamacu/main.go
      ! grep -q 'func hostCheck' cmd/kamacu/main.go
      # serve.go has them now
      grep -q 'http.ListenAndServe' cmd/kamacu/serve.go
      grep -q 'func ensureLoopback' cmd/kamacu/serve.go
      grep -q 'func hostCheck' cmd/kamacu/serve.go
      # Makefile dev-backend target updated
      grep -q 'go run ./cmd/kamacu serve' Makefile
      # smoke.sh updated
      grep -q './bin/kamacu serve --addr' scripts/smoke.sh
      # README updated
      grep -q './bin/kamacu serve' README.md
      # Build is clean
      go build -o /tmp/kamacu-plan01 ./cmd/kamacu
      # Bare invocation prints help and exits non-zero
      ! /tmp/kamacu-plan01 >/dev/null 2>&1
      /tmp/kamacu-plan01 2>&1 | grep -qi 'usage:'
      # `serve --help` shows the 5 flags
      /tmp/kamacu-plan01 serve --help 2>&1 | grep -q -- '--addr'
      /tmp/kamacu-plan01 serve --help 2>&1 | grep -q -- '--db'
      /tmp/kamacu-plan01 serve --help 2>&1 | grep -q -- '--claude-bin'
      /tmp/kamacu-plan01 serve --help 2>&1 | grep -q -- '--dev-origin'
      /tmp/kamacu-plan01 serve --help 2>&1 | grep -q -- '--insecure-allow-remote'
    </automated>
  </verify>
  <done>
    - go.mod contains `github.com/google/subcommands v1.2.0`
    - cmd/kamacu/serve.go exists with serveCmd implementing all 5 subcommands.Command methods; today's entire main() body lives in serveCmd.Execute
    - cmd/kamacu/main.go is <30 lines, contains zero http.ListenAndServe / ensureLoopback / hostCheck / sweepOrphanTmux definitions, registers ONLY serveCmd + HelpCommand
    - Makefile dev-backend target uses `go run ./cmd/kamacu serve`
    - scripts/smoke.sh uses `./bin/kamacu serve --addr ...`
    - README.md run instructions use `./bin/kamacu serve`
    - `go build -o /tmp/kamacu-plan01 ./cmd/kamacu` succeeds
    - bare `/tmp/kamacu-plan01` exits non-zero and prints help containing `Usage:` and `serve`
    - `/tmp/kamacu-plan01 serve --help` shows all 5 expected flags with their original defaults
  </done>
</task>

<task type="auto">
  <name>Task 2: Verify byte-for-byte behavior preservation via smoke.sh and go test</name>
  <files>cmd/kamacu/main.go, cmd/kamacu/serve.go, scripts/smoke.sh</files>
  <read_first>
    - `cmd/kamacu/serve.go` (post-Task-1) — confirm the body's call order matches today's main.go exactly: ensureLoopback gate → migrate.Prepare → os.MkdirAll → store.Open → store.Migrate → migrate.Complete (conditional) → BackfillProjectIcons → BackfillWorkspaces → BackfillAgents → BackfillAgentExtraParams → BackfillOpenCodeAgent → opencode.InstallPlugin → tmux.WriteConfig → tokenBytes/rand.Read → originPatterns → wtSvc/mgr construction → mgr.SetAgentConfig → tmuxClient → mux + Routes/SessionRoutes/etc → sweepOrphanTmux → reaper goroutine → http.ListenAndServe. Ordering is load-bearing for the migration and backfill logic.
    - `scripts/smoke.sh` — the full end-to-end flow (create project → create task → move → restart → persistence → SPA serving) is the regression net; it must complete with `SMOKE OK`.
    - `cmd/kamacu/main.go` (post-Task-1) — re-confirm only serveCmd + HelpCommand are registered.
  </read_first>
  <action>
    Run the project's two regression nets to prove byte-for-byte behavior preservation across the refactor:

    1. `make build` — runs `npm`/Vite (frontend) and `go build` (backend). Confirms the binary compiles end-to-end with the embed.

    2. `./scripts/smoke.sh` — builds, starts `./bin/kamacu serve --addr 127.0.0.1:7402 --db "$WORK/k.db"`, exercises the create-project → invalid-repo-400 → create-task → move → patch → restart → persistence → SPA-serving flow, expects `SMOKE OK`. If the script fails on the bare `./bin/kamacu` step it indicates an edit was missed in Task 1.

    3. `go test ./...` — runs the entire Go test suite. Zero failures expected; the bare-main shape was never asserted on by tests (all integration tests use `httptest.Server`, not the binary), so the refactor is transparent.

    4. Manual sanity: start the server on a throwaway port and curl healthz, confirming the full mux + SPA fallback + middleware stack survives the move:
       - `./bin/kamacu serve --addr 127.0.0.1:7403 --db /tmp/kamacu-plan01-verify.db &`
       - Wait briefly, then `curl -fsS http://127.0.0.1:7403/api/healthz` — expect `{"status":"ok"}`.
       - `curl -fsS http://127.0.0.1:7403/` — expect HTML (SPA fallback works).
       - `kill` the background process.

    If any check fails, the cause is misplaced logic in serveCmd.Execute (a missing call, a reordered block, or a wrong flag field reference). Diff `cmd/kamacu/serve.go` against the PRE-refactor `cmd/kamacu/main.go` body and confirm 1:1 ordering.
  </action>
  <verify>
    <automated>
      set -e
      make build
      ./scripts/smoke.sh
      go test ./...
      # Manual sanity
      ./bin/kamacu serve --addr 127.0.0.1:7403 --db /tmp/kamacu-plan01-verify.db &
      SRV=$!
      for _ in $(seq 1 50); do
        curl -fsS http://127.0.0.1:7403/api/healthz >/dev/null 2>&1 && break
        sleep 0.1
      done
      test "$(curl -fsS http://127.0.0.1:7403/api/healthz)" = '{"status":"ok"}'
      curl -fsS http://127.0.0.1:7403/ | grep -qi '<html'
      kill $SRV
      wait $SRV 2>/dev/null || true
      rm -f /tmp/kamacu-plan01-verify.db
    </automated>
  </verify>
  <done>
    - `make build` exits 0
    - `./scripts/smoke.sh` prints `SMOKE OK`
    - `go test ./...` exits 0 with zero failures
    - `curl http://127.0.0.1:7403/api/healthz` returns `{"status":"ok"}` (full mux + middleware stack works)
    - `curl http://127.0.0.1:7403/` returns HTML (SPA fallback works)
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| user shell → binary | User invokes `kamacu serve [flags]` (or bare `kamacu`); argv crosses into the process. SAME boundary as today — Phase 06 only changes which argv shape starts the server, not the boundary itself. |

## STRIDE Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation Plan |
|-----------|----------|-----------|----------|-------------|-----------------|
| T-06-01 | Tampering | cmd/kamacu/main.go dispatcher | low | accept | Flag set is byte-for-byte identical to today's `flag.X` calls — `serve` simply names the existing path. No new parsing surface. Verified by `./bin/kamacu serve --help` showing the same 5 flags with the same defaults. |
| T-06-02 | Elevation of privilege | scripts/Makefile/README break-clean | low | accept | D-04 intentionally breaks bare `kamacu` invocation. Any external script that relied on the old shape fails loudly with non-zero exit + help text — this is the intended posture (single-user, fresh-per-start token). NOT a regression in security; it is a stricter contract. |
| T-06-SC | Tampering | go.mod adds `github.com/google/subcommands v1.2.0` | low | accept | Package Legitimacy Audit in 06-RESEARCH.md § "Package Legitimacy Audit" verified the package as OK (Google-maintained, 7+ years stable, zero transitive deps). Direct source verification at v1.2.0 tag. No `[SUS]` / `[SLOP]` verdicts; no blocking human checkpoint required. |

</threat_model>

<verification>
- `go.mod` includes `github.com/google/subcommands v1.2.0` as a direct requirement
- `cmd/kamacu/serve.go` defines a `serveCmd` type implementing all five `subcommands.Command` interface methods
- `cmd/kamacu/main.go` registers ONLY `serveCmd{}` and `subcommands.HelpCommand()`, contains no `http.ListenAndServe` / `ensureLoopback` / `hostCheck` / `sweepOrphanTmux`, and is shorter than 30 lines
- Bare `./bin/kamacu` prints help and exits non-zero (D-04 break-clean)
- `./bin/kamacu serve --help` shows all 5 expected flags with original defaults
- `./scripts/smoke.sh` prints `SMOKE OK`
- `go test ./...` passes
- Manual curl of `/api/healthz` and `/` against a running `kamacu serve` instance both succeed
</verification>

<success_criteria>
Phase 06 D-01..D-05 fully delivered by this plan:
- D-01: `github.com/google/subcommands` adopted, `cmd/kamacu/main.go` uses the Command interface
- D-02: main.go is thin (register + dispatch only); today's server body lives in `cmd/kamacu/serve.go`
- D-03: `kamacu serve` runs the HTTP server; bare `kamacu` prints help
- D-04: break-clean — README/Makefile/scripts updated, no transitional shim
- D-05: only `serve` is registered (NO `mcp` Command in this plan — that is Plan 02's responsibility)
</success_criteria>

<output>
Create `.planning/phases/06-mcp-subcommand-foundation/06-01-SUMMARY.md` when done
</output>

## Artifacts this phase produces

This plan creates the following new symbols / files / struct fields (consumed by Phase 07+ plan-review-convergence source-grounding):

- **CLI command (new):** `serveCmd` struct in `cmd/kamacu/serve.go` implementing `subcommands.Command`
  - Methods: `Name()`, `Synopsis()`, `Usage()`, `SetFlags(*flag.FlagSet)`, `Execute(ctx, *flag.FlagSet, ...any) subcommands.ExitStatus`
  - Fields: `addr string`, `db string`, `claudeBin string`, `devOrigins []string`, `insecureAllowRemote bool`
- **Helper functions relocated** (from main.go to serve.go, unchanged behavior): `sweepOrphanTmux(parent context.Context, db *sql.DB, tmuxClient tmux.Client)`, `ensureLoopback(addr string) error`, `hookBaseURL(addr string) string`, `hostCheck(next http.Handler) http.Handler`
- **Dependency (new in go.mod):** `github.com/google/subcommands v1.2.0` (direct)
- **New file paths:** `cmd/kamacu/serve.go`
- **Refactored entry point:** `cmd/kamacu/main.go` — new `main()` body is the dispatcher (register + flag.Parse + os.Exit(subcommands.Execute))

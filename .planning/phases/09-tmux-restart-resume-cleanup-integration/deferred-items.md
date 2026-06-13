# Deferred Items — Phase 09

## 09-01 (out-of-scope discoveries)

- **`internal/api` package does not build during parallel execution.**
  - Observed: `go test ./internal/api/` fails — `not enough arguments in call to SessionRoutes` (test files call the 3-arg signature; `internal/api/sessions.go` already declares the 4-arg `tmux.Client` signature).
  - Root cause: sibling parallel-executor agents have in-flight uncommitted changes to `cmd/kangent/main.go`, `internal/api/sessions.go`, `internal/api/sessions_test.go`, `internal/session/*`, `internal/settings/*` — none of which are 09-01 plan files.
  - Scope: 09-01 only touches `internal/tmux/{tmux,tmux_test}.go` and `internal/store/migrations/00006_*.sql`; both packages build, test, and vet cleanly in isolation.
  - Action: NOT fixed by 09-01 (out of scope per executor SCOPE BOUNDARY). The orchestrator validates the full build/hooks once all parallel agents complete.

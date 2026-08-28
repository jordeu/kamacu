# Phase 14 Deferred Items

## Pre-existing test failures (out of scope — discovered during 14-01 execution)

- `TestInput_Happy_WritesAndAppendsCR`, `TestInput_TrailingLF_TranslatedToCR`,
  `TestInput_EmptyMessage_WritesBareCR` in `internal/api/sessions_test.go` fail with
  `bytes_written = 15, want 13` (empty-message bracketed-paste case) — verified to fail
  identically at the pre-phase baseline commit `ddd00a4` via a throwaway worktree, i.e.
  NOT caused by Phase 14 work. Root cause lives in the `POST /api/sessions/{id}/input`
  endpoint's bracketed-paste wrapping (quick task 260728-t4c area). Needs a dedicated
  debug pass; do not re-run hoping it resolves.
- `gofmt -l` flags `cmd/kamacu/serve.go` (struct-field alignment drift in the `serveCmd`
  struct at the top of the file, lines ~39-50) — pre-existing, untouched by Phase 14's
  one-line registry addition. Fix in a formatting-only cleanup commit if desired.
- Full `internal/api` suite takes ~190s (PTY/tmux integration tests) — plan verification
  correctly uses the targeted `-run 'TestGetGlobal|TestPutGlobal'` filter.

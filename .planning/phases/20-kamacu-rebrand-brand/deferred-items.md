# Deferred Items — Phase 20

Out-of-scope discoveries logged during execution (NOT fixed — unrelated to the current task's changes).

## Pre-existing gofmt formatting drift (not caused by the rename)

These 8 files were already `gofmt`-dirty at the phase base commit (`0672e3b`), verified via `git show HEAD:<file> | gofmt -l`. The `"kangent/` → `"kamacu/` import rewrite did not shift any import ordering, so it did not cause or worsen these. `go build`/`vet`/`test ./...` are unaffected (they do not require gofmt). Left untouched to avoid out-of-scope churn; a dedicated `gofmt -w ./...` pass is the right home.

- `cmd/kamacu/main_test.go` (was `cmd/kangent/main_test.go` at HEAD)
- `internal/api/icons.go`
- `internal/github/prlist.go`
- `internal/github/service.go`
- `internal/reaper/reaper_test.go`
- `internal/settings/settings_test.go`
- `internal/ws/handler_test.go`
- `internal/ws/integration_test.go`

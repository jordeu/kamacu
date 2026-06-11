---
phase: 06-settings-polish
plan: 1
subsystem: api
tags: [settings, sqlite, goose, validation, git-ref, tokenizer, go]

# Dependency graph
requires:
  - phase: 01-foundation
    provides: goose migration pipeline, store.Open/Migrate, API route-registration + respond helpers
  - phase: 03-worktrees
    provides: worktree.Slug constructive charset (reimplemented as sanitizeTitle for {title})
provides:
  - settings KV table (migration 00004, no seeded rows — absent row = code default)
  - internal/settings package - keys, Defaults, Get/GetAll/Set, Validate, CheckRefFormat, ExpandTemplate, Tokenize, ExpandHome
  - GET /api/settings + PUT /api/settings/{key} with canonical validation copy through the {"error"} pipe
  - AllowedShells single source of truth (validation + GET options array)
affects: [06-02, 06-03, 06-04, settings, agent-spawn, worktree-creation]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Settings defaults in code: only sql.ErrNoRows falls back; stored empty string is a real value"
    - "Per-key PUT isolates validation failures per field (SET-04)"
    - "Canonical UI error copy lives in Go error strings, mirrored verbatim by the frontend"
    - "Branch-template legality via git check-ref-format refs/heads/<sample> (positional form, repo-less)"

key-files:
  created:
    - internal/store/migrations/00004_settings.sql
    - internal/settings/settings.go
    - internal/settings/home.go
    - internal/settings/validate.go
    - internal/settings/template.go
    - internal/settings/tokenize.go
    - internal/settings/settings_test.go
    - internal/settings/validate_test.go
    - internal/settings/template_test.go
    - internal/settings/tokenize_test.go
    - internal/api/settings.go
    - internal/api/settings_test.go
  modified:
    - internal/api/routes.go
    - cmd/kangent/main.go

key-decisions:
  - "Validate runs in the PUT handler (400) AND inside Set (validate-before-write) so the BRANCH-02 guarantee holds for any caller while validation vs storage errors stay distinct"
  - "{title} sanitizer cap fixed at 100 chars (research OQ3 resolved)"
  - "Tokenize is lenient on unclosed quotes (rest-of-string one token) — no UI-SPEC error copy exists for the pass-through field"
  - "Task-1 Validate shipped as a nil stub replaced by Task 2, keeping Set's call shape stable across the TDD sequence"

patterns-established:
  - "Pattern: settings stored raw (worktree base keeps ~ form); ExpandHome only at validation exists-check and at use"
  - "Pattern: sample-expansion invariance — template legal with sample token values is legal for all task values"

requirements-completed: [SET-02, SET-04, AGENT-02, BRANCH-02]

# Metrics
duration: 11min
completed: 2026-06-11
---

# Phase 6 Plan 1: Settings Foundation Summary

**SQLite-backed settings KV store (migration 00004) with code defaults, per-key validation carrying the UI-SPEC canonical error copy, branch-template expansion + git ref validation, quote-aware extra-params tokenizer, and the GET/PUT REST surface**

## Performance

- **Duration:** 11 min
- **Started:** 2026-06-11T15:00:54Z
- **Completed:** 2026-06-11T15:11:56Z
- **Tasks:** 3 (all TDD)
- **Files modified:** 14

## Accomplishments

- Settings persist in SQLite over the API: migration 00004 KV table, upsert Set, defaults in code (absent row = default, no seeded rows — new settings never need a migration)
- AGENT-02 / D-51 reversal landed: `agent_extra_params` defaults to `--dangerously-skip-permissions`; a stored empty string is preserved as "no extra parameters" (Pitfall 1 covered by explicit round-trip test)
- BRANCH-02 save-time half: mandatory validation order (empty → unknown token → missing {id} → git legality) with all seven canonical error strings verbatim; invalid templates never stored (proven by 400-then-GET test)
- CheckRefFormat rejects leading-dash names app-side (git-legal but argv hazard) then defers to `git check-ref-format refs/heads/<name>` positional form
- ExpandTemplate ({slug}/{id}/{title}, single pass, 100-char title cap) and lenient quote-aware Tokenize, both dependency-free of other internal packages
- REST surface live: GET returns all four keys with value+default (+options for shell from the same AllowedShells slice used in validation); per-key PUT isolates failures (SET-04)
- ExpandHome relocated from cmd/kangent/main.go to internal/settings; both main.go call sites updated

## Task Commits

Each TDD task produced a RED and a GREEN commit:

1. **Task 1: Migration 00004 + settings store core + ExpandHome relocation** - `6653d59` (test), `b5a9dc1` (feat)
2. **Task 2: Validation, template expansion, tokenizer** - `d047a10` (test), `7df8ed8` (feat)
3. **Task 3: Settings REST API** - `4b30fe9` (test), `5a086ae` (feat)

No REFACTOR commits were needed — implementations came out clean against the tests.

## Files Created/Modified

- `internal/store/migrations/00004_settings.sql` - settings KV table, no seeds
- `internal/settings/settings.go` - key constants, Defaults map, Get/GetAll/Set (upsert, ErrNoRows-only defaulting)
- `internal/settings/home.go` - relocated ExpandHome
- `internal/settings/validate.go` - Validate, CheckRefFormat, AllowedShells, canonical error copy
- `internal/settings/template.go` - ExpandTemplate + sanitizeTitle (100-char cap)
- `internal/settings/tokenize.go` - quote-aware lenient tokenizer (research Pattern 4 verbatim)
- `internal/settings/*_test.go` - table-driven coverage of every behavior row, exact-error-string assertions
- `internal/api/settings.go` - SettingsRoutes: GET /api/settings, PUT /api/settings/{key}
- `internal/api/settings_test.go` - httptest coverage incl. nothing-stored-on-400 and persistence SELECT
- `internal/api/routes.go` - SettingsRoutes registered inside Routes
- `cmd/kangent/main.go` - local expandHome removed; settings.ExpandHome at both call sites

## Decisions Made

- Validation runs in the handler (clean 400) and again inside Set (validate-before-write invariant kept for non-HTTP callers) — the plan offered either; doing both costs one cheap call and keeps both guarantees
- `{title}` cap = 100 chars (research OQ3 recommendation adopted)
- Unclosed quotes tokenize leniently (rest-of-string as one token) — the spec-minimal reading; no canonical copy exists for this field
- Task 1 shipped validate.go as a nil stub so Set's signature/behavior was final from the start; Task 2's RED tests then failed on assertions (not compilation) for validation logic

## Deviations from Plan

None - plan executed exactly as written.

(Two comment-text rewordings were made so the plan's negative grep acceptance criteria pass — comments mentioning the forbidden `--branch` flag and the worktree package path were rephrased; zero code behavior involved.)

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Wave 2 plans can now wire `settings.Get/GetAll`, `ExpandTemplate`, `CheckRefFormat`, `Tokenize`, and `ExpandHome` into the spawn/creation call sites (sessions.go, provisionWorktree) and build the settings page against GET/PUT
- `AllowedShells` is the SHELL-02 seam: adding shells later is a one-line data change
- No behavior change anywhere else yet — all existing call sites still use their v1.0 hardcodes, exactly as the plan scoped

---
*Phase: 06-settings-polish*
*Completed: 2026-06-11*

## Self-Check: PASSED

All 12 created files verified on disk; all 6 task commits verified in git log.

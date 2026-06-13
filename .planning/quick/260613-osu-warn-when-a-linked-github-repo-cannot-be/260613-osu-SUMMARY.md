---
phase: quick-260613-osu
plan: 01
subsystem: api
tags: [github, projects, patch, soft-save, advisory]
requires:
  - "internal/github.ValidateRepo (verified bool) + github.Available()"
  - "PATCH /api/projects/{id} degrade-don't-break save (Phase 10-02)"
  - "ProjectSettingsDialog.tsx already-wired updated.verify_state advisory (Phase 10)"
provides:
  - "PATCH /api/projects/{id} response-only verify_state advisory (no_gh | unverifiable)"
  - "projectUpdateResponse wrapper (Project + omitempty verify_state)"
  - "Project.verify_state?: string optional TS field"
affects:
  - "ProjectSettingsDialog.tsx dormant advisory is now activated (no FE code change)"
tech-stack:
  added: []
  patterns:
    - "Response-only embedded-struct wrapper with omitempty for a transient, non-persisted field — keeps GET/list/create bodies byte-for-byte unchanged"
key-files:
  created: []
  modified:
    - "internal/api/projects.go"
    - "internal/api/projects_test.go"
    - "web/src/api/types.ts"
decisions:
  - "verify_state computed AFTER scanProject from repoSet/repoVerified locals carried out of the SET-clause block; only the non-empty github_repo branch sets repoSet=true, so unlink and non-repo PATCHes never emit the field"
  - "Test branches on github.Available() (mirrors TestGithubStatus) so it is host-independent: unverifiable on a gh-present host, no_gh on a gh-absent host"
metrics:
  duration: "~4 min"
  completed: "2026-06-13"
  tasks: 3
  files: 3
---

# Phase quick-260613-osu Plan 01: Warn When a Linked GitHub Repo Cannot Be Verified Summary

Activated the dormant "soft-save-with-warning" advisory (GHPRJ-03 / D-11): PATCH `/api/projects/{id}` now emits a transient `verify_state` field (`unverifiable` or `no_gh`) when a syntactically-valid GitHub repo is saved but `gh` cannot confirm it — without ever blocking the save. The already-wired `ProjectSettingsDialog.tsx` advisory now fires with zero frontend behavior change.

## What Was Built

- **`projectUpdateResponse` wrapper** (`internal/api/projects.go`): embeds `Project` plus `VerifyState string \`json:"verify_state,omitempty"\``. omitempty makes the PATCH body byte-for-byte identical to a bare `Project` whenever `verify_state` is `""`.
- **`update` handler change**: captured the previously-discarded `verified` bool from `github.ValidateRepo`, recorded `repoSet`/`repoVerified` locals (set only in the non-empty `github_repo` branch), and computed `verify_state` after `scanProject` — `no_gh` when `!github.Available()`, else `unverifiable`, else `""`.
- **Host-independent test** `TestUpdateProjectVerifyState`: bogus-repo PATCH asserts the host-correct advisory (branch on `github.Available()`); description-only and unlink PATCHes assert the field is ABSENT.
- **TS type**: `Project.verify_state?: string` so the field the server now returns is typed (the dialog previously read it via an inline cast).

## Contract Preserved (no regressions)

- No DB column, no migration, no `scanProject`/`projectColumns` change, no `Project` JSON-tag change → GET `/api/projects`, POST, and the list response are byte-for-byte unchanged.
- The 400 (syntactically invalid ref still hard-blocks with the canonical copy), 404, and 500 paths are untouched.
- `TestUpdateProjectPartial` and all other api tests remain green.
- `ProjectSettingsDialog.tsx` is unmodified; it consumes the now-emitted `updated.verify_state` ("no_gh" → gh-degraded advisory, "unverifiable" → soft-verify advisory, keeps dialog open).

## Tasks

| Task | Name                                              | Commit  | Files                          |
| ---- | ------------------------------------------------- | ------- | ------------------------------ |
| 1    | RED — failing test for PATCH verify_state advisory | 8e83653 | internal/api/projects_test.go  |
| 2    | GREEN — emit verify_state via response-only wrapper | 932ee0b | internal/api/projects.go       |
| 3    | Add optional verify_state to Project TS interface  | 69a6915 | web/src/api/types.ts           |

## Verification

- `go test ./internal/api/` exits 0 — `TestUpdateProjectVerifyState` + `TestUpdateProjectPartial` + all existing project/github tests pass.
- `go build ./...` exits 0.
- `gofmt -l internal/api/projects.go internal/api/projects_test.go` prints nothing (clean).
- Host state: `gh` present on this host → bogus-repo PATCH yields `verify_state: "unverifiable"` (RED failure was exactly `verify_state="" want "unverifiable"`, confirming the field was previously discarded).

## Deviations from Plan

None - plan executed exactly as written (RED → GREEN → type addition).

## Known Stubs

None. The `verify_state` field is fully wired end to end: server computes and emits it, the TS type carries it, and the already-existing dialog renders it.

## Self-Check: PASSED

- Files: internal/api/projects.go, internal/api/projects_test.go, web/src/api/types.ts, 260613-osu-SUMMARY.md — all FOUND.
- Commits: 8e83653, 932ee0b, 69a6915 — all FOUND.

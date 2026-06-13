---
phase: quick-260613-ph5
plan: 01
subsystem: api
tags: [github, gh-cli, project-settings, validation, react, go]

# Dependency graph
requires:
  - phase: 10-github-foundations
    provides: internal/github (ParseRepoRef/ValidateRepo/Available), PATCH /api/projects partial update, ProjectSettingsDialog
  - phase: quick-260613-osu
    provides: verify_state soft-save advisory (now reverted by this task)
provides:
  - Mandatory hard-block GitHub repo-link validation on PATCH /api/projects (400, no persist, when gh cannot verify)
  - verify_state machinery fully removed (backend handler, frontend dialog, Project type)
  - Highlighted destructive error alert in ProjectSettingsDialog; 2xx always closes; github_repo sent only when changed
affects: [phase-11, phase-12, phase-13, pr-review, github]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Mandatory gh verification gate: ValidateRepo verified bool drives a 400 hard-block instead of a soft save"
    - "Partial-update frontend: send a field only when actually changed (github_repo omitted = backend leaves untouched)"
    - "Host-independent backend test: a bogus repo never verifies on any host, so the 400 holds without network determinism; valid-repo path exercised live"

key-files:
  created: []
  modified:
    - internal/api/projects.go
    - internal/api/projects_test.go
    - web/src/components/sidebar/ProjectSettingsDialog.tsx
    - web/src/api/mutations.ts
    - web/src/api/types.ts
    - web/dist/index.html

key-decisions:
  - "Reverse D-11 'never hard-block on gh' ONLY for the repo-link UX: a non-empty github_repo is now mandatory-verified; everything else (description, name, unlink, omitted field) still never touches gh"
  - "Not-found vs no-gh error copy chosen by github.Available() so the message is honest about why verification failed"
  - "Frontend sends github_repo only when changed, so a description-only edit can never be hard-blocked by a transient gh hiccup"

patterns-established:
  - "Pattern 1: gh verification is a hard gate (verified==false -> 400, row untouched) in the non-empty github_repo branch; explicit-'' unlink and omitted field bypass gh entirely"
  - "Pattern 2: the dialog closes on any 2xx and stays open on error with a highlighted destructive alert + preserved description draft"

requirements-completed: [GHPRJ-03]

# Metrics
duration: 8min
completed: 2026-06-13
---

# Phase quick-260613-ph5: Mandatory GitHub Repo-Link Validation Summary

**Made GitHub repo-linking in Project settings a mandatory gh-verified hard-block — an unverifiable ref now returns 400 without persisting, the dialog shows a highlighted destructive alert and stays open, and all 260613-osu soft-advisory (verify_state) machinery is removed end-to-end.**

## Performance

- **Duration:** 8 min
- **Started:** 2026-06-13T16:23:38Z
- **Completed:** 2026-06-13T16:31:31Z
- **Tasks:** 3
- **Files modified:** 6

## Accomplishments
- Backend `PATCH /api/projects/{id}` now hard-blocks a non-empty `github_repo` that `gh` cannot verify (400, row left untouched), with not-found vs no-gh copy chosen by `github.Available()`; a verified ref persists the gh-canonicalized `owner/name`.
- Removed the `projectUpdateResponse` struct and all `verify_state` computation; the success response is a plain `Project` again.
- Replaced `TestUpdateProjectVerifyState` with the host-independent `TestUpdateProjectMandatoryValidation` (unverifiable -> 400 not persisted; description-only -> 200; explicit unlink -> 200 NULL); the valid-repo -> 200 path is exercised live on this gh-authenticated host.
- Frontend dialog: dropped the soft-advisory warning state + helpers and `verify_state` handling, made `github_repo` optional and sent only when changed, made a 2xx save always close, and replaced both plain destructive errors with a highlighted destructive alert (rounded border + bg tint) while preserving the description draft.
- Trimmed `verify_state` from the `Project` type and rebuilt the embedded SPA.

## Task Commits

Each task was committed atomically:

1. **Task 1 (RED): mandatory-validation test replaces verify_state advisory** - `7b4cc02` (test)
2. **Task 1 (GREEN): hard-block unverifiable github repo links in PATCH** - `69119c2` (feat)
3. **Task 2: hard-block repo link in settings dialog with highlighted error** - `857db3b` (feat)
4. **Task 3: rebuild embedded SPA with mandatory repo-link dialog** - `38415e3` (chore)

_TDD Task 1 has two commits (test -> feat); no separate REFACTOR was needed._

## Files Created/Modified
- `internal/api/projects.go` - Removed `projectUpdateResponse`/verify_state; added `msgRepoNotFound`/`msgGHUnavailable` consts; mandatory `if !verified -> 400` hard-block in the non-empty github_repo branch; plain `writeJSON(w, 200, p)`.
- `internal/api/projects_test.go` - Deleted `TestUpdateProjectVerifyState` and the now-unused `internal/github` import; added `TestUpdateProjectMandatoryValidation`; updated `TestUpdateProjectPartial`'s seed ref to a verifiable one (`cli/cli`) since linking is now mandatory-verified.
- `web/src/components/sidebar/ProjectSettingsDialog.tsx` - Removed `warning` state, `ReactNode` import, `softVerifyWarning`/`ghDegradedWarning`, and verify_state handling; `repoChanged` gate; always-close on 2xx; highlighted destructive alert on both error sites.
- `web/src/api/mutations.ts` - `useUpdateProjectSettings` `github_repo` now optional; included in PATCH body only when defined.
- `web/src/api/types.ts` - Removed `verify_state?: string` from `Project`.
- `web/dist/index.html` - Regenerated embed pointer for the new hashed assets (`vite build`; hashed assets are gitignored per repo convention).

## Decisions Made
- Reverse D-11's "never hard-block on gh" ONLY for the repo-link UX (user-decided): a non-empty `github_repo` is mandatory-verified; description/name/unlink/omitted paths still never invoke gh.
- Choose the 400 message by `github.Available()` (repo-not-found vs gh-unavailable) so the user learns why it failed.
- Send `github_repo` from the dialog only when it actually changed, so a description-only edit never re-validates and can't be blocked by a transient gh failure.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Updated `TestUpdateProjectPartial` seed ref to a verifiable repo**
- **Found during:** Task 1 (GREEN)
- **Issue:** The pre-existing `TestUpdateProjectPartial` seeded a link with `"owner/name"` and asserted a 200, then proved a later syntactic-reject PATCH (`not-a-repo`) does not clobber that stored link. Under the new mandatory validation, `"owner/name"` does not verify on this gh-authenticated host, so the seed PATCH now returns 400 and the test failed (`seed link status = 400, want 200`). The plan's Task 1 note assumed this test would pass unchanged; the assumption was invalid because the seed used an unverifiable ref. The test's intent (a rejected PATCH must not clobber an existing link) is still valid and must be preserved.
- **Fix:** Changed the seed ref (and its `assertDBProject` expectation) from `"owner/name"` to `"cli/cli"`, which is already proven verifiable earlier in the same test. The syntactic-reject (`not-a-repo` -> 400 + canonical copy) and the "row unchanged" assertions are otherwise untouched.
- **Files modified:** `internal/api/projects_test.go`
- **Verification:** `go test ./internal/api/` exits 0 (both `TestUpdateProjectMandatoryValidation` and `TestUpdateProjectPartial` pass).
- **Committed in:** `69119c2` (Task 1 GREEN commit)

---

**Total deviations:** 1 auto-fixed (1 bug — a pre-existing test incompatible with the mandatory-validation contract).
**Impact on plan:** Minimal and necessary. The fix preserves the original test's intent (rejected PATCH does not clobber an existing link) while making it consistent with mandatory verification. No scope creep.

## Issues Encountered
- The full `go test ./internal/api/` run takes ~80s because several tests now make live `gh repo view cli/cli` network calls (mandatory verification + the `cli/cli` seed). This is expected on a gh-authenticated host and matches the plan's "valid-repo path is exercised live" intent; no action needed.

## User Setup Required
None - no external service configuration required. `gh` is present and authenticated on this host (account `jordeu`).

## Next Phase Readiness
- A linked repo is now guaranteed gh-verified, so downstream PR phases (11-13) never carry a phantom link.
- All four verify gates green: `go test ./internal/api/` (0), `go build ./...` (0), `cd web && npm run build` (0), `gofmt -l` clean on changed Go files. No `verify_state`/soft-advisory references remain in `internal/api/` (outside a docstring word) or `web/src/`.
- No blockers.

---
*Phase: quick-260613-ph5*
*Completed: 2026-06-13*

## Self-Check: PASSED

- All 6 modified files present on disk + SUMMARY.md created.
- All 4 task commits present in git history (`7b4cc02`, `69119c2`, `857db3b`, `38415e3`).

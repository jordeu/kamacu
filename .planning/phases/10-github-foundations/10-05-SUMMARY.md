---
phase: 10-github-foundations
plan: 05
subsystem: ui
tags: [react, typescript, github, gh-cli, settings, toggle, tanstack-query, gap-closure]

# Dependency graph
requires:
  - phase: 10-github-foundations (Plan 10-04)
    provides: "GET /api/github/status — always-200 endpoint returning { gh_available: bool } from github.Available()"
provides:
  - "useGithubStatus() query hook reading GET /api/github/status -> { gh_available }"
  - "gh-gated GitHub integration toggle on /settings: forced OFF (never shown on) when gh is missing, enable attempt blocked with install-gh guidance, behaves as before when gh present"
  - "Corrected GITHUB_INTEGRATION_HELP copy (Gap 2): no longer over-claims the app behaves exactly as before"
  - "Rebuilt embedded SPA (web/dist) so the single binary ships the gh-gated toggle + corrected copy"
affects: [11-pr-review-column, 12-pr-tasks, frontend-settings]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Frontend gh-availability gate: useGithubStatus() reads the always-200 status endpoint; ghAvailable = ghStatus?.gh_available === true treats loading/error/missing as not-available so the toggle never falsely shows on"
    - "Effective-state gating for a binary control: effectiveEnabled = enabled && ghAvailable drives `checked`, decoupling the displayed state from the stored KV so a stale 'on' row cannot show on when gh is gone"
    - "Enable-guard in onCheckedChange: a blocked enable surfaces a muted install message instead of committing, while turn-off always commits — the Switch stays interactive (no `disabled`) so the click can surface the guidance"

key-files:
  created: []
  modified:
    - web/src/api/queries.ts
    - web/src/pages/SettingsPage.tsx
    - web/dist/index.html

key-decisions:
  - "Treat a loading/errored status query as gh-not-available (ghStatus?.gh_available === true), so the toggle is never shown on before availability is known — fail safe toward OFF"
  - "Keep the Switch interactive (no `disabled` prop): a disabled switch would swallow the click and never surface the install-gh message; the onCheckedChange guard + effectiveEnabled fully satisfy the gap"
  - "Effective checked state = enabled && ghAvailable so a stale stored 'on' row never displays as on when gh is missing (GHSET-01/GHSET-03)"

patterns-established:
  - "Frontend reads gh availability via useGithubStatus() and gates GitHub controls' default/enable behavior on it — the single client read of is-gh-installed"

requirements-completed: [GHSET-01, GHSET-02, GHSET-03]

# Metrics
duration: 3min
completed: 2026-06-13
---

# Phase 10 Plan 05: gh-Gated GitHub Toggle + Corrected Help Copy Summary

**The /settings GitHub integration toggle is now gh-aware: it consumes the Plan-10-04 `GET /api/github/status` endpoint via a new `useGithubStatus()` hook, shows OFF (never on) and blocks enable with install-gh guidance when `gh` is absent, behaves exactly as before when `gh` is present — and the help copy no longer over-claims. Closes the frontend half of Gap 1 plus Gap 2.**

## Performance

- **Duration:** ~3 min
- **Started:** 2026-06-13T15:29:27Z
- **Completed:** 2026-06-13T15:32:00Z
- **Tasks:** 3 (all `type="auto"`)
- **Files modified:** 3 (0 created, 3 modified)

## Accomplishments

- **`useGithubStatus()` hook (Task 1):** Added to `web/src/api/queries.ts`, mirroring the existing `useProjectGithubOrigin` shape — `useQuery` on `["github-status"]` reading `GET /api/github/status` typed as `{ gh_available: boolean }`. `useQuery`/`get` were already imported; no duplicate imports.
- **gh-gated toggle (Task 2, Gap 1 frontend half):** `GithubSection` calls `useGithubStatus()`, derives `ghAvailable = ghStatus?.gh_available === true` (loading/error/missing all treated as not-available), and computes `effectiveEnabled = enabled && ghAvailable` passed to `<Switch checked={effectiveEnabled}>` — so the toggle is never shown on when `gh` is missing, even if a stale stored row says "on". The `onCheckedChange` guard blocks an enable attempt when `gh` is unavailable (sets the install message, does not commit); turning off always commits. When `gh` is missing the help line shows the install-gh guidance even before any click.
- **Corrected help copy (Task 2, Gap 2):** `GITHUB_INTEGRATION_HELP` now reads exactly `Show GitHub features across Kangent. Turn off to hide all GitHub UI.` — the trailing `— the app behaves exactly as it did before.` clause is gone (string absent from the file).
- **Rebuilt embedded SPA (Task 3):** `cd web && npm run build` regenerated `web/dist`, and `go build ./...` is green — the single binary now serves the gh-gated toggle and corrected copy. Only the tracked `web/dist/index.html` embed placeholder is committed (`web/dist/assets/` is gitignored; the `//go:embed dist` picks up freshly-built assets at build time — 10-03 precedent).

**Host state:** `gh` is currently absent on this host (confirmed `command -v gh` returns nothing), so the toggle correctly displays OFF and is un-enableable with the install-gh message.

## Task Commits

Each task was committed atomically:

1. **Task 1: Add useGithubStatus() query hook** - `53349f8` (feat)
2. **Task 2: gh-gate the GitHub toggle + fix help copy** - `c60f0b1` (feat)
3. **Task 3: Rebuild the embedded SPA** - `0332f78` (build)

**Plan metadata:** committed separately (docs: complete plan).

## Files Created/Modified

- `web/src/api/queries.ts` — Modified. Added `useGithubStatus()` reading `GET /api/github/status -> { gh_available }`.
- `web/src/pages/SettingsPage.tsx` — Modified. Imported `useGithubStatus`; corrected `GITHUB_INTEGRATION_HELP`; added `GITHUB_GH_MISSING_HELP`; `GithubSection` now derives `ghAvailable`/`effectiveEnabled`, guards enable, and renders the install-gh line when gh is missing.
- `web/dist/index.html` — Modified. Rebuilt embed placeholder so the binary ships the change.

## Decisions Made

- **Fail safe toward OFF:** `ghAvailable` is `ghStatus?.gh_available === true`, so a still-loading or errored status query is treated as not-available — the toggle never flashes on before availability is known.
- **Keep the Switch interactive (no `disabled`):** a disabled switch would swallow the click and never surface the install-gh message. The `onCheckedChange` guard plus `effectiveEnabled` fully satisfy the gap while letting the click reveal the guidance.
- **`effectiveEnabled = enabled && ghAvailable` drives `checked`:** decouples the displayed state from the stored KV so a stale `"on"` row cannot show on when `gh` is gone (GHSET-01/GHSET-03).

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None. Both Task 2 and Task 3 builds were green; the only build output was the pre-existing chunk-size warning (>500 kB bundle), which is a Vite advisory, not an error (exit 0). Gated on `npm run build` per the gap-closure note (the repo has pre-existing lint errors out of scope, per 10-03-SUMMARY), and `npm run lint` was deliberately not run.

## User Setup Required

None - the toggle reads gh availability from the existing always-200 status endpoint; no external service configuration. (If the user wants the toggle enableable, they must install the GitHub CLI `gh` on the host — surfaced in-app by the install-gh guidance, by design.)

## Next Phase Readiness

- Gap 1 is now fully closed (backend in 10-04, frontend here): the `/settings` toggle is gh-aware end to end.
- Gap 2 is closed: the over-claiming copy is gone.
- Phase 11 (PR Review column) can reuse `useGithubStatus()` as the single client read of is-gh-installed alongside the existing `useSettings()` `github_integration === "on"` OFF-cascade gate.
- Behavioral re-check (human, post-build): with `gh` absent the toggle shows OFF and a click surfaces the install message without enabling; restore `gh` on PATH and the toggle behaves normally. This is the live host-state check the verifier flagged as human-testable.

## Self-Check: PASSED

- FOUND: web/src/api/queries.ts (contains `useGithubStatus`, `/api/github/status`, `gh_available`)
- FOUND: web/src/pages/SettingsPage.tsx (exact corrected copy present; over-claim clause absent; `useGithubStatus`, `gh_available`, `ghAvailable`, install message, `checked={effectiveEnabled}` present; `checked={enabled}` absent)
- FOUND: web/dist/index.html
- FOUND commit: 53349f8 (feat — Task 1)
- FOUND commit: c60f0b1 (feat — Task 2)
- FOUND commit: 0332f78 (build — Task 3)
- web build exit 0; go build ./... exit 0

---
*Phase: 10-github-foundations*
*Completed: 2026-06-13*

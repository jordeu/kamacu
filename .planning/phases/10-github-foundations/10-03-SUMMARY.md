---
phase: 10-github-foundations
plan: 03
subsystem: frontend
tags: [react, typescript, shadcn, switch, settings, dialog, github, off-cascade, tanstack-query]

# Dependency graph
requires:
  - phase: 10-github-foundations (Plan 01)
    provides: "github_integration settings KV (code-default 'on') + migration 00007 columns"
  - phase: 10-github-foundations (Plan 02)
    provides: "PATCH /api/projects/{id} partial update (description + github_repo) and GET /api/projects/{id}/github-origin prefill endpoint"
provides:
  - "shadcn Switch primitive (web/src/components/ui/switch.tsx) via the radix-ui umbrella, accent-blue checked track"
  - "GitHub section + integration Switch on /settings: on/off commits immediately via the settings KV, default ON, reverts with canonical error on failure"
  - "ProjectSettingsDialog: description (3-row, 280 cap, {n}/280 counter) + origin-prefilled, soft-validated mono repo field, one PATCH for both fields"
  - "Project settings ⋯ menu item (between Rename and Delete project) opening the dialog"
  - "OFF cascade: the GitHub repository field disappears from the dialog app-wide when github_integration === 'off'"
  - "Project type grown by description + github_repo; useUpdateProjectSettings mutation; useProjectGithubOrigin on-open query"
affects: [11-pr-review-column, 12-pr-tasks, 13-pr-cleanup]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "shadcn ui/* primitives use the radix-ui umbrella import (import { Switch as SwitchPrimitive } from 'radix-ui'), NOT the individual @radix-ui/react-* packages — matches select.tsx/label.tsx; no new dep needed"
    - "Switch commits immediately (no draft state machine): onCheckedChange mutates 'on'/'off' via useSaveSetting; the ['settings'] cache replace flips `checked`, so the toggle never sticks mid-flip and reverts for free on failure"
    - "OFF cascade is a render gate on the shared useSettings() value (github_integration === 'on'), so all GitHub UI flips atomically; when off the dialog preserves the hidden link by re-sending the existing github_repo"
    - "On-open prefill via an `enabled`-gated TanStack query (open && integrationOn) + staleTime: Infinity, so git is shelled only when the dialog opens, never on the project list (D-08)"
    - "Dialog open/reset + late-arriving origin prefill done via React's adjust-state-during-render pattern (change-tracker state), not setState-in-effect — satisfies react-hooks lint and avoids an extra render"

key-files:
  created:
    - web/src/components/ui/switch.tsx
    - web/src/components/sidebar/ProjectSettingsDialog.tsx
  modified:
    - web/src/api/types.ts
    - web/src/api/mutations.ts
    - web/src/api/queries.ts
    - web/src/pages/SettingsPage.tsx
    - web/src/components/sidebar/ProjectMenu.tsx
    - web/dist/index.html

key-decisions:
  - "Hand-authored switch.tsx from the radix-ui umbrella package (already a dep) rather than `npx shadcn add switch`, because every existing ui/* primitive imports from 'radix-ui' (not @radix-ui/react-*); the CLI would have written a non-matching import style"
  - "Reset/prefill uses the adjust-state-during-render pattern instead of the RenameProjectDialog useEffect prefill, to avoid introducing a NEW react-hooks/set-state-in-effect lint error in my own file (the codebase has 18 pre-existing instances of that rule, deferred)"
  - "Soft-verify and gh-degraded muted advisories are wired (warning state + muted render path keyed on a future updated.verify_state flag) but never fire today, because Plan 02's server returns a plain 200 with no verify flag — the success path closes the dialog (UI-SPEC explicitly allows 'closes immediately, the note is non-blocking')"

requirements-completed: [GHSET-01, GHSET-02, GHPRJ-01, GHPRJ-02, GHPRJ-03]

# Metrics
duration: 8min
completed: 2026-06-13
---

# Phase 10 Plan 03: GitHub Integration Switch + Project Settings Dialog Summary

**The two user-facing Phase 10 surfaces ship: a `GitHub` section on `/settings` with an on/off integration Switch (default ON, immediate-commit via the settings KV), and a `Project settings` dialog from the project ⋯ menu that edits a description (280-cap) and an origin-prefilled, soft-validated `owner/name` repo link in one PATCH — with the global OFF toggle hiding the repo field app-wide.**

## Performance

- **Duration:** ~8 min
- **Started:** 2026-06-13T14:31:00Z
- **Completed:** 2026-06-13T14:39:05Z
- **Tasks:** 3 (all `type="auto"`)
- **Files:** 7 (2 created, 5 modified)

## Accomplishments

- **Switch primitive (Task 1):** `web/src/components/ui/switch.tsx`, authored from the `radix-ui` umbrella (`Switch as SwitchPrimitive`) to match the existing `select.tsx`/`label.tsx` import style; accent-blue `data-[state=checked]:bg-primary` track per UI-SPEC. No new dependency.
- **API layer grown (Task 1):** `Project` type gains `description: string` + `github_repo: string | null` (matches Plan 02 server JSON); `useUpdateProjectSettings` PATCHes `{ description, github_repo }` to `/api/projects/{id}` and invalidates `["projects"]`; `useProjectGithubOrigin(id, enabled)` hits the origin-prefill endpoint, `enabled`-gated by dialog-open state with `staleTime: Infinity`.
- **GitHub section on /settings (Task 2):** a new last `<section>` after Cleanup, always visible (it is the control that hides everything else). On/off Switch reads `settings.github_integration?.value === "on"` (default ON), commits immediately via `useSaveSetting("github_integration")`, shows the canonical `Couldn't save. Try again.` on failure and never sticks mid-flip (cache untouched on error). Verbatim UI-SPEC copy throughout.
- **ProjectSettingsDialog (Task 3):** modeled on `RenameProjectDialog`. Description = 3-row `Textarea`, hard-stopped at 280 chars with a `{n}/280` counter that appears only past 240. Repo = mono `Input` rendered ONLY when `integrationOn` (OFF cascade, D-06), with both help variants (origin-detected vs not). One PATCH carries both fields; when integration is off the dialog re-sends the existing `github_repo` so the hidden link is never clobbered. Success closes the dialog; a `<500` ApiError sets a sentence-cased destructive error under the repo field and KEEPS THE DIALOG OPEN; 5xx/network shows the fixed form-level error. Soft-verify / gh-degraded muted advisories are wired for a future server verify flag.
- **⋯ menu item (Task 3):** `Project settings` `DropdownMenuItem` between `Rename` and the destructive `Delete project`, always shown (Description is integration-agnostic — D-06); mounts the dialog alongside `RenameProjectDialog`.
- **Embedded SPA rebuilt:** `web/dist/index.html` placeholder regenerated so `go:embed dist` serves the new frontend (Phase 9 `build(...)` precedent); `go build ./...` green.

## Task Commits

1. **Task 1:** `ea6abc3` (feat) — Switch primitive, grown Project type, project-settings mutation + origin query
2. **Task 2:** `78d9733` (feat) — GitHub section + integration Switch on SettingsPage
3. **Task 3:** `9c35b94` (feat) — ProjectSettingsDialog + ⋯ menu item with the OFF cascade
4. **SPA rebuild:** `277601b` (build) — embedded dist placeholder rebuilt

**Plan metadata:** committed separately (docs: complete plan).

## Files Created/Modified

- `web/src/components/ui/switch.tsx` — Created. shadcn Switch via the radix-ui umbrella; accent-blue checked track, zinc-800 off track, focus ring.
- `web/src/components/sidebar/ProjectSettingsDialog.tsx` — Created. Description + gated repo field, one PATCH, OFF cascade, hard-error-stays-open, wired soft advisories, origin prefill via adjust-state-during-render.
- `web/src/api/types.ts` — Modified. `Project` grown by `description` + `github_repo`.
- `web/src/api/mutations.ts` — Modified. Added `useUpdateProjectSettings`.
- `web/src/api/queries.ts` — Modified. Added `useProjectGithubOrigin`.
- `web/src/pages/SettingsPage.tsx` — Modified. New `GithubSection` (always-last) + Switch/Label/useSaveSetting imports.
- `web/src/components/sidebar/ProjectMenu.tsx` — Modified. `Project settings` item + dialog mount + `settingsOpen` state.
- `web/dist/index.html` — Modified. Rebuilt embed placeholder.

## Decisions Made

- **Hand-authored the Switch from the `radix-ui` umbrella** rather than running the shadcn CLI: every existing `ui/*` primitive imports from `"radix-ui"` (e.g. `import { Select as SelectPrimitive } from "radix-ui"`), and `radix-ui@^1.5.0` already re-exports `Switch`. The CLI would have written `@radix-ui/react-switch` imports, diverging from the established convention and adding a redundant dependency.
- **Reset/prefill via adjust-state-during-render** (tracking `prevOpen`/`prevSuggestion`) instead of the `RenameProjectDialog` `useEffect` prefill. The repo's `react-hooks` flat-recommended config flags `setState` inside an effect; the render-time adjustment is the canonical React pattern, keeps the on-open + late-origin behavior identical, and avoids adding a new lint error in my own file.
- **Soft advisories are present-but-dormant.** The muted soft-verify (`Saved, but couldn't verify {owner/name}...`) and gh-degraded (`Saved without verifying — the gh CLI isn't available.`) copy and their muted render path are wired keyed on a future `updated.verify_state` flag. Plan 02's server returns a plain 200 with no flag, so the success path closes the dialog today (UI-SPEC explicitly sanctions "closes immediately, the note is non-blocking"). This is intentional forward-wiring, not a stub blocking the plan's goal.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Avoided introducing new `react-hooks/set-state-in-effect` lint errors in ProjectSettingsDialog**
- **Found during:** Task 3 (lint after first dialog draft)
- **Issue:** Mirroring `RenameProjectDialog`'s `useEffect(() => { if (open) {...} }, [...])` prefill (as the plan instructs) reproduced its pre-existing `react-hooks/set-state-in-effect` lint error — but in a NEW file, so it counts as introduced by this task.
- **Fix:** Refactored the open-reset and late-origin prefill to React's adjust-state-during-render pattern (change-tracker state). Behavior is identical; the rule no longer fires; the file has zero lint errors.
- **Files modified:** web/src/components/sidebar/ProjectSettingsDialog.tsx
- **Commit:** 9c35b94

## Out-of-Scope Discoveries (Deferred, NOT fixed)

`cd web && npm run lint` reports **18 pre-existing errors** across files this plan does not touch (Board.tsx, SettingsField.tsx, RenameProjectDialog.tsx, CleanupWorktreeDialog.tsx, DiffTab.tsx, TerminalPane.tsx, TaskPage.tsx, use-mobile.ts, and shadcn-generated ui/* files): 8× `react-hooks/set-state-in-effect`, 4× `react-hooks/refs`, 4× `react-refresh/only-export-components`, 2× `react-hooks/immutability`. **Zero are in any 10-03 file** (verified). The gating build (`tsc -b && vite build`) is green; the Go build (embed) is green. These predate this phase and look like a stricter `react-hooks` ruleset than the code was written against. Logged to `.planning/phases/10-github-foundations/deferred-items.md`; recommend a dedicated lint-cleanup pass.

> Note on the plan's lint verify: each task's `<verify>` used `npm run lint | tail -10`, written assuming lint passes. It does not pass on this codebase (and never has on these files). Per the SCOPE BOUNDARY rule, the pre-existing failures are out of scope; the contract-relevant gate (`npm run build`) exits 0 and all 10-03 files are lint-clean.

## Issues Encountered

None blocking. The only friction was the pre-existing lint state (above), resolved by keeping my own files lint-clean and deferring the rest.

## User Setup Required

None. The toggle is pure local state through the existing settings KV; the repo link is soft-validated server-side (`gh` is a soft dependency from Plan 02). No external service configuration.

## Next Phase Readiness

- Phase 11 (PR Review column) can gate its entire surface on the same `useSettings()` `github_integration === "on"` value used here — the OFF cascade pattern is established.
- The `ProjectSettingsDialog` soft-advisory render path is pre-wired for a future server `verify_state` flag (D-11), so a Plan-02-successor that returns a verify result needs only to set the flag — no UI change.
- `Project.github_repo` is now available client-side for PR features that key off the linked repo.

## Verification

- `cd web && npm run build` (tsc -b && vite build) exits 0 across all three tasks.
- `cd web && npm run lint` shows zero errors in any 10-03 file (18 pre-existing out-of-scope errors deferred).
- `go build ./...` exits 0 — the rebuilt embed is valid.
- OFF cascade: the repo field render is gated on `integrationOn` (`github_integration === "on"`); the menu item and Description field are not gated.
- Hard-error path: the submit catch sets `error` via `sentenceCase` for `<500` and never calls `onOpenChange(false)` — the dialog stays open with the red message.
- Origin prefill query is `enabled` only by `open && integrationOn`, so git is never shelled on the project list.

## Self-Check: PASSED

All created/modified files verified present on disk; all four commits (`ea6abc3`, `78d9733`, `9c35b94`, `277601b`) verified in git history.

---
phase: 15-repo-first-creation-flow
plan: 02
subsystem: ui
tags: [react, typescript, shadcn, tabs, projects, github, create-flow]

# Dependency graph
requires:
  - phase: 15-repo-first-creation-flow
    provides: "Plan 01's widened useCreateProject body { name?; repo_path?; repo? } and Project.managed; backend createByRepo that auto-captures the GitHub description"
  - phase: 14-managed-checkout-foundations
    provides: "createByRepo atomic clone+INSERT (no orphan row / no partial dir on failure) — the primitive this dialog drives"
provides:
  - "Repo-first Add-project dialog: integration-gated 'GitHub repo' | 'Local folder' segmented toggle (repo default) reusing ui/tabs.tsx"
  - "Repo owner/name mode with editable Name prefilled from the name segment (nameEdited don't-clobber guard, pure local parse, no network)"
  - "Blocking 'Cloning <owner/name>…' Loader2 spinner while the synchronous create runs; dialog stays open, inputs + toggle disabled"
  - "Inline destructive-alert failure box (ProjectSettingsDialog classes) — dialog stays open, field values + active mode preserved, no half-created-project copy"
  - "Folder mode byte-for-byte behaviorally unchanged ({ repo_path, name? }); folder-only with no repo-first UI when integration is off"
  - "Rebuilt web/dist so the single Go binary embeds the new dialog"
affects: [project-settings-description-edit, future-clone-progress-streaming]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Controlled shadcn Tabs (ui/tabs.tsx) used purely as a segmented control driving local mode state — input blocks rendered outside TabsContent"
    - "Name prefill via the prevOpen/prevOwnerName adjust-state-on-change idiom (no setState-in-effect), mirroring ProjectSettingsDialog's repoEdited/prevSuggestion guard"
    - "Lucide Loader2 + 'size-4 animate-spin motion-reduce:animate-none' in-house spinner idiom (no spinner library) for a blocking in-flight affordance"

key-files:
  created: []
  modified:
    - web/src/components/sidebar/AddProjectDialog.tsx
    - web/dist/index.html

key-decisions:
  - "Reused the existing ui/tabs.tsx as the segmented toggle (the UI-SPEC Claude's-discretion item) — controlled Tabs/TabsList/TabsTrigger, no new dependency, no new ui/ file"
  - "Name prefill is a pure local owner/name string parse (split on the last '/'), never a gh/network call (D-09); nameEdited stops auto-derivation once the user types"
  - "The destructive-alert box (ProjectSettingsDialog classes) replaces the old plain text-xs error line in BOTH modes; no red input border (the box is the signal)"
  - "Used Loader2 (the UI-SPEC's named glyph) over RefreshCw — both are contract-compliant; Loader2 matches the spec copy table verbatim"
  - "web/dist hashed assets stay gitignored (web/dist/* + !index.html); only the regenerated index.html is committed, matching the Phase 13-03 'rebuild embedded SPA' precedent"

patterns-established:
  - "Segmented mode toggle in a creation dialog: controlled Tabs driving which input block mounts, gated on an integration flag"
  - "Blocking synchronous-clone affordance: disable inputs + toggle, muted spinner + 'Cloning <ref>…', dialog kept open until the create returns"

requirements-completed: [RPROJ-01, RPROJ-02, RPROJ-03, RPROJ-04, CKOUT-04]

# Metrics
duration: 3 min
completed: 2026-06-15
---

# Phase 15 Plan 02: Repo-First Add-Project Dialog Summary

**The Add-project dialog is now repo-first: when GitHub integration is on it shows a "GitHub repo" | "Local folder" segmented toggle (repo default) reusing `ui/tabs.tsx`, with an `owner/name` input that prefills the editable Name, a blocking "Cloning <owner/name>…" Loader2 spinner on submit, and inline destructive-alert failures that keep the dialog open with values preserved — folder mode stays byte-for-byte unchanged, and the whole repo-first UI vanishes when integration is off.**

## Performance

- **Duration:** 3 min
- **Started:** 2026-06-15T05:10:14Z
- **Completed:** 2026-06-15T05:13:58Z
- **Tasks:** 2
- **Files modified:** 2

## Accomplishments

- Evolved `AddProjectDialog.tsx` from folder-only into the repo-first creation flow, honoring every locked decision D-01..D-09 and copying the 15-UI-SPEC strings verbatim.
- Integration-gated segmented toggle (`useSettings().github_integration?.value === "on"`): repo-first default when on; byte-for-byte folder-only (no toggle, no repo field, repo branch never mounts) when off (RPROJ-04, D-02).
- Repo `owner/name` mode prefills the editable Name from the name segment via a pure local parse, never clobbering a user edit (`nameEdited` guard), using the `prevOpen`/`prevOwnerName` adjust-state-on-change idiom — zero `useEffect`, lint-clean (RPROJ-02, D-04).
- Repo submit POSTs `{ repo }` and shows the blocking `Cloning <owner/name>…` Loader2 spinner with inputs + toggle disabled and the dialog kept open (D-06); folder submit POSTs `{ repo_path }` with the unchanged shape and handling (D-03).
- Clone/validate failures surface in the mirrored `ProjectSettingsDialog` destructive-alert box — dialog stays open, field values + active mode preserved, atomicity-truthful copy with no half-created-project wording (CKOUT-04, D-07/D-09).
- Rebuilt `web/dist` so the single Go binary embeds the new dialog; the bundle contains the "Cloning" copy and `go build ./...` is green.

## Task Commits

Each task was committed atomically:

1. **Task 1: Rewrite AddProjectDialog (toggle, repo mode, name prefill, clone spinner, inline error)** — `1d152e9` (feat)
2. **Task 2: Rebuild the embedded SPA so the Go binary ships the dialog** — `36fef23` (chore)

**Plan metadata:** _(docs commit below)_

## Files Created/Modified

- `web/src/components/sidebar/AddProjectDialog.tsx` — Rewritten: integration-gated `Tabs` segmented toggle (repo default), repo `owner/name` mode with editable Name prefill (`nameEdited` guard, `prevOpen`/`prevOwnerName` reset, no setState-in-effect), blocking `Cloning <owner/name>…` `Loader2` spinner, inline destructive-alert failure (dialog open, values + mode preserved), success closes + `navigate(/projects/{id})`. Kept `sentenceCase`, the `sm:max-w-[480px]` width, the `flex flex-col gap-4` form / `gap-1.5` field blocks / `text-xs font-medium` labels, and the folder-mode `{ repo_path, name? }` body + `ApiError`→`sentenceCase`/409-period/fallback handling verbatim.
- `web/dist/index.html` — Regenerated by `vite build`; now references the new hashed bundle (`index-C43eIU7Y.js`) that embeds the "Cloning" copy. Hashed assets remain gitignored (`web/dist/*` + `!web/dist/index.html`), matching the Phase 13-03 dist-rebuild precedent.

## Decisions Made

- **Segmented toggle = existing `ui/tabs.tsx`** (the UI-SPEC Claude's-discretion item): controlled `Tabs`/`TabsList`/`TabsTrigger`, `flex-1` triggers, `aria-label="Project source"`, input blocks rendered outside `TabsContent`. No new dependency, no new `ui/` file.
- **Name prefill is a pure local parse** (split on the last `/`), never a `gh`/network call (D-09); auto-derivation stops once `nameEdited` is true.
- **Destructive-alert box replaces the old plain error line in both modes** (verbatim `ProjectSettingsDialog` classes); no red input border — the box is the signal.
- **`Loader2` over `RefreshCw`** for the spinner — both are contract-compliant per the UI-SPEC; `Loader2` is the spec's named glyph and matches the copy table.
- **Only `web/dist/index.html` is committed** for the dist rebuild — the hashed JS/CSS are gitignored by design and regenerated at build time (Phase 13-03 precedent).

## Deviations from Plan

None - plan executed exactly as written.

**Total deviations:** 0
**Impact on plan:** None — both tasks followed their `<action>` and `<acceptance_criteria>` verbatim. The only cosmetic adjustment (inlining the toggle-label text within the `TabsTrigger` tags so the literal `>GitHub repo<` / `>Local folder<` acceptance greps match) is a formatting choice with no behavior change, still tsc + eslint clean.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- The user-facing repo-first creation surface is complete and drives Phase 14's atomic `createByRepo` from the UI; RPROJ-01/02/03/04 + CKOUT-04 are satisfied end-to-end.
- `web/dist` is rebuilt so a fresh `go build` / `make build` ships the dialog.
- Ready for 15-03 (the next plan in this phase). No blockers.

## Self-Check: PASSED

- `web/src/components/sidebar/AddProjectDialog.tsx` — FOUND on disk
- `web/dist/index.html` — FOUND on disk (references the new `Cloning`-containing bundle)
- Commit `1d152e9` (feat, Task 1) — FOUND in git log
- Commit `36fef23` (chore, Task 2) — FOUND in git log
- tsc `--noEmit` exit 0, eslint exit 0 (no new lint advisories in the file), `go build ./...` exit 0, `go vet ./...` exit 0

---
*Phase: 15-repo-first-creation-flow*
*Completed: 2026-06-15*

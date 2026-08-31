---
phase: 16-global-view-settings-bar-integration
plan: 03
subsystem: ui
tags: [react, tanstack-query, typescript, global-scratchpad, settings, dialog]

# Dependency graph
requires:
  - phase: 14-global-config-api
    provides: GET/PUT /api/global wire (partial pointer grammar, 409 gate with reasons, managed-clone dispatch)
  - phase: 16-global-view-settings-bar-integration/01
    provides: the ["global"] query + useSaveGlobal PUT-to-cache contract, ApiError.reasons mechanics
  - phase: 16-global-view-settings-bar-integration/02
    provides: the /global route (the Open Scratchpad CTA destination)
provides:
  - web/src/components/settings/ScratchpadSection.tsx — the Settings Scratchpad section (summary card + agent selector + Open Scratchpad CTA + Change-root dialog)
  - GCONF-05 idle-reachability affordance (Open Scratchpad → /global), closing the D-39 Settings↔view loop in both directions
  - D-45 verbatim 409-reasons surfacing in the UI (sentenceCase lead + mono-span reasons list via 16-01 ApiError.reasons)
affects: [phase-17-uat]

# Tech tracking
tech-stack:
  added: []  # zero new packages, zero shadcn adds
  patterns:
    - "Composed card div (rounded-lg border bg-card p-4) instead of a card primitive — the zero-add constraint made structural"
    - "Shared error-box element rendered below the active input for BOTH submit and clear-root failures — one verbatim-server-message posture per dialog"

key-files:
  created:
    - web/src/components/settings/ScratchpadSection.tsx
  modified:
    - web/src/pages/SettingsPage.tsx
    - web/src/dist placeholder: web/dist/index.html (tracked artifact refresh)

key-decisions:
  - "Clear root placed in the DialogFooter with sm:mr-auto (destructive variant, left of Cancel/Save) — UI-SPEC left placement discretion resolved to the footer-left slot"
  - "The error box (message + 409 reasons ul) is one element rendered below the active input for every failure source (Save root AND Clear root) — AddProjectDialog's exactly-one-of help/error idiom kept, with clear-root failures surfacing through the same slot since the dialog stays open"
  - "aria-expanded={rootDialogOpen} on the Change-root trigger — the legitimate dialog-trigger ARIA pattern that also keeps Task 1's standalone build green before the dialog exists (see Deviations)"
  - "GCONF-05 left for end-of-phase/Phase 17 UAT closure per the plan's verification section — the implementation ships here"

patterns-established:
  - "Server-truth card rows: github_repo non-null IS the managed marker (Badge secondary), root_path mono/truncate/title, empty root = muted line — no booleans, no mirrors"

requirements-completed: [GCONF-05]

# Coverage metadata (#1602)
coverage:
  - id: D1
    description: "ScratchpadSection summary card mounted in the Settings 640px column after AgentsSection: root row (folder mono/truncate/title, managed owner/name + secondary Badge, unconfigured No root configured yet.), default-agent Select (cached agent_id controlled, instant PUT {agent_id}, Couldn't save inline error, Applies at the next Start. hint), Open Scratchpad primary CTA → /global + Change root…/Configure root… outline trigger"
    requirement: GCONF-05
    verification:
      - kind: other
        ref: "npm --prefix web run build + grep gates (Open Scratchpad, Applies at the next Start., No root configured yet., ScratchpadSection ×2 in SettingsPage) + npx eslint ScratchpadSection.tsx clean"
        status: pass
    human_judgment: false
  - id: D2
    description: "Change-root dialog (AddProjectDialog-derived, D-43): Scratchpad root title, GitHub repo | Local folder Tabs gated on github_integration (repo default), mono inputs with exactly-one-of help/error, prevOpen adjust-during-render reset (zero useEffect), blocking Cloning {owner/name}… spinner, Save root submit, only-2xx-closes failure posture"
    requirement: GCONF-05
    verification:
      - kind: other
        ref: "grep gates: prevOpen ×2, Cloning ×2, sentenceCase ×3, useEffect=0, dangerouslySetInnerHTML=0, href#=0; build + eslint clean"
        status: pass
    human_judgment: false
  - id: D3
    description: "Verbatim 409/400 surfacing (D-45): ApiError → sentenceCase(err.message) lead + reasons[].target mono list-disc items, NO client re-wording; non-ApiError → Couldn't save. Try again.; Clear root (D-44) destructive button rendered only when configured, PUT {root_path: \"\"} behind the same gate, no nested confirm"
    requirement: GCONF-05
    verification:
      - kind: other
        ref: "grep gates: reasons ×7, Clear root ×1, saveError maps ApiError message/reasons verbatim; git diff web/package.json web/components.json empty"
        status: pass
    human_judgment: true
    rationale: "Observable dialog behavior (blocked change with a live global session surfacing the 409 lead + reasons list, clone spinner, clear-to-unconfigured) requires a running app with real config/session state — the plan's verification section defers this to end-of-phase manual UAT"

# Metrics
duration: 9 min
completed: 2026-08-27
status: complete
---

# Phase 16 Plan 03: Settings Scratchpad section Summary

**The Settings Scratchpad section: a server-truth summary card (root row with managed badge, instant-save default-agent Select, Open Scratchpad CTA to /global) plus the AddProjectDialog-derived Change-root dialog with segmented capture, blocking clone spinner, verbatim 409-reasons surfacing, and the gated no-confirm Clear root action**

## Performance

- **Duration:** 9 min
- **Started:** 2026-08-27T11:20:44Z
- **Completed:** 2026-08-27T11:29:35Z
- **Tasks:** 2
- **Files modified:** 3 (2 src + tracked dist artifact refresh)

## Accomplishments
- ScratchpadSection.tsx (NEW): section wrapper in the standing uppercase-muted h2 idiom + composed rounded-lg border bg-card summary card (deliberately NOT a shadcn add) with three server-truth rows off the shared ["global"] query
- Default-agent Select copies the ProjectSettingsDialog idiom verbatim but is CONTROLLED by the cached agent_id — instant PUT {agent_id} on change, inline Couldn't save. Try again. on failure, and the untouched cache snaps the control back automatically (Pitfall 10 posture)
- Open Scratchpad primary CTA navigates to /global — the GCONF-05 idle-reachability affordance closing the D-39 Settings↔view loop both directions (16-02's /global degraded-state CTAs land back on /settings)
- Change-root dialog (D-43): prevOpen adjust-during-render reset (zero setState-in-effect debt), integration-gated segmented Tabs (repo default), mono inputs with exactly-one-of help/error, blocking Cloning {owner/name}… spinner, and the every-failure-keeps-the-dialog-open posture (only a 2xx closes)
- 409 gate surfacing (D-45): sentenceCase(err.message) lead sentence + reasons[].target as mono list-disc items — wire strings verbatim through 16-01's ApiError.reasons, zero client re-wording; Clear root (D-44) is an explicit destructive button (only when configured) sending the one clear spelling PUT {root_path: ""} with NO nested confirmation dialog

## Task Commits

Each task was committed atomically:

1. **Task 1: ScratchpadSection summary card — root row, agent selector, Open Scratchpad CTA + SettingsPage mount** - `f253c8f` (feat)
2. **Task 2: Change-root dialog — segmented capture, clone feedback, verbatim 409 reasons, gated Clear root** - `7954503` (feat)

**Artifact refresh:** `e95b4b2` (chore: rebuild embedded SPA dist — repo convention, tracked-but-ignored index.html via `git add -u`)

## Files Created/Modified
- `web/src/components/settings/ScratchpadSection.tsx` - NEW: summary card + agent selector + CTA + Change-root dialog (module-local) + sentenceCase helper
- `web/src/pages/SettingsPage.tsx` - import + mount directly after AgentsSection (2-line diff + comment)
- `web/dist/index.html` - rebuilt hash (tracked artifact)

## Decisions Made
- Clear root sits in the DialogFooter with `sm:mr-auto` (destructive variant, left of Cancel/Save) — resolves the UI-SPEC's planner-discretion placement to the footer-left slot
- The error box is ONE element (message + optional reasons ul) rendered below the active input for both Save-root and Clear-root failures — keeps AddProjectDialog's exactly-one-of help/error idiom while guaranteeing clear-root 409s surface (the dialog stays open on failure, so the field block always renders)
- Dist artifact refreshed as a separate `chore:` commit per repo precedent (16-01/16-02)
- GCONF-05 checked in REQUIREMENTS.md per this plan's requirements frontmatter; its observable UAT rides the end-of-phase manual pass (16-02 decision posture carried forward — the 409-with-live-session flow needs a running app)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] aria-expanded on the Change-root trigger button (Task 1)**
- **Found during:** Task 1 (ScratchpadSection creation)
- **Issue:** The plan splits the open state into Task 1 but the dialog into Task 2 — with `noUnusedLocals`, `rootDialogOpen` is never read in Task 1's standalone build, failing tsc (`TS6133`) and Task 1's own build gate
- **Fix:** Added `aria-expanded={rootDialogOpen}` to the Change root…/Configure root… button — the standard dialog-trigger ARIA pattern, meaningful before and after the dialog exists
- **Files modified:** web/src/components/settings/ScratchpadSection.tsx
- **Verification:** Task 1 build green + eslint clean at `f253c8f`; attribute remains a genuine a11y improvement in the final file
- **Committed in:** f253c8f (Task 1 commit)

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** Minimal — a one-attribute a11y addition required by the plan's own two-task file split. No scope creep.

## Issues Encountered

None

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- Phase 16 implementation is complete (all 3 plans landed): wire layer + /global view + Settings section — the end-of-phase manual UAT from the plans' verification sections now drives GCONF-05/GVIEW-01/GVIEW-04/GINT-01 closure
- Zero new packages, zero shadcn adds; commit range touches exactly ScratchpadSection.tsx + SettingsPage.tsx + dist/index.html — no TaskPage/GlobalTaskPage/AgentTab overlap (parallel-safety verified post-hoc)
- Phase 17 (hardening & E2E) can consume the section as-is; the stale types.ts Agent.engine union remains deferred (16-01 Pitfall 2 note)

## Self-Check: PASSED

Both key src files exist on disk (ScratchpadSection.tsx NEW, 375 lines); all 3 task/artifact commits (f253c8f, 7954503, e95b4b2) found in git log; plan-level build green, eslint clean in isolation, all grep gates re-run green (CTA copy, tracker, reasons, Clear root, zero dangerouslySetInnerHTML, zero href-#, zero package/registry drift).

---
*Phase: 16-global-view-settings-bar-integration*
*Completed: 2026-08-27*

---
phase: 02-agent-management-ui-and-per-project-sele
plan: "02"
subsystem: ui
tags: [react, shadcn-select, project-settings, agent-id, conditional-patch]

requires:
  - phase: 02-agent-management-ui-and-per-project-sele
    provides: agents API client (02-01)
  - phase: 01-agent-data-foundation-and-spawn-engine
    provides: projects.agent_id threading
provides:
  - Agent selector (shadcn Select) in Project Settings
  - useUpdateProjectSettings carries agent_id (conditional PATCH)
affects: [per-project-agent-selection, spawn-engine]

tech-stack:
  added: [shadcn-select]
  patterns: [conditional-patch-on-change, select-from-query-data]

key-files:
  created: []
  modified:
    - web/src/components/sidebar/ProjectSettingsDialog.tsx
    - web/src/api/mutations.ts

key-decisions:
  - "Conditional PATCH: only send agent_id when it changes, avoiding needless writes"
  - "useUpdateProjectSettings extended to carry agent_id alongside existing fields"

patterns-established:
  - "Project-level agent binding surfaced through a Select populated by useAgents"

requirements-completed: []

coverage:
  - id: D1
    description: Agent selector in Project Settings with conditional PATCH on change
    requirement: ""
    verification:
      - kind: unit
        ref: "tsc -b + vite build green"
        status: pass
    human_judgment: false

duration: ~2min
completed: 2026-07-06
status: complete
---

# Plan 02-02: Project Settings agent selector Summary

**A shadcn Select in Project Settings lets the user pick the project's agent; changing it conditionally PATCHes agent_id via the extended useUpdateProjectSettings hook.**

## Performance

- **Completed:** 2026-07-06
- **Tasks:** 1
- **Files modified:** 2

## Accomplishments
- Agent selector (shadcn Select) wired into ProjectSettingsDialog, populated by useAgents
- Conditional PATCH on change — only sends agent_id when it actually differs
- useUpdateProjectSettings carries agent_id

## Task Commits

1. **T02: Project Settings agent selector** — `665e924` (feat)

## Files Created/Modified
- `web/src/components/sidebar/ProjectSettingsDialog.tsx` — agent Select + on-change PATCH
- `web/src/api/mutations.ts` — useUpdateProjectSettings extended with agent_id

## Decisions Made
- Conditional PATCH avoids unnecessary writes when the agent is unchanged

## Deviations from Plan

None — work was already implemented and committed; this summary documents the delivered state.

## Issues Encountered
None

## Next Phase Readiness
- Per-project agent binding complete; the spawn engine (01-05) reads the bound agent's engine

---
*Phase: 02-agent-management-ui-and-per-project-sele*
*Completed: 2026-07-06*

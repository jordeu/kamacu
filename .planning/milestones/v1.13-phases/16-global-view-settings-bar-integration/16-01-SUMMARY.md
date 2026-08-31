---
phase: 16-global-view-settings-bar-integration
plan: 01
subsystem: ui
tags: [react, tanstack-query, typescript, wire-types, global-scratchpad]

# Dependency graph
requires:
  - phase: 14-global-config-api
    provides: GET/PUT /api/global wire (globalConfig, 409 gate with reasons)
  - phase: 15-global-sessions-backend
    provides: scope:"global" spawn/list/reattach branches, source:"global" status entries
provides:
  - GlobalConfig interface + useGlobal()/useSaveGlobal() — the shared ["global"] query (D-17/D-23)
  - Six scope-aware session hooks keyed ["sessions","global"]
  - AgentStatusEntry.source union widened with "global"
  - ApiError.reasons?: {kind, target}[] (409 blocker surfacing mechanics)
  - TermSession.global?: boolean
  - ActiveSessionsBar GINT-01 edits (sessionId re-key, /global navigation, pathname highlight)
affects: [16-02-global-view, 16-03-settings-section, phase-17-uat]

# Tech tracking
tech-stack:
  added: []  # zero new packages (carry-forward locked)
  patterns:
    - "Scope-aware hook spelling: key [\"sessions\",\"global\"] under the [\"sessions\"] prefix so scope-blind prefix invalidation keeps working"
    - "PUT-response-as-GET-shape cache write: setQueryData([\"global\"], response), no refetch (D-23)"

key-files:
  created:
    - web/src/api/global.ts
  modified:
    - web/src/api/client.ts
    - web/src/api/agents.ts
    - web/src/api/sessions.ts
    - web/src/components/layout/ActiveSessionsBar.tsx
    - web/dist/index.html

key-decisions:
  - "ApiErrorReason exported as a named interface (not inline) so 16-03's Change-root dialog can import the {kind, target} shape it renders"
  - "agent.engine typed as plain string in GlobalConfig (Go wire emits claude|custom|opencode; the types.ts Agent union is stale — Pitfall 2, left untouched)"
  - "GINT-01/GVIEW-01 NOT marked complete: this plan ships the wire/data halves by its own success criteria; the requirements close when 16-02 lands + Phase 17 UAT"

patterns-established:
  - "Scope-aware hook set: task-coupled hooks stay untouched; global variants copy the setQueryData-then-prefix-invalidate onSuccess contract verbatim"

# GINT-01/GVIEW-01 intentionally NOT listed — see key-decisions; halves ship here,
# requirements close with 16-02 + Phase 17 UAT.
requirements-completed: []

# Coverage metadata (#1602)
coverage:
  - id: D1
    description: "TS wire-type widenings: AgentStatusEntry.source admits \"global\", TermSession.global?: boolean, ApiError carries optional structured reasons ({kind, target}[]) populated from gated 409 bodies"
    requirement: GINT-01
    verification:
      - kind: other
        ref: "npm --prefix web run build (tsc -b) + grep gates: '\"manual\" | \"github_pr\" | \"global\"' in agents.ts, 9 'reasons' hits in client.ts, 'global?: boolean' in sessions.ts"
        status: pass
    human_judgment: false
  - id: D2
    description: "web/src/api/global.ts — GlobalConfig interface (engine as plain string), useGlobal() on queryKey [\"global\"] with 5s refetch, useSaveGlobal() partial-body PUT writing the response into the [\"global\"] cache"
    requirement: GVIEW-01
    verification:
      - kind: other
        ref: "npm --prefix web run build + grep gates: queryKey: [\"global\"] ×1, setQueryData([\"global\"]) present, refetchInterval: 5000"
        status: pass
    human_judgment: false
  - id: D3
    description: "Six scope-aware session hooks in sessions.ts (useGlobalSessions/useSpawnGlobalSession/useSpawnGlobalAgent/useResumeGlobalAgent/useReattachGlobalTmux/useRenameGlobalSession) keyed [\"sessions\",\"global\"] with exact POST bodies and the setQueryData-then-invalidate contract"
    requirement: GVIEW-01
    verification:
      - kind: other
        ref: "npm --prefix web run build + grep gates: [\"sessions\", \"global\"] ×8 (6 functional + 2 comments), api/sessions?scope=global ×1, agent variants invalidate [\"agent-statuses\"]"
        status: pass
    human_judgment: false
  - id: D4
    description: "ActiveSessionsBar GINT-01 edits: rows keyed entry.sessionId, source \"global\" rows navigate to /global and highlight on location.pathname === \"/global\" (task rows unchanged)"
    requirement: GINT-01
    verification:
      - kind: other
        ref: "npm --prefix web run build + grep gates: key={entry.sessionId} ×1, source === \"global\" ×2, useLocation imported+used; repo-wide source === audit shows no new sites outside the bar"
        status: pass
    human_judgment: true
    rationale: "Observable behavior (a live Global · Scratchpad row rendering, click-through, highlight) needs a running app with a spawned global session — the plan defers the manual smoke to end-of-phase UAT, and the /global route itself lands in 16-02"

# Metrics
duration: 10 min
completed: 2026-08-27
status: complete
---

# Phase 16 Plan 01: Global wire layer & bar integration Summary

**TS wire widenings (source union, TermSession.global, ApiError.reasons), the new global config client (["global"] query + partial PUT), six scope-aware session hooks keyed ["sessions","global"], and the Active Sessions bar's three GINT-01 edits (sessionId re-key, /global navigation branch, pathname highlight)**

## Performance

- **Duration:** 10 min
- **Started:** 2026-08-27T07:36:04Z
- **Completed:** 2026-08-27T07:46:19Z
- **Tasks:** 3
- **Files modified:** 6 (5 src + tracked dist/index.html artifact refresh)

## Accomplishments
- ApiError additively carries the structured 409 blocker list (`reasons?: {kind, target}[]`) so 16-03 can render it verbatim — existing single-message callers untouched
- `web/src/api/global.ts` (NEW): the ONE shared ["global"] query (5s refetch, D-17 one round-trip) + `useSaveGlobal()` partial-PATCH PUT whose response IS the GET shape written straight into the cache (D-23)
- Six scope-aware session hooks with exact Phase-15 POST grammar (`{scope:"global"}` × kind/resume/reattach_tmux_name variants) and the spawn-select race-fix contract copied verbatim from the task hooks
- The bar renders global rows as standard `Global · Scratchpad` rows keyed by sessionId that navigate to `/global` and highlight there — replacing the Phase-15 interim dead-route artifact

## Task Commits

Each task was committed atomically:

1. **Task 1: Widen the TS wire types + surface 409 reasons on ApiError** - `5704fc4` (feat)
2. **Task 2: Create web/src/api/global.ts + the scope-aware session hooks** - `72766b8` (feat)
3. **Task 3: ActiveSessionsBar — the three surgical GINT-01 edits (P6)** - `25387ef` (feat)

**Artifact refresh:** `30961b3` (chore: rebuild embedded SPA dist — repo convention, tracked-but-ignored index.html via `git add -u`)

## Files Created/Modified
- `web/src/api/global.ts` - NEW: GlobalConfig interface, useGlobal(), useSaveGlobal()
- `web/src/api/client.ts` - ApiErrorReason interface + ApiError.reasons populated in the !res.ok path
- `web/src/api/agents.ts` - source union widened to `"manual" | "github_pr" | "global"`
- `web/src/api/sessions.ts` - TermSession.global? + the six global hooks
- `web/src/components/layout/ActiveSessionsBar.tsx` - key/nav/highlight branches (SessionRow body untouched)
- `web/dist/index.html` - rebuilt hash (tracked artifact)

## Decisions Made
- `ApiErrorReason` exported as a named interface (not inline on the field) so 16-03's dialog imports the exact `{kind, target}` shape it renders
- `agent.engine: string` (plain) in GlobalConfig — mirrors the Go wire; the stale `types.ts` `Agent.engine` union is deliberately NOT widened (Pitfall 2, deferred to Phase 17)
- GINT-01/GVIEW-01 left unchecked in REQUIREMENTS.md: the plan's own success criteria scope this plan to the wire/data halves; the requirements close when 16-02 lands and Phase 17 UAT passes
- Dist artifact refreshed as a separate `chore:` commit per repo precedent (b3c3f3a)

## Deviations from Plan

None - plan executed exactly as written.

Minor note (not a deviation): Task 2's JSDoc originally contained the literal `api/sessions?scope=global`, tripping the exactly-1 grep gate at 2 hits; reworded the comment to `?scope=global` before committing. No functional difference.

## Issues Encountered

None

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- The ["global"] query and the full ["sessions","global"] hook set are ready for 16-02 (GlobalTaskPage + /global route) and 16-03 (Settings Scratchpad section) — pure composition remains
- The bar's /global navigation targets a route that does not exist until 16-02 registers it in App.tsx (planned seam — the interim dead route is the known, accepted state between plans)
- Manual smoke (live global agent row in the bar) deferred to end-of-phase UAT per the plan's verification section
- Zero new packages; zero backend edits; zero TaskPage/TaskTabs/TerminalPane edits — scope hygiene verified against the commit range

---
*Phase: 16-global-view-settings-bar-integration*
*Completed: 2026-08-27*

## Self-Check: PASSED

All 5 key files exist on disk; all 4 task/artifact commits (5704fc4, 72766b8, 25387ef, 30961b3) found in git log; plan-level build + grep gates re-run green (see coverage refs).

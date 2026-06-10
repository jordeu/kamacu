---
phase: 02-terminal-engine
plan: 02
subsystem: ui
tags: [xterm, vite, tanstack-query, websocket, terminal]

# Dependency graph
requires:
  - phase: 01-foundation
    provides: "Vite + React SPA scaffold, typed fetch layer (web/src/api/client.ts), TanStack Query client"
provides:
  - "xterm 6 package set pinned exactly (@xterm/xterm 6.0.0, addon-fit 0.11.0, addon-webgl 0.19.0)"
  - "WS-capable Vite dev proxy (/api object form with ws: true)"
  - "web/src/components/terminal/xtermTheme.ts — zincTheme ITheme + terminalFontFamily/terminalFontSize per UI-SPEC"
  - "web/src/api/sessions.ts — TermSession type + useSessions/useSpawnSession/useStopSession/useDeleteSession"
affects: [02-04, 02-05, 03-task-view]

# Tech tracking
tech-stack:
  added: ["@xterm/xterm@6.0.0", "@xterm/addon-fit@0.11.0", "@xterm/addon-webgl@0.19.0"]
  patterns:
    - "Session REST hooks mirror existing mutations.ts shape: useQueryClient + invalidateQueries on [\"sessions\"]"
    - "Terminal visual constants live in a dedicated module (xtermTheme.ts) — literal values, no CSS var() (renderer cannot resolve them)"

key-files:
  created:
    - web/src/components/terminal/xtermTheme.ts
    - web/src/api/sessions.ts
  modified:
    - web/package.json
    - web/package-lock.json
    - web/vite.config.ts

key-decisions:
  - "xterm deps pinned exactly (no caret) — addon set must move together with xterm 6 per STACK.md"
  - "selectionForeground deliberately unset in zincTheme to preserve cell colors under selection (UI-SPEC)"
  - "refetchInterval 5000 on the sessions query keeps non-attached rows' status honest (research Pattern 4)"

patterns-established:
  - "Terminal theme/font constants exported from xtermTheme.ts — Phases 3-5 import verbatim, never redefine"
  - "Session ids are opaque strings at every boundary (TermSession.id never parsed)"

requirements-completed: [TERM-03]

# Metrics
duration: 3min
completed: 2026-06-10
---

# Phase 2 Plan 02: Terminal Frontend Foundation Summary

**xterm 6 set pinned exactly, Vite /api proxy made WebSocket-capable (ws: true), UI-SPEC zinc theme encoded as xtermTheme.ts, and typed TanStack Query session hooks (list/spawn/stop/delete) built against the fixed REST contract**

## Performance

- **Duration:** 3 min
- **Started:** 2026-06-10T10:17:36Z
- **Completed:** 2026-06-10T10:20:13Z
- **Tasks:** 2
- **Files modified:** 5

## Accomplishments

- Installed `@xterm/xterm@6.0.0`, `@xterm/addon-fit@0.11.0`, `@xterm/addon-webgl@0.19.0` with exact pins; confirmed no unscoped `xterm`, no `addon-canvas`, no `addon-attach`
- Closed Pitfall 4 before anyone hits it: the string-shorthand Vite proxy (which silently drops WebSocket upgrades) replaced with `{ target: "http://127.0.0.1:7333", ws: true }`
- `xtermTheme.ts` encodes the UI-SPEC zinc table + ANSI 16 palette byte-for-byte (all 18 hex values verified), `selectionForeground` left unset, literal mono font stack + 13px exported
- `sessions.ts` provides `TermSession` and four hooks covering the full session REST contract, mirroring the existing `mutations.ts` invalidation pattern with `["sessions"]` and a 5s refetch interval on the list query

## Task Commits

Each task was committed atomically:

1. **Task 1: Install xterm package set and fix the Vite WS proxy** - `c1bcd57` (chore)
2. **Task 2: xterm zinc theme module and typed session REST hooks** - `1935f46` (feat)

## Files Created/Modified

- `web/src/components/terminal/xtermTheme.ts` - zincTheme ITheme object + terminalFontFamily + terminalFontSize (UI-SPEC contract, carries to Phases 3-5 verbatim)
- `web/src/api/sessions.ts` - TermSession type + useSessions/useSpawnSession/useStopSession/useDeleteSession
- `web/package.json` / `web/package-lock.json` - xterm 6 set added with exact pins
- `web/vite.config.ts` - WS-capable dev proxy

## Decisions Made

- Pinned xterm deps exactly (stripped npm's default caret ranges) — the addon set must move together with xterm 6 and the plan's acceptance criteria require exact versions
- No zustand for session state — plain TanStack Query is sufficient this phase (per plan/research)

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

- `npm install pkg@x.y.z` saves caret ranges by default; edited package.json to exact pins and re-ran `npm install` to sync the lockfile (covered by Task 1's acceptance criteria, not a deviation)

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- 02-04 (TerminalPane) and 02-05 (TerminalPage) can import `zincTheme`, `terminalFontFamily`, `terminalFontSize`, and the four session hooks without exploration
- WS connections through the dev proxy will now upgrade correctly; the Go server still needs `--dev-origin` handling (02-01/02-03 scope) for dev-origin allowlisting
- Backend REST endpoints (`/api/sessions` et al.) land in plan 02-01 (parallel wave) — hooks are built against the research-fixed contract

---
*Phase: 02-terminal-engine*
*Completed: 2026-06-10*

## Self-Check: PASSED

- web/src/components/terminal/xtermTheme.ts: FOUND
- web/src/api/sessions.ts: FOUND
- Commit c1bcd57: FOUND
- Commit 1935f46: FOUND

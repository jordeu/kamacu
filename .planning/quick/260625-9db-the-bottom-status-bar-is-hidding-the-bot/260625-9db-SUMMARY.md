---
phase: quick-260625-9db
plan: 01
subsystem: web-frontend-layout
tags: [layout, sidebar, status-bar, css, tailwind, bugfix]
requires:
  - "AppLayout <main> flex container"
  - "ProjectSidebar <Sidebar collapsible=icon> primitive (forwards className to sidebar-container)"
  - "ActiveSessionsBar fixed bottom overlay (h-9 / 36px collapsed)"
provides:
  - "36px bottom-space reservation under both overlapped surfaces (pb-9)"
  - "Add-project button + Settings gear no longer covered by the status bar"
  - "Agent terminal / shell view no longer clipped at the bottom"
affects:
  - web/src/components/layout/AppLayout.tsx
  - web/src/components/sidebar/ProjectSidebar.tsx
tech-stack:
  added: []
  patterns:
    - "Reserve overlay height with matching bottom padding (pb-9 == bar h-9) rather than reflowing the bar"
    - "Forward className to shadcn Sidebar primitive to pad the inner fixed sidebar-container without editing the generated primitive"
key-files:
  created: []
  modified:
    - web/src/components/layout/AppLayout.tsx
    - web/src/components/sidebar/ProjectSidebar.tsx
    - web/dist/index.html
decisions:
  - "Keep ActiveSessionsBar as a fixed-bottom overlay (preserve D-03/D-13); reserve space underneath via pb-9 on the two overlapped surfaces instead of changing the bar."
  - "Pad <main> (one correct layer) so TerminalPane's existing ResizeObserver auto-re-fits xterm; do NOT add padding to terminal panes directly."
  - "Add padding via className forwarded to <Sidebar>; do NOT edit the generated shadcn sidebar.tsx primitive."
metrics:
  duration: "~3 min (active); human-verify checkpoint pending"
  completed: "2026-06-25"
  tasks_completed: 1
  tasks_total: 2
  files_modified: 3
---

# Quick Task 260625-9db: Status Bar Hiding Sidebar Footer & Main View Summary

Reserved 36px of clear bottom space (`pb-9`, matching the status bar's collapsed `h-9` height) on both `<main>` and the sidebar container so the fixed `ActiveSessionsBar` overlay no longer covers the sidebar's Add-project button or clips the bottom of the agent terminal / shell view — two Tailwind className edits, no changes to the shadcn primitive or the bar component.

## What Was Done

**Task 1 (auto) — COMPLETE, committed `77fe135`:**

- `web/src/components/layout/AppLayout.tsx`: `<main className="relative flex-1 overflow-hidden">` → added `pb-9`. Shrinks `<main>`'s usable content box by 36px at the bottom so the task view is no longer clipped by the bar. The `TerminalPane` `ResizeObserver` (already watching the viewport element) re-fits xterm automatically when `<main>` shrinks — no terminal code changes.
- `web/src/components/sidebar/ProjectSidebar.tsx`: `<Sidebar collapsible="icon">` → `<Sidebar collapsible="icon" className="pb-9">`. The `Sidebar` primitive forwards `className` to its inner `fixed inset-y-0 h-svh` sidebar-container, lifting `SidebarFooter` (Add-project + Settings) 36px above the viewport bottom, clearing the status bar.
- `web/dist/index.html`: regenerated bundle-hash references from `vite build` (tracked placeholder for `go:embed`; `web/dist/assets/*` are gitignored and rebuilt at build time — per repo `.gitignore` convention).

The `ActiveSessionsBar` and the generated `web/src/components/ui/sidebar.tsx` primitive were NOT touched. The bar keeps its fixed-bottom overlay + expand-upward behavior.

## Verification

- `cd web && npm run build` (tsc -b + vite build): **PASS** — 2218 modules transformed, built cleanly.
- `cd web && npm run lint`: 20 pre-existing errors remain (all in unrelated files: `Board.tsx`, `PRCard.tsx`, `ReviewColumn.tsx`, `SettingsField.tsx`, `StatusDot.tsx`, `TaskPage.tsx`, `useTerminalSocket.ts`, `use-mobile.ts`, `button.tsx`, `sidebar.tsx`, `tabs.tsx`, `TerminalPane.tsx`). These are the documented react-hooks baseline (STATE.md "Pending Todos" — deferred to a dedicated lint-cleanup pass) and are out of scope. Linting the two edited files in isolation (`npx eslint AppLayout.tsx ProjectSidebar.tsx`) returns **exit 0 — zero errors**.

## Deviations from Plan

None — plan executed exactly as written (the two specified one-line className edits). The `web/dist/index.html` rebuild is an expected build artifact of the source change, consistent with the repo's existing `chore: rebuild embedded SPA` / `fix(19)` precedent of committing the tracked placeholder alongside source.

## Pending Checkpoint (human-verify) — NOT YET DONE

Task 2 is a `checkpoint:human-verify` gate that requires a browser at localhost — it CANNOT be performed by the executor. The orchestrator / user must verify:

1. Expand sidebar → "Add project" button fully visible and clickable (opens Add Project dialog); Settings gear fully visible.
2. Open a task with a running agent session (or a bash shell tab) → bottom of terminal/shell view fully visible, nothing clipped behind the bar.
3. Terminal fills its container cleanly — no truncated last row, no leftover blank strip between terminal and bar (xterm fit recomputed on the 36px-shorter content area). If off, resize window once to force re-fit and report.
4. Click the status bar to expand → session list still floats UP over content (overlay unchanged), collapses back to the 36px bar.

**Resume signal:** Type "approved" or describe any remaining overlap/clipping/fit issues.

## Known Stubs

None.

## Self-Check: PASSED

- `web/src/components/layout/AppLayout.tsx` — FOUND, contains `pb-9` on `<main>`
- `web/src/components/sidebar/ProjectSidebar.tsx` — FOUND, `<Sidebar>` has `className="pb-9"`
- Commit `77fe135` — FOUND in git log

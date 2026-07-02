---
phase: 22-github-style-diff-review
plan: 05
subsystem: verification
tags: [verification, human-verify, uat, diff, github-style, sticky-headers]

# Dependency graph
requires:
  - phase: 22-github-style-diff-review
    provides: "Plans 01–04 assembled: per-file diff hash, Viewed persistence + PUT endpoint, client Viewed control, two-pane file tree + scroll-spy"
provides:
  - "End-to-end confirmation that DIFF-01..04 hold in the running app (incl. restart-persistence and change-reset, which only surface at runtime)"
  - "UAT polish fixes applied during the human-verify checkpoint (viewport pin, expanded-diff gap, GitHub stacking headers, bidirectional tree-jump)"
type: execute
autonomous: false
requirements: [DIFF-01, DIFF-02, DIFF-03, DIFF-04]
---

# 22-05 — End-to-end verification (human-verify)

Verification-only plan (no source files in scope): run the full automated gate,
then a human walkthrough of DIFF-01..04 against a live task worktree. Approved by
the user after the polish fixes below.

## Task 1 — Full-phase automated gate

Run from repo root:

| Gate | Result |
|------|--------|
| `go build ./...` | ✅ exit 0 |
| `go vet ./...` | ✅ exit 0 |
| `go test ./...` | ✅ exit 0 (incl. Plan 01 `TestFileHash` + Plan 02 Viewed/keep-history/FK-cascade tests) |
| `cd web && npm run build` (tsc -b + vite) | ✅ exit 0 |
| `cd web && npm run lint` | ⚠️ exit 1 — **20 pre-existing errors, 0 introduced by Phase 22** |

The lint red is the documented backlog (STATE.md pending todos: "~18–20 pre-existing
react-hooks eslint errors") across 15 files Phase 22 never authored. All five files
Phase 22 created/modified (`diffs.ts`, `checkbox.tsx`, `DiffFileSection.tsx`,
`FileTree.tsx`, `use-scroll-spy.ts`, `DiffTab.tsx`) lint clean. Not a phase gap.

## Task 2 — Human-verify DIFF-01..04 (blocking gate) — APPROVED

User confirmed against a live worktree:
- **DIFF-01** — left file tree lists all changed files (path-compressed, folders expanded); tree click scroll-jumps; scroll-spy highlight follows the top file; `PanelLeft` toggles the pane; tree click is scroll-only.
- **DIFF-02** — per-file collapse/expand works independently of Viewed.
- **DIFF-03** — check → collapse + dim + blue; state survives tab switch AND server restart (server-persisted).
- **DIFF-04** — a changed viewed file returns un-viewed + expanded; unchanged viewed files stay collapsed; revert restores the checkmark (keep-history). Binary strict-D-01 limitation understood, not treated as a defect.

## UAT polish fixes applied during the checkpoint

Runtime issues surfaced during the walkthrough and were fixed on `master` (all
frontend; backend untouched; build green; 0 new lint):

1. **`34c3226`** — Pin the app shell to the viewport (`h-svh` + `overflow-hidden` on the shadcn `SidebarProvider`, which was only `min-h-svh`). Tall diff content had grown the shell past the viewport so the whole window scrolled, dragging the sidebar + file tree up. Now scrolling is confined to the diff's right pane; the tree stays fixed.
2. **`e265301`** — Remove the empty band at the top of expanded diffs. The header was `sticky top-[41px]` inside an `overflow-hidden` Collapsible (its own sticky containing block), so at rest the header shifted down 41px within its box. Dropped `overflow-hidden`.
3. **`8ee54f5`** — Move the "N files changed" totals bar OUT of the scroll container into a fixed header, so nothing can render above it.
4. **`1db8a32`** — GitHub-style **stacking** file headers: flattened the rounded per-file cards into a shared full-width list (Collapsible root `display:contents`) with separator borders, so the current file's header stays pinned under the totals bar until the next pushes it up (always exactly one header pinned). Scroll-spy now tracks the headers directly (band top 0 → pinned header is topmost in-band = active). User chose this behavior over "cards without sticky."
5. **`ad3366a` → `1f18a04`** — Bidirectional tree-jump. Measuring the sticky header (`scrollIntoView`, then its `offsetTop`) resolved to its pinned position, so downward jumps worked but upward jumps did nothing. Added a zero-height non-sticky `[data-diff-anchor]` at each file's flow top and scroll to its `offsetTop` (a static element's `offsetTop` is the true flow position) — correct in both directions. `data-diff-path` stays on the header for the scroll-spy.

## Verification

- Automated: backend `go build`/`go vet`/`go test ./...` green; frontend `npm run build` green; `npm run lint` shows only the pre-existing backlog (0 new).
- Human: all four DIFF-01..04 criteria confirmed in the running app, including restart-persistence, change-reset, and the polished sticky/scroll behaviors.

## Self-Check: PASSED

- [x] Full automated gate green (lint red is pre-existing backlog only)
- [x] Human confirmed DIFF-01..04 against a live worktree
- [x] UAT polish fixes committed on master, build + lint (0 new) verified per fix
- [x] Binaries rebuilt (`bin/kamacu`) so the user tested the current embedded frontend

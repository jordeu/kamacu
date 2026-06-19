---
phase: 18-project-icon-data-foundation
plan: 03
subsystem: frontend-api
tags: [frontend, typescript, tanstack-query, wire-path, icons]
requires:
  - "Plan 01/02 Go wire shape (icon_letters / icon_color json tags) — the TS mirror target (D-12)"
provides:
  - "TS Project interface with non-null icon_letters + icon_color string fields"
  - "useUpdateProjectSettings PATCH carrying optional icon_letters? + icon_color? (omitted = untouched)"
affects:
  - "Phase 19 editors: read icon_letters/icon_color off the project list/GET and write them via this PATCH path"
tech-stack:
  added: []
  patterns:
    - "Conditional PATCH body assembly: start from {description}, add a field only when !== undefined (D-09 partial-PATCH, omitted key = untouched)"
key-files:
  created: []
  modified:
    - "web/src/api/types.ts"
    - "web/src/api/mutations.ts"
decisions:
  - "D-12: TS Project interface mirrors the Go wire shape — icon_letters/icon_color both non-null string (never null on the wire)"
  - "D-09: useUpdateProjectSettings sends each icon field only when defined; an omitted field is never put on the PATCH body, matching the backend partial-PATCH contract"
metrics:
  duration: "~3m"
  completed: "2026-06-19"
  tasks: 1
  files: 2
---

# Phase 18 Plan 03: Frontend Icon Wire Path Summary

Mirrored the backend icon wire shape into the frontend: added non-null `icon_letters` + `icon_color` to the TS `Project` interface and extended `useUpdateProjectSettings` to carry both as optional, only-sent-when-defined PATCH fields — establishing the typed read/write path Phase 19's editors will consume, with no UI added.

## What Was Built

### Task 1: Add icon fields to Project type + useUpdateProjectSettings payload (commit `bf7bb02`)

- **`web/src/api/types.ts`** — Added `icon_letters: string` and `icon_color: string` to the `Project` interface, positioned between `managed: boolean` and `created_at`, with a comment mirroring the `managed` field's style noting they are the v1.7 icon identity (two uppercase letters + a curated-palette `#rrggbb` hex), both NOT NULL on the wire (never null), serialized by the backend as `json:"icon_letters"` / `json:"icon_color"` (D-12, mirrors the Go json tags from Plans 01/02).

- **`web/src/api/mutations.ts`** — Extended `useUpdateProjectSettings`:
  - Added `icon_letters?: string` and `icon_color?: string` to the `mutationFn` argument type.
  - Replaced the two-branch `github_repo === undefined ? {description} : {description, github_repo}` ternary with a conditionally-assembled `body` object: it starts from `{ description }` and adds `github_repo`, `icon_letters`, and `icon_color` only when each is `!== undefined`. An omitted field is therefore never placed on the PATCH body, matching the backend partial-PATCH contract (D-09: omitted = untouched).
  - The existing `onSuccess` invalidation of `["projects"]` is unchanged.
  - No UI, swatch picker, or input component added — that is Phase 19. This only establishes the typed wire path.

## Verification

- `cd web && npm run build` → exit 0 (tsc -b + vite build pass; the extended `Project` type + mutation compile cleanly).
- `cd web && npm run lint` → 20 pre-existing errors, all in unrelated files (`TaskPage.tsx`, `use-mobile.ts`, `Board.tsx`, several `components/*` and `ui/*`); the two files modified by this plan (`src/api/types.ts`, `src/api/mutations.ts`) are lint-clean and do not appear in the lint output. Pre-existing lint errors in untouched files are out of scope (SCOPE BOUNDARY) and were not modified.
- Grep checks: `icon_letters: string` + `icon_color: string` present in `types.ts`; `icon_letters?: string` + `icon_color?: string` present in `mutations.ts`.
- Payload omit semantics: confirmed by reading the assembled `body` object — an `undefined` `icon_letters`/`icon_color`/`github_repo` is never added, so it is never sent.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Restored declared frontend dependencies in the worktree**
- **Found during:** Task 1 verification (`npm run build` failed with `sh: 1: tsc: not found`).
- **Issue:** The fresh git worktree had no `web/node_modules` (worktree-isolation artifact), so the locally-pinned `tsc`/`vite` toolchain was unavailable and the build/lint could not run.
- **Fix:** Ran `npm ci` in `web/`, which restores exactly the dependencies already declared in the committed `package-lock.json` (633 packages, 0 vulnerabilities). This is a restore of already-declared deps from the committed lockfile, NOT installation of a new/unknown package — so it is not the slopsquatting-risk install excluded from Rule 3 and required no human-verify checkpoint. `node_modules` is gitignored and was not committed.
- **Files modified:** None tracked (node_modules is gitignored).
- **Commit:** N/A (no source change).

**2. [Rule 3 - Blocking] Reverted regenerated `web/dist/index.html` build placeholder**
- **Found during:** Task 1 verification (after `npm run build`).
- **Issue:** `npm run build` regenerated `web/dist/index.html` to point at a new JS bundle hash. `web/dist/*` is gitignored except the committed `index.html` placeholder (kept only so `go:embed dist` never breaks). Committing the regenerated `index.html` would have pointed the committed placeholder at a gitignored bundle hash absent from a clean checkout/CI (where `make build` regenerates everything). It is also not in the plan's `files_modified`.
- **Fix:** `git checkout -- web/dist/index.html` to restore the committed placeholder; left dist regeneration to the normal `make build` pipeline.
- **Files modified:** None (reverted to committed state).
- **Commit:** N/A.

## Known Stubs

None. The two icon fields are real typed wire surface, not placeholders. No UI was added this phase by design (Phase 19 owns rendering and the editor), which the plan explicitly scopes.

## Self-Check: PASSED

- `web/src/api/types.ts` — FOUND (contains `icon_letters: string` + `icon_color: string`)
- `web/src/api/mutations.ts` — FOUND (contains `icon_letters?: string` + `icon_color?: string`)
- Commit `bf7bb02` — FOUND in git log

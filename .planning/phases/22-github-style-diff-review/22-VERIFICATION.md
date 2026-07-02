---
phase: 22-github-style-diff-review
verified: 2026-07-02T09:14:34Z
status: passed
score: 5/5 must-haves verified
overrides_applied: 0
re_verification:
  # No previous VERIFICATION.md existed — this is initial verification.
---

# Phase 22: GitHub-Style Diff Review Verification Report

**Phase Goal:** The diff tab reviews changes the way GitHub's "Files changed" does — a left file tree, per-file collapse/expand, and a sticky per-file "Viewed" state that only resets when the file actually changes again.
**Verified:** 2026-07-02T09:14:34Z
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | DIFF-01: left-hand tree of all changed files; selecting a file scrolls to its diff | ✓ VERIFIED | `FileTree.tsx` builds a nested, path-compressed tree from `files`; `DiffTab.tsx` renders it in a `w-72 shrink-0` left pane; leaf click calls `onSelect=scrollToPath` (scroll-only via `container.scrollTo({top: anchor.offsetTop})` on the `[data-diff-anchor]` marker); `useScrollSpy` IntersectionObserver drives `activePath` highlight; `PanelLeft` toggle hides/shows the pane. Rendered live in `TaskPage.tsx:380 <DiffTab taskId={task.id} />`. |
| 2 | DIFF-02: each file's diff can be individually collapsed/expanded | ✓ VERIFIED | `DiffFileSection.tsx` uses a CONTROLLED `Collapsible` (`open`/`onOpenChange={setOpen}`) with a `CollapsibleTrigger` chevron independent of Viewed; manual collapse never mutates viewed state (separate `setOpen` path). |
| 3 | DIFF-03: marking Viewed collapses the file; persists across reopen and server restart | ✓ VERIFIED | Checkbox `onCheckedChange` → `toggle.mutate({path,hash,viewed})` + `setOpen(!viewed)`. Persistence: `diff_viewed` SQLite table (migration 00011, embedded via goose, applies to v11 in tests) keyed by `(task_id,file_path,diff_hash)`; `GET /api/tasks/{id}/diff` read-merges rows into `File.Viewed`. Backend test `TestViewedPersistToggleAndKeepHistory` proves toggle + reopen persistence; `TestViewedFKCascadeOnTaskDelete` proves cascade. Restart persistence is architecturally guaranteed (state on disk in SQLite, re-read on every GET) and was confirmed by the approved human-verify checkpoint (22-05). |
| 4 | DIFF-04: a changed viewed file resets to un-viewed + re-expanded; unchanged viewed files stay collapsed+viewed | ✓ VERIFIED | Server mints a new `hash` when rendered content changes (`hashFile` over Status/Binary/OldPath/Hunks); no `diff_viewed` row matches the new hash → `viewed=false`. `DiffTab` keys sections `key={`${file.path}:${file.hash}`}` → new hash remounts the section; `useState(!file.viewed && changedLines<=400)` → un-viewed file re-opens. Unchanged files keep their hash → row matches → stay viewed+collapsed. Backend `TestViewedPersistToggleAndKeepHistory` proves hash-change→viewed:false with old row lingering (keep-history) and revert→viewed:true restore. Confirmed by approved human-verify (22-05). |
| 5 | Full backend + frontend gates are green | ✓ VERIFIED | `go build ./...` exit 0; `go vet ./...` exit 0; `go test ./internal/diff/... ./internal/api/... ./internal/store/...` PASS (incl. `TestFileHash` 5/5 sub-tests + all Viewed tests); `cd web && npm run build` exit 0. `npm run lint` reports exactly 20 pre-existing errors, 0 introduced by Phase 22 (see Anti-Patterns note). |

**Score:** 5/5 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/diff/parse.go` | `File.Hash` + `File.Viewed` fields (json `hash`/`viewed`) | ✓ VERIFIED | Both fields present with json tags and intent doc-comments incl. strict-D-01 binary note. |
| `internal/diff/diff.go` | pure `hashFile` helper; `Compute` sets `File.Hash` post-sort | ✓ VERIFIED | `hashFile(File) string` is a length-prefixed sha256 helper (Status/Binary/OldPath/Hunks in; Base/Path/counts out); `for i := range files { files[i].Hash = hashFile(files[i]) }` runs after `sort.Slice`. |
| `internal/store/migrations/00011_diff_viewed.sql` | keep-history table, composite PK, FK ON DELETE CASCADE | ✓ VERIFIED | `CREATE TABLE diff_viewed` with `PRIMARY KEY (task_id, file_path, diff_hash)`, `REFERENCES tasks(id) ON DELETE CASCADE`, no separate index; goose Up/Down. Applies to v11 in tests. |
| `internal/api/diffs.go` | GET Viewed read-merge + `PUT /api/tasks/{id}/diff/viewed` | ✓ VERIFIED | Drained SELECT closed before write; `setViewed` INSERT-on-true (`ON CONFLICT DO NOTHING`) / DELETE-on-false with `?` placeholders; validates empty path, >4096 path, non-64-hex hash → 400. Route registered in `DiffRoutes`. |
| `web/src/api/diffs.ts` | `DiffFile` hash+viewed; `useToggleViewed` optimistic mutation | ✓ VERIFIED | Fields added; `useToggleViewed` mirrors `useMoveTask` (cancel→snapshot→setQueryData→onError rollback→onSettled invalidate), PUT to `/diff/viewed`. |
| `web/src/components/ui/checkbox.tsx` | shadcn checkbox via unified radix-ui import | ✓ VERIFIED | `import { Checkbox as CheckboxPrimitive } from "radix-ui"`; no new npm dependency (radix-ui@1.5.0 present). |
| `web/src/components/task/DiffFileSection.tsx` | sibling Viewed checkbox, controlled collapse, sticky header, dim-when-viewed, data-diff-path | ✓ VERIFIED | Checkbox is a `<label>` sibling of `CollapsibleTrigger` with `onClick stopPropagation`; blue-500 checked override; `data-diff-path` + `data-diff-anchor`; binary branch has checkbox; dims on viewed. |
| `web/src/components/task/FileTree.tsx` | client buildTree + path compression + scroll-only rows | ✓ VERIFIED | `buildTree` + `compressDir` single-child folding; recursive render; leaf click = `onSelect` only (no useToggleViewed); selected state = `border-blue-500 bg-accent`; viewed dim + Check glyph. |
| `web/src/hooks/use-scroll-spy.ts` | IntersectionObserver scroll-spy returning topmost active path | ✓ VERIFIED | Observer `root: scrollEl`, `threshold: 0`, observes `[data-diff-path]`, topmost-in-band → activePath, `disconnect()` on cleanup, deps `[scrollRef, pathListKey]`. |
| `web/src/components/task/DiffTab.tsx` | two-pane layout, scrollRef, activePath, PanelLeft, FileTree wiring, key=path:hash | ✓ VERIFIED | Two-pane `flex h-full min-h-0`; fixed totals bar outside scroll pane; `scrollRef` on right pane; `useScrollSpy`; `scrollToPath` w/ `CSS.escape` + prefers-reduced-motion; section `key={path:hash}`; `taskId` passed. |

### Key Link Verification

| From | To | Via | Status | Details |
|------|-----|-----|--------|---------|
| `DiffFileSection` checkbox | `useToggleViewed` | `onCheckedChange → mutate({path,hash,viewed})` + `setOpen(!viewed)` | ✓ WIRED | Present in `DiffFileSection.tsx` viewedLabel. |
| `useToggleViewed` | `PUT /api/tasks/{id}/diff/viewed` | `put()` + optimistic `setQueryData(["diff",taskId])` | ✓ WIRED | `put<void>(`/api/tasks/${taskId}/diff/viewed`, {path,hash,viewed})`. |
| `FileTree` row click | right-pane scroll container | `scrollRef → scrollTo(anchor.offsetTop)` (scroll-only) | ✓ WIRED | `onSelect=scrollToPath` → `container.scrollTo`; jumps to `[data-diff-anchor]` (UAT fix for bidirectional jump). |
| `use-scroll-spy` IntersectionObserver | `activePath` | `root=scrollRef`, observe `[data-diff-path]`, topmost intersecting | ✓ WIRED | activePath returned and passed to FileTree. |
| `DiffTab` | `FileTree` + `DiffFileSection` | two-pane flex; activePath highlight; onSelect=scrollToPath | ✓ WIRED | `w-72` tree pane + right pane render loop. |
| `api.get` (`h.get`) | `diff_viewed` table | drained SELECT scoped `WHERE task_id=?`, sets `File.Viewed` | ✓ WIRED | Cursor closed before response; keep-history match at current hash. |
| `DiffRoutes` | `h.setViewed` | `PUT /api/tasks/{id}/diff/viewed` | ✓ WIRED | Route registered; `DiffRoutes` called in production at `cmd/kamacu/main.go:186`. |
| `DiffTab` (UI) | task view | `<DiffTab taskId={task.id} />` | ✓ WIRED | Rendered in `web/src/pages/TaskPage.tsx:380` — feature reachable. |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|--------------|--------|--------------------|--------|
| `DiffTab` | `data.files` | `useTaskDiff` → `GET /api/tasks/{id}/diff` → `diff.Compute` (real git plumbing) + `diff_viewed` SELECT | Yes | ✓ FLOWING |
| `FileTree` | `files` prop | Passed from `DiffTab` `data.files` (not hardcoded) | Yes | ✓ FLOWING |
| Viewed checkbox | `file.viewed` | Server read-merge; toggle → real INSERT/DELETE | Yes | ✓ FLOWING |
| `FileSection` collapse | `open` state | Derived from `file.viewed` + `changedLines`; remount on `path:hash` | Yes | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Rendered-per-file hash determinism + content-sensitivity + D-01 + rename≠modify + binary stability | `go test ./internal/diff/... -run TestFileHash -v -count=1` | 5/5 sub-tests PASS | ✓ PASS |
| Viewed persist/toggle + keep-history + revert-restore | `go test ./internal/api/... -run TestViewedPersistToggleAndKeepHistory -count=1` | PASS | ✓ PASS |
| FK cascade prunes Viewed rows on task delete | `go test ./internal/api/... -run TestViewedFKCascadeOnTaskDelete -count=1` | PASS | ✓ PASS |
| PUT input validation (empty path / bad hash / oversize) → 400 | `go test ./internal/api/... -run TestViewedRejectsBadInput -count=1` | PASS | ✓ PASS |
| Migration 00011 applies cleanly to a fresh DB | (observed in test output) | `goose: successfully migrated database to version: 11` | ✓ PASS |
| Frontend type-check + bundle | `cd web && npm run build` | exit 0 (2223 modules) | ✓ PASS |

### Probe Execution

Not applicable — this phase declares no `scripts/*/tests/probe-*.sh` probes. Verification is via the Go test suite and the frontend build/lint gate (executed above), plus the approved human-verify checkpoint.

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|-------------|-------------|--------|----------|
| DIFF-01 | 22-04, 22-05 | Left-hand tree of all changed files; selecting scrolls to its diff | ✓ SATISFIED | `FileTree` + `DiffTab` two-pane + `useScrollSpy`; scroll-only click; rendered in TaskPage. |
| DIFF-02 | 22-03, 22-04, 22-05 | Each file's diff can be individually collapsed/expanded | ✓ SATISFIED | Controlled `Collapsible` with chevron trigger, independent of Viewed. |
| DIFF-03 | 22-01, 22-02, 22-03, 22-05 | Viewed checkbox collapses file; persists across reopen + restart | ✓ SATISFIED | `diff_viewed` table + read-merge + PUT toggle; backend tests + approved human-verify for restart. |
| DIFF-04 | 22-01, 22-02, 22-03, 22-04, 22-05 | Changed viewed file auto-resets un-viewed + re-expanded; unchanged stay collapsed+viewed | ✓ SATISFIED | Hash-keyed rows + `key=path:hash` remount; backend keep-history test + approved human-verify. |

All four requirement IDs (DIFF-01..04) are declared across the plan frontmatter and map to Phase 22 in REQUIREMENTS.md. No orphaned requirements — every ID is accounted for and satisfied.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| `web/src/components/task/DiffTab.tsx` | 53 | `react-hooks/set-state-in-effect` lint error on the `setShowLoading` effect | ℹ️ Info | PRE-EXISTING: git blame shows this effect was authored in commit `cd70308 feat(05-04)` on 2026-06-11 (Phase 05), preserved verbatim through the Phase 22 restructure. Part of the documented 20-error backlog in STATE.md. Phase 22 introduced 0 new lint errors. Not a phase gap. |
| (misc) | — | grep hits for "placeholder"/"todo" | ℹ️ Info (false positives) | `diffs.ts:49` = comment on TanStack `placeholderData`; `diffs_test.go:294` = `'todo'` kanban status literal in test SQL; `diffs.go:173` = "SQL `?` placeholders" prose. None are stubs or debt markers. |

No `TBD`/`FIXME`/`XXX` blocker debt markers in any Phase 22 file. No empty-implementation or hollow-prop stubs found. The full lint run reports exactly 20 pre-existing errors across 15 files Phase 22 never authored (Board.tsx, PRCard.tsx, ReviewColumn.tsx, TerminalPane.tsx, useTerminalSocket.ts, use-mobile.ts, TaskPage.tsx, SettingsField.tsx, CleanupWorktreeDialog.tsx, RenameProjectDialog.tsx, plus react-refresh config warnings on button.tsx/sidebar.tsx/tabs.tsx/StatusDot.tsx) — matching the documented backlog exactly.

### Human Verification Required

None outstanding. The phase's runtime-only behaviors — DIFF-03 persist-across-restart and DIFF-04 reset-on-change — were verified through the formal `checkpoint:human-verify` gate in Plan 22-05 (Task 2), which the user **APPROVED** (recorded in `22-05-SUMMARY.md`, incl. UAT polish fixes for viewport pin, expanded-diff gap, GitHub stacking headers, and bidirectional tree-jump). All four DIFF-01..04 criteria were confirmed against a live worktree. These same behaviors are additionally backed by passing store/api-layer Go tests and are deterministic/inspectable in the codebase. No new human-verification items remain.

### Gaps Summary

No gaps. The phase goal is achieved end-to-end:

- **Backend** (`internal/diff`, `internal/api`, migration 00011) delivers a deterministic rendered-per-file hash, a keep-history `diff_viewed` table, a GET read-merge, and a validated PUT toggle — all registered in production (`cmd/kamacu/main.go:186`) and covered by passing tests (`TestFileHash`, `TestViewedPersistToggleAndKeepHistory`, `TestViewedFKCascadeOnTaskDelete`, `TestViewedRejectsBadInput`).
- **Frontend** (`DiffTab`, `FileTree`, `use-scroll-spy`, `DiffFileSection`, `checkbox`, `diffs.ts`) delivers the GitHub two-pane layout: fixed 288px path-compressed tree, scroll-only navigation, scroll-spy highlight, PanelLeft toggle, per-file controlled collapse, blue-500 sibling Viewed checkbox, and `key=path:hash` remount for auto-reset — rendered live in `TaskPage`.
- **Gates** are green (go build/vet/test, npm build); lint carries only the pre-existing 20-error backlog with 0 new errors from Phase 22 (the single Phase-22-file hit at `DiffTab.tsx:53` is Phase-05 code preserved verbatim).
- The accepted **binary strict-D-01 limitation** (a binary marked Viewed does not auto-reset on a byte-only change) is documented in code and the plans, and was explicitly understood/accepted at the human checkpoint — not a defect.

---

_Verified: 2026-07-02T09:14:34Z_
_Verifier: Claude (gsd-verifier)_

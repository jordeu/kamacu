# Phase 22: GitHub-Style Diff Review - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-07-02
**Phase:** 22-github-style-diff-review
**Areas discussed:** Reset trigger (DIFF-04), Viewed on binary/new/deleted, Tree highlight (scroll-spy), Tree presentation defaults

> Note: the visual + interaction contract was already locked in `22-UI-SPEC.md` (two-pane layout, 288px tree, tree-click = scroll-only, `blue-500` Viewed checkbox as sibling of the collapse trigger, sticky headers, silent auto-reset, no "N of M viewed" progress, no split view / per-line comments). Discussion covered only the behavioral/semantic decisions the UI-SPEC deferred to "server-side."

---

## Reset trigger (DIFF-04) — what counts as "the file changed again"

| Option | Description | Selected |
|--------|-------------|----------|
| Rendered per-file diff | Hash the exact diff shown for that file (hunks/patch). Resets iff what you'd re-review changed — GitHub semantics. No reset on base movement that doesn't alter the file's diff. | ✓ |
| Worktree file content | Hash the file's current bytes (git blob OID). Resets on any content change, but can drift from "what I'm reviewing." | |
| Base-inclusive signature | Hash base-commit + patch. Also resets when the base branch advances even if the file's diff is byte-identical. More frequent resets. | |

**User's choice:** Rendered per-file diff
**Notes:** The diff API already hands the frontend structured per-file hunks, so hashing the rendered per-file patch server-side is cheap and authoritative.

## Reset trigger (DIFF-04) — retention of viewed rows

| Option | Description | Selected |
|--------|-------------|----------|
| Restore it (keep history) | Keep viewed rows per (task, path, hash). Reverting a file to a byte-identical previously-viewed diff restores its checkmark. Matches GitHub. Rows accumulate; prune on task delete. | ✓ |
| Only latest state | One row per (task, path): latest hash + viewed bool. Simpler/self-pruning, but reverting to a previously-viewed state does NOT restore the checkmark. | |

**User's choice:** Restore it (keep history)
**Notes:** Sets the persistence key to (task_id, file_path, diff_hash) and rules out a latest-only row.

---

## Viewed on binary / new / deleted files

| Option | Description | Selected |
|--------|-------------|----------|
| All files incl. binary | Every changed file gets a Viewed checkbox, including binary (dims + tree checkmark; collapse is a no-op for header-only binary). Consistent, GitHub-like. | ✓ |
| Only files with hunks | Binary (header-only) files have no Viewed control; only text files can be marked viewed. New/deleted still count. | |

**User's choice:** All files incl. binary
**Notes:** No file type is excluded from Viewed. New/deleted files carry hunks and behave like modified files.

---

## Tree highlight — scroll-spy vs click-only

| Option | Description | Selected |
|--------|-------------|----------|
| Scroll-spy (follows scroll) | Highlighted tree row tracks the file at the top of the viewport (IntersectionObserver) and updates on click. Matches UI-SPEC's "file currently scrolled to." | ✓ |
| Click-only | Highlight only reflects the last clicked file; scrolling doesn't move it. Simpler but can look out-of-sync. | |

**User's choice:** Scroll-spy (follows scroll)
**Notes:** Observes the right pane's `DiffFileSection` headers; tree clicks stay scroll-only (no collapse/viewed mutation).

---

## Tree presentation — folder default

| Option | Description | Selected |
|--------|-------------|----------|
| Expanded | Folders start expanded; whole review visible at once. Path-compress single-child folders GitHub-style. | ✓ |
| Collapsed | Folders start collapsed; drill in. Better for very large change sets. | |

**User's choice:** Expanded

## Tree presentation — nested tree vs tree/flat toggle

| Option | Description | Selected |
|--------|-------------|----------|
| Nested tree only | Ship the nested directory tree the UI-SPEC locked; flat-list toggle deferred (DIFF-FUT). Tight scope. | ✓ |
| Add tree/flat toggle | Include a tree/flat-list switch like GitHub. More UI + state now. | |

**User's choice:** Nested tree only

---

## Claude's Discretion

- Exact viewed-state table name, columns, and indexing (key shape + keep-history locked; the rest is planner's).
- Whether the per-file hash + viewed state ride the existing `GET /api/tasks/{id}/diff` response or sit on sibling endpoints (recommend server-side hashing in `internal/diff`).
- Optimistic UI vs await-on-toggle for the Viewed checkbox write.

## Deferred Ideas

- Flat file-list toggle (tree/list switch) — DIFF-FUT.
- Resizable tree/diff splitter (fixed 288px this phase) — future enhancement.
- Keyboard file navigation (j/k / arrow jump) — not scoped; DIFF-FUT candidate.
- "N of M viewed" progress, split view, per-line comments, "changed since viewed" badge — DIFF-FUT-01/02/03, out of scope per UI-SPEC.

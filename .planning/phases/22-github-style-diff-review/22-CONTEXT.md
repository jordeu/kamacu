# Phase 22: GitHub-Style Diff Review - Context

**Gathered:** 2026-07-02
**Status:** Ready for planning

<domain>
## Phase Boundary

Turn the read-only single-scroll Diff tab into a GitHub "Files changed" review surface: a left-hand file tree, per-file collapse/expand, and a persistent per-file **"Viewed"** state that survives reopen + server restart and auto-resets (re-expands) only when that file's rendered diff changes again. Delivers DIFF-01..04.

Read-only review only — no per-line comments, no editing, no split view. The visual + interaction contract is already locked in `22-UI-SPEC.md`; this phase's discussion settled the **behavioral/semantic** decisions the UI-SPEC deferred to "server-side."

</domain>

<decisions>
## Implementation Decisions

### Viewed persistence & reset (DIFF-03 / DIFF-04)
- **D-01 — Reset trigger = rendered per-file diff hash.** The "content hash" that keys Viewed state is computed over the exact diff shown for that file (its hunks / patch text) — NOT the worktree blob, and NOT a base-inclusive signature. Viewed un-checks iff what you'd re-review actually changed (GitHub semantics). Base-branch movement that does not alter the file's rendered diff does NOT reset it.
- **D-02 — Keep viewed history (no collapse to latest-only).** Persist a row per `(task_id, file_path, diff_hash)`. A file that reverts to a byte-identical, previously-viewed diff restores its checkmark automatically. Rows accumulate per file over time — that is accepted; prune on task delete (FK cascade / existing cleanup). Do NOT store a single latest-hash-per-file row.
- **D-03 — Server-side persistence, new migration.** New DB table (next migration `00011_*`, embedded goose, run at startup) keyed task + path + rendered-diff hash so state survives restart (per STATE "Diff seams" note). The **key shape** (task, path, rendered-diff hash) and **keep-history** are locked here; exact columns/indexes are the planner's call.

### Viewed control scope
- **D-04 — Every changed file gets a Viewed checkbox, including binary.** Binary files render header-only today (no hunk body); marking one Viewed dims its header + shows the tree checkmark, and "collapse on view" is a harmless no-op for it. New and deleted files (which carry hunks) behave like modified files. No file type is excluded from Viewed.

### File-tree navigation (DIFF-01)
- **D-05 — Scroll-spy tree highlight.** The highlighted tree row follows whichever file is at the top of the diff viewport as you scroll (IntersectionObserver on the `DiffFileSection` headers), in addition to updating on click. Realizes the UI-SPEC's "file the right pane is currently scrolled to." Tree clicks remain **scroll-only** (never mutate collapse or Viewed) per UI-SPEC.
- **D-06 — Folders default expanded, path-compressed.** All folders start expanded so the whole review is visible at once (changed-file trees are usually small). Path-compress single-child folders GitHub-style (e.g. `internal/api`).
- **D-07 — Nested tree only.** No tree/flat-list toggle this phase (deferred, DIFF-FUT). Fixed 288px (`w-72`) tree pane, not resizable this phase (UI-SPEC).

### Claude's Discretion
- Exact viewed-state table name, columns, and indexing (key shape + keep-history are locked; the rest is planner's).
- Whether the per-file rendered-diff hash and viewed state ride the existing `GET /api/tasks/{id}/diff` response or sit on sibling read/write endpoints. **Recommendation:** compute the hash server-side in `internal/diff` (the same bytes that key persistence are authoritative — matches the structured-diff-on-server, dumb-map-on-client pattern); researcher/planner finalize the endpoint shape.
- Optimistic UI vs await-on-toggle for the Viewed checkbox write (either is acceptable; TanStack mutation + invalidate is the established pattern).

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase design contract
- `.planning/phases/22-github-style-diff-review/22-UI-SPEC.md` — the APPROVED visual + interaction contract: two-pane layout, `blue-500` Viewed checkbox as a **sibling** of the collapse trigger (no button-in-button), sticky file headers, silent auto-reset (no "changed since viewed" badge), copywriting, spacing/typography/color. MUST read first.

### Requirements & roadmap
- `.planning/REQUIREMENTS.md` §DIFF-01..DIFF-04 — the four requirements this phase delivers.
- `.planning/ROADMAP.md` §"Phase 22: GitHub-Style Diff Review" — goal + success criteria.
- `.planning/STATE.md` §"Diff seams (Phase 22)" — codebase grounding: current diff is a read-only unified-diff tab; DIFF-03/04 need per-file Viewed keyed by file + content hash (new persistence surviving restart, resetting on content change).

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `web/src/components/task/DiffTab.tsx` — the single-scroll container to split into two panes; totals bar + loading/empty/error/refetch states reused verbatim (UI-SPEC "preserved states").
- `web/src/components/task/DiffFileSection.tsx` — per-file collapsible section; header row is restructured into a flex row (`CollapsibleTrigger` takes `flex-1`; the Viewed checkbox is a sibling that `stopPropagation`s) and gains sticky positioning.
- `web/src/api/diffs.ts` — `useTaskDiff` query + `DiffResponse` / `DiffFile` types; `DiffFile` gains a per-file `hash` + viewed flag; add a viewed-toggle mutation.
- `internal/api/diffs.go` — `GET /api/tasks/{id}/diff` handler; the place to surface the per-file rendered-diff hash + viewed state (or add sibling viewed read/write endpoints).
- `internal/diff/diff.go` — `Compute()` builds the per-file hunk structure; the rendered-diff hash should be derived here from each file's patch/hunks (authoritative bytes).
- shadcn `checkbox` primitive is NOT yet present — add via `npx shadcn add checkbox` (UI-SPEC); `switch` exists but is the wrong affordance.

### Established Patterns
- **Structured-diff-on-server, dumb-map-on-client:** the server pre-structures per-file hunks; the frontend never parses diffs. The Viewed hash must be server-computed to stay authoritative.
- **goose migrations** at `internal/store/migrations/` (embedded, run at startup); latest `00010_muted_palette.sql`, next `00011_*`. Idempotent startup hooks exist (`BackfillProjectIcons`) if a backfill is ever needed.
- **Fetch-on-tab-activation, no polling (D-61):** DiffTab mounts only while active; refetch keeps prior data rendered (no blanking).
- **Style literals:** hover `bg-[#27272a]`, focus `ring-blue-500`, `tabular-nums` counts, `−` (U+2212) for deletions — established across `DiffFileSection` / `TaskTabs`.
- **Verification model:** NO frontend test framework — verify via `cd web && npm run build` (tsc -b + vite build) + `npm run lint` + human-verify; backend via `go test ./...` / `go build` / `go vet`.

### Integration Points
- New viewed-state persistence table wired to whatever the Diff tab reads on open; the checkbox reflects persisted state on every open (incl. post-restart).
- The left tree pane is new UI inside the Diff tab; scroll-spy observes the right pane's `DiffFileSection` headers within its `overflow-y-auto` scroll container.

</code_context>

<specifics>
## Specific Ideas

- **"Like GitHub's Files changed"** is the explicit reference model: left file tree, per-file Viewed checkbox that collapses the file on check, sticky headers so Viewed stays reachable, Viewed persists and silently resets only when that file's diff changes. Path-compress single-child folders like GitHub.
- Reverting a file to an exact previously-viewed state should restore its checkmark (the reason for keep-history, D-02).
- Keep the existing tone: sentence-case copy, `−` (U+2212) minus for deletions, `tabular-nums` on all counts.

</specifics>

<deferred>
## Deferred Ideas

- **Flat file-list toggle** (GitHub's tree/list switch) — DIFF-FUT.
- **Resizable tree/diff splitter** — fixed 288px this phase; a resizable pane is a future enhancement.
- **Keyboard file navigation** (j/k / arrow jump between files) — not scoped this phase; DIFF-FUT candidate.
- **"N of M files viewed" progress**, **split (side-by-side) view**, **per-line review comments**, **"changed since viewed" badge / diff-of-diffs indicator** — all DIFF-FUT-01/02/03, explicitly out of scope per UI-SPEC.

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 22-github-style-diff-review*
*Context gathered: 2026-07-02*

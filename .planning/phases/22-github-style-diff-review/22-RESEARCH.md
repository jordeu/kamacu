# Phase 22: GitHub-Style Diff Review - Research

**Researched:** 2026-07-02
**Domain:** Go stdlib hashing + SQLite persistence (backend); React 19 + IntersectionObserver + Radix/shadcn (frontend); GitHub "Files changed" review UX
**Confidence:** HIGH

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01 — Reset trigger = rendered per-file diff hash.** The "content hash" that keys Viewed state is computed over the exact diff shown for that file (its hunks / patch text) — NOT the worktree blob, and NOT a base-inclusive signature. Viewed un-checks iff what you'd re-review actually changed (GitHub semantics). Base-branch movement that does not alter the file's rendered diff does NOT reset it.
- **D-02 — Keep viewed history (no collapse to latest-only).** Persist a row per `(task_id, file_path, diff_hash)`. A file that reverts to a byte-identical, previously-viewed diff restores its checkmark automatically. Rows accumulate per file over time — that is accepted; prune on task delete (FK cascade / existing cleanup). Do NOT store a single latest-hash-per-file row.
- **D-03 — Server-side persistence, new migration.** New DB table (next migration `00011_*`, embedded goose, run at startup) keyed task + path + rendered-diff hash so state survives restart. Key shape (task, path, rendered-diff hash) and keep-history are locked; exact columns/indexes are the planner's call.
- **D-04 — Every changed file gets a Viewed checkbox, including binary.** Binary files render header-only today (no hunk body); marking one Viewed dims its header + shows the tree checkmark, and "collapse on view" is a harmless no-op for it. New and deleted files (which carry hunks) behave like modified files. No file type is excluded from Viewed.
- **D-05 — Scroll-spy tree highlight.** The highlighted tree row follows whichever file is at the top of the diff viewport as you scroll (IntersectionObserver on the `DiffFileSection` headers), in addition to updating on click. Tree clicks remain **scroll-only** (never mutate collapse or Viewed).
- **D-06 — Folders default expanded, path-compressed.** All folders start expanded. Path-compress single-child folders GitHub-style (e.g. `internal/api`).
- **D-07 — Nested tree only.** No tree/flat-list toggle this phase. Fixed 288px (`w-72`) tree pane, not resizable.

### Claude's Discretion
- Exact viewed-state table name, columns, and indexing (key shape + keep-history are locked; the rest is planner's).
- Whether the per-file rendered-diff hash and viewed state ride the existing `GET /api/tasks/{id}/diff` response or sit on sibling read/write endpoints. **CONTEXT recommendation:** compute the hash server-side in `internal/diff` (the same bytes that key persistence are authoritative — matches the structured-diff-on-server, dumb-map-on-client pattern); researcher/planner finalize the endpoint shape.
- Optimistic UI vs await-on-toggle for the Viewed checkbox write (either acceptable; TanStack mutation + invalidate is the established pattern).

### Deferred Ideas (OUT OF SCOPE)
- Flat file-list toggle (DIFF-FUT).
- Resizable tree/diff splitter (fixed 288px this phase).
- Keyboard file navigation (j/k / arrow jump) — DIFF-FUT candidate.
- "N of M files viewed" progress, split (side-by-side) view, per-line review comments, "changed since viewed" badge / diff-of-diffs indicator — all DIFF-FUT, explicitly out of scope per UI-SPEC.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| DIFF-01 | Left tree of all changed files; selecting a file focuses/scrolls to its diff | Client-side tree build from `data.files` (§Area 4); scroll-only click via `scrollIntoView` + `scroll-mt` (§Pitfall 2); scroll-spy highlight via IntersectionObserver (§Area 3) |
| DIFF-02 | Each file's diff can be individually collapsed/expanded | Existing Radix `Collapsible` in `DiffFileSection.tsx`, promoted to **controlled** `open` state so collapse stays independent of Viewed (§Area 5, §Pitfall 9) |
| DIFF-03 | "Viewed" checkbox; marking it collapses the file; persists across reopen + restart | shadcn `checkbox` sibling of `CollapsibleTrigger` (§Area 5); server-side `diff_viewed` table keyed `(task_id, file_path, diff_hash)` (§Area 2); read rides the diff response, write on a sibling `PUT` endpoint |
| DIFF-04 | A previously-viewed file that changes resets to un-viewed + re-expanded; unchanged viewed files stay collapsed | Rendered-per-file SHA-256 hash computed in `internal/diff` (§Area 1); on open, `viewed = row-exists(task, path, currentHash)` — a changed file's new hash has no row → un-viewed (§Area 2) |
</phase_requirements>

## Summary

This phase is almost entirely **integration inside an already-mature codebase** — the diff pipeline (`internal/diff`), the read-only tab (`DiffTab.tsx` / `DiffFileSection.tsx`), the store discipline (hand-written `database/sql` on `modernc.org/sqlite`, embedded goose migrations), and the TanStack Query patterns are all in place and were read directly for this research. There are **no new npm registry dependencies**: the only addition is `npx shadcn add checkbox`, which copies a component into the repo using the already-installed `radix-ui@1.5.0` unified package (verified present, exports `Checkbox.Root` + `Checkbox.Indicator`). The genuinely new technical elements are five, and all five resolve cleanly with standard-library / already-present tools.

**Primary recommendation:** Compute a **SHA-256 of a canonically-serialized per-file rendered diff** inside `internal/diff.Compute()` (a pure, table-testable Go function — the strongest test seam in the phase). Persist Viewed as a keep-history `diff_viewed(task_id, file_path, diff_hash)` table with a composite primary key (no extra index, FK-cascade prunes on task delete). Ride `hash`+`viewed` on the existing `GET /api/tasks/{id}/diff` response; add one sibling `PUT /api/tasks/{id}/diff/viewed` write endpoint. On the client, build the file tree **client-side** from the flat `files` list (pure `useMemo`), drive scroll-spy with **one IntersectionObserver** whose `root` is the right-pane scroll container, and make the file section's `Collapsible` **controlled** so collapse and Viewed stay orthogonal.

The one decision that deserves user/planner confirmation is **binary-file hash semantics** (§Open Questions Q1): D-01 says "hash the rendered diff," but a binary file's rendered diff is a content-invariant "Binary file changed" header, so a strict reading means a binary file's Viewed state will *not* reset when its bytes change. `parse.go` does not currently capture the `index <oid>..<oid>` line that would allow byte-change detection. Flag this before locking.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Rendered per-file diff hash (D-01) | API/Backend (`internal/diff`) | — | Same bytes that key persistence must be authoritative; matches structured-diff-on-server pattern. Pure function, best Go test seam. |
| Viewed-state persistence (D-02/D-03) | Database/Storage (SQLite) | API/Backend (`internal/api`) | Survives restart; FK cascade prunes on task delete. |
| Viewed read (on open) | API/Backend | Frontend (TanStack Query) | Read rides the existing diff response; one SELECT per open. |
| Viewed toggle (write) | API/Backend | Frontend (TanStack mutation) | Sibling `PUT` endpoint; optimistic cache update client-side. |
| File-tree construction + path compression (D-06/D-07) | Browser/Client (React) | — | Pure presentation derived from the flat path list already in the response; no server round-trip. |
| Scroll-spy tree highlight (D-05) | Browser/Client | — | IntersectionObserver on the right-pane scroll container; DOM-only concern. |
| Per-file collapse/expand (DIFF-02) | Browser/Client | — | Existing Radix Collapsible, promoted to controlled state. |
| "Viewed" checkbox primitive | Frontend (shadcn/Radix) | — | Copied component, no server involvement. |

## Standard Stack

**Everything needed is already installed or in the Go standard library.** No version research was required beyond confirming presence.

### Core (already present)
| Library | Version | Purpose | Evidence |
|---------|---------|---------|----------|
| `crypto/sha256` + `encoding/hex` | Go stdlib (Go 1.26) | Rendered-diff hash | stdlib; no dependency added |
| `modernc.org/sqlite` | v1.52.0 | Viewed-state table (via `database/sql`) | `internal/store/store.go` — driver name `"sqlite"`, `foreign_keys(1)` pragma set, `SetMaxOpenConns(1)` |
| `github.com/pressly/goose/v3` | v3.27.1 | `00011_*` migration (embedded) | `internal/store/migrate.go` — `//go:embed migrations/*.sql`, dialect `"sqlite3"` |
| `net/http` ServeMux | Go stdlib | `PUT /api/tasks/{id}/diff/viewed` | `internal/api/diffs.go` `DiffRoutes` registers method+path wildcards |
| `radix-ui` | 1.5.0 | Checkbox primitive | `web/node_modules/radix-ui` — exports `Checkbox.Root`, `Checkbox.Indicator` (verified via `node -e`) |
| `@tanstack/react-query` | 5.101.0 | Viewed toggle mutation + invalidate | `web/src/api/mutations.ts` — `useMoveTask` optimistic pattern |
| `lucide-react` | 1.17.0 | `PanelLeft`, `ChevronRight`, `Check`, `Folder` icons | already used across task components |
| `IntersectionObserver` | Browser DOM (WebGL-era evergreen) | Scroll-spy | native; no polyfill for a localhost/desktop app |

### New shadcn primitive
| Component | Add command | Import it generates | Verified |
|-----------|-------------|---------------------|----------|
| `checkbox` | `npx shadcn add checkbox` | `import { Checkbox as CheckboxPrimitive } from "radix-ui"` using `CheckboxPrimitive.Root` + `CheckboxPrimitive.Indicator`, lucide check icon | `[VERIFIED: ui.shadcn.com registry radix-nova/checkbox.json]` + local `radix-ui` export check — matches the existing `web/src/components/ui/switch.tsx` unified-import pattern exactly |

**Installation:**
```bash
cd web && npx shadcn add checkbox   # copies web/src/components/ui/checkbox.tsx; no new npm dependency (radix-ui already installed)
```
Backend needs **no** `go get` — `crypto/sha256`, `encoding/hex`, `database/sql`, goose, and the ServeMux are all already imported in the tree.

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| shadcn `checkbox` | existing `switch` primitive | Wrong affordance per UI-SPEC — GitHub's "Viewed" is a checkbox, not a toggle. `switch` stays unused. |
| Hash rides `GET .../diff` response | Sibling `GET .../diff/viewed` read endpoint | Extra round-trip on every tab open for no benefit; the fetch-on-activation model (D-61) already re-reads on open. Riding the response is strictly simpler. |
| Client-side tree build | Server-side tree in the diff response | The flat `files` list is already sorted server-side; the tree is pure presentation. Server-side build would leak layout concerns into `internal/diff` and add a response shape to version. Keep it client-side. |
| SHA-256 | FNV / CRC32 | Collision resistance matters (a false collision = "viewed not resetting when it should"). SHA-256 is stdlib, fast enough at single-user localhost scale, and unambiguous. Non-crypto hashes trade safety for speed we don't need. |

## Package Legitimacy Audit

**No external registry packages are installed in this phase.** `npx shadcn add checkbox` copies a source file into `web/src/components/ui/checkbox.tsx`; it depends only on `radix-ui@1.5.0`, which is already a declared dependency in `web/package.json` and part of the CLAUDE.md-blessed stack. slopcheck was unavailable in this session, but there is nothing to check — the audit is N/A because zero new registry packages are added.

| Package | Registry | Disposition |
|---------|----------|-------------|
| `radix-ui` (Checkbox) | npm (already installed, v1.5.0) | Pre-existing dependency — no install |
| *(no other packages)* | — | — |

## Current Code — What Was Read

Every recommendation below cites code read this session.

| File | Key facts for the planner |
|------|---------------------------|
| `internal/diff/diff.go` | `Compute(ctx, wt, base) (*Diff, error)` builds `[]File` (diff.go:85). Pure over git output; no DB access. Files sorted by path ascending (diff.go:179). This is where the per-file hash is derived. |
| `internal/diff/parse.go` | `Diff`/`Totals`/`File`/`Hunk`/`Line` structs (parse.go:30-66). `File` has `Path, OldPath, Status, Binary, Additions, Deletions, Hunks`. **`parse.go` does NOT capture the `index <oid>..<oid>` line** (verified — only `diff --git`, `---`/`+++`, `Binary files`, mode lines, rename lines are parsed). Relevant to binary-hash Open Question Q1. |
| `internal/api/diffs.go` | `DiffRoutes(mux, db, wt)` registers only `GET /api/tasks/{id}/diff` (diffs.go:20-23). Handler `h.get` resolves base, calls `diff.Compute`, sets `d.Base`, `writeJSON` (diffs.go:103-109). `diffHandlers` already holds `db` (diffs.go:25-28) — the viewed read + write endpoints slot in here with no new wiring. |
| `web/src/api/diffs.ts` | `DiffResponse`/`DiffFile`/`DiffHunk`/`DiffLine` types; `useTaskDiff(taskId)` query keyed `["diff", taskId]`, `staleTime:0` refetch-on-mount (diffs.ts:45-50). `DiffFile` gains `hash`+`viewed`; add a toggle mutation. |
| `web/src/components/task/DiffTab.tsx` | The single scroll container: `flex h-full min-h-0 flex-col overflow-y-auto p-4` (DiffTab.tsx:84) with a sticky totals bar `sticky top-0 z-10 ... px-3 py-2` (DiffTab.tsx:85) and stacked `DiffFileSection`s (DiffTab.tsx:126-128). Loading/empty/error/refetch states preserved verbatim (UI-SPEC). This becomes the **two-pane** row; the right pane is the IO root + scroll target. |
| `web/src/components/task/DiffFileSection.tsx` | Header row IS the `CollapsibleTrigger` today (`flex w-full ... px-3 py-2`, DiffFileSection.tsx:59). `defaultOpen={changedLines <= 400}` (uncontrolled, DiffFileSection.tsx:56). Binary branch is a non-collapsible header (DiffFileSection.tsx:33-52). Both branches need the Viewed checkbox; the trigger must shrink to `flex-1` with the checkbox as a sibling. |
| `web/src/components/task/TaskTabs.tsx` | **Precedent for the sibling-not-child rule:** the tab × close is a `<span role="button" tabIndex={0}>` with `onClick`/`onKeyDown` + `e.stopPropagation()` (TaskTabs.tsx:104-125) — "button-in-button is invalid HTML inside the TabsTrigger." The Viewed checkbox follows the same sibling structure. |
| `web/src/api/mutations.ts` | `useMoveTask` is the canonical optimistic pattern: `onMutate` cancels + snapshots + `setQueryData`, `onError` rolls back, `onSettled` invalidates (mutations.ts:148-168). Copy this shape for the Viewed toggle. |
| `internal/settings/settings.go` | The upsert idiom: `INSERT ... ON CONFLICT(key) DO UPDATE ...` with `?` placeholders (settings.go `Set`). `Get` treats `sql.ErrNoRows` as "absent = default." |
| `internal/api/icons.go` | `BackfillProjectIcons` documents the **`SetMaxOpenConns(1)` deadlock rule**: never hold a SELECT cursor open while issuing writes on the same connection (icons.go:178-183) — collect-then-write. All SQL uses `?` placeholders, never string concat (T-18-01). |
| `internal/store/store.go` | `foreign_keys(1)` pragma is set in the DSN; `SetMaxOpenConns(1)`. FK cascade will fire (given the pragma) — but tests that open a raw DB must also enable it. |
| `internal/store/migrate.go` | Embedded goose, dialect `"sqlite3"`, run at startup in `main.go:98`. `00011_*` is the next migration. |
| `internal/store/migrations/00007_github_foundations.sql` | Confirms SQLite 3.53 supports `DROP COLUMN` / `ALTER` directly and the one-statement-per-line goose style; `tasks(id)` is the FK target with `ON DELETE CASCADE` established since `00001_init.sql`. |
| `cmd/kamacu/main.go` | `store.Migrate(db)` at :98, `api.DiffRoutes(mux, db, wtSvc)` at :186. No startup backfill is needed for `diff_viewed` (empty table is correct on first boot — nothing to seed). |

---

## Area 1 — Rendered-Per-File Diff Hash (D-01)

**Recommendation:** add a `Hash string \`json:"hash"\`` field to `diff.File` (parse.go:45) and compute it inside `Compute()` (diff.go:85) via a pure helper `hashFile(f File) string` using `crypto/sha256`. The authoritative bytes are the **parsed, rendered per-file structure** — exactly what the client renders (dumb-map-on-client) — serialized deterministically and hashed. `Compute` stays DB-free; the handler layers Viewed on top (Area 2).

### What bytes are authoritative — and why
Per D-01 the hash must cover "the exact diff shown for that file (its hunks / patch text)" and **exclude** the base and the worktree blob so base movement that doesn't change the rendered diff does not reset Viewed. Because the diff is computed with three-dot merge-base semantics (diff.go:85-101), a base commit that never touches file *F* leaves *F*'s hunks byte-identical — so hashing the hunks already gives the D-01 "base movement doesn't reset" property for free.

**Include** (the rendered, per-file content):
- `Status` (modified/new/deleted/renamed) — a status change is a visible change.
- `Binary` flag.
- `OldPath` (rendered as "old → new" for renames).
- Every `Hunk.Header` (the `@@ … @@` line, shown verbatim) and every `Line.Kind` + `Line.Text`, in order.

**Exclude:**
- `Base` (D-01: not base-inclusive).
- `Path` — it is the separate key column `file_path`. Two distinct files with byte-identical rendered content correctly get the same `diff_hash` but different `file_path` rows (composite key disambiguates; verified against keep-history semantics).
- `Additions`/`Deletions` — derived from the hunks; including them is harmless but redundant. Recommend excluding for conceptual cleanliness.

### Serialization discipline (avoid collisions AND avoid spurious resets)
Use **length-prefixed writes** into the hash so no field value can be confused with a delimiter:
```go
// Source: pattern over internal/diff structs (parse.go:45-66); crypto/sha256 stdlib
func hashFile(f File) string {
    h := sha256.New()
    ws := func(s string) { fmt.Fprintf(h, "%d:", len(s)); io.WriteString(h, s) }
    ws(f.Status)
    if f.Binary { ws("bin") } else { ws("txt") }
    if f.OldPath != nil { ws(*f.OldPath) } else { ws("") }
    fmt.Fprintf(h, "H%d:", len(f.Hunks))
    for _, hk := range f.Hunks {
        ws(hk.Header)
        fmt.Fprintf(h, "L%d:", len(hk.Lines))
        for _, ln := range hk.Lines {
            ws(ln.Kind); ws(ln.Text)
        }
    }
    return hex.EncodeToString(h.Sum(nil))
}
```
This is deterministic across runs and machines (no map iteration; slices preserve order). Set `f.Hash = hashFile(f)` for each file right before appending in `Compute` (diff.go:139 and diff.go:175 for the untracked branch), or in a single final pass after the sort (diff.go:179) — a final pass is cleanest and guarantees one code path for both tracked and untracked files.

### Endpoint shape — ride the existing response
Add `hash` to `DiffFile` and surface it on `GET /api/tasks/{id}/diff` (no new read endpoint). CONTEXT's discretion recommendation and the fetch-on-open model (D-61) both point here: the tab already re-reads on every activation, so the hash + viewed arrive together with zero extra round-trips.

**Landmines:**
- **Do not hash the raw patch string** you might be tempted to re-fetch — `Compute` already discarded it into structs. Hash the structs (what's actually rendered). Re-shelling `git diff` for hashing would be slower and could diverge from what's shown.
- **Line-number-only shifts:** hunk headers contain line numbers. If file *F*'s own content changed, its hunks (and headers) change → hash changes → reset (correct). If only an unrelated base commit moved, *F*'s hunks are untouched → hash stable (correct). There is no scenario under three-dot semantics where an unchanged rendered *F* gets new hunk-header numbers, so hashing headers is safe.
- **Binary files:** see Open Question Q1 — a strict "rendered only" hash cannot detect binary byte changes because the rendered output is a constant header.

**Confidence:** HIGH (stdlib hashing; struct shapes and three-dot semantics read directly).

---

## Area 2 — Keep-History Viewed Persistence (D-02, D-03)

### Table shape (`00011_*` migration)
```sql
-- +goose Up
CREATE TABLE diff_viewed (
  task_id    INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  file_path  TEXT    NOT NULL,
  diff_hash  TEXT    NOT NULL,
  created_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  PRIMARY KEY (task_id, file_path, diff_hash)
);
-- +goose Down
DROP TABLE diff_viewed;
```
Rationale, tied to locked decisions and the existing schema conventions (`00001_init.sql`, `00005_tmux_sessions.sql`):
- **Composite PK `(task_id, file_path, diff_hash)`** enforces keep-history uniqueness (D-02: a row *per* rendered diff, not latest-only) and doubles as the index. The "all viewed rows for a task" read uses the PK's **leftmost prefix** `task_id` — index-covered, so **no separate index is needed**. The exact-match delete uses the full PK.
- **`REFERENCES tasks(id) ON DELETE CASCADE`** prunes every viewed row when a task is deleted (D-02's "prune on task delete"). This matches the `00001_init.sql` FK style and requires the `foreign_keys(1)` pragma, which `store.go` already sets.
- **`created_at`** mirrors every other table's timestamp default; useful if a future "N of M viewed" or pruning-by-age feature lands (DIFF-FUT), free to include now.
- Naming: `diff_viewed` (parallels `tmux_sessions`, `agent_sessions`). Planner may rename; the shape is the recommendation.

**No startup backfill.** Unlike `BackfillProjectIcons`, an empty `diff_viewed` on first boot is exactly correct (nothing is viewed yet). Do not add a backfill hook.

### Query flow (hand-written `database/sql`, `?` placeholders only)
The store here is **hand-written `database/sql`** (not sqlc) — `internal/settings/settings.go` and `internal/api/icons.go` are the pattern to match. Put a small `internal/diffviewed` package (mirroring `internal/settings`) or inline helpers in `internal/api/diffs.go`; either matches house style. Three statements:

```go
// READ (in the diff handler, after diff.Compute, before writeJSON).
// One query, cursor closed before any write → safe under SetMaxOpenConns(1) (icons.go:178-183).
rows, _ := db.Query(`SELECT file_path, diff_hash FROM diff_viewed WHERE task_id = ?`, id)
// build set{ (path,hash) }, then: for i := range d.Files { d.Files[i].Viewed = set has (path, d.Files[i].Hash) }

// MARK viewed (idempotent):
db.Exec(`INSERT INTO diff_viewed(task_id, file_path, diff_hash) VALUES(?,?,?)
         ON CONFLICT(task_id, file_path, diff_hash) DO NOTHING`, id, path, hash)

// UNMARK viewed (exact row only — preserves other historical hashes, D-02):
db.Exec(`DELETE FROM diff_viewed WHERE task_id=? AND file_path=? AND diff_hash=?`, id, path, hash)
```

**Semantics walk-through (validates DIFF-03/DIFF-04):**
- On open, file *F* is `viewed = true` iff a row exists for `(task, F.path, F.currentHash)`.
- Mark viewed → insert `(task, F.path, H1)`.
- *F* changes to `H2` → on open, lookup `(task, F.path, H2)` → **no row** → un-viewed + expanded (DIFF-04). The `H1` row lingers (keep-history).
- *F* reverts to `H1` → row still present → checkmark restored automatically (D-02).
- Uncheck at `H1` → delete `(task, F.path, H1)` only; a later different hash `H3` is unaffected.

### Write endpoint
Register in `DiffRoutes` (diffs.go:20-23) — `diffHandlers` already has `db`:
```go
mux.HandleFunc("PUT /api/tasks/{id}/diff/viewed", h.setViewed)
```
Body: `{ "path": string, "hash": string, "viewed": bool }`. Validate `id` exists (the FK insert will fail on a bad task, but validate for a clean 404), reject empty `path`/`hash` with 400 (V5 input validation), then insert-or-delete. Respond `204 No Content` (the client already knows the new state; `onSettled` invalidate re-reads authoritative state).

**Trust the client-supplied hash on write — do NOT re-`Compute`.** The client sends the hash it just rendered. If *F* changed server-side between load and click, we'd insert a row for a now-stale hash; on the next open that stale row won't match *F*'s current hash → *F* shows un-viewed → which is the correct outcome (the file changed). The stale row is harmless keep-history. Re-computing the full diff on every toggle would be wasteful and buys nothing. (Recorded as an intentional design decision, not an oversight.)

**Landmines:**
- **`SetMaxOpenConns(1)` (store.go):** the read is a single SELECT fully drained + closed before `writeJSON`; the writes are single `Exec`s — none hold a cursor open during a write, so no deadlock. Only a *batched collect-then-write* (icons.go style) would need the collect-first discipline; the toggle doesn't.
- **FK cascade needs the pragma.** Production is fine (`store.go`). Backend tests that open a bare DB must set `foreign_keys(1)` or the cascade-on-task-delete assertion silently passes without cascading.
- **Never string-concat `path`/`hash` into SQL** — `?` placeholders only (T-18-01, icons.go:183).

**Confidence:** HIGH (schema conventions, upsert idiom, connection discipline all read from existing code).

---

## Area 3 — IntersectionObserver Scroll-Spy in an Overflow Container (D-05)

**Recommendation:** one `IntersectionObserver` created in a `useEffect` at the **DiffTab level** (it owns both panes and the scroll container), with `root` = the right-pane scroll container ref, observing each file section's wrapper element. Track intersecting sections and set `activePath` = the **topmost intersecting** section. Tree clicks call a separate `scrollToPath` that only scrolls (never mutates collapse/Viewed).

### Verified observer configuration
`[VERIFIED: developer.mozilla.org IntersectionObserver]` — `root` may be any scrollable ancestor element (an `overflow-y-auto` div), not just the viewport; `rootMargin` shrinks/grows the root's box (units: `px` and `%` only); `threshold` is the visibility ratio.

```ts
// root = the RIGHT pane scroll container (DiffTab.tsx:84 becomes the right pane).
// The right pane has a sticky totals bar at top:0; push the detection zone
// BELOW it with a negative top margin ≈ totals-bar height, and make it a thin
// band near the top with a large negative bottom margin.
const observer = new IntersectionObserver(onIntersect, {
  root: scrollRef.current,
  rootMargin: `-${TOTALS_BAR_PX}px 0px -70% 0px`, // active zone = the top ~30% under the totals bar
  threshold: 0,
});
```

Callback picks the topmost section currently in the band:
```ts
const visible = new Map<string, number>(); // path -> boundingClientRect.top
function onIntersect(entries: IntersectionObserverEntry[]) {
  for (const e of entries) {
    const path = (e.target as HTMLElement).dataset.diffPath!;
    if (e.isIntersecting) visible.set(path, e.boundingClientRect.top);
    else visible.delete(path);
  }
  let topPath: string | null = null, topY = Infinity;
  for (const [p, y] of visible) if (y < topY) { topY = y; topPath = p; }
  if (topPath) setActivePath(topPath);
}
```

### Sticky-header interaction — observe the WRAPPER, not the sticky header
The file header row becomes `position: sticky` (UI-SPEC). **Do not observe the sticky header** — a sticky element's reported `boundingClientRect` reflects its pinned position, so multiple pinned headers can read as "at the top" simultaneously and confuse the topmost calc. **Observe the non-sticky section wrapper** (the `Collapsible` root `<div>`, which carries the whole section and whose top edge is its true flow position). Tag each wrapper `data-diff-path={file.path}` so the observer resolves paths without prop-drilling refs.

### Element discovery without ref plumbing
Give each section wrapper a stable `data-diff-path` attribute (DiffFileSection already renders one root `<div>` per file). In the effect, `scrollRef.current!.querySelectorAll('[data-diff-path]')` and `observer.observe(el)` each. Re-run the effect when the **file list changes** (depend on a joined path string, e.g. `files.map(f => f.path).join('\n')`), and `observer.disconnect()` in cleanup. This avoids a `Map<path, ref>` threaded through `DiffFileSection`.

### React 19 / cleanup pitfalls
- **Recreate on list change, disconnect on cleanup.** StrictMode double-invokes the effect in dev; the `disconnect()` cleanup makes that safe (second run re-observes the same nodes). Depend on the path-list string, not the `files` array identity.
- **Collapse does not require re-observe.** Radix unmounts only `CollapsibleContent`; the observed wrapper stays mounted (its height just shrinks). The observer stays valid across collapse/expand.
- **`eslint-plugin-react-hooks` is on** (`eslint.config.js`) and `npm run lint` is a gate — satisfy `exhaustive-deps` on the effect (include `scrollRef`'s stability and the path-list dep). Prefer `useLayoutEffect` only if a first-paint highlight flash appears; `useEffect` is fine otherwise.
- **`threshold: 0`** is correct here — the band is defined by `rootMargin`, not the ratio; a nonzero threshold would fight tall sections.

### Click = scroll-only (deterministic, per UI-SPEC & Decisions)
```ts
function scrollToPath(path: string) {
  const el = scrollRef.current!.querySelector(`[data-diff-path="${CSS.escape(path)}"]`);
  const reduce = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  el?.scrollIntoView({ block: 'start', behavior: reduce ? 'auto' : 'smooth' });
}
```
Pair with `scroll-mt-[${TOTALS_BAR_PX}px]` on each section wrapper so `block:'start'` lands the header just under the sticky totals bar (see Pitfall 2). The click never touches collapse or Viewed state.

**Optional refinement (note, don't require):** a programmatic smooth scroll fires many callbacks, so the highlight can flicker through intermediate files mid-animation before settling on the target. For the "usually small" changed-file trees (D-06 rationale) this is barely perceptible. If UAT dislikes it, gate spy updates behind a "programmatic scroll in progress" flag cleared on `scrollend` (or a timeout). Present as Option B; default is the simple version.

**`TOTALS_BAR_PX`:** measure the totals bar via a ref + `getBoundingClientRect().height` (robust to font/zoom), or use a constant with a comment (`py-2` + `text-sm` + border ≈ ~41px). Measuring is safer; recommend a small `useState` set from a `ResizeObserver`/ref on the totals bar, or a documented constant if the bar height is truly fixed.

**Confidence:** HIGH for the observer config (verified against MDN); MEDIUM for the exact `rootMargin` percentages/`TOTALS_BAR_PX` (tune during implementation/UAT — these are cosmetic thresholds, not correctness).

---

## Area 4 — File Tree Construction + Path Compression (D-06, D-07)

**Recommendation:** build the tree **client-side** in a pure `useMemo` from `data.files`. No server involvement.

### Why client-side
The `files` array is already sorted by path server-side (diff.go:179) and carries everything a tree needs (path, status, additions, deletions, viewed). The tree is pure presentation; building it server-side would push layout concerns into `internal/diff` and add a versioned response shape for zero benefit. Memoize on the path list so it only rebuilds when files change.

### Algorithm
```ts
type TreeNode =
  | { type: 'dir'; name: string; path: string; children: TreeNode[] }
  | { type: 'file'; name: string; path: string; file: DiffFile };

function buildTree(files: DiffFile[]): TreeNode[] {
  // 1. Insert each file by splitting file.path on '/', creating dir nodes as needed.
  // 2. Path-compress (D-06): fold any dir whose children is exactly ONE dir (and no
  //    file directly under it) into "name/childName", recursively (GitHub-style,
  //    e.g. internal/api). Stop folding at the first dir that has >1 child or a file.
  // 3. Sort: dirs before files, each alphabetical (matches server path sort).
}
```
- **Path compression** collapses single-child directory *chains* (`internal` → `api` → `diffs.go` renders `internal/api` as one row when `internal` and `api` each have a single child). Fold only when a dir has exactly one child that is itself a dir; keep folding down the chain.
- **Folders default expanded (D-06):** per-directory local open state, all initialized `true`. A directory row toggles its own subtree (collapse/expand the tree, unrelated to the diff's per-file collapse).
- **Leaf rows** show `font-mono text-sm truncate` filename + `+add −del` mini-counts (`text-xs tabular-nums`, green/red, `−` = U+2212), plus a `Check` glyph (`size-3 muted-foreground`) when `file.viewed`, and the selected border/bg when `path === activePath` (UI-SPEC rows spec).

### Pane structure
The tree pane is `w-72 shrink-0 border-r border-border overflow-y-auto` (fixed 288px, D-07). It sits to the left of the existing scroll container inside a new `flex h-full min-h-0` row that replaces DiffTab's current single-column root (DiffTab.tsx:84). A `PanelLeft` ghost icon button in the totals bar toggles pane visibility (`Hide file tree` / `Show file tree` aria — UI-SPEC).

**Landmines:**
- **Rename rows:** `file.path` is the *new* path; the tree keys on the new path (matches the diff section). The "old → new" rendering stays in the diff section, not the tree.
- **Empty/single-file trees:** with one changed file the tree is a single leaf (possibly under a compressed path) — render it; the pane still shows.
- **`activePath` selection is presentation only** — it never changes the tree's own dir open/close state, and clicking a leaf never mutates diff collapse/Viewed (scroll-only).

**Confidence:** HIGH (pure client logic over an already-available, already-sorted list).

---

## Area 5 — shadcn `checkbox` as a Sibling of the Collapse Trigger

**Recommendation:** `npx shadcn add checkbox`. It generates `web/src/components/ui/checkbox.tsx` importing `import { Checkbox as CheckboxPrimitive } from "radix-ui"` (`CheckboxPrimitive.Root` + `.Indicator`, lucide check) — identical convention to `web/src/components/ui/switch.tsx`. No new npm dependency (`radix-ui@1.5.0` already installed and exports `Checkbox`).

### Wiring — sibling, not child (avoids button-in-button)
`CheckboxPrimitive.Root` renders a real `<button role="checkbox">`, so it **cannot** nest inside the `CollapsibleTrigger` `<button>`. This is the exact constraint the `TaskTabs` × close already obeys (`<span role="button">` sibling with `stopPropagation`, TaskTabs.tsx:104-125). Restructure the `DiffFileSection` header:

```tsx
// BEFORE: the whole header row IS the trigger (DiffFileSection.tsx:59).
// AFTER: a flex row DIV; trigger takes flex-1; checkbox is a sibling.
<Collapsible open={open} onOpenChange={setOpen} data-diff-path={file.path}
  className="group/diff-file overflow-hidden rounded-lg border border-border scroll-mt-[41px]">
  <div className="sticky top-[41px] z-[5] flex items-center gap-1 bg-card px-3 py-2">
    <CollapsibleTrigger className="flex flex-1 items-center gap-1 outline-none hover:bg-[#27272a] focus-visible:ring-2 focus-visible:ring-blue-500">
      <ChevronRight className="size-4 shrink-0 transition-transform group-data-[state=open]/diff-file:rotate-90" />
      <span className={`flex-1 truncate text-left font-mono text-sm ${file.viewed ? 'text-muted-foreground' : ''}`}>{pathLabel}</span>
      {/* status suffix + ±counts, dimmed to muted-foreground when file.viewed */}
    </CollapsibleTrigger>
    <label className="flex items-center gap-1 text-xs text-muted-foreground">
      <Checkbox
        checked={file.viewed}
        onClick={(e) => e.stopPropagation()}
        onCheckedChange={(c) => onToggleViewed(file.path, file.hash, c === true)}
        aria-label={file.viewed ? `Mark ${leaf} as not viewed` : `Mark ${leaf} as viewed`}
        className="data-[state=checked]:border-blue-500 data-[state=checked]:bg-blue-500 data-[state=checked]:text-white"
      />
      Viewed
    </label>
  </div>
  <CollapsibleContent>…</CollapsibleContent>
</Collapsible>
```
Notes:
- **`blue-500`, not shadcn default `primary`** (UI-SPEC Decision 2): the default checkbox fill is near-white in dark; override with `data-[state=checked]:bg-blue-500`/`border-blue-500`. `bg-blue-500`/`ring-blue-500` are already used across the codebase (DiffFileSection.tsx:59, TaskTabs.tsx) so the utility is available (Tailwind v4 default palette).
- **`e.stopPropagation()` on the checkbox** so toggling Viewed doesn't also fire the collapse trigger.
- **Sticky header** on the wrapping `<div>`, offset by the totals-bar height; `scroll-mt-*` on the `Collapsible` root for click-to-scroll landing.
- **Binary branch** (DiffFileSection.tsx:33-52) also gets the checkbox sibling (D-04); collapse-on-view is a no-op there (it's already non-collapsible).

### Collapse ↔ Viewed coupling (controlled Collapsible)
Today the Collapsible is **uncontrolled** (`defaultOpen`, DiffFileSection.tsx:56). To honor "marking Viewed collapses, unchecking re-expands, but collapse stays independently toggleable" (DIFF-02 vs DIFF-03), promote it to **controlled**:
- Local `const [open, setOpen] = useState(!file.viewed && changedLines <= 400)`.
- Marking Viewed → `onToggleViewed(...true)` **and** `setOpen(false)`.
- Unchecking → `onToggleViewed(...false)` **and** `setOpen(true)`.
- The trigger's `onOpenChange={setOpen}` keeps manual collapse working without touching Viewed.
- Re-seed `open` when `file.viewed` flips server-side across a refetch (auto-reset re-expands a changed file): sync via an effect on `file.viewed`, or remount by keying the section on `` `${file.path}:${file.hash}` `` so a changed file gets a fresh section with the correct initial `open`. **Keying on `path:hash` is the cleanest** — a changed file (new hash) remounts un-viewed + expanded automatically, matching DIFF-04, and avoids effect-sync bugs.

### Toggle mutation (client)
```ts
// web/src/api/diffs.ts — mirror useMoveTask optimistic pattern (mutations.ts:148-168)
export function useToggleViewed(taskId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ path, hash, viewed }: { path: string; hash: string; viewed: boolean }) =>
      put(`/api/tasks/${taskId}/diff/viewed`, { path, hash, viewed }),
    onMutate: async (v) => {
      await qc.cancelQueries({ queryKey: ["diff", taskId] });
      const prev = qc.getQueryData<DiffResponse>(["diff", taskId]);
      qc.setQueryData<DiffResponse>(["diff", taskId], (d) =>
        d ? { ...d, files: d.files.map(f => f.path === v.path ? { ...f, viewed: v.viewed } : f) } : d);
      return { prev };
    },
    onError: (_e, _v, ctx) => qc.setQueryData(["diff", taskId], ctx?.prev),
    onSettled: () => qc.invalidateQueries({ queryKey: ["diff", taskId] }),
  });
}
```
Optimistic is worth it here (the collapse-on-view feedback should be instant); the pattern already exists (`useMoveTask`). CONTEXT marks await-only as equally acceptable — but with optimistic, the checkbox + dim + collapse all reflect immediately.

**Confidence:** HIGH (checkbox source + import verified against the shadcn registry and local `radix-ui`; sibling precedent and mutation pattern read from the tree).

---

## Architecture Patterns

### System data flow (Phase 22)
```
TAB OPEN ─────────────────────────────────────────────────────────────────────┐
 GET /api/tasks/{id}/diff                                                       │
   ├─ diff.Compute(wt, base)  ──► []File{…, Hash: hashFile(f)}   (internal/diff)│
   ├─ SELECT file_path,diff_hash FROM diff_viewed WHERE task_id=?  (internal/api)│
   │     └─ set File.Viewed = (path,hash) ∈ viewedSet                            │
   └─ JSON { files:[ {path,status,hunks,hash,viewed}, … ] }                      │
        │                                                                        │
        ▼ web/src/api/diffs.ts useTaskDiff (queryKey ["diff", id])              │
   ┌────┴───────────────────────────────────────────────────────────────────┐  │
   │ DiffTab (flex row, owns scrollRef + activePath + IntersectionObserver)  │  │
   │  ├─ FileTree  ◄── buildTree(files) (useMemo) ── click ─► scrollToPath   │  │
   │  │     ▲ activePath highlight                                            │  │
   │  │     └──────────────── scroll-spy (IO on scrollRef) ◄─┐               │  │
   │  └─ RightPane (scrollRef, sticky totals bar + PanelLeft) │               │  │
   │        └─ DiffFileSection[] (key=path:hash)  ───────────┘               │  │
   │              header: [trigger flex-1] [Checkbox ☐ Viewed] (siblings)    │  │
   └────────────────────────────────────────────────────────────────────────┘  │
        │ check "Viewed"                                                         │
        ▼ useToggleViewed (optimistic setQueryData + invalidate)                │
   PUT /api/tasks/{id}/diff/viewed {path,hash,viewed}                            │
     ├─ viewed=true  → INSERT … ON CONFLICT DO NOTHING                           │
     └─ viewed=false → DELETE WHERE task_id,file_path,diff_hash  ────────────────┘
```

### Recommended file touch-list (for the planner)
```
internal/diff/parse.go        # + File.Hash field
internal/diff/diff.go         # + hashFile(); set f.Hash in Compute
internal/diff/diff_test.go    # hash stability / change / rename / binary tests (primary Go seam)
internal/api/diffs.go         # + Viewed read after Compute; + PUT .../diff/viewed handler + route
internal/api/diffs_test.go    # viewed read/write + FK-cascade tests
internal/store/migrations/00011_diff_viewed.sql   # new table
web/src/api/diffs.ts          # + hash,viewed on DiffFile; + useToggleViewed
web/src/components/ui/checkbox.tsx                 # npx shadcn add checkbox
web/src/components/task/DiffTab.tsx                # two-pane row; scrollRef; IO; activePath; PanelLeft toggle
web/src/components/task/DiffFileSection.tsx        # sticky header; controlled Collapsible; Checkbox sibling; data-diff-path; dim-when-viewed
web/src/components/task/FileTree.tsx               # NEW: buildTree + recursive render (client-side)
```

### Anti-patterns to avoid
- **Client-side diff parsing / client-side hashing.** The hash must be server-authoritative (structured-diff-on-server). The client only echoes `file.hash` back on toggle.
- **Observing the sticky header for scroll-spy.** Observe the non-sticky wrapper (§Area 3).
- **Uncontrolled Collapsible + separate "viewed collapses it" hack.** Use a controlled `open` + `key={path:hash}` remount (§Area 5).
- **A second GET endpoint for viewed state.** Ride the existing response.
- **Re-`Compute` on every toggle to validate the hash.** Trust the client hash; keep-history makes stale rows harmless (§Area 2).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Content fingerprint for reset | Custom string-compare / ad-hoc hash | `crypto/sha256` + length-prefixed serialization | Collision-safe, stdlib, deterministic; a bad hash = wrong Viewed resets |
| "Which file is at the top?" | `scroll` listener + `getBoundingClientRect` per frame | `IntersectionObserver` (root = scroll container) | Jank-free, batched, evergreen-native |
| Checkbox control | Custom `<input>` / div toggle | shadcn `checkbox` (Radix `Checkbox.Root`) | Accessibility, focus ring, `onCheckedChange`, matches design system |
| Collapse/expand | Custom height animation | Existing Radix `Collapsible` (controlled) | Already in the tree; unmounts closed content |
| Persistence + cascade prune | Bespoke JSON file / latest-only map | SQLite table + composite PK + FK cascade | Survives restart; prunes on task delete; matches store discipline |
| Migration runner | Manual `CREATE TABLE` at boot | goose `00011_*` (embedded) | Versioned, idempotent, run-at-startup (migrate.go) |

## Common Pitfalls

### Pitfall 1: Button-in-button (checkbox inside the collapse trigger)
**What goes wrong:** nesting `Checkbox.Root` (a `<button>`) inside `CollapsibleTrigger` (a `<button>`) is invalid HTML; React/hydration and click semantics break.
**Avoid:** make the checkbox a **sibling** of a `flex-1` trigger inside a flex `<div>` (TaskTabs.tsx:104-125 precedent); `stopPropagation` on the checkbox.

### Pitfall 2: `scrollIntoView` hides the header under the sticky totals bar
**What goes wrong:** `block:'start'` scrolls the section top to the container top — behind the sticky totals bar.
**Avoid:** `scroll-mt-[TOTALS_BAR_PX]` on each section wrapper (CSS scroll-margin). Honor `prefers-reduced-motion` (behavior `'auto'`).

### Pitfall 3: IntersectionObserver defaults to the viewport, not the scroll container
**What goes wrong:** omitting `root` observes intersections with the browser viewport, so the whole diff pane reads as "visible" and the spy never fires per-file.
**Avoid:** set `root: scrollRef.current` (the `overflow-y-auto` right pane). Create the observer only after the ref is attached (in `useEffect`).

### Pitfall 4: Stale/duplicated observer across refetch + StrictMode
**What goes wrong:** not disconnecting leaks observers; not re-observing after the file list changes tracks removed nodes; StrictMode double-mounts.
**Avoid:** `observer.disconnect()` in the effect cleanup; depend the effect on the joined path list so it rebuilds when files change; re-query `[data-diff-path]` nodes each run.

### Pitfall 5: Hash under/over-sensitivity
**What goes wrong:** hashing too little → collision → Viewed doesn't reset when it should (DIFF-04 fails). Hashing non-deterministically (map order) → spurious resets.
**Avoid:** length-prefixed serialization over ordered slices (§Area 1); include status/binary/oldPath/hunks; exclude base/path.

### Pitfall 6: FK cascade silently disabled in tests
**What goes wrong:** a test that opens SQLite without `foreign_keys(1)` won't cascade-delete `diff_viewed` on task delete — the assertion passes without exercising the cascade.
**Avoid:** open test DBs via `store.Open` (which sets the pragma) or set it explicitly.

### Pitfall 7: `SetMaxOpenConns(1)` cursor-during-write deadlock
**What goes wrong:** holding the viewed-read cursor open while issuing a write on the same (single) connection deadlocks (icons.go:178-183).
**Avoid:** the read is a single fully-drained SELECT, closed before any write; toggle writes are standalone `Exec`s — no interleaving. Only add the collect-first discipline if a future batch path appears.

### Pitfall 8: Collapse and Viewed getting welded together
**What goes wrong:** driving collapse purely off `viewed` breaks independent collapse (DIFF-02).
**Avoid:** controlled `open` state seeded from `!viewed && changedLines<=400`, `key={path:hash}` remount on change, manual `onOpenChange` preserved.

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `@radix-ui/react-checkbox` scoped package | Unified `radix-ui` package (`Checkbox.Root`) | shadcn "new-york"/radix-nova styles, 2025 | `npx shadcn add checkbox` here imports from `radix-ui` — matches `switch.tsx`; no scoped install |
| `scroll` event scroll-spy | `IntersectionObserver` | Evergreen for years | Native, batched, correct for overflow containers |
| Uncontrolled Collapsible `defaultOpen` | Controlled `open` + remount key | this phase | Needed to couple Viewed→collapse while keeping collapse independent |

**Deprecated/outdated:** none relevant — the stack is current per CLAUDE.md (verified 2026-06).

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Binary files' Viewed should follow the *rendered* header (content-invariant), so a binary byte-change does NOT reset Viewed under a strict D-01 reading | Area 1 / Open Q1 | A user who edits a binary and sees it still "viewed" may consider it a bug; fixing later needs a parse.go change to capture blob OIDs |
| A2 | `TOTALS_BAR_PX` ≈ 41px (`py-2` + `text-sm` + border); used for `rootMargin` top offset, sticky `top`, and `scroll-mt` | Area 3/5 | Cosmetic only — headers land slightly off; measure at runtime to eliminate |
| A3 | Trusting the client-supplied hash on the toggle write (no server re-Compute) is acceptable | Area 2 | A stale row could persist; harmless under keep-history (won't match current hash) |
| A4 | `key={path:hash}` remount is preferred over effect-syncing `open` on `file.viewed` change | Area 5 | If remount causes a visible flash on auto-reset, fall back to an effect that re-seeds `open` |

**No `[ASSUMED]` package names** — the only package is `radix-ui`, already installed and CLAUDE.md-blessed.

## Open Questions

1. **Binary-file Viewed reset semantics (needs user/planner confirmation).**
   - What we know: D-01 hashes the *rendered* diff. A binary file renders a constant "Binary file changed" header (DiffFileSection.tsx:33-52) with no hunks. `parse.go` does **not** capture the `index <oldOID>..<newOID>` line (verified) — so a rendered-only hash is identical regardless of the binary's actual bytes.
   - What's unclear: should marking a binary Viewed and then changing its bytes reset Viewed (DIFF-04 "when a file changes again")? A strict rendered-only reading says **no**; user intent may say **yes**.
   - Recommendation: default to the strict D-01 reading (rendered-only) so nothing contradicts the locked decision, and **flag it**. If "yes" is wanted, add capture of the binary `index abc..def` blob OIDs in `parse.go` and fold the new OID into `hashFile` **for binary files only**. Cheap, localized, but it is a (minor) deviation from "not the worktree blob" — hence confirm before locking.

2. **`TOTALS_BAR_PX` source.** Measure at runtime (ref + `getBoundingClientRect`) vs. hardcode a constant. Recommend measuring to survive font/zoom changes; a constant is acceptable if the bar height is provably fixed. (Cosmetic; not a correctness gate.)

## Environment Availability

Skip — the phase adds no external tools/services. `git` (already required, the app's premise), SQLite (embedded `modernc`), and the browser's native `IntersectionObserver` are the only "dependencies," all present. `npx shadcn add checkbox` needs network once at dev time to fetch the component; it is a one-shot code-copy, not a runtime dependency.

## Testing / Verification Seams

> `workflow.nyquist_validation` is **false** in `.planning/config.json`, so no formal Nyquist test map is required. This section captures the **real seams** the orchestrator flagged: backend Go tests. There is **no frontend test framework** (verified: no vitest/jest/testing-library in `web/package.json`).

**Frontend verification (unchanged house rule):** `cd web && npm run build` (`tsc -b && vite build`) + `npm run lint` (eslint, react-hooks) + human-verify against UI-SPEC.

**Backend Go seams (add tests here):**

| Seam | Test type | Command | Notes |
|------|-----------|---------|-------|
| `hashFile` / `Compute` hash field | unit (real git in `t.TempDir()`) | `go test ./internal/diff/...` | Extend `diff_test.go` (its `TestMain` isolates git config, diff_test.go:18-31). Assert: (a) same content → same hash across two `Compute`s; (b) editing a file → different hash; (c) an unrelated base commit that doesn't touch file *F* → *F*'s hash unchanged (proves D-01 "base movement doesn't reset"); (d) rename vs modify with identical hunks → different hash; (e) binary hash stability (documents Open Q1 behavior). |
| Viewed read + toggle | unit/integration | `go test ./internal/api/...` | Extend `diffs_test.go`. Assert: insert-then-read marks the file viewed; delete unmarks; a file whose hash changed reads un-viewed while the old-hash row lingers (keep-history); reverting to the old hash restores viewed. |
| FK cascade on task delete | integration | `go test ./internal/api/...` | Open via `store.Open` (pragma on); delete a task; assert `diff_viewed` rows for it are gone. |
| Build/vet gates | — | `go build ./... && go vet ./...` | Standard. |

## Security Domain

> `security_enforcement` is not set in config; this is a **single-user, local-only, no-auth** app (CLAUDE.md). The applicable controls are narrow.

| ASVS Category | Applies | Standard Control |
|---------------|---------|------------------|
| V5 Input Validation | yes | The `PUT .../diff/viewed` handler validates `id` (404 if task missing), rejects empty `path`/`hash` (400). `path`/`hash` are opaque strings persisted as-is — never interpolated into SQL. |
| V6 Cryptography | yes (hashing only) | `crypto/sha256` for the content fingerprint — never hand-roll. Not a security hash (integrity of review state), but stdlib is correct anyway. |
| V2 Auth / V3 Session / V4 Access Control | no | Localhost single-user; no auth surface added. |

| Pattern | STRIDE | Mitigation |
|---------|--------|-----------|
| SQL injection via `path`/`hash` | Tampering | `?` placeholders only (T-18-01, icons.go:183); never string-concat |
| Cross-task viewed leakage | Info disclosure | Every query is scoped `WHERE task_id = ?`; composite PK includes `task_id` |

## Sources

### Primary (HIGH confidence)
- **Codebase (read this session):** `internal/diff/{diff,parse,diff_test}.go`, `internal/api/{diffs,routes,settings,icons}.go`, `internal/store/{store,migrate}.go` + `migrations/000{01,04,05,07,10}.sql`, `cmd/kamacu/main.go`, `web/src/api/{diffs,mutations,client,queries}.ts`, `web/src/components/task/{DiffTab,DiffFileSection,TaskTabs}.tsx`, `web/src/components/ui/{switch,collapsible}.tsx`, `web/{package.json,components.json,eslint.config.js,src/index.css}`, `.planning/config.json`, `22-CONTEXT.md`, `22-UI-SPEC.md`, `STATE.md`, `REQUIREMENTS.md`.
- **`radix-ui@1.5.0`** — local `node -e` export check: `Checkbox` namespace exports `Root`, `Indicator` (+ `unstable_*`).
- **`https://ui.shadcn.com/r/styles/radix-nova/checkbox.json`** [VERIFIED] — checkbox.tsx imports `from "radix-ui"`, uses `CheckboxPrimitive.Root` + `.Indicator`.
- **`https://developer.mozilla.org/en-US/docs/Web/API/IntersectionObserver/IntersectionObserver`** [VERIFIED] — `root` may be any scrollable element; `rootMargin` shrinks/grows the root box (px/% only); `threshold` semantics.

### Secondary (MEDIUM confidence)
- `https://ui.shadcn.com/docs/components/checkbox` — install command + controlled `checked`/`onCheckedChange` API.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — nothing new to install; all versions read from `package.json`/CLAUDE.md and confirmed present.
- Area 1 (hash): HIGH — stdlib; struct shapes + three-dot semantics read directly.
- Area 2 (persistence): HIGH — schema conventions, upsert idiom, connection discipline all read from existing code.
- Area 3 (scroll-spy): HIGH config (MDN-verified), MEDIUM on exact `rootMargin`/`TOTALS_BAR_PX` tuning.
- Area 4 (tree): HIGH — pure client logic over an already-sorted list.
- Area 5 (checkbox): HIGH — registry source + local export verified; sibling precedent in-tree.
- Open Q1 (binary hash): flagged for confirmation — the one genuine semantic ambiguity.

**Research date:** 2026-07-02
**Valid until:** ~2026-08-01 (stable stack; re-verify shadcn checkbox import only if `radix-ui` majors or the shadcn CLI style changes).

# Phase 22: GitHub-Style Diff Review - Pattern Map

**Mapped:** 2026-07-02
**Files analyzed:** 11 (7 modified/new backend+frontend, 1 new migration, 1 new store, 1 new tree component, 1 new hook, 1 generated primitive)
**Analogs found:** 9 with strong analogs / 11 total (2 net-new client concerns — tree + scroll-spy — have partial analogs only)

> Every excerpt below is verbatim from the live tree with file:line. The planner
> should reference these directly in plan action steps — signatures, store
> discipline (`?` placeholders, `SetMaxOpenConns(1)`), and component conventions
> (sibling-not-child, `radix-ui` unified import) must be replicated exactly.

---

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `internal/diff/parse.go` (MODIFY) | model / struct | transform | itself (`File` struct, parse.go:45-53) | exact (add field) |
| `internal/diff/diff.go` (MODIFY) | service / pure compute | transform | itself (`countHunkLines`, diff.go:199) + `crypto/sha256` stdlib | exact (add helper) |
| `internal/api/diffs.go` (MODIFY) | controller | request-response + CRUD | itself (`h.get`, diffs.go:34) + `internal/api/settings.go` (`h.put`, settings.go:57) | exact / role-match |
| `internal/diffviewed/*.go` (NEW store pkg) | store / persistence | CRUD | `internal/settings/settings.go` + `internal/api/icons.go:184` | role-match |
| `internal/store/migrations/00011_diff_viewed.sql` (NEW) | migration | DDL | `internal/store/migrations/00005_tmux_sessions.sql` | exact |
| `web/src/api/diffs.ts` (MODIFY) | hook (query+mutation) + model | CRUD / request-response | itself (`useTaskDiff`, diffs.ts:45) + `web/src/api/mutations.ts` (`useMoveTask`, mutations.ts:148) | exact |
| `web/src/components/task/DiffTab.tsx` (MODIFY) | component | event-driven + request-response | itself (DiffTab.tsx) | exact (restructure) |
| `web/src/components/task/DiffFileSection.tsx` (MODIFY) | component | request-response | itself + `web/src/components/task/TaskTabs.tsx:101-129` (sibling ×) | exact (restructure) |
| `web/src/components/task/FileTree.tsx` (NEW) | component + utility | transform | tree render: DiffFileSection header rows; pure `buildTree`: `applyMove` (mutations.ts:123) | partial (no recursive-tree analog) |
| `web/src/hooks/use-scroll-spy.ts` (NEW) | hook | event-driven | `web/src/hooks/use-mobile.ts` (subscribe/cleanup DOM hook) | partial (no IntersectionObserver analog) |
| `web/src/components/ui/checkbox.tsx` (NEW, `npx shadcn add checkbox`) | component (primitive) | request-response | `web/src/components/ui/switch.tsx` | exact (generated) |

---

## Pattern Assignments

### `internal/diff/parse.go` (model, transform) — add `Hash` field

**Analog:** itself. The `File` struct is the shape the hash serializes over and the JSON contract the client reads.

**Struct to extend** (parse.go:43-53) — add `Hash string \`json:"hash"\`` (the researcher recommends placing it after `Path`):
```go
type File struct {
	Path      string  `json:"path"`
	OldPath   *string `json:"oldPath"`
	Status    string  `json:"status"` // modified | new | deleted | renamed
	Binary    bool    `json:"binary"`
	Additions *int    `json:"additions"` // nil for binary
	Deletions *int    `json:"deletions"` // nil for binary
	Hunks     []Hunk  `json:"hunks"`
}
```

**Hunk / Line shape the hash walks** (parse.go:55-66) — ordered slices, deterministic (no maps):
```go
type Hunk struct {
	Header string `json:"header"`
	Lines  []Line `json:"lines"`
}
type Line struct {
	Kind string `json:"kind"` // context | add | del
	Text string `json:"text"`
}
```

> **Doc-comment discipline:** every struct/field in this file carries an intent comment (e.g. parse.go:43-44 "Additions/Deletions are nil for binary files…"). Add a matching one-line comment for `Hash` (what bytes it covers, what it excludes — Status/Binary/OldPath/Hunks in; Base/Path/counts out per RESEARCH §Area 1).

---

### `internal/diff/diff.go` (service, transform) — `hashFile()` + set `f.Hash` in `Compute`

**Analog:** itself — `countHunkLines` (diff.go:199-211) is the existing pure per-file helper over `[]Hunk`; `hashFile` is a sibling of the same shape. Hashing uses `crypto/sha256` + `encoding/hex` (Go stdlib, not yet imported here).

**Existing pure-helper precedent** (diff.go:197-211) — copy this shape (package-level func over `File`/`[]Hunk`, no I/O):
```go
// countHunkLines sums add/del lines across hunks — used for untracked files,
// which numstat never reports.
func countHunkLines(hunks []Hunk) (add, del int) {
	for _, h := range hunks {
		for _, ln := range h.Lines {
			switch ln.Kind {
			case "add":
				add++
			case "del":
				del++
			}
		}
	}
	return add, del
}
```

**Where to set `f.Hash`** — `Compute` builds `files` from two code paths (tracked at diff.go:113-140, untracked at diff.go:149-176) then sorts (diff.go:179). The single cleanest insertion point is **one final pass after the sort, before building `d`** (RESEARCH §Area 1 — one code path covers both tracked and untracked):
```go
	// Stable ordering: sort everything by path ascending (planner's pick).
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	// [NEW] one pass: f.Hash = hashFile(f) for i := range files
	d := &Diff{Files: files}
```

**New `hashFile` (RESEARCH §Area 1 recommended body — length-prefixed, deterministic):**
```go
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
		for _, ln := range hk.Lines { ws(ln.Kind); ws(ln.Text) }
	}
	return hex.EncodeToString(h.Sum(nil))
}
```

**Imports to add** (`fmt` already imported at diff.go:7; add `crypto/sha256`, `encoding/hex`, `io`). `Compute` stays DB-free — the handler layers Viewed on top.

**Test seam** (primary Go seam): extend `internal/diff/diff_test.go`. Its harness (diff_test.go:18-52) isolates git config in `TestMain` and provides `git()`/`write()`/`findFile()` fixture helpers over real repos in `t.TempDir()`. Assert: same content → same hash; edit → different hash; unrelated base commit not touching file F → F.Hash unchanged; rename vs modify with identical hunks → different hash; binary hash stability (documents Open Q1).

---

### `internal/api/diffs.go` (controller, request-response + CRUD)

**Analog for the read merge:** itself (`h.get`, diffs.go:34-110). **Analog for the new PUT write:** `internal/api/settings.go` `h.put` (settings.go:57-82) — the canonical decode → validate → persist → respond handler.

**Route registration** (diffs.go:20-23) — `diffHandlers` already holds `db` (diffs.go:25-28), so the write endpoint needs no new wiring:
```go
func DiffRoutes(mux *http.ServeMux, db *sql.DB, wt *worktree.Service) {
	h := &diffHandlers{db: db, wt: wt}
	mux.HandleFunc("GET /api/tasks/{id}/diff", h.get)
	// [NEW] mux.HandleFunc("PUT /api/tasks/{id}/diff/viewed", h.setViewed)
}
```

**Read merge point** — after `diff.Compute`, before `writeJSON` (diffs.go:103-109). This is where the viewed SELECT runs and sets `d.Files[i].Viewed`:
```go
	d, err := diff.Compute(r.Context(), path, base)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	d.Base = base // totals bar + empty state share the resolved base
	// [NEW] SELECT file_path,diff_hash FROM diff_viewed WHERE task_id=? ; drain+close;
	//       for i := range d.Files { d.Files[i].Viewed = set has (Path, Hash) }
	writeJSON(w, http.StatusOK, d)
```

**ID parse + 404 idiom** — reuse `pathID` (projects.go:794-802) exactly, and mirror the `sql.ErrNoRows → 404` shape already in `h.get` (diffs.go:35-54):
```go
func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}
```

**PUT handler shape** — copy `settingsHandlers.put` (settings.go:57-82): `pathID` → decode body → validate (400 on empty path/hash) → persist → respond:
```go
func (h *settingsHandlers) put(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if _, ok := settings.Defaults[key]; !ok {
		writeError(w, http.StatusNotFound, "unknown setting")
		return
	}
	var req struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	// ... validate, then write ...
	writeJSON(w, http.StatusOK, entryFor(key, req.Value))
}
```
For `setViewed`: decode `{ path, hash, viewed bool }`, reject empty `path`/`hash` with 400 (V5), then INSERT-or-DELETE, respond **`204 No Content`** (the client's `onSettled` invalidate re-reads authoritative state). Do NOT re-`Compute` to validate the hash (RESEARCH §Area 2 — trust the client hash; keep-history makes a stale row harmless).

**Response helpers** (respond.go:9-18) — always use these, never hand-roll JSON:
```go
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
```

**Test seam:** extend `internal/api/diffs_test.go`. `newDiffServer` (diffs_test.go:28-49) opens a real DB via `store.Open` (FK pragma on — Pitfall 6), runs `store.Migrate`, and wires `DiffRoutes` over httptest exactly as production. Assert insert-then-read marks viewed; delete unmarks; changed hash reads un-viewed while old-hash row lingers; revert restores; FK cascade on task delete.

---

### `internal/diffviewed/*.go` OR inline helpers (store, CRUD)

**Analogs:** `internal/settings/settings.go` (hand-written `database/sql`, upsert idiom, `sql.ErrNoRows` handling) and `internal/api/icons.go:184` (collect-then-write discipline under `SetMaxOpenConns(1)`, `?`-placeholders-only rule).

**Upsert idiom** (settings.go:87-102) — `INSERT … ON CONFLICT … DO …`, `?` placeholders. For the viewed table use `DO NOTHING` on the composite PK:
```go
func Set(db *sql.DB, key, value string) error {
	if _, ok := Defaults[key]; !ok {
		return fmt.Errorf("unknown setting %q", key)
	}
	if err := Validate(key, value); err != nil {
		return err
	}
	_, err := db.Exec(
		`INSERT INTO settings(key, value) VALUES(?, ?)
		 ON CONFLICT(key) DO UPDATE SET
		   value = excluded.value,
		   updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')`,
		key, value,
	)
	return err
}
```

**Read-into-set discipline** (`GetAll`, settings.go:62-82) — `Query` → `defer rows.Close()` → scan loop → `rows.Err()`. The viewed read builds a `set{(path,hash)}` and closes the cursor before any writes:
```go
func GetAll(db *sql.DB) (map[string]string, error) {
	all := make(map[string]string, len(Defaults))
	for k, v := range Defaults { all[k] = v }
	rows, err := db.Query(`SELECT key, value FROM settings`)
	if err != nil { return nil, err }
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil { return nil, err }
		if _, ok := all[k]; ok { all[k] = v }
	}
	return all, rows.Err()
}
```

**`SetMaxOpenConns(1)` cursor-during-write rule** (icons.go:178-216) — the authoritative statement of the deadlock discipline and the `?`-placeholder mandate (T-18-01). The viewed read is a single fully-drained SELECT closed before any `Exec`, so it is safe without batching; this comment is the reference for WHY:
```go
// Collect-then-update is REQUIRED: the store opens with db.SetMaxOpenConns(1)
// (single-writer discipline, store.go), so holding the SELECT cursor open while
// issuing UPDATEs on the same connection would deadlock. We scan every row that
// needs filling into a slice ... close the cursor, then run the parameterized
// UPDATEs. All SQL uses '?' placeholders only — name/id are NEVER
// string-concatenated into the query (T-18-01).
func BackfillProjectIcons(db *sql.DB) error {
	rows, err := db.Query(`SELECT id, name FROM projects WHERE icon_letters = '' OR icon_color = ''`)
	...
	defer rows.Close()
	...
	for _, f := range fills {
		if _, err := db.Exec(
			`UPDATE projects SET icon_letters = ?, icon_color = ? WHERE id = ?`,
			f.letters, f.color, f.id,
		); err != nil { return err }
	}
	return nil
}
```

**Three statements the store package needs** (RESEARCH §Area 2, `?` placeholders only):
```go
db.Query(`SELECT file_path, diff_hash FROM diff_viewed WHERE task_id = ?`, id)              // read
db.Exec(`INSERT INTO diff_viewed(task_id, file_path, diff_hash) VALUES(?,?,?)
         ON CONFLICT(task_id, file_path, diff_hash) DO NOTHING`, id, path, hash)            // mark
db.Exec(`DELETE FROM diff_viewed WHERE task_id=? AND file_path=? AND diff_hash=?`, id, path, hash) // unmark
```

> **No startup backfill** (unlike `BackfillProjectIcons` at main.go:124): an empty `diff_viewed` on first boot is correct. Do NOT add a backfill hook.

---

### `internal/store/migrations/00011_diff_viewed.sql` (migration, DDL)

**Analog:** `00005_tmux_sessions.sql` — CREATE TABLE with `REFERENCES tasks(id)`, `strftime` timestamp default, `UNIQUE(...)` composite, `-- +goose Up` / `Down`. FK-cascade precedent is `00001_init.sql:12` (`REFERENCES projects(id) ON DELETE CASCADE`). Reversible-Down copy discipline is documented in `00010_muted_palette.sql:19-28` (a `SELECT 1;` no-op when Down isn't safely reversible).

**Structure to copy** (00005_tmux_sessions.sql:1-17):
```sql
-- +goose Up
-- tmux session identity ... (intent comment block)
CREATE TABLE tmux_sessions (
    id         INTEGER PRIMARY KEY,
    task_id    INTEGER NOT NULL REFERENCES tasks(id),
    n          INTEGER NOT NULL,
    name       TEXT    NOT NULL UNIQUE,
    label      TEXT    NOT NULL DEFAULT '',
    created_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    UNIQUE(task_id, n)
);
-- +goose Down
DROP TABLE tmux_sessions;
```

**Recommended `00011` body** (RESEARCH §Area 2 — composite PK doubles as the index; add `ON DELETE CASCADE` per 00001_init.sql:12):
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

**Runner** — no new wiring; `store.Migrate` (migrate.go:15-21) auto-applies embedded `migrations/*.sql` at startup (main.go:98). Add a top-of-file intent comment block like 00005/00010.

---

### `web/src/api/diffs.ts` (hook + model)

**Analogs:** itself (`useTaskDiff`, diffs.ts:45-50, `queryKey ["diff", taskId]`) for the query + type; `useMoveTask` (mutations.ts:148-168) for the optimistic mutation; `put` (client.ts:55-60) for the HTTP verb.

**Type to extend** (diffs.ts:16-24) — add `hash: string;` and `viewed: boolean;` to `DiffFile`:
```ts
export interface DiffFile {
  path: string;
  oldPath: string | null;
  status: "modified" | "new" | "deleted" | "renamed";
  binary: boolean;
  additions: number | null; // null for binary
  deletions: number | null;
  hunks: DiffHunk[];
  // [NEW] hash: string; viewed: boolean;
}
```

**Query key already established** (diffs.ts:45-50) — the mutation invalidates the same key:
```ts
export function useTaskDiff(taskId: number) {
  return useQuery<DiffResponse, ApiError>({
    queryKey: ["diff", taskId],
    queryFn: () => get<DiffResponse>(`/api/tasks/${taskId}/diff`),
  });
}
```

**Optimistic mutation to mirror** (`useMoveTask`, mutations.ts:148-168) — `onMutate` cancel+snapshot+`setQueryData`, `onError` rollback, `onSettled` invalidate:
```ts
export function useMoveTask(projectId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, status, afterId }: MoveArgs) =>
      post<Task>(`/api/tasks/${id}/move`, { status, after_id: afterId }),
    onMutate: async (args) => {
      await queryClient.cancelQueries({ queryKey: ["tasks", projectId] });
      const prev = queryClient.getQueryData<Task[]>(["tasks", projectId]);
      queryClient.setQueryData<Task[]>(["tasks", projectId], (old) => applyMove(old, args));
      return { prev };
    },
    onError: (_error, _args, context) => {
      queryClient.setQueryData(["tasks", projectId], context?.prev);
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: ["tasks", projectId] });
    },
  });
}
```

**`put` helper** (client.ts:55-60) — the verb for the toggle write; `api()` (client.ts:11-35) already returns `undefined` for 204 (client.ts:30-32), which matches the `204 No Content` backend response:
```ts
export function put<T>(path: string, body?: unknown): Promise<T> {
  return api<T>(path, {
    method: "PUT",
    body: body === undefined ? undefined : JSON.stringify(body),
  });
}
```

**Target `useToggleViewed`** (RESEARCH §Area 5) — `mutationFn: put(\`/api/tasks/${taskId}/diff/viewed\`, {path, hash, viewed})`, optimistic `setQueryData(["diff", taskId], d => map files → set viewed on match)`, invalidate `["diff", taskId]` on settle.

---

### `web/src/components/task/DiffTab.tsx` (component, event-driven + request-response)

**Analog:** itself — restructure the single scroll container (DiffTab.tsx:84) into a two-pane `flex h-full min-h-0` row (left `FileTree` `w-72 shrink-0 border-r overflow-y-auto`; right pane = the existing scroll container as the IO root + scroll target). Preserve loading/empty/error/refetch states verbatim (UI-SPEC).

**States to preserve unchanged** (DiffTab.tsx:41-77) — error card, 150ms-delayed loading, empty state, `files.length === 0` guard. Note the sticky totals bar to keep (`TOTALS_BAR_PX` reference, DiffTab.tsx:85):
```tsx
    <div className="flex h-full min-h-0 flex-col overflow-y-auto p-4">
      <div className="sticky top-0 z-10 flex items-center border-b border-border bg-background px-3 py-2">
        <p className="flex-1 text-sm text-zinc-50">
          {fileCount}
          {totals.additions > 0 && (<>{", "}<span className="text-green-400 tabular-nums">{`+${totals.additions}`}</span></>)}
          {totals.deletions > 0 && (<>{" "}<span className="text-red-400 tabular-nums">{`−${totals.deletions}`}</span></>)}
          {" vs "}<span className="font-mono">{base}</span>
        </p>
        {/* Refresh button (Tooltip + ghost icon) stays; PanelLeft toggle sits beside it */}
```

**Section render loop** (DiffTab.tsx:125-128) — change the `key` from `file.path` to `` `${file.path}:${file.hash}` `` so a changed file (new hash) remounts un-viewed + expanded (RESEARCH §Area 5 / DIFF-04), and pass `onToggleViewed` + scroll refs:
```tsx
      <div className="flex flex-col gap-2 pt-2">
        {files.map((file) => (
          <DiffFileSection key={file.path} file={file} />
        ))}
      </div>
```

**Effect-with-cleanup precedent** — DiffTab already uses `useEffect` with a `window.setTimeout` + cleanup (DiffTab.tsx:32-39); the IntersectionObserver effect follows the same mount/cleanup discipline (`observer.disconnect()` in cleanup, depend on the joined path-list string — RESEARCH §Area 3, Pitfall 4).

---

### `web/src/components/task/DiffFileSection.tsx` (component, request-response)

**Analogs:** itself (header row is the `CollapsibleTrigger` today, DiffFileSection.tsx:54-71) + `TaskTabs.tsx:101-129` for the **sibling-not-child** interactive control precedent (avoids invalid button-in-button).

**Current uncontrolled trigger** (DiffFileSection.tsx:54-71) — restructure: trigger shrinks to `flex-1` inside a flex `<div>`, checkbox becomes a sibling; promote `defaultOpen` to controlled `open`/`onOpenChange`; add `data-diff-path={file.path}` + `scroll-mt-[TOTALS_BAR_PX]` to the `Collapsible` root; add sticky header `top` offset:
```tsx
    <Collapsible
      defaultOpen={changedLines <= 400}
      className="group/diff-file overflow-hidden rounded-lg border border-border"
    >
      <CollapsibleTrigger className="flex w-full items-center gap-1 bg-card px-3 py-2 outline-none hover:bg-[#27272a] focus-visible:ring-2 focus-visible:ring-blue-500">
        <ChevronRight className="size-4 shrink-0 transition-transform group-data-[state=open]/diff-file:rotate-90" />
        <span className="flex-1 truncate text-left font-mono text-sm">{pathLabel}</span>
        {statusSuffix && (<span className="text-xs text-muted-foreground">{statusSuffix}</span>)}
        <span className="flex items-center gap-1 text-xs font-medium tabular-nums">
          <span className="text-green-400">{`+${file.additions ?? 0}`}</span>
          <span className="text-red-400">{`−${file.deletions ?? 0}`}</span>
        </span>
      </CollapsibleTrigger>
      <CollapsibleContent>...</CollapsibleContent>
    </Collapsible>
```

**Binary branch** (DiffFileSection.tsx:33-52) — also gets the checkbox sibling (D-04); collapse-on-view is a no-op there (already non-collapsible). Dim `pathLabel`/stats to `text-muted-foreground` when `file.viewed`.

**Sibling-not-child precedent** (TaskTabs.tsx:104-125) — the exact structure the Viewed checkbox must follow (`stopPropagation` so toggling doesn't fire the collapse trigger; established `hover:bg-[#27272a]` + `focus-visible:ring-blue-500` literals):
```tsx
                  {/* span[role=button], NOT <button> — button-in-button is
                      invalid HTML inside the TabsTrigger. */}
                  <span
                    role="button"
                    tabIndex={0}
                    aria-label={`Stop ${tab.label} and close tab`}
                    className="ml-1 rounded-sm p-0.5 text-zinc-400 outline-none hover:bg-[#27272a] hover:text-zinc-50 focus-visible:ring-2 focus-visible:ring-blue-500"
                    onClick={(e) => {
                      e.stopPropagation();
                      tab.onClose?.();
                    }}
                    onKeyDown={(e) => {
                      if (e.key === "Enter" || e.key === " ") {
                        e.preventDefault();
                        e.stopPropagation();
                        tab.onClose?.();
                      }
                    }}
                  >
                    <X className="size-3" />
                  </span>
```

**Controlled collapse ↔ Viewed coupling** (RESEARCH §Area 5): `const [open, setOpen] = useState(!file.viewed && changedLines <= 400)`; mark viewed → `onToggleViewed(...true)` + `setOpen(false)`; uncheck → `onToggleViewed(...false)` + `setOpen(true)`; trigger `onOpenChange={setOpen}` keeps manual collapse independent. The `key={path:hash}` remount (in DiffTab) handles auto-reset re-seeding.

---

### `web/src/components/task/FileTree.tsx` (NEW — component + utility, transform)

**Match quality: partial.** No recursive-tree component exists in the codebase (verified: no `buildTree`/`TreeNode`/recursive render anywhere; `sidebar.tsx` is a shadcn primitive, not a data tree). Assemble from two in-tree patterns:

**Pure derive-from-cache util precedent** (`applyMove`, mutations.ts:123-146) — the model for a pure `buildTree(files)` in a `useMemo` (immutable transform over the query data, no side effects):
```ts
function applyMove(
  tasks: Task[] | undefined,
  { id, status, afterId }: MoveArgs,
): Task[] | undefined {
  if (!tasks) return tasks;
  const moved = tasks.find((t) => t.id === id);
  if (!moved) return tasks;
  const column = tasks
    .filter((t) => t.status === status && t.id !== id)
    .sort((a, b) => a.position - b.position);
  ...
}
```

**Row visual language** — mirror the `DiffFileSection` header row exactly (DiffFileSection.tsx:59-70): `font-mono text-sm truncate` filename, `text-xs tabular-nums` green/red `+add −del` counts, `−` = U+2212, `hover:bg-[#27272a]`, `focus-visible:ring-blue-500`. Selected row = `bg-accent` + 2px `blue-500` left border; viewed row dims to `muted-foreground` + a `Check` glyph (`size-3`). Dir rows use `ChevronRight` rotate-on-open (same icon convention as DiffFileSection.tsx:60).

**Algorithm** (RESEARCH §Area 4): split `file.path` on `/`; path-compress single-child dir chains GitHub-style; folders default-expanded; dirs-before-files alphabetical. Click = scroll-only (`scrollRef.current.querySelector('[data-diff-path=...]').scrollIntoView`), never mutates collapse/Viewed.

---

### `web/src/hooks/use-scroll-spy.ts` (NEW — hook, event-driven)

**Match quality: partial.** No IntersectionObserver usage exists in the codebase. The closest analog is `web/src/hooks/use-mobile.ts` — a browser-DOM-subscription hook with the exact naming (`use-*.ts` kebab, `use*` export), `useState` + `useEffect(subscribe → cleanup)` shape:
```ts
export function useIsMobile() {
  const [isMobile, setIsMobile] = React.useState<boolean | undefined>(undefined)
  React.useEffect(() => {
    const mql = window.matchMedia(`(max-width: ${MOBILE_BREAKPOINT - 1}px)`)
    const onChange = () => { setIsMobile(window.innerWidth < MOBILE_BREAKPOINT) }
    mql.addEventListener("change", onChange)
    setIsMobile(window.innerWidth < MOBILE_BREAKPOINT)
    return () => mql.removeEventListener("change", onChange)
  }, [])
  return !!isMobile
}
```

Adapt: create the `IntersectionObserver` in `useEffect` with `root: scrollRef.current`, `rootMargin` pushing the band below the sticky totals bar, `threshold: 0`; observe `[data-diff-path]` wrappers (NOT the sticky header — Pitfall 3/§Area 3); `disconnect()` on cleanup; depend on the joined path-list string; return `activePath`. (The hook may also live inline in DiffTab per RESEARCH §Area 3 — either matches house style; a `use-scroll-spy.ts` file matches the `hooks/` convention.)

---

### `web/src/components/ui/checkbox.tsx` (NEW — primitive, generated)

**Analog:** `web/src/components/ui/switch.tsx` — the exact unified `radix-ui` import convention the generated `checkbox.tsx` will match:
```tsx
import * as React from "react"
import { Switch as SwitchPrimitive } from "radix-ui"
import { cn } from "@/lib/utils"

function Switch({ className, ...props }: React.ComponentProps<typeof SwitchPrimitive.Root>) {
  return (
    <SwitchPrimitive.Root
      data-slot="switch"
      className={cn("... data-[state=checked]:bg-primary ...", className)}
      {...props}
    >
      <SwitchPrimitive.Thumb data-slot="switch-thumb" className={cn("...")} />
    </SwitchPrimitive.Root>
  )
}
export { Switch }
```

Generate with `cd web && npx shadcn add checkbox` (no new npm dep — `radix-ui@1.5.0` already installed, exports `Checkbox.Root`/`Indicator`). Then override the fill to `blue-500` at the call site (UI-SPEC Decision 2 — the default `primary` fill is near-white in dark): `className="data-[state=checked]:border-blue-500 data-[state=checked]:bg-blue-500 data-[state=checked]:text-white"`.

---

## Shared Patterns

### SQL discipline (all backend store/handler files)
**Source:** `internal/api/icons.go:178-216` + `internal/store/store.go:12-25`
**Apply to:** the new viewed-state store package + the diff handler read/write.
```go
db.SetMaxOpenConns(1) // single-writer discipline (store.go:23)
// icons.go:182-183 — the T-18-01 rule:
// All SQL uses '?' placeholders only — never string-concatenated (SQL injection = Tampering).
// Never hold a SELECT cursor open while issuing a write on the same connection.
```
The FK pragma is set in the DSN (`store.go:16` `_pragma=foreign_keys(1)`), so `ON DELETE CASCADE` fires in production — but tests must open via `store.Open` (diffs_test.go:31) or the cascade silently no-ops (Pitfall 6).

### HTTP handler skeleton (backend controllers)
**Source:** `internal/api/settings.go:57-82` + `internal/api/respond.go` + `internal/api/projects.go:794`
**Apply to:** the new `PUT /api/tasks/{id}/diff/viewed`.
Pattern: `pathID(w,r)` → `sql.ErrNoRows → writeError 404` → `json.NewDecoder(r.Body).Decode` (400 on bad JSON) → validate (400 on empty path/hash) → persist → `writeJSON`/`204`. Errors always relay through `writeError` (`{"error": msg}`), which the client surfaces as `error.message` (client.ts:22-27).

### Optimistic TanStack mutation (frontend writes)
**Source:** `web/src/api/mutations.ts:148-168` (`useMoveTask`)
**Apply to:** `useToggleViewed`.
`onMutate`: cancel queries + snapshot + `setQueryData`; `onError`: restore snapshot; `onSettled`: `invalidateQueries`. Same `queryKey` as the read (`["diff", taskId]`, diffs.ts:47).

### Sibling-not-child interactive control (frontend components)
**Source:** `web/src/components/task/TaskTabs.tsx:104-125`
**Apply to:** the Viewed checkbox in `DiffFileSection` (both text and binary branches).
A focusable control that lives inside another trigger must be a **sibling** with `e.stopPropagation()`, never nested (invalid button-in-button). Established interaction literals: `hover:bg-[#27272a]`, `focus-visible:ring-2 focus-visible:ring-blue-500`.

### Style / copy literals (frontend)
**Source:** `DiffFileSection.tsx` / `DiffTab.tsx` / `TaskTabs.tsx`
**Apply to:** tree rows, checkbox label, sticky headers.
`−` = U+2212 (never hyphen) for deletions; `tabular-nums` on all counts; `text-green-400` / `text-red-400` for ±; `font-mono text-sm` for paths; `text-xs text-muted-foreground` for labels/status; `blue-500` reserved for active/selected/focus/checked only (UI-SPEC Color).

---

## No Analog Found

Files with no close in-repo analog (planner should lean on RESEARCH.md §Area 3/§Area 4 patterns, not a copied file):

| File | Role | Data Flow | Reason |
|------|------|-----------|--------|
| `web/src/components/task/FileTree.tsx` | component + utility | transform | No recursive data-tree component exists (verified: no `buildTree`/`TreeNode`/recursive render). Assemble from `applyMove` (pure transform) + DiffFileSection row styling. RESEARCH §Area 4 has the full algorithm. |
| `web/src/hooks/use-scroll-spy.ts` | hook | event-driven | No `IntersectionObserver` usage anywhere in the tree. Closest is `use-mobile.ts` (subscribe/cleanup DOM hook shape + naming). RESEARCH §Area 3 has the verified observer config. |

---

## Metadata

**Analog search scope:** `internal/diff/`, `internal/api/`, `internal/store/` (+ `migrations/`), `internal/settings/`, `cmd/kamacu/main.go`, `web/src/api/`, `web/src/components/task/`, `web/src/components/ui/`, `web/src/hooks/`.
**Files scanned/read:** ~22 (all cited above with file:line).
**Project skills:** none present (`.claude/skills/` and `.agents/skills/` absent).
**Open item to flag in planning:** binary-file Viewed-reset semantics (RESEARCH Open Q1) — a rendered-only hash cannot detect binary byte changes because `parse.go` does not capture the `index <oid>..<oid>` line; default to strict D-01 (rendered-only) and confirm before locking.
**Pattern extraction date:** 2026-07-02
</content>
</invoke>

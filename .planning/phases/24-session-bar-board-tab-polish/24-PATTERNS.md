# Phase 24: Session-Bar, Board & Tab Polish - Pattern Map

**Mapped:** 2026-07-03
**Files analyzed:** 11 (9 modified, 1 removed, 1 new)
**Analogs found:** 11 / 11 (every surface reuses an existing pattern — this phase is deliberately pattern-reuse)

> **All analogs are in-repo.** This phase adds no new architectural vocabulary; every
> item maps to code already living in the same file it will change (inline editor,
> ⋯ menu, bracketed paste, collapse helper) or a sibling handler (PATCH endpoint,
> mutable-field-under-mutex). Excerpts below are the exact code to copy.

---

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `web/src/components/task/TaskTabs.tsx` | component | event-driven (UI) | task-title inline editor in `web/src/pages/TaskPage.tsx` (self) | exact — copy the click→`<Input autoFocus>` contract |
| `web/src/pages/TaskPage.tsx` (rename wiring) | component | event-driven (UI) | its own `commitTitle`/`titleDraft` block | exact (same file, same pattern) |
| `web/src/components/task/AgentTab.tsx` | component | event-driven (UI) | its own `insertAction` + `pasteApiRef` + task-actions ⋯ menu in `TaskPage.tsx` | exact — fold header actions into a ⋯ menu, delete auto-paste |
| `web/src/components/terminal/TerminalPane.tsx` | component | request-response (WS) | its own `showStop` header slot + `headerActions` prop | role-match — add an agent-only `headerMenu` prop that supersedes `showStop` |
| `web/src/api/sessions.ts` | service (API client) | CRUD | `useStopSession` mutation (self) + `useUpdateTask` PATCH mutation | exact — copy the mutation + invalidate shape |
| `web/src/components/layout/ActiveSessionsBar.tsx` | component | event-driven (UI) | its own `collapse()` helper + `kamacu:sessions-bar-collapsed` key | exact (same file) |
| `web/src/components/board/Column.tsx` | component | event-driven (UI) | trivial deletion of the `status === "todo"` `<QuickAdd>` slot | exact |
| `web/src/components/board/QuickAdd.tsx` | component | — | file removed (D-12) | n/a — retire; creation survives via `BoardPage.tsx` |
| `internal/api/sessions.go` (rename handler + label fix) | controller (HTTP) | CRUD | `PATCH /api/tasks/{id}` `update` in `internal/api/tasks.go` | exact — copy the partial-update `*string` decode + `writeError` posture |
| `internal/session/session.go` (`SetLabel`) | model | transform | `SetWaiting`/`SetIdle` mutex-guarded field setters (self) | exact — add a mutex-guarded `SetLabel` mirroring `SetWaiting` |
| `internal/api/routes.go` **or** `sessions.go` route table | route | — | `SessionRoutes` `mux.HandleFunc` block (self) + `PATCH /api/tasks/{id}` registration | exact — register `PATCH /api/sessions/{id}` alongside the existing session routes |

> **New file:** none strictly required. A rename mutation lands *inside* the existing
> `web/src/api/sessions.ts`; the rename handler lands *inside* `internal/api/sessions.go`.

---

## Pattern Assignments

### `web/src/components/task/TaskTabs.tsx` — tab rename inline editor (D-01/D-02/D-05)

**Analog:** the task-title inline-edit contract in `web/src/pages/TaskPage.tsx` (lines 488–513).

**Copy this contract verbatim** (click/double-click → `<Input autoFocus>`, Enter commits, Esc cancels, blur commits) — `TaskPage.tsx:488-513`:

```tsx
) : titleDraft === null ? (
  <button
    type="button"
    className="min-w-0 flex-1 truncate rounded-md px-1 py-0.5 text-left text-base font-medium hover:bg-muted/50"
    title="Edit title"
    onClick={() => setTitleDraft(task.title)}
  >
    {task.title}
  </button>
) : (
  <Input
    autoFocus
    value={titleDraft}
    className="h-8 flex-1 text-base font-medium"
    onChange={(e) => setTitleDraft(e.target.value)}
    onBlur={(e) => commitTitle(e.currentTarget.value)}
    onKeyDown={(e) => {
      if (e.key === "Enter") {
        e.currentTarget.blur();
      } else if (e.key === "Escape") {
        cancelTitleEditRef.current = true;
        e.currentTarget.blur();
      }
    }}
  />
)
```

**Esc-cancels-without-committing idiom** — a `cancelRef` set on Escape, read-and-cleared in the commit path, so the blur that Escape triggers does NOT save (`TaskPage.tsx:112` + `281-290`):

```tsx
const cancelTitleEditRef = useRef(false);
// …
function commitTitle(value: string) {
  const cancelled = cancelTitleEditRef.current;
  cancelTitleEditRef.current = false;
  setTitleDraft(null);
  if (cancelled || !task) return;
  const trimmed = value.trim();
  if (!trimmed || trimmed === task.title) return;   // trimmed-empty reverts without saving
  updateTask.mutate({ id: task.id, title: trimmed });
}
```

**D-05 divergence (do NOT copy the title's "empty reverts silently" branch):** the tab
rename treats a trimmed-empty commit as an explicit reset-to-`Bash N`, so instead of the
title's `if (!trimmed) return;` no-op, send an empty/reset intent to the rename mutation
(the server re-derives the `Bash N` default — see the backend section).

**Where it lands:** `TabDef` (lines 20–35) gains an optional rename affordance. Today the
trigger renders `{tab.leading}{tab.label}{tab.onClose && <X/>}` (lines 92–129). Add a
`renamable`/`onRename` field (only set for bash tabs — D-02 keeps Agent/Description/Diff
fixed) and swap the `<span>{tab.label}</span>` (line 95–99) for the editor on double-click.
Note the existing `× ` uses `span[role="button"]` **not** `<button>` (line 107) — the inline
`<Input>` must be careful about the same button-in-button constraint inside `TabsTrigger`;
single-click must still select the tab (Radix `TabsTrigger` `onClick`), so gate the editor
on `onDoubleClick` and `stopPropagation` as the × does (line 112).

---

### `web/src/pages/TaskPage.tsx` — build bash `TabDef`s from `s.label` (D-03) + agent seed gating (D-08)

**Analog:** self — the `visibleSessions.map` that builds bash `TabDef`s (lines 384–416) and the PR-seed derivation (lines 265–279).

Bash tabs already take their label from the server session (`label: s.label`, line 388). The
rename affordance is added here by passing the per-tab `onRename` into `TabDef`, wired to the
new sessions-rename mutation (see `web/src/api/sessions.ts`).

**D-08 seed gating (unchanged derivation, reused for the menu-item visibility)** — `TaskPage.tsx:265-279`:

```tsx
const isPR = task.source === "github_pr";
const prTitle = prDetail?.title ?? task.title;
const seedTemplate = settings?.pr_review_seed?.value ?? "";
const seed =
  isPR && seedTemplate.trim() !== ""
    ? seedTemplate
        .replaceAll("<n>", String(task.pr_number ?? ""))
        .replaceAll("<title>", prTitle)
    : undefined;
```

`seed` is already threaded into `<AgentTab … seed={seed} />` (line 350). D-08 says the
`Insert review prompt` menu item shows only when `seed` is defined AND the agent is running —
so the existing `seed !== undefined` gate is exactly the derivation to reuse; the menu item's
visibility check is `seed && agentSession?.status === "running"`.

**D-06 ⋯ menu shape to mirror** — the task-actions `DropdownMenu` in `TaskPage.tsx:520-546`:

```tsx
<DropdownMenu>
  <DropdownMenuTrigger asChild>
    <Button variant="ghost" size="icon-sm" aria-label="Task actions">
      <Ellipsis className="size-4" />
    </Button>
  </DropdownMenuTrigger>
  <DropdownMenuContent align="end">
    {task.worktree_path && (
      <>
        <DropdownMenuItem onSelect={() => setCleanupOpen(true)}>
          Clean up worktree
        </DropdownMenuItem>
        <DropdownMenuSeparator />
      </>
    )}
    <DropdownMenuItem variant="destructive" onSelect={() => setDeleteOpen(true)}>
      Delete task
    </DropdownMenuItem>
  </DropdownMenuContent>
</DropdownMenu>
```

Note the conditional-item + `<DropdownMenuSeparator/>` pattern and `align="end"` — the agent
menu (D-06) reuses this exact structure: `Insert description` (conditional), `Insert review
prompt` (conditional), separator, then `Stop`. Imports already present at `TaskPage.tsx:4`
(`Ellipsis`) and `22-28` (the dropdown-menu barrel).

---

### `web/src/components/task/AgentTab.tsx` — fold header actions into a ⋯ menu, delete auto-paste (D-06/D-09)

**Analog:** self — `insertAction` (lines 135–151), `pasteApiRef` (line 47), `handleConnect` (156–177), and the `TaskPage.tsx` ⋯ menu (above).

**KEEP the paste mechanism** — `pasteApiRef.current?.paste(...)` is bracketed-paste,
never-sent (lines 47 + 142 + 175):

```tsx
const pasteApiRef = useRef<{ paste: (t: string) => void } | null>(null);
// …
onClick={() => pasteApiRef.current?.paste(task.description)}   // Insert description
// …
pasteApiRef.current?.paste(seed);                              // review seed (moves to a menu item)
```

**REMOVE the auto-paste (D-09)** — delete the module-scope guard and the connect-time seed
paste. The lines to delete are `AgentTab.tsx:19-21`:

```tsx
// Agent session ids whose PR-review seed has already been injected. Module
// scope (not a ref) …
const seededSessionIds = new Set<string>();
```

…and the seed block inside `handleConnect`, `AgentTab.tsx:165-176`:

```tsx
// PR-review seed (D-06/D-07): prefill once per agent session after connect.
// …
if (seed && agentSession && !seededSessionIds.has(agentSession.id)) {
  seededSessionIds.add(agentSession.id);
  pasteApiRef.current?.paste(seed);
}
```

`handleConnect` keeps ONLY the D-45 optimistic waiting→idle clear (lines 157–163). After the
removal, the seed fires **only** from the new `Insert review prompt` menu item's
`onSelect={() => pasteApiRef.current?.paste(seed)}`.

**MOVE `Insert description` into the menu (D-06):** today it is a standalone `headerActions`
button (`insertAction`, lines 135–151, passed at line 227 `headerActions={insertAction}`).
Replace `headerActions` with a `headerMenu` (see TerminalPane) whose items are, in order:

```tsx
// Insert description — only when task.description !== "" && running (existing gate, line 136)
// Insert review prompt — only when seed && agentSession.status === "running" (D-08)
// <separator>
// Stop — calls the same stop mechanism the pane's showStop button used
```

The Stop item must call `useStopSession().mutate(agentSession.id)` (see the pane's `handleStop`,
`TerminalPane.tsx:203-207`) because D-07 removes the pane's inline Stop button for the agent
pane only.

---

### `web/src/components/terminal/TerminalPane.tsx` — agent-only header menu supersedes Stop (D-07)

**Analog:** self — the header slot (lines 303–325), the `showStop` gate (line 210), and the existing `headerActions` prop pass-through (line 24 + 312).

**Current header right group** — `TerminalPane.tsx:303-325`:

```tsx
<div className="flex h-9 shrink-0 items-center gap-2 border-b border-border px-3">
  <span className="text-sm font-medium">{label}</span>
  <div className="ml-auto flex items-center gap-2">
    {headerStatus && (
      <span className="text-xs text-muted-foreground">{headerStatus}</span>
    )}
    {headerActions}
    {showStop && (
      <Button
        variant="ghost" size="sm" disabled={stopping} onClick={handleStop}
        className="text-red-500 hover:text-red-500"
      >
        {stopping ? "Stopping…" : "Stop"}
      </Button>
    )}
  </div>
</div>
```

`showStop = status === "running" && !exited && conn.kind !== "not-found"` (line 210). D-07's
mechanism: add an optional `headerMenu?: ReactNode` prop (mirroring `headerActions?: ReactNode`,
line 24). When `headerMenu` is provided (agent pane), render it **instead of** the `showStop`
Stop button; bash panes pass no `headerMenu`, so their inline Stop is untouched. Keep the
`handleStop`/`stopping` logic in the pane so the menu's Stop item can reuse it, OR let the
agent menu own its own stop mutation (planner's call — D-07 explicitly leaves the mechanism to
the planner). The `Stop` in `AgentTab`'s menu is the cleanest fit since `AgentTab` already holds
`agentSession.id`.

---

### `web/src/api/sessions.ts` — rename mutation (Claude's Discretion API shape)

**Analog:** self — `useStopSession` (lines 125–133) for the mutation+invalidate shape, and `useUpdateTask` (`web/src/api/mutations.ts`, PATCH) for the PATCH verb.

**Copy `useStopSession`'s shape** (`sessions.ts:125-133`), swapping `post`→a PATCH helper and
adding the `{ label }` body:

```tsx
export function useStopSession() {
  const queryClient = useQueryClient();
  return useMutation<void, ApiError, string>({
    mutationFn: (id: string) => post<void>(`/api/sessions/${id}/stop`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["sessions"] });
    },
  });
}
```

New mutation (recommended shape — `PATCH /api/sessions/{id}` with `{ label }`, returning the
updated `TermSession`, then optimistic cache write like `useSpawnSession` at lines 48–54):

```tsx
export function useRenameSession(taskId: number) {
  const queryClient = useQueryClient();
  return useMutation<TermSession, ApiError, { id: string; label: string }>({
    mutationFn: ({ id, label }) => patch<TermSession>(`/api/sessions/${id}`, { label }),
    onSuccess: (session) => {
      queryClient.setQueryData<TermSession[]>(["sessions", taskId], (old) =>
        old ? old.map((s) => (s.id === session.id ? session : s)) : old,
      );
      queryClient.invalidateQueries({ queryKey: ["sessions"] });
    },
  });
}
```

> **Check `./client`:** `sessions.ts:2` imports `del, get, post`. There is no `patch` export
> yet — the planner must add a `patch` helper to `web/src/api/client.ts` (mirroring the
> existing `post`) or route the rename through a `post` to `/api/sessions/{id}/rename`. The
> label field already exists on `TermSession` (`sessions.ts:6`), so no type change is needed.

---

### `web/src/components/layout/ActiveSessionsBar.tsx` — drop total (D-10) + outside-click auto-collapse (D-11)

**Analog:** self — the `collapse()` helper (lines 46–49) and the collapsed-bar counts (lines 135–147).

**D-10 — drop the total.** Remove the `total` span from the collapsed bar; keep the three
`CountGroup`s. `ActiveSessionsBar.tsx:135-147`:

```tsx
<>
  <CountGroup status="working" count={working} />
  <CountGroup status="waiting" count={waiting} />
  <CountGroup status="idle" count={idle} />
  {/* DELETE the total span below (D-10): */}
  <span className="text-xs font-medium text-muted-foreground tabular-nums">
    {total}
  </span>
</>
```

`total` is still used by the empty-state check (`total === 0`, lines 91 + 130) and the expanded
list — keep the variable (line 66), only remove the collapsed-bar span. The per-state colored
counts (working/waiting/idle, waiting amber-emphasized) stay (`CountGroup`, lines 177–202).

**D-11 — outside-click auto-collapse.** Reuse the exact `collapse()` helper + global key,
`ActiveSessionsBar.tsx:33 + 46-49`:

```tsx
const storageKey = "kamacu:sessions-bar-collapsed";
// …
const collapse = () => {
  setCollapsed(true);
  localStorage.setItem(storageKey, "1");   // persists "1", consistent with row-open (D-11)
};
```

Add a ref on the outer `fixed inset-x-0 bottom-0` wrapper (line 86) and a document
`mousedown`/`pointerdown` listener that calls `collapse()` when the target is outside the ref
AND the bar is expanded. Follow the `TaskPage.tsx` document-listener idiom (add-on-mount,
remove-on-cleanup, `useEffect`) — `TaskPage.tsx:215-231`:

```tsx
useEffect(() => {
  function onKeyDown(e: KeyboardEvent) { /* … */ }
  window.addEventListener("keydown", onKeyDown);
  return () => window.removeEventListener("keydown", onKeyDown);
}, [navigate, projectId]);
```

Clicking the bar itself still toggles (`onClick={toggle}`, line 122) — the listener must skip
when the click is inside the ref, so the toggle is not double-fired into a re-expand. (Whether
Escape also collapses, and capture-phase details, are explicitly Claude's Discretion in D-11.)

---

### `web/src/components/board/Column.tsx` + `web/src/components/board/QuickAdd.tsx` — remove To Do quick-add (D-12)

**Analog:** trivial deletion. `Column.tsx:46`:

```tsx
{status === "todo" && <QuickAdd projectId={projectId} />}   // DELETE this line
```

Also remove the `import { QuickAdd } from "./QuickAdd";` (line 9) and the doc-comment reference
to To Do's quick-add on the `topSlot` prop (line 16). Then delete
`web/src/components/board/QuickAdd.tsx` entirely.

**Safety net (confirms D-12 is safe):** task creation survives via `BoardPage.tsx` — the header
`New task` button (`BoardPage.tsx:52` `<Button onClick={() => setDialogOpen(true)}>New task</Button>`)
and the `n` shortcut (`BoardPage.tsx:23-43`), both opening `NewTaskDialog`. Neither depends on
`QuickAdd`. Grep for other `QuickAdd` importers before deleting (only `Column.tsx` imports it).

---

### `internal/api/sessions.go` — session-label rename handler + D-04 label fix (Claude's Discretion backend)

**Analog:** the `PATCH /api/tasks/{id}` `update` handler in `internal/api/tasks.go:294-341`.

**Copy the partial-update decode + `writeError` posture** — `tasks.go:294-340`:

```go
func (h *taskHandlers) update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		Title       *string `json:"title"`
		Description *string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	// … build `sets`/`args`, TrimSpace, empty-title guard, then UPDATE … RETURNING
}
```

For the session rename: decode `{ "label": *string }`, `strings.TrimSpace` it, resolve the
session via `h.mgr.Get(r.PathValue("id"))` (mirroring `stop`, `sessions.go:355-363`), set the
in-memory label (see `session.go` `SetLabel` below), and — for tmux tabs — write the row:

```go
if sess.TmuxName() != "" {
	if _, err := h.db.Exec(`UPDATE tmux_sessions SET label = ? WHERE name = ?`,
		newLabel, sess.TmuxName()); err != nil {
		slog.Warn("persisting tmux session label", "name", sess.TmuxName(), "error", err)
	}
}
writeJSON(w, http.StatusOK, sess.Info())
```

This is the **exact same `UPDATE tmux_sessions SET label = ?` write already in the spawn
handler** (`sessions.go:343-347`):

```go
if opts.TmuxName != "" {
	if _, err := h.db.Exec(`UPDATE tmux_sessions SET label = ? WHERE name = ?`, sess.Info().Label, opts.TmuxName); err != nil {
		slog.Warn("persisting tmux session label", "name", opts.TmuxName, "error", err)
	}
}
```

**D-04 empty-commit → `Bash N` reset:** when the trimmed label is empty, the server must
re-derive the default. The default `Bash N` is machine-derivable from the tmux name
`kamacu-<task>-<n>` (parse the trailing `<n>`) — this is the "re-derive from tmux name"
option the CONTEXT calls out (D-04 alternative). Store the re-derived `Bash N` back into
`tmux_sessions.label` and set it on the in-memory session.

> **⚠️ VERIFY THE D-04 PREMISE — the CONTEXT is likely outdated.** CONTEXT.md D-04 states
> `tmux_sessions.label` "is **never written** — the spawn `INSERT` writes only
> `(task_id, n, name)`". The `INSERT` (`sessions.go:303`) does write only those three columns,
> **but** the spawn handler then executes `UPDATE tmux_sessions SET label = sess.Info().Label
> WHERE name = ?` (`sessions.go:343-347`, added in commit `6c9aa80`, Phase 8-04). So the label
> **is** persisted at spawn today, and `reconcileTmux`'s `"Bash ?"` fallback
> (`sessions.go:114-117`) only fires when that UPDATE genuinely left the column empty. The
> planner must confirm empirically (spawn a tmux tab, restart, check the label) whether the
> `"Bash ?"` bug still reproduces. If it does, the residual bug is narrower than D-04 describes
> — likely the reattach path (`sessions.go:263-274`) reading but not re-writing label, or a
> code path where the UPDATE is skipped. Either way, the fix is to guarantee a non-empty
> label survives restart; the safest belt-and-braces is to also fold the `INSERT` into
> `(task_id, n, name, label)` so the column is never transiently `''`.

**Route registration** — add alongside the existing session routes in `SessionRoutes`
(`sessions.go:24-30`), mirroring `PATCH /api/tasks/{id}` (`routes.go:30`):

```go
mux.HandleFunc("GET /api/sessions", s.list)
mux.HandleFunc("POST /api/sessions", s.create)
mux.HandleFunc("POST /api/sessions/{id}/stop", s.stop)
mux.HandleFunc("DELETE /api/sessions/{id}", s.delete)
mux.HandleFunc("PATCH /api/sessions/{id}", s.rename)   // NEW (D-01/D-03)
```

---

### `internal/session/session.go` — mutable, mutex-guarded `SetLabel`

**Analog:** self — the mutex-guarded field setters `SetWaiting`/`SetIdle`/`ClearWaitingOnAttach` (lines 259–294) and the `Info()` snapshot under the same mutex (lines 197–221).

`Session.label` is set once at Spawn (`session.go:80` field, `manager.go:266-276` assignment)
and read under `s.mu` in `Info()` (line 206). Making it mutable = one setter mirroring
`SetWaiting` (`session.go:259-263`):

```go
// SetWaiting marks the agent as needing input …
func (s *Session) SetWaiting() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.waiting = true
}
```

The new setter:

```go
// SetLabel renames the session's display label under the manager mutex. The
// label was set-once at Spawn (D-04); this is the rename write path.
func (s *Session) SetLabel(label string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.label = label
}
```

Because `Info()` already reads `s.label` under `s.mu` (line 206), the next `List`/`ListByTask`
snapshot and the rename handler's `sess.Info()` response both see the new label with no other
change. `TmuxName()` (line 230) is already exposed for the row-write guard.

---

## Shared Patterns

### Inline click-to-edit (Enter/Esc/blur)
**Source:** `web/src/pages/TaskPage.tsx:488-513` (+ `cancelTitleEditRef` at 112, `commitTitle` at 281-290)
**Apply to:** TaskTabs tab rename (D-01). Enter→`blur()`, Esc→set cancel-ref then `blur()`,
blur→commit; trim before saving. The tab variant diverges only in D-05 (empty commit = reset,
not silent no-op).

### ⋯ DropdownMenu + Ellipsis
**Source:** `web/src/pages/TaskPage.tsx:520-546` (imports at 4 + 22-28)
**Apply to:** the agent header menu (D-06). Structure: conditional item(s) →
`<DropdownMenuSeparator/>` → action item; `align="end"`; trigger is a ghost icon-sm Button
wrapping `<Ellipsis className="size-4" />`.

### Bracketed-paste, prefilled-never-sent
**Source:** `web/src/components/task/AgentTab.tsx:47 + 142 + 175` (`pasteApiRef.current?.paste(...)`)
**Apply to:** `Insert description` and `Insert review prompt` menu items (D-06/D-09). The paste
handle is wired by `TerminalPane`'s `onReady` (`TerminalPane.tsx:177 + 223-225`); xterm wraps
the text in `\x1b[200~..\x1b[201~` — never auto-submits. D-09 changes only the *trigger*
(connect→manual), not the mechanism.

### Collapse + persist under one global localStorage key
**Source:** `web/src/components/layout/ActiveSessionsBar.tsx:33 + 46-49`
**Apply to:** the outside-click auto-collapse (D-11). `collapse()` sets `collapsed=true` and
writes `"1"` under `kamacu:sessions-bar-collapsed` — reuse verbatim; only add the document
pointer listener that calls it.

### Document event listener (add-on-mount / remove-on-cleanup)
**Source:** `web/src/pages/TaskPage.tsx:215-231` and `web/src/pages/BoardPage.tsx:23-43`
**Apply to:** the D-11 outside-click listener. `useEffect` → `addEventListener` →
`return () => removeEventListener`. Guard "inside" via a ref (like the `n`-shortcut guards
typing targets / open dialogs).

### PATCH partial-update handler
**Source:** `internal/api/tasks.go:294-341` (registered `routes.go:30`)
**Apply to:** the session-rename endpoint (Claude's Discretion). `*string` pointer field for
"present vs absent", `json.NewDecoder(...).Decode` → 400 on error, `strings.TrimSpace`,
`writeError`/`writeJSON` helpers, return the updated resource. Route registered next to the
existing session routes (`sessions.go:24-30`).

### Mutex-guarded mutable field
**Source:** `internal/session/session.go:259-294` (`SetWaiting`/`SetIdle`) + `Info()` at 197-221
**Apply to:** `SetLabel` (Claude's Discretion — making `Session.label` mutable). Lock `s.mu`,
mutate, unlock; `Info()` already reads the field under the same lock.

### tmux label row-write (already exists — reuse, don't reinvent)
**Source:** `internal/api/sessions.go:343-347` (spawn back-fill) + `reconcileTmux` read at 76 + 114-117
**Apply to:** the rename handler's tmux-tab persistence AND the D-04 default-label fix. The
`UPDATE tmux_sessions SET label = ? WHERE name = ?` statement is the canonical write; the
`"Bash ?"` fallback at 114-117 is the symptom to eliminate.

---

## No Analog Found

None. Every file in scope maps to an existing in-repo pattern (most in the same file). This
phase is entirely pattern-reuse — no new architectural vocabulary is introduced.

---

## Metadata

**Analog search scope:** `web/src/pages`, `web/src/components/{task,terminal,layout,board,ui}`,
`web/src/api`, `internal/api`, `internal/session`, `internal/settings`, `internal/store/migrations`
**Files scanned:** 14 (TaskPage, AgentTab, TerminalPane, TaskTabs, ActiveSessionsBar, Column,
QuickAdd, BoardPage, sessions.ts, sessions.go, session.go, manager.go, tasks.go, routes.go,
settings.go, dropdown-menu.tsx, migration 00005)
**Pattern extraction date:** 2026-07-03
**Key cross-file note for the planner:** the CONTEXT's D-04 "label is never written" premise is
contradicted by `sessions.go:343-347` (label IS written at spawn since Phase 8) — verify the
`"Bash ?"` reproduction before scoping the fix.

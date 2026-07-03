# Phase 24: Session-Bar, Board & Tab Polish - Context

**Gathered:** 2026-07-03
**Status:** Ready for planning

<domain>
## Phase Boundary

Sharpen four daily-driver surfaces of the task/session UI. Delivers TABS-01/02,
REVMENU-01/02, POLISH-01/02/03:

- **TABS-01/02** — **renameable** bash/tmux tabs whose custom label persists
  across reopening the task and (for tmux-backed tabs) a server restart; new tabs
  keep their auto default (`Bash 1`, `Bash 2`, …) until renamed.
- **REVMENU-01** — the agent view's top-right **Stop button is replaced by a
  ⋯ menu** offering (at least) `Stop` and `Insert review prompt`.
- **REVMENU-02** — the PR-review prompt is **no longer auto-inserted**; it only
  enters the prompt when the user chooses `Insert review prompt`.
- **POLISH-01** — the bottom active-sessions bar **drops the total-sessions
  count** (per-state colored counts stay).
- **POLISH-02** — the active-sessions bar **auto-collapses on an outside click**.
- **POLISH-03** — the To Do column **drops the inline `+ New task`** shortcut.

Scope anchor: this is **UI polish on existing surfaces**. NOT in scope — new tab
kinds, agent-prompt automation of any kind (auto-insert is being *removed*), a new
task-creation flow, or session-bar features beyond the two named cleanups.
</domain>

<decisions>
## Implementation Decisions

### Tab Rename — Affordance (TABS-01)
- **D-01 (double-click inline edit):** The user renames a bash/tmux tab by
  **double-clicking the tab label** in the strip; it swaps to an inline
  `<Input autoFocus>` — **Enter commits, Esc cancels, blur commits** — then reverts
  to a normal trigger. This **reuses the exact task-title edit contract** already in
  `web/src/pages/TaskPage.tsx` (click→Input, Enter/Esc/blur), so there is no new UI
  vocabulary. **Single-click still just selects the tab.** Rejected: a per-tab ⋯
  dropdown (adds a second control to every trigger, which already carries an ×) and a
  hover pencil icon (icon clutter).
- **D-02 (bash/tmux tabs only):** Rename applies **only to bash/tmux tabs**. The
  `Agent`, `Description`, and `Diff` tabs keep their fixed labels — no rename
  affordance.

### Tab Rename — Persistence & Revert (TABS-01/02)
- **D-03 (rename every bash tab; persist what survives):** The rename affordance is
  offered on **every** bash tab regardless of shell mode. The custom label persists
  **in memory** (so it survives navigating away and reopening the task) for **all**
  bash tabs, and **additionally persists across a server restart** for **tmux-backed
  tabs** via the existing `tmux_sessions.label` column. This matches TABS-01
  literally: **the default shell is plain `bash`** (`settings.DefaultSettings[KeyShell]
  = "bash"`), and a plain-bash tab is a PTY child of the server that **does not
  survive a restart at all** — so there is nothing to restore for it, and restart-
  persistence is only meaningful for the opt-in `shell = "tmux"` case. Rejected:
  restricting rename to tmux tabs only (a default-shell user could never rename).
- **D-04 (fix the latent "Bash ?" restart bug):** Today `tmux_sessions.label` is
  **never written** — the spawn `INSERT` at `internal/api/sessions.go` writes only
  `(task_id, n, name)`, so the column stays `''`, and after a restart `reconcileTmux`
  falls back to **`"Bash ?"`** for every survivor. This phase must **persist the
  default `Bash N` at spawn** (or re-derive it deterministically from the tmux name's
  `<n>` on reconcile) so a restarted tmux tab shows its **real** name — default or
  custom — never `"Bash ?"`. Renaming updates the same stored label.
- **D-05 (empty commit reverts to auto default):** Committing an **empty or
  whitespace-only** value in the inline editor **resets the tab to its auto default**
  (`Bash N`, the original spawn ordinal) — a clean "undo my rename" gesture. Labels
  are **trimmed** before saving; whitespace-only is treated as empty. Rejected:
  rejecting the empty commit and keeping the previous name (no way to reset short of
  retyping `Bash N` by hand).

### Review Menu & Agent Header (REVMENU-01/02)
- **D-06 (single ⋯ menu, everything folded in):** The agent pane header's right side
  collapses to a **single ⋯ dropdown** — mirroring the existing task-actions ⋯
  pattern (`DropdownMenu` + `Ellipsis`) in `TaskPage.tsx`. While the agent is
  running the menu holds, in order: **`Insert description`** (only when the task
  description is non-empty), **`Insert review prompt`** (PR reviews only — see D-08),
  a separator, then **`Stop`**. The current standalone `Insert description` button
  (`AgentTab.tsx` `headerActions`) **moves into this menu**.
- **D-07 (agent pane only — bash panes keep inline Stop):** The ⋯-menu treatment
  replaces the Stop button on the **agent pane only**. `TerminalPane` is shared by
  agent and bash panes; **bash tabs keep their inline `Stop` button** unchanged. The
  planner decides the mechanism (e.g. a prop that swaps the header's Stop button for a
  caller-supplied menu, agent-only).
- **D-08 ("Insert review prompt" gating):** The `Insert review prompt` item appears
  **only when the task is a PR review AND the `pr_review_seed` template is non-empty
  AND the agent session is running** (paste needs a live terminal). This exactly
  mirrors how the auto-seed is derived today (`isPR && seedTemplate.trim() !== ""` in
  `TaskPage.tsx`), and the template still interpolates the live PR's `<n>`/`<title>`.
  A normal board task's agent menu never shows the item.
- **D-09 (auto-insert removed — manual only):** REVMENU-02 — the PR-review seed is
  **no longer pasted automatically** on agent connect. Remove the connect-time
  auto-paste (`AgentTab.tsx` `handleConnect` + the module-scope `seededSessionIds`
  guard). The **same paste mechanism** (`pasteApiRef.current?.paste(seed)`, bracketed
  paste, **never auto-sent**) now fires **only** from the `Insert review prompt` menu
  item. The user still edits and presses Enter.

### Active-Sessions Bar & Board (POLISH-01/02/03)
- **D-10 (drop total count):** Remove the total-sessions count from
  `ActiveSessionsBar.tsx`'s collapsed bar. The **per-state colored counts**
  (working / waiting / idle, waiting amber-emphasized) **remain**.
- **D-11 (auto-collapse on outside click):** The bar auto-collapses when the user
  clicks outside it. Reuse the **existing `collapse()` helper** (added by quick task
  260618-mlu, currently called on row-open per SBAR-06) — wire a click-outside
  listener (ref + document `mousedown`/`pointerdown`) that calls it. Persist collapsed
  as `"1"` under the existing global key `kamacu:sessions-bar-collapsed`, consistent
  with the row-open collapse. Clicking the bar itself still toggles.
- **D-12 (remove To Do quick-add):** Remove the inline `+ New task` `QuickAdd` row
  from the To Do column (`Column.tsx` `status === "todo"` slot; delete/retire
  `web/src/components/board/QuickAdd.tsx`). **Safe** — task creation remains via the
  board-header **"New task"** button and the **`n`** keyboard shortcut, both of which
  open `NewTaskDialog` (`BoardPage.tsx`).

### Claude's Discretion
Left to research/planning:
- Exact rename API shape (a `PATCH /api/sessions/{id}` with `{label}`, or similar)
  and whether it updates the in-memory `Session.label` under the manager mutex plus
  the `tmux_sessions.label` row for tmux tabs. `Session.label` is set-once today —
  making it mutable (guarded) is a planning call.
- Whether the default `Bash N` is stored at spawn (extend the `INSERT`) or
  re-derived from the tmux name's `<n>` on reconcile — either satisfies D-04.
- The `TerminalPane` mechanism for the agent-only header-menu swap (D-07).
- Rename behavior on an exited / closing / not-yet-reattached (orphaned survivor)
  bash tab — enable, disable, or hide the affordance.
- Outside-click listener details (capture phase, which nested surfaces count as
  "inside", whether Escape also collapses) for D-11.
- Menu item copy/ordering, dropdown alignment, and label max-length / truncation.
</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase scope & requirements
- `.planning/ROADMAP.md` § "Phase 24: Session-Bar, Board & Tab Polish" — goal + the 5 success criteria (TABS-01/02, REVMENU-01/02, POLISH-01/02/03)
- `.planning/REQUIREMENTS.md` — TABS-01/02, REVMENU-01/02, POLISH-01/02/03 wording, and the "Auto-inserting or auto-sending any agent prompt" out-of-scope row (the intent behind REVMENU-02)
- `.planning/STATE.md` § "Planning grounding for v1.8" — the "Tabs seams" and "Review-menu / polish seams" for Phase 24 (treated as fact)

### Tab rename — persistence backend (MUST read — the label column already exists)
- `internal/store/migrations/00005_tmux_sessions.sql` — the `tmux_sessions` table with the **`label TEXT NOT NULL DEFAULT ''`** column (present since Phase 8, currently unwritten). Rows are inserted at spawn BEFORE spawning and are NOT deleted on kill (MAX(n)+1 monotonic).
- `internal/api/sessions.go` — the spawn handler's `INSERT INTO tmux_sessions (task_id, n, name)` (**does not write `label`** — the D-04 bug), `reconcileTmux` (reads `label`, falls back to `"Bash ?"` when empty — TMUX-05 post-restart survivor path), and the `reattach` variant that reads the persisted `label`.
- `internal/session/manager.go` — `Spawn` label assignment (`"Bash %d"` from the per-task `taskCounters`, in memory only), the `Session.label` field, `HasLiveTmux`, `List`/`ListByTask`.
- `internal/session/session.go` — the `Session` struct (`label` set once at spawn) and `Info` snapshot (`Label` field surfaced to the API/UI).

### Tabs / agent header — frontend
- `web/src/components/task/TaskTabs.tsx` — the tab strip (`TabDef[]`, `TabsTrigger`, the × close affordance); the rename inline-editor lands here.
- `web/src/pages/TaskPage.tsx` — the **task-title inline-edit pattern to reuse** (click→`<Input autoFocus>`, Enter/Esc/blur), `visibleSessions.map` building bash `TabDef`s from `s.label`, the PR-review `seed` derivation (`isPR && seedTemplate.trim() !== ""`), and the established **task-actions `DropdownMenu` + `Ellipsis`** pattern for D-06.
- `web/src/components/task/AgentTab.tsx` — the `Insert description` `headerActions` button (moves into the menu), and `handleConnect` + module-scope `seededSessionIds` **auto-paste to remove** (D-09); `pasteApiRef.current?.paste(...)` is the manual-insert mechanism to keep.
- `web/src/components/terminal/TerminalPane.tsx` — the shared pane header (`label` left, `headerStatus` + `headerActions` + the `showStop` `Stop` button right); D-07 swaps Stop for a menu on the **agent pane only**.
- `web/src/api/sessions.ts` — `TermSession` (`label` field), `useSpawnSession`/`useStopSession`; a new rename mutation likely lands here.

### Active-sessions bar & board — frontend
- `web/src/components/layout/ActiveSessionsBar.tsx` — the collapsed bar (`total` count to drop per D-10), the `collapse()` helper + `toggle`, and the `kamacu:sessions-bar-collapsed` global key (D-11).
- `web/src/components/board/Column.tsx` — the `status === "todo"` `<QuickAdd>` slot to remove (D-12).
- `web/src/components/board/QuickAdd.tsx` — the inline `+ New task` row being retired (D-12).
- `web/src/pages/BoardPage.tsx` — the **surviving** task-creation paths: the header `New task` button + `n` shortcut → `NewTaskDialog` (confirms D-12 is safe).

### Settings / shell context
- `internal/settings/settings.go` — `KeyShell` default **`"bash"`** (why plain-bash tabs are the default and don't survive restart — grounds D-03) and `KeyAgentExtraParams`; `pr_review_seed` is the KV template behind D-08.

No new external specs/ADRs — decisions are fully captured above.
</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- **Task-title inline editor** (`TaskPage.tsx`) — click→`<Input autoFocus>` with
  Enter/Esc/blur commit/cancel; **copy this contract verbatim** for the tab rename
  (D-01).
- **`tmux_sessions.label` column** — already exists and is already **read** by
  `reconcileTmux`; the phase only needs to start **writing** it (D-03/D-04). No new
  migration required for the label itself.
- **`collapse()` helper** (`ActiveSessionsBar.tsx`) — the exact collapse+persist
  used on row-open; the outside-click listener calls it (D-11).
- **task-actions `DropdownMenu` + `Ellipsis`** (`TaskPage.tsx`) — the ⋯-menu
  pattern to mirror for the agent header (D-06).
- **`pasteApiRef` bracketed-paste mechanism** (`AgentTab.tsx`) — reused for the
  manual `Insert review prompt` (D-09); never auto-sends.

### Established Patterns
- **Server is tab truth (D-28); `tmux_sessions` is the persistence table** — labels
  belong there for restart survival, not localStorage.
- **Prompt injection = bracketed paste, prefilled-never-sent** (D-35/36/37 Insert
  description; D-06/07 PR seed) — REVMENU keeps this, only changing the *trigger* from
  auto→manual.
- **Session bar collapse persists under one global localStorage key**, default
  collapsed (SBAR-07) — D-11 extends the same key, no new persistence.
- **git/tmux identity is machine-derived** — `Bash N` is re-derivable from the tmux
  name `kamacu-<task>-<n>` if the planner prefers re-derivation over a stored default.

### Integration Points
- **New rename endpoint** — a session-label PATCH updating the in-memory
  `Session.label` (mutex-guarded; it is set-once today) and, for tmux tabs, the
  `tmux_sessions.label` row. Registered alongside the existing session routes.
- **Spawn `INSERT`** (`sessions.go`) — extend to persist the default `Bash N`
  label (or handle it at reconcile) to close the `"Bash ?"` bug (D-04).
- **Agent-only header menu** — `AgentTab` passes a menu into `TerminalPane` that
  supersedes the shared `showStop` button for the agent pane; bash panes untouched
  (D-07).
- **Outside-click listener** — `ActiveSessionsBar` gains a ref + document pointer
  listener that calls `collapse()` (D-11).

</code_context>

<specifics>
## Specific Ideas

- **The "Bash ?" symptom is the tell for D-04:** if a restarted tmux tab shows
  `Bash ?`, the label persistence is incomplete — that string is the current
  `reconcileTmux` fallback and must never appear after this phase.
- **Rename mirrors the task title exactly** — the user explicitly likened the
  desired feel to the existing double-click-to-edit title (chosen preview showed the
  inline `┃my-server┃×` editor in the strip).
- **One adaptive ⋯ menu, not two controls** — the agent header should read as a
  single ⋯ (like the task-actions menu), with `Insert description` folded in rather
  than left as a separate button.
</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope. (Scope-adjacent ideas that were
explicitly *ruled out* rather than deferred: showing `Insert review prompt` for
non-PR tasks, and restricting rename to tmux tabs only — both rejected in D-08/D-03.)

</deferred>

---

*Phase: 24-session-bar-board-tab-polish*
*Context gathered: 2026-07-03*

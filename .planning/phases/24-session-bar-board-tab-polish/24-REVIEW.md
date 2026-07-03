---
phase: 24-session-bar-board-tab-polish
reviewed: 2026-07-03T17:47:51Z
depth: standard
files_reviewed: 11
files_reviewed_list:
  - internal/api/sessions.go
  - internal/api/sessions_test.go
  - internal/session/session.go
  - web/src/api/sessions.ts
  - web/src/components/board/Board.tsx
  - web/src/components/board/Column.tsx
  - web/src/components/layout/ActiveSessionsBar.tsx
  - web/src/components/task/AgentTab.tsx
  - web/src/components/task/TaskTabs.tsx
  - web/src/components/terminal/TerminalPane.tsx
  - web/src/pages/TaskPage.tsx
findings:
  critical: 0
  warning: 4
  info: 6
  total: 10
status: issues_found
---

# Phase 24: Code Review Report

**Reviewed:** 2026-07-03T17:47:51Z
**Depth:** standard
**Files Reviewed:** 11
**Status:** issues_found

## Summary

Phase 24 adds the session-rename backend (`Session.SetLabel`, `PATCH /api/sessions/{id}`, tmux `label` persistence), the active-sessions-bar outside-click auto-collapse plus total-count removal, the agent `⋯` dropdown that folds Insert-description / Insert-review-prompt / Stop, the tab-rename frontend (`TaskTabs` inline editor + `useRenameSession`), and a board cleanup that drops `QuickAdd` / the `projectId` prop from `Column`.

Overall the change is coherent and well-guarded — the rename path is mutex-guarded, the tmux `label` persistence is warn-only (correct degradation posture), and the `defaultTmuxLabel` sentinel-removal is backed by tests. The Go backend compiles clean. No BLOCKER-severity defects were found: no injection, no auth bypass, no data-loss risk, no crash. SQL uses parameterized queries throughout; the rename UPDATE targets `tmux_sessions.name` (a `NOT NULL UNIQUE` column) so it can never touch the wrong row.

The findings below are correctness/robustness edge cases and quality issues. The most material is a genuine input-validation gap in the rename handler (empty/whitespace-only body decode diverges from the sibling `create` handler and rejects a legitimately-empty PATCH), plus several UX/consistency rough edges around the rename editor and the agent menu Stop item.

## Warnings

### WR-01: `rename` rejects an empty request body with 400, diverging from the sibling `create` handler

**File:** `internal/api/sessions.go:414-417`
**Issue:** The `rename` handler decodes the body with:
```go
if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
    writeError(w, http.StatusBadRequest, "invalid JSON body")
    return
}
```
Unlike `create` (line 168), which deliberately tolerates `io.EOF` (`... && !errors.Is(err, io.EOF)`), `rename` treats an empty body as a hard 400. The handler's own doc comment and the very next block (`if req.Label == nil { ... return the current snapshot unchanged }`) explicitly document a "nothing to rename" no-op path — but that path is unreachable for an empty body, because `Decode` on an empty body returns `io.EOF` and 400s first. A `PATCH /api/sessions/{id}` with no body (a reasonable "touch/no-op") therefore fails instead of returning the current snapshot. The current frontend always sends `{ label }`, so this is not user-visible today, but it is an API-contract inconsistency and a latent trap for any future caller.
**Fix:**
```go
if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
    writeError(w, http.StatusBadRequest, "invalid JSON body")
    return
}
```
`errors` and `io` are already imported. With this change the existing `req.Label == nil` no-op branch handles the empty body as the doc comment claims.

### WR-02: Agent `⋯` menu Stop item has no in-flight/disabled guard — repeatable Stop clicks fire redundant POSTs and lose error feedback

**File:** `web/src/components/task/AgentTab.tsx:176-181`
**Issue:** The dropdown Stop item calls `stopSession.mutate(agentSession.id)` with no `disabled` state and no error handling:
```jsx
<DropdownMenuItem
  variant="destructive"
  onSelect={() => stopSession.mutate(agentSession.id)}
>
  {`Stop`}
</DropdownMenuItem>
```
This is a regression in robustness versus the inline Stop button in `TerminalPane` (lines 205-209), which tracks `stopping` state (`disabled={stopping}`, `onError: () => setStopping(false)`) so a click is single-shot and the button stays disabled through the D-14 grace window. Because the menu closes on select and the agent tab stays `running` for up to ~5s, a user who reopens the menu and clicks Stop again fires a second `POST /stop`. Stop is idempotent server-side (202 either way), so this is not a correctness bug, but it wastes requests and — more importantly — `stopSession` here has no `onError`, so a failed Stop is silently swallowed with no UI signal, unlike every other mutation in this component (all of which surface `Couldn't start a session. Try again.`).
**Fix:** Disable the item while the stop is pending and surface failure, e.g. gate `onSelect` on `!stopSession.isPending` and add an `onError` toast/log, or reuse the same `stopping`-style local flag the pane uses. At minimum add `disabled={stopSession.isPending}` to the item.

### WR-03: Rename input stays editable after the session exits, then silently drops the commit

**File:** `web/src/components/task/TaskTabs.tsx:65-80`, `129-152`; `web/src/pages/TaskPage.tsx:402-405`
**Issue:** `TaskPage` gates `onRename` on `s.status === "running" && !closing`, so when a bash session exits (via poll or `x` frame) while its label is being renamed, `tab.onRename` flips to `undefined`. But `TaskTabs` keeps `renamingId === tab.id` true and keeps rendering the `<Input>`. On commit, `commitRename` calls `tab.onRename?.(...)` — now a no-op — so the user's typed rename is silently discarded with zero feedback (the input just disappears and the label snaps back to the muted exited label). The user has no way to know their rename was dropped.
**Fix:** Either (a) keep the tab renamable while it is still in the visible strip and let the server 404/no-op decide, or (b) when `renamingId` points at a tab whose `onRename` is now `undefined`, clear `renamingId` and drop the input up front so the state is honest. Option (a) is cleaner: the backend already 404s an unknown/exited session id, and the frontend already invalidates on the poll — so allowing the PATCH and letting it fail is consistent with the rest of the tab lifecycle.

### WR-04: Empty-reset on a plain-bash (non-tmux) tab is a confusing no-op that still round-trips through the server

**File:** `internal/api/sessions.go:430-436`; `web/src/components/task/TaskTabs.tsx:75-79`
**Issue:** The frontend treats a trimmed-empty commit as an explicit "reset to default" intent and sends `""` (TaskTabs lines 75-77, comment "DO NOT early-return on empty"). The backend, for a non-tmux session, resolves an empty label to `sess.Info().Label` — i.e. it leaves the label **unchanged** (sessions.go lines 430-435). So on a plain-bash tab, "clear the name and commit" does nothing but the user is given no indication the reset was rejected/ignored; the tab keeps whatever custom name it had. This is an inconsistent contract: the same gesture "resets to Bash N" for a tmux tab but is a silent no-op for a plain-bash tab. Since `onRename` is offered on *every* bash tab regardless of shell mode (TaskPage comment lines 71-73), a user cannot tell which behavior they will get. The behavior is documented as "Claude's discretion" in the code, but from the user's seat it reads as a bug.
**Fix:** Make the contract uniform — for a non-tmux tab, either (a) reset the in-memory label to a derivable default (the per-task counter would need exposing on `Session`), or (b) reject the empty commit with a clear response so the frontend can keep the editor open / show the prior name, rather than silently retaining the old custom label. Minimally, document the divergence in the user-facing behavior or suppress the `onRename` empty-send for non-tmux tabs so no pointless request is made.

## Info

### IN-01: `topSlot` prop on `Column` is now dead — no caller passes it

**File:** `web/src/components/board/Column.tsx:11-16`, `44`
**Issue:** After the phase-24 refactor, `Column` is instantiated only once (`Board.tsx:201`), and that call passes only `status` and `tasks`. The `topSlot?: ReactNode` prop and its `{topSlot}` render slot (line 44) are now unreachable dead API surface — the same commit that removed `QuickAdd` also removed the only conceptual user of `topSlot`.
**Fix:** Drop `topSlot` from `ColumnProps` and remove the `{topSlot}` line, or leave a short comment noting it is a deliberate extension point if future use is planned.

### IN-02: `headerActions` prop on `TerminalPane` is now unused by all callers

**File:** `web/src/components/terminal/TerminalPane.tsx:24`, `314`
**Issue:** Phase 24 moved the agent's Insert-description action out of `headerActions` and into the new `headerMenu` (⋯) dropdown. `AgentTab` no longer passes `headerActions` (it passes `headerMenu={agentMenu}`), and no bash caller ever passed it. The `headerActions?: ReactNode` prop and its render site (line 314) are now dead.
**Fix:** Remove `headerActions` from `TerminalPaneProps` and the header render if no near-term use is planned; otherwise leave a comment marking it as a reserved slot.

### IN-03: `commitRename` compares the typed value against the pre-cap label, so a >200-char rename re-fires on the next edit

**File:** `web/src/components/task/TaskTabs.tsx:77`; `internal/api/sessions.go:438-440`
**Issue:** The server caps the label at 200 runes (`maxLabelRunes`). The frontend's `commitRename` only sends when `trimmed !== tab.label`. After a >200-rune rename, `tab.label` becomes the *capped* server value, but if the user re-opens the editor (seeded from the capped `tab.label`) and commits unchanged, `trimmed === tab.label` so nothing is sent — consistent. However, there is no client-side maxLength on the `<Input>` (line 136-152), so the editor accepts arbitrarily long input that the server silently truncates, giving the user no feedback that their label was cut.
**Fix:** Add `maxLength={200}` to the rename `<Input>` (and, ideally, share the constant with the backend) so the truncation is visible at the point of entry rather than a silent server-side cut.

### IN-04: `defaultTmuxLabel` "Bash" fallback masks a malformed machine-minted name instead of logging it

**File:** `internal/api/sessions.go:387-394`
**Issue:** `defaultTmuxLabel` returns a bare `"Bash"` (no ordinal) when the tmux name does not end in `-<int>`. The comment notes the name is server-controlled, so a malformed name should be impossible in practice — which means hitting the fallback silently indicates a real invariant break (a corrupted/hand-written `tmux_sessions.name`). Returning a plausible-looking `"Bash"` hides that. This is defensive-by-design and not wrong, but a `slog.Warn` on the fallback branch would surface an otherwise-invisible data-integrity anomaly.
**Fix:** Add a `slog.Warn("tmux name has no numeric ordinal", "name", tmuxName)` before `return "Bash"` so a malformed name is observable.

### IN-05: Duplicated `UPDATE tmux_sessions SET label` statement across spawn back-fill and rename

**File:** `internal/api/sessions.go:356-358` and `446-448`
**Issue:** The exact same warn-only persistence statement (`UPDATE tmux_sessions SET label = ? WHERE name = ?` with identical `slog.Warn("persisting tmux session label", ...)` handling) appears in both `create` (spawn back-fill) and `rename`. The comments even cross-reference each other ("the SAME statement the spawn back-fill uses"). This is minor duplication that will drift if the persistence logic changes.
**Fix:** Extract a small helper, e.g. `func (h *sessionHandlers) persistTmuxLabel(name, label string)`, called from both sites.

### IN-06: Rename `draft`/`renamingId` state is not reset when the whole `TaskTabs` component's tab set changes identity

**File:** `web/src/components/task/TaskTabs.tsx:61-63`
**Issue:** `renamingId` and `draft` are component-local state keyed to a tab id string. They rely on the `autoFocus` + blur-commit contract to self-clear when the user navigates away. This works for the common case, but there is no guard for a rename being open on a tab that is removed from `tabs` without a blur (e.g. a fast poll drops the session between renders). In that window `renamingId` points at a tab id no longer in `tabs`, so the editor simply stops rendering and the in-progress rename is dropped with no commit — a benign but silent loss (related to WR-03).
**Fix:** Reset `renamingId`/`draft` in an effect when `renamingId` is set but no `tab.id` in `tabs` matches it, making the state self-heal rather than relying solely on blur.

---

_Reviewed: 2026-07-03T17:47:51Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_

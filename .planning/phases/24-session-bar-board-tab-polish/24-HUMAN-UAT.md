---
status: complete
phase: 24-session-bar-board-tab-polish
source: [24-VERIFICATION.md]
started: 2026-07-03T17:59:26Z
updated: 2026-07-04T07:10:00Z
---

## Current Test

[complete — all 6 passed; item 2 fixed in 1b12b52 and re-verified by human in a running app]

## Tests

### 1. Rename a bash/tmux tab and confirm the custom label survives reopening the task
expected: Double-click a bash tab label → inline Input appears; type a name, press Enter → the tab shows the custom name; navigate away and reopen the task → the custom name is still there
result: pass

### 2. Rename a TMUX tab, then restart the server, and confirm the label persists (never "Bash ?")
expected: For a tmux-backed tab, a custom label survives a full server restart via tmux_sessions.label; a never-renamed survivor shows its real "Bash N" default, never the old "Bash ?" sentinel
result: pass — fixed in 1b12b52 (reattach dropped the persisted label + back-fill clobber) and re-verified by human in a running app. Regression test TestSessionTmuxReattachPreservesCustomLabel covers rename → restart → reattach. See GAP-01 below.

### 3. Empty-commit a tab rename and confirm it resets to the auto default
expected: Clear the rename Input (or type only whitespace) and commit → a tmux tab resets to its "Bash N" default; a new tab still defaults to "Bash 1", "Bash 2", … until renamed
result: pass

### 4. Open the agent view and confirm the top-right control is a three-dots (⋯) menu, not a Stop button
expected: The agent pane header shows an Ellipsis (⋯) trigger; opening it offers "Stop" and (for a PR-review task) "Insert review prompt" plus "Insert description"; bash tabs still show their inline Stop button
result: pass

### 5. Open a PR review and confirm the review prompt is NOT auto-inserted, then insert it via the menu
expected: On opening a PR-review agent session, the prompt is empty (no auto-paste); selecting "Insert review prompt" from the ⋯ menu pastes the seed (bracketed, un-sent — the user still presses Enter)
result: pass

### 6. Confirm the bottom active-sessions bar auto-collapses when clicking outside it
expected: Expand the bar, then click anywhere outside it (e.g. back into the task view) → the bar collapses and stays collapsed on reload; clicking the bar itself still toggles normally
result: pass

## Summary

total: 6
passed: 6
issues: 0
pending: 0
skipped: 0
blocked: 0
note: item 2 fixed in 1b12b52 (regression-tested) and re-verified by human

## Gaps

### GAP-01 (TABS-01): tmux custom label lost on restart via reattach clobber
status: resolved
fixed_in: 1b12b52
fix_applied: |
  reattach branch now captures tmux_sessions.label into reattachLabel and
  reapplies it via sess.SetLabel after Spawn (internal/api/sessions.go), so both
  the wire reply and the post-spawn back-fill keep the custom name. Empty stored
  labels still fall through to the derived default. Regression test
  TestSessionTmuxReattachPreservesCustomLabel added (fails without the fix).
requirement: TABS-01
symptom: A renamed tmux tab reverts to "Bash N" after a server restart; the persisted tmux_sessions.label is also overwritten.
root_cause: |
  The `create` handler's reattach branch (internal/api/sessions.go:263-281) reads the
  persisted label only for an existence check and never applies it — `Manager.Spawn`
  (internal/session/manager.go:266-275) has no label parameter and always derives
  "Bash N" from its per-task counter. The unconditional post-spawn label back-fill
  (internal/api/sessions.go:355-359) then persists that wrong derived label back over
  the good stored value. Missed because no test covers rename → restart → reattach.
fix_options:
  - Carry the stored label through reattach: hoist it to a `reattachLabel` var, call
    `sess.SetLabel(reattachLabel)` after Spawn (mirroring reconcile's empty→defaultTmuxLabel),
    so the existing back-fill persists the correct label idempotently.
  - Fix-at-source: add a `Label` field to `SpawnOpts` and honor it in `Spawn`, set from the
    reattach branch.
test_needed: rename tmux tab → simulate restart (fresh Manager) → reattach → assert live
  label == custom AND tmux_sessions.label still == custom.

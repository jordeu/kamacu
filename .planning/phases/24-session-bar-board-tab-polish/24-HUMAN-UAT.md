---
status: partial
phase: 24-session-bar-board-tab-polish
source: [24-VERIFICATION.md]
started: 2026-07-03T17:59:26Z
updated: 2026-07-03T17:59:26Z
---

## Current Test

[awaiting human testing]

## Tests

### 1. Rename a bash/tmux tab and confirm the custom label survives reopening the task
expected: Double-click a bash tab label → inline Input appears; type a name, press Enter → the tab shows the custom name; navigate away and reopen the task → the custom name is still there
result: [pending]

### 2. Rename a TMUX tab, then restart the server, and confirm the label persists (never "Bash ?")
expected: For a tmux-backed tab, a custom label survives a full server restart via tmux_sessions.label; a never-renamed survivor shows its real "Bash N" default, never the old "Bash ?" sentinel
result: [pending]

### 3. Empty-commit a tab rename and confirm it resets to the auto default
expected: Clear the rename Input (or type only whitespace) and commit → a tmux tab resets to its "Bash N" default; a new tab still defaults to "Bash 1", "Bash 2", … until renamed
result: [pending]

### 4. Open the agent view and confirm the top-right control is a three-dots (⋯) menu, not a Stop button
expected: The agent pane header shows an Ellipsis (⋯) trigger; opening it offers "Stop" and (for a PR-review task) "Insert review prompt" plus "Insert description"; bash tabs still show their inline Stop button
result: [pending]

### 5. Open a PR review and confirm the review prompt is NOT auto-inserted, then insert it via the menu
expected: On opening a PR-review agent session, the prompt is empty (no auto-paste); selecting "Insert review prompt" from the ⋯ menu pastes the seed (bracketed, un-sent — the user still presses Enter)
result: [pending]

### 6. Confirm the bottom active-sessions bar auto-collapses when clicking outside it
expected: Expand the bar, then click anywhere outside it (e.g. back into the task view) → the bar collapses and stays collapsed on reload; clicking the bar itself still toggles normally
result: [pending]

## Summary

total: 6
passed: 0
issues: 0
pending: 6
skipped: 0
blocked: 0

## Gaps

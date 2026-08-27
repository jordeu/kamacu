---
status: complete
phase: 16-global-view-settings-bar-integration
source: [16-VERIFICATION.md]
started: 2026-08-27T14:10:00Z
updated: 2026-08-27T14:58:00Z
---

## Current Test

[testing complete]

## Tests

### 1. Live bar row + click-through + highlight (GINT-01)
expected: A live global agent session renders as a Global · Scratchpad bar row; clicking navigates to /global; the row highlights while /global is the current page; task rows keep their existing navigation/highlight
result: pass

### 2. /global interactive session parity (GVIEW-01 behavior half)
expected: Start agent spawns the configured default agent in the global root (terminal streams); ⋯ menu renders exactly Stop (destructive, no Insert items, no separator); Stop works; trailing + spawns bash tabs with task-parity options; 409 on a second concurrent agent
result: pass

### 3. 7-state matrix walkthrough (GVIEW-04)
expected: unconfigured → D-39 hero with Open Settings; folder root → shell + persistent D-38 banner; repo root → shell with NO banner; root renamed on disk while live → D-42 advisory banner with terminals still streaming; all stopped + vanished → distinct D-40 hero naming the path in mono
result: pass

### 4. Settings Scratchpad flows (GCONF-05 observable closure)
expected: Open Scratchpad reaches /global; default-agent Select instant-saves (hint 'Applies at the next Start.'); Change root with a repo shows the blocking Cloning spinner then the managed badge; with a live session, Save root/Clear root surface the 409 lead sentence + mono reasons list with the dialog still open; Clear root (all stopped) returns to 'No root configured yet.'
result: pass

## Summary

total: 4
passed: 4
issues: 0
pending: 0
skipped: 0
blocked: 0

## Gaps

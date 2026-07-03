---
phase: 24-session-bar-board-tab-polish
verified: 2026-07-03T20:05:00Z
status: human_needed
score: 5/5 must-haves verified
overrides_applied: 0
human_verification:
  - test: "Rename a bash/tmux tab and confirm the custom label survives reopening the task"
    expected: "Double-click a bash tab label → inline Input appears; type a name, press Enter → the tab shows the custom name; navigate away and reopen the task → the custom name is still there"
    why_human: "End-to-end UI gesture + in-memory server persistence across navigation; grep confirms the double-click editor, useRenameSession mutation, and SetLabel write path exist, but the rendered gesture and cross-navigation survival need a running app"
  - test: "Rename a TMUX tab, then restart the server, and confirm the label persists (never 'Bash ?')"
    expected: "For a tmux-backed tab, a custom label survives a full server restart via tmux_sessions.label; a never-renamed survivor shows its real 'Bash N' default, never the old 'Bash ?' sentinel"
    why_human: "Requires an actual server restart with a live tmux session; the backend logic (persist UPDATE + defaultTmuxLabel re-derive) is unit-tested (TestRenameTmuxSession, TestReconcileNeverShowsBashQuestion pass), but real restart survival is runtime"
  - test: "Empty-commit a tab rename and confirm it resets to the auto default"
    expected: "Clear the rename Input (or type only whitespace) and commit → a tmux tab resets to its 'Bash N' default; a new tab still defaults to 'Bash 1', 'Bash 2', … until renamed"
    why_human: "The empty-reset path is unit-tested on the backend (TestRenameEmptyResetsDefault passes) and structurally present in TaskTabs commitRename, but the visible reset behavior needs the running UI"
  - test: "Open the agent view and confirm the top-right control is a three-dots (⋯) menu, not a Stop button"
    expected: "The agent pane header shows an Ellipsis (⋯) trigger; opening it offers 'Stop' and (for a PR-review task) 'Insert review prompt' plus 'Insert description'; bash tabs still show their inline Stop button"
    why_human: "Visual appearance and menu contents; grep confirms headerMenu supersedes showStop in TerminalPane and the menu items are built in AgentTab, but the rendered dropdown needs a running app"
  - test: "Open a PR review and confirm the review prompt is NOT auto-inserted, then insert it via the menu"
    expected: "On opening a PR-review agent session, the prompt is empty (no auto-paste); selecting 'Insert review prompt' from the ⋯ menu pastes the seed (bracketed, un-sent — the user still presses Enter)"
    why_human: "Runtime terminal/PTY behavior; grep confirms seededSessionIds and the connect-time seed block are deleted and the only paste(seed) call is in the menu item onSelect, but the actual no-auto-paste + manual-paste behavior needs a live agent session"
  - test: "Confirm the bottom active-sessions bar auto-collapses when clicking outside it"
    expected: "Expand the bar, then click anywhere outside it (e.g. back into the task view) → the bar collapses and stays collapsed on reload; clicking the bar itself still toggles normally"
    why_human: "DOM outside-click interaction; grep confirms the mousedown listener + barRef + collapse() wiring, but the click-outside gesture and no-double-fire behavior need a running app"
---

# Phase 24: Session-Bar, Board & Tab Polish Verification Report

**Phase Goal:** Sharpen the daily-driver surfaces — renameable bash/tmux tabs, a task-view actions menu that replaces the auto-inserted review prompt, and a cleaner active-sessions bar and board.
**Verified:** 2026-07-03T20:05:00Z
**Status:** human_needed
**Re-verification:** No — initial verification

## Goal Achievement

Every observable truth is structurally VERIFIED in the actual codebase (source under `internal/` and `web/src/`, not SUMMARY claims), backed by passing backend tests and a green frontend build. Status is `human_needed` (not `passed`) because six user-facing runtime behaviors — the rename gesture, restart persistence, the ⋯ menu appearance, the PR-review no-auto-paste, and the outside-click collapse — are UI/runtime flows that grep and unit tests confirm structurally but cannot exercise observationally.

### Observable Truths

| # | Truth | Status | Evidence |
| --- | ----- | ------ | -------- |
| 1 | User can rename a bash/tmux tab to a custom label that persists across reopening the task and a server restart; new tabs default to Bash N until renamed (TABS-01, TABS-02) | ✓ VERIFIED | Backend: `Session.SetLabel` (session.go:271-275, mutex-guarded); `rename` handler + `PATCH /api/sessions/{id}` (sessions.go:405-451, 31); tmux persistence `UPDATE tmux_sessions SET label` (sessions.go:446); INSERT writes default `Bash N` (sessions.go:314-315); `defaultTmuxLabel` re-derives on restart, sentinel `"Bash ?"` count = 0 (sessions.go:387-394, 123). Frontend: `useRenameSession` PATCH (sessions.ts:142-154); double-click inline editor with Enter/Esc/blur (TaskTabs.tsx:129-172); per-bash-tab `onRename` wiring, fixed tabs excluded (TaskPage.tsx:402-405, 340-388). Tests: TestRenameTmuxSession, TestRenameEmptyResetsDefault, TestReconcileNeverShowsBashQuestion PASS |
| 2 | The agent view's top-right Stop button is replaced by a ⋯ menu offering Stop and Insert review prompt (REVMENU-01) | ✓ VERIFIED | TerminalPane renders `headerMenu` in place of the Stop button when set, keeps inline Stop when undefined (TerminalPane.tsx:25, 318-332); AgentTab builds the ⋯ DropdownMenu with Insert description / Insert review prompt / Stop (AgentTab.tsx:153-184); `headerMenu={agentMenu}` passed (AgentTab.tsx:249); bash panes keep `showStop` inline Stop (TerminalPane.tsx:212, 321-331) |
| 3 | Opening a PR review no longer auto-inserts the review prompt — it appears only when the user chooses Insert review prompt (REVMENU-02) | ✓ VERIFIED | `seededSessionIds` fully removed (grep count = 0 in AgentTab.tsx); `handleConnect` now does ONLY the D-45 optimistic clear, the connect-time seed block is gone (AgentTab.tsx:191-199); the sole `pasteApiRef.current?.paste(seed)` call is inside the Insert-review-prompt menu item onSelect (AgentTab.tsx:170); bracketed paste, never auto-sent |
| 4 | The bottom active-sessions bar drops the total count (per-state colored counts remain) and auto-collapses on outside click (POLISH-01, POLISH-02) | ✓ VERIFIED | Total render span removed — `tabular-nums">{total}` count = 0; only three `CountGroup`s (working/waiting/idle) render in the collapsed bar (ActiveSessionsBar.tsx:155-162); `total` var retained solely for `total === 0` empty-state (line 86, 111, 150); document `mousedown` listener guarded on `collapsed`, ref-contains check, calls `collapse()` (ActiveSessionsBar.tsx:59-69); barRef on outer wrapper (line 106) |
| 5 | The To Do column no longer shows the inline + New task shortcut (POLISH-03) | ✓ VERIFIED | `QuickAdd.tsx` deleted (git ls-files empty; was present in pre-phase-24 worktree); zero `QuickAdd` references under `web/src` (grep -rc = 0); Column.tsx has no `status === "todo"` QuickAdd render (grep count = 0) |

**Score:** 5/5 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| `internal/session/session.go` | Session.SetLabel mutex-guarded rename write path | ✓ VERIFIED | `func (s *Session) SetLabel(label string)` locks s.mu, sets s.label (271-275); Info() already reads s.label under s.mu |
| `internal/api/sessions.go` | rename handler + PATCH route + hardened label persistence | ✓ VERIFIED | rename handler (405-451); PATCH route registered (31); INSERT with label column (315); defaultTmuxLabel re-derive replaces sentinel (387-394); go build + go vet clean |
| `web/src/api/sessions.ts` | useRenameSession mutation (PATCH) | ✓ VERIFIED | Exports useRenameSession(taskId), mutationFn PATCHes `/api/sessions/${id}` with {label}, onSuccess optimistic cache write + invalidate (142-154) |
| `web/src/components/task/TaskTabs.tsx` | double-click inline rename editor | ✓ VERIFIED | onRename field on TabDef (43); onDoubleClick gated on tab.onRename (160-168); inline Input with cancelRef commit path, empty→reset (65-80, 136-152) |
| `web/src/components/terminal/TerminalPane.tsx` | agent-only headerMenu prop | ✓ VERIFIED | headerMenu?: ReactNode (25); supersedes Stop when set, keeps showStop when undefined (318-332); handleStop/stopping/showStop retained for bash panes |
| `web/src/components/task/AgentTab.tsx` | ⋯ menu; no connect-time auto-paste | ✓ VERIFIED | DropdownMenu with 3 items (154-183); seededSessionIds removed; handleConnect seed block deleted (191-199) |
| `web/src/components/layout/ActiveSessionsBar.tsx` | total removed + outside-click collapse | ✓ VERIFIED | total render span removed; mousedown listener → collapse() (59-69) |
| `web/src/components/board/Column.tsx` | To Do column without QuickAdd | ✓ VERIFIED | QuickAdd import + render removed; zero refs |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | -- | --- | ------ | ------- |
| rename handler | Session.SetLabel | sess.SetLabel(newLabel) under manager Get | ✓ WIRED | sessions.go:441 |
| rename handler | tmux_sessions.label row | UPDATE tmux_sessions SET label WHERE name | ✓ WIRED | sessions.go:446 |
| TaskTabs rename commit | useRenameSession | onRename(label) → mutate({id,label}) | ✓ WIRED | TaskTabs commitRename → tab.onRename; TaskPage:404 → renameSession.mutate |
| useRenameSession | PATCH /api/sessions/{id} | patch\<TermSession\> | ✓ WIRED | sessions.ts:145-146 |
| AgentTab Insert-review-prompt item | pasteApiRef.current.paste(seed) | onSelect manual paste | ✓ WIRED | AgentTab.tsx:170 (sole paste(seed) call site) |
| AgentTab headerMenu prop | TerminalPane header slot | headerMenu supersedes showStop | ✓ WIRED | AgentTab:249 → TerminalPane:318 |
| ActiveSessionsBar mousedown listener | collapse() helper | ref-guarded outside-click → collapse() | ✓ WIRED | ActiveSessionsBar.tsx:63-64 |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| -------- | ------------- | ------ | ------------------ | ------ |
| TaskTabs rename editor | tab.label | server TermSession.label via useSessions → SetLabel/persist round-trip | Yes (server returns updated Info; optimistic cache write) | ✓ FLOWING |
| ActiveSessionsBar counts | live/working/waiting/idle | useAgentStatuses 5s poll (existing hook) | Yes (real poll data, unchanged) | ✓ FLOWING |
| AgentTab menu seed item | seed prop | derived upstream in TaskPage (PR-review interpolation) | Yes (undefined for non-PR; real seed for PR tasks) | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| Backend compiles | `go build ./...` | exit 0 | ✓ PASS |
| Backend vets clean | `go vet ./internal/session/ ./internal/api/` | exit 0 | ✓ PASS |
| Rename tests pass | `go test ./internal/api/ -run 'Rename\|ReconcileNeverShowsBash' -count=1` | 4/4 PASS | ✓ PASS |
| Full backend suite | `go test ./internal/... -count=1` | all packages ok (incl. flaky tmux reattach passed) | ✓ PASS |
| Frontend builds | `cd web && npm run build` | exit 0 (chunk-size warning only) | ✓ PASS |
| QuickAdd removed | `grep -rc QuickAdd web/src` | 0 | ✓ PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| ----------- | ----------- | ----------- | ------ | -------- |
| TABS-01 | 24-01, 24-04 | Rename bash/tmux tab; persists across reopen + restart | ✓ SATISFIED | Backend SetLabel + PATCH + tmux persist; frontend editor + mutation; restart survival unit-tested (needs human runtime confirm — see Truth 1) |
| TABS-02 | 24-01, 24-04 | New tabs keep Bash N default until renamed | ✓ SATISFIED | INSERT writes Bash N (sessions.go:314); defaultTmuxLabel re-derive; sentinel gone |
| REVMENU-01 | 24-03 | Stop button replaced by ⋯ menu with Stop + Insert review prompt | ✓ SATISFIED | headerMenu supersedes Stop; menu items built in AgentTab |
| REVMENU-02 | 24-03 | PR review prompt no longer auto-inserted; only via menu | ✓ SATISFIED | seededSessionIds + connect-time seed removed; sole paste in menu onSelect |
| POLISH-01 | 24-02 | Active-sessions bar drops total count | ✓ SATISFIED | total render span removed; three CountGroups remain |
| POLISH-02 | 24-02 | Active-sessions bar auto-collapses on outside click | ✓ SATISFIED | document mousedown → collapse() with ref-contains guard |
| POLISH-03 | 24-02 | To Do column no longer shows + New task shortcut | ✓ SATISFIED | QuickAdd deleted, zero refs, Column render removed |

All 7 phase requirement IDs declared in PLAN frontmatter (24-01: TABS-01/02; 24-02: POLISH-01/02/03; 24-03: REVMENU-01/02; 24-04: TABS-01/02) are accounted for and satisfied. REQUIREMENTS.md maps exactly these 7 to Phase 24 (lines 123-129, 141) — no orphaned requirements.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| (none) | — | No TBD/FIXME/XXX/TODO/HACK/PLACEHOLDER in any phase-24 file | ℹ️ Info | Clean |
| web/src/** | — | ESLint 21 errors (react-hooks/set-state-in-effect, react-refresh) | ℹ️ Info | **PRE-EXISTING, NOT a phase-24 regression** — independently confirmed: pre-phase-24 source (`c342276~1`) produces the identical 21-error count; the only per-file diff is line-number shifts in TerminalPane.tsx/TaskPage.tsx from added `headerMenu`/`onRename` lines. Already logged in deferred-items.md. `npm run lint` exit 1 is a codebase-wide config issue independent of this phase |
| web/src/components/board/Column.tsx | 15, 44 | `topSlot` prop now dead (no caller) | ℹ️ Info | Review IN-01; harmless unused extension point. QuickAdd (the POLISH-03 target) is fully removed — goal unaffected |
| web/src/components/terminal/TerminalPane.tsx | 24, 314 | `headerActions` prop now unused | ℹ️ Info | Review IN-02; harmless dead prop. Goal unaffected |

**Review cross-reference:** 24-REVIEW.md found 0 critical / 4 warnings / 6 info. All 4 warnings (WR-01 empty-body 400 divergence, WR-02 Stop-item no in-flight guard, WR-03 rename-input-after-exit silent drop, WR-04 non-tmux empty-reset no-op) are correctness/robustness edge cases that do NOT block any of the 5 goal truths — each is an edge-case UX rough edge, not a missing capability. None prevent goal achievement.

### Human Verification Required

The code is structurally complete and all supporting artifacts and links are verified. The following six user-facing behaviors are UI/runtime flows that require a running app to confirm observationally (see frontmatter `human_verification` for full detail):

1. **Rename persistence across reopen** — double-click a bash tab, rename, reopen the task; label persists.
2. **TMUX rename survives server restart** — restart the server; the custom tmux label persists and never shows "Bash ?".
3. **Empty-commit reset** — clearing the rename resets a tmux tab to Bash N; new tabs default to Bash N.
4. **Agent ⋯ menu appearance** — the agent header shows the ⋯ menu (Stop + Insert review prompt + Insert description); bash tabs keep inline Stop.
5. **PR review no-auto-paste + manual insert** — opening a PR review leaves the prompt empty; Insert review prompt pastes it (bracketed, un-sent).
6. **Outside-click auto-collapse** — clicking outside the expanded bar collapses it; clicking the bar itself still toggles.

### Gaps Summary

No gaps. All 5 success criteria and all 7 requirement IDs are satisfied in the actual codebase. Backend builds, vets, and passes its full test suite (including the four new rename tests and the flaky tmux reattach test on this run). Frontend builds green. The pre-existing `npm run lint` failure (21 errors) is independently confirmed to predate phase 24 by an identical count on the pre-phase-24 source tree — it is a codebase-wide lint-config issue already logged as a deferred item, not a phase-24 defect. The phase goal is achieved in code; six runtime UI behaviors remain for human confirmation before final sign-off.

---

_Verified: 2026-07-03T20:05:00Z_
_Verifier: Claude (gsd-verifier)_

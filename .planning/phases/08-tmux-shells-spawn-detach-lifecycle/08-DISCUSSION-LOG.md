# Phase 8: tmux Shells — Spawn & Detach Lifecycle - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-06-12
**Phase:** 08-tmux-shells-spawn-detach-lifecycle
**Areas discussed:** Detached-session visibility, Close vs Kill UX, Tab labels & tmux identity, Mixed-shell transitions, tmux leakage (status bar / scrollback / prefix), Done-task session lifecycle

---

## ⚠ Mid-discussion pivot

The first three areas were initially answered under a "close = detach, detached tabs visible in the strip" model. During the Tab labels area the user corrected the model: **× kills the session; detach only happens when leaving the task view; tmux must be invisible — its purpose is surviving a Kangent restart.** The user then chose to **roll back all** earlier UI picks. Superseded answers are kept below, struck through, for the audit trail. This pivot amended TMUX-03/TMUX-04 in REQUIREMENTS.md and ROADMAP.md.

---

## Detached-session visibility (answers superseded by pivot)

| Option | Description | Selected |
|--------|-------------|----------|
| Tab stays, marked detached | Tab persists with detached indicator; click reattaches | ~~✓~~ superseded |
| Tab disappears, returns on task reopen | Tabs mirror live tmux sessions on task load | |
| Tab disappears, reattach via + menu | + offers new session + detached list | |

Follow-ups (all superseded except where noted):
- Reattach on click: ~~immediately (no intermediate screen)~~
- Outside-task-view visibility: **Task view only — board untouched** (still holds, D-85)
- Stale detached session (died externally): **honest exited banner, never silent removal** (survives the pivot in spirit — D-82)
- Tab ordering: ~~keep original position~~ (moot — no detached tabs exist)

## Close vs Kill UX (answers superseded by pivot)

| Option | Description | Selected |
|--------|-------------|----------|
| Tab context menu | Right-click → Detach / Kill session | ~~✓~~ superseded |
| In-terminal toolbar action | Kill control in terminal chrome | |
| Both | | |

- Kill confirmation: ~~lightweight confirm dialog~~ → superseded: × kills instantly, bash parity, no confirm.

## Tab labels & tmux identity

| Option | Description | Selected |
|--------|-------------|----------|
| Bash N + tmux badge | Familiar labels + small tmux indicator | ~~✓~~ superseded |
| Indistinguishable (plain Bash N) | tmux invisible | ✓ (via rollback) |
| Show tmux session name | kangent-<task>-n as label | |

- Detached visual marker: ~~subtle status dot~~ (moot)
- Close glyph question triggered the pivot. **User (verbatim):** "if the user close the bash tmux tab then this should kill the session, not detach it. We only detach when we close the task view so we are working on another task. The main purpose of using tmux is that the session survive a kangent restart, the user should not be much aware that is a tmux session"
- Rollback question: **Roll back all** (no badge, no detached states, no menu, no confirm) ✓
- Requirements update: **Update REQUIREMENTS.md/ROADMAP.md now** (not CONTEXT-only) ✓

## Mixed-shell transitions

| Option | Description | Selected |
|--------|-------------|----------|
| Applies at next spawn | Existing sessions untouched; mixed tabs OK (Phase 6 precedent) | ✓ |
| Warn about live sessions | Settings-page note | |

| Option | Description | Selected |
|--------|-------------|----------|
| Honest spawn error | tmux gone from PATH → clear error in tab | ✓ |
| Silent fallback to bash | | |

## tmux leakage (second round, post-pivot)

| Question | Selected |
|----------|----------|
| Status bar on kangent socket | **Hide it** (`status off`) |
| Scrollback history | **tmux mouse mode** (`set -g mouse on`) — wheel scrolls tmux's real history; accepted copy-on-select caveat |
| Ctrl+B prefix | **Leave default** — power users keep tmux features |

Note: these reverse the REQUIREMENTS.md Out of Scope row "Injecting tmux config" — row amended in the same commit.

## Done-task session lifecycle (user-raised, third round)

- Initial question (what happens to tmux sessions on move-to-Done) was answered with a new requirement. **User (verbatim):** "We don't want to accumulate tmux session, neither bash session, we need to kill them after some time at done. By default (this should be configurable at global settings) after one day at done they should be killed, both tmux and bash sessions."

| Question | Selected |
|----------|----------|
| Reaper placement & agent coverage | **Phase 9, include agent sessions** |
| Timer semantics | **TTL from entering Done** (default 24h, global setting, 0/never disables, leaving Done cancels) |
| Also auto-clean worktrees on TTL? | **No — sessions only**; worktree cleanup stays manual |

→ Recorded as REAP-01 (new requirement, Phase 9) + D-86/D-87.

## Claude's Discretion

- detach-client vs keep-attach-PTY on task-view close (user-facing contract fixed; mechanism free)
- Config injection mechanism (set-option vs generated -f file)
- Error/exited copy; exit-vs-detach wiring; TMUX-07 regression-test shape

## Deferred Ideas

- REAP-01 ships in Phase 9 (decided here, not Phase 8 scope)
- Worktree auto-clean on TTL — declined; MAINT-01 remains the parked vehicle
- TMUX-FUT-01 — user's own tmux server (already parked)

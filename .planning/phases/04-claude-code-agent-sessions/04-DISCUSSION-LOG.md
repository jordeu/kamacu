# Phase 4: Claude Code Agent Sessions - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-06-10
**Phase:** 4-claude-code-agent-sessions
**Areas discussed:** Start & initial prompt, Agent tab & lifecycle, Status badges & attention, Claude spawn config, Board-level attention, Insert button details, Task-view status echo

---

## Start & Initial Prompt

| Option | Selected |
|--------|----------|
| Auto-send as first prompt (Recommended) | |
| One-click insert | ✓ |
| Start clean | |

| Option | Selected |
|--------|----------|
| Just start clean when no description (Recommended) | ✓ |
| Send the title | |

## Agent Tab & Lifecycle

| Option | Selected |
|--------|----------|
| Permanent first tab (Recommended) | ✓ |
| Appears on Start | |

| Option | Selected |
|--------|----------|
| Stop button + Start again, fresh session (Recommended) | ✓ |
| Restart = --continue | |

| Option | Selected |
|--------|----------|
| Agent counts in cleanup gate like bash (Recommended) | ✓ |
| Agent blocks cleanup | |

| Option | Selected |
|--------|----------|
| Tab order: Agent, Description, Bash... (Recommended) | ✓ |
| Description, Agent, Bash... | |

| Option | Selected |
|--------|----------|
| Smart default tab (Recommended) | |
| Always first tab | ✓ |

| Option | Selected |
|--------|----------|
| Start disabled with reason when no worktree (Recommended) | ✓ |
| Start creates worktree | |

## Status Badges & Attention

| Option | Selected |
|--------|----------|
| Colored dot + label (Recommended) | |
| Dot only | ✓ |
| Text only | |

| Option | Selected |
|--------|----------|
| Badge + card elevation on waiting (Recommended) | ✓ |
| Badge only | |

| Option | Selected |
|--------|----------|
| Hooks + output activity (Recommended) | ✓ |
| Output activity only | |

| Option | Selected |
|--------|----------|
| Agent only drives card badge (Recommended) | ✓ |
| Any session | |

| Option | Selected |
|--------|----------|
| Waiting clears on attach (Recommended) | ✓ |
| On input | |

| Option | Selected |
|--------|----------|
| Exited: gray ok / red error (Recommended) | ✓ |
| Single exited style | |

| Option | Selected |
|--------|----------|
| Stop hook → idle (Recommended) | ✓ |
| Distinct 'done' state | |

## Claude Spawn Config

| Option | Selected |
|--------|----------|
| Default interactive (Recommended) | ✓ |
| Per-task mode picker | |

| Option | Selected |
|--------|----------|
| Inherit everything (Recommended) | ✓ |
| Isolated profile | |

| Option | Selected |
|--------|----------|
| Hooks invisible & additive (Recommended) | ✓ |
| Custom constraints | |

## Additional Areas (second round)

| Option | Selected |
|--------|----------|
| Sidebar waiting count per project (Recommended) | ✓ |
| Dot, no number | |
| Nothing in sidebar | |

| Option | Selected |
|--------|----------|
| Insert button in agent tab header, always available (Recommended) | ✓ |
| Only at start | |

| Option | Selected |
|--------|----------|
| Status dot on the Agent tab label (Recommended) | ✓ |
| Header status text | |
| No echo | |

(Concurrent-agents area offered but not selected — unlimited, no global view.)

## Claude's Discretion

- Alt-screen reattach strategy, hook payload/session-ID mechanics, dot styling (UI-SPEC), status transport (polling vs push), idle threshold, agent session labeling

## Deferred Ideas

- Browser notifications (v2 NOTF-01), move-to-review suggestion (v2 NOTF-02, never automatic), --continue/--resume (Phase 5), permission-mode picker

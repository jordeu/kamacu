# Phase 5: Recovery & Review - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-06-11
**Phase:** 5-recovery-review
**Areas discussed:** Resume UX, Diff tab content, Post-restart experience

---

## Resume UX

| Option | Selected |
|--------|----------|
| Both exit & restart (Recommended) | ✓ — user added the UUID-first workflow note: Kangent mints the UUID, spawns `claude --session-id <uuid>`, stores in SQLite, resumes with `claude --resume <uuid>`; no output parsing (confirmed already implemented in Phase 4) |
| Restart only | |

| Option | Selected |
|--------|----------|
| claude --resume <id> (Recommended) | ✓ |
| claude --continue | |

| Option | Selected |
|--------|----------|
| Show error, offer Reset (Recommended) | ✓ |
| Auto-fallback to fresh | |

| Option | Selected |
|--------|----------|
| Muted gray exited dot after restart (Recommended) | ✓ |
| Distinct resumable hint | |

| Option | Selected |
|--------|----------|
| Bash tabs vanish quietly after restart (Recommended) | ✓ |
| Exited stubs | |

## Diff Tab Content

| Option | Selected |
|--------|----------|
| Everything vs base (Recommended) | ✓ |
| Committed only | |
| Two sections | |

| Option | Selected |
|--------|----------|
| File list + unified (Recommended) | ✓ |
| Side-by-side | |

| Option | Selected |
|--------|----------|
| On open + manual refresh (Recommended) | ✓ |
| Auto-refresh | |

| Option | Selected |
|--------|----------|
| After Description: Agent, Description, Diff, Bash... (Recommended) | ✓ |
| Last, before + | |

| Option | Selected |
|--------|----------|
| Collapse big files >400 lines (Recommended) | ✓ |
| Render everything | |

| Option | Selected |
|--------|----------|
| Quiet empty state (Recommended) | ✓ |
| Hide the tab | |

| Option | Selected |
|--------|----------|
| Hidden without worktree (Recommended) | |
| Disabled like bash + | ✓ |

| Option | Selected |
|--------|----------|
| Untracked: full content as added (Recommended) | ✓ |
| Name + stats only | |

| Option | Selected |
|--------|----------|
| Totals bar header (Recommended) | ✓ |
| No header | |

| Option | Selected |
|--------|----------|
| Merge-base / three-dot (Recommended) | ✓ |
| Current base tip | |

## Post-Restart Experience

| Option | Selected |
|--------|----------|
| Silent reconciliation (Recommended) | ✓ |
| Restart notice | |

| Option | Selected |
|--------|----------|
| Pre-start + Resume on agent tab (Recommended) | ✓ |
| Other | |

| Option | Selected |
|--------|----------|
| Task-level persistence only (Recommended) | ✓ |
| Full session log | |

## Claude's Discretion

- End-state detection mechanics, diff plumbing/parsing/rendering split, resumable pre-start copy, /terminal route fate, carried plan-mode-amber UAT check

## Deferred Ideas

- Sessions audit table, restart notice banner, diff comments / agent feedback interactions

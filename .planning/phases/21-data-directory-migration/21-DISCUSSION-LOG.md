# Phase 21: Data Directory Migration - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-07-01
**Phase:** 21-data-directory-migration
**Areas discussed:** Failure-safety model, Live tmux sessions, Trigger & error UX, DB name & flags

---

## Area selection

| Area | Description | Selected |
|------|-------------|----------|
| Failure-safety model | Copy-vs-move, failure behavior, old-dir disposal | ✓ |
| Live tmux sessions | Reconcile running kangent-* agents at upgrade time | ✓ |
| Trigger & error UX | When migration runs + how failure is surfaced | ✓ |
| DB name & flags | DB/config file rename, --db default, custom-path gating | ✓ |

**User's choice:** All four areas.

---

## Failure-safety model

### Move strategy

| Option | Description | Selected |
|--------|-------------|----------|
| Rename, then repair | os.Rename (atomic same-FS, no 2× disk), then repair + DB rewrite; reversible | ✓ |
| Copy, verify, then delete | Copy tree, verify, rewrite copy, delete original last; literal "untouched" but 2× disk | |
| You decide (research it) | Let researcher/planner weigh it | |

**User's choice:** Rename, then repair.
**Notes:** Chosen because managed `repos/` + `worktrees/` can be many GB, making transient 2× disk a real risk. Rename is instant + atomic on the same filesystem.

### Recovery on post-rename failure

| Option | Description | Selected |
|--------|-------------|----------|
| Roll forward on next boot | Leave at ~/.kamacu, refuse to serve, idempotent repair re-runs next boot; preflight before rename | ✓ |
| Roll back to ~/.kangent | Rename back to restore prior state on failure | |

**User's choice:** Roll forward on next boot.
**Notes:** Preflight checks (disk/permissions/same-filesystem) run BEFORE the rename so pre-commit failures leave ~/.kangent untouched. Rename is the commit point; roll-back rejected because the rename-back could itself fail and partial DB rewrites would need undoing.

---

## Live tmux sessions

| Option | Description | Selected |
|--------|-------------|----------|
| Retire + resume on reopen | Kill live kangent-* sessions; agents come back via `claude --resume`; no orphaned process | ✓ |
| Block until idle | Refuse to migrate while any live session exists | |
| Keep old socket alive | Leave -L kangent sessions running alongside -L kamacu | |

**User's choice:** Retire + resume on reopen.
**Notes:** Flagged that the rename moves worktree CWDs, so keeping old sessions alive points shells at deleted paths (mostly broken). "Retire + resume" matches the existing Phase 5 recovery path. Bash shells respawn fresh (scrollback loss) — accepted as unavoidable given tmux can't move sessions between sockets.

---

## Trigger & error UX

| Option | Description | Selected |
|--------|-------------|----------|
| Refuse to boot + log | Startup one-shot; on failure exit non-zero with clear slog error (terminal-visible) | ✓ |
| Degraded error page | Server comes up in a "migration failed" mode; browser shows explanation | |
| Both | Refuse to fully serve but expose minimal status to the browser | |

**User's choice:** Refuse to boot + log.
**Notes:** kamacu is launched from a terminal (bin/kamacu), so the error line is right there. Success is silent (single info log, no UI toast) — captured as a default consistent with the minimal path chosen throughout.

---

## DB name & flags

### DB / config filename

| Option | Description | Selected |
|--------|-------------|----------|
| Rename to kamacu.* | kangent.db → kamacu.db, kangent-tmux.conf → kamacu-tmux.conf, flip --db default | ✓ |
| Keep old filenames | Move dir only; leave ~/.kamacu/kangent.db etc. | |

**User's choice:** Rename to kamacu.*
**Notes:** Full brand consistency down to filenames; matches the point of the rebrand.

### Trigger gate

| Option | Description | Selected |
|--------|-------------|----------|
| Default paths only | Migrate iff ~/.kangent exists, ~/.kamacu absent, no custom --db override | ✓ |
| Any ~/.kangent present | Attempt whenever ~/.kangent exists, regardless of flags | |

**User's choice:** Default paths only.
**Notes:** A custom --db / custom data path opts out entirely so a bespoke or side-by-side layout is never touched. Primary safety gate alongside the preflight checks.

---

## Claude's Discretion

- localStorage migration mechanics (MIGRATE-04) — captured as a sensible default (one-time client-side copy of `kangent.*` → `kamacu.*`, guarded/idempotent, generic over the prefix). Not put to a full question round (low ambiguity).
- Path-rewrite scope (MIGRATE-02) — captured as a default: rewrite only DB paths under the old data root; leave external/unmanaged checkouts untouched.
- Exact preflight check set + error wording; new-package vs existing-startup-path placement; file-rename ordering vs `git worktree repair`; old `-L kangent` server kill mechanics; roll-forward-state detection on reboot.

## Deferred Ideas

- Degraded browser error page for migration failure — rejected for this phase.
- Permanent dual-path support (`~/.kangent` + `~/.kamacu`) — out of scope per REQUIREMENTS.md.
- Standalone refactor to centralize the data-dir path into one constant beyond what MIGRATE needs.

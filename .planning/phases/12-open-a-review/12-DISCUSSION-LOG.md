# Phase 12: Open-a-Review - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-06-14
**Phase:** 12-open-a-review
**Areas discussed:** Default tab on open, Agent session in a review, Review header & identity, Description tab content, Review-seed prompt

---

## Area selection

| Option | Description | Selected |
|--------|-------------|----------|
| Default tab on open | Diff-first vs Agent-first for a review | ✓ |
| Agent session in a review | Explicit Start / auto-start; plain vs review-seeded | ✓ |
| Review header & identity | Read-only PR title + metadata vs mirror task header | ✓ |
| Description tab content | PR body read-only vs editable vs drop | ✓ |

**User's choice:** All four areas.

---

## Default tab on open

| Option | Description | Selected |
|--------|-------------|----------|
| Diff first (Recommended) | Land on the Diff tab — you opened it to review changes; override task default (D-39) for source='github_pr' | |
| Agent first | Stay consistent with tasks — always land on Agent | ✓ |

**User's choice:** Agent first.
**Notes:** Prioritized consistency with the task view over diff-first. The Diff tab is still immediately available (worktree exists from open). → CONTEXT D-04.

---

## Agent session in a review

| Option | Description | Selected |
|--------|-------------|----------|
| Explicit Start, plain session (Recommended) | Start button spawns a blank claude in the PR worktree | |
| Explicit Start, review-seeded | Start button, but prefill a suggested 'review this PR' prompt | ✓ |
| Auto-start on open | Spawn claude immediately when the review opens | |

**User's choice:** Explicit Start, review-seeded.
**Notes:** Keeps the Phase-4 explicit-Start pattern but adds a review-oriented seed. → CONTEXT D-05, D-06.

---

## Review header & identity

| Option | Description | Selected |
|--------|-------------|----------|
| Read-only PR identity (Recommended) | Non-editable PR title; meta line #num · @author · base · ↗ GitHub; ⋯ menu without 'Delete task' | ✓ |
| Mirror the task header as-is | Editable title, standard ⋯ menu | |

**User's choice:** Read-only PR identity.
**Notes:** PR title mirrors GitHub (must not drift); no 'Delete task' (not a board task), ⋯ menu omitted this phase (cleanup is Phase 13). → CONTEXT D-08, D-09, D-10.

---

## Description tab content

| Option | Description | Selected |
|--------|-------------|----------|
| PR body, read-only (Recommended) | Render the PR's GitHub description (markdown) read-only | ✓ |
| Editable scratch notes | Keep it editable like a task description | |
| Drop the tab for reviews | Only Agent / Diff / bash tabs | |

**User's choice:** PR body, read-only.
**Notes:** Review context; mirrors GitHub, can't drift. → CONTEXT D-11.

---

## Review-seed prompt (follow-up on the agent-session choice)

### Seed delivery

| Option | Description | Selected |
|--------|-------------|----------|
| Prefill, user sends (Recommended) | Start drops the seed into the input, ready to edit; user presses Enter | ✓ |
| Auto-send on Start | Start immediately submits the seed prompt | |

**User's choice:** Prefill, user sends. → CONTEXT D-06.

### Seed text

| Option | Description | Selected |
|--------|-------------|----------|
| Fixed Kangent template + PR ref (Recommended) | Canned 'Review PR #<n> "<title>". Summarize the changes, then flag bugs, risky changes, and missing tests.' | ✓ |
| Just a minimal nudge | Tiny 'Please review this PR.' | |
| Template references the diff/base | Richer template naming the base branch / diff | |

**User's choice:** Fixed Kangent template + PR ref. → CONTEXT D-07.

## Claude's Discretion

- PR worktree on-disk naming; review route shape (reuse task route vs distinct reviews route); worktree-meta-vs-PR-meta line; exact seed wording and empty-PR-body copy; provisioning loading UX.

## Deferred Ideas

- Auto-cleanup + gated manual cleanup of PR worktrees (GHCLN-01/02/03 → Phase 13).
- In-app GitHub writes (reviews/comments) — not in v1.3.
- Configurable / richer review-seed prompts (per-project templates, base-aware) — later polish.

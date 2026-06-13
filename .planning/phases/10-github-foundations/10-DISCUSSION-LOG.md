# Phase 10: GitHub Foundations - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-06-13
**Phase:** 10-github-foundations
**Areas discussed:** Project config surface, Repo linking UX, Link validation behavior, Global toggle UX, Description display

---

## Gray-area selection

All four offered gray areas were selected for discussion: Project config surface, Repo linking UX, Link validation behavior, Global toggle UX.

---

## Project config surface

| Option | Description | Selected |
|--------|-------------|----------|
| Dialog from the ⋯ menu | 'Project settings' dialog from the existing ⋯ dropdown, mirrors RenameProjectDialog; description + repo link in one place | ✓ |
| Dedicated full-page route | Per-project settings page like /settings; more room, heavier | |
| Inline panel on the board | Expandable config strip atop the board | |

**User's choice:** Dialog from the ⋯ menu (recommended).
**Notes:** Smallest, most consistent with existing Rename/Delete patterns.

---

## Repo linking UX

| Option | Description | Selected |
|--------|-------------|----------|
| Auto-detect from git origin, editable | Prefill from the project's local `origin` remote; user can edit/override/clear; accept owner/name or URL, canonicalize | ✓ |
| Manual entry only | User types repo; accept owner/name or URL; no auto-detect | |

**User's choice:** Auto-detect from git origin, editable (recommended).
**Notes:** Projects already point at a checkout, so the common case becomes "confirm, not type."

---

## Link validation behavior

| Option | Description | Selected |
|--------|-------------|----------|
| Soft-save with a warning | Accept syntactically valid owner/name; if gh can't verify, save anyway + non-blocking note; gh absent → save syntactically | ✓ |
| Hard-block unless verified | When gh present, refuse to save what `gh repo view` can't confirm | |

**User's choice:** Soft-save with a warning (recommended).
**Notes:** Aligns with degrade-don't-break; `gh auth status` exit codes are unreliable (cli/cli#8845); the Review column shows its own degraded state later.

---

## Global toggle UX

| Option | Description | Selected |
|--------|-------------|----------|
| Switch in a new 'GitHub' section | Real on/off Switch (adds shadcn switch); off hides ALL GitHub UI app-wide | ✓ |
| Reuse the select control (on/off) | on/off select via existing SettingsField; same hide behavior | |

**User's choice:** Switch in a new 'GitHub' section (recommended).
**Notes:** A switch reads as on/off better than a dropdown; small component add.

---

## Description display (follow-up)

| Option | Description | Selected |
|--------|-------------|----------|
| Board header | Under the project name atop the board | |
| Sidebar under project name | Truncated under the sidebar entry, full text on hover | |
| Only in the config dialog for now | Store/edit only; no display surface yet | ✓ |

**User's choice:** Only in the config dialog for now.
**Notes:** Keeps Phase 10 leanest; a display surface can be added later. Logged as a deferred idea.

---

## Claude's Discretion

- Switch integration approach (extend SettingsField vs dedicated field).
- Description input affordance + soft length cap.
- Origin auto-detect delivery mechanism (no git shell-out on every project list).
- "Couldn't verify" warning copy/placement; URL canonicalization details.

## Deferred Ideas

- Surfacing the project description outside the config dialog (board header / sidebar).

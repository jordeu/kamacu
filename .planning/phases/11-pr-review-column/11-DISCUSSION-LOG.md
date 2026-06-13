# Phase 11: PR Review Column - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-06-13
**Phase:** 11-pr-review-column
**Areas discussed:** Card richness, Collapse persistence, Checks pill, Click behavior

---

## Area selection

| Option | Description | Selected |
|--------|-------------|----------|
| Card richness | Minimal GHCOL-03 vs pull cheap deferred richness forward | ✓ |
| Collapse persistence | per-project vs global; localStorage vs SQLite; default state | ✓ |
| Checks pill | dot vs labeled pill; color mapping; "no checks" handling | ✓ |
| Click behavior | inert vs github.com link, pre-Phase-12 | ✓ |

**User's choice:** All four areas.

---

## Card richness

| Option | Description | Selected |
|--------|-------------|----------|
| Minimal + diffstat + fork | GHCOL-03 set + the two highest-signal zero-cost extras (+/− and fork pill) | |
| Strict minimal (GHCOL-03) | Only #, title, author, updated, checks pill; all richness deferred to GHCARD | ✓ |
| All cheap richness | Minimal + diffstat + fork + head→base — research's full Phase B card | |

**User's choice:** Strict minimal (GHCOL-03).
**Notes:** Diffstat (+/−), fork pill, and head→base stay as Future Requirements (GHCARD-01/02/03) despite their JSON being free. The rich `--json` set is still fetched (Phase 12/13 consume `headRefName`/`headRefOid`/`baseRefName`/`isCrossRepository`); only the deferred fields are not rendered. → D-01, D-02.

---

## Collapse persistence

| Option | Description | Selected |
|--------|-------------|----------|
| localStorage, per-project, default expanded | Frontend-only, no migration/endpoint; per-project key; starts expanded | ✓ |
| localStorage, global, default expanded | One collapse pref shared across all projects | |
| SQLite settings, per-project | Backend-persisted, cross-browser; costs a KV/endpoint | |

**User's choice:** localStorage, per-project, default expanded.
**Notes:** Honors Phase 10's "no further migration through Phase 12" and the single-user-localhost model. Collapsed column shows a live PR-count badge. → D-05, D-06, D-07.

---

## Checks pill

| Option | Description | Selected |
|--------|-------------|----------|
| Small colored dot, "none" omitted | green=pass/red=fail/amber=pending; no dot when no checks (matches StatusDot) | ✓ |
| Labeled pill (✓/✗/• + text) | small pill with icon + "passing/failing/pending"; more explicit, new visual | |
| Colored dot, "none" shown muted | same dot but a muted grey dot when no checks (consistent status slot) | |

**User's choice:** Small colored dot, "none" omitted.
**Notes:** Reuses the card's existing dot visual language; the "no checks" case renders nothing and must not reserve a gutter / shift layout (dotless-card rule). → D-03, D-04.

---

## Click behavior

| Option | Description | Selected |
|--------|-------------|----------|
| Inert body + ↗ to GitHub | Card body does nothing (reserved for Phase 12); small ↗ icon opens the PR on github.com via `url` | ✓ |
| Fully inert until Phase 12 | Entirely non-interactive until Phase 12 wires the worktree-open | |
| Whole card → github.com | Whole-card click opens github.com now; Phase 12 repurposes it (behavior change mid-milestone) | |

**User's choice:** Inert body + ↗ to GitHub.
**Notes:** Keeps the whole-card click reserved for Phase 12's worktree-open (nothing to unlearn) while giving the shipped Phase 11 immediate utility via an external-link icon. → D-08, D-09.

---

## Claude's Discretion

- **Poll cadence (D-10/D-11):** User accepted the default — mirror the Phase 7 quota pattern (60s `refetchInterval`, `refetchIntervalInBackground:false`, server-side per-repo cache TTL + backoff, only the active project's column polls). Declined a separate poll-cadence discussion.
- Degraded/empty/loading copy & placement (D-12); column min-width/header/badge styling (D-13); `internal/github.Service` cache shape + `PRSummary` field set; checks-dot component reuse vs sibling.

## Deferred Ideas

- Diffstat / fork pill / head→base (GHCARD-01/02/03), reviewDecision/labels/comments (GHCARD-04), team-request & draft filters (GHFILT), cross-project inbox & author view (GHWIDE), SQLite/cross-browser collapse persistence. All out of Phase 11 scope — see CONTEXT.md `<deferred>`.

# Requirements: Kamacu

**Defined:** 2026-07-29
**Core Value:** One place to see and drive all agent work: every task gets its own isolated worktree and a persistent agent session you can open, leave, and reattach to from the browser.

## v1.12 Requirements

Requirements for v1.12 — Activity & Statistics. Each maps to roadmap phases.

### Activity Page & Controls (ACT)

- [x] **ACT-01**: User can open a dedicated top-level Activity page from a persistent sidebar entry (like Settings), accessible app-wide
- [x] **ACT-02**: User can scope the Activity page to Global / a specific Workspace / a specific Project via a selector; the lists and stats update to that scope
- [x] **ACT-03**: User can toggle the time window between Week (last 7 days) and Month (last 30 days), rolling from now; the toggle drives both the lists and the statistics
- [x] **ACT-04**: When GitHub integration is off (or `gh` absent), the reviews section quietly degrades to empty while tasks-done and task stats still work

### Tasks Done (TASKS)

- [x] **TASKS-01**: User can see a list of tasks completed (moved to Done) within the selected window and scope
- [x] **TASKS-02**: Tasks-done entries are grouped by project, showing the task title and when it was completed
- [x] **TASKS-03**: Clicking a tasks-done entry navigates to that task's view

### Reviews Done (REVIEWS)

- [x] **REVIEWS-01**: User can see a list of PRs they reviewed (`reviewed-by:@me`) that MERGED or CLOSED within the selected window and scope, with the merge/close date
- [x] **REVIEWS-02**: Reviews-done entries are grouped by project, showing the PR number, title, and merge/close date
- [x] **REVIEWS-03**: Clicking a reviews-done entry opens that PR (its review workspace if it exists, else the PR)
- [x] **REVIEWS-04**: The reviews-done list is best-effort and degrades gracefully when `gh` is unavailable or a repo's fetch fails (never blocks the rest of the page)

### Statistics (STATS)

- [x] **STATS-01**: User can see a count of tasks done and a count of reviews done within the selected window and scope
- [x] **STATS-02**: User can see min / max / median cycle time (In Progress → Done) for tasks done in the window
- [x] **STATS-03**: User can see min / max / median dwell time per column (time in In Progress, and time in In Review) for tasks done in the window
- [x] **STATS-04**: Cycle/dwell time stats cover tasks only; reviews contribute a count, not time stats (GitHub's merge signal has no Kamacu in-progress/in-review timestamps)

## Future Requirements

Deferred to a later release. Tracked but not in the current roadmap.

### Activity Enhancements

- **ACTFUT-01**: Custom date ranges + an "All time" preset beyond the Week/Month toggle
- **ACTFUT-02**: Richer visualizations — throughput trend charts, bar graphs of tasks/reviews done over time
- **ACTFUT-03**: CSV/JSON export of the activity lists and statistics
- **ACTFUT-04**: Per-agent or per-workspace breakdown statistics
- **ACTFUT-05**: Activity for events beyond done/merged (e.g. tasks moved to In Review, sessions started)

## Out of Scope

Explicitly excluded. Documented to prevent scope creep.

| Feature | Reason |
|---------|--------|
| Custom date ranges / date pickers | Week/Month presets are enough for v1.12; custom ranges deferred (ACTFUT-01) |
| Charts/graphs beyond numeric readouts | min/max/median shown as numbers; richer viz deferred (ACTFUT-02) |
| Time stats for reviews | GitHub's merge/close signal has no Kamacu in-progress/in-review timestamps; reviews surface counts only (STATS-04) |
| CSV/JSON export of activity data | Deferred (ACTFUT-03) |
| Cross-forge review history (GitLab, Bitbucket, Gitea) | GitHub-only for the reviewed-by:@me signal, consistent with v1.3 |
| Activity for events other than done/merged | v1.12 covers completed work only; broader event feeds deferred (ACTFUT-05) |
| Multi-user / team aggregate stats | Single user at localhost only (core constraint) |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| ACT-01 | Phase 11 | Complete |
| ACT-02 | Phase 11 | Complete |
| ACT-03 | Phase 11 | Complete |
| ACT-04 | Phase 11 | Complete |
| TASKS-01 | Phase 10 | Complete |
| TASKS-02 | Phase 11 | Complete |
| TASKS-03 | Phase 11 | Complete |
| REVIEWS-01 | Phase 10 | Complete |
| REVIEWS-02 | Phase 11 | Complete |
| REVIEWS-03 | Phase 11 | Complete |
| REVIEWS-04 | Phase 10 | Complete |
| STATS-01 | Phase 10 | Complete |
| STATS-02 | Phase 10 | Complete |
| STATS-03 | Phase 10 | Complete |
| STATS-04 | Phase 10 | Complete |

**Coverage:**

- v1.12 requirements: 15 total
- Mapped to phases: 15
- Unmapped: 0 ✓

**Phase distribution:** Phase 10 (Activity Data & API): 7 — TASKS-01, REVIEWS-01, REVIEWS-04, STATS-01..04. Phase 11 (Activity Page & Controls): 8 — ACT-01..04, TASKS-02..03, REVIEWS-02..03.

---
*Requirements defined: 2026-07-29*
*Last updated: 2026-07-29 after roadmap creation (milestone v1.12 Activity & Statistics — Phases 10–11, 15/15 requirements mapped)*

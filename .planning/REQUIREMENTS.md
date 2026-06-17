# Requirements: Kangent — v1.5 Sharper Review Column

**Defined:** 2026-06-17
**Core Value:** One place to see and drive all agent work: every task gets its own isolated worktree and a persistent Claude Code session you can open, leave, and reattach to from the browser.

## v1.5 Requirements

Requirements for the Sharper Review Column milestone. Each maps to a roadmap phase. Builds entirely on shipped v1.3/v1.4 GitHub machinery — no new external surface beyond a second `gh pr list` search.

### Card Signals

The Review column must show two independent signals per PR — *your* agent's state and the PR's CI state — without the two being confused for each other.

- [x] **SIGNL-01**: When an open review session exists for a PR, its card shows the agent status dot (working / waiting / idle / exited) using the same palette and semantics as task cards (reused `StatusDot`)
- [ ] **SIGNL-02**: A PR card with an open review session is highlighted with a colored left border (the same visual language as the waiting-task card border, D-44) so open-session PRs are distinguishable at a glance
- [ ] **SIGNL-03**: The agent dot and open-session border apply to PR cards in both the "awaiting your review" and "Recently Reviewed" sections

### CI Status

- [ ] **CHECK-01**: A PR's CI status renders as an icon — green check (passing), red cross (failing), orange circle (running/pending) — replacing the previous colored CI dot, so the colored-dot vocabulary is reserved for agent state
- [ ] **CHECK-02**: A PR with no checks (`none`) renders no CI indicator (no icon, no reserved gutter, no layout shift) — preserving the existing dotless behavior

### Recently Reviewed

- [x] **REVWD-01**: A "Recently Reviewed" section appears at the bottom of the Review column, listing open PRs the user has reviewed — approve OR request-changes — via `reviewed-by:@me state:open`, rendered as the same PR cards (click to open/reattach the review)
- [x] **REVWD-02**: A reviewed PR stays in Recently Reviewed until its PR is merged or closed, then it leaves the list
- [x] **REVWD-03**: A PR appears in exactly one section — the top "awaiting your review" section takes precedence, so a re-requested PR returns to the top and is never duplicated in Recently Reviewed
- [x] **REVWD-04**: The Recently Reviewed section shares the Review column's manual refresh + visibility-paused auto-poll and reuses the same loading / degraded states; when it holds no PRs it is quietly omitted (no blank section, no fabricated count)

## Future Requirements

Acknowledged but deferred — not in the v1.5 roadmap.

### Recently Reviewed (refinements)

- **REVWD-FUT-01**: Time window or count cap on Recently Reviewed (e.g. last N days / last N PRs) if the open-reviewed set grows large for heavy reviewers
- **REVWD-FUT-02**: Review-decision badge on Recently Reviewed cards (approved vs changes-requested pill) to distinguish *how* you reviewed

### Card richness (carried from v1.3 GHCARD-*)

- **GHCARD-01..04**: Richer PR cards (diff size, fork pill, head→base line, review-decision/labels) — still deferred

## Out of Scope

Explicitly excluded for v1.5. Documented to prevent scope creep.

| Feature | Reason |
|---------|--------|
| Time-windowed Recently Reviewed | v1.5 bounds the list by open state (leaves on merge/close); a time window is a future refinement (REVWD-FUT-01) |
| In-app approve / request-changes / merge actions | Project-wide out of scope — review actions stay in the terminal (manual-git philosophy) |
| Agent dot on PRs with no open session | No session means no agent state — nothing to show by definition |
| Cross-project / author-side review surfaces (GHWIDE-*) | Out of scope; the column stays per-project, reviewer-side |
| New CI states beyond pass/fail/running/none | The server already reduces the rollup to these four; no new classification in v1.5 |

## Traceability

Which phases cover which requirements. Populated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| SIGNL-01 | Phase 16 | Complete |
| SIGNL-02 | Phase 16 | Pending |
| SIGNL-03 | Phase 16 | Pending |
| CHECK-01 | Phase 16 | Pending |
| CHECK-02 | Phase 16 | Pending |
| REVWD-01 | Phase 16 | Complete |
| REVWD-02 | Phase 16 | Complete |
| REVWD-03 | Phase 16 | Complete |
| REVWD-04 | Phase 16 | Complete |

**Coverage:**
- v1.5 requirements: 9 total
- Mapped to phases: 9 (all → Phase 16, Sharper Review Column)
- Unmapped: 0 ✓

---
*Requirements defined: 2026-06-17*
*Last updated: 2026-06-17 — roadmap created; all 9 requirements mapped to Phase 16*

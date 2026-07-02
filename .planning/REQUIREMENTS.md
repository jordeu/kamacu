# Requirements: Kamacu (v1.8 — Kamacu Rebrand & UX Polish)

**Defined:** 2026-07-01
**Core Value:** One place to see and drive all agent work: every task gets its own isolated worktree and a persistent Claude Code session you can open, leave, and reattach to from the browser.

## v1.8 Requirements

Requirements for the Kamacu Rebrand & UX Polish milestone. Each maps to exactly one roadmap phase.

### Rebrand (product identity in code + UI)

- [x] **REBRAND-01**: The product name is shown as "Kamacu" everywhere the brand appears (browser tab title, sidebar brand title, page/section headers, settings/about copy)
- [x] **REBRAND-02**: The app builds as a `kamacu` binary and the Go module path is renamed to kamacu, with all imports updated (`make build` produces `kamacu`; `go build`/`vet`/`test ./...` green)
- [x] **REBRAND-03**: Remaining code identifiers, log lines, and user-facing strings that say "kangent" are updated to "kamacu" wherever the change does not affect on-disk or runtime compatibility (path/socket compatibility is handled by the Migrate category)

### Migrate (one-time on-disk data migration)

- [x] **MIGRATE-01**: On first launch after upgrade, the app performs a one-time, gated migration of the data directory `~/.kangent` → `~/.kamacu` (managed repos, worktrees, and the SQLite DB)
- [x] **MIGRATE-02**: After the move, every task/PR git worktree stays valid — worktree links are repaired (`git worktree repair`) and the DB's stored worktree/repo paths are rewritten to the new `~/.kamacu` location
- [x] **MIGRATE-03**: The tmux socket (`-L kangent` → `-L kamacu`) and session prefix (`kangent-<task>-<n>` → `kamacu-<task>-<n>`) are switched for new sessions, and pre-existing live `kangent-*` sessions are reconciled (reattached or cleanly retired) without orphaning a running agent
- [x] **MIGRATE-04**: Browser `localStorage` keys under `kangent.*` are migrated to `kamacu.*` so sidebar state, collapse preferences, and panel settings carry over
- [x] **MIGRATE-05**: The migration is idempotent and safe — an already-migrated or fresh install is detected and skipped; a failure leaves the original `~/.kangent` untouched and surfaces a clear error instead of a half-migrated state

### Brand (logo, favicon, docs)

- [x] **BRAND-01**: The app ships a new Kamacu logo (SVG) rendered in the UI brand area
- [x] **BRAND-02**: The browser tab shows a Kamacu favicon
- [x] **BRAND-03**: The repo has a `README.md` describing what Kamacu is, how to build and run it, and its core project → task → agent → review workflow

### Diff (GitHub "Files changed"-style review)

- [x] **DIFF-01**: The diff view shows a left-hand tree of all changed files; selecting a file focuses/scrolls to its diff
- [x] **DIFF-02**: Each file's diff can be individually collapsed and expanded
- [x] **DIFF-03**: Each file has a "Viewed" checkbox; marking it viewed collapses that file, and the viewed state persists across reopening the diff view (and server restarts)
- [x] **DIFF-04**: When a previously-viewed file changes again, it is automatically reset to un-viewed and re-expanded on the next open; unchanged viewed files stay collapsed and viewed

### Worktree Cleanup (Settings panel)

- [ ] **WTREE-01**: Settings has a worktree-management section listing every cleanup-candidate worktree — orphaned, stale-pointer, or referenced-but-finished (task Done / PR merged or closed) — with its task/PR association, classification, and dirty / unpushed / stash flags. Active-work worktrees (in-progress tasks, open/pending-review PRs) are intentionally hidden so the panel reads as a cleanup queue, not a full inventory (refined post-approval, 2026-07-02; reverses the original "every worktree" wording).
- [ ] **WTREE-02**: The user can force-remove an individual worktree from the list, overriding the dirty/unpushed/stash gates, behind a confirmation
- [ ] **WTREE-03**: A bulk "clean eligible" action removes all safely-removable worktrees (done/merged-and-pristine, or orphaned) in one action
- [ ] **WTREE-04**: Orphaned worktrees (present on disk / in `git worktree list` but with no matching DB task) are detected, listed, and removable — reconciling the accumulation the reaper currently skips

### Tabs (rename bash/tmux tabs)

- [ ] **TABS-01**: The user can rename a bash/tmux tab to a custom label; the custom name persists across reopening the task and server restarts
- [ ] **TABS-02**: New tabs keep their auto-generated default names (Bash 1, Bash 2, …) until the user renames them

### Review Menu (task-view actions + review prompt)

- [ ] **REVMENU-01**: The agent view's top-right Stop button is replaced by a three-dots menu with "Stop" and "Insert review prompt" actions
- [ ] **REVMENU-02**: The PR review prompt is no longer auto-inserted into a PR-review session — it is placed into the prompt only when the user chooses "Insert review prompt"

### Polish (session bar + board)

- [ ] **POLISH-01**: The bottom active-sessions bar no longer shows the total-sessions count (per-state colored counts remain)
- [ ] **POLISH-02**: The bottom active-sessions bar auto-collapses when the user clicks outside it (e.g., clicking back into the task view)
- [ ] **POLISH-03**: The To Do column no longer shows the inline "+ New task" shortcut

## Future Requirements

Deferred to a later milestone. Tracked but not in this roadmap.

### Diff

- **DIFF-FUT-01**: "N of M files viewed" progress summary in the diff header
- **DIFF-FUT-02**: Side-by-side (split) diff view toggle
- **DIFF-FUT-03**: Per-file / per-line review comments or annotations

### Worktree Cleanup

- **WTREE-FUT-01**: Scheduled / automatic stale-worktree purge (the deferred MAINT-01), modeled on the reaper goroutine

### Tabs

- **TABS-FUT-01**: Drag-to-reorder tabs

### Brand

- **BRAND-FUT-01**: Light/dark or animated logo variants

### Carried forward from prior milestones (unchanged)

- v1.7 icon follow-ups (ICON-FUT-01..05), v1.4 managed-checkout follow-ups (CKMNT-01..03, CKUX-01), v1.3 GitHub follow-ups (GHCARD/GHFILT/GHWIDE), and the NOTF/AGNT deferred set remain parked in their archived milestone requirements and PROJECT.md "Deferred".

## Out of Scope

Explicitly excluded. Documented to prevent scope creep.

| Feature | Reason |
|---------|--------|
| Renaming the GitHub repo / git remote from kangent → kamacu | Local product rename only; the repo/remote is the user's to rename separately if desired |
| Permanent dual-path support (reading both `~/.kangent` and `~/.kamacu` indefinitely) | Migration is a one-time move, not a permanent dual-mount; keeps the runtime simple |
| Auto-inserting or auto-sending any agent prompt | The milestone's intent is manual, user-triggered prompt insertion (REVMENU-02) |
| Per-project diff-view or cleanup settings | Global behavior only for this milestone; per-project overrides stay deferred |
| Multi-user / auth / remote deployment | Unchanged core constraint — single user at localhost |

## Traceability

Which phases cover which requirements. Populated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| REBRAND-01 | Phase 20 | Complete |
| REBRAND-02 | Phase 20 | Complete |
| REBRAND-03 | Phase 20 | Complete |
| MIGRATE-01 | Phase 21 | Complete |
| MIGRATE-02 | Phase 21 | Complete |
| MIGRATE-03 | Phase 21 | Complete |
| MIGRATE-04 | Phase 21 | Complete |
| MIGRATE-05 | Phase 21 | Complete |
| BRAND-01 | Phase 20 | Complete |
| BRAND-02 | Phase 20 | Complete |
| BRAND-03 | Phase 20 | Complete |
| DIFF-01 | Phase 22 | Complete |
| DIFF-02 | Phase 22 | Complete |
| DIFF-03 | Phase 22 | Complete |
| DIFF-04 | Phase 22 | Complete |
| WTREE-01 | Phase 23 | Pending |
| WTREE-02 | Phase 23 | Pending |
| WTREE-03 | Phase 23 | Pending |
| WTREE-04 | Phase 23 | Pending |
| TABS-01 | Phase 24 | Pending |
| TABS-02 | Phase 24 | Pending |
| REVMENU-01 | Phase 24 | Pending |
| REVMENU-02 | Phase 24 | Pending |
| POLISH-01 | Phase 24 | Pending |
| POLISH-02 | Phase 24 | Pending |
| POLISH-03 | Phase 24 | Pending |

**Coverage:**
- v1.8 requirements: 26 total
- Mapped to phases: 26 ✓
- Unmapped: 0 ✓

**Per-phase counts:**
- Phase 20 (Kamacu Rebrand & Brand): 6 — REBRAND-01/02/03, BRAND-01/02/03
- Phase 21 (Data Directory Migration): 5 — MIGRATE-01/02/03/04/05
- Phase 22 (GitHub-Style Diff Review): 4 — DIFF-01/02/03/04
- Phase 23 (Worktree Cleanup Panel): 4 — WTREE-01/02/03/04
- Phase 24 (Session-Bar, Board & Tab Polish): 7 — TABS-01/02, REVMENU-01/02, POLISH-01/02/03

---
*Requirements defined: 2026-07-01*
*Last updated: 2026-07-01 — roadmap created (Phases 20–24), traceability populated, 26/26 mapped*
</content>

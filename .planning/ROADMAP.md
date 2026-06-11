# Roadmap: Kangent

## Milestones

- ✅ **v1.0 MVP** — Phases 1–5 (shipped 2026-06-11) — see [milestones/v1.0-ROADMAP.md](milestones/v1.0-ROADMAP.md)
- 🔵 **v1.1 Settings & Polish** — Phase 6 (in progress) — a global settings page wiring configurable agent params, worktree location, shell, and branch template into the existing v1.0 systems, plus a task-header layout fix

## Phases

<details>
<summary>✅ v1.0 MVP (Phases 1–5) — SHIPPED 2026-06-11</summary>

- [x] Phase 1: Foundation — Projects & Board (7/7 plans) — completed 2026-06-10
- [x] Phase 2: Terminal Engine (5/5 plans) — completed 2026-06-10
- [x] Phase 3: Worktree Isolation & Bash Tabs (6/6 plans) — completed 2026-06-10
- [x] Phase 4: Claude Code Agent Sessions (5/5 plans) — completed 2026-06-11
- [x] Phase 5: Recovery & Review (5/5 plans) — completed 2026-06-11

Full details: [milestones/v1.0-ROADMAP.md](milestones/v1.0-ROADMAP.md)

</details>

### v1.1 Settings & Polish (in progress)

- [ ] **Phase 6: Settings & Polish** - SQLite-backed global settings (store + API + page) wired into agent spawn, worktree creation, shell, and branch naming, plus the task-view header width fix

## Phase Details

### Phase 6: Settings & Polish
**Goal**: User can configure how agent sessions, worktrees, branches, and bash shells are created from one global settings page — changes take effect at the next spawn/creation with no restart — and the task-view header spans the full page width
**Depends on**: Phase 5 (extends v1.0's agent spawn, worktree, session, and task-view systems; nothing new in v1.1 is buildable without those call sites existing)
**Requirements**: SET-01, SET-02, SET-03, SET-04, AGENT-01, AGENT-02, WT-01, WT-02, SHELL-01, SHELL-02, BRANCH-01, BRANCH-02, UI-01
**Success Criteria** (what must be TRUE):
  1. User can open a dedicated full-page settings route via a gear control at the bottom of the projects sidebar, edit every setting there, restore any setting to its default, and see validation errors inline without losing other in-progress edits (SET-01, SET-04)
  2. All settings persist in a local SQLite settings table and survive a server restart, read and written over the API (SET-02)
  3. Editing the claude extra-params field (which defaults to including `--dangerously-skip-permissions`) and then pressing Start produces a `claude` spawn that includes those params; removing `--dangerously-skip-permissions` restores interactive permission prompts on the next spawn — with running sessions unaffected (AGENT-01, AGENT-02, SET-03)
  4. Changing the worktree base directory, then creating a new task, places that task's worktree under the new location; worktrees created before the change keep their stored absolute paths and remain fully usable (WT-01, WT-02)
  5. Editing the branch-name template (tokens `{slug}`, `{id}`, `{title}`; default `task/{slug}-{id}`) and then creating a task produces a branch matching the new template; an invalid template is rejected with a clear inline message and is never used to create a branch (BRANCH-01, BRANCH-02)
  6. Selecting the bash-tab shell from the dropdown (bash only in v1.1) and then opening a new bash tab spawns that shell — the shell command is read from settings, not hardcoded (SHELL-01, SHELL-02)
  7. The task-view header (title row plus the three-dots actions menu) spans the full page width, with the menu trigger flush to the right edge (UI-01)
**Plans:** 3/4 plans executed

Plans:
- [x] 06-01-PLAN.md — Settings foundation: migration 00004, internal/settings package (defaults, validation, template, tokenizer), GET/PUT /api/settings
- [x] 06-02-PLAN.md — Backend wiring: agent extra-params + shell into SpawnOpts, branch template + worktree base into provisionWorktree (create + Retry)
- [x] 06-03-PLAN.md — Frontend: /settings page + sidebar gear + per-field save model, UI-01 task-header width fix
- [ ] 06-04-PLAN.md — Build + full-suite gate, then human-verify checkpoint across all seven success criteria
**UI hint**: yes

**Phase notes:**
- **Reverses D-51 (intentional):** AGENT-02 makes the extra-params field default to including `--dangerously-skip-permissions`, flipping v1.0's interactive-by-default posture (Decision D-51). This is a deliberate milestone decision, not a regression — the user can remove the flag to restore prompts. Planner/executor should treat the default-on skip-permissions as the specified behavior and log the D-51 reversal in PROJECT.md Key Decisions at transition.
- **Integration over net-new:** every setting wires into an EXISTING v1.0 call site rather than building a new subsystem:
  - claude extra-params → agent spawn (`internal/session/agent.go`, Phase 4)
  - worktree base location → worktree creation (`internal/worktree` + `internal/api/tasks.go`, Phase 3)
  - bash-tab shell → bash session spawn (`internal/session`, Phases 2/3) — de-hardcode the currently hardcoded `bash`
  - branch-name template → branch naming in worktree creation (`internal/worktree`, Phase 3)
  - header layout → `web/src/pages/TaskPage.tsx` (Phases 1/4)
- **Settings semantics (from PROJECT.md):** global-only (no per-project overrides — that's SET-FUT-01, deferred), applied at next spawn/creation (no server restart, SET-03), stored in SQLite (SET-02). New migration adds the settings table.
- **WT-02 / no migration of existing trees:** existing worktrees keep their stored absolute paths; changing the base only affects new creations. Do not move live trees (explicitly out of scope).
- **SHELL scope:** dropdown is bash-only in v1.1; the value still comes from settings so additional shells (SHELL-FUT-01) are future data, not code.

## Progress

**Execution Order:**
Phases execute in numeric order: 1 → 2 → 3 → 4 → 5 → 6

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 1. Foundation — Projects & Board | v1.0 | 7/7 | Complete | 2026-06-10 |
| 2. Terminal Engine | v1.0 | 5/5 | Complete | 2026-06-10 |
| 3. Worktree Isolation & Bash Tabs | v1.0 | 6/6 | Complete | 2026-06-10 |
| 4. Claude Code Agent Sessions | v1.0 | 5/5 | Complete | 2026-06-11 |
| 5. Recovery & Review | v1.0 | 5/5 | Complete | 2026-06-11 |
| 6. Settings & Polish | v1.1 | 3/4 | In Progress|  |

---
*v1.0 shipped 2026-06-11 — 5 phases, 28 plans, 74 tasks*
*v1.1 roadmap created 2026-06-11 — Phase 6, granularity coarse (single phase), 12/12 requirements mapped*

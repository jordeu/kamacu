# Requirements: Kangent — v1.1 Settings & Polish

**Defined:** 2026-06-11
**Core Value:** One place to see and drive all agent work: every task gets its own isolated worktree and a persistent Claude Code session you can open, leave, and reattach to from the browser.
**Milestone goal:** A global settings page for configuring how agent sessions, worktrees, branches, and shells are created — plus a task-view header layout fix.

## v1.1 Requirements

### Settings Infrastructure

- [ ] **SET-01**: User can open a global settings page via a gear control at the bottom of the projects sidebar, on a dedicated full-page route
- [x] **SET-02**: Settings persist in a local SQLite settings table across restarts and are read/written over the API
- [ ] **SET-03**: Settings changes take effect at the next session spawn or worktree creation — running sessions and existing worktrees are unaffected, with no server restart required
- [x] **SET-04**: Every setting has a sensible default and can be restored to it; the page surfaces validation errors inline without losing other edits

### Claude Agent Parameters

- [ ] **AGENT-01**: User can edit a free-text list of extra parameters appended to every `claude` agent spawn
- [x] **AGENT-02**: The extra-params field defaults to including `--dangerously-skip-permissions`, which the user can remove to restore interactive permission prompts

### Worktree Location

- [ ] **WT-01**: User can configure the base directory under which new task worktrees are created (default `~/.kangent/worktrees/`)
- [ ] **WT-02**: Changing the worktree location affects only worktrees created afterward; existing worktrees keep their stored absolute paths and remain fully usable

### Shell Selection

- [ ] **SHELL-01**: User can select the shell command used for bash tabs from a dropdown (bash is the only option in v1.1)
- [ ] **SHELL-02**: The bash-tab shell command is read from settings rather than hardcoded, so additional shells can be added later as data

### Branch Naming

- [ ] **BRANCH-01**: User can edit the per-task branch-name template using tokens (`{slug}`, `{id}`, `{title}`), default `task/{slug}-{id}`
- [x] **BRANCH-02**: The branch template is validated to always produce a legal, collision-safe git ref; an invalid template is rejected with a clear message and never used to create a branch

### Task View Polish

- [ ] **UI-01**: The task-view header (title row plus the three-dots actions menu) spans the full page width so the menu trigger aligns flush to the right edge

## Future Requirements

Deferred beyond v1.1 (tracked, not in this roadmap):

- **NOTF-01**: Browser notification when an agent needs input or finishes
- **NOTF-02**: One-click "Move to In Review?" suggestion on the Stop hook
- **MAINT-01**: Stale-worktree purge list
- **AGNT-01**: MCP server for agent board access
- **SET-FUT-01**: Per-project settings overrides (v1.1 is global-only)
- **SHELL-FUT-01**: Additional shells (zsh, fish, etc.) in the shell dropdown

## Out of Scope

| Feature | Reason |
|---------|--------|
| Per-project / per-task settings overrides | v1.1 is global-only; overrides are a future capability |
| Migrating existing worktrees when the location changes | New-location-for-new-worktrees only; moving live trees risks breaking sessions |
| Additional shells beyond bash | Only bash is supported in v1.1; the seam is built, the options are future data |
| Free-text shell command (arbitrary binary) | Dropdown of curated shells, not an arbitrary command field, to keep it safe and clear |
| Validating that claude extra-params are individually correct flags | The field is passed through to `claude`; the app does not police claude's own flag set |
| Settings sync / export / import | Local single-user app; settings live in the local DB only |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| SET-01 | Phase 6 | Pending |
| SET-02 | Phase 6 | Complete |
| SET-03 | Phase 6 | Pending |
| SET-04 | Phase 6 | Complete |
| AGENT-01 | Phase 6 | Pending |
| AGENT-02 | Phase 6 | Complete |
| WT-01 | Phase 6 | Pending |
| WT-02 | Phase 6 | Pending |
| SHELL-01 | Phase 6 | Pending |
| SHELL-02 | Phase 6 | Pending |
| BRANCH-01 | Phase 6 | Pending |
| BRANCH-02 | Phase 6 | Complete |
| UI-01 | Phase 6 | Pending |

**Coverage:**
- v1.1 requirements: 12 total
- Mapped to phases: 12 ✓
- Unmapped: 0

---
*Requirements defined: 2026-06-11*
*Last updated: 2026-06-11 after roadmap creation (all 12 mapped to Phase 6)*

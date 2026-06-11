---
phase: 06-settings-polish
plan: 2
subsystem: api
tags: [settings, agent-spawn, worktree, branch-template, shell, go, tdd]

# Dependency graph
requires:
  - phase: 06-settings-polish (plan 06-01)
    provides: internal/settings package (Get/Set/Tokenize/ExpandTemplate/CheckRefFormat/ExpandHome), settings KV table
  - phase: 04-claude-code-agent-sessions
    provides: agent spawn argv construction (manager.go), fake-claude stub test seams
  - phase: 03-worktrees
    provides: provisionWorktree choke point, worktree.Service, Slug
provides:
  - SpawnOpts.ExtraArgs + SpawnOpts.Shell wired into both manager spawn branches (manager stays DB-free)
  - Agent spawns (fresh AND resume) append tokenized agent_extra_params after the fixed flags
  - Bash spawns run the LookPath-resolved settings shell; "" keeps the $SHELL fallback
  - worktree.PathUnder pure function (settings-driven base, repo-basename structure kept)
  - provisionWorktree reads branch_template + worktree_base at use with create-time CheckRefFormat defense
affects: [06-03, 06-04, settings, agent-spawn, worktree-creation]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Read-at-use settings: handlers read the DB per spawn/creation and thread values via SpawnOpts/locals — SET-03 is structural, never cached"
    - "Engines stay DB-free (Pitfall 5): no database/sql import in internal/session or internal/worktree"
    - "Test harnesses seed worktree_base with their temp dir so the absent-row default can never write into the real home"

key-files:
  created: []
  modified:
    - internal/session/manager.go
    - internal/session/session_test.go
    - internal/api/sessions.go
    - internal/api/sessions_test.go
    - internal/api/tasks.go
    - internal/api/tasks_test.go
    - internal/worktree/worktree.go
    - internal/worktree/worktree_test.go
    - internal/api/worktrees_test.go
    - internal/api/projects_test.go
    - internal/api/agents_test.go
    - internal/api/integration_test.go
    - internal/api/agent_integration_test.go
    - internal/api/recovery_integration_test.go
    - internal/api/diffs_test.go
    - internal/api/settings_test.go
    - cmd/kangent/main.go

key-decisions:
  - "Settings read failure at spawn is a 500 'couldn't start a session' — local SQLite errors are exceptional, never a silent fallback to defaults"
  - "Settings-read failures inside provisionWorktree route through the same D-25 worktree_error path as git errors — task create still 201s"
  - "Service.Root/PathFor kept as the legacy test shape; production placement is the new pure PathUnder (≈25 test call sites not churned)"
  - "Harness worktree_base seeding inlined per harness (plan-prescribed shape) rather than a shared helper"

patterns-established:
  - "Read-at-use: every settings consumer reads inside the handler at the moment of use"
  - "Harness seeding: any api test harness that provisions worktrees seeds settings worktree_base with its temp dir"

requirements-completed: [SET-03, AGENT-01, AGENT-02, WT-01, WT-02, SHELL-02, BRANCH-01, BRANCH-02]

# Metrics
duration: 24min
completed: 2026-06-11
---

# Phase 6 Plan 2: Settings Wiring Summary

**All four settings wired into their v1.0 call sites: tokenized claude extra-params append to every agent spawn (fresh + resume), bash tabs run the LookPath-resolved settings shell, and provisionWorktree builds template-driven branches under the settings worktree base with a create-time ref-format defense — all read-at-use, managers DB-free.**

## Performance

- **Duration:** 24 min
- **Started:** 2026-06-11T15:15:50Z
- **Completed:** 2026-06-11T15:40:40Z
- **Tasks:** 2 (both TDD)
- **Files modified:** 17

## Accomplishments

- AGENT-01/02 live: with default settings every agent spawn's argv ends with `--dangerously-skip-permissions` (D-51 reversal); storing `""` removes all extras at the very next Start — proven for fresh AND resume spawns through the REST surface against the fake-claude argv recorder
- SHELL-02: bash spawns (task tabs and the unscoped /terminal dev path — one handler) run the settings shell resolved via `exec.LookPath`; an unresolvable shell fails cleanly before PTY allocation; `Shell: ""` keeps the Phase 2 `$SHELL` fallback byte-for-byte so direct-Spawn tests stay green unmodified
- SET-03 is structural: the create handler and provisionWorktree read settings from the DB at each spawn/creation — proven by a test that corrupts the shell row (spawn 500s), fixes it, and spawns successfully with no restart
- BRANCH-01/02: `provisionWorktree` expands the current `branch_template` (create AND Retry — Pitfall 7 covered by a Retry-after-template-change test) and re-runs `CheckRefFormat` at creation time, so a hand-corrupted SQLite row lands in `worktree_error` (D-25) instead of reaching `git worktree add`
- WT-01/02: new `worktree.PathUnder(base, repo, slug, id)` keeps the repo-basename structure under the ~-expanded settings base; a base change affects only new worktrees — the old task's stored absolute path and on-disk tree are asserted untouched
- Default settings reproduce v1.0 behavior byte-for-byte (branch `task/<slug>-<id>`, same path shape) — existing naming tests pass unchanged
- Every api test harness now seeds `worktree_base` with its temp dir, so the absent-row default (`~/.kangent/worktrees/`) can never make tests write into the developer's real home

## Task Commits

Each TDD task produced a RED and a GREEN commit:

1. **Task 1: SpawnOpts.ExtraArgs + Shell, handler read-at-use** - `1b1bde3` (test), `995380c` (feat)
2. **Task 2: provisionWorktree settings-driven template + base, create-time defense** - `adcd498` (test), `eb34f7b` (feat)

No REFACTOR commits were needed.

## Files Created/Modified

- `internal/session/manager.go` - SpawnOpts.ExtraArgs/Shell; agent argv append after fixed flags; bash LookPath resolution with early error; D-51-reversal comment incl. the user-`--settings`-override sharp edge
- `internal/api/sessions.go` - create handler reads agent_extra_params (Tokenize → ExtraArgs) and shell per spawn; real DB error → 500
- `internal/worktree/worktree.go` - PathUnder pure function; PathFor comment points at it
- `internal/api/tasks.go` - provisionWorktree: settings.Get(branch_template/worktree_base) + ExpandTemplate + ExpandHome + PathUnder + create-time CheckRefFormat, all failures through the D-25 worktree_error path
- `cmd/kangent/main.go` - D-23 comment updated: wtRoot is now only the legacy Service field
- `internal/session/session_test.go` - shell table cases (settings/fallback/missing), extras-ordering tests (fresh + resume + none)
- `internal/api/sessions_test.go` - default-extras argv, PUT-""-removability, resume-carries-extras, shell read-at-use tests
- `internal/api/tasks_test.go` - template-change, base-change (WT-02 old-tree-untouched), corrupted-template→worktree_error tests
- `internal/api/worktrees_test.go` - Retry-after-template-change test
- `internal/worktree/worktree_test.go` - PathUnder unit cases + PathFor-agreement check
- `internal/api/{projects,agents,integration,agent_integration,recovery_integration,diffs}_test.go` - harness worktree_base seeding
- `internal/api/settings_test.go` - all-defaults test clears the harness seed it now inherits

## Decisions Made

- Settings read failure at spawn → 500 "couldn't start a session" (plan-specified posture: never silently fall back on a real DB error)
- provisionWorktree settings failures reuse the existing worktree_error UPDATE via a small `fail` closure — same D-25 shape, HTTP request still succeeds
- Kept `Service.Root`/`PathFor` untouched for the ~25 worktree-test call sites; only the production creation path moved to `PathUnder`
- Harness seeding written inline per harness (the plan's prescribed shape, also satisfies the `KeyWorktreeBase`-in-≥6-test-files acceptance grep — 10 files match)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] 06-01's TestSettingsGetAllDefaults broken by the planned harness seeding**
- **Found during:** Task 2 (harness migration)
- **Issue:** The plan's harness migration seeds `worktree_base` in `newTestServer`, which the 06-01 test `TestSettingsGetAllDefaults` uses while asserting the pristine all-defaults GET shape — the seeded row made `worktree_base.value` a temp dir instead of the default
- **Fix:** The test now deletes the harness's `worktree_base` seed row before the GET (it provisions no worktrees, so the safety seed is unnecessary there)
- **Files modified:** internal/api/settings_test.go
- **Verification:** `go test ./internal/api/` green including the settings suite
- **Committed in:** eb34f7b (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** Direct consequence of the planned harness seeding; no scope creep.

## Specified Behavior Changes (not regressions)

- Bash tabs previously ran `$SHELL` (possibly zsh); they now run the settings value `bash` — that IS SHELL-01/02 as specified
- Agent spawns now include `--dangerously-skip-permissions` by default — the intentional AGENT-02 / D-51 reversal; under it the amber waiting dot mostly disappears (permission prompts never fire), returning when the user removes the flag

## Issues Encountered

None beyond the documented deviation.

## Known Stubs

None — all wired paths read real settings and drive real spawns/creations.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- 06-03 (settings page frontend) can rely on the full backend behavior: PUTs take effect at the next spawn/creation with no restart
- 06-04 verification can exercise the live loop: edit extra-params → Start → bypass footer present; clear the field → Start → permission prompts return
- `go test ./...` green; `git diff go.mod` empty; no new dependencies

---
*Phase: 06-settings-polish*
*Completed: 2026-06-11*

## Self-Check: PASSED

All 4 task commits verified in git log; SpawnOpts.ExtraArgs, worktree.PathUnder, and the tasks.go settings wiring verified on disk.

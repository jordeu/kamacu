---
phase: 01-agent-data-foundation-and-spawn-engine
plan: "05"
subsystem: session
tags: [spawn-engine, claude, custom, pty, tokenize, template-render]

requires:
  - phase: 01-agent-data-foundation-and-spawn-engine
    provides: agents table + engine field, projects.agent_id
provides:
  - Spawn engine fork on agent.engine (claude byte-for-byte unchanged, custom render)
  - renderAgentCommand helper (tokenize-then-substitute-per-token)
affects: [custom-agent-status, fake-claude-regression]

tech-stack:
  added: []
  patterns: [engine-branch-in-spawn, tokenize-first-then-substitute-per-token, session-stays-pure-leaf]

key-files:
  created: []
  modified:
    - internal/api/agents_crud.go
    - internal/api/agents_crud_test.go
    - internal/api/sessions.go
    - internal/session/manager.go

key-decisions:
  - "claude path is byte-for-byte unchanged (session-id/--resume, --settings hook overlay, ExtraArgs) — fake-claude regression suite passes UNCHANGED, retiring the milestone's key risk"
  - "custom path: exec.LookPath(AgentArgs[0]) + exec.Command in the worktree PTY, same TERM/COLORTERM env; no hook overlay / BEL / resume"
  - "API layer renders the template (renderAgentCommand reusing settings.Tokenize) so session stays a pure leaf (no settings import); SpawnOpts carries already-split AgentArgs"
  - "Tokenize-first-then-substitute-per-token fixes a spaces-in-path bug (string-replace-then-tokenize re-split substituted values into multiple tokens)"

patterns-established:
  - "Engine branch keyed on SpawnOpts.AgentEngine keeps the claude golden path untouched"
  - "Template rendering belongs in the API layer, not the session package, to keep session dependency-free"

requirements-completed: []

coverage:
  - id: D1
    description: claude spawn path byte-for-byte unchanged (fake-claude regression green)
    requirement: ""
    verification:
      - kind: integration
        ref: "TestAgentLifecycle (claude gate, fake-claude)"
        status: pass
    human_judgment: false
  - id: D2
    description: custom spawn path renders command template correctly incl. spaces-in-path
    requirement: ""
    verification:
      - kind: unit
        ref: "internal/api/agents_crud_test.go (7 renderAgentCommand cases incl. worktree-with-spaces, unknown-placeholder)"
        status: pass
    human_judgment: false

duration: ~8min
completed: 2026-07-06
status: complete
---

# Plan 01-05: Spawn engine branch on agent.engine Summary

**The spawn block now forks on agent.engine: claude is byte-for-byte unchanged (fake-claude regression green — milestone key risk retired); the custom path renders the command template via tokenize-first-then-substitute-per-token, fixing a spaces-in-path bug.**

## Performance

- **Completed:** 2026-07-06
- **Tasks:** 1
- **Files modified:** 4

## Accomplishments
- `KindAgent` spawn block branches on `SpawnOpts.AgentEngine`:
  - `claude` (or `''` back-compat): unchanged argv (session-id/--resume, --settings hook overlay, ExtraArgs)
  - `custom`: `exec.LookPath(AgentArgs[0])` + `exec.Command` in the worktree PTY, same TERM/COLORTERM env; no hook overlay / BEL / resume
- `renderAgentCommand` in agents_crud.go renders the template (reuses `settings.Tokenize`, no new deps)
- session stays a pure leaf — no settings import; SpawnOpts carries already-split AgentArgs
- Bug fix: initial render was string-replace-then-tokenize (broke `{{worktree}}` values with spaces); rewritten to tokenize-first-then-substitute-per-token

## Task Commits

1. **T05: spawn engine branch on agent.engine** — `a84a4db` (feat)

## Files Created/Modified
- `internal/api/agents_crud.go` — `renderAgentCommand` (tokenize-then-substitute-per-token)
- `internal/api/agents_crud_test.go` — 7 unit cases (incl. worktree-with-spaces, unknown-placeholder-left-literal)
- `internal/api/sessions.go` — passes rendered AgentArgs + engine into SpawnOpts
- `internal/session/manager.go` — engine branch in the KindAgent spawn block

## Decisions Made
- Tokenize-first-then-substitute-per-token so a substituted value containing spaces stays one token (caught by unit test)
- Template rendering in the API layer keeps `session` a pure leaf (no `settings` import)

## Deviations from Plan

None — work was already implemented and committed; this summary documents the delivered state.

## Issues Encountered
- Initial render (string-replace-then-tokenize) broke `{{worktree}}` values containing spaces — re-split into multiple tokens. Caught by unit test; fixed by reordering to tokenize-first-then-substitute-per-token.

## Next Phase Readiness
- Spawn engine forked; ready for custom-agent status reporting (01-06) which adds the engine branch to `agentStatusLocked`

---
*Phase: 01-agent-data-foundation-and-spawn-engine*
*Completed: 2026-07-06*

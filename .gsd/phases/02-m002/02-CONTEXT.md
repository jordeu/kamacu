# CONTEXT — M002: opencode built-in agent engine

## Vision
Add opencode as a first-class built-in agent (alongside claude) so users can run opencode sessions per task with the same terminal UX and the same running/waiting/idle status detection claude has today.

## Why
The agent layer is already engine-generalized (`engine` column + `agentStatusLocked()` branch). opencode's explicit event taxonomy (`permission.asked`, `session.idle`, `message.updated`) maps 1:1 onto our status model — more precisely than claude's hooks. This is the lowest-friction way to support a second agent without a parallel status subsystem.

## Scope
- New engine value `"opencode"` in the full-heuristics branch of `agentStatusLocked()`.
- opencode spawn branch in `manager.go`: `opencode --session <id> --dangerously-skip-permissions <extras>` in a PTY, inherit-all env (D-52 posture).
- Gated global status plugin (`~/.config/opencode/plugins/kamacu-status.ts`) that maps opencode events → our hook receiver with claude-compatible event names; no-ops unless `KAMACU_SESSION_ID` env set at spawn.
- DB migration adding the opencode system seed row (`is_system=1`, `engine='opencode'`) + backfill safety net.
- `sessions.go` engine resolution from the task's agent row.
- Tests: engine-matrix extension, fake-opencode argv regression (mirrors `testdata/fake-claude`), real-binary status matrix.

## Out of scope
- opencode SSE/subscriber status model (rejected: permission events are ephemeral, issue #21154).
- opencode-specific config UI beyond the existing agent CRUD.
- BEL fallback for opencode (claude-only; unverified opencode emits BEL).
- Non-TUI `opencode run --format json` chat path.

## Key decisions
- **D013:** opencode reuses the claude hook-receiver path via a gated global plugin; hooks.go + session.go status machine unchanged.

## Key risks
1. Plugin lifecycle: ensuring the gated global plugin exists + is current at spawn; graceful degradation if opencode/bun is absent.
2. opencode `--session` resume semantics parity with claude `--resume` (survives kamacu restart?).
3. Plugin event ordering: does `session.created` fire before the first `permission.asked` (canary ordering)?
4. Version drift: opencode event names / plugin API across versions (mitigate: pin a target version, tolerate unknown events).

## Definition of done
- A task can be assigned the opencode agent and spawn a real opencode session in the terminal.
- Status reflects working/waiting/idle/exited driven by opencode events (not just activity heuristics).
- Session resume works after a kamacu restart.
- Engine-matrix + fake-opencode regression tests green; real-binary status matrix captured.
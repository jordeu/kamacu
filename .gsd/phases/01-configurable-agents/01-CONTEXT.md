# M001 — Configurable Agents

## Goal

Let the user run agents other than Claude Code in the task "Agent" tab. Today the agent spawn is hardcoded to `claude`. This milestone makes agents first-class configurable: define custom agents (name + command) in global Settings, pick a default, and choose per-project which agent runs.

**Core value:** The app is "one place to drive all agent work" — that should include non-Claude agents (gemini, aider, codex, etc.), not just Claude Code.

## Background — why this is more than "run a different binary"

The current agent spawn (`internal/session/manager.go:157-179`) is deeply Claude-specific. It hardcodes:
- `claude` binary via `exec.LookPath("claude")`
- Claude-only argv: `--session-id <uuid>` (fresh) or `--resume <id>` (reattach), plus `--settings <overlayJSON>`
- A `--settings` **hook overlay** (JSON: `Notification`/`Stop`/`SessionStart` matchers) that curls status back to `/api/hooks/sessions/{id}` — this IS the working/waiting/idle status system (STAT-02, D-53)
- A BEL (terminal-bell) fallback scanner for "waiting" detection
- `tasks.claude_session_id` for resume (RCVR-02, D-55), and a Claude-OAuth quota indicator (v1.2)

So "configure different agents" requires deciding how much of this machinery generalizes.

## Design decisions (locked at questioning, 2026-07-06)

**D-M001-1 — Command model: templated command.** Each agent stores a command template; the worktree is always the cwd, and placeholders (`{{worktree}}`, `{{session_id}}`) let power users wire custom flags. Claude's seed definition references them for `--session-id`/`--resume`.

**D-M001-2 — Status & resume scope: generic status, claude-only resume.** Custom agents show **running/exited only** (PTY process liveness). The working/waiting/idle states, the hook overlay, BEL scanning, and process-exit `--resume` stay **claude-only**. Terminal detach/reattach still works for all agents (the PTY lives independently of WS connections — that IS the reattach feature). On exit, a custom agent's "Reset session" starts fresh (same as a bash tab today).

**D-M001-3 — Claude representation: a pre-seeded agent row.** An `agents` table holds every agent. Claude is seeded as a **non-deletable** (`is_system=1`) row and marked the default (`is_default=1`). Users add/edit custom agents; the default flag can be reassigned to any agent. Uniform model — code branches on the `engine` column, never on the literal name "Claude".

## Internal model (how "claude-only" is encoded cleanly)

An `engine` column on `agents` (`'claude'` | `'custom'`, default `'custom'`) encodes the capability tier honestly:
- **`engine='claude'`** (the seed): spawn builds today's exact argv — session-id/resume/settings hook overlay/quota. Zero regression. The hook overlay + BEL + resume machinery stay in this path.
- **`engine='custom'`**: spawn renders the command template (shell-words split, cwd=worktree, `{{worktree}}`/`{{session_id}}` substitution). No hooks, no BEL scanning, no resume. Status = PTY liveness (running/exited).

This is not name-string matching — it is a declared capability, extensible to future engines. The user-facing agent form shows only **name + command** (and template help). The claude seed's hook/resume plumbing is internal, not user-editable. The claude seed's name + command (binary path) ARE editable; its engine stays `'claude'`.

## Data model

```
agents (NEW, migration 00013)
  id            INTEGER PK
  name          TEXT NOT NULL
  command       TEXT NOT NULL          -- template; e.g. "claude", "gemini", "aider --model sonnet"
  engine        TEXT NOT NULL DEFAULT 'custom'   -- 'claude' | 'custom'
  is_default    INTEGER NOT NULL DEFAULT 0       -- exactly one row has this = 1 (flag-keyed, rename-proof — mirrors workspaces D-002)
  is_system     INTEGER NOT NULL DEFAULT 0       -- 1 for the claude seed -> non-deletable
  created_at, updated_at

projects (migration 00013)
  + agent_id INTEGER NOT NULL DEFAULT 1 REFERENCES agents(id) ON DELETE RESTRICT
  -- backfilled to the default agent in-SQL; idempotent BackfillAgents startup hook
  -- (mirrors BackfillWorkspaces / BackfillProjectIcons)
```

## Scope

**In scope:**
- `agents` table + seed + `projects.agent_id` FK + backfill (migration 00013).
- `/api/agents` CRUD (list, create, update, delete with `is_system` guard, set-default) — reuse the v1.9 workspaces CRUD shape.
- Spawn engine branch on `engine`; claude path preserved byte-for-byte.
- Global Settings "Agents" section (list/add/edit/delete/set-default).
- Project Settings agent selector.
- Custom agents render in the same Agent tab terminal shell; running/exited status.
- Per-project agent resolves at spawn.

**Out of scope (documented):**
- Generic working/waiting/idle status for custom agents (rejected — unreliable TUI heuristics across diverse agents).
- Per-agent hook/resume templates for custom agents (rejected this milestone — claude-only; could reopen if a real agent needs it).
- Agent icon/color identity (name-only, like workspaces v1.9).
- Bulk reassignment of projects to a different agent.
- Editing the claude engine's hook overlay JSON (internal plumbing, not user-facing).
- Folding the existing global `agent_extra_params` setting into the claude agent row — it stays a global setting applied to claude spawns only (documented as future cleanup; avoids a settings->agent data migration this milestone).

## Carry-forward constraints (must respect)

- **D001** single-binary, **D004** modernc SQLite + goose, **D005** degrade-don't-break, **D006** destructive-defaults-leave-it (agent delete = block-until-no-project-uses-it), **D007** zero-new-deps gate, **D008** backend->UI phase split (this milestone is 2 slices), **D012** human-gate may redesign.
- v1.9 workspaces pattern (migration 00012, `BackfillWorkspaces`, `is_default`-flag-keyed guards) is the direct template — reuse its shape.

## Verification

- `go test ./...`, `go vet ./...`, `make test` (per PREFERENCES).
- Frontend gate: `cd web && tsc -b && vite build`.
- Staged-upgrade test: existing install -> every project under the default agent, tasks intact, exactly one default, idempotent re-run (mirrors the 00012 workspaces staged test).
- Regression proof for the claude path: the existing claude-spawn tests (fake-claude stub) pass unchanged — the claude argv is byte-for-byte preserved.
- Human-verify gate: add a custom agent (e.g. `echo` or a real CLI), select it on a project, spawn it in the Agent tab, confirm running/exited status and terminal reattach; then confirm a claude project still shows working/waiting + resume with zero regression.

## Why no research phase

The "generic status, claude-only resume" choice (D-M001-2) eliminated the only genuine unknown: we do NOT need to study how gemini/aider/codex render their TUIs, because we are not detecting working/waiting for them. We just run their command in the worktree dir and report PTY liveness. The claude path is unchanged and already verified since v1.0.

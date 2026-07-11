# Requirements: Kamacu — v1.10 Configurable Agents

**Defined:** 2026-07-06
**Core Value:** One place to see and drive all agent work: every task gets its own isolated worktree and a persistent agent session you can open, leave, and reattach to from the browser.

## v1.10 Requirements

Requirements for the Configurable Agents milestone. Agents become first-class configurable data — users can define custom agent CLIs and assign them per-project, with opencode joining Claude Code as a built-in engine.

### Agent Data Foundation

- [x] **AGDATA-01** (R012): An `agents` table (migration 00013) holds every runnable agent — name, command template, engine (`claude`|`custom`), `is_default`, `is_system`. Claude Code is pre-seeded as a non-deletable (`is_system=1`) row marked the default (`is_default=1`). Exactly one row is `is_default=1` at all times (flag-keyed, rename-proof).
- [x] **AGDATA-02** (R013): `projects.agent_id` foreign key (`NOT NULL ... REFERENCES agents(id) ON DELETE RESTRICT`, migration 00013), backfilling every existing project to the default agent in-SQL, plus an idempotent `BackfillAgents` startup hook guaranteeing a project always has an agent.

### Agent Management

- [x] **AGMGMT-01** (R014): The user can create a custom agent from global Settings by providing a name and a command template (e.g. `gemini`, `aider --model sonnet`). The worktree is always the cwd; `{{worktree}}` and `{{session_id}}` placeholders are available for power users.
- [x] **AGMGMT-02** (R015): The user can edit any agent's name and command. The claude seed's engine stays `claude` (its hook/resume plumbing is internal), but its name and command (binary path) are editable. Custom agents are fully editable. The `is_system` seed is non-deletable.
- [x] **AGMGMT-03** (R018): The user can delete a custom agent only when no project uses it (block-until-unassigned, never cascade-reassign) and never the claude seed (`is_system=1`). The `ON DELETE RESTRICT` FK is the backstop; a count-guard gives a clear error message.
- [x] **AGMGMT-04** (R019): The user can set any agent as the global default; exactly one agent is `is_default=1` at all times. Setting a new default clears the previous flag in the same transaction. Existing projects do NOT auto-follow a default change; their `agent_id` is stable.

### Spawn Engine

- [x] **AGSPAWN-01** (R016): The claude engine spawn path is preserved byte-for-byte — session-id (fresh) / `--resume` (reattach), the `--settings` hook overlay, BEL fallback scanning, working/waiting/idle status, `tasks.claude_session_id` resume, and the Claude-OAuth quota indicator all continue to work with zero regression.
- [x] **AGSPAWN-02** (R017): The custom engine spawn path renders the agent's command template (shell-words split, cwd=worktree, `{{worktree}}`/`{{session_id}}` substitution) in the task's worktree PTY. Custom-agent sessions report running/exited status only; no hook overlay, no BEL scanning, no `--resume`. Terminal detach/reattach works mid-run.
- [x] **AGSPAWN-03** (R020): A project's agent resolves at spawn time from `projects.agent_id` (read fresh inside the spawn handler, never cached client-side). A newly created project is assigned the current global default agent. Changing a project's agent applies to the NEXT spawn only.

### Agent Management UI

- [x] **AGUI-01** (R021): Global Settings gains an Agents section — an ordered list of agents (name + command + engine badge), add, edit, delete (disabled with a tooltip when `is_system` or in-use), and set-default. The claude seed renders as a system agent with its name + command editable but no delete affordance.
- [x] **AGUI-02**: A per-project agent selector lets the user pick which agent a project uses; the Active Sessions bar shows custom-agent running sessions (LIVE filter widened to include `running`).
- [x] **AGUI-03**: Extra Claude params (`--dangerously-skip-permissions` etc.) move from an orphaned global setting onto the Claude agent's own row (migration 00014 + `BackfillAgentExtraParams`), edited in the Claude agent's edit dialog.

### opencode Built-in Engine

- [x] **OCENG-01**: opencode is seeded as a second non-deletable system agent (migration 00015 + `BackfillOpenCodeAgent`, `engine='opencode'`, `is_system=1`) alongside Claude — present at startup, never API-created or deleted.
- [x] **OCENG-02**: The opencode engine gets its own spawn branch (exempt from the custom-render tokenize-substitute path), with the `KAMACU_*` hook env injected so activity-based status works — not just running/exited.
- [x] **OCENG-03**: The opencode status plugin's `notify()` uses `fetch()`+`AbortController(3s)` (not a curl shell-out that silently no-ops when curl is absent), so activity signals reliably unlock the waiting/idle states.

### opencode Session Resume

- [x] **OCRESUME-01**: `tasks.opencode_session_id` (nullable, migration 00016) persists opencode's opaque `ses_…` session id — opencode has no transcript files (sessions live in `opencode.db`), so resume keys off this persisted id alone.
- [x] **OCRESUME-02**: `captureOpencodeSessionAsync` — an async bounded poll launched at spawn — discovers opencode's `ses_…` id (minted sometime after start) and persists it; best-effort + warn-only so a capture failure costs only restart-resume, not the live session.
- [x] **OCRESUME-03**: opencode `--resume` argv wired so a dead/restarted opencode session is resumable from the persisted id — closing the restart-resume loop. A build-tagged real-opencode e2e harness proves the shipped plugin loads and its POSTs reach a receiver.

## Future Requirements

Acknowledged but deferred beyond v1.10. Tracked, not in the current roadmap.

- **AGFUT-01**: Per-agent workspace-scoped overrides (agent config varies by workspace).
- **AGFUT-02**: Agent marketplace / sharing (export/import agent templates).
- **AGFUT-03**: Additional built-in engines beyond claude + opencode (e.g. codex, gemini CLI as first-class seeds with their own status heuristics).

## Out of Scope

Explicitly excluded for v1.10. Documented to prevent scope creep.

| Feature | Reason |
|---------|--------|
| Custom-agent working/waiting/idle status heuristics | Custom TUIs can't be reliably introspected; running/exited is honest (D-M001-2). opencode gets heuristics via its own engine branch + plugin, not the generic custom path. |
| `--resume` for custom-engine agents | Only engines with a known resume protocol (claude `--resume`, opencode `--resume`) get it; custom agents get "Reset session" (fresh spawn) on exit. |
| Deleting the claude/opencode `is_system` seeds | They are the permanent baseline; `is_system=1` is non-deletable. |
| Auto-following a default-agent change | Existing projects keep their stable `agent_id`; only new projects pick up the new default (read-at-use, D011 pattern). |
| Multi-user / shared agent configs | Single-user localhost app — out of scope project-wide. |

## Traceability

Which phases cover which requirements.

| Requirement | Phase | Status |
|-------------|-------|--------|
| AGDATA-01 (R012) | Phase 01 | Complete |
| AGDATA-02 (R013) | Phase 01 | Complete |
| AGMGMT-01 (R014) | Phase 01 | Complete |
| AGMGMT-02 (R015) | Phase 01 | Complete |
| AGMGMT-03 (R018) | Phase 01 | Complete |
| AGMGMT-04 (R019) | Phase 01 | Complete |
| AGSPAWN-01 (R016) | Phase 01 | Complete |
| AGSPAWN-02 (R017) | Phase 01 | Complete |
| AGSPAWN-03 (R020) | Phase 01 | Complete |
| AGUI-01 (R021) | Phase 02 | Complete |
| AGUI-02 | Phase 02 | Complete |
| AGUI-03 | Phase 02 | Complete |
| OCENG-01 | Phase 03 | Complete |
| OCENG-02 | Phase 03 | Complete |
| OCENG-03 | Phase 04 | Complete |
| OCRESUME-01 | Phase 05 | Complete |
| OCRESUME-02 | Phase 05 | Complete |
| OCRESUME-03 | Phase 05 | Complete |

**Coverage:**
- v1.10 requirements: 18 total
- Mapped to phases: 18 ✓ (Phase 01: 9 · Phase 02: 3 · Phase 03: 2 · Phase 04: 1 · Phase 05: 3)
- Unmapped: 0

---
*Requirements defined: 2026-07-06*
*Last updated: 2026-07-11 — all 18 requirements satisfied, v1.10 ready to ship*

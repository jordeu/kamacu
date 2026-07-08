# RESEARCH — opencode as a built-in agent engine (M002)

**Question:** Can we add opencode as a first-class built-in agent (like claude) with running/waiting/idle status detection? Is the claude mechanism portable?

**Verdict: YES — fully feasible, and opencode's event surface is more explicit than claude's.** The existing claude hook-receiver path is reused almost unchanged.

---

## 1. How claude does it today (the mechanism to port)

Two halves, already generalized behind an `engine` field:

### Spawn (`internal/session/manager.go`)
Claude engine builds an argv: `claude --session-id <uuid> --settings <inline-JSON> <extras>`. The inline `--settings` document (`internal/session/agent.go buildOverlayJSON`) injects **command hooks** that `curl` back to our `POST /api/hooks/sessions/{id}` endpoint with a per-instance token. Hooks: `Notification` (matcher `permission_prompt|elicitation_dialog`), `Stop`, `SessionStart`. Nothing written to disk — the whole hook config rides as one argv.

### Status (`internal/session/session.go`)
`agentStatusLocked()` branches on engine:
- `claude` / `""` → full heuristics: `waiting` (Notification hook / BEL fallback), `idle` (Stop hook + settle window), `working` (activity timestamp), `exited`.
- `custom` → collapsed to `running`/`exited` only (deliberate: arbitrary TUIs are not understood).

The hook receiver (`internal/api/hooks.go`) switches on `hook_event_name`: `Notification`→`SetWaiting`, `Stop`→`SetIdle`, `SessionStart`→`MarkHooksAlive`.

**The seam already exists:** `if s.engine != "" && s.engine != "claude" { return "running" }`. Adding opencode = add `"opencode"` to the full-heuristics branch + wire a push path.

## 2. opencode's equivalent surface

opencode **always runs a server** (the TUI is a client over an HTTP/OpenAPI backend, `opencode serve`). It publishes an explicit **event taxonomy** (opencode.ai/docs/plugins) mapping near-1:1 onto our status model — more precisely than claude's:

| Our state | claude trigger | opencode event | notes |
|-----------|----------------|----------------|-------|
| waiting | Notification (`permission_prompt`) | `permission.asked` | explicit |
| idle | Stop (turn end) | `session.idle` | explicit |
| working | activity heuristic | `message.updated` / `message.part.updated` | explicit |
| (canary) | SessionStart hook | `session.created` / `server.connected` | hooks-alive proof |
| error | — (none) | `session.error` | bonus claude lacks |

Two consumption models:

- **Plugin (in-process push)** — STABLE. A `.opencode/plugins/status.ts` subscribes to `event` and curls our hook endpoint. Direct analog of claude `--settings` hooks. The shipping `opencode-ntfy` plugin proves `session.idle`/`permission.asked`/`session.error` fire reliably in-process (no experimental flag required).
- **SSE subscriber (external pull)** — `GET /global/event`. **REJECTED for status:** opencode issue #21154 documents `permission.asked`/`question.asked` are *ephemeral BusEvents* — lost if no client is subscribed at the instant they fire. Breaks reattach-time "waiting" detection. Plugin model has no such race.

## 3. Three real differences from claude (risks)

1. **Plugins are files on disk, not argv-injected.** claude's whole hook config rides as one `--settings` argv (zero disk writes). opencode plugins live in `.opencode/plugins/` (worktree) or `~/.config/opencode/plugins/` (global). Worktree-level pollutes git status; global applies to ALL opencode runs on the machine. **Mitigation:** write once to global dir; gate plugin body on a spawn-time env (`KAMACU_SESSION_ID` + token + baseURL) so it no-ops for non-Kamacu invocations. Less clean than claude, acceptable.

2. **Resume flags differ.** claude: `--resume <id>`. opencode: `--session <id>` (or `-c`/`--continue` for last). Session ID format `ses_…`. Mappable — different constants in the spawn branch.

3. **No BEL fallback (unverified).** claude scans bare BEL (0x07) as a last-resort "waiting" signal when hooks aren't confirmed alive. opencode's TUI BEL-on-permission behavior is unverified. Since the plugin path gives reliable status, keep BEL claude-only; opencode relies on its plugin canary (`session.created` → `MarkHooksAlive`).

Parity confirmed: opencode has `--dangerously-skip-permissions` (matches our extra_params default), and `--format json` / `opencode run` for a non-TUI path if ever needed.

## 4. Recommended implementation (reuses ~90% of existing code)

- New engine `"opencode"` added to the full-heuristics branch of `agentStatusLocked()` (alongside `claude`).
- Spawn branch in `manager.go`: (a) ensure gated global status plugin exists, (b) spawn `opencode --session <id> --dangerously-skip-permissions <extras>` in a PTY with `KAMACU_SESSION_ID`/token/baseURL env set.
- The plugin maps opencode events → **claude-compatible hook event names** (`permission.asked`→`Notification`, `session.idle`→`Stop`, `session.created`→`SessionStart`). **`hooks.go` and the entire `session.go` status machine need zero changes** — the receiver already switches on those three names.
- DB seed row (`is_system=1`, `engine='opencode'`) mirroring the claude seed in migration `00013`.
- Tests: extend the `agent_engine_test.go` matrix with an `opencode`-engine case asserting it lands in the heuristic branch (not collapsed to `running`); fake-opencode binary for the spawn argv regression (mirroring `testdata/fake-claude`).

## 5. Unknowns to resolve in a spike/first slice

- Whether opencode's TUI emits BEL on permission prompts (would unlock a cheap fallback). Low priority.
- Exact argv opencode wants for a non-interactive-but-PTY session + whether `--session` survives restart (resume semantics parity with claude `--resume`).
- Plugin load timing: does `session.created` reliably fire before the first `permission.asked`? (canary ordering).
- Whether the experimental event flag touches in-process plugin `event` hooks (evidence says no, but verify on the target opencode version).

## Sources
- opencode.ai/docs/plugins/ — event taxonomy (permission/session/message/lsp/tool events), plugin load order (global→project), TS plugin API
- opencode.ai/docs/server/ — `opencode serve`, `/global/event` SSE, OpenAPI 3.1 spec
- opencode.ai/docs/cli/ — `--session`, `--continue`, `--dangerously-skip-permissions`, `--format json`, `OPENCODE_EXPERIMENTAL_EVENT_SYSTEM`
- github.com/anomalyco/opencode issue #21154 — permission/question prompts are ephemeral BusEvents (SSE race on reattach)
- github.com/anomalyco/opencode issue #16879 — plugin `event` hook is fire-and-forget (no await); `event.sync` proposed
- github.com/lannuttia/opencode-ntfy.sh — shipping plugin firing on session.idle/session.error/permission.asked (proof of stability)
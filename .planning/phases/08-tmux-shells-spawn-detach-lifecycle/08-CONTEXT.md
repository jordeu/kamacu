# Phase 8: tmux Shells — Spawn & Detach Lifecycle - Context

**Gathered:** 2026-06-12
**Status:** Ready for planning

<domain>
## Phase Boundary

tmux as a selectable bash-tab shell that makes shells durable **invisibly**: tabs look and behave exactly like plain bash tabs (× kills, leaving the task view leaves the session running, reopening reattaches). The user-visible payoff — surviving a Kangent server restart with Resume — lands in Phase 9; this phase builds the spawn/kill/exit lifecycle it reconciles against. Plain bash tabs and agent sessions are untouched (regression-guarded).

**Requirement amendments made during this discussion** (REQUIREMENTS.md and ROADMAP.md updated in the same commit):
- **TMUX-03 (amended):** detach is implicit — sessions survive *leaving the task view* (and, Phase 9, a server restart); reopening the task reattaches. The original "closing the tab detaches" is reversed.
- **TMUX-04 (amended):** closing the tab (×) **kills** the tmux session — kill-on-close parity with plain bash tabs. No separate "Kill session" affordance exists or is needed.
- **REAP-01 (new, Phase 9):** Done-TTL session reaper (see decisions below).
- **Out of Scope row reversed:** "Injecting tmux config" was out of scope; the invisible-tmux model requires Kangent-controlled config on the dedicated socket (status off, mouse on).

</domain>

<decisions>
## Implementation Decisions

### Invisible tmux (the core model — emerged mid-discussion, supersedes earlier picks)
- **D-77:** tmux-backed tabs are visually and behaviorally **indistinguishable** from plain bash tabs: same `Bash N` labels, no badge, no detached-tab states, no context menus, no kill confirmation. tmux is a durability implementation detail the user shouldn't notice.
- **D-78:** **× = kill.** Closing a tmux tab kills the tmux session, exactly like closing a bash tab kills the shell (D-29 stays universal). Detach happens implicitly: leaving the task view (or server shutdown) leaves the tmux session running; reopening the task reattaches. The whole point is surviving a Kangent restart (Phase 9).
- **D-85:** No board-level surfacing of shell sessions — task view only; board cards keep their current agent-status dots untouched.

### tmux server config (dedicated `-L kangent` socket — Kangent-controlled)
- **D-79:** Status bar **off** (`status off`) — the tab must look like a plain shell.
- **D-80:** Mouse mode **on** (`set -g mouse on`) — wheel-scrolling works against tmux's real history (the history that survives detach/restart). Accepted caveat: inside tmux tabs, selection goes through tmux rather than xterm copy-on-select (deviation from D-17 scoped to tmux tabs).
- **D-81:** Ctrl+B prefix left at tmux default — power users keep splits/copy-mode; no unbinding, no extra config.

### Failure & edge behavior
- **D-82:** A tmux session that died while nobody was attached (shell exited while away, or killed externally via `tmux -L kangent`) shows the **honest exited state** (existing exited banner) when discovered — never a silent disappearance (TMUX-06 spirit, D-56 no-silent-fallback).
- **D-83:** Shell setting flips (bash ↔ tmux) apply **at next spawn only**; existing sessions/tabs untouched; a task may transiently have mixed bash and tmux tabs (Phase 6 extra-params precedent).
- **D-84:** If tmux disappears from PATH after being selected, a new tab spawn fails with an **honest error** ("tmux not found — change the shell setting or reinstall"); no silent fallback to plain bash.

### Done-TTL session reaper (scoped to Phase 9 — locked here so Phase 9 planning inherits it)
- **D-86:** Tasks in Done get their sessions automatically killed after a TTL — **bash, tmux, AND agent (claude) sessions**. Global setting, default **24h**, clocked from when the task **entered Done**; leaving Done cancels the timer; `0`/never disables. Enforced by a periodic server-side check.
- **D-87:** The reaper **never touches worktrees** — worktree cleanup stays manual via the existing gated dialog (D-31/D-32/D-33 unchanged).

### Settled by roadmap research (carried forward, do not revisit)
- Dedicated socket `-L kangent` (now with Kangent-set options per D-79/D-80 instead of fully vanilla `-f /dev/null`), `=name` exact-match targets, no new session `Kind` (tmux tabs are `KindBash` + `tmuxName`), name minted/persisted by the HTTP handler (`tmux_sessions` table, migration 00005, identity only), env scrubbing (`TMUX`/`TMUX_PANE` removed, `TERM` pinned), cross-generation ring-buffer replay skipped for tmux tabs, new leaf package `internal/tmux`.
- Lifecycle strategy (detach vs kill at server shutdown) is an explicit per-session property decided at spawn time — not `if isTmux` branches at stop time.

### Claude's Discretion
- Whether "leaving the task view" literally runs `detach-client` or the attach PTY simply stays alive server-side while the server lives — the user-facing contract is what matters: navigation never kills anything, reopening reattaches, the tmux session survives server stop while attach PTYs die.
- Mechanism for injecting socket config (post-create `set-option` calls vs a generated `-f` config file) — must apply deterministically on every server start.
- Exact error copy for tmux-not-found and exited states (follow UI-SPEC patterns; reuse the existing exited banner).
- How exit-vs-detach discrimination (`tmux has-session` after the attach PTY exits) is wired into the existing exited-state path.
- Regression-test shape for the shared stop path (TMUX-07).

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Requirements & roadmap (amended this session — read the post-amendment versions)
- `.planning/ROADMAP.md` — Phase 8 + Phase 9 sections: settled research decisions (socket, naming, table, env scrubbing, `internal/tmux` package) and the regression-risk note
- `.planning/REQUIREMENTS.md` — TMUX-01..08 (TMUX-03/04 amended), REAP-01 (new), Out of Scope table (tmux-config row reversed)

### Prior phase contracts this phase must not break
- `.planning/phases/02-terminal-engine/02-CONTEXT.md` — D-14 (stop = SIGTERM→5s→SIGKILL), D-16 (no idle timeouts — now excepted by REAP-01 for Done tasks), D-17 (copy-on-select), D-28/D-29 via Phase 3
- `.planning/phases/03-worktree-isolation-bash-tabs/03-CONTEXT.md` — D-27..D-34 (tab=session model, cleanup gates the reaper must respect)
- `.planning/phases/05-recovery-review/05-CONTEXT.md` — D-54..D-58 (Resume UX pattern Phase 9 mirrors), D-56 (honest-failure principle reused here)

No external specs beyond these — tmux behavior details to be verified empirically (roadmap's 5-minute smoke test of `=` targets and `detach-client` flags).

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/settings/validate.go:16` — `AllowedShells = []string{"bash"}`: the single seam for adding "tmux"; the same slice already drives the dropdown options via `internal/api/settings.go:26` (SHELL-01). Conditional inclusion = LookPath check.
- `internal/session/manager.go` (~:160-170) — shell resolved via `exec.LookPath` BEFORE PTY allocation; the honest-spawn-error path (D-84) already half-exists.
- `internal/session/session.go:382` — `Stop()` implements D-14 process-group teardown; this is the shared stop path TMUX-07 regression-guards. tmux kill must be a per-session lifecycle property, not a branch here.
- `internal/session/session.go` — `Kind` (KindBash/KindAgent) stays as-is; tmux tabs are `KindBash` + a `tmuxName` field per roadmap.
- Existing exited-state banner (D-15) — reused unchanged for tmux exit and stale-session discovery (D-82).

### Established Patterns
- Settings apply at next spawn (Phase 6 extra-params) — D-83 follows it.
- Honest failure over silent fallback (D-56) — D-82/D-84 follow it.
- Server is source of truth for tabs (D-28) — unchanged; tmux only changes what survives server death.

### Integration Points
- `internal/api/sessions.go` — spawn handler mints/persists `kangent-<task>-<n>` names (new `tmux_sessions` table, migration 00005 in `internal/store`).
- New leaf package `internal/tmux` (HasSession/KillSession/DetachClient/NewSessionArgs) imported by `internal/session` and `internal/api`.
- PTY/WS/ring-buffer transport untouched — a tmux attach client is just another full-screen PTY child.

</code_context>

<specifics>
## Specific Ideas

- "The user should not be much aware that is a tmux session" — the user's literal framing; it drove the rollback of all badge/menu/confirm ideas and the status-off/mouse-on config decisions.
- "The main purpose of using tmux is that the session survives a Kangent restart" — durability across restarts is THE feature; everything else is plumbing.
- "We don't want to accumulate tmux sessions, neither bash sessions" — origin of REAP-01.

</specifics>

<deferred>
## Deferred Ideas

- **REAP-01 implementation** — Done-TTL session reaper (D-86/D-87): decided here, ships in **Phase 9** alongside cleanup integration. Added to REQUIREMENTS.md and the Phase 9 roadmap section.
- Auto-removing **worktrees** on the Done TTL — explicitly declined (D-87); worktree cleanup stays manual. If disk accumulation becomes a pain, the parked MAINT-01 (stale-worktree purge list) is the vehicle.
- **TMUX-FUT-01** (user's own tmux server instead of dedicated socket) — already parked in REQUIREMENTS.md future section.

</deferred>

---

*Phase: 08-tmux-shells-spawn-detach-lifecycle*
*Context gathered: 2026-06-12*

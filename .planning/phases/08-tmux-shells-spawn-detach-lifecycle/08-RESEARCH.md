# Phase 8: tmux Shells — Spawn & Detach Lifecycle - Research

**Researched:** 2026-06-12
**Domain:** tmux process lifecycle integration (Go `os/exec` + PTY), invisible-tmux UX
**Confidence:** HIGH — every load-bearing tmux behavior was verified empirically on the host (`tmux 3.4`), and all codebase seams were read directly

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Invisible tmux (the core model — emerged mid-discussion, supersedes earlier picks)**
- **D-77:** tmux-backed tabs are visually and behaviorally **indistinguishable** from plain bash tabs: same `Bash N` labels, no badge, no detached-tab states, no context menus, no kill confirmation. tmux is a durability implementation detail the user shouldn't notice.
- **D-78:** **× = kill.** Closing a tmux tab kills the tmux session, exactly like closing a bash tab kills the shell (D-29 stays universal). Detach happens implicitly: leaving the task view (or server shutdown) leaves the tmux session running; reopening the task reattaches. The whole point is surviving a Kangent restart (Phase 9).
- **D-85:** No board-level surfacing of shell sessions — task view only; board cards keep their current agent-status dots untouched.

**tmux server config (dedicated `-L kangent` socket — Kangent-controlled)**
- **D-79:** Status bar **off** (`status off`) — the tab must look like a plain shell.
- **D-80:** Mouse mode **on** (`set -g mouse on`) — wheel-scrolling works against tmux's real history (the history that survives detach/restart). Accepted caveat: inside tmux tabs, selection goes through tmux rather than xterm copy-on-select (deviation from D-17 scoped to tmux tabs).
- **D-81:** Ctrl+B prefix left at tmux default — power users keep splits/copy-mode; no unbinding, no extra config.

**Failure & edge behavior**
- **D-82:** A tmux session that died while nobody was attached (shell exited while away, or killed externally via `tmux -L kangent`) shows the **honest exited state** (existing exited banner) when discovered — never a silent disappearance (TMUX-06 spirit, D-56 no-silent-fallback).
- **D-83:** Shell setting flips (bash ↔ tmux) apply **at next spawn only**; existing sessions/tabs untouched; a task may transiently have mixed bash and tmux tabs (Phase 6 extra-params precedent).
- **D-84:** If tmux disappears from PATH after being selected, a new tab spawn fails with an **honest error** ("tmux not found — change the shell setting or reinstall"); no silent fallback to plain bash.

**Done-TTL session reaper (scoped to Phase 9 — locked here so Phase 9 planning inherits it)**
- **D-86:** Tasks in Done get their sessions automatically killed after a TTL — **bash, tmux, AND agent (claude) sessions**. Global setting, default **24h**, clocked from when the task **entered Done**; leaving Done cancels the timer; `0`/never disables. Enforced by a periodic server-side check.
- **D-87:** The reaper **never touches worktrees** — worktree cleanup stays manual via the existing gated dialog (D-31/D-32/D-33 unchanged).

**Settled by roadmap research (carried forward, do not revisit)**
- Dedicated socket `-L kangent` (now with Kangent-set options per D-79/D-80 instead of fully vanilla `-f /dev/null`), `=name` exact-match targets, no new session `Kind` (tmux tabs are `KindBash` + `tmuxName`), name minted/persisted by the HTTP handler (`tmux_sessions` table, migration 00005, identity only), env scrubbing (`TMUX`/`TMUX_PANE` removed, `TERM` pinned), cross-generation ring-buffer replay skipped for tmux tabs, new leaf package `internal/tmux`.
- Lifecycle strategy (detach vs kill at server shutdown) is an explicit per-session property decided at spawn time — not `if isTmux` branches at stop time.

### Claude's Discretion
- Whether "leaving the task view" literally runs `detach-client` or the attach PTY simply stays alive server-side while the server lives — the user-facing contract is what matters: navigation never kills anything, reopening reattaches, the tmux session survives server stop while attach PTYs die.
- Mechanism for injecting socket config (post-create `set-option` calls vs a generated `-f` config file) — must apply deterministically on every server start.
- Exact error copy for tmux-not-found and exited states (follow UI-SPEC patterns; reuse the existing exited banner).
- How exit-vs-detach discrimination (`tmux has-session` after the attach PTY exits) is wired into the existing exited-state path.
- Regression-test shape for the shared stop path (TMUX-07).

### Deferred Ideas (OUT OF SCOPE)
- **REAP-01 implementation** — Done-TTL session reaper (D-86/D-87): decided here, ships in **Phase 9** alongside cleanup integration. Added to REQUIREMENTS.md and the Phase 9 roadmap section.
- Auto-removing **worktrees** on the Done TTL — explicitly declined (D-87); worktree cleanup stays manual. If disk accumulation becomes a pain, the parked MAINT-01 (stale-worktree purge list) is the vehicle.
- **TMUX-FUT-01** (user's own tmux server instead of dedicated socket) — already parked in REQUIREMENTS.md future section.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| TMUX-01 | "tmux" in shell dropdown only when on PATH | `AllowedShells` is the single seam (`internal/settings/validate.go:16`); converting it to a call-time function with `exec.LookPath("tmux")` feeds both save-time validation and the GET options array with zero frontend changes (options already data-driven, `SettingsField.tsx:146`). See Pattern 1. |
| TMUX-02 | Attach-or-create on `-L kangent` socket, `kangent-<task>-<n>` names, worktree cwd | `new-session -A -s <name> -c <dir>` verified on host tmux 3.4 (attach-or-create idempotent, `-c` cwd confirmed via `pane_current_path`). Name minting from `tmux_sessions` (migration 00005). See Patterns 2–4, Empirical Findings 1, 13. |
| TMUX-03 | Implicit detach on leaving task view; reopen reattaches; clean repaint | Within a server lifetime this is the EXISTING TERM-05 behavior: WS detach never touches the PTY; reattach = ring replay + SIGWINCH jiggle (`session.go Resize forceRedraw`), which already forces full-screen TUIs (claude) to repaint. tmux client repaints identically. **No new mechanism needed** — see Pattern 6. |
| TMUX-04 | × kills the tmux session (kill-on-close parity) | Per-session stop strategy set at spawn: tmux Stop = `kill-session -t =name`, which makes the attach client exit `[exited]`/code 0 and flow through the unchanged waitExit→markExited path. SIGTERM-only stop proven insufficient (Empirical Finding 9). See Pattern 5. |
| TMUX-06 | Inner-shell exit shows exited state; exit vs detach via `has-session` | Client exit codes are 0 in ALL cases (exit, kill, detach) — `has-session -t =name` after attach-PTY exit is the ONLY discriminator, empirically confirmed (Findings 6–8). Wire into `waitExit` before `markExited`. See Pattern 7. |
| TMUX-07 | Plain bash + agent stop behavior unchanged, regression-guarded | Stop strategy is a per-session field assigned in `Manager.Spawn`; default strategy is the byte-identical current `signalSession`/grace/sweep path. Regression tests assert bash/agent sessions get the default strategy and existing Stop tests stay green. See Pattern 5, Testing notes. |
</phase_requirements>

## Summary

This phase makes tmux a third party in an existing, well-tested PTY pipeline — and the central research result is that **almost nothing in that pipeline changes**. A tmux attach client is just another full-screen PTY child (like claude): spawn it under `creack/pty`, let the pump/ring/WS transport carry it untouched, and the existing reattach jiggle already produces the clean repaint TMUX-03 asks for. Within a server lifetime, "leaving the task view" already detaches nothing server-side (TERM-05) — the attach PTY simply stays alive, so TMUX-03 requires zero new mechanism in Phase 8.

The two genuinely new mechanics are both lifecycle, and both were verified empirically on the host's tmux 3.4: (1) **stop must be `kill-session`, not signals** — SIGTERM to the attach client kills only the client and leaves the session running (the proven "Stop doesn't stop" trap: the tmux server daemonizes into its own process session, unreachable by the `/proc` sweep); (2) **exit vs detach is discriminable only via `tmux has-session`** — the attach client exits 0 in every case (inner `exit 7`, `kill-session`, detach), so the client's exit code carries no information. Both land as a per-session stop strategy and a post-`Wait` liveness probe, decided at spawn time per the locked decision.

Two host-specific quirks matter for the plan: tmux 3.4 **prefix-matches bare session names** (`has-session -t kangent-99` matches `kangent-99-1` — `=` exact match is mandatory everywhere), and tmux 3.4 rejects bare `=name` for *pane*-target commands (`send-keys`, `capture-pane`) — use `=name:` there (only relevant to tests; the production surface is all target-*session* commands where `=name` works). For config injection, the empirical answer is decisive: a generated `-f` config file applied on every invocation is deterministic; the command-sequence alternative (`new-session \; set-option …`) raced to "server exited unexpectedly" once in testing.

**Primary recommendation:** Build `internal/tmux` as a pure args/exec leaf package; thread one new `SpawnOpts.TmuxName` through `Manager.Spawn` with a stop-strategy field set at spawn; mint names from a `tmux_sessions` table (DB `MAX(n)+1`, never the in-memory counter); generate the tmux config file at startup and pass `-f` + `-L kangent` on every invocation; leave PTY/WS/ring transport byte-identical.

## Standard Stack

### Core

No new Go modules and no new npm packages. The entire phase is stdlib + the system tmux binary.

| Component | Version | Purpose | Why Standard |
|-----------|---------|---------|--------------|
| system `tmux` via `os/exec` | host: 3.4 (verified) | Durable shell sessions | Same shell-out posture as git worktrees (project CLAUDE.md: drive the real binary, parse machine-readable output, never reimplement) |
| `github.com/creack/pty` | v1.1.24 (already in go.mod) | PTY for the attach client | The attach client is a full-screen PTY child exactly like claude — `pty.StartWithSize` unchanged |
| `github.com/pressly/goose/v3` | v3.27.1 (already in go.mod) | Migration 00005 (`tmux_sessions`) | Existing embedded-migrations pattern (`internal/store/migrations/`) |
| `exec.LookPath` | stdlib | TMUX-01 PATH detection + D-84 honest spawn error | Mirrors the existing shell LookPath-before-PTY seam in `Manager.Spawn` (~line 169) |

### Supporting

| Component | Purpose | When to Use |
|-----------|---------|-------------|
| `exec.CommandContext` + timeout | All `internal/tmux` invocations (`has-session`, `kill-session`) | Always — a hung tmux binary must never block `Stop()` indefinitely (recommend ~5s ctx timeout, matching the D-14 grace scale) |
| `script -qec` (util-linux) | Test-only PTY allocation for integration tests of the attach client | Only in tests that can't use `creack/pty` directly; prefer `pty.Start` in Go tests |

**Installation:** nothing to install. `tmux 3.4` is on the host PATH (verified `tmux -V`).

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| shelling out to tmux | a Go tmux control-mode (`-CC`) client library | Control mode replaces the PTY byte stream with a protocol — would force a parallel transport next to the PTY/WS pipeline the roadmap explicitly locks as untouched. Rejected. |
| generated `-f` config file | post-create `set-option` command sequence | Command sequence empirically raced ("server exited unexpectedly") on fresh server start; `-f` is read exactly once at server start and is deterministic. Use `-f`. |
| `kill-session` for × | SIGTERM/SIGKILL the attach client | Empirically kills ONLY the client; session survives = "close doesn't kill" bug. Signals remain only as the fallback if the tmux CLI itself errors. |

## Architecture Patterns

### Recommended Structure

```
internal/
├── tmux/              # NEW leaf package — no imports of session/api/store
│   ├── tmux.go        # SocketName, BaseArgs(confPath), NewSessionArgs, HasSession, KillSession, (DetachClient)
│   └── tmux_test.go   # real-tmux tests on a per-test -L socket
├── session/
│   ├── manager.go     # Spawn: +TmuxName branch in the KindBash arm; stop strategy assigned here
│   └── session.go     # Session: +tmuxName, +stop strategy field; waitExit: +has-session probe
├── api/
│   └── sessions.go    # create handler: shell=="tmux" → mint name from DB, pass TmuxName; honest D-84 error copy
├── store/migrations/
│   └── 00005_tmux_sessions.sql
└── settings/
    └── validate.go    # AllowedShells: var → call-time function with LookPath("tmux")
```

### Pattern 1: Conditional shell option (TMUX-01)

`AllowedShells` is consumed in exactly two places — `Validate` (save-time) and `entryFor` (GET options). Convert the var to a function so both stay one source of truth and the option tracks PATH truth at read time:

```go
// internal/settings/validate.go
// AllowedShells returns the curated shell set (SHELL-01). "tmux" is offered
// only while the binary resolves on PATH (TMUX-01) — checked at call time so
// install/uninstall is reflected without a restart. LookPath on localhost is
// microseconds; both call sites (save-time validation, GET options) share it.
func AllowedShells() []string {
    shells := []string{"bash"}
    if _, err := exec.LookPath("tmux"); err == nil {
        shells = append(shells, "tmux")
    }
    return shells
}
```

Frontend: **zero changes** — the dropdown maps `entry.options` (`SettingsField.tsx:146`), which was built for exactly this (SHELL-FUT-01 seam). Note the honest edge this buys: if tmux disappears from PATH, the option vanishes from the dropdown AND re-saving "tmux" is rejected, while the already-stored `shell=tmux` value still flows to spawn where D-84's honest error fires.

### Pattern 2: `internal/tmux` leaf package surface

Pure argv-building + thin exec wrappers. Target-**session** commands with mandatory `=` exact match:

```go
// internal/tmux/tmux.go — verified against host tmux 3.4
const Socket = "kangent"

// BaseArgs prefixes EVERY tmux invocation: dedicated socket + Kangent config.
// -f is read only when an invocation starts the server, so passing it always
// makes config application deterministic regardless of which call wins.
func BaseArgs(confPath string) []string {
    return []string{"-L", Socket, "-f", confPath}
}

// NewSessionArgs: attach-or-create, exact name, worktree cwd.
// NOTE: no -d — the caller runs this under creack/pty as the foreground
// attach client. -c applies only at creation; names are fresh per tab.
func NewSessionArgs(confPath, name, dir string) []string {
    return append(BaseArgs(confPath), "new-session", "-A", "-s", name, "-c", dir)
}

// HasSession: exit 0 = alive; exit 1 = dead (covers BOTH "can't find session"
// and "no server running" — empirically identical exit codes). Distinguish
// exec/binary errors from a clean exit-1 so callers never misread "tmux
// missing" as "session dead".
func HasSession(ctx context.Context, confPath, name string) (bool, error)

// KillSession: kill-session -t =name. Exit 1 + "no server running"/"can't
// find session" is success-idempotent (already dead).
func KillSession(ctx context.Context, confPath, name string) error
```

`DetachClient` (`detach-client -s =name`, verified working) is **not needed in Phase 8** — leaving the task view keeps the attach PTY alive (Pattern 6), and server shutdown kills attach clients via PTY hangup for free. Include it in the package only if Phase 9 wants explicit detach at graceful shutdown; otherwise defer.

### Pattern 3: Config injection — generated `-f` file (resolves the discretion point)

Empirically decided: write the config file once at startup, pass `-f` on every invocation.

```
# kangent-managed tmux config (regenerated at startup — do not edit)
set -g status off
set -g mouse on
set -g history-limit 50000
```

- Write at Kangent startup (e.g., next to the SQLite DB in the data dir, or `os.CreateTemp` per process) **before** any spawn; content is the three lines above (`history-limit` is discretionary but cheap — D-80 makes tmux history THE scrollback, and the tmux default is only 2000 lines).
- Verified: options from `-f` apply when the server starts; later invocations with a different `-f` do NOT reset them; an already-running server (Phase 9 restart case) keeps its options — correct, since they were set by the same file.
- Verified: the command-sequence alternative (`new-session -d -s x \; set-option -g status off …`) failed once with "server exited unexpectedly" on a fresh server — racy, rejected.

### Pattern 4: Name minting in the HTTP handler (TMUX-02)

Migration 00005, identity only — never status:

```sql
-- +goose Up
CREATE TABLE tmux_sessions (
    id         INTEGER PRIMARY KEY,
    task_id    INTEGER NOT NULL REFERENCES tasks(id),
    n          INTEGER NOT NULL,
    name       TEXT    NOT NULL UNIQUE,   -- kangent-<task>-<n>
    label      TEXT    NOT NULL,          -- "Bash N" tab label (Phase 9 resume display)
    created_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    UNIQUE(task_id, n)
);
-- +goose Down
DROP TABLE tmux_sessions;
```

- `n` = `SELECT COALESCE(MAX(n),0)+1 FROM tmux_sessions WHERE task_id = ?` — **never** the Manager's in-memory `taskCounters` (resets on restart; names must stay unique across restarts for Phase 9 reattach).
- Name: `fmt.Sprintf("kangent-%d-%d", taskID, n)` — task IDs are int64, so the charset is `[A-Za-z0-9-]`, inside the required `[A-Za-z0-9_-]`. (tmux silently rewrites `.`/`:` to `_` in names — verified — but this charset never triggers it.)
- Insert the row BEFORE `Spawn` (reserves `n` under the UNIQUE constraint), delete it if `Spawn` fails. Mirrors the existing persist-before-reply posture of `claude_session_id`.
- Storing `label` keeps the table identity-only while giving Phase 9's reconcile a display name; recommended but planner's call.
- Rows are NOT deleted on kill in Phase 8 — `MAX(n)+1` stays monotonic and Phase 9's lazy GC (roadmap: "DB-derived detached entries… lazy row GC") owns cleanup. Keeps Phase 8's session package free of store imports.

### Pattern 5: Per-session stop strategy (TMUX-04, TMUX-07 — the regression-risk control)

Assigned in `Spawn`, never branched at stop time:

```go
// session.go — new fields
type Session struct {
    // ...
    tmuxName string        // "" = not tmux-backed
    killer   func() error  // non-nil: how Stop terminates the underlying work
                           // (assigned ONCE in Spawn — the locked per-session
                           // lifecycle property; nil = default signal path)
}

// Stop(): identical shape; the strategy slots in before the signal fallback.
s.stopOnce.Do(func() {
    s.mu.Lock(); s.stopRequested = true; s.mu.Unlock()
    if s.killer != nil {
        if err := s.killer(); err == nil {
            select {
            case <-s.done:           // kill-session → server drops the client →
                return               // attach client exits → waitExit fires
            case <-time.After(s.termGrace):
            }
        }
        // tmux CLI failed or client didn't die: fall through to signals —
        // at minimum the attach client dies and the session shows exited.
    }
    s.signalSession(syscall.SIGTERM)
    // ... existing grace + SIGKILL + /proc sweep unchanged
})
```

- For bash/agent sessions `killer` is nil → **byte-identical current behavior** (TMUX-07).
- For tmux sessions, `killer = func() error { return tmux.KillSession(ctx, conf, name) }` — verified: `kill-session` makes the attach client print `[exited]` and exit 0, flowing through the untouched `waitExit → markExited` machinery; `stopRequested=true` keeps the banner gray.
- The `/proc` sweep stays as the fallback tail for ALL sessions — for tmux it can only ever reach the attach client (the server daemonizes into its own process session — empirically confirmed: server sess ≠ client sess), which is exactly the safe degradation: worst case is a detach, never a leak of Kangent-owned processes.
- `StopAllForTask` and the stop/× HTTP handler need **zero changes** — they call `Stop()`, which now does the right thing per session. Cleanup (D-32) therefore kills attached tmux sessions for free.

### Pattern 6: Implicit detach = do nothing (TMUX-03, resolves the discretion point)

Recommendation: the attach PTY **simply stays alive server-side** — no `detach-client` calls anywhere in Phase 8.

- Leaving the task view unmounts `TerminalPane` → WS closes → `sess.Detach(connID)` — which by TERM-05 contract never touches the PTY. The tmux attach client keeps running exactly like a backgrounded claude session today.
- Reopening the task view reattaches the WS: ring replay (last tmux screen) + the existing once-per-attach SIGWINCH jiggle (`Resize(_, _, forceRedraw=true)`) → tmux performs a full repaint. This is the same accepted pattern agent tabs use; "clean single repaint" comes free.
- Kangent server death (Phase 9's concern, but worth locking the mental model): there is no shutdown teardown in `cmd/kangent/main.go` — when the process dies, every ptmx fd closes, the PTY slaves hang up, bash/claude/attach-clients die via SIGHUP, and the daemonized tmux server survives. Durability across restarts requires **zero shutdown code**.
- Ring-buffer replay: unchanged in Phase 8. The "skip replay for tmux tabs" roadmap note is about **cross-generation** replay (Phase 9's fresh attach client must not get the previous generation's bytes); within Phase 8 every Session's ring is its own generation.

### Pattern 7: Exit-vs-detach discrimination in waitExit (TMUX-06)

```go
func (s *Session) waitExit() {
    _ = s.cmd.Wait()
    code := ... // existing
    if s.tmuxName != "" {
        alive, err := tmux.HasSession(ctx, conf, s.tmuxName) // ~5s ctx timeout
        if err == nil && alive {
            // Attach client died but the tmux session lives: the user detached
            // manually (Ctrl+B d) or something killed only the client.
            // Phase 8 handling: see Open Question 1.
        }
        // alive==false (exit 1): genuine end — inner shell exited, × kill, or
        // external kill (D-82) — proceed to the normal exited path.
    }
    s.markExited(code)
    _ = s.ptmx.Close()
    close(s.done)
}
```

Empirical truth table this is built on (all on tmux 3.4):

| Event | Client output | Client exit code | `has-session -t =name` after |
|---|---|---|---|
| inner shell `exit 7` | `[exited]` | **0** | 1 (dead) |
| `kill-session -t =name` | `[exited]` | **0** | 1 (dead) |
| detach (`detach-client` / Ctrl+B d) | `[detached (from session X)]` | **0** | 0 (alive) |
| SIGTERM to attach client | (none) | 143 | 0 (alive) |

The client exit code carries **no information** about the inner shell (always 0 on clean ends) — `has-session` is the only discriminator, and the inner shell's real exit code is unrecoverable. tmux tabs will show "exited (code 0)" regardless of the inner exit code; with `stopRequested` already discriminating ×-stops (gray), this is acceptable and should be documented in the plan, not fought.

### Pattern 8: Spawn-path changes (Manager + handler)

`Manager.Spawn` — extend the existing KindBash arm; LookPath-before-PTY posture preserved:

```go
// SpawnOpts: +TmuxName string  (bash-only; "" = plain shell)
if opts.TmuxName != "" {
    bin, err := exec.LookPath("tmux")
    if err != nil {
        return nil, ErrTmuxNotFound // sentinel — handler maps to D-84 copy
    }
    cmd = exec.Command(bin, tmux.NewSessionArgs(confPath, opts.TmuxName, dir)...)
    cmd.Dir = dir
    cmd.Env = []string{ /* same explicit minimal env as plain bash */ }
} else { /* existing shell path untouched */ }
```

- **Env scrubbing is already satisfied**: bash sessions use an explicit allow-list env (TERM, COLORTERM, HOME, PATH, LANG, USER, SHELL — `manager.go:177`) that never contained `TMUX`/`TMUX_PANE`. Reuse it verbatim (drop or keep `SHELL` — inside tmux the server sets the user's shell anyway). Also verified: even with `TMUX` set in the client's env, attaching to a *different* `-L` socket works on 3.4 (no nesting guard) — the scrub is belt-and-braces, not load-bearing.
- Handler (`internal/api/sessions.go create`): after the existing `settings.Get(KeyShell)` read, when the value is `"tmux"` mint the name (Pattern 4) and set `opts.TmuxName` instead of `opts.Shell`. The label counter stays the Manager's (in-memory "Bash N") — store the resulting label in the row.
- D-84 honest error: handler maps `errors.Is(err, session.ErrTmuxNotFound)` to a 500/409 with `{"error":"tmux not found — change the shell setting or reinstall"}`. Frontend: `TaskPage.tsx:371` currently hardcodes "Couldn't start a session. Try again." — render `spawn.error.message` (an `ApiError` whose message is body.error) when present, falling back to the generic copy. This is the phase's only frontend change.

### Anti-Patterns to Avoid

- **`if isTmux` branches inside `Stop()`/handlers:** locked out — the strategy is a field assigned at spawn (Pattern 5).
- **Bare session-name targets:** `has-session -t kangent-99` PREFIX-matched `kangent-99-1` on the host (exit 0!). Every target must be `=name`.
- **A new session `Kind`:** locked out — tmux tabs are `KindBash` + `tmuxName`; the frontend must keep seeing `kind:"bash"` so tabs stay indistinguishable (D-77).
- **Status/lifecycle columns in `tmux_sessions`:** locked to identity-only; tmux itself is the status authority (Phase 9 probes it).
- **`remain-on-exit` or auto-respawn:** explicitly out of scope (REQUIREMENTS Out of Scope).
- **Serializing/replaying xterm scrollback across generations for tmux tabs:** tmux owns history (D-80); replay-over-redraw garbles the screen.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Session liveness | `/proc` scanning for the tmux server/inner shell | `tmux has-session -t =name` (exit code) | The server daemonizes out of reach; has-session is the single authoritative probe (verified exit codes: 0 alive, 1 dead — both for "session gone" and "server gone") |
| Durable shell state | xterm.js buffer serialization, custom scrollback persistence | tmux history (`history-limit` in the `-f` config) | Already ruled out in REQUIREMENTS; tmux repaints itself on attach |
| Killing the inner shell | signaling process trees across sessions | `tmux kill-session -t =name` | Signals can only reach the attach client (proven); kill-session ends the inner shell, the session, and (when last) the server atomically |
| Config application | tracking "did I configure this server yet" state | `-f <conffile>` on every invocation | tmux reads `-f` exactly once at server start — passing it always makes "whichever call starts the server" deterministic (verified) |

**Key insight:** tmux already solved every hard problem in this phase (durability, repaint, liveness, teardown). The work is wiring its three CLI verbs into the existing seams without disturbing them.

## Empirical Findings (host tmux 3.4, 2026-06-12)

All commands run on dedicated test sockets (`-L ktest*`), cleaned up with `kill-server` afterward.

1. **Attach-or-create:** `tmux -L S -f /dev/null new-session -A -d -s kangent-99-1 -c /tmp` → exit 0, session created, pane cwd `/tmp` (verified via `list-panes -F '#{pane_current_path}'`). Re-running with the session existing and **no tty** fails: `open terminal failed: not a terminal` exit 1 — irrelevant in production (always under creack/pty) but relevant to tests.
2. **Exact match is mandatory:** `has-session -t =kangent-99-1` → 0; `has-session -t =kangent-99` → 1 (`can't find session`). **Without `=`: `has-session -t kangent-99` → 0** — prefix-matched the longer name.
3. **tmux 3.4 pane-target quirk:** `send-keys -t =t1` / `capture-pane -t =t1` fail with `can't find pane: =t1`; the `=t1:` form works. Target-*session* commands (`has-session`, `kill-session`, `detach-client -s`) accept bare `=t1`. Production surface is all target-session; tests using send-keys need `=name:`.
4. **kill-session exit codes:** live → 0; dead session / dead server → 1 + stderr (`can't find session` / `no server running on /tmp/tmux-1000/S`). Treat exit-1 as idempotent success in `KillSession`.
5. **Server lifecycle:** killing the last session exits the server; the socket file lingers in `/tmp/tmux-$UID/` and is reused cleanly by the next server start (verified across many restarts).
6. **Inner shell exit:** `exit 7` inside → client prints `[exited]`, **client exit code 0** (inner code lost), `has-session` → 1.
7. **kill-session while attached:** client prints `[exited]`, exits 0, `has-session` → 1. Indistinguishable from inner exit at the client — fine, both mean "ended".
8. **Detach while attached:** `detach-client -s =name` → client prints `[detached (from session X)]`, exits 0, `has-session` → **0** (alive). Exit code identical to exit/kill — `has-session` is the only discriminator.
9. **SIGTERM the attach client:** client dies (exit 143 path), **session survives** (`has-session` → 0). `ps` confirms `tmux: server` runs in its own process session (daemonized) — the existing `/proc` session sweep cannot reach it. The "Stop doesn't stop" trap is real.
10. **`-f` config:** `set -g status off`, `set -g mouse on`, `set -g history-limit 50000` from a `-f` file all applied at server start (verified via `show-options -g`); a second invocation with `-f /dev/null` changed nothing (config read once at server start).
11. **Command-sequence config (`new-session -d \; set-option …`):** failed once with `server exited unexpectedly` (race at fresh server start), succeeded on retry — nondeterministic, rejected in favor of `-f`.
12. **Nesting guard:** with `TMUX=/tmp/fake,123,0` in the client env, attaching on a different `-L` socket worked normally (no "nested with care" error) on 3.4. Env scrubbing is hygiene, not a functional requirement.
13. **Name mangling:** `new-session -s 'bad.name'` succeeds but silently creates `bad_name` (`.`/`:` → `_`); `kangent-<int>-<int>` never triggers this.
14. **`list-sessions -F '#{session_name}'`** emits clean one-per-line names (Phase 9's reconcile input).

## Common Pitfalls

### Pitfall 1: Stop that only signals — "close doesn't kill"
**What goes wrong:** × on a tmux tab SIGTERMs the attach client; the tab closes but `top` keeps running in a hidden tmux session forever.
**Why:** the tmux server daemonizes; the inner shell is the *server's* child, not the attach client's.
**Avoid:** stop strategy = `kill-session` first (Pattern 5); signals only as fallback.
**Warning sign:** after closing a tab, `tmux -L kangent list-sessions` still shows the name.

### Pitfall 2: Bare-name prefix matching
**What goes wrong:** `kill-session -t kangent-1-1` kills `kangent-1-10` (or has-session reports the wrong session alive) once a task has ≥10 tabs ever spawned.
**Avoid:** `=name` on every target; a regression test with two prefix-sharing names is cheap and worth it.

### Pitfall 3: Trusting the attach client's exit code
**What goes wrong:** treating client exit 0 as "clean shell exit" or trying to surface the inner shell's exit code — it's always 0 (Finding 6–8).
**Avoid:** status comes from `has-session`; the exited banner shows code 0 for tmux tabs; `stopRequested` (not exit code) discriminates ×-stops. Don't add code-based logic for tmux tabs.

### Pitfall 4: Minting `n` from the in-memory counter
**What goes wrong:** Manager's `taskCounters` resets on restart → name collision with a surviving tmux session → `new-session -A` silently **attaches a second tab to the same shell** (two tabs, one shell, interleaved input).
**Avoid:** `n` from `tmux_sessions` `MAX(n)+1` (Pattern 4); UNIQUE constraints as the backstop.

### Pitfall 5: Blocking Stop on a hung tmux CLI
**What goes wrong:** `kill-session` exec with no timeout inside `Stop()` (which the DELETE-worktree handler calls inline) hangs the cleanup request.
**Avoid:** `exec.CommandContext` with a short timeout in every `internal/tmux` call; on timeout fall through to the signal path.

### Pitfall 6: Misreading `has-session` errors
**What goes wrong:** tmux binary missing/broken at probe time → treating the exec failure as "session dead" → falsely marking a live session exited (violates D-82's honesty in the other direction).
**Avoid:** `HasSession` returns `(bool, error)`; exit-code-1 is `false, nil`; exec/start failures are `false, err` and the caller logs + defaults to the exited path explicitly (documented choice, not an accident).

### Pitfall 7: Touching the transport
**What goes wrong:** adding tmux-special replay/redraw logic to ws/handler or the ring — risks regressing claude tabs (the resize-storm bug class).
**Avoid:** Phase 8 transport diff must be zero. The existing jiggle is sufficient (claude tabs prove it daily).

### Pitfall 8: Tests spawning tmux without a PTY
**What goes wrong:** `exec.Command("tmux", ..., "new-session", "-A", ...).Run()` in a test fails `open terminal failed: not a terminal` when attaching (Finding 1).
**Avoid:** integration tests drive `Manager.Spawn` (which allocates the PTY) or create sessions with `-d`; each test uses a unique `-L kangent-test-<pid>` socket with `kill-server` cleanup in `t.Cleanup`.

## Code Examples

### tmux invocations (verified verbatim on host)

```bash
# spawn (production shape — run under creack/pty, no -d):
tmux -L kangent -f /path/to/kangent-tmux.conf new-session -A -s kangent-12-3 -c /home/u/worktrees/task-12

# liveness probe (exit 0 alive / 1 dead):
tmux -L kangent -f /path/to/kangent-tmux.conf has-session -t =kangent-12-3

# × kill:
tmux -L kangent -f /path/to/kangent-tmux.conf kill-session -t =kangent-12-3

# Phase 9 reconcile input:
tmux -L kangent list-sessions -F '#{session_name}'

# user escape hatch (README note, Phase 9):
tmux -L kangent kill-server
```

### Regression-test shape for the shared stop path (TMUX-07)

```go
// 1. Existing Stop tests (bash SIGTERM→grace→KILL, /proc sweep) must pass
//    unmodified — they ARE the regression guard.
// 2. New: spawned bash/agent sessions have a nil killer (default strategy).
// 3. New: tmux session Stop calls kill-session (observe via a fake `tmux`
//    script on PATH recording argv, or assert has-session==1 with real tmux).
// 4. New: prefix-collision test — sessions kangent-1-1 and kangent-1-10 on a
//    test socket; killing -1 must leave -10 alive.
// 5. New: exit-vs-detach — real tmux, send-keys 'exit' vs detach-client, then
//    assert session status exited vs (Open Q1 behavior).
```

## State of the Art

| Old Approach (pre-amendment) | Current Approach | Changed | Impact |
|---|---|---|---|
| × detaches, separate kill affordance | × kills (D-78), detach implicit on leaving task view | 2026-06-12 discussion | No new tab UI at all; stop handler unchanged |
| Vanilla `-f /dev/null` server | Kangent-controlled config: status off, mouse on | 2026-06-12 (D-79/D-80) | Generated conf file; "injecting tmux config" out-of-scope row reversed |
| — | tmux 3.4 `=name` pane-target regression | found empirically | Production unaffected (target-session only); tests use `=name:` |

## Open Questions

1. **Detach-while-alive (user presses Ctrl+B d) — what does the tab show in Phase 8?**
   - What we know: D-81 keeps the prefix, so manual detach is possible; the attach PTY exits, `has-session` says alive; Phase 8 has no `resumable` flag (that's Phase 9's TMUX-05).
   - What's unclear: exited banner (slightly dishonest — the shell still runs) vs auto-reattach (a new Session respawn — real machinery for a rare power-user edge).
   - **Recommendation:** show the existing exited banner (code 0, gray via no-stopRequested→actually plain exited) and leave the `tmux_sessions` row in place; the session keeps running and Phase 9's reconcile/resume naturally upgrades this case to Resume. Record the discrimination result (e.g., internal `detachedAlive` bool) so Phase 9 can read it. Do NOT build auto-reattach in Phase 8.
2. **Where the generated conf file lives.**
   - Recommendation: data dir next to the SQLite DB (stable path, survives reboots so a tmux server started in a previous boot session still references an existing file); regenerate (overwrite) at every Kangent startup. Temp-dir works too but risks dangling `-f` paths after reboot cleanups while a tmux server persists.
3. **Does the exited banner's "code 0" need copy adjustment for tmux tabs?**
   - Recommendation: no — indistinguishability (D-77) argues for identical copy; the code is truthfully the attach client's. Defer any copy change unless verification feedback flags it.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| tmux | entire phase | ✓ | 3.4 | option absent from dropdown; D-84 honest spawn error |
| Go toolchain | build/test | ✓ | go 1.26 line (go.mod) | — |
| `script` (util-linux) | optional test PTY harness | ✓ | — | use creack/pty in Go tests |

**Missing dependencies with no fallback:** none.

## Sources

### Primary (HIGH confidence)
- **Empirical verification on host tmux 3.4** (2026-06-12, this session): attach-or-create, `=` exact vs prefix matching, the 3.4 pane-target `=name` quirk, has-session/kill-session exit codes, client exit-code/output matrix for exit vs kill vs detach vs SIGTERM, server daemonization (`ps` session IDs), `-f` config semantics + command-sequence race, TMUX-env nesting behavior on a separate socket, name mangling, `-c` cwd, socket-file reuse. All test sockets killed and cleaned.
- **Codebase reads:** `internal/settings/validate.go`, `internal/api/settings.go`, `internal/api/sessions.go`, `internal/session/manager.go`, `internal/session/session.go`, `internal/ws/handler.go`, `internal/store/migrate.go` + migrations 00001–00004, `cmd/kangent/main.go` (no shutdown teardown), `web/src/pages/TaskPage.tsx`, `web/src/components/task/TaskTabs.tsx`, `web/src/api/sessions.ts`, `web/src/components/settings/SettingsField.tsx`.
- `.planning/ROADMAP.md` Phase 8/9 sections, `.planning/REQUIREMENTS.md` (post-amendment), `08-CONTEXT.md`.

### Secondary (MEDIUM confidence)
- tmux man page knowledge (training data, cross-checked empirically where load-bearing): `-f` read-at-server-start, session-option scope of `status`/`mouse`, `history-limit` default 2000, mouse mode emitting SGR sequences that xterm.js forwards (the mouse-on UX itself needs eyes-on verification during execution — it cannot be tested headlessly).

### Tertiary (LOW confidence)
- None — no web searches were needed; the domain is the host binary, which was tested directly.

## Project Constraints (from CLAUDE.md)

- Shell out to system binaries with arg-array exec (never `sh -c`), parse machine-readable output — the git-worktree posture extends to tmux (`-F` formats, exit codes).
- PTY work stays on `creack/pty` v1 (never the orphaned v2 import path).
- WebSocket transport stays binary-frame `coder/websocket`; PTY/WS pipeline untouched this phase.
- SQLite via `modernc.org/sqlite` + goose embedded migrations (`sqlite3` dialect string) — migration 00005 follows the existing pattern.
- GSD workflow: implementation goes through `/gsd:execute-phase`; this document feeds `/gsd:plan-phase`.
- No co-author/Claude mentions in commits (user global instruction).

## Metadata

**Confidence breakdown:**
- tmux lifecycle semantics (spawn/kill/exit/detach/discrimination): HIGH — all verified on the exact host binary the feature will run against
- Architecture integration points (seams, stop strategy, minting): HIGH — code read directly; patterns mirror existing in-repo precedents (LookPath seam, persist-before-reply, settings read-at-use)
- Mouse-mode scrollback UX inside xterm.js: MEDIUM — mechanism is standard (tmux mouse reporting over the PTY), but the feel (wheel → copy-mode history) needs eyes-on confirmation during execution
- tmux 3.4 quirks portability: MEDIUM — the `=name` pane-target quirk and nesting-guard behavior are 3.4-specific observations; production only uses target-session commands, so other versions are unlikely to differ where it matters

**Research date:** 2026-06-12
**Valid until:** stable (~60 days) — host tmux version and the codebase seams are the only inputs; re-verify the Empirical Findings if the host tmux is upgraded past 3.4

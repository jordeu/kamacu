# Feature Research

**Domain:** Kangent v1.2 — (a) LLM quota/usage indicators for Claude subscriptions; (b) resumable (tmux-backed) terminal sessions in a web terminal app
**Researched:** 2026-06-11
**Confidence:** HIGH for SlayZone UX (read its actual source), HIGH for the OAuth usage endpoint shape (SlayZone source + 4 independent community tools), MEDIUM for Claude Code `/usage` exact rendering (docs + community descriptions, not observed live), HIGH for tmux conventions (stable man-page behavior + corroborating sources)

> Supersedes the v1.0 feature-landscape research (2026-06-10) for milestone v1.2 planning. v1.0 features are shipped and validated in PROJECT.md; this file covers only the two new features.

## How the Reference Implementations Actually Work

### (a) SlayZone's usage indicator — from source, not screenshots

Read directly from `packages/apps/app/src/renderer/src/components/usage/UsagePopover.tsx`, `useUsage.ts`, and `packages/domains/terminal/src/electron/usage.ts` (main branch, 2026-06-11).

**Data source.** `GET https://api.anthropic.com/api/oauth/usage` with headers:
- `Authorization: Bearer <oauth access token>`
- `anthropic-beta: oauth-2025-04-20`
- `User-Agent: claude-code/<version>` (version read from `claude --version`, fallback constant)

Token discovery on Linux: `~/.claude/.credentials.json` → `claudeAiOauth.accessToken` (macOS uses the "Claude Code-credentials" keychain entry; SlayZone's source comment confirms the Linux file path against a real user issue, #51). Response fields mapped: `five_hour`, `seven_day`, `seven_day_opus`, `seven_day_sonnet` — each `{ utilization: number /* 0–100 */, resets_at: ISO-8601 }`, each nullable (windows absent for some plans). Corroborated by Usage4Claude (which additionally surfaces an "extra usage" balance), jens-duttke/usage-monitor-for-claude, and ClaudeBar — this endpoint is the de-facto community standard; nobody scrapes the `/usage` TUI.

**Compact trigger (top bar).** Tiny inline bars (48×6px) each with a 10px label like "Claude 5h" — which windows appear inline is *pinnable* from the popup (default: first window per provider), persisted as a setting. If any provider has an error, a small yellow warning triangle appears next to the bars with a tooltip carrying the error text. Hover opens the popup (200ms close-delay grace so the pointer can travel into it).

**Color thresholds** (`barColor`): `pct >= 85` → red, `pct >= 60` → yellow, else green. ClaudeBar independently uses the same green/yellow/red scheme; this is the genre convention.

**Popup (w-72).** Per provider: a section header, then one row per window: pin dot · label (`5h`, `7d`, `Opus`, `Son.`) · horizontal bar (color-thresholded, width = min(pct,100)) · `42%` (rounded, tabular-nums) · reset countdown under a "Resets in" column header. Countdown format: `now` / `37m` / `4h 12m` / `2d 5h` (duration, not wall-clock time). Footer row: `Updated 3m ago` (left) + a refresh icon button (right) that forces a fetch bypassing the cache.

**Polling & caching.** Frontend: TanStack Query `refetchInterval` (SlayZone uses 5 min; Kangent's spec says ~60s — fine, the backend cache absorbs it), `refetchIntervalInBackground: false` (no polling while window hidden). Backend: 60s cache TTL for auto-polls, **10s hard floor** even for forced refreshes (blocks spam-clicking), in-flight request dedup, and 429 handling that honors `Retry-After` with a 30s minimum backoff.

**Error & staleness states** (the part most tools get wrong; SlayZone handles all of them):
- **No credentials:** `Not logged in — run \`claude\` to authenticate` (no fetch attempted)
- **401:** `Token expired — re-authenticate with \`claude\`` — it does *not* try to refresh the OAuth token itself
- **403:** `Access denied — check your Claude plan`
- **5xx:** `Anthropic API error (503)`
- **Network:** friendly mapping (`Network error — check your connection`, `Request timed out`, `SSL error — VPN or proxy may be interfering`), one automatic retry after 500ms for transient socket errors
- **Stale-but-cached:** on fetch failure, last good windows keep rendering with an amber line `<error> · 14m old`; after **3 consecutive failures** cached windows are dropped so "9h old" never persists forever
- **Error with no cache:** popup section shows just the warning triangle + message; compact trigger shows the triangle
- **Account switch:** cache keyed by token; token change discards the old account's cached windows

### (a) Claude Code's own `/usage`, and API-key vs subscription users

- `/usage` (subscription users) shows a full-screen panel with percentage bars for **Current session (5h)**, **Current week — all models (7d)**, and **Current week — Opus**, each with reset time; usage is shared across claude.ai, Claude Code, and IDE surfaces. (MEDIUM — help-center + multiple community descriptions.)
- The 7d/Opus/Sonnet windows are plan-dependent and Anthropic has changed them over time (5h caps doubled May 2026) — **render whatever windows the endpoint returns, never hardcode the set**.
- **API-key users have no quota windows at all** — they're pay-as-you-go; `/usage` doesn't apply and they use `/cost`. Community tools (claude-code-usage-bar) simply *skip* the quota bars when no OAuth rate-limit data exists. For Kangent: if `~/.claude/.credentials.json` has no OAuth token, show the not-logged-in state (or hide bars); do not invent spend tracking.

### (b) Resumable terminal session conventions — tmux + VS Code

**tmux idioms (the canon for "detach ≠ kill"):**
- **Attach-or-create:** `tmux new-session -A -s <name> -c <dir>` — attaches if the session exists, creates otherwise. This single command is the standard web-terminal wrapper pattern (ttyd's documented persistent-session recipe is exactly `ttyd tmux new -A -s ttyd`).
- **Detach vs kill:** killing the *attach client* (closing the PTY) detaches; the session and its processes keep running under the tmux server, which is a separate daemon that outlives the wrapper app. Killing for real is an explicit `tmux kill-session -t <name>` — users expect a separate, deliberate action for this (tmux itself separates `detach` from `kill-session`).
- **Exit vs detach detection:** the attach client process exits in *both* cases. The reliable discriminator is `tmux has-session -t <name>` after the attach PTY exits — exit code 0 → session still alive (it was a detach); non-zero → the shell inside ended (session is dead). `tmux list-sessions -F '#{session_name}'` enumerates survivors for restart reconciliation.
- **Scrollback ownership:** tmux switches the outer terminal to the alternate screen, so xterm.js's own scrollback buffer never accumulates history — tmux's internal history (default 2000 lines/pane) is the scrollback, accessed via copy-mode (`prefix [`) or, with `set -g mouse on`, wheel-up. Users who choose tmux know this tradeoff; it's the same in every terminal emulator.
- **Sizing:** with one attached client (Kangent's PTY), the session tracks that client's size; `pty.Setsize` propagates normally. If the user *also* attaches from a real terminal (a feature — sessions on the default tmux server are reachable via `tmux attach -t kangent-...` from anywhere), tmux ≥3.1's `window-size latest` default resolves the conflict sanely.

**VS Code terminal persistence (the other major reference):**
- Window reload/restart: scrollback is serialized and **replayed into xterm.js with a visible "History restored" marker line**; amount limited by `terminal.integrated.persistentSessionScrollback` (small by default). Locally the processes die on app exit; only in remote (SSH/WSL) do processes truly survive, because a server owns them — which is exactly Kangent's architecture, with tmux extending survival past the Kangent server itself.
- Lesson: users accept *limited* scrollback restoration with a clear marker; nobody expects byte-perfect infinite history across restarts.

**Prior art in the genre:** claude-squad (surveyed in v1.0 research) is built entirely on tmux-backed sessions with worktree-per-session — the tmux-as-durability-layer pattern is established in exactly Kangent's product category.

**Mapping to Kangent's existing machinery:** within one Kangent server lifetime, the existing ring-buffer replay keeps working unchanged (the PTY is just running `tmux attach` instead of `bash`). Across a Kangent restart, the ring buffer is gone but tmux redraws the full visible screen on attach (it's a full-screen application, like claude — the existing "resize nudge after replay" pattern applies), and history beyond the screen lives in tmux copy-mode. That's the same recovery contract as the v1.1 `claude --resume` flow: process state survives, in-app scrollback doesn't.

## Feature Landscape

### Table Stakes (Users Expect These)

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Compact indicator: mini bar + "Claude 5h" label, top-right, board + task view | The whole point — at-a-glance dispatcher check; SlayZone trigger pattern | LOW | 48×6px bar, threshold color, hidden/neutral when no data |
| Hover popup: one row per window — label, color bar, rounded %, "Resets in Xh Ym" | SlayZone, `/usage`, and every menu-bar tool present exactly this | LOW | Render server-returned windows dynamically (5h, 7d, Opus, Sonnet…); duration countdown, not wall-clock |
| Green/yellow/red thresholds (<60 / 60–84 / ≥85) | Genre convention (SlayZone, ClaudeBar) | LOW | Lift SlayZone's exact numbers |
| "Updated Xm ago" + manual refresh button in popup footer | Staleness honesty is table stakes for cached quota data | LOW | Refresh = force fetch, bypass cache |
| ~60s auto-poll, paused when tab hidden | Spec'd; SlayZone uses visible-only polling | LOW | TanStack Query `refetchInterval` + `refetchIntervalInBackground: false` |
| Backend cache + rate-limit hygiene: 60s TTL, 10s hard floor, in-flight dedup, 429 Retry-After backoff (min 30s) | Hitting an undocumented OAuth endpoint every 60s without backoff risks 429s/lockout | MEDIUM | Single Go fetcher behind an API route; SlayZone's cache design maps 1:1 |
| Error states: not logged in / token expired (401) / 403 / 5xx / network — message in popup, warning triangle on trigger | Credentials expire constantly; a quota widget that errors opaquely is worse than none | MEDIUM | Copy SlayZone's message catalog; never auto-refresh the OAuth token |
| Stale-but-cached display: keep last windows + "error · Xm old", drop after ~3 consecutive failures | Transient network blips shouldn't blank the indicator | MEDIUM | Per SlayZone `MAX_STALE_FAILURES` |
| API-key-only graceful degradation | No OAuth creds → no quota windows exist; bars must not error-spam | LOW | Show "Not logged in" state or hide; never fabricate quota from transcripts |
| "tmux" in shell dropdown, only when `tmux` resolves on PATH | Mirrors v1.1 LookPath shell validation; offering a broken option is worse | LOW | Existing API-driven dropdown seam (SHELL-FUT-01) |
| Close tab = detach; reopen = reattach | The tmux contract users are choosing; tmux's own detach/kill split | MEDIUM | Spawn `tmux new-session -A -s kangent-<task>-<tab> -c <worktree>`; on tab close, kill only the attach PTY — must branch the existing full-process-tree stop by shell type |
| Explicit "kill for real" affordance, distinct from closing the tab | tmux users expect kill-session to be deliberate; otherwise sessions leak forever | MEDIUM | e.g. tab context-menu "Kill session" → `tmux kill-session -t`; tab close stays detach |
| Survive Kangent restart with Resume affordance | The headline feature; mirrors v1.1 `claude --resume` UX exactly | MEDIUM | Startup reconcile: `tmux has-session` per persisted tab → set `resumable`; Resume re-runs attach-or-create |
| Exit-vs-detach detection | A tab must show dead when the shell exits inside tmux, not "resumable" | MEDIUM | Attach PTY exited → `tmux has-session`: 0 = detached (resumable), else dead (existing exited state) |

### Differentiators (Competitive Advantage)

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| Sessions on the user's default tmux server (no `-L kangent` socket) | User can `tmux attach -t kangent-…` from any real terminal — the dispatcher cockpit extends to the shell | LOW | Deliberate decision; cost: sessions visible in user's `tmux ls` (arguably also a feature). Document the naming convention |
| Server-driven window rendering (no hardcoded 5h/7d set) | Anthropic changes plans/windows (May 2026 limit changes); UI keeps working | LOW | Render whatever `oauth/usage` returns, labels from a small key→label map with passthrough |
| Token-change cache invalidation | User re-logs-in as different account → no cross-account ghost data | LOW | Key cache by token value, per SlayZone |
| Red-zone emphasis on the compact trigger when any window ≥85% | Dispatcher glances need the "stop starting agents" signal | LOW | Cheap addition to threshold logic |
| Pinnable inline bars (choose which windows show compact) | SlayZone has it; nice for Opus-heavy users | MEDIUM | Defer — Kangent spec fixes the compact bar to 5h; revisit if asked |

### Anti-Features (Commonly Requested, Often Problematic)

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| Scraping `claude /usage` TUI output via PTY | "Use the official UI as the source" | Brittle ANSI parsing, version-dependent layout, costs a PTY + claude startup per poll | OAuth usage endpoint (community-standard, structured JSON) |
| Computing quota from local JSONL transcripts (ccusage-style estimation) | Works offline, no endpoint risk | Estimates drift from server truth; misses usage from other devices/claude.ai; the tool category converged on the endpoint | OAuth usage endpoint; show staleness honestly |
| Auto-refreshing the expired OAuth token | "Don't make me re-auth" | Token rotation races with Claude Code's own refresh; risks invalidating the CLI's session — SlayZone deliberately doesn't | 401 → "Token expired — re-authenticate with `claude`" |
| Spend/cost analytics for API-key users | "Show me something too" | Different data model (dollars, no windows, no resets); scope creep | `/cost` exists in the CLI; show not-logged-in state |
| Quota threshold notifications / auto-pause agents | Menu-bar tools do notify | Kangent has no notification system yet (NOTF-01 parked); coupling quota to agent lifecycle is policy, not display | Red bar + popup; revisit with NOTF-01 |
| Wrapping the *agent* (claude) session in tmux too | "Make everything survive restarts" | claude already has `--resume`; nesting a TUI in tmux adds redraw artifacts, prefix-key conflicts, double alternate-screen | Keep tmux for bash tabs only (per spec) |
| Injecting tmux config (hide status bar, force mouse mode, bigger history) | "Cleaner embedded look" | Overrides user's `.tmux.conf` expectations; status bar is the visual cue that this tab detaches; mouse-mode opinions are religious | Run tmux vanilla on the user's config; document copy-mode for scrollback |
| Serializing xterm.js scrollback across Kangent restarts (VS Code-style) | "Keep my history" | tmux already owns the history; replaying a stale serialized buffer *on top of* tmux's redraw produces duplicated/garbled screens | tmux redraw on attach + copy-mode for history; ring-buffer replay within server lifetime |
| `remain-on-exit` / auto-respawn of dead shells | "Never lose the tab" | A dead pane that eats keystrokes confuses more than a clean exited state | Existing dimmed-exited + Reset session pattern |
| Killing tmux sessions silently on task deletion | "Clean up automatically" | Same hazard class as worktree removal with running work | Extend the existing dirty-tree/running-session cleanup gates to cover live tmux sessions |

## Feature Dependencies

```
Quota indicator (compact bar)
    └──requires──> Quota popup data model (windows[])
                       └──requires──> Go fetcher: token discovery + oauth/usage + cache/backoff
                                          └──requires──> OAuth creds file (~/.claude/.credentials.json) [external]
Error/staleness states ──requires──> Go fetcher cache (last-good retention, failure counter)
Auto-poll (~60s) ──requires──> new GET /api/usage route + TanStack Query polling

"tmux" dropdown option
    └──requires──> existing shell-setting seam (SHELL-FUT-01, v1.1)   [exists]
    └──requires──> LookPath detection (mirrors v1.1 shell validation) [pattern exists]
Detach-on-close ──requires──> per-shell-type branch in session stop logic (currently full-process-tree kill)
Reattach on reopen ──requires──> deterministic session naming (kangent-<task>-<tab>)
Restart Resume ──requires──> v1.1 reconcile + `resumable` flag plumbing [exists]
                └──requires──> persisted tmux session name in SQLite session row
Exit-vs-detach detection ──requires──> `tmux has-session` check on PTY exit (extends cmd.Wait reaper)
Task cleanup gating ──enhances──> existing worktree cleanup gates (add: live tmux session check)

Quota feature ──independent of──> tmux feature (no shared code; safe to phase separately)
```

### Dependency Notes

- **Detach-on-close requires branching the stop path:** v1 kills the full process tree on session stop. For tmux tabs, tab-close must kill *only* the attach client PTY (detach), while the explicit "Kill session" action does `tmux kill-session`. This is the one place the existing lifecycle changes shape rather than just extends.
- **Restart Resume reuses v1.1 wholesale:** the agent Resume flow (reconcile on startup → `resumable` flag → Resume button → respawn command) is the exact template; only the probe (`tmux has-session`) and respawn command (`tmux new -A -s <name>`) differ.
- **Quota is fully parallel:** new backend route + new frontend component; touches nothing in session/PTY code. Good first or concurrent phase.
- **Cleanup gates must learn about tmux:** the dirty-tree/running-session gate on task cleanup currently sees Kangent-owned PTYs; a detached tmux session has *no* Kangent PTY but is still running work in the worktree. The gate needs the `has-session` probe too, and cleanup should offer to kill the session.

## MVP Definition

### Launch With (v1.2)

- [ ] Go usage fetcher: credentials-file token, oauth/usage call, 60s cache + 10s floor + 429 backoff, stale retention, full error catalog — the backbone everything else renders
- [ ] Compact "Claude 5h" bar + threshold colors, top-right on board and task view
- [ ] Hover popup: all returned windows with bars, %, reset countdowns; "Updated Xm ago"; manual refresh
- [ ] ~60s visible-only auto-poll
- [ ] tmux shell option (LookPath-gated) in settings dropdown
- [ ] Attach-or-create spawn, detach-on-tab-close, reattach-on-reopen, deterministic naming
- [ ] Restart reconcile + Resume affordance for surviving tmux sessions
- [ ] Exit-vs-detach detection (dead state when shell exits inside tmux)
- [ ] Explicit kill-session affordance

### Add After Validation (v1.2.x)

- [ ] Cleanup-gate integration for detached tmux sessions — as soon as first task with tmux tabs gets deleted
- [ ] Red-zone trigger emphasis — trivial once thresholds exist
- [ ] Token-change cache invalidation — if multi-account use appears

### Future Consideration (v2+)

- [ ] Pinnable inline bars (SlayZone parity) — only if users ask for Opus-at-a-glance
- [ ] Quota notifications — blocked on NOTF-01
- [ ] Extra-usage balance display — endpoint reportedly exposes it (Usage4Claude); verify empirically first
- [ ] Multi-provider usage (Codex etc.) — out of scope per PROJECT.md (Claude-only v1)

## Feature Prioritization Matrix

| Feature | User Value | Implementation Cost | Priority |
|---------|------------|---------------------|----------|
| Usage fetcher + cache/backoff/errors | HIGH | MEDIUM | P1 |
| Compact bar + popup UI | HIGH | LOW | P1 |
| Auto-poll + manual refresh | HIGH | LOW | P1 |
| tmux dropdown option (gated) | MEDIUM | LOW | P1 |
| Detach/reattach lifecycle | HIGH | MEDIUM | P1 |
| Restart Resume for tmux | HIGH | MEDIUM | P1 |
| Exit-vs-detach detection | HIGH | LOW | P1 |
| Explicit kill-session | MEDIUM | LOW | P1 |
| Cleanup-gate tmux awareness | MEDIUM | LOW | P2 |
| Stale-data retention polish (3-failure drop) | MEDIUM | LOW | P2 |
| Pinnable bars | LOW | MEDIUM | P3 |

## Competitor Feature Analysis

| Feature | SlayZone | Claude Code `/usage` | claude-squad / ttyd / VS Code | Our Approach |
|---------|----------|----------------------|-------------------------------|--------------|
| Quota data source | oauth/usage endpoint, creds file/keychain | First-party (same data) | n/a | Same endpoint, Linux creds file; verify against installed claude before planning (PROJECT.md flag) |
| Window presentation | Inline pinned mini-bars + popup rows: bar, %, "Resets in" duration | Full-screen bars: session, week, week-Opus, reset times | n/a | SlayZone popup pattern, fixed "Claude 5h" compact bar |
| Colors | green <60 / yellow 60–84 / red ≥85 | n/a (single-color bars) | n/a | SlayZone's exact thresholds |
| Staleness | "Updated Xm ago", stale windows + amber "error · Xm old", drop after 3 failures | Always live | n/a | Copy SlayZone |
| Polling | 5min visible-only frontend, 60s backend TTL, 10s floor, 429 backoff | On demand | n/a | 60s frontend (spec), same backend protections |
| API-key users | "Not logged in" error row | `/cost` instead | n/a | Not-logged-in state; no spend tracking |
| Tab close semantics | n/a (terminal sync, Cmd+R workaround) | n/a | tmux: detach; kill is explicit `kill-session`; claude-squad runs every session in tmux | Close = detach, context-menu Kill = kill-session |
| Restart survival | n/a | `--resume` (Kangent v1.1 already) | tmux server outlives wrapper; VS Code remote reconnects | `has-session` reconcile + Resume button (v1.1 pattern) |
| Scrollback across reattach | n/a | TUI redraws screen | tmux copy-mode owns history; VS Code replays limited buffer + "History restored" marker | tmux redraw + resize nudge (existing pattern); copy-mode for history; no serialization |

## Sources

- SlayZone source (HIGH, read 2026-06-11): [UsagePopover.tsx](https://github.com/debuglebowski/SlayZone/blob/main/packages/apps/app/src/renderer/src/components/usage/UsagePopover.tsx), [useUsage.ts](https://github.com/debuglebowski/SlayZone/blob/main/packages/apps/app/src/renderer/src/components/usage/useUsage.ts), [electron/usage.ts](https://github.com/debuglebowski/SlayZone/blob/main/packages/domains/terminal/src/electron/usage.ts), shared types in `packages/domains/terminal/src/shared/types.ts`
- Endpoint corroboration (MEDIUM each, HIGH in aggregate): [Usage4Claude](https://github.com/f-is-h/Usage4Claude) (5h/7d/extra/Opus/Sonnet windows), [usage-monitor-for-claude](https://github.com/jens-duttke/usage-monitor-for-claude), [ClaudeBar](https://github.com/tddworks/ClaudeBar) (green/yellow/red), [claude-code-usage-bar](https://github.com/leeguooooo/claude-code-usage-bar) (API-key users: bars skipped)
- Claude quotas & `/usage` (MEDIUM): [Models, usage, and limits in Claude Code](https://support.claude.com/en/articles/14552983-models-usage-and-limits-in-claude-code) (`/cost` for API-key users), [morphllm limits overview](https://www.morphllm.com/claude-code-usage-limits) (5h rolling window, weekly + Opus weekly, May 2026 changes), [allthings.how weekly caps](https://allthings.how/claude-code-usage-limits-explained-pro-max-and-weekly-caps/)
- VS Code persistence (HIGH): [Terminal Advanced docs](https://code.visualstudio.com/docs/terminal/advanced) (`enablePersistentSessions`, `persistentSessionScrollback`, process revive), [vscode#133516](https://github.com/microsoft/vscode/issues/133516)
- tmux behavior (HIGH, stable man-page semantics + corroboration): [tmux#3705](https://github.com/tmux/tmux/issues/3705) (wheel/copy-mode in alternate screen), [freeCodeCamp scrollback buffer](https://www.freecodecamp.org/news/tmux-in-practice-scrollback-buffer-47d5ffa71c93/) (tmux owns history; terminal scrollback unused); `new-session -A`, `has-session`, `kill-session`, `list-sessions -F` from man tmux; claude-squad (v1.0 research) as genre precedent for tmux-backed sessions

---
*Feature research for: Kangent v1.2 — Claude quota indicator + tmux-backed resumable shell tabs*
*Researched: 2026-06-11*

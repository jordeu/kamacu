# Project Research Summary

**Project:** Kangent — milestone v1.2 (Quota & Resumable Shells)
**Domain:** Claude OAuth quota/usage integration + tmux-backed resumable terminal sessions in an existing Go+React PTY app
**Researched:** 2026-06-11
**Confidence:** HIGH

## Executive Summary

v1.2 adds two fully independent features to a working codebase: (a) a Claude quota indicator (compact bar + hover popup in the board/task headers) and (b) tmux-backed shell tabs whose close action detaches instead of kills, surviving both tab closure and Kangent restarts. Research converged with unusual certainty: **zero new dependencies are needed**. The quota data comes from the undocumented-but-community-standard `GET https://api.anthropic.com/api/oauth/usage` endpoint (verified live on this machine, HTTP 200, full response captured), authenticated with the bearer token read from `~/.claude/.credentials.json`. The tmux feature is purely a different child command inside the existing creack/pty session — `tmux -L kangent new-session -A -s <name> -c <dir>` — with all tmux behaviors (attach-or-create idempotency, exit-code ambiguity, `has-session` discrimination, name sanitization) empirically verified on tmux 3.4.

The recommended approach follows two production-tested references. For quota: copy SlayZone's implementation wholesale (read from source) — server-side proxy at `GET /api/usage`, 60s cache TTL with a 10s hard floor, 429 Retry-After backoff, stale-but-cached display with a 3-failure drop, and its exact error-message catalog. For tmux: the architecture research mapped every change to existing codebase seams at line level — handler-owns-persistence (the `claude_session_id` precedent), DB-derived status via liveness probe (the Phase 5 `resumable` pattern with `tmux has-session` swapped in as the probe), and the v1.1 `AllowedShells`/LookPath seam (SHELL-FUT-01) built for exactly this.

The two key risks: **(1) the detach-vs-kill lifecycle fork is the highest regression-risk change in the milestone** — the existing Stop path's process-tree kill cannot reach the daemonized tmux server, so a naive implementation ships "Stop doesn't stop" (orphaned shells running invisibly) or "close kills" (the feature silently doesn't exist); mitigate with an explicit per-session lifecycle strategy decided at spawn time and regression tests on plain bash/agent stops. **(2) The quota endpoint is undocumented and formally gray-area** — never refresh the OAuth token (refresh-token rotation can log the user out of claude itself), always send the `claude-code/<version>` User-Agent (omitting it lands in a persistent-429 bucket), and build the indicator as strictly best-effort so upstream changes degrade rather than break the app.

## Key Findings

### Recommended Stack

No new Go modules, no new npm dependencies. Quota = stdlib `net/http` + `encoding/json` + `os.ReadFile` on the backend, existing TanStack Query (`refetchInterval: 60_000`, visible-only) on the frontend, plus shadcn copy-ins (`hover-card`, `progress` — verified absent from `web/src/components/ui/`). tmux = `os/exec` argument construction; the PTY/WS/ring-buffer transport layer is untouched because a tmux client is just another full-screen PTY child. tmux is a host prerequisite gated by `exec.LookPath` (the v1.1 shell-validation pattern), minimum version 2.1 (everything used dates from <=2015; 3.4 verified locally).

**Core technologies:**
- `net/http` + `encoding/json` (stdlib): authenticated GET against `api/oauth/usage`, lenient decode (response carries rotating experimental fields; `resets_at` can be null) — an HTTP/OAuth library would be absurd for one request
- `os/exec` + system tmux >= 2.1: session lifecycle via 6 one-line commands on a **dedicated `-L kangent` socket with `-f /dev/null`** — Go tmux wrapper libraries rejected (thin exec wrappers, zero gain)
- Existing creack/pty + WS layer: zero transport changes; tmux redraws fully on attach so the existing resize-nudge replay pattern applies
- TanStack Query 5 + shadcn HoverCard/Progress (existing patterns): poll, dedupe across mounts, popup UI

**Explicitly rejected:** browser-direct fetch (CORS + token leak), OAuth token refresh in Kangent, scraping the `/usage` TUI, JSONL-transcript estimation (ccusage-style), Go tmux libraries, the user's default tmux socket.

### Expected Features

**Must have (table stakes, all P1):**
- Compact "Claude 5h" bar with green/yellow/red thresholds (<60 / 60-84 / >=85 — the genre convention) in board + task headers
- Hover popup: one row per server-returned window (label, bar, %, "Resets in Xh Ym" duration countdown), "Updated Xm ago" + manual refresh
- Backend fetcher with the full protection set: 60s TTL, 10s hard floor, in-flight dedup, 429 Retry-After backoff (min 30s), stale retention with 3-failure drop
- Full error catalog: not-logged-in / 401 token-expired / 403 / 5xx / network — never auto-refresh the token; API-key users degrade to a hidden/muted indicator (no fake 0% bars)
- "tmux" in shell dropdown, LookPath-gated; attach-or-create spawn with deterministic names (`kangent-task-<id>-<seq>`, `[A-Za-z0-9_-]` only)
- Close tab = detach, explicit Kill = `kill-session` (distinct, deliberate affordances); exit-vs-detach detection via `has-session` after PTY exit
- Restart reconcile + Resume affordance (mirrors v1.1 `claude --resume` UX exactly)

**Should have (differentiators):**
- Server-driven window rendering (no hardcoded 5h/7d set — Anthropic changes plans; render whatever the endpoint returns)
- Red-zone emphasis on the compact trigger at >=85% — the "stop starting agents" glance signal

**Defer (v1.2.x / v2+):**
- Cleanup-gate awareness of detached tmux sessions (P2 — needed at first task-deletion with tmux tabs)
- Pinnable inline bars (SlayZone parity), quota notifications (blocked on NOTF-01), extra-usage balance, multi-provider

### Architecture Approach

Both features slot into existing seams; nothing restructures. Quota: new DB-free `internal/quota` package (service struct + mutex cache, mirroring `session.Manager`'s shape) behind `GET /api/usage`; the browser never talks to Anthropic. tmux: new leaf package `internal/tmux` (pure exec wrappers, imported by both `internal/session` and `internal/api` — resolves the no-cycle constraint), tmux name minted and persisted by the HTTP handler (new `tmux_sessions` table, migration 00005, storing identity only — **never status**; status is always derived at read time from the manager snapshot + `has-session` probe), carried into the manager via `SpawnOpts.TmuxName`. No new session `Kind` — tmux tabs are `KindBash` with a `tmuxName` field. The frontend gets one new component (`QuotaIndicator`, mounted in both page headers since no shared app header exists) and tmux-aware tab semantics in TaskPage (detach-on-close, ghost tabs, Resume).

**Major components:**
1. `internal/quota` + `internal/api/usage.go` — credentials read, upstream fetch, TTL cache + last-good fallback, normalized snapshot served at `/api/usage`
2. `internal/tmux` — `HasSession`/`KillSession`/`DetachClient`/`NewSessionArgs`, always `-L kangent -f /dev/null` and `=name` exact-match targets via one helper
3. `internal/session` (MOD) — tmux command construction in the KindBash branch only; manager stays DB-free
4. `internal/api/sessions.go` (MOD) — name mint/persist at spawn, detach endpoint, tmux-aware stop (kill-session), list-merge reconciliation with lazy row GC, reattach spawn variant
5. Cleanup paths (worktrees.go, tasks.go MOD) — kill-session before worktree removal/task delete; detached sessions counted by the dirty-tree gate
6. `web/src/components/quota/QuotaIndicator.tsx` + TaskPage tab semantics

### Critical Pitfalls

1. **Detach-vs-kill threaded through the shared Stop path (highest regression risk)** — the tmux server daemonizes (own sid), so the existing /proc process-tree sweep cannot kill it; Stop must run `tmux kill-session`, close must detach only. Make lifecycle strategy an explicit per-session property at spawn time, not `if isTmux` branches at stop time; regression-test plain bash and agent stops.
2. **Refreshing the OAuth token** — refresh-token rotation can invalidate the claude CLI's stored credentials and log the user out mid-work. Kangent is a read-only passenger: re-read the file each poll, on 401 show "run claude to re-authenticate", never write the file.
3. **429 hammering** — omitting `User-Agent: claude-code/<version>` lands in a persistent-429 bucket (claude-code #31021); naive retry compounds it. Send the UA, jittered ~60s polls, exponential backoff honoring Retry-After, single-flight, serve stale through transients.
4. **Ring-buffer replay across attach generations** — replaying the previous attach's alt-screen exit + `[detached]` message before tmux's fresh full redraw garbles the terminal. Skip cross-generation replay for tmux tabs (tmux repaints itself); keep replay only for mid-attach browser reconnects.
5. **Sharing the user's tmux server/config** — a user `destroy-unattached on` silently kills the entire feature; default-socket sessions clutter `tmux ls` and risk mutual `kill-server`. Every invocation goes through one helper injecting `-L kangent -f /dev/null`. (Note: FEATURES.md floated default-socket as a differentiator; STACK, ARCHITECTURE, and PITFALLS all independently mandate the dedicated socket — adopt `-L kangent`. External attach remains possible via `tmux -L kangent attach`.)
6. **Env leakage** — Kangent launched inside the user's tmux propagates `$TMUX`/`TERM=tmux-256color` into all spawned PTYs, silently confusing prompts and TUIs. Scrub `TMUX`/`TMUX_PANE` and pin `TERM=xterm-256color` at the single spawn seam.

## Implications for Roadmap

The two features share **no code** — they can be separate phases in either order or parallel. Quota is the smaller, lower-risk slice; tmux carries the lifecycle-fork regression risk and has a strict internal build order.

### Phase 1: Claude Quota Indicator
**Rationale:** Fully self-contained (new backend route + new frontend component; touches zero session/PTY code), endpoint and credentials already verified end-to-end on this machine — lowest-risk, fastest visible win.
**Delivers:** `internal/quota` fetcher (creds read, UA header, cache/backoff/single-flight, stale retention), `GET /api/usage`, QuotaIndicator (compact bar + HoverCard popup) mounted in BoardPage + TaskPage headers, 60s visible-only poll + manual refresh.
**Addresses:** All quota table stakes; server-driven window rendering; red-zone emphasis (trivial add).
**Avoids:** Pitfalls 1-4 — encode "read-only credentials, never refresh, never write" and the header/backoff policy as plan-level decisions; verification includes the six-state degradation matrix (file absent / key absent / expired / 429 / 5xx / malformed) and a grep audit for `sk-ant-oat` in logs and API payloads.

### Phase 2: tmux Foundations & Spawn Path
**Rationale:** The lifecycle-strategy decision (Pitfall 5 in PITFALLS.md) must come first — everything else in the feature hangs off it. Ends at a verifiable milestone: a tmux tab works end-to-end through the untouched WS/terminal stack.
**Delivers:** Settings seam (`AllowedShells` += "tmux", `AvailableShells()` LookPath filter — zero frontend dropdown changes thanks to SHELL-FUT-01); `internal/tmux` wrappers (one helper, always `-L kangent -f /dev/null`, `=name` targets); migration 00005 (`tmux_sessions`: identity only, never status); `SpawnOpts.TmuxName`/`Label` + manager command construction; env scrubbing (`TMUX`/`TMUX_PANE` removed, `TERM` pinned) for all session types.
**Uses:** v1.1 LookPath posture, `claude_session_id` handler-owns-persistence precedent, DB-stable per-task seq for naming (in-memory `taskCounters` resets on restart — would collide with surviving sessions).
**Avoids:** Socket/config isolation and env-hygiene pitfalls — line one of the spawn implementation.

### Phase 3: tmux Lifecycle — Detach, Kill, Exit Detection
**Rationale:** The behavioral heart and the regression-risk center; depends on Phase 2's spawn path. Detach/kill/exit semantics must land together — shipping any subset produces "Stop doesn't stop" or "close kills".
**Delivers:** `POST /api/sessions/{id}/detach` (close on tmux tabs detaches via the existing closing flow); Stop = `kill-session` then reap; exit-vs-detach classification (`has-session` after PTY exit — rc=0 both ways, verified [E2]); cross-generation ring-buffer replay skipped for tmux tabs; frontend close-branching on `tmuxName`, distinct Kill affordance, tmux tab badge.
**Avoids:** The detach-vs-kill and replay-garbage pitfalls. Explicit regression tests: plain bash Stop still kills the full tree; agent lifecycle untouched; close->reopen with `top` running shows the process alive with a clean single repaint.

### Phase 4: Reconciliation, Resume & Cleanup Integration
**Rationale:** Builds on Phases 2-3; mirrors how v1.0 split attach (Phase 2) from restart-resume (Phase 5). Completes the headline "survives Kangent restart" promise and closes the orphan-session hazard.
**Delivers:** DB-derived detached entries in the sessions list (Phase 5 `resumable` pattern, `has-session` as the probe, lazy row GC when the session died); ghost tabs + Resume (reattach = same `new-session -A` spawn with the persisted name, gated like the one-agent-per-task 409); kill-session in worktree-remove and task-delete paths *before* `wt.Remove`; detached sessions surfaced in the cleanup-gate dialog; orphan sweep (tmux-has-it + DB-doesn't -> kill); README note on `tmux -L kangent kill-server`.
**Avoids:** Stale-session/reconciliation pitfall — verify Resume only appears for live sessions (including the tmux-died-while-Kangent-was-down case) and task deletion leaves zero kangent sessions.

### Phase Ordering Rationale

- Quota first (or parallel): zero coupling to session code, verified mechanism, immediate user value; it's also the milestone's only external-API risk, so landing it early surfaces any endpoint surprises with maximum runway.
- tmux phases follow strict dependency: lifecycle-strategy decision -> spawn path -> close/kill semantics -> reconciliation/cleanup. The architecture research's suggested build order (settings seam -> wrappers/migration -> spawn -> lifecycle -> cleanup -> frontend) maps onto Phases 2-4.
- Cleanup integration deliberately last within tmux: it needs working detach semantics to test against, and the existing dirty-tree gate is the safety net meanwhile.

### Research Flags

Phases likely needing deeper research during planning:
- **None require `/gsd:research-phase`.** All four documents are grounded in line-level codebase reads and same-day empirical verification (live endpoint call, tmux 3.4 command matrix).

Phases with standard patterns (skip research-phase):
- **Phase 1 (Quota):** endpoint, headers, response shape, credentials layout, and cache policy all verified or read from SlayZone source; implementation is transcription.
- **Phases 2-4 (tmux):** every integration point cited at file:line against the actual codebase; tmux behaviors empirically confirmed. One cheap planning-time task: a 5-minute smoke test of `=` exact-match targets and `detach-client` flags on the host tmux (the only MEDIUM-confidence tmux details, from man-page knowledge rather than the empirical run).

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | Everything verified empirically on this machine (live HTTP 200 from the usage endpoint, tmux 3.4 command matrix) or read from current source; zero new dependencies removes version risk |
| Features | HIGH | SlayZone UX read from actual source; endpoint corroborated by 4 independent community tools; only `/usage` TUI exact rendering is MEDIUM (irrelevant — we don't scrape it) |
| Architecture | HIGH | Every integration claim verified at file:line against the Kangent codebase; quota mechanism designed behind a seam that the stack research has since filled in |
| Pitfalls | HIGH | tmux behaviors and credentials layout empirically verified ([E1]-[E6]); endpoint failure modes sourced from official claude-code repo issues |

**Overall confidence:** HIGH

### Gaps to Address

- **Undocumented endpoint stability:** `api/oauth/usage` can change or be restricted without notice (gray-area under the Feb 2026 credential policy). Handle by design: lenient decode, `available:false` degradation, indicator built as best-effort and isolated so breakage costs nothing.
- **Socket decision divergence:** FEATURES.md proposed the user's default tmux socket as a differentiator; the other three documents mandate `-L kangent`. Resolution adopted here: dedicated socket (hostile-`.tmux.conf` and `destroy-unattached` risks outweigh discoverability; external attach still works with `-L kangent`). Roadmapper should treat this as decided.
- **tmux config surface (status bar, mouse mode, history-limit):** STACK/PITFALLS lean toward `status off` + `mouse on` + bounded `history-limit` on the isolated server; FEATURES warns against overriding user expectations — moot under `-f /dev/null` isolation (there is no user config to override). Decide the exact `set -g` set during Phase 2 planning; it's a 3-line decision, not research.
- **macOS:** credentials live in the Keychain, `tmux-256color` terminfo may be missing. Scope to Linux for v1.2 and write it down (PROJECT.md targets local Linux).
- **Quota poll while idle:** first iteration may poll without connected browsers; acceptable with jitter + backoff, fix before milestone close (noted in PITFALLS tech-debt table).

## Sources

### Primary (HIGH confidence)
- Empirical, this machine, 2026-06-11: live `GET https://api.anthropic.com/api/oauth/usage` -> HTTP 200 (full body captured in STACK.md); `~/.claude/.credentials.json` structure (claude 2.1.173); tmux 3.4 full command matrix incl. attach exit codes, `has-session` semantics, name sanitization, nesting-guard conditions, server defaults
- Kangent codebase at `/home/jordi/workspace/github/kangent` — all architecture integration points verified at file:line
- SlayZone source (read 2026-06-11): `electron/usage.ts`, `UsagePopover.tsx`, `useUsage.ts` — complete quota implementation and UX
- [claude-code #31021](https://github.com/anthropics/claude-code/issues/31021) — persistent 429 without claude-code User-Agent; [claude-code #10039](https://github.com/anthropics/claude-code/issues/10039) — credentials-file fragility
- [Claude Code authentication docs](https://code.claude.com/docs/en/authentication); tmux(1) man page; VS Code terminal persistence docs

### Secondary (MEDIUM confidence)
- [Claude-Code-Usage-Monitor #202](https://github.com/Maciek-roboblog/Claude-Code-Usage-Monitor/issues/202) — endpoint schema + UA bucket corroboration
- Community quota tools in aggregate (Usage4Claude, usage-monitor-for-claude, ClaudeBar, claude-code-usage-bar) — endpoint as de-facto standard; color thresholds; API-key handling
- Anthropic help-center + community articles on `/usage` windows and May 2026 limit changes
- tmux changelog (version floors for `-A`, `=` targets); Feb 2026 OAuth credential-use policy article

### Tertiary (LOW confidence)
- None load-bearing.

---
*Research completed: 2026-06-11*
*Ready for roadmap: yes*

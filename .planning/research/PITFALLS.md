# Pitfalls Research

**Domain:** Adding (a) a Claude OAuth quota poller and (b) tmux-backed resumable shell tabs to an existing Go PTY/WS terminal app (Kangent v1.2)
**Researched:** 2026-06-11
**Confidence:** HIGH for tmux behaviors (empirically verified on tmux 3.4 on this machine) and credentials file layout (inspected locally); MEDIUM for the usage endpoint specifics (undocumented API, verified against multiple community sources and claude-code GitHub issues)

## Empirical Findings (verified on this machine, 2026-06-11)

These ground several pitfalls below; cited inline as [E1]–[E6].

- **[E1]** `~/.claude/.credentials.json` exists on Linux (mode 0600). Top-level keys: `claudeAiOauth` (`accessToken`, `refreshToken`, `expiresAt` as **epoch milliseconds int**, `scopes`, `subscriptionType` — here `"team"` — and `rateLimitTier`) **plus** `mcpOAuth.*` entries containing third-party MCP server tokens/secrets. The file holds more secrets than just the Claude token.
- **[E2]** `tmux attach` exits with **rc=0 in both cases** — server-side detach AND inner-shell exit. Exit code cannot distinguish them. `tmux has-session -t <name>` immediately after attach exits is the discriminator: rc=0 → detached (session alive), rc=1 → session gone. tmux prints `[detached (from session X)]` vs `[exited]` plus alt-screen/mouse-mode reset sequences (`?1049l`, `?1000l`…) into the PTY before exiting — these land in Kangent's ring buffer.
- **[E3]** `has-session` against a socket with **no server running** also returns rc=1 with `no server running on /tmp/tmux-1000/<sock>` on stderr. With default `exit-empty on`, the server exits when the last session dies — "no server" is a *normal* state, not an error.
- **[E4]** tmux 3.4 defaults on a `-f /dev/null` server: `window-size latest`, `aggressive-resize off`, `status on`, `exit-empty on`. The status-bar clock redraws periodically → an idle tmux pane still produces continuous PTY output.
- **[E5]** The famous "sessions should be nested with care, unset $TMUX to force" refusal did **not** fire when attaching with `$TMUX` set in the environment: tmux only refuses when the attaching client's *tty is itself a pane of the same server* (`server_client_check_nested`). Kangent's creack/pty PTYs are never panes of the kangent tmux server, so attach works even if Kangent itself was launched inside the user's tmux. The leak is still harmful for other reasons (see Pitfall 8).
- **[E6]** `TERM=tmux-256color` inside panes of a `-f /dev/null` server (not `xterm-256color` and not `screen-256color`).

## Critical Pitfalls

### Pitfall 1: Kangent refreshing the OAuth token itself

**What goes wrong:**
Kangent sees an expired `accessToken`, uses the `refreshToken` to mint a new one, and writes it back (or doesn't). Anthropic's OAuth flow rotates refresh tokens; a refresh performed outside claude can leave claude holding a stale refresh token → the user's claude CLI gets logged out mid-work, or two writers race on `.credentials.json`. On macOS the canonical store is the Keychain, so a file write doesn't even land where claude reads.

**Why it happens:**
"Token expired → refresh it" is the textbook OAuth client behavior; developers reflexively implement it.

**How to avoid:**
Kangent is a *read-only passenger* on claude's credentials. Never call the token endpoint, never write the credentials file. On 401/expired token: mark the indicator "stale — open a claude session to refresh", because **running claude refreshes the token itself** as a side effect. Since Kangent's whole purpose is spawning claude sessions, the token is naturally fresh whenever quota matters. Re-read the credentials file before each poll (cheap, picks up claude's own refreshes); check `expiresAt` (epoch **ms**, not seconds [E1]) before calling and skip the request if already expired.

**Warning signs:**
Any code path that constructs a request to an OAuth token endpoint; any `os.WriteFile` targeting `.credentials.json`; user reports "claude asked me to log in again after using Kangent".

**Phase to address:**
Quota indicator phase — encode "read-only, never refresh, never write" as a plan-level decision before any HTTP code exists.

---

### Pitfall 2: Hammering `/api/oauth/usage` into a persistent 429

**What goes wrong:**
The endpoint rate-limits aggressively. Two specific failure modes reported in claude-code issues: (1) requests **without `User-Agent: claude-code/<version>`** fall into a far stricter bucket and get *persistent* 429s (claude-code issue #31021); (2) naive 60s polling with retry-on-error and no backoff compounds into a 429 loop where the indicator never recovers.

**Why it happens:**
It's an undocumented endpoint; nothing tells you the UA header is load-bearing. A fixed-interval poller with "retry immediately on failure" is the natural first implementation.

**How to avoid:**
- Send `Authorization: Bearer <token>`, `anthropic-beta: oauth-2025-04-20`, and `User-Agent: claude-code/<installed version>` (read the version from `claude --version` once at startup, or pin a known-good string).
- Poll at ~60s **with jitter** (e.g., 60s ± 10s) and only while at least one browser client is connected/visible — Kangent is a long-running server; don't poll an idle machine 24/7.
- On 429/5xx: exponential backoff (respect `Retry-After` if present), serve the last cached result with its real "Updated Xm ago" timestamp. Manual refresh should be rate-limited client-side too (disable button for ~10s after a click) — a user mashing refresh during an outage is the classic 429 amplifier.
- Single-flight: manual refresh and the background tick must coalesce, not stack.

**Warning signs:**
429s in logs; indicator stuck on "error" for minutes; multiple in-flight requests visible when both the timer and a manual refresh fire.

**Phase to address:**
Quota indicator phase — the poller's backoff/caching design is the core of the feature, not a polish item.

---

### Pitfall 3: Assuming one credentials layout across OS and claude versions

**What goes wrong:**
The quota feature works on the dev machine (Linux, file present) and silently breaks elsewhere: on macOS the token lives in the Keychain (`Claude Code-credentials` generic password) and `.credentials.json` may not exist at all; for API-key users (`ANTHROPIC_API_KEY` / `apiKeyHelper`) there is **no subscription quota** and no `claudeAiOauth` block; the file schema is unversioned and has already changed across claude releases (and claude-code issue #10039 documents claude-on-Mac *deleting* the file a Linux setup relied on, in shared-HOME scenarios).

**Why it happens:**
The dev machine is the only test environment; the file is right there and parses fine.

**How to avoid:**
Treat "no quota available" as a first-class state, not an error:
- Parse defensively: read only `claudeAiOauth.{accessToken,expiresAt,subscriptionType}`; if the key is missing, malformed, or the file is absent → indicator hides (or shows a muted "n/a" tooltip explaining why). Never crash, never log the parse failure with file contents.
- On macOS, attempt the file first (claude has a documented file fallback), and if absent either shell out to `security find-generic-password -s "Claude Code-credentials" -w` or simply show "quota unavailable on this setup" for v1.2 — decide explicitly rather than discovering it post-ship. PROJECT.md says local Linux is the actual deployment; scoping macOS out is legitimate *if written down*.
- API-key-only users: detect absence of `claudeAiOauth` and degrade to hidden indicator. Don't show 0% bars — that reads as "you have full quota".

**Warning signs:**
Indicator shows an error state on a colleague's machine; any code doing `json.Unmarshal` into a struct mirroring the whole credentials file (it will break when claude adds fields — use targeted extraction).

**Phase to address:**
Quota indicator phase — the "degrade gracefully" matrix (file missing / key missing / token expired / endpoint error) should be in the plan's acceptance criteria.

---

### Pitfall 4: Leaking the OAuth token (and MCP secrets) into logs, errors, or the API

**What goes wrong:**
The bearer token ends up in: a wrapped error string (`fmt.Errorf("usage request failed: %v", req)`), an slog of the HTTP request, the JSON the backend returns to the browser, or a debug dump of the parsed credentials. Worse than the Claude token: the same file contains `mcpOAuth.*` client secrets and access tokens for third-party services [E1] — a careless "log what we read" exposes those credentials too.

**Why it happens:**
Error wrapping and request logging are habitual; nobody thinks of the credentials *file* as multi-tenant.

**How to avoid:**
- Extract only the three needed fields and discard the rest of the parsed document immediately; never hold or log the raw file bytes.
- The token must never cross the Kangent API: the **server** polls and the browser receives only the digested quota struct (utilization, resets_at, updated_at, status).
- Construct errors from status codes and trimmed response bodies only; never include the request object in errors.
- Note the ToS context: since Feb 2026 Anthropic's credential policy restricts consumer OAuth tokens to Claude Code/claude.ai. A local read-only usage display with the claude-code UA is what the whole ecosystem of usage monitors does, but it is formally gray-area and the endpoint can change or be restricted without notice. Build the feature as strictly best-effort and isolated, so removal/breakage costs nothing.

**Warning signs:**
`accessToken` greppable in any log file; the `/api/quota` response containing anything other than digested numbers; tests asserting on raw credential structures.

**Phase to address:**
Quota indicator phase — add "grep logs and API responses for `sk-ant-oat`" to the verification checklist.

---

### Pitfall 5: Threading detach-vs-kill through the shared session Stop path (regression risk to existing tabs)

**What goes wrong:**
This is the highest regression-risk item of the milestone. The existing SessionManager has one lifecycle: spawn → PTY read loop → Stop = full-process-tree kill → reap → mark dead. tmux tabs need a *fork* in that lifecycle: closing the tab must end the **attach client** (detach) while the tmux session lives on; an explicit Stop must `tmux kill-session`. Three concrete failure modes:
1. **Stop doesn't stop:** the existing process-tree kill only reaches Kangent's child — the `tmux attach` client. The shell and its processes live in the **tmux server's** tree, not Kangent's. User clicks Stop, sees the tab die, but a build/server keeps running invisibly inside tmux.
2. **Close kills:** the close-tab path reuses the kill path unchanged, so tmux tabs die on close — the entire feature silently doesn't work, and may even pass shallow testing because the tab "reopens" (as a fresh session).
3. **Collateral regression:** refactoring Stop into detach/kill variants accidentally changes semantics for agent sessions or plain bash tabs (e.g., the worktree-cleanup "running session" gate, exited-state UI, or restart reconciliation start treating sessions inconsistently).

**Why it happens:**
The natural implementation is `if isTmux { ... }` branches sprinkled through the existing Stop/close/reap code, and the kill-vs-detach distinction is easy to conflate because *both* end with the PTY closing and the read loop exiting.

**How to avoid:**
- Make lifecycle strategy an explicit property of the session (e.g., `Terminator` interface or a `detachOnClose bool` + per-type `Kill()` implementation), decided at spawn time, instead of type-checks at stop time.
- Define the three verbs precisely in the plan: **Close tab** = drop WS clients (existing behavior — server session persists for all types); **Detach** (tmux only) = end the attach PTY, session persists in tmux; **Stop/Kill** = plain sessions: process-tree kill (unchanged); tmux sessions: `tmux -L kangent kill-session -t <name>`, *then* reap the attach client.
- After the attach process exits for any reason, run `has-session` to classify detach vs exit [E2] and set session state accordingly (detached → resumable; exited → dead, same UX as today's exited bash tab).
- Regression-test the existing flows explicitly: plain bash tab Stop still kills the whole tree; agent session Stop unchanged; worktree cleanup gate counts a *detached* tmux session as "running".

**Warning signs:**
`ps` shows shells alive after Stop on a tmux tab; closing a tmux tab and reopening yields a fresh prompt instead of your running program; any change to the signature/semantics of the shared kill function without an accompanying test for plain sessions.

**Phase to address:**
tmux phase, first plan — this is the architectural decision; everything else in the feature hangs off it.

---

### Pitfall 6: Ring-buffer replay + tmux redraw = garbage on reattach

**What goes wrong:**
The existing reattach path replays the ring buffer, then nudges a resize. For tmux tabs the ring buffer contains: the *previous* attach's full alternate-screen session, mouse-mode enable/disable toggles, the alt-screen **exit** sequence (`?1049l`) and the literal `[detached (from session X)]` message that tmux printed when the last attach ended [E2]. Replaying that into xterm.js, then starting a **new** `tmux attach` (a brand-new process with its own full redraw), produces flicker, a stray `[detached]` line, briefly-toggled mouse modes, and a confusing double-paint. Worse: if the old attach PTY is still tracked when a new attach spawns (restart-resume bugs), two PTY read loops feed one ring buffer and the terminal garbles permanently.

**Why it happens:**
Replay-then-stream is the correct pattern for plain PTYs (the process can't redraw on demand) and it's tempting to reuse it untouched. tmux inverts the assumption: the redraw capability lives server-side in tmux, so replay is redundant.

**How to avoid:**
- For tmux sessions, **skip ring-buffer replay across attach generations** (clear or reset the buffer when the attach client ends). Reattach = spawn a fresh `tmux -L kangent attach -t <name>` in a fresh PTY sized to the current xterm dims; tmux repaints the whole screen itself. The ring buffer still serves mid-attach browser-tab reconnects (same attach process still alive) — keep replay for that case only.
- Ensure exactly one live attach process per Kangent tmux session: before spawning a new attach, confirm the old one is reaped (or `tmux detach-client` it first).
- Scrollback expectations change: tmux runs in the alternate screen, so xterm.js's own scrollback stays empty — the wheel does nothing unless tmux mouse mode is on (wheel then enters copy-mode). Decide explicitly: `set -g mouse on` in the kangent server config gives wheel-scroll via copy-mode (closest to current bash-tab feel); document that select-to-copy then needs Shift (standard xterm.js + tmux-mouse caveat). Do not attempt the `smcup@/rmcup@` terminal-override hack to force xterm scrollback — it's notorious for corrupted displays on window/pane switches.

**Warning signs:**
`[detached]`/`[exited]` text visible after reopening a tab; screen drawn twice on reattach; mouse wheel behavior differing between first attach and reattach; garbled output when two browser tabs race to resume after a server restart.

**Phase to address:**
tmux phase — the attach/replay decision belongs in the same plan as the lifecycle fork (Pitfall 5); restart-resume builds on it.

---

### Pitfall 7: Sharing the user's tmux server / config instead of an isolated `-L kangent` socket

**What goes wrong:**
Spawning sessions on the default tmux server makes Kangent hostage to `~/.tmux.conf`: a user with `set -g destroy-unattached on` has every Kangent session **destroyed the instant the tab closes** (feature silently dead); custom prefix keys, status bars, plugins (tpm, resurrect), `default-command`, and hooks all alter behavior; Kangent sessions clutter the user's `tmux ls`; and `kill-server` from either side nukes the other's sessions. Session-name collisions with user sessions are possible too.

**Why it happens:**
`tmux new-session` "just works" against the default socket; isolation looks like extra ceremony.

**How to avoid:**
- Always run with a dedicated socket: `tmux -L kangent` on **every** invocation (new-session, attach, has-session, list-sessions, kill-session). One missed `-L` and that command hits the user's server.
- Start the server with `-f /dev/null` so user config never loads. Note `-f` only matters on the command that boots the server — with `exit-empty on` (default [E3,E4]) the server dies when the last session ends and the *next* command boots a fresh one, so `-f /dev/null` must be passed uniformly via a single helper, not just on a one-time "init".
- Apply Kangent's own minimal config after server start via `set -g` commands (e.g., `mouse on`, possibly `status off`, a bounded `history-limit`), not via a config file the user might edit expectations into.
- Deterministic, collision-proof session names derived from stable IDs: `kangent-<taskID>-<tabID>` (avoid colons/periods — tmux rewrites them). Use `has-session` before `new-session`, or `new-session -A -d` (attach-or-create, detached) to make spawn idempotent. Note `new-session` without `-d` in a non-tty context errors with `open terminal failed: not a terminal` — always create detached, then attach separately inside the PTY.
- Decide the status bar question consciously [E4]: `status on` gives a visible "this is tmux" affordance but adds a clock that redraws continuously (constant PTY output into the ring buffer even when idle — harmless at this scale but surprising in debugging) and looks different from plain bash tabs. `set -g status off` makes tmux tabs visually identical to bash tabs — arguably better, with the tab UI itself carrying the "resumable" badge.

**Warning signs:**
Kangent sessions visible in plain `tmux ls`; behavior differing between machines with/without a `~/.tmux.conf`; sessions vanishing on detach (destroy-unattached); prefix key weirdness in Kangent terminals.

**Phase to address:**
tmux phase — socket/config isolation is line one of the spawn implementation.

---

### Pitfall 8: Environment leakage into spawned PTYs (`$TMUX`, `TERM`)

**What goes wrong:**
If Kangent itself is launched from inside a tmux session (very likely — dev servers commonly run in tmux), every PTY Kangent spawns inherits `TMUX`, `TMUX_PANE`, and `TERM=tmux-256color/screen-256color`. Consequences: shell prompts and tools in **plain bash tabs and agent sessions** believe they're inside tmux (prompt frameworks show tmux segments; scripts gate behavior on `$TMUX`); a user typing `tmux ls`/`tmux kill-server` in a Kangent bash tab unexpectedly operates in their *personal* server's context; and TERM mismatches cause subtle TUI rendering issues in claude. Empirically the feared hard failure — tmux refusing nested attach — does **not** occur here ([E5]: refusal requires the client tty to be a pane of the same server), so this bug is *silent*, which makes it worse.

**Why it happens:**
`exec.Cmd` inherits the parent environment by default; nothing fails loudly, so it ships.

**How to avoid:**
In the PTY env-setup for all session types, explicitly remove `TMUX` and `TMUX_PANE`, and set `TERM=xterm-256color` for non-tmux PTYs (this may already happen for xterm.js compatibility — verify rather than assume). For the tmux attach PTY itself, also pass `TERM=xterm-256color` (that's the *outer* terminal type tmux renders for; inside panes tmux sets `TERM=tmux-256color` on its own [E6]). On hosts with old ncurses (notably macOS) the `tmux-256color` terminfo entry may be missing, breaking TUIs inside panes — if targeting macOS, set `default-terminal screen-256color` on the kangent server; on Linux it's a non-issue.

**Warning signs:**
`echo $TMUX` non-empty in a plain Kangent bash tab; rendering differences depending on how Kangent was launched; "missing or unsuitable terminal: tmux-256color" from TUIs inside tmux tabs on macOS.

**Phase to address:**
tmux phase — one env-scrubbing function applied at the existing single spawn seam, plus a check that current spawns already pin TERM.

---

### Pitfall 9: Stale tmux sessions accumulating; reconciliation and cleanup gaps

**What goes wrong:**
tmux sessions now outlive everything: browser tabs, the Kangent server, even the task. Without explicit lifecycle hooks: deleting a task leaves its tmux session running forever (holding a cwd inside a worktree that cleanup then removes — the shell's cwd dangles); restart reconciliation, which today marks all sessions dead, marks live tmux sessions dead too and never offers Resume (or worse, offers Resume for sessions whose tmux side actually died while Kangent was down); uninstalling Kangent leaves an invisible `tmux -L kangent` server with orphan shells consuming memory indefinitely.

**Why it happens:**
v1.0's reconciliation logic was built on the invariant "server restart ⇒ all child processes are gone", which tmux deliberately breaks. Task-deletion cleanup was built when sessions could not outlive their PTY.

**How to avoid:**
- **Startup reconciliation:** `tmux -L kangent list-sessions -F '#{session_name}'` (tolerating "no server running" as empty [E3]), diff against DB session rows: DB-says-tmux + tmux-has-it → resumable (Resume affordance, mirroring the claude `--resume` UX); DB-has-it + tmux-doesn't → dead (existing path); tmux-has-it + DB-doesn't → orphan → `kill-session` (the `kangent-` name prefix on the dedicated socket makes this safe).
- **Task deletion / worktree cleanup:** kill the task's tmux sessions *before* removing the worktree; extend the existing running-session gate so a **detached** tmux session counts as running (it's invisible in the UI — the gate is the only thing standing between the user and silently orphaning their running processes).
- **Uninstall/cleanup story:** at minimum document `tmux -L kangent kill-server`; ideally a settings-page "kill all detached shells" action covers both stale accumulation and pre-uninstall cleanup.

**Warning signs:**
`tmux -L kangent ls` showing sessions for deleted tasks; Resume offered but attach lands on "no sessions"; worktree removal succeeding while a detached shell still has its cwd there.

**Phase to address:**
Split: kill-on-delete + gate extension in the tmux phase; restart reconciliation + Resume in the resume/persistence plan of that phase (mirrors how v1.0 split Phase 2 attach from Phase 5 restart-resume).

## Technical Debt Patterns

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|----------------|-----------------|
| `if isTmux {}` branches in shared Stop/close/reap code | Fast to write | Every future shell type multiplies branches; kill/detach semantics drift apart silently | Never — use a per-session lifecycle strategy (Pitfall 5) |
| Polling quota even with no browser connected | Simpler poller loop | 24/7 background hits on an undocumented rate-limited endpoint from an always-on server | Acceptable first iteration only with ≥60s jittered interval and backoff; fix before milestone close |
| Skipping macOS Keychain support (file-only credentials) | Ships the Linux case now | macOS users see no indicator | Acceptable if explicitly documented as Linux-first |
| Reusing ring-buffer replay unchanged for tmux | No replay-path changes | `[detached]` artifacts, double-paint, garbled reattach | Never — skip cross-generation replay for tmux (Pitfall 6) |
| Hardcoding the `User-Agent: claude-code/x.y` string | No version probing | Anthropic may bucket stale UA strings into stricter limits later | Acceptable v1.2; prefer probing `claude --version` at startup |
| No "kill all detached shells" affordance | Less UI | Orphan accumulation only fixable via CLI | Acceptable for v1.2 if README documents `tmux -L kangent kill-server` |

## Integration Gotchas

| Integration | Common Mistake | Correct Approach |
|-------------|----------------|------------------|
| `/api/oauth/usage` | Omitting `User-Agent: claude-code/<ver>` → persistent 429 bucket | Send Bearer + `anthropic-beta: oauth-2025-04-20` + claude-code UA; treat the endpoint as undocumented/best-effort |
| `/api/oauth/usage` | Assuming all quota fields exist | `seven_day_opus`/`seven_day_sonnet`/`extra_usage` are nullable; render only present windows |
| `.credentials.json` | Unmarshal whole file into a rigid struct | Extract only `claudeAiOauth.{accessToken,expiresAt}`; tolerate unknown/missing fields; file also contains unrelated `mcpOAuth` secrets [E1] |
| `.credentials.json` | Caching the token for the process lifetime | Re-read before each poll — claude rotates it underneath you |
| tmux CLI | Forgetting `-L kangent -f /dev/null` on one of the invocation sites | One helper that always injects both; never call tmux directly |
| tmux attach | Using attach exit code to detect shell exit | rc=0 either way [E2]; classify with `has-session` after exit |
| tmux has-session | Treating "no server running" stderr as a failure | rc=1 + that message simply means zero sessions [E3] |
| tmux new-session | Spawning attached in a non-tty context | `new-session -d` (or `-A -d`), then a separate `attach` inside the PTY |
| tmux resize | Worrying about smallest-client clamping | `window-size latest` is the 3.x default [E4]; only matters if the user co-attaches externally, where last-resize-wins flapping is expected and acceptable |
| Existing settings seam | Validating tmux only at settings-save | Also `LookPath` at spawn time (tmux can disappear after the setting was saved), mirroring v1.1 shell validation |

## Performance Traps

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|----------------|
| Quota poll retry-without-backoff | 429 loop, indicator permanently "error" | Exponential backoff + serve-stale + single-flight | First endpoint hiccup |
| tmux status-bar clock churning the ring buffer | Idle tmux tabs continuously writing to the ring buffer; "activity" never quiesces | `status off` on the kangent server, or accept the churn knowingly [E4] | Cosmetic/debugging nuisance only at this scale |
| Per-component quota fetching in the frontend | N components × interval requests | One server-side poller; browser reads cached `/api/quota` (TanStack Query, staleTime ≈ poll interval) | A few open browser tabs |
| Unbounded tmux `history-limit` on long-lived detached sessions | tmux server memory grows for weeks | Set a sane `history-limit` (e.g., 10000) in kangent server options | Weeks of detached uptime |

## Security Mistakes

| Mistake | Risk | Prevention |
|---------|------|------------|
| Token in logs/error strings | `sk-ant-oat01-…` greppable on disk | Redact; never log raw credentials file or request objects; grep-check in verification |
| Token forwarded to the browser | Any localhost page or extension reading it via the Kangent API | Server-side poll only; API returns digested quota numbers exclusively |
| Logging the parsed credentials file | Leaks third-party MCP tokens/secrets too [E1] | Targeted field extraction; discard the rest immediately |
| New quota route skipping existing protections | Origin/loopback discipline bypassed for the new endpoint | `/api/quota` goes through the same Origin/Host validation as existing routes |
| Writing/chmodding the credentials file | Corrupting claude's auth state; loosening perms | Never write; open read-only |

## UX Pitfalls

| Pitfall | User Impact | Better Approach |
|---------|-------------|-----------------|
| Showing 0%/empty bars for API-key users | Reads as "full quota available" — user starts agents expecting limits that don't exist | Hide indicator (or muted "n/a — API key auth") when `claudeAiOauth` absent |
| Stale quota shown as current | User starts an agent believing quota is free | "Updated Xm ago" from last *success*; visually distinguish stale (>5m) data |
| Error state on every transient 429 | Indicator cries wolf, gets ignored | Serve cached values through transient errors; only surface error after sustained failure |
| tmux tab looks identical to bash tab | User doesn't know close≠kill; surprised either way | Badge tmux tabs (e.g., "persistent"); distinct Detach-close vs Stop affordances |
| Mouse wheel dead in tmux tabs | "Scrollback is broken" reports | `mouse on` in kangent tmux config (wheel → copy-mode); note Shift-select for copy |
| Resume offered for a dead tmux session | Click → "no sessions" error flash | Reconcile with `has-session` before rendering Resume, not at click time |
| Stop on tmux tab leaves processes running (Pitfall 5) | User believes work stopped; build/server still running invisibly | Stop = `kill-session`; verify via `has-session` post-kill |

## "Looks Done But Isn't" Checklist

- [ ] **Quota indicator:** works with file present — verify behavior with file *absent*, `claudeAiOauth` key absent, token expired, endpoint 429, endpoint 5xx, and malformed JSON (six distinct states)
- [ ] **Quota poller:** 60s tick works — verify manual refresh + tick coalesce (single-flight) and polling pauses with no connected browser
- [ ] **Quota security:** numbers render — grep logs and `/api/quota` payloads for `sk-ant-oat`
- [ ] **tmux close/reopen:** reattach works — verify a *running* process (e.g., `top`) is still live and cleanly repainted after close→reopen, not just a fresh prompt
- [ ] **tmux Stop:** tab closes — verify `tmux -L kangent has-session` fails afterward and the inner process tree is gone (`ps`)
- [ ] **tmux + restart:** Resume appears — also verify the case where the tmux session *died* while Kangent was down (must show dead, not Resume)
- [ ] **Inner exit:** typing `exit` in a tmux tab — verify the tab shows exited state (not "detached/resumable") via the has-session classification [E2]
- [ ] **Plain tabs regression:** tmux feature merged — verify plain bash tab Stop still kills the full process tree and agent session lifecycle is untouched
- [ ] **Worktree cleanup gate:** counts attached sessions — verify a *detached* tmux session also blocks/warns on cleanup, and task deletion kills its tmux sessions
- [ ] **Env hygiene:** works from a plain shell — launch Kangent from *inside* the user's tmux and verify `$TMUX` is empty in spawned tabs and claude renders correctly
- [ ] **tmux missing:** dropdown hides/disables tmux when not installed — verify the spawn-time failure path if tmux is removed after the setting was saved

## Recovery Strategies

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| Persistent 429 (bad UA / hammering) | LOW | Fix headers, add backoff; bucket resets after cooldown |
| Token invalidated by accidental refresh attempt | MEDIUM | User runs `claude` → `/login`; delete the refresh code path entirely |
| Stop-doesn't-kill shipped (Pitfall 5) | MEDIUM | Hotfix Stop to `kill-session`; meanwhile `tmux -L kangent ls` + `kill-session` clears strays |
| Orphaned kangent tmux server post-uninstall | LOW | `tmux -L kangent kill-server`; document in README |
| Garbled reattach (replay + redraw) | LOW | Disable cross-generation replay for tmux sessions; users hard-refresh meanwhile |
| Credentials schema change in a claude release | LOW | Defensive parser already degrades to "n/a"; ship updated field mapping |
| Reconciliation marks live tmux sessions dead | MEDIUM | Sessions are still alive in tmux — add the list-sessions diff; deterministic `kangent-<task>-<tab>` names make re-linking possible after the fact |

## Pitfall-to-Phase Mapping

| Pitfall | Prevention Phase | Verification |
|---------|------------------|--------------|
| 1. Self-refresh of OAuth token | Quota phase | No token-endpoint calls or credential writes anywhere in the diff |
| 2. 429 hammering | Quota phase | Headers include claude-code UA; kill network mid-poll → indicator serves stale gracefully, recovers with backoff |
| 3. Credentials layout assumptions | Quota phase | Six-state matrix passes; `mv .credentials.json` away → indicator hides cleanly |
| 4. Token leakage | Quota phase | grep audit of logs + API payloads |
| 5. Detach-vs-kill in shared Stop path | tmux phase (first plan) | Lifecycle-strategy seam exists; regression tests for plain bash + agent Stop pass |
| 6. Replay/redraw garbage | tmux phase | Close→reopen with `top` running: clean single repaint, no `[detached]` artifact |
| 7. Socket/config isolation | tmux phase | All tmux calls go through one `-L kangent -f /dev/null` helper; sessions invisible to plain `tmux ls`; survives a hostile `~/.tmux.conf` (`destroy-unattached on`) |
| 8. Env leakage | tmux phase | Launch Kangent inside tmux; `$TMUX` empty in all spawned PTYs |
| 9. Stale sessions / reconciliation | tmux phase (resume plan) | Restart with live + dead tmux sessions → Resume only for live; task deletion leaves zero kangent sessions |

## Sources

- Local empirical verification (tmux 3.4, Linux, 2026-06-11): attach exit codes, has-session semantics, nesting-refusal conditions, defaults (`window-size latest`, `exit-empty on`, `status on`), in-pane `TERM`, credentials file structure — HIGH
- [claude-code issue #31021 — /api/oauth/usage persistent 429 without claude-code User-Agent](https://github.com/anthropics/claude-code/issues/31021) — HIGH (official repo issue)
- [Claude-Code-Usage-Monitor issue #202 — OAuth usage API response schema (five_hour/seven_day/model windows, nullable fields)](https://github.com/Maciek-roboblog/Claude-Code-Usage-Monitor/issues/202) — MEDIUM (community, corroborated by multiple monitors)
- [Claude Code docs — Authentication (credential storage, Keychain on macOS, refresh behavior)](https://code.claude.com/docs/en/authentication) — HIGH
- [claude-code issue #10039 — claude on Mac deletes .credentials.json used by Linux](https://github.com/anthropics/claude-code/issues/10039) — HIGH
- [Medium: Claude Code OAuth vs API key auth in 2026 (Feb 2026 credential-use policy restricting OAuth tokens to Claude Code/claude.ai)](https://lalatenduswain.medium.com/claude-code-on-claude-max-plan-understanding-oauth-token-vs-api-key-authentication-in-2026-96a6213d2cde) — MEDIUM
- [oldeucryptoboi — macOS Keychain `Claude Code-credentials` extraction and file fallback](https://oldeucryptoboi.com/blog/claude-code-ssh-keychain-fix/) — MEDIUM
- [tmux(1) manual — -L sockets, has-session, window-size, exit-empty, destroy-unattached](https://man7.org/linux/man-pages/man1/tmux.1.html) — HIGH
- [xterm.js issue #802](https://github.com/xtermjs/xterm.js/issues/802) / [#3184](https://github.com/xtermjs/xterm.js/issues/3184) — alternate-screen scrollback behavior — MEDIUM
- [tmux issue #1302 — alternateScroll / mouse wheel in tmux under xterm-like emulators](https://github.com/tmux/tmux/issues/1302) — MEDIUM
- tmux source behavior (`server_client_check_nested`: nesting refusal requires client tty to be a pane of the same server) — confirmed empirically [E5] — HIGH

---
*Pitfalls research for: Kangent v1.2 — Claude quota indicator + tmux-backed resumable shell tabs*
*Researched: 2026-06-11*

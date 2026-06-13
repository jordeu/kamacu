# Requirements: Kangent

**Defined:** 2026-06-12
**Milestone:** v1.2 Quota & Resumable Shells
**Core Value:** One place to see and drive all agent work: every task gets its own isolated worktree and a persistent Claude Code session you can open, leave, and reattach to from the browser.

## v1.2 Requirements

Requirements for this milestone. Each maps to roadmap phases.

### Claude Quota Indicator

- [x] **QUOTA-01**: User sees a compact "Claude 5h" indicator with a threshold-colored usage bar (green <60 / yellow 60–84 / red ≥85) in the top-right of the main area, on both board and task view
- [x] **QUOTA-02**: Hovering the indicator opens a popup listing every quota window the API returns (5h, 7d, per-model — server-driven, never hardcoded) with a colored bar, rounded percentage, and "Resets in Xh Ym" countdown per row
- [x] **QUOTA-03**: Popup footer shows "Updated Xm ago" with a manual refresh button that forces a fresh fetch (bypasses cache)
- [x] **QUOTA-04**: Quota data auto-refreshes about every 60s while the browser tab is visible; no background polling when hidden
- [x] **QUOTA-05**: Server fetches quota from Anthropic's OAuth usage endpoint using the local claude credentials (`~/.claude/.credentials.json`) with the load-bearing `claude-code/<version>` User-Agent, a 60s cache TTL, 10s hard floor, in-flight dedup, and 429 Retry-After backoff (min 30s); Kangent never writes or refreshes the token
- [x] **QUOTA-06**: Errors degrade gracefully — not-logged-in / token-expired (401) / 403 / 5xx / network states show clear messages in the popup with a warning on the trigger; stale cached data stays visible marked "error · Xm old" and is dropped after 3 consecutive failures; API-key-only users (no OAuth credentials) see a neutral/hidden indicator, never fabricated 0% bars
- [x] **QUOTA-07**: When any quota window is ≥85%, the compact trigger shows red-zone emphasis (the "stop starting agents" glance signal)
- [x] **QUOTA-08**: The server quota cache is keyed by token, so switching Claude accounts never shows the previous account's data

### Resumable tmux Shells

- [x] **TMUX-01**: User can select "tmux" in the global shell setting dropdown; the option is offered only when `tmux` resolves on PATH
- [x] **TMUX-02**: New bash tabs with tmux selected spawn attach-or-create tmux sessions (deterministic `kangent-<task>-<n>` names, `[A-Za-z0-9_-]` only) on the dedicated `-L kangent` socket, with the task worktree as cwd
- [x] **TMUX-03**: tmux sessions detach implicitly — leaving the task view (or a server shutdown) leaves the session running; reopening the task reattaches to the same session *(amended 2026-06-12 during Phase 8 discussion: detach moved from tab-close to task-view-close)*
- [x] **TMUX-04**: Closing a tmux-backed tab (×) kills the tmux session — kill-on-close parity with plain bash tabs; tmux tabs are visually and behaviorally indistinguishable from bash tabs *(amended 2026-06-12: × IS the kill action; no separate affordance)*
- [ ] **TMUX-05**: After a Kangent server restart, reopening a task whose tmux session still exists auto-reattaches to it — invisibly, no Resume button (reuses the v1.1 agent reconcile mechanism — DB-derived ghost entry gated by `tmux has-session` → `new-session -A` reattach — but diverges from the agent's explicit Resume UX because tmux reattach is cheap and lossless) *(amended 2026-06-13 during Phase 9 discussion, D-88: auto + invisible, not a Resume button)*
- [x] **TMUX-06**: When the shell exits inside tmux, the tab shows the existing exited state, not "resumable" — exit vs detach discriminated via `tmux has-session` after the attach PTY exits
- [x] **TMUX-07**: Plain-bash tabs and agent sessions keep their kill-on-stop behavior unchanged — the shared stop path is regression-guarded by tests
- [x] **TMUX-08**: Task/worktree cleanup gates count live detached tmux sessions as running work, and confirmed cleanup kills the task's tmux sessions (no orphaned shells in deleted directories)
- [x] **REAP-01**: Sessions of tasks in Done — bash, tmux, AND agent — are automatically killed after a configurable TTL (global setting, default 24h, clocked from entering Done; leaving Done cancels; 0/never disables). Worktrees are never auto-removed — worktree cleanup stays manual via the existing gated dialog *(added 2026-06-12 during Phase 8 discussion)*

## Future Requirements (v2+)

### Claude Quota Indicator

- **QUOTA-FUT-01**: Extra-usage credits block rendered in the popup (endpoint exposes it; verify shape empirically first)
- **QUOTA-FUT-02**: Pinnable inline bars — choose which windows show on the compact trigger (SlayZone parity)
- **QUOTA-FUT-03**: Quota threshold notifications — blocked on NOTF-01 (no notification system yet)

### Resumable tmux Shells

- **TMUX-FUT-01**: Option to use the user's default tmux server instead of the dedicated socket (external `tmux attach` convenience)
- **TMUX-FUT-02**: Multi-provider usage indicators (Codex etc.) — out of scope per PROJECT.md (Claude-only)

## Out of Scope

Explicitly excluded. Documented to prevent scope creep.

| Feature | Reason |
|---------|--------|
| Scraping `claude /usage` TUI output via PTY | Brittle ANSI parsing, version-dependent; OAuth usage endpoint is the community-standard structured source |
| Computing quota from local JSONL transcripts | Estimates drift from server truth; misses usage from other devices; the tool category converged on the endpoint |
| Auto-refreshing the expired OAuth token | Token rotation races with Claude Code's own refresh and risks invalidating the CLI session; 401 → "re-authenticate with `claude`" |
| Spend/cost analytics for API-key users | Different data model (dollars, no windows); `/cost` exists in the CLI |
| Wrapping agent (claude) sessions in tmux | claude already has `--resume`; nesting a TUI in tmux adds redraw artifacts and double alternate-screen problems |
| Loading the user's own tmux config into Kangent shells | The dedicated `-L kangent` socket runs Kangent-controlled minimal config — `status off` + `mouse on` for the invisible-tmux experience (D-79/D-80, reversed the original "run vanilla" stance on 2026-06-12) |
| Serializing xterm.js scrollback across restarts | tmux owns history; replaying a stale buffer over tmux's redraw garbles the screen |
| `remain-on-exit` / auto-respawn of dead shells | A dead pane that eats keystrokes confuses more than a clean exited state |
| macOS keychain credential support | Linux-first per PROJECT.md; written decision, not silent omission |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| QUOTA-01 | Phase 7 | Complete |
| QUOTA-02 | Phase 7 | Complete |
| QUOTA-03 | Phase 7 | Complete |
| QUOTA-04 | Phase 7 | Complete |
| QUOTA-05 | Phase 7 | Complete |
| QUOTA-06 | Phase 7 | Complete |
| QUOTA-07 | Phase 7 | Complete |
| QUOTA-08 | Phase 7 | Complete |
| TMUX-01 | Phase 8 | Complete |
| TMUX-02 | Phase 8 | Complete |
| TMUX-03 | Phase 8 | Complete |
| TMUX-04 | Phase 8 | Complete |
| TMUX-06 | Phase 8 | Complete |
| TMUX-07 | Phase 8 | Complete |
| TMUX-05 | Phase 9 | Pending |
| TMUX-08 | Phase 9 | Complete |
| REAP-01 | Phase 9 | Complete |

**Coverage:**
- v1.2 requirements: 17 total
- Mapped to phases: 17
- Unmapped: 0 ✓

---
*Requirements defined: 2026-06-12*
*Last updated: 2026-06-12 after Phase 8 discussion (TMUX-03/04 amended, REAP-01 added, tmux-config out-of-scope row reversed)*

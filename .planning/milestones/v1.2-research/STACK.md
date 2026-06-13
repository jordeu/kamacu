# Stack Research — v1.2: Claude Quota Indicator + tmux-Backed Resumable Shells

**Domain:** Claude Code quota/usage API integration + tmux session persistence for an existing Go+React PTY app
**Researched:** 2026-06-11
**Confidence:** HIGH (everything below verified empirically on this machine or read from current source, not training data)

> Supersedes the v1.0 stack research at this path (2026-06-10), which is fully mirrored into CLAUDE.md. This file covers only the v1.2 additions.

## Headline

**Zero new dependencies.** Both features are built entirely on the existing stack:

- **Quota indicator:** `net/http` GET against an undocumented-but-stable Anthropic OAuth endpoint, bearer token read from `~/.claude/.credentials.json` with `os.ReadFile` + `encoding/json`. Frontend is TanStack Query (`refetchInterval`) + shadcn components already in the copy-in workflow.
- **tmux shells:** pure `os/exec` argument construction — the *command spawned inside the existing creack/pty session* becomes `tmux ... new-session -A ...` instead of `bash`. tmux is a host prerequisite detected via `exec.LookPath` (same pattern as v1.1 shell validation), never a Go module.

## Part A — Claude Quota/Usage Data (verified end-to-end)

### How SlayZone does it (read from current source, 2026-06-11)

SlayZone's entire quota feature lives in `packages/domains/terminal/src/electron/usage.ts`. Verbatim findings:

1. **Token source (Linux):** `~/.claude/.credentials.json` → JSON path `claudeAiOauth.accessToken`. (macOS: keychain generic password, service `Claude Code-credentials`, same JSON inside.)
2. **Endpoint:** `GET https://api.anthropic.com/api/oauth/usage`
3. **Headers:**
   - `Authorization: Bearer <accessToken>`
   - `anthropic-beta: oauth-2025-04-20`
   - `Content-Type: application/json`
   - `User-Agent: claude-code/<installed claude version>` (SlayZone runs `claude --version` once and caches it; falls back to a hardcoded version string)
4. **Caching/backoff:** 60s auto-poll TTL, **10s hard floor** even on manual refresh (anti spam-click), in-flight dedup, on 429 honor `Retry-After` with a **30s minimum backoff**, keep last good data marked stale on failure, drop stale windows after 3 consecutive failures.

### Verified on this machine (2026-06-11, claude 2.1.173)

`~/.claude/.credentials.json` exists (mode 0600) with this shape (values redacted):

```json
{
  "claudeAiOauth": {
    "accessToken": "sk-ant-oat01-…",   // 108 chars
    "refreshToken": "sk-ant-ort01-…",  // 108 chars
    "expiresAt": 1781209313478,        // epoch ms — ~8h lifetime observed
    "scopes": ["user:file_upload"],
    "subscriptionType": "team",
    "rateLimitTier": "default_clau…"
  },
  "mcpOAuth": { … }                    // unrelated; ignore
}
```

Live request returned **HTTP 200** with exactly this body:

```json
{
  "five_hour":   { "utilization": 12.0, "resets_at": "2026-06-12T00:00:00.069303+00:00" },
  "seven_day":   { "utilization": 68.0, "resets_at": "2026-06-13T21:00:00.069327+00:00" },
  "seven_day_oauth_apps": null,
  "seven_day_opus": null,
  "seven_day_sonnet": { "utilization": 0.0, "resets_at": null },
  "seven_day_cowork": null,
  "seven_day_omelette": null,
  "tangelo": null,
  "iguana_necktie": null,
  "omelette_promotional": null,
  "cinder_cove": null,
  "extra_usage": {
    "is_enabled": true, "monthly_limit": null, "used_credits": 240.0,
    "utilization": null, "currency": "EUR", "disabled_reason": null
  }
}
```

**Response-shape gotchas (all observed, not theoretical):**
- `utilization` is a percent float 0–100, already computed server-side.
- `resets_at` is ISO 8601 with sub-second precision + offset — **and can be `null` even when the window object exists** (`seven_day_sonnet` above). Render "—" for null resets.
- Whole window objects can be `null` per plan (`seven_day_opus` is null on this team plan). Only render non-null windows — exactly what SlayZone's `filter` does.
- The payload contains rotating experimental fields (`tangelo`, `iguana_necktie`, `seven_day_cowork`, …). **Decode into a Go struct with only the known fields** — `encoding/json` ignores unknowns natively. Never fail on unrecognized keys.
- `extra_usage` (pay-per-use overflow credits) exists; optional to surface, but it's there if the popup wants it.

### Token lifecycle (the one real risk)

- The access token is **short-lived (~8h observed)**. The claude CLI refreshes it whenever it runs and rewrites `.credentials.json`.
- **Re-read the credentials file on every poll.** Never cache the token in memory beyond one request. File is 2.5KB; cost is nil.
- **Do NOT implement token refresh in Kangent.** The refresh token is in the file and the OAuth flow is known in the ecosystem, but refresh-token rotation means a Kangent-initiated refresh could invalidate the CLI's stored token and break the user's `claude` login. On 401, show "Token expired — run claude to re-authenticate" (SlayZone's exact UX). In practice any running agent session keeps the token fresh.

### Polling etiquette (corroborated externally)

The Claude-Code-Usage-Monitor project (issue #202) independently confirms this endpoint and adds a critical detail: **without the `claude-code/<version>` User-Agent you land in an aggressively rate-limited bucket and get persistent 429s**; with it, polling at sub-minute intervals is safe. Kangent's plan:

- ~60s auto-poll, manual refresh with a 10s server-side hard floor (copy SlayZone's numbers — they're production-tested).
- On 429: parse `Retry-After` (seconds or HTTP-date), back off `max(retryAfter, 30s)`, serve cached data meanwhile.
- Server-side cache + in-flight dedup so N open browser tabs ≠ N upstream requests.

### Architecture: backend proxy, not browser fetch

The browser must **not** call `api.anthropic.com` directly: CORS would block it, and the OAuth token must never reach the frontend. Add one endpoint, e.g. `GET /api/quota` (with `?refresh=1` for manual refresh), returning normalized windows `{key, label, utilization, resetsAt}` + `fetchedAt`. This mirrors the existing settings API pattern.

## Recommended Stack

### Core Technologies (all existing — no additions)

| Technology | Version | Purpose (new use) | Why |
|------------|---------|-------------------|-----|
| `net/http` (stdlib) | Go 1.26 stdlib | GET `https://api.anthropic.com/api/oauth/usage` | One authenticated GET with 4 headers; an HTTP client library would be absurd. Use `http.Client{Timeout: 10 * time.Second}` (SlayZone uses 10s). |
| `encoding/json` + `os.ReadFile` (stdlib) | stdlib | Read `~/.claude/.credentials.json` → `claudeAiOauth.accessToken`; decode usage response | Known fixed paths/shapes, verified above. Unknown response fields ignored for free. |
| `os/exec` + system `tmux` | tmux ≥ 2.1 (3.4 on this machine) | tmux session lifecycle | Same shell-out discipline as git worktrees. tmux's CLI is its API; no Go tmux library is worth a dependency (see What NOT to Use). |
| `exec.LookPath("tmux")` (stdlib) | stdlib | Gate the "tmux" option in `AllowedShells` | Mirrors v1.1's shell validation exactly. Only offer tmux in the dropdown when it resolves. |
| creack/pty v1.1.24 (existing) | existing | The PTY now runs `tmux` instead of `bash` | Nothing changes in the PTY/WS/ring-buffer layer. tmux is just a different full-screen child process. |
| TanStack Query 5 (existing) | existing | Quota auto-poll | `useQuery({ queryKey: ['quota'], refetchInterval: 60_000, refetchIntervalInBackground: false })` + invalidation for manual refresh. Built for exactly this. |
| shadcn/ui (existing copy-in) | CLI latest | Popup + bars | `npx shadcn add hover-card progress` (copied in, not deps — consistent with project convention). `Popover` if click-to-pin is preferred over hover. |

### Supporting Libraries

None needed. Explicitly considered and rejected:

| Candidate | Verdict | Why |
|-----------|---------|-----|
| Any Go OAuth lib (`golang.org/x/oauth2`) | **No** | We consume an existing token from disk; we never run an OAuth flow. |
| Go tmux wrappers (`github.com/jubnzv/go-tmux`, etc.) | **No** | Thin `exec` wrappers around the same CLI calls; low adoption; a dep for ~6 one-line commands violates zero-new-dependency discipline for zero gain. |
| `ccusage` / `claude-monitor` as subprocess | **No** | They focus on cost analytics from local JSONL transcripts; the quota windows come from the OAuth endpoint, which we call directly. Spawning a Node/Python tool to make one HTTP GET is strictly worse. |
| Parsing `claude /usage` TUI output | **No** | `/usage` is an interactive TUI screen, not a scriptable command; scraping ANSI output of a full-screen app is the most fragile possible source for data the endpoint serves as JSON. |

### Development Tools

No changes. tmux 3.4 is already installed at `/bin/tmux`; CI/dev needs nothing new (tmux is runtime-gated by LookPath).

## Part B — tmux Integration Details (verified on tmux 3.4)

### The tmux CLI surface Kangent needs

All commands verified working on this machine. **Use a dedicated socket namespace `-L kangent`** on every invocation so Kangent's sessions never collide with (or appear in) the user's personal tmux server, and `list-sessions` reconciliation only sees Kangent's own sessions.

| Command | Purpose | Notes (verified) |
|---------|---------|------------------|
| `tmux -L kangent new-session -A -s <name> -c <worktree-dir>` | Spawn-or-reattach — **this is the command run inside the PTY** | `-A` attaches if `<name>` exists, creates otherwise; idempotent, so spawn and resume are the same code path. `-c` sets cwd on create, ignored on attach (session keeps its dir). Requires tmux ≥ 1.8. |
| `tmux -L kangent has-session -t =<name>` | Existence check (restart reconciliation, Resume affordance) | Exit 0/1. The `=` prefix forces exact match (without it, `-t foo` prefix-matches `foo-2`). Exit 1 + "no server running" stderr when no server at all — treat as "not found". |
| `tmux -L kangent list-sessions -F '#{session_name} #{session_created} #{session_attached}'` | Startup reconciliation: enumerate surviving sessions | Exits 1 with "no server running on …" when the server is down — treat as empty list, not an error. `session_attached` is a client count (useful for "attached elsewhere" states). |
| `tmux -L kangent kill-session -t =<name>` | Explicit destroy (kill intent, task cleanup) | Server auto-exits when the last session dies; next `new-session` auto-starts it. No server lifecycle management needed, ever. |
| `tmux -L kangent set-option -t <name> status off` | Hide the green status bar so tmux tabs look like plain bash tabs | Verified: session-scoped, overrides user config. Run right after create (or keep the bar as a visual "durable" cue — UX decision). |
| `tmux -L kangent detach-client -s <name>` | Detach from outside (rarely needed) | Usually unnecessary: killing the attach client / closing the PTY detaches automatically (client gets SIGHUP, session survives — that **is** the feature). |

### Detach/reattach mechanics with the existing session layer

- **Detach on tab close:** kill the PTY child (the `tmux … new-session -A` *client* process) exactly as bash tabs are killed today. The tmux client dies; the tmux *server* and session keep running. No new transport code — only the "session exited" semantics differ (process exit ≠ work lost).
- **Reattach:** spawn a fresh PTY running the same `new-session -A -s <name>` command. tmux fully redraws on attach, so the existing ring-buffer + resize-nudge replay machinery works unchanged (the redraw makes replay artifacts moot).
- **Server restart:** Kangent's PTYs die, the tmux server survives (it's a daemon, not a Kangent child). On boot, reconcile persisted tab records against `list-sessions`; surviving names get the Resume affordance (mirror of v1.1's `claude --resume` UX).
- **Session naming:** deterministic, e.g. `kangent-<taskID>-<tabID>`, **restricted to `[A-Za-z0-9_-]`**. Verified: tmux silently rewrites `.` and `:` in `-s` names to `_` (they're target-spec separators), so a name containing them won't round-trip — never derive names from raw slugs without sanitizing.

### Minimum tmux version

Everything used here is ancient: `new-session -A` (1.8, 2013), `=` exact-match targets (2.1, 2015), `-L` sockets and `-F` format strings (older still). **Declare tmux ≥ 2.1, realistically expect ≥ 3.0** — Ubuntu 22.04 ships 3.2a, Debian 12 ships 3.3a, this machine has 3.4. A `tmux -V` parse is unnecessary; LookPath gating is sufficient.

## Installation

```bash
# Backend: nothing. Zero new Go modules.

# Frontend: nothing in package.json. Only shadcn copy-ins if not already present:
npx shadcn@latest add hover-card progress
# (popover likely already present via existing dropdowns; check components/ui/)

# Host prerequisite (runtime-detected, not bundled):
#   tmux >= 2.1 on PATH — feature-gated via exec.LookPath, like shells in v1.1
```

## What NOT to Use

| Avoid | Why | Use Instead |
|-------|-----|-------------|
| Browser-direct fetch of `api.anthropic.com/api/oauth/usage` | CORS-blocked; leaks OAuth token to frontend | Backend proxy `GET /api/quota` with server-side cache |
| Calling the endpoint without `User-Agent: claude-code/<ver>` | Lands in an aggressively rate-limited bucket → persistent 429s (corroborated by Claude-Code-Usage-Monitor #202) | Run `claude --version` once at startup, cache, send `claude-code/<ver>`; hardcoded fallback like SlayZone's |
| Implementing OAuth token refresh in Kangent | Refresh-token rotation can invalidate the claude CLI's stored credentials — you'd break the user's login to draw a progress bar | Re-read `.credentials.json` each poll; on 401 show "run claude to re-authenticate" |
| Strict-decoding the usage response / assuming `resets_at` non-null | Payload carries rotating experimental fields (`tangelo`, `iguana_necktie`, …) and `resets_at: null` occurs in real responses | Lenient struct with only `five_hour`, `seven_day`, `seven_day_opus`, `seven_day_sonnet` (+ optionally `extra_usage`); pointer fields, null-safe rendering |
| Scraping `claude /usage` TUI output | Interactive full-screen ANSI screen; maximally fragile; same data is JSON one GET away | The OAuth usage endpoint |
| Go tmux libraries (go-tmux et al.) | exec wrappers around the same CLI; new dep for six one-liners | `os/exec` + `tmux -L kangent …` |
| Default tmux socket (no `-L`) | Kangent sessions mix with the user's personal tmux server; reconciliation would enumerate (and could kill) the user's own sessions | Dedicated `-L kangent` socket on every invocation |
| `tmux send-keys` / control mode (`-CC`) for I/O | The PTY layer already transports bytes; control mode is an iTerm2-style protocol Kangent doesn't need | Plain `new-session -A` as the PTY child |
| Session names containing `.` or `:` | tmux silently rewrites them to `_` (verified) — stored name ≠ actual name, reconciliation breaks | `kangent-<taskID>-<tabID>` from `[A-Za-z0-9_-]` only |

## Stack Patterns by Variant

**Quota backend (`GET /api/quota`):**
- In-memory cache struct `{payload, fetchedAt, backoffUntil, consecutiveFailures}` behind a mutex + in-flight dedup (a plain mutex/chan is fine; don't add `golang.org/x/sync/singleflight` for one call site).
- Auto-poll path serves cache if `< 60s` old; `?refresh=1` bypasses TTL but never the 10s hard floor; 429 sets `backoffUntil = now + max(RetryAfter, 30s)`.
- On fetch failure keep last good windows with a stale/error field so the UI shows "Updated 9m ago" + warning instead of blanking — drop after 3 consecutive failures (SlayZone's exact policy, production-tested).
- Distinguish "no credentials file" (claude never logged in) from 401 (token expired) — different user messages.
- macOS portability (if ever needed): token lives in the login keychain (`security find-generic-password -s "Claude Code-credentials" -w`), same JSON inside. Linux file path is the only target today.

**tmux scrollback UX (decide during planning, not stack):**
- tmux is a full-screen alternate-screen app, so xterm.js's own scrollback won't accumulate for tmux tabs — scrolling happens via tmux copy-mode. Consider `set-option -t <name> mouse on` at create so wheel-scroll enters copy-mode naturally; otherwise document the difference. This is the one user-visible behavior change vs plain bash tabs.

**Settings seam:**
- Add `"tmux"` to the Go `AllowedShells` slice, gated by `exec.LookPath("tmux")` — the v1.1 seam (SHELL-FUT-01) was built for exactly this. The shell setting selects the PTY child command template, not a different session subsystem.

## Version Compatibility

| Component | Compatible With | Notes |
|-----------|-----------------|-------|
| Endpoint `api/oauth/usage` + `anthropic-beta: oauth-2025-04-20` | claude 2.1.173 credentials (verified 2026-06-11, HTTP 200) | Undocumented API: ship lenient parsing + a graceful "quota unavailable" state so an upstream change degrades, never breaks, the app |
| tmux ≥ 2.1 (3.4 verified) | creack/pty v1.1.24, existing WS/ring-buffer layer | tmux is just another PTY child; zero transport changes |
| shadcn `hover-card`/`progress` | Tailwind 4 + React 19 (existing) | Copy-in components; no dependency tracking |
| TanStack Query 5.x (existing) | `refetchInterval` background polling | Already a dependency; no version change |

## Sources

- [SlayZone `usage.ts`](https://github.com/debuglebowski/SlayZone/blob/main/packages/domains/terminal/src/electron/usage.ts) — full quota implementation read 2026-06-11: endpoint, headers, token paths, cache/backoff policy (HIGH)
- **Empirical, this machine, 2026-06-11:** `~/.claude/.credentials.json` structure (claude 2.1.173); live `GET https://api.anthropic.com/api/oauth/usage` → HTTP 200 with full response body captured above; token `expiresAt` ≈ 8h lifetime (HIGH)
- **Empirical, this machine:** tmux 3.4 — `new-session -A`/`-d`, `has-session -t =`, `list-sessions -F`, `set-option status off`, `kill-session`, `.`/`:` name-sanitization behavior all executed and verified on a throwaway `-L` socket (HIGH)
- [Claude-Code-Usage-Monitor issue #202](https://github.com/Maciek-roboblog/Claude-Code-Usage-Monitor/issues/202) — independent confirmation of endpoint + User-Agent rate-limit-bucket behavior (MEDIUM, corroborates HIGH empirical result)
- tmux changelog knowledge (`new-session -A` in 1.8; `=` exact-match in 2.1) — version floor only; all commands verified live on 3.4 regardless (MEDIUM)

---
*Stack research for: Kangent v1.2 — Claude quota indicator + tmux-backed resumable shells*
*Researched: 2026-06-11*

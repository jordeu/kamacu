# Phase 7: Claude Quota Indicator - Research

**Researched:** 2026-06-12
**Domain:** Anthropic OAuth usage endpoint proxy (Go) + compact quota indicator UI (React/shadcn)
**Confidence:** HIGH — every load-bearing claim verified empirically on this machine today (2026-06-12) or read at line level from the actual codebase; builds on the milestone research (`.planning/research/STACK.md`, `ARCHITECTURE.md`, `PITFALLS.md`, `FEATURES.md`) without redoing it

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

#### Compact trigger look
- **D-68:** Trigger is SlayZone-exact: "Claude 5h" text + thin colored bar underneath (per the reference screenshot). No percentage number on the trigger; percentages live in the popup.
- **D-69:** Bar colors are **neutral-until-it-matters**: muted zinc below 60%, yellow 60–84%, red ≥85%. Green NEVER appears — preserves the project's reserved-color-budget / quiet-by-default palette (status dots own green/amber/gray for agent state). This intentionally diverges from SlayZone's full traffic-light bars. Popup row bars follow the same scheme.
- **D-70:** Red-zone emphasis (QUOTA-07) is the red bar alone — no red label text, no motion, no badge. With D-69, red only exists ≥85%, so the red bar IS the "stop starting agents" signal.

#### No-data states
- **D-71:** No OAuth credentials at all (file absent / API-key-only user): hide the trigger entirely. Server reports unavailable; frontend renders nothing in the header.
- **D-72:** Credentials exist but token expired/rejected (401) with no cached data: trigger stays visible as a warning chip; popup explains the token expired and says to run `claude` to re-authenticate. Quota silently vanishing after ~8h token expiry would be confusing — this state must be discoverable.
- **D-73:** Stale-data display (fetch failing, cache not yet dropped): subtle, popup-only. Trigger keeps the last-known bar unchanged; popup footer shows "error · Xm old" in amber text. After 3 consecutive failures the cached windows drop (per QUOTA-06) — then the 401 case renders D-72's warning chip and other errors render an equivalent warning-chip state.

#### Popup content details
- **D-74:** Row labels are full short names: "5h", "7d", "Opus", "Sonnet" — not SlayZone's truncated "Son.". Unknown/future window keys render their raw API key name (passthrough — server-driven window set per QUOTA-02).
- **D-75:** Row order: API order with 5h first, then 7d, then per-model windows as returned. No utilization-based sorting (rows must not jump between polls).
- **D-76:** Footer is SlayZone-exact: "Updated Xm ago" left, refresh icon button right. No persistent last-error line; errors surface via D-72/D-73 states.

### Claude's Discretion
- Exact trigger dimensions/spacing (reference: SlayZone's ~48×6px bar), hover affordance, popup width.
- Per-row reset copy ("Resets in Xh Ym" countdown — format details, "now" case when resets_at is past/null).
- Warning-chip visual for D-72 (and non-401 total-failure states) — small, motion-free, consistent with existing muted styling.
- Loading state before first fetch resolves (likely render nothing or a skeleton-free quiet placeholder).

### Deferred Ideas (OUT OF SCOPE)
- Extra-usage credits block in the popup (QUOTA-FUT-01) — endpoint exposes it; deferred to v2+
- Pinnable inline bars / choosing which windows show compact (QUOTA-FUT-02)
- Quota threshold notifications (QUOTA-FUT-03, blocked on NOTF-01)
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| QUOTA-01 | Compact "Claude 5h" indicator with threshold-colored bar, top-right, board + task view | Exact mount points verified (BoardPage.tsx:46–50, TaskPage.tsx:383–450); D-69 overrides roadmap colors: zinc <60, never green; existing palette classes surveyed for consistency |
| QUOTA-02 | Hover popup lists every API-returned window with bar, %, "Resets in" countdown | Live response shape re-verified 2026-06-12 (HTTP 200, identical to STACK.md capture); server-driven window list pattern; radix-ui HoverCard verified available with zero new deps |
| QUOTA-03 | Popup footer "Updated Xm ago" + manual refresh bypassing cache | `?refresh=1` server param + TanStack mutation-then-`setQueryData` pattern (precedent: `useSpawnSession`); 10s server hard floor still applies |
| QUOTA-04 | ~60s visible-only auto-refresh | `useQuery({ refetchInterval: 60_000, refetchIntervalInBackground: false })` — `useAgentStatuses` is the in-repo template; TanStack dedupes across both mounts |
| QUOTA-05 | Server fetch with creds file, claude-code UA, 60s TTL, 10s floor, dedup, 429 backoff, never write token | Headers verified live today; `claude --version` probed → concrete UA strategy below; demand-driven cache design (no background poller) |
| QUOTA-06 | Six-state graceful degradation, stale display, 3-failure drop, hidden for API-key users | Six-state matrix mapped to a unit-testable seam design (injectable creds path + base URL + httptest), mirroring the `transcriptExists(globRoot, …)` precedent |
| QUOTA-07 | Red-zone trigger emphasis when ANY window ≥85% | Subtle interaction with D-68 documented: trigger bar width = 5h utilization, color = max across all windows (see Pattern 4) |
| QUOTA-08 | Cache keyed by token — account switch never shows stale account data | Token-fingerprint comparison in the cache struct; creds file re-read per poll already required (file rewritten frequently — scopes changed between 06-11 and 06-12 snapshots) |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

- **GSD workflow enforcement:** file changes go through `/gsd:execute-phase` (or other GSD entry points) — this research is planning input only.
- **Commit policy:** never mention or co-author Claude / happy-otter on commits or PRs.
- **Stack constraints honored by this phase:** Go backend stdlib-first (no new Go modules), React 19 + TanStack Query 5 + Tailwind 4 + shadcn copy-ins (no new npm deps — verified below), `log/slog` for logging, single-binary embed unaffected.
- **What NOT to use (relevant subset):** no browser-direct Anthropic fetch; no SDK-driven anything; binary/stdlib discipline.

## Summary

Everything needed to plan this phase is now verified. The upstream endpoint (`GET https://api.anthropic.com/api/oauth/usage`) was re-verified live today: HTTP 200, response shape byte-identical to the 06-11 STACK.md capture — `five_hour`/`seven_day` populated, `seven_day_opus` null, `seven_day_sonnet` present with `resets_at: null`, rotating experimental fields, `extra_usage` present (deferred). The credentials file (`~/.claude/.credentials.json`, mode 0600) still has the exact `claudeAiOauth.{accessToken, expiresAt(epoch-ms int), …}` shape — and notably its `scopes` array changed between yesterday's snapshot and today, confirming claude rewrites the file frequently and the re-read-per-poll rule is mandatory, which also makes QUOTA-08's token-keyed cache load-bearing in practice.

The open items from context are all resolved empirically: (1) `claude --version` prints `2.1.174 (Claude Code)` — version drifted from 2.1.173 in one day, so probe-at-startup with a pinned fallback is the right call (concrete recipe below); (2) the shadcn `hover-card` add is genuinely zero-new-dependency — this project uses the consolidated `radix-ui@1.5.0` package which exports `HoverCard`, and the registry item for the project's `radix-nova` style declares `dependencies: none` and imports `from "radix-ui"`; (3) exact header mount points located at line level in both pages; (4) the Go testability seam has a direct in-repo precedent (`transcriptExists(globRoot, …)` + `defaultTranscriptGlobRoot()` in resume.go) — `internal/quota` takes an injectable credentials path and base URL, making the whole six-state matrix unit-testable with `t.TempDir()` + `httptest.Server` (already used across `internal/api` tests).

One design recommendation that goes slightly beyond the milestone research: make the server cache purely **demand-driven** (no background goroutine — the browser's 60s poll is the only trigger; the TTL gates upstream calls). This is simpler than a ticker, matches SlayZone's actual design, and closes the STATE.md tech-debt note about idle polling outright: no connected browser ⇒ zero upstream requests.

**Primary recommendation:** Build `internal/quota` as a DB-free service with injectable `credentialsPath`/`baseURL`/`userAgent`, a mutex-guarded token-keyed cache, and a five-value state enum the frontend maps 1:1 onto D-71/D-72/D-73; expose it at `GET /api/usage` via the existing `XxxRoutes(mux, …)` pattern; render one `QuotaIndicator` component (hover-card + hand-rolled 48×6px div bars) mounted in both page headers.

## Standard Stack

### Core (zero additions — all verified present)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `net/http`, `encoding/json`, `os`, `sync`, `time` (Go stdlib) | Go 1.26 (go.mod) | Upstream GET, creds parsing, cache, TTL | One authenticated GET with 4 headers; CLAUDE.md stdlib discipline |
| `os/exec` (stdlib) | stdlib | `claude --version` probe for the User-Agent | Mirrors `agent.go` ClaudeBin resolution (flag value or `exec.LookPath("claude")`) |
| `@tanstack/react-query` | ^5.101.0 (installed) | 60s visible-only poll + manual-refresh mutation | `useAgentStatuses` (web/src/api/agents.ts) is the in-repo template |
| `radix-ui` | ^1.5.0 (installed) | HoverCard primitive | **Verified:** `node_modules/radix-ui/dist/index.d.ts:25-26` re-exports `@radix-ui/react-hover-card` as `HoverCard` |
| shadcn CLI | ^4.11.0 (already in web/package.json deps) | Copy in `hover-card.tsx` | `components.json` present (style `radix-nova`); registry item verified — see below |
| `lucide-react` | ^1.17.0 (installed) | Refresh icon (`RefreshCw`), warning icon (`TriangleAlert`) | Existing icon library |

### shadcn hover-card: verified copy-in behavior

Fetched the actual registry item (`https://ui.shadcn.com/r/styles/radix-nova/hover-card.json`, 2026-06-12):
- `dependencies: none`, `registryDependencies: none` — **no package.json change**
- Generated file imports `import { HoverCard as HoverCardPrimitive } from "radix-ui"` and `cn` from utils — exactly the pattern of the existing `tooltip.tsx`/`dropdown-menu.tsx`
- Exports `HoverCard`, `HoverCardTrigger`, `HoverCardContent` (Portal-wrapped, `w-64` default content — popup width is discretion; override via className)

```bash
cd web && npx shadcn@latest add hover-card
# writes web/src/components/ui/hover-card.tsx only; verify no package.json diff afterward
```

Hand-copying is unnecessary — the CLI output is deterministic and matches project conventions. (Phase 6 precedent of sanctioned shadcn blocks holds.)

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Hand-rolled div bars (two nested divs, width %) | shadcn `progress` | Progress adds a Radix primitive + ARIA progressbar semantics for a 48×6px decorative bar needing custom threshold colors; the div is less code. CONTEXT sanctions only hover-card — **use div bars** |
| Demand-driven server cache | Background ticker goroutine | Ticker polls an undocumented endpoint 24/7 on an idle server (STATE.md tech-debt note); demand-driven gives zero idle traffic for free — **use demand-driven** |
| Plain mutex + "fetching" flag for in-flight dedup | `golang.org/x/sync/singleflight` | New dependency for one call site; STACK.md already rejected it |

## Architecture Patterns

### Recommended Structure (new/modified files only)

```
internal/
├── quota/
│   ├── quota.go          # NEW: Service (cache, TTL, floor, backoff, token-keying), creds read, upstream fetch
│   └── quota_test.go     # NEW: six-state matrix via httptest + t.TempDir
├── api/
│   └── usage.go          # NEW: UsageRoutes(mux, *quota.Service) → GET /api/usage
cmd/kangent/main.go        # MOD: construct quota.Service, api.UsageRoutes(mux, qs) at main.go:103-109
web/src/
├── api/
│   └── usage.ts          # NEW: useQuota() + useRefreshQuota()
├── components/
│   ├── quota/QuotaIndicator.tsx   # NEW: trigger + popup, one file
│   └── ui/hover-card.tsx          # NEW: shadcn CLI copy-in
└── pages/
    ├── BoardPage.tsx     # MOD: mount (line ~49)
    └── TaskPage.tsx      # MOD: mount (line ~424)
```

Note: ARCHITECTURE.md sketched a `Fetcher` interface seam because the fetch mechanism was then owned by parallel research. That research is done — the mechanism is known and trivial (one GET). **Collapse the seam to injectable config instead of an interface:** `credentialsPath`, `baseURL`, `userAgent`, `now func() time.Time` (optional). An interface for one prod implementation is ceremony; injectable strings cover all six test states. (If the planner prefers the interface, it's harmless — but config injection is the smaller honest design.)

### Pattern 1: `internal/quota.Service` — demand-driven, token-keyed cache

```go
// internal/quota/quota.go — shape, not final code
type Service struct {
    mu        sync.Mutex
    credsPath string        // default: filepath.Join(home, ".claude", ".credentials.json")
    baseURL   string        // default: "https://api.anthropic.com"
    userAgent string        // "claude-code/<probed version>" — see Pattern 5
    client    *http.Client  // Timeout: 10 * time.Second (SlayZone's value)

    // cache state (all guarded by mu)
    cached       *Snapshot  // last-good normalized windows
    tokenFP      string     // sha256 of the token the cache was fetched with (QUOTA-08)
    fetchedAt    time.Time  // drives "Updated Xm ago" AND the 60s TTL
    lastAttempt  time.Time  // drives the 10s hard floor (refresh spam guard)
    backoffUntil time.Time  // 429: now + max(Retry-After, 30s)
    failures     int        // consecutive; >= 3 ⇒ drop cached windows (QUOTA-06)
    inflight     bool       // in-flight dedup: concurrent Get() waits or serves cache
}

func (s *Service) Get(ctx context.Context, force bool) Result
```

Per-call algorithm (each browser poll / manual refresh):
1. **Read creds fresh** (`os.ReadFile(credsPath)`, decode only `claudeAiOauth.{accessToken, expiresAt}`; discard everything else immediately — the file also carries `mcpOAuth` third-party secrets, PITFALLS [E1]).
2. File absent / key absent / empty token → `state: "no_credentials"`, no upstream call ever (D-71).
3. **Token change check (QUOTA-08):** `sha256(token) != s.tokenFP` → drop `cached`, reset failures, then proceed.
4. `expiresAt` (epoch **ms**) in the past → don't burn an upstream request on a guaranteed 401: behave as 401 (`auth_expired`; serve stale if cache held per D-73, else D-72 state).
5. Serve cache if: not force and `fetchedAt` within 60s; or `now < backoffUntil`; or force but `lastAttempt` within the 10s floor; or another fetch is in flight.
6. Otherwise fetch: `GET {baseURL}/api/oauth/usage` with `Authorization: Bearer <token>`, `anthropic-beta: oauth-2025-04-20`, `Content-Type: application/json`, `User-Agent: {userAgent}`.
7. On 200: lenient decode (pointer fields; unknown keys ignored natively), normalize to ordered windows (Pattern 3), cache + `tokenFP` + reset failures.
8. On 401: `state: "auth_expired"` (D-72); failures++; keep/serve stale until 3 failures.
9. On 429: `backoffUntil = now + max(parseRetryAfter, 30s)`; failures++; serve stale.
10. On 5xx / network / malformed JSON: failures++; serve stale; at `failures >= 3` drop `cached` → state becomes `auth_expired` (if last error was 401) or `"error"`.

The 10s floor binds `lastAttempt` (attempts), the 60s TTL binds `fetchedAt` (successes) — keep them separate fields or a failing endpoint gets hammered by the 60s poll exactly when it shouldn't.

### Pattern 2: `GET /api/usage` contract (always 200 — degrade, never error)

```jsonc
// GET /api/usage          → cached-or-fresh per TTL
// GET /api/usage?refresh=1 → force (server still enforces 10s floor + backoff)
{
  "state": "ok",                    // "ok" | "no_credentials" | "auth_expired" | "error"
  "stale": false,                   // true ⇒ windows are last-good from a previous success (D-73)
  "fetchedAt": "2026-06-12T07:10:11Z",  // last SUCCESSFUL fetch; null until first success
  "windows": [                      // null/empty when no data to show
    { "key": "five_hour",        "label": "5h",     "utilization": 15, "resetsAt": "2026-06-12T07:49:59Z" },
    { "key": "seven_day",        "label": "7d",     "utilization": 73, "resetsAt": "2026-06-13T20:59:59Z" },
    { "key": "seven_day_sonnet", "label": "Sonnet", "utilization": 0,  "resetsAt": null }
  ]
}
```

Frontend state mapping (exactly one UI per server state — this is the six-state matrix collapsed to its render targets):

| Server state | Windows | Trigger renders (per decisions) |
|---|---|---|
| `no_credentials` | — | **nothing** (component returns null) — D-71 |
| `ok`, `stale:false` | present | normal bar — D-68/D-69 |
| `ok`/`auth_expired`/`error`, `stale:true` | present (cached) | last-known bar unchanged; popup footer "error · Xm old" amber — D-73 |
| `auth_expired`, no windows | null | warning chip; popup: token expired, run `claude` — D-72 |
| `error`, no windows | null | warning chip (D-72-equivalent visual, discretion) |

Registration follows the existing convention exactly (`internal/api/usage.go`, using `writeJSON` from respond.go):

```go
func UsageRoutes(mux *http.ServeMux, qs *quota.Service) {
    mux.HandleFunc("GET /api/usage", func(w http.ResponseWriter, r *http.Request) {
        writeJSON(w, http.StatusOK, qs.Get(r.Context(), r.URL.Query().Get("refresh") == "1"))
    })
}
// cmd/kangent/main.go (after line 109): api.UsageRoutes(mux, quotaSvc)
```

The route inherits loopback enforcement from the server listen address like every other route; no extra guard needed (PITFALLS security table: "new quota route skipping existing protections" — it doesn't, by construction).

### Pattern 3: Window normalization (server-driven set, QUOTA-02 / D-74 / D-75)

Decode into a struct with only known keys, but **emit windows in a fixed preferred order with passthrough for the rest**:

```go
type usageWindow struct {
    Utilization *float64 `json:"utilization"`
    ResetsAt    *string  `json:"resets_at"`   // ISO 8601, CAN be null (observed live, twice)
}
// Known order + labels (D-74/D-75): five_hour→"5h", seven_day→"7d",
// seven_day_opus→"Opus", seven_day_sonnet→"Sonnet".
// Skip null window objects entirely (observed: seven_day_opus null on team plan).
// Skip noise keys: seven_day_oauth_apps, tangelo, iguana_necktie, … are not
// decoded (struct omits them) — if Anthropic ships a NEW meaningful window key,
// it appears once the struct adds it; D-74's raw-key passthrough applies to
// windows the server chooses to forward, keeping label lookup map-with-fallback.
```

Planner note on D-74 passthrough: with `encoding/json` struct decoding, truly unknown keys never reach the code. The honest reading: hold the known four in the struct (matching every observed real window), render `label` from a `map[key]label` with raw-key fallback so a future struct addition needs no frontend change. Decoding into `map[string]json.RawMessage` to forward genuinely unknown keys is the stricter interpretation of "server-driven" — slightly more code, also acceptable. Either satisfies QUOTA-02; the map-decode version is more future-proof and recommended (the experimental noise keys are all `null`, and null windows are skipped anyway — observed live, the filter handles them).

### Pattern 4: Trigger color vs trigger width (the one subtle requirement interaction)

D-68: trigger bar shows the **5h** window. QUOTA-07/D-70: red emphasis when **any** window ≥85%, expressed as the red bar alone. These compose as:

- **Bar width** = `five_hour.utilization` (it's the "Claude 5h" bar)
- **Bar color** = threshold color of `max(utilization across all returned windows)` — zinc <60, yellow 60–84, red ≥85

So a 20%-full bar turns red when the 7d window hits 85% — width stays honest to 5h, color carries the global stop signal. This must be explicit in the plan or QUOTA-07 silently regresses to 5h-only.

Color classes (consistency with the existing reserved palette, verified in-repo): `bg-zinc-600` (muted idle/exited family, StatusDot.tsx:31), `bg-amber-400` for the 60–84 band ("yellow" in D-69 — amber-400 is this codebase's yellow: waiting dot + sidebar chip both use it), `bg-red-500` (≥85, matches stopped-state red). Stale footer text: `text-amber-400` (matches ProjectSidebar.tsx:81 chip). Exact shades are discretion; these are the consistent choices.

### Pattern 5: User-Agent strategy (open item — RESOLVED empirically)

`claude --version` on this machine (2026-06-12) prints: `2.1.174 (Claude Code)` — single line, version first, space, parenthetical. It was 2.1.173 in the 06-11 research — **drifts roughly daily**, so a hardcoded UA goes stale within weeks (PITFALLS tech-debt table flagged exactly this).

**Concrete approach:**
1. Resolve the binary the way `agent.go:24` does: the `-claude-bin` flag value if set, else `exec.LookPath("claude")`. Plumb the existing flag from main.go:29 into the quota service constructor.
2. At Service construction (or lazily via `sync.Once` on first fetch — either fine; construction is simpler), run `claude --version` with a short timeout (~5s), take the first whitespace-separated token, validate against `^\d+\.\d+\.\d+`.
3. On any failure (binary missing, parse miss, timeout): fall back to the pinned constant `"2.1.174"` (known-good today; bump opportunistically).
4. Probe **once per process** — version can't change under a running server in any way that matters, and the probe spawns a Node process (claude startup is not free).

UA format: `claude-code/<version>` — exactly this string shape; it selects the lenient rate-limit bucket (PITFALLS Pitfall 2, claude-code issue #31021).

### Pattern 6: Frontend hooks (copy `useAgentStatuses`, add the refresh mutation)

```typescript
// web/src/api/usage.ts
export function useQuota() {
  return useQuery({
    queryKey: ["usage"],
    queryFn: () => get<UsageResponse>("/api/usage"),
    refetchInterval: 60_000,
    refetchIntervalInBackground: false, // QUOTA-04: visible-only (explicit, though false is the default)
  });
}
export function useRefreshQuota() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => get<UsageResponse>("/api/usage?refresh=1"),
    onSuccess: (data) => qc.setQueryData(["usage"], data), // write-then-done, useSpawnSession precedent
  });
}
```

TanStack dedupes the `["usage"]` query across the two mounts (BoardPage + TaskPage) — only one is mounted at a time anyway (different routes), but the pattern also covers any future co-mount. Disable the refresh button while the mutation is pending; the server's 10s floor is the real guard (a floored refresh just returns the cache — harmless).

### Pattern 7: Exact mounts (verified at line level, 2026-06-12)

**BoardPage.tsx:46–50** — header is `flex items-center justify-between`; the right side is currently the bare `New task` button. Wrap it:
```tsx
<header className="flex items-center justify-between px-6 py-4">
  <h1 …>{project?.name ?? ""}</h1>
  <div className="flex items-center gap-3">
    <QuotaIndicator />
    <Button onClick={…}>New task</Button>   {/* stays the page's only inverted element */}
  </div>
</header>
```

**TaskPage.tsx:383–450** — header is `flex items-center gap-2`: back button, `flex-1` title (button or Input), then the `DropdownMenu` (Ellipsis). Insert `<QuotaIndicator />` between the title block and the `<DropdownMenu>` (~line 424) — the flex-1 title pushes it right, beside the three-dots menu, per CONTEXT.

No shared AppLayout header exists — do NOT invent one (architecture research; re-confirmed: AppLayout renders sidebar + bare `<main><Outlet/></main>`).

### Anti-Patterns to Avoid
- **Browser-direct Anthropic fetch** — CORS-blocked and leaks the token; server proxy only (CLAUDE.md What-NOT-to-use + PITFALLS Anti-Pattern 1).
- **Strict decode / non-null assumptions** — `resets_at: null` and whole-window null both observed live today.
- **Token in memory beyond one request / token in logs / token in `/api/usage` payload** — re-read per poll; response carries digested numbers only; grep audit in verification.
- **Implementing token refresh** — read-only passenger, period (Pitfall 1; STATE.md standing decision).
- **A background poll goroutine** — demand-driven is strictly better here (idle server = zero upstream traffic).
- **Fabricated 0% bars for API-key users** — reads as "full quota"; `no_credentials` renders nothing (D-71, QUOTA-06).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Hover popup that stays open while hovered (refresh button must be clickable) | Custom mouseenter/leave timers | Radix `HoverCard` via shadcn copy-in | Pointer-travel grace, portal, positioning, a11y all solved; already installed via `radix-ui@1.5.0` |
| HTTP retry/backoff semantics for 429 | Generic retry library | `Retry-After` header parse + `backoffUntil` timestamp in the cache struct | One endpoint, one policy (max(Retry-After, 30s)); SlayZone-proven numbers |
| "Xm ago" / "Resets in Xh Ym" formatting | date-fns / dayjs | ~15-line helper from ms deltas | Zero-dep discipline; format catalog is fixed (now / 37m / 4h 12m / 2d 5h, FEATURES.md) |
| The bars themselves | shadcn `progress` | Two nested divs, `style={{width: pct+"%"}}` | 48×6px decorative bar with threshold colors; Progress is the heavier wrong tool here |

**Key insight:** the only genuinely fiddly UI problem (hover persistence into the popup) is exactly what Radix HoverCard exists for; everything else in this phase is deliberately small.

## Runtime State Inventory

Not applicable — greenfield additive phase (no rename/refactor/migration). No DB migration, no stored data, no OS-registered state. The one external runtime dependency (the credentials file) is read-only by hard requirement.

## Common Pitfalls

(Quota-specific distillation of PITFALLS.md Pitfalls 1–4 plus new findings; tmux pitfalls are Phases 8–9.)

### Pitfall 1: Treating the credentials file as stable
**What goes wrong:** caching the token, or assuming the file's contents persist between polls.
**Why:** claude rewrites it constantly — observed: `scopes` array changed between 06-11 and 06-12; token rotates ~8h.
**Avoid:** `os.ReadFile` per `Get()` call (2.5KB — free); token-fingerprint cache keying (QUOTA-08) makes account switches self-correcting.
**Warning signs:** any token field on the Service struct that outlives one request.

### Pitfall 2: Missing/wrong User-Agent → permanent 429 bucket
**What goes wrong:** requests without `claude-code/<ver>` land in an aggressively limited bucket; the indicator never recovers.
**Avoid:** Pattern 5 probe + pinned fallback `2.1.174`. Verification: a 429-state test must assert the UA header was sent (httptest captures headers).

### Pitfall 3: Conflating the 10s floor with the 60s TTL
**What goes wrong:** one timestamp serves both → a failing upstream gets retried every 60s poll forever (failures never had a successful `fetchedAt`, so "cache expired" is always true).
**Avoid:** separate `lastAttempt` (floor, set on every attempt) from `fetchedAt` (TTL, set on success); `backoffUntil` overrides both.
**Warning signs:** burst of upstream requests in logs during an outage.

### Pitfall 4: Token/secret leakage
**What goes wrong:** token in a wrapped error (`fmt.Errorf("...: %v", req)`), in an slog call, or in the `/api/usage` body; the file also contains `mcpOAuth` third-party secrets.
**Avoid:** decode only the two needed fields; build errors from status codes + truncated bodies; slog status codes only.
**Verification (roadmap-mandated):** `grep -ri "sk-ant-oat"` over server logs and a captured `/api/usage` response — must be empty; also grep the test files (don't paste real tokens into fixtures — generate fakes like `sk-ant-oat01-` + padding).

### Pitfall 5: Trigger color ignoring non-5h windows
**What goes wrong:** the natural implementation colors the trigger bar by its own (5h) value — QUOTA-07 silently unmet; on this very machine today 7d=73% while 5h=15%, so the divergence is the *normal* case, not an edge.
**Avoid:** Pattern 4 (width=5h, color=max-of-all). Verification: a state where 5h<60 but another window ≥85 must show a red bar.

### Pitfall 6: Expired-token requests and the D-72/D-73 boundary
**What goes wrong:** (a) calling upstream with a locally-expired token burns rate budget on guaranteed 401s every poll; (b) the first 401 blanking the indicator (should serve stale for 3 failures first — D-73 precedes D-72).
**Avoid:** local `expiresAt` check short-circuits to the 401 path without a request; 401 increments the same `failures` counter and only after the cache drops does the warning chip appear.
**Nuance:** an expired local token does NOT mean stale displayed data is wrong — any running claude session refreshes the file underneath; the next poll's re-read picks it up and recovers automatically. The state machine must allow `auth_expired → ok` with no special casing.

### Pitfall 7: "Updated Xm ago" drifting stale on screen
**What goes wrong:** the footer text renders once and never updates between polls; after a few failed polls it reads "Updated 0m ago" forever.
**Avoid:** compute from `fetchedAt` at render; a cheap 30s interval re-render (or rely on the 60s query tick — each poll re-renders regardless of data change since `dataUpdatedAt` changes) keeps it honest. Minute granularity makes the 60s tick sufficient — note it in the plan and skip the extra timer.

## Code Examples

### Six-state test matrix (the verification core — all unit-testable, no real creds)

```go
// internal/quota/quota_test.go — seam usage, precedent: transcriptExists(globRoot,…)
func newTestService(t *testing.T, credsJSON string, handler http.HandlerFunc) *Service {
    dir := t.TempDir()
    path := filepath.Join(dir, ".credentials.json")
    if credsJSON != "" {
        os.WriteFile(path, []byte(credsJSON), 0o600)
    }
    srv := httptest.NewServer(handler)
    t.Cleanup(srv.Close)
    return New(Config{CredentialsPath: path, BaseURL: srv.URL, UserAgent: "claude-code/test"})
}

// State 1 — file absent:        credsJSON="" (no file)            → state "no_credentials", zero handler hits
// State 2 — key absent:         `{"mcpOAuth":{}}`                  → "no_credentials", zero handler hits
// State 3 — expired:            valid shape, expiresAt: 1          → "auth_expired", zero handler hits
// State 4 — 429:                handler: Retry-After: 60, 429      → stale served, no re-request before backoffUntil
// State 5 — 5xx:                handler: 503 ×3                    → stale, stale, then windows dropped → "error"
// State 6 — malformed:          handler: 200 "not json"            → same failure path as 5xx
// Plus: 401 → "auth_expired"; token-change → cache dropped (QUOTA-08);
//       happy path asserts UA + anthropic-beta headers on the captured request.
// Fake token: "sk-ant-oat01-" + strings.Repeat("x", 95)  (108 chars, greppable-safe)
```

### Lenient response decode (shape verified live 2026-06-12)

```go
// Captured live: five_hour/seven_day populated; seven_day_opus null;
// seven_day_sonnet {utilization:0, resets_at:null}; noise keys all null.
type oauthUsageResponse map[string]json.RawMessage // map-decode variant (recommended, Pattern 3)

type usageWindow struct {
    Utilization *float64 `json:"utilization"`
    ResetsAt    *string  `json:"resets_at"`
}
// Iterate a fixed preference order [five_hour, seven_day, seven_day_opus, seven_day_sonnet],
// then remaining keys whose RawMessage decodes into usageWindow with non-nil Utilization
// (skips nulls and extra_usage, whose shape doesn't match) — D-74 passthrough, D-75 order.
```

### Credentials extraction (only these fields — file holds unrelated secrets)

```go
type credsFile struct {
    ClaudeAiOauth struct {
        AccessToken string `json:"accessToken"`
        ExpiresAt   int64  `json:"expiresAt"` // epoch MILLISECONDS (verified: 1781238586291)
    } `json:"claudeAiOauth"`
}
// expired := time.UnixMilli(c.ClaudeAiOauth.ExpiresAt).Before(time.Now())
```

### Verbatim-copy strings (project convention: single grep-able template literals)

The user-facing strings the plan should pin as literals in `QuotaIndicator.tsx`:
- Trigger label: `Claude 5h`
- D-72 popup copy: token-expired explanation directing the user to run `claude` (exact wording is discretion; SlayZone's: ``Token expired — re-authenticate with `claude` ``)
- D-73 footer: `error · {m}m old`
- D-76 footer: `Updated {m}m ago`
- Row labels: `5h`, `7d`, `Opus`, `Sonnet`

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Roadmap's green <60 threshold color | D-69: zinc <60, green never appears | Phase 7 discuss (2026-06-12) | QUOTA-01's "green" wording is overridden by CONTEXT — plans must follow D-69 |
| Per-component `@radix-ui/react-*` packages | Consolidated `radix-ui` package (this repo, v1.5.0) | shadcn radix-nova era | hover-card add is zero-dependency; imports `from "radix-ui"` |
| `Fetcher` interface seam (ARCHITECTURE.md) | Injectable config (path/URL/UA) | This research | Mechanism is now known; config injection is the smaller test seam |
| Idle-server background quota polling (STATE.md tech-debt note) | Demand-driven cache, no ticker | This research | Tech-debt note resolved by design: no browser ⇒ no upstream calls |

**Verified current (2026-06-12):** endpoint returns HTTP 200 with the exact 06-11 shape; claude 2.1.174; credentials schema unchanged (scopes contents drifted, schema didn't).

## Open Questions

1. **Upstream endpoint longevity** — `api/oauth/usage` is undocumented and formally gray-area (Feb 2026 credential policy). Not resolvable; handled by design: best-effort, lenient decode, `no_credentials`/`error` degradation, feature removable at zero cost. Already a tracked concern in STATE.md.
2. **D-74 passthrough depth** — struct-decode (known 4 windows, label map with fallback) vs map-decode (genuinely forwards unknown window keys). Recommendation: map-decode (Pattern 3); planner may choose either — both satisfy QUOTA-02.
3. **Warning-chip visual (D-72)** — discretion. Recommendation: keep the `Claude 5h` text with a small `TriangleAlert` icon in `text-amber-400` replacing the bar — same footprint, motion-free, palette-consistent. Final call at implementation.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| `claude` CLI | UA version probe | ✓ | 2.1.174 (Claude Code) | Pinned UA constant `2.1.174` |
| `~/.claude/.credentials.json` | Quota fetch | ✓ (0600, valid token at research time) | claudeAiOauth schema verified | `no_credentials` state (D-71) — also a supported runtime condition, not just a fallback |
| api.anthropic.com reachability | Upstream GET | ✓ (HTTP 200 verified today) | — | Stale/error degradation (D-73) |
| `radix-ui` npm pkg | HoverCard | ✓ installed | 1.5.0 (exports HoverCard) | — |
| shadcn CLI + registry | hover-card copy-in | ✓ (local dep 4.11.0; registry item fetched OK) | — | Hand-copy the verified registry file content |
| Node/npm, Go toolchain, Vite proxy | Build/dev | ✓ (existing project infra; `/api` proxy at vite.config.ts:10 covers the new route) | Go 1.26 | — |

**Missing dependencies with no fallback:** none.

## Validation Architecture

Skipped — `.planning/config.json` sets `workflow.nyquist_validation: false` explicitly. (Testing guidance is embedded above: the six-state matrix in Code Examples is the verification core; `go test ./internal/quota ./internal/api` plus the grep audit and the live degradation checks from the roadmap's phase notes cover QUOTA-01..08.)

## Sources

### Primary (HIGH confidence)
- **Empirical, this machine, 2026-06-12:** `claude --version` → `2.1.174 (Claude Code)`; credentials file keys/types re-verified (epoch-ms `expiresAt`, scopes drifted since 06-11); live `GET /api/oauth/usage` → HTTP 200, shape identical to STACK.md capture; `radix-ui@1.5.0` HoverCard export confirmed in installed `dist/index.d.ts`
- **shadcn registry, fetched 2026-06-12:** `ui.shadcn.com/r/styles/radix-nova/hover-card.json` — `dependencies: none`, imports `from "radix-ui"`
- **Kangent codebase (line-verified today):** BoardPage.tsx:46–50, TaskPage.tsx:383–450 (mounts); routes.go/main.go:103–115 (registration pattern); respond.go (writeJSON); resume.go:16–37 (injectable-root seam precedent); agents.ts (poll hook template); client.ts (get helper); agent.go:24 + main.go:29 (claude-bin resolution); StatusDot.tsx/ProjectSidebar.tsx (palette); vite.config.ts:10 (proxy); components.json; web/package.json; go.mod; `.planning/config.json`
- `.planning/research/STACK.md`, `ARCHITECTURE.md`, `PITFALLS.md`, `FEATURES.md` (2026-06-11, themselves empirically grounded) — cache/backoff policy, SlayZone UX catalog, security pitfalls, integration map

### Secondary (MEDIUM confidence)
- claude-code issue #31021 (UA rate-limit bucket), Claude-Code-Usage-Monitor #202 (endpoint corroboration) — via PITFALLS.md, corroborating today's HIGH empirical results

### Tertiary (LOW confidence)
- None load-bearing.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — zero new deps, every claim checked against installed packages and the live registry
- Architecture: HIGH — integration points line-verified today; patterns extend in-repo precedents
- Pitfalls: HIGH — grounded in yesterday's empirical milestone research plus today's re-verification (one new pitfall — trigger color vs width — derived directly from today's live data where 7d=73% vs 5h=15%)
- Endpoint stability: inherently MEDIUM (undocumented API) — mitigated by the degrade-don't-break design, not by confidence

**Research date:** 2026-06-12
**Valid until:** ~2026-07-12 for codebase/stack claims; the upstream endpoint shape should be re-smoke-tested if execution slips more than a couple of weeks (it's one curl)

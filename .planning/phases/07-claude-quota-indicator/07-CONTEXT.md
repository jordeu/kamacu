# Phase 7: Claude Quota Indicator - Context

**Gathered:** 2026-06-12
**Status:** Ready for planning

<domain>
## Phase Boundary

A compact Claude quota trigger in the top-right of both page headers (board and task view) with an all-windows hover popup, backed by a new DB-free server proxy of the OAuth usage endpoint (`GET /api/usage`). Strictly best-effort: degrade-don't-break. Covers QUOTA-01..08. Touches zero session/PTY code — fully independent of Phases 8–9.

</domain>

<decisions>
## Implementation Decisions

### Compact trigger look
- **D-68:** Trigger is SlayZone-exact: "Claude 5h" text + thin colored bar underneath (per the reference screenshot). No percentage number on the trigger; percentages live in the popup.
- **D-69:** Bar colors are **neutral-until-it-matters**: muted zinc below 60%, yellow 60–84%, red ≥85%. Green NEVER appears — preserves the project's reserved-color-budget / quiet-by-default palette (status dots own green/amber/gray for agent state). This intentionally diverges from SlayZone's full traffic-light bars. Popup row bars follow the same scheme.
- **D-70:** Red-zone emphasis (QUOTA-07) is the red bar alone — no red label text, no motion, no badge. With D-69, red only exists ≥85%, so the red bar IS the "stop starting agents" signal.

### No-data states
- **D-71:** No OAuth credentials at all (file absent / API-key-only user): hide the trigger entirely. Server reports unavailable; frontend renders nothing in the header.
- **D-72:** Credentials exist but token expired/rejected (401) with no cached data: trigger stays visible as a warning chip; popup explains the token expired and says to run `claude` to re-authenticate. Quota silently vanishing after ~8h token expiry would be confusing — this state must be discoverable.
- **D-73:** Stale-data display (fetch failing, cache not yet dropped): subtle, popup-only. Trigger keeps the last-known bar unchanged; popup footer shows "error · Xm old" in amber text. After 3 consecutive failures the cached windows drop (per QUOTA-06) — then the 401 case renders D-72's warning chip and other errors render an equivalent warning-chip state.

### Popup content details
- **D-74:** Row labels are full short names: "5h", "7d", "Opus", "Sonnet" — not SlayZone's truncated "Son.". Unknown/future window keys render their raw API key name (passthrough — server-driven window set per QUOTA-02).
- **D-75:** Row order: API order with 5h first, then 7d, then per-model windows as returned. No utilization-based sorting (rows must not jump between polls).
- **D-76:** Footer is SlayZone-exact: "Updated Xm ago" left, refresh icon button right. No persistent last-error line; errors surface via D-72/D-73 states.

### Claude's Discretion
- Exact trigger dimensions/spacing (reference: SlayZone's ~48×6px bar), hover affordance, popup width.
- Per-row reset copy ("Resets in Xh Ym" countdown — format details, "now" case when resets_at is past/null).
- Warning-chip visual for D-72 (and non-401 total-failure states) — small, motion-free, consistent with existing muted styling.
- Loading state before first fetch resolves (likely render nothing or a skeleton-free quiet placeholder).

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Milestone research (empirically verified 2026-06-12)
- `.planning/research/STACK.md` — verified endpoint (`GET https://api.anthropic.com/api/oauth/usage`), auth (bearer from `~/.claude/.credentials.json` → `claudeAiOauth.accessToken`), load-bearing `claude-code/<version>` User-Agent, captured live response shape (`five_hour`/`seven_day`/`seven_day_opus`/`seven_day_sonnet`, each `{utilization, resets_at}`, nullable, lenient-decode), SlayZone's cache/backoff policy (60s TTL, 10s floor, 429 → max(Retry-After, 30s), serve-stale, 3-failure drop)
- `.planning/research/ARCHITECTURE.md` — integration map: new DB-free `internal/quota` package + `GET /api/usage`; no shared app header exists (each page builds its own header row); single shared `QuotaIndicator` component mounted in BoardPage + TaskPage header right groups; TanStack dedupe via the `useAgentStatuses` pattern; shadcn `hover-card` must be added (`ui/` has none)
- `.planning/research/PITFALLS.md` — never refresh/write the OAuth token (rotation can log the CLI out); `expiresAt` is epoch milliseconds; credentials file contains third-party `mcpOAuth` secrets — never log/expose the file; token must never appear in logs or API payloads (grep audit for `sk-ant-oat`); poll jitter; single-flight manual refresh
- `.planning/research/FEATURES.md` — SlayZone UX specifics from source (thresholds, countdown format, error catalog, staleness policy); anti-features (no TUI scraping, no JSONL estimation, no token auto-refresh)

### Roadmap
- `.planning/ROADMAP.md` §Phase 7 — goal, 5 success criteria, phase notes (six-state degradation matrix in verification; grep audit for token leaks)

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `web/src/components/ui/` — tooltip, skeleton, button, separator exist; **no hover-card/popover** — add via `npx shadcn@latest add hover-card` (Radix HoverCard keeps content open while hovered so the refresh button is clickable)
- `web/src/api/agents.ts` `useAgentStatuses` — the multi-consumer deduped polling pattern to copy for `useQuota()`
- `web/src/api/client.ts` — `get`/`put` helpers (v1.1) for the new usage fetch
- `internal/api/resume.go` — precedent for the server reading `~/.claude` (transcript glob); home-dir access pattern
- `internal/settings/home.go` `ExpandHome` — path expansion helper

### Established Patterns
- DB-free leaf packages under `internal/` (settings, diff) — `internal/quota` follows
- Read-at-use in handlers; managers/packages stay decoupled
- Verbatim copy contracts: user-facing strings kept as single grep-able template literals
- Reserved color budget: color means something — quota bars stay neutral below 60% (D-69)

### Integration Points
- `web/src/pages/BoardPage.tsx` header right group (beside New task) — `<QuotaIndicator/>` mount 1
- `web/src/pages/TaskPage.tsx` header right group (beside the three-dots menu, post-UI-01 full-width row) — mount 2
- `internal/api/routes.go` — register `GET /api/usage`
- No shared AppLayout header exists — do NOT invent one; mount per-page (architecture research recommendation)

</code_context>

<specifics>
## Specific Ideas

- The reference is the user's SlayZone screenshot: "Claude 5h" + thin bar trigger top-right; popup with one row per window (label, bar, %, "Resets in" column), footer "Updated 0m ago" + refresh icon. Layout SlayZone-exact; colors deliberately quieter (D-69) and labels fuller (D-74).

</specifics>

<deferred>
## Deferred Ideas

- Extra-usage credits block in the popup (QUOTA-FUT-01) — endpoint exposes it; deferred to v2+
- Pinnable inline bars / choosing which windows show compact (QUOTA-FUT-02)
- Quota threshold notifications (QUOTA-FUT-03, blocked on NOTF-01)

</deferred>

---

*Phase: 07-claude-quota-indicator*
*Context gathered: 2026-06-12*

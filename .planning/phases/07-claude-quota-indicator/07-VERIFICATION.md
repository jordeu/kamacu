---
phase: 07-claude-quota-indicator
verified: 2026-06-12T06:05:00Z
status: passed
score: 5/5 must-haves verified
---

# Phase 7: Claude Quota Indicator Verification Report

**Phase Goal:** User can see Claude quota usage at a glance — before starting agents — from anywhere in the app, with a trustworthy degrade-don't-break indicator
**Verified:** 2026-06-12T06:05:00Z
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| #   | Truth | Status | Evidence |
| --- | ----- | ------ | -------- |
| 1   | Compact "Claude 5h" indicator with threshold-colored bar top-right on board and task view (QUOTA-01) | ✓ VERIFIED | `QuotaIndicator.tsx:207-226` (trigger: "Claude 5h" text + `h-1.5 w-12` bar, `barColor` thresholds zinc <60 / amber 60–84 / red ≥85 per approved D-69 override of "green <60"); mounted `BoardPage.tsx:50` (header right group beside New task) and `TaskPage.tsx:426` (before `<DropdownMenu>` at :428). Human-approved visually across four passes (07-03 checkpoint). |
| 2   | Hover popup: one row per server-returned window (never hardcoded) with bar, rounded %, reset countdown; footer "Updated Xm ago" + cache-bypassing refresh (QUOTA-02, QUOTA-03) | ✓ VERIFIED | `QuotaPopup` maps `data.windows` as-is in server order (`QuotaIndicator.tsx:132-134`); only hardcoded key is the `five_hour` width lookup (:203). `Math.round(w.utilization)%` (:109); `ResetCell` renders compact bare-duration fixed-width column with full "Resets in Xh Ym" wording in the title attribute — the approved 07-03 decision superseding the literal roadmap text; 10s popup-scoped ticking clock via `useNow(10_000)` (:125). Footer `Updated {formatAgo}` + `RefreshCw` button → `useRefreshQuota()` → `GET /api/usage?refresh=1` (`usage.ts:31`), disabled while pending. |
| 3   | 60s visible-only poll; server enforces 60s TTL, 10s floor, in-flight dedup, 429 Retry-After backoff (min 30s), `claude-code/<version>` UA; never writes/refreshes the token (QUOTA-04, QUOTA-05) | ✓ VERIFIED | `usage.ts:23-24` `refetchInterval: 60_000` + `refetchIntervalInBackground: false`. `quota.go:211-216` `cacheTTL=60s`, `attemptFloor=10s`, `minBackoff429=30s`; gate logic :259-266 binds TTL to `fetchedAt` (successes) and floor to `lastAttempt` (attempts), `inflight` dedup; `parseRetryAfter` floors at 30s (:346-352). UA `"claude-code/" + DetectVersion(...)` (:196) with pinned fallback 2.1.174; `anthropic-beta: oauth-2025-04-20` header (:315). Zero write APIs against creds path (grep `WriteFile|os.Create|OpenFile` → empty); zero `RefreshToken|refresh_token` matches. 17 unit tests green. |
| 4   | Failure states degrade gracefully: clear popup messages + trigger warning; stale data marked "error · Xm old", dropped after 3 consecutive failures; API-key-only users see hidden indicator — never fabricated 0% bars (QUOTA-06) | ✓ VERIFIED | Six-state matrix proven in `quota_test.go` (17 tests: file absent, key absent, locally expired, 429, 5xx, malformed, 401, token change, TTL/floor, recovery). `recordFailureLocked` drops cache at `maxFailures=3` (`quota.go:357-364`). Frontend five-state mapping: `return null` for loading/no_credentials (:170), TriangleAlert amber chip + "Token expired — re-authenticate with \`claude\`" / "Quota unavailable" for empty auth_expired/error (:174-198), stale footer "error · Xm old" in amber (:137-141). `/api/usage` always returns 200 (`usage.go:11-15`). |
| 5   | Any window ≥85% → red-zone trigger emphasis; server cache keyed by token so account switches never leak data (QUOTA-07, QUOTA-08) | ✓ VERIFIED | Bar COLOR = `barColor(Math.max(...windows.map(w => w.utilization)))` across ALL windows while WIDTH tracks 5h (`QuotaIndicator.tsx:203-217`) — human verified live with divergent data (7d=79% driving color, 5h driving width). Cache keyed by `sha256Hex(token)`; mismatch drops cached windows, resets failures (`quota.go:237-244`), covered by token-change test. |

**Score:** 5/5 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| `internal/quota/quota.go` | Service with token-keyed cache, six-state degradation, UA probe (min 150 lines) | ✓ VERIFIED | 419 lines; all gsd-tools checks passed; substantive implementation matching plan spec |
| `internal/quota/quota_test.go` | Six-state matrix tests (min 120 lines) | ✓ VERIFIED | 17 `func Test` (≥8 required); httptest + t.TempDir; generated fake tokens via `strings.Repeat` |
| `internal/api/usage.go` | `GET /api/usage` route, always-200 | ✓ VERIFIED | Route registered; degradation carried in body state; no token material |
| `internal/api/usage_test.go` | Route tests incl. leak guard | ✓ VERIFIED | 3 tests; asserts body never contains `sk-ant-oat`; `?refresh=1` → force plumbing proven |
| `web/src/api/usage.ts` | `useQuota` + `useRefreshQuota` | ✓ VERIFIED | Both exported; contract types match backend JSON tags exactly |
| `web/src/components/quota/QuotaIndicator.tsx` | Trigger + popup, five-state mapping (min 120 lines) | ✓ VERIFIED | 227 lines; all states, thresholds, format helpers present; motion-free (no animate/pulse); zero "green" matches |
| `web/src/components/ui/hover-card.tsx` | shadcn copy-in, zero new deps | ✓ VERIFIED | `from "radix-ui"` import; HoverCardContent exported; no package.json dependency change |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | --- | --- | ------ | ------- |
| `cmd/kangent/main.go` | `internal/api/usage.go` | `api.UsageRoutes(mux, quotaSvc)` | ✓ WIRED | main.go:111-112 (gsd-tools "failure" was a regex-escaping artifact; manually confirmed) |
| `internal/quota/quota.go` | `~/.claude/.credentials.json` | `os.ReadFile` per Get() call | ✓ WIRED | quota.go:65; re-read every poll inside Get() (:225), never cached, never written |
| `internal/quota/quota.go` | `api.anthropic.com/api/oauth/usage` | Bearer + anthropic-beta + claude-code UA | ✓ WIRED | quota.go:310-317 |
| `web/src/api/usage.ts` | `/api/usage` | queryFn + `?refresh=1` mutation | ✓ WIRED | usage.ts:22, :31 |
| `QuotaIndicator.tsx` | `web/src/api/usage.ts` | `useQuota()` / `useRefreshQuota()` | ✓ WIRED | :166, :126 |
| `BoardPage.tsx` / `TaskPage.tsx` | `QuotaIndicator.tsx` | `<QuotaIndicator />` in headers | ✓ WIRED | BoardPage.tsx:50; TaskPage.tsx:426 (before DropdownMenu :428) |
| browser | `GET /api/usage` | useQuota 60s poll through the binary | ✓ WIRED | Proven by live smoke (below) |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| -------- | ------------- | ------ | ------------------ | ------ |
| QuotaIndicator.tsx | `data` (useQuota) | GET /api/usage → quota.Service → Anthropic OAuth usage endpoint | Yes — live smoke returned real windows (5h=71%, 7d=80%, Sonnet=0%) | ✓ FLOWING |
| QuotaPopup footer | `data.fetchedAt` | Service `fetchedAt` (last successful fetch) | Yes — real ISO timestamp in payload | ✓ FLOWING |

No hardcoded-empty props at either mount site; `<QuotaIndicator />` takes no props by design.

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| Backend test matrix green | `go vet ./... && go test ./internal/quota ./internal/api -count=1` | ok (quota 0.02s, api 83s) | ✓ PASS |
| Frontend compiles + builds | `npx tsc -b --noEmit && npm run build` | exit 0 (chunk-size warning only, pre-existing) | ✓ PASS |
| Live `GET /api/usage` returns real data | built binary on :7419, curl | HTTP 200, `state:"ok"`, 3 server-ordered windows with real utilization | ✓ PASS |
| No token in payload or logs | `grep -c "sk-ant-oat"` on payload + server log | 0 and 0 | ✓ PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| ----------- | ----------- | ----------- | ------ | -------- |
| QUOTA-01 | 07-02, 07-03 | Compact indicator, threshold bar, both headers | ✓ SATISFIED | Truth 1; human-approved placement |
| QUOTA-02 | 07-02 | Server-driven popup rows, bar/%/countdown | ✓ SATISFIED | Truth 2 |
| QUOTA-03 | 07-02 | "Updated Xm ago" footer + cache-bypass refresh | ✓ SATISFIED | Truth 2 |
| QUOTA-04 | 07-02 | 60s visible-only auto-refresh | ✓ SATISFIED | Truth 3 (frontend half) |
| QUOTA-05 | 07-01 | UA, TTL, floor, dedup, backoff; never writes token | ✓ SATISFIED | Truth 3 (server half) + audits |
| QUOTA-06 | 07-01, 07-03 | Graceful degradation, stale-then-drop, no fabricated bars | ✓ SATISFIED | Truth 4; six-state test matrix |
| QUOTA-07 | 07-02, 07-03 | ≥85% red-zone glance signal | ✓ SATISFIED | Truth 5; max-across-windows color |
| QUOTA-08 | 07-01 | Token-keyed cache, no cross-account leakage | ✓ SATISFIED | Truth 5; sha256 fingerprint + test |

No orphaned requirements — all 8 IDs mapped to Phase 7 in REQUIREMENTS.md are claimed by plans 07-01/02/03.

### Anti-Patterns Found

None. Zero TODO/FIXME/placeholder matches in phase files; zero `green` classes (D-69); zero animation classes (D-70); zero token material outside generated test fakes; zero credential-write or token-refresh APIs.

### Human Verification Required

None outstanding. The blocking human-verify checkpoint in plan 07-03 was completed and APPROVED (four inspection passes; fix commits e5cde8c, 6673b53, 6b4c67f verified present in git log) covering trigger placement, popup rows, refresh behavior, red-zone color rule, and the ticking footer.

### Approved Deviations from Literal Roadmap Wording

- **D-69 (STATE.md):** muted zinc replaces "green <60" — green never appears in quota bars.
- **07-03 checkpoint decision:** "Resets in Xh Ym" text replaced by a compact bare-duration fixed-width column (full wording preserved in the title attribute), plus a 10s popup-scoped ticking clock for footer age and countdowns.

### Gaps Summary

No gaps. All five success criteria observable on the running app; the degrade-don't-break contract is proven by a 17-test six-state matrix, the always-200 route, and a clean source + runtime token-leak audit. The phase goal is achieved.

---

_Verified: 2026-06-12T06:05:00Z_
_Verifier: Claude (gsd-verifier)_

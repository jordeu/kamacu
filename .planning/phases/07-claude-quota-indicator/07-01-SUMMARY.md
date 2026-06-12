---
phase: 07-claude-quota-indicator
plan: 01
subsystem: api
tags: [go, quota, oauth, anthropic, http-cache, tdd]

# Dependency graph
requires:
  - phase: 06-settings
    provides: XxxRoutes(mux, dep) registration convention, injectable-seam test precedent (transcriptExists)
provides:
  - internal/quota package — demand-driven, token-keyed, backoff-protected proxy of Anthropic's OAuth usage endpoint
  - GET /api/usage route (always 200; {state, stale, fetchedAt, windows} contract for the frontend plan)
  - quota.Result/quota.Window JSON contract (camelCase: state/stale/fetchedAt/windows/key/label/utilization/resetsAt)
affects: [07-02 frontend quota indicator, 07-03 phase verification]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Demand-driven server cache: browser poll is the only trigger; idle server makes zero upstream requests (no ticker goroutine)"
    - "Injectable Config seam (CredentialsPath/BaseURL/UserAgent/Now) instead of interfaces for unit-testable external IO"
    - "Read-only passenger on ~/.claude/.credentials.json: re-read per poll, decode 2 fields only, never write/log/expose"

key-files:
  created:
    - internal/quota/quota.go
    - internal/quota/quota_test.go
    - internal/api/usage.go
    - internal/api/usage_test.go
  modified:
    - cmd/kangent/main.go

key-decisions:
  - "quota.Config gained an optional Now func() time.Time field (research-sanctioned seam) so the api-package handler tests can control TTL/floor without sleeps"
  - "10s attempt floor binds ALL upstream attempts, not just ?refresh=1 — honors the 'never more than once per 10s' truth even when the cache has been dropped during an outage"
  - "tokenFP is set on every Get (not only on success) so consecutive failures under one token accumulate correctly toward the 3-failure drop"

patterns-established:
  - "Six-state degradation enum: ok | no_credentials | auth_expired | error, with stale serving until 3 consecutive failures then drop"
  - "Status-code-only error discipline for upstream HTTP: never log/wrap tokens, requests, or response bodies"

requirements-completed: [QUOTA-05, QUOTA-06, QUOTA-08]

# Metrics
duration: 14min
completed: 2026-06-12
---

# Phase 7 Plan 01: Quota Server Proxy Summary

**Demand-driven token-keyed quota cache (`internal/quota`) proxying Anthropic's OAuth usage endpoint at always-200 `GET /api/usage`, with the full six-state degradation matrix proven by 20 unit tests and zero new dependencies**

## Performance

- **Duration:** 14 min
- **Started:** 2026-06-12T04:26:18Z
- **Completed:** 2026-06-12T04:40:31Z
- **Tasks:** 3 (all TDD: RED + GREEN commits each)
- **Files modified:** 5

## Accomplishments

- `internal/quota` package: per-poll credentials re-read (2 fields only), lenient map-decode normalization with fixed window order + raw-key passthrough (D-74/D-75), sha256 token-fingerprint cache keying (QUOTA-08)
- Full six-state matrix unit-tested with httptest + t.TempDir + injectable clock — no real credentials, no sleeps: file absent / key absent / locally expired / 429 backoff / 5xx stale-then-drop / malformed body, plus 401, token-change, TTL/floor separation, and auth-expired recovery
- `GET /api/usage` registered and wired in main.go with the `-claude-bin` flag; route inherits loopback enforcement like every other route
- UA probe (`claude --version`, 5s timeout, pinned `2.1.174` fallback) selects the lenient rate-limit bucket (QUOTA-05); locally-expired tokens short-circuit without burning upstream calls
- Resolves the STATE.md idle-polling tech-debt note by design: no connected browser ⇒ zero upstream requests

## Task Commits

Each task was committed atomically (TDD: test RED then feat GREEN):

1. **Task 1: types, credentials read, normalization** — `9e3abb0` (test) + `ae632cf` (feat)
2. **Task 2: quota.Service six-state cache** — `2eed7cc` (test) + `3536ce7` (feat)
3. **Task 3: GET /api/usage route + main.go wiring** — `02d7e88` (test) + `a2d1c91` (feat)

## Files Created/Modified

- `internal/quota/quota.go` — Window/Result contract types, readCredentials (UnixMilli expiry), normalizeWindows (extra_usage skip), Service (TTL/floor/backoff/dedup/token-keying), DetectVersion UA probe
- `internal/quota/quota_test.go` — 17 test functions: credentials/normalization unit tests + the six-state Service matrix
- `internal/api/usage.go` — UsageRoutes registering the always-200 `GET /api/usage` proxy
- `internal/api/usage_test.go` — handler tests: always-200 contract, refresh-param plumbing, token-leak guard
- `cmd/kangent/main.go` — quota.New(Config{ClaudeBin: \*claudeBin}) + api.UsageRoutes registration after AgentRoutes

## Decisions Made

- **`Now` seam in quota.Config:** the plan-mandated handler test ("?refresh=1 past the 10s floor triggers a second upstream hit") cannot control time from package `api` against the 4-field Config. Research (ARCHITECTURE seam note) explicitly recommended `now func() time.Time` as part of the injectable config, so it was added as an optional fifth field defaulting to `time.Now`.
- **Floor binds all attempts:** plan step 5 literally gated the 10s floor on `force` only, but the plan's own must-have truth says "never more than once per 10s even with ?refresh=1" and Pitfall 3 warns about a failing upstream being hammered by the 60s poll once the cache drops (TTL no longer gates when `cached == nil`). The floor check therefore applies to every attempt.
- **tokenFP set on every Get:** if the fingerprint were only recorded on success, every pre-success failure would look like a token change and reset the failure counter, so the 3-failure drop could never trigger from a cold start.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Added optional `Now func() time.Time` to quota.Config**
- **Found during:** Task 3 (GET /api/usage handler tests)
- **Issue:** The required refresh-param test needs to advance past the 10s floor; `Service.now` is package-private and unreachable from `internal/api` tests, and real sleeps are forbidden by the no-sleep test design
- **Fix:** Added `Now` as an optional Config field (nil → `time.Now`), exactly the seam the research doc recommended
- **Files modified:** internal/quota/quota.go
- **Verification:** `go test ./internal/api -run Usage` passes; quota package tests unaffected
- **Committed in:** a2d1c91 (Task 3 commit)

**2. [Rule 2 - Missing Critical] 10s attempt floor applies to non-force polls too**
- **Found during:** Task 2 (Service implementation)
- **Issue:** Plan step 5's literal condition `(force && now.Sub(lastAttempt) < 10s)` leaves dropped-cache non-force polls ungated below 10s (multiple tabs → upstream hammering during an outage), contradicting the must-have truth "never more than once per 10s even with ?refresh=1"
- **Fix:** Floor condition checks `now.Sub(lastAttempt) < 10s` for every attempt; all planned behaviors (TTL serve, force bypass past floor, force block within floor) still hold
- **Files modified:** internal/quota/quota.go
- **Verification:** TestServiceTTLAndFloor passes; six-state matrix unaffected
- **Committed in:** 3536ce7 (Task 2 commit)

---

**Total deviations:** 2 auto-fixed (1 blocking, 1 missing critical)
**Impact on plan:** Both strengthen the plan's own stated invariants; no scope creep, no contract changes visible to the frontend plan.

## Issues Encountered

None — whole-repo `go vet` + `go test ./... -count=1` green; token-leak grep audits (`sk-ant-oat` outside tests, `RefreshToken|refresh_token` in internal/quota) both empty.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- The `GET /api/usage` JSON contract ({state, stale, fetchedAt, windows[{key,label,utilization,resetsAt}]}) is live for plan 07-02's frontend indicator
- Plan 07-03 (verification) can exercise the route against real credentials; degradation states are all reachable via the documented conditions

---
*Phase: 07-claude-quota-indicator*
*Completed: 2026-06-12*

## Self-Check: PASSED

- All 6 key files exist on disk
- All 6 task commits present in git history (9e3abb0, ae632cf, 2eed7cc, 3536ce7, 02d7e88, a2d1c91)

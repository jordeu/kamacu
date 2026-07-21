---
phase: 06
slug: mcp-subcommand-foundation
status: verified
# threats_open = count of OPEN threats at or above workflow.security_block_on severity (the blocking gate)
threats_open: 0
asvs_level: 1
created: 2026-07-21
---

# Phase 06 — Security

> Per-phase security contract: threat register, accepted risks, and audit trail.

---

## Trust Boundaries

| Boundary | Description | Data Crossing |
|----------|-------------|---------------|
| user shell → binary | User invokes `kamacu serve [flags]` (or bare `kamacu`); argv crosses into the process. UNCHANGED by Phase 06 — only the argv shape that starts the server changes. | argv flags (low sensitivity) |
| agent CLI → MCP subcommand stdin | The agent CLI (claude / opencode) spawns `kamacu mcp serve` and writes JSON-RPC requests to its stdin. TRUSTED channel (agent spawned by Kamacu inside a task PTY); malformed input possible on agent bug. | JSON-RPC requests (medium sensitivity) |
| MCP subcommand stdout → agent CLI | JSON-RPC responses. Non-JSON bytes corrupt the stream (the SC2 invariant). | JSON-RPC responses (medium sensitivity) |
| MCP subcommand → Kamacu HTTP API (loopback) | Plain HTTP requests to `127.0.0.1:7333` (or `KAMACU_HOOK_BASE`). Same boundary as SPA fetches — loopback binding is the actual auth boundary (D-06). | HTTP requests with `X-Kamacu-Token` header (low — header decorative on general routes) |
| browser → Kamacu HTTP API | SPA fetches + malicious-webpage no-CORS POSTs at localhost. UNCHANGED — hook receiver on `/api/hooks/sessions/{id}` is where Kamacu reads the per-instance token; the rename preserves the gate byte-for-byte. | HTTP requests with `X-Kamacu-Token` header (medium sensitivity) |
| claude / opencode task → Kamacu hook receiver | Spawned agent hooks POST to `/api/hooks/sessions/{id}` with the per-instance token in the header. UNCHANGED auth posture; only the header NAME changes. | HTTP requests with `X-Kamacu-Token` header (medium sensitivity) |

---

## Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation | Status |
|-----------|----------|-----------|----------|-------------|------------|--------|
| T-06-01 | Tampering | cmd/kamacu/main.go dispatcher | low | accept | Flag set byte-for-byte identical to today's `flag.X` calls — `serve` only names the existing path. Verified: `./bin/kamacu serve --help` shows same 5 flags with same defaults. | closed |
| T-06-02 | Elevation of privilege | scripts/Makefile/README break-clean | low | accept | D-04 intentionally breaks bare `kamacu` invocation — external scripts fail loudly with non-zero exit + help. Stricter contract, not regression. | closed |
| T-06-03 | Spoofing | `X-Kamacu-Token` header on bridge requests | low | accept | D-06: Kamacu ignores header on `/api/*` routes; loopback binding is v1.11 auth boundary. Token decorative on general routes; sent for forward compatibility. | closed |
| T-06-04 | Tampering / DoS | stdout pollution desyncing JSON-RPC stream | high | mitigate | D-12: stdlib defaults (slog → stderr; `mcp.NewServer(..., nil)` → SDK discards logs; SDK's `OnInternalError` → stderr). L1 grep verified: zero `fmt.Print*` / `log.Print*` / `os.Stdout.Write` in `internal/mcp/`. SC2 regression test (`TestSC2_HandlerReturningError_*`, two sub-tests) covers clean-error-response invariant. | closed |
| T-06-05 | Information Disclosure | `KAMACU_HOOK_BASE` env pointing to non-loopback URL | medium | accept | D-06: loopback binding enforced by SERVER (`ensureLoopback` + `hostCheck`), not client. Non-Kamacu server at redirected URL has no Kamacu data shape. Acceptable for single-user local app. | closed |
| T-06-06 | Denial of Service | bridged HTTP timeout | low | accept | 10-second `http.Client.Timeout`, no retry. Timeout surfaces Kamacu-down to agent — correct behavior. | closed |
| T-06-07 | Tampering | process crash on tool-handler panic | medium | accept | RESEARCH Pitfall 1: SDK does NOT recover panics. Phase 06 ships NO panic-recovery (Open Question 1 deferred to Phase 08). `list_projects` handler has minimal panic surface — `req.Params.Arguments` not unmarshaled, no map writes / slice indexing on user input. | closed |
| T-06-08 | Elevation of Privilege | bridge response size unbounded | low | mitigate | `io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))` where `maxBodyBytes = 1 << 20` (1 MiB). L1 grep verified in `internal/mcp/bridge.go:23,86`. Bump cap in Phase 07+ if a tool returns more. | closed |
| T-06-09 | Spoofing | malicious webpage no-CORS POST at localhost:7333 with guessed hook token | medium | mitigate | UNCHANGED posture: 32-byte `crypto/rand` token, `subtle.ConstantTimeCompare` on receiver (`internal/api/hooks.go:40`, byte-for-byte preserved), fresh-per-start, re-injected into spawned agents every boot. `TestHookTokenGate` proves missing/wrong tokens return 401. | closed |
| T-06-10 | Tampering | on-disk opencode plugin carries stale `X-Kangent-Token` literal until next `kamacu serve` boot | low | accept | RESEARCH Pitfall 6: rename lands in `//go:embed` source AND receiver. After next `kamacu serve`, `opencode.InstallPlugin` regenerates on-disk file via skip-on-byte-identical. D-07 (same-release + fresh-per-start token) means any mismatch is one-time during rename. | closed |
| T-06-11 | Information Disclosure | header literal rename visible in network traffic | low | accept | Header NAME was never a secret — only token VALUE is. Rename reveals nothing an attacker didn't already know. Token value remains 32-byte random and is never logged. | closed |
| T-06-12 | Tampering | subtle rename typo (e.g. `X-Kamacu-token` lowercase, `X_Kamacu_Token` underscore) desyncs senders from receiver | medium | mitigate | Test suite is the safety net: `TestHookTokenGate` exercises receiver; `TestAgent*` exercises claude sender overlay template; `TestPluginSource*` exercises opencode plugin source. L1 grep verified: all 4 test files (`hooks_test.go`, `agent_test.go`, `plugin_test.go`, `e2e_test.go`) carry the renamed literal at expected positions. | closed |
| T-06-SC-01 | Tampering | go.mod adds `github.com/google/subcommands v1.2.0` | low | accept | Package Legitimacy Audit (06-RESEARCH.md § "Package Legitimacy Audit") verified OK — Google-maintained, 7+ years stable, zero transitive deps. Direct source verification at v1.6.1 tag. No `[SUS]` / `[SLOP]`. | closed |
| T-06-SC-02 | Tampering | go.mod adds `github.com/modelcontextprotocol/go-sdk v1.6.1` + transitive deps | medium | accept | Package Legitimacy Audit verified both SDK and `google/subcommands` as OK. All transitive deps (`google/jsonschema-go`, `segmentio/encoding`, `yosida95/uritemplate/v3`, `golang-jwt/jwt/v5`, `golang.org/x/oauth2`, `golang.org/x/tools`) pre-evaluated as Low risk. No `[SUS]` / `[SLOP]`. | closed |

*Status: open · closed · open — below high threshold (non-blocking)*
*Severity: critical > high > medium > low — only open threats at or above workflow.security_block_on (high) count toward threats_open*
*Disposition: mitigate (implementation required) · accept (documented risk) · transfer (third-party)*

---

## Accepted Risks Log

| Risk ID | Threat Ref | Rationale | Accepted By | Date |
|---------|------------|-----------|-------------|------|
| AR-06-01 | T-06-01 | Flag set byte-for-byte identical to today's surface — no new parsing risk introduced by the dispatcher refactor. | gsd-secure-phase (L1) | 2026-07-21 |
| AR-06-02 | T-06-02 | D-04 break-clean is the intended single-user / fresh-per-start posture. Stricter contract is a security improvement, not regression. | gsd-secure-phase (L1) | 2026-07-21 |
| AR-06-03 | T-06-03 | Server-side token middleware on `/api/*` is explicitly out of v1.11 scope (D-06). Loopback binding is the actual auth boundary. | gsd-secure-phase (L1) | 2026-07-21 |
| AR-06-04 | T-06-05 | Server enforces loopback. Non-Kamacu server at redirected URL has no Kamacu data shape. Acceptable for single-user local app. | gsd-secure-phase (L1) | 2026-07-21 |
| AR-06-05 | T-06-06 | 10s timeout, no retry. Surfacing Kamacu-down to the agent is correct behavior. | gsd-secure-phase (L1) | 2026-07-21 |
| AR-06-06 | T-06-07 | SDK panic-recovery deferred to Phase 08 when `subscribe_session_output` introduces more panic surface area. `list_projects` has minimal panic surface. | gsd-secure-phase (L1) | 2026-07-21 |
| AR-06-07 | T-06-10 | D-07 (server+agents always same-release, fresh-per-start token) bounds any mismatch to one-time during rename. | gsd-secure-phase (L1) | 2026-07-21 |
| AR-06-08 | T-06-11 | Header name was never secret; only token value is. Token value remains 32-byte random, never logged. | gsd-secure-phase (L1) | 2026-07-21 |
| AR-06-09 | T-06-SC-01 | Google-maintained, 7+ years stable, zero transitive deps. Package Legitimacy Audit clean. | gsd-secure-phase (L1) | 2026-07-21 |
| AR-06-10 | T-06-SC-02 | All SDK + transitive deps pre-evaluated as Low risk in 06-RESEARCH.md. Package Legitimacy Audit clean. | gsd-secure-phase (L1) | 2026-07-21 |

*Accepted risks do not resurface in future audit runs.*

---

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open | Run By |
|------------|---------------|--------|------|--------|
| 2026-07-21 | 13 | 13 | 0 | gsd-secure-phase (L1 grep-depth, ASVS=1) |

---

## Sign-Off

- [x] All threats have a disposition (mitigate / accept / transfer)
- [x] Accepted risks documented in Accepted Risks Log
- [x] `threats_open: 0` confirmed
- [x] `status: verified` set in frontmatter

**Approval:** verified 2026-07-21 (L1 grep-depth, ASVS=1, register authored at plan time, short-circuit rule applied)

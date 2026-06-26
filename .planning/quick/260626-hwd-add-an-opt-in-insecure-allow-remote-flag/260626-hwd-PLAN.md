---
phase: quick-260626-hwd
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - cmd/kangent/main.go
  - cmd/kangent/main_test.go
  - internal/ws/handler.go
  - internal/ws/integration_test.go
  - internal/ws/handler_test.go
  - internal/api/agent_integration_test.go
autonomous: true
requirements: [INSECURE-REMOTE-01]
must_haves:
  truths:
    - "Default path is byte-for-byte unchanged: WITHOUT --insecure-allow-remote, ensureLoopback still rejects a non-loopback --addr (os.Exit(1)), hostCheck still 403s non-loopback Host headers, and the WS Origin allowlist stays loopback-only."
    - "WITH --insecure-allow-remote, a non-loopback --addr (e.g. 0.0.0.0:7333, 192.168.1.5:7333) is permitted: ensureLoopback is bypassed and the process starts."
    - "WITH --insecure-allow-remote, a non-loopback Host header is served (hostCheck is a pass-through, no 403)."
    - "WITH --insecure-allow-remote, a cross-origin WebSocket upgrade is accepted (Origin verification skipped via coder/websocket InsecureSkipVerify)."
    - "WITH --insecure-allow-remote, a LOUD slog.Warn security banner prints at startup naming the exposure (no auth, anyone reaching the address gets a shell)."
    - "The agent-status hook BaseURL is loopback-correct: a wildcard --addr host (0.0.0.0 / :: / empty) is normalized to 127.0.0.1:<port> for the hook URL, while a specific non-loopback IP --addr is left as-is."
  artifacts:
    - path: "cmd/kangent/main.go"
      provides: "--insecure-allow-remote flag, conditional ensureLoopback/hostCheck, warn banner, hook BaseURL wildcard normalization"
      contains: "insecure-allow-remote"
    - path: "internal/ws/handler.go"
      provides: "NewHandler accepts an insecureAnyOrigin flag; sets InsecureSkipVerify on websocket.AcceptOptions when true"
      contains: "InsecureSkipVerify"
    - path: "cmd/kangent/main_test.go"
      provides: "Retained default-path assertions (ensureLoopback rejects non-loopback) + new coverage that the flag opens bind + relaxes hostCheck"
      contains: "insecure"
  key_links:
    - from: "cmd/kangent/main.go (--insecure-allow-remote flag)"
      to: "ensureLoopback gate, hostCheck wrap, ws.NewHandler"
      via: "single boolean guarding all three layers + the warn banner"
      pattern: "insecureAllowRemote|insecure-allow-remote"
    - from: "cmd/kangent/main.go (hook BaseURL)"
      to: "session.AgentConfig.BaseURL"
      via: "hookBaseURL normalizing wildcard host to 127.0.0.1:<port>"
      pattern: "BaseURL"
---

<objective>
Add an opt-in `--insecure-allow-remote` boolean flag (default false) that lets kangent bind to a non-loopback address via the existing `--addr` (e.g. `--addr 0.0.0.0:7333`). The flag bypasses all THREE loopback-enforcement layers (bind gate, hostCheck middleware, WS Origin allowlist), prints a loud security warning at startup, and normalizes the agent-status hook BaseURL for wildcard binds.

Purpose: The user has explicitly accepted that kangent has NO authentication and that exposing it = an unauthenticated remote shell. This is a deliberate escape hatch for trusted-network use. SAFE BY DEFAULT is the headline invariant: with the flag absent, behavior is byte-for-byte unchanged.

Output: The flag, the conditional enforcement wiring, the warn banner, the hook BaseURL fix, and extended tests. `go build ./... && go vet ./... && go test ./...` passes. No authentication, no TLS (both explicitly out of scope).
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/STATE.md

@cmd/kangent/main.go
@cmd/kangent/main_test.go
@internal/ws/handler.go
@internal/ws/integration_test.go
@internal/ws/handler_test.go
@internal/api/agent_integration_test.go

<interfaces>
<!-- Verified facts from the codebase. Executor: use these directly. -->

Current ws.NewHandler signature (internal/ws/handler.go):
    func NewHandler(mgr *session.Manager, originPatterns []string) *Handler
Call sites (3): cmd/kangent/main.go:143 (originPatterns), internal/ws/integration_test.go:42 (nil), internal/api/agent_integration_test.go:75 (nil). handler_test.go uses NewHandler(mgr, nil) at line 28.

coder/websocket v1.8.14 Origin semantics (VERIFIED against the module source at
/home/jordi/go/pkg/mod/github.com/coder/websocket@v1.8.14/accept.go):
  - AcceptOptions has BOTH OriginPatterns []string AND InsecureSkipVerify bool.
  - The "*" pattern technically matches (match() uses path.Match, and "*" matches any host:port with no "/"),
    BUT the library's own doc comment (accept.go line 51) explicitly says:
    "Do not use * as a pattern to allow any origin, prefer to use InsecureSkipVerify instead".
  - DECISION (locked): use InsecureSkipVerify: true for the any-origin case — the library-blessed path. Do NOT append "*" to originPatterns.

Existing main.go anchors (line numbers approximate):
  - line 34: addr := flag.String("addr", "127.0.0.1:7333", ...)
  - lines 37-41: flag.Func("dev-origin", ...) populating devOrigins
  - line 45: ensureLoopback(*addr) gate (os.Exit(1) on error)
  - lines 107-108: net.SplitHostPort(*addr) → port; originPatterns built
  - line 125: BaseURL: "http://" + *addr  (agent-status hook receiver)
  - line 143: ws.NewHandler(mgr, originPatterns)
  - line 195: slog.Info("kangent listening", ...)
  - line 196: http.ListenAndServe(*addr, hostCheck(mux))
  - ensureLoopback ~268, hostCheck ~288 (both unexported, table-tested in main_test.go)
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Thread an insecureAnyOrigin flag through ws.NewHandler</name>
  <files>internal/ws/handler.go, internal/ws/integration_test.go, internal/ws/handler_test.go, internal/api/agent_integration_test.go</files>
  <behavior>
    - When NewHandler is called with insecureAnyOrigin=false, ServeHTTP keeps current behavior: websocket.Accept verifies Origin against originPatterns (TestIntegrationEvilOriginRejected still passes — http://evil.example → 403).
    - When NewHandler is called with insecureAnyOrigin=true, websocket.Accept is called with InsecureSkipVerify:true, so a cross-origin upgrade (e.g. Origin http://evil.example) is accepted (reaches 101).
  </behavior>
  <action>
    Add a third parameter `insecureAnyOrigin bool` to `NewHandler` (signature becomes `NewHandler(mgr *session.Manager, originPatterns []string, insecureAnyOrigin bool) *Handler`) and store it on the Handler struct as a new field. In ServeHTTP, set `InsecureSkipVerify: h.insecureAnyOrigin` on the `websocket.AcceptOptions` literal alongside the existing `OriginPatterns: h.originPatterns`. Use InsecureSkipVerify rather than appending "*" to originPatterns — this is the coder/websocket-documented way to allow any origin (accept.go line 51 explicitly warns against the "*" pattern). Update the doc comment on NewHandler to note the new param: when true, Origin verification is disabled entirely (the opt-in --insecure-allow-remote path). Update all THREE non-production call sites to pass `false`: internal/ws/integration_test.go:42, internal/ws/handler_test.go:28, internal/api/agent_integration_test.go:75. Leave cmd/kangent/main.go's call site for Task 2 (it will pass the real flag value). Add ONE new test in internal/ws/integration_test.go modeled on TestIntegrationEvilOriginRejected — e.g. TestIntegrationInsecureAnyOriginAccepted — that builds the server with NewHandler(mgr, nil, true), dials with Origin http://evil.example, and asserts the upgrade SUCCEEDS (err is nil, conn opens); close the conn cleanly. Match the existing test style (newTestServer helper shape, integrationCtx). Note newTestServer currently hard-codes NewHandler(mgr, nil) — either parameterize it or build a small inline server in the new test; keep the existing TestIntegration/TestIntegrationEvilOriginRejected wiring intact (they keep passing false).
  </action>
  <verify>
    <automated>cd /home/jordi/workspace/github/kangent && go build ./... && go test ./internal/ws/ ./internal/api/ -run 'Integration|Handler' -count=1</automated>
  </verify>
  <done>NewHandler takes (mgr, originPatterns, insecureAnyOrigin); all call sites compile; InsecureSkipVerify is wired to the new field; the new accepted-origin test passes AND TestIntegrationEvilOriginRejected still passes.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: Add --insecure-allow-remote flag, conditional enforcement, warn banner, and hook BaseURL fix in main.go + tests</name>
  <files>cmd/kangent/main.go, cmd/kangent/main_test.go</files>
  <behavior>
    - ensureLoopback retains current behavior unchanged (TestEnsureLoopback table passes as-is: non-loopback → error).
    - hostCheck retains current behavior unchanged (TestHostCheck table passes as-is: non-loopback Host → 403).
    - New: hookBaseURL("0.0.0.0:7333") == "http://127.0.0.1:7333"; hookBaseURL("[::]:7333") == "http://127.0.0.1:7333"; hookBaseURL(":7333") == "http://127.0.0.1:7333"; hookBaseURL("192.168.1.5:7333") == "http://192.168.1.5:7333"; hookBaseURL("127.0.0.1:7333") == "http://127.0.0.1:7333".
    - New: when the insecure flag is true, hostCheck is bypassed — a pass-through helper (or skipping the wrap) lets a non-loopback Host header (e.g. 192.168.1.5:7333) reach the inner handler with 200, not 403.
  </behavior>
  <action>
    In main(): register `insecureAllowRemote := flag.Bool("insecure-allow-remote", false, "opt-in: allow binding --addr to a non-loopback address (NO AUTH — anyone who can reach the address gets a shell)")` next to the other flags (around line 41, before flag.Parse()).

    Bind gate (around line 45): only call `ensureLoopback(*addr)` / os.Exit(1) when `!*insecureAllowRemote`. Do NOT change the ensureLoopback function body — it stays exactly as-is so TestEnsureLoopback is unchanged.

    Warn banner: when `*insecureAllowRemote` is true, emit a LOUD `slog.Warn` at startup (place it right after flag.Parse or right before "kangent listening") naming the exposure — e.g. message "SECURITY: kangent is listening with NO authentication via --insecure-allow-remote — anyone who can reach this address gets a shell on this host", with structured fields `"addr", *addr`. Keep it a single unmistakable Warn line.

    Decide the no-non-loopback-addr case simply: if `*insecureAllowRemote` is set but `*addr` is still a loopback address, that is a harmless no-op — do NOT error. (Optional: a one-line slog.Info noting the flag had no effect; keep it simple, no second branch of logic required.)

    hostCheck wrap (line 196): when `*insecureAllowRemote` is true, serve the mux WITHOUT the hostCheck wrapper (e.g. choose the handler: `var handler http.Handler = hostCheck(mux); if *insecureAllowRemote { handler = mux }`, then `http.ListenAndServe(*addr, handler)`). Do NOT change the hostCheck function body — TestHostCheck stays unchanged.

    WS Origin: pass the flag to the handler — change line 143 to `ws.NewHandler(mgr, originPatterns, *insecureAllowRemote)` (Task 1 added the third param).

    Hook BaseURL: add an unexported helper `func hookBaseURL(addr string) string` near ensureLoopback. It returns `"http://" + addr` EXCEPT when the host portion is a wildcard — empty host (":7333"), "0.0.0.0", or "::"/"[::]" — in which case it substitutes loopback host "127.0.0.1" with the original port, returning e.g. "http://127.0.0.1:7333". A SPECIFIC non-loopback IP (e.g. 192.168.1.5) is returned unchanged. Use net.SplitHostPort to parse; on a SplitHostPort error fall back to "http://" + addr (preserve current behavior). Then change line 125 from `BaseURL: "http://" + *addr` to `BaseURL: hookBaseURL(*addr)`. Rationale: Claude Code hooks curl this URL and ALWAYS run on the server host; when bound to a wildcard the server also listens on loopback, so the hook must target 127.0.0.1, never 0.0.0.0.

    Tests in cmd/kangent/main_test.go (extend, do not delete): (a) KEEP TestEnsureLoopback and TestHostCheck exactly as-is — they prove the default path is unchanged. (b) Add TestHookBaseURL: a table covering 0.0.0.0:7333, [::]:7333, :7333, 192.168.1.5:7333, 127.0.0.1:7333, localhost:7333 → expected URLs per the behavior block above. (c) Add a test proving the flag relaxes hostCheck: since hostCheck is the unit under test, assert the SELECTION logic directly — e.g. a small helper or inline replicating main's choice (`handler := hostCheck(mux)` vs `mux` based on the bool) and assert that with the bool true a non-loopback Host (192.168.1.5:7333) reaches the inner handler (200), while with the bool false it is 403. Match the existing httptest table style (handlerRan flag, httptest.NewRecorder). Do NOT add a test that boots main() or binds a real socket.

    NEVER place fenced code in the running app; this is directive prose. Keep changes minimal and within the existing file style (slog, net, http already imported).
  </action>
  <verify>
    <automated>cd /home/jordi/workspace/github/kangent && go build ./... && go vet ./... && go test ./... -count=1</automated>
  </verify>
  <done>`--insecure-allow-remote` flag exists; ensureLoopback + hostCheck are skipped only when the flag is set; warn banner emitted on the insecure path; hookBaseURL normalizes wildcard hosts only; ws handler receives the flag; TestEnsureLoopback + TestHostCheck unchanged and passing; new TestHookBaseURL and hostCheck-relaxation test pass; full `go build && go vet && go test ./...` is green.</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| network → kangent HTTP/WS | By DEFAULT only loopback crosses here (3 layers). WITH the opt-in flag, the user DELIBERATELY widens this boundary to a non-loopback network. |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-hwd-01 | Elevation of Privilege | bind gate / hostCheck / WS Origin relaxed by --insecure-allow-remote | accept | EXPLICIT, user-accepted tradeoff per the task brief: kangent serves an unauthenticated shell; exposing it grants remote shell. Mitigated operationally by (a) opt-in only — default is byte-for-byte loopback-locked, (b) a loud slog.Warn banner at startup naming the exposure. No auth/TLS by design decision. |
| T-hwd-02 | Spoofing | DNS-rebinding / cross-origin (hostCheck + WS Origin) | accept (only when flag set) | When the flag is ABSENT these defenses remain fully active (regression-guarded by retained TestEnsureLoopback/TestHostCheck/TestIntegrationEvilOriginRejected). When the flag is PRESENT the user has accepted an open network surface; rebinding/cross-origin defense is intentionally disabled for that mode. |
| T-hwd-03 | Information Disclosure | agent-status hook BaseURL on wildcard bind | mitigate | Normalize wildcard host (0.0.0.0/::/empty) to 127.0.0.1 for the hook URL so local hooks never curl a wildcard/remote-looking address; specific-IP binds keep the reachable IP. Functional-correctness fix, prevents broken hook delivery. |
| T-hwd-SC | Tampering | npm/pip/cargo installs | n/a | No new dependencies added. coder/websocket InsecureSkipVerify and net stdlib are already in use. No package-install tasks in this plan. |
</threat_model>

<verification>
- `cd /home/jordi/workspace/github/kangent && go build ./... && go vet ./... && go test ./... -count=1` passes.
- Default-path regression: TestEnsureLoopback, TestHostCheck, and TestIntegrationEvilOriginRejected pass UNCHANGED (the safe-by-default invariant).
- New coverage: TestHookBaseURL (wildcard normalization), the hostCheck-relaxation test, and the WS any-origin-accepted test pass.
</verification>

<success_criteria>
- A single boolean flag `--insecure-allow-remote` (default false) gates all three enforcement layers + the warn banner.
- With the flag ABSENT: ensureLoopback rejects non-loopback (os.Exit), hostCheck 403s non-loopback Host, WS Origin allowlist stays loopback-only — byte-for-byte unchanged from current behavior.
- With the flag PRESENT: a non-loopback --addr binds, non-loopback Host headers are served, cross-origin WS upgrades are accepted, and a loud security banner prints.
- Hook BaseURL: wildcard --addr host normalized to 127.0.0.1:<port>; specific IP left as-is.
- No authentication, no TLS added.
- `go build ./... && go vet ./... && go test ./...` green.
</success_criteria>

<output>
Create `.planning/quick/260626-hwd-add-an-opt-in-insecure-allow-remote-flag/260626-hwd-SUMMARY.md` when done.
</output>

# Phase 06: MCP Subcommand Foundation - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-07-21
**Phase:** 06-mcp-subcommand-foundation
**Areas discussed:** Subcommand dispatch shape, Token validation on /api/*, Proof tool choice, Tool registration breadth

---

## Subcommand dispatch shape

### Q1: How should `kamacu mcp serve` be dispatched in main.go?

| Option | Description | Selected |
|--------|-------------|----------|
| Manual os.Args peek | Inspect os.Args[1] before flag.Parse(): if it's "mcp", route to an mcpRun helper; else fall through to today's flag-based server path. Smallest change, no new deps. | |
| flag.NewFlagSet subcommands | Refactor main.go into a small dispatcher: each subcommand owns a *flag.FlagSet. Still stdlib-only, more idiomatic. | |
| Adopt google/subcommands | Introduce github.com/google/subcommands. Heavier refactor but gives proper `kamacu help`, exit code conventions, and a Command interface. Adds one indirect dep. | ✓ |
| You decide | Pick what fits best — likely manual os.Args peek given it's one subcommand. | |

**User's choice:** Adopt google/subcommands.
**Notes:** User picked the heavier refactor for a clean Command interface that future subcommands slot into. Accepts the one new stdlib-adjacent dep.

### Q2: File layout — main.go or internal/mcp?

| Option | Description | Selected |
|--------|-------------|----------|
| Thin dispatch in main.go, body in internal/mcp | main.go only routes — calls mcp.Serve(args, env). Matches the project pattern. Body is testable in isolation. | ✓ |
| All in cmd/kamacu/ | Add cmd/kamacu/mcp_serve.go next to main.go; everything in package main. Simpler but bypasses internal/ convention. | |
| You decide | Likely thin-dispatch + internal/mcp/. | |

**User's choice:** Thin dispatch in main.go, body in internal/mcp.

### Q3: Server subcommand name (under google/subcommands)?

| Option | Description | Selected |
|--------|-------------|----------|
| Implicit default (no name) | Today's UX preserved: `kamacu` (no args) runs server via fallback. Zero behavior change. | |
| Explicit `serve` | `kamacu serve` runs server; `kamacu` no-args prints help. Breaking change. | ✓ |
| Both — serve alias + default | Register `serve` AND keep `kamacu` running server via fallback. Preserves UX, more code. | |
| You decide | Likely implicit-default. | |

**User's choice:** Explicit `serve`.
**Notes:** User consciously accepts the breaking change for cleaner subcommand model.

### Q4: Stub future subcommands or just `mcp serve`?

| Option | Description | Selected |
|--------|-------------|----------|
| Just `mcp serve` | Only register what ships now. Cleanest, no dead code. | ✓ |
| Stub `mcp` subtree only | Reserve `mcp <other>` namespace for future. | |
| You decide | Likely just `mcp serve`. | |

**User's choice:** Just `mcp serve`.

### Q5: Migration approach for the breaking `serve` change?

| Option | Description | Selected |
|--------|-------------|----------|
| Break clean — update everything | Update README/docs/scripts. Bare `kamacu` prints help. No transitional code. | ✓ |
| Transitional shim | Bare `kamacu` warns + runs server. Remove later. | |
| You decide | Likely break-clean. | |

**User's choice:** Break clean — update everything.
**Notes:** User explicitly preferred honest breaking changes over transitional shims throughout the discussion.

---

## Token validation on /api/*

### Q1: How should KAMACU_HOOK_TOKEN be enforced on /api/* routes in v1.11?

| Option | Description | Selected |
|--------|-------------|----------|
| Loopback-only (no new middleware) | Token sent but no new server-side enforcement. Loopback binding is the actual auth boundary. Matches existing SPA posture. | ✓ |
| Opt-in MCP middleware | Validates token when an MCP-specific header is present; SPA bypasses. | |
| Universal token on /api/* | Token validation on all /api/* routes. Strongest but biggest blast radius. | |
| You decide | Likely loopback-only. | |

**User's choice:** Loopback-only.
**Notes:** v1.11 stays aligned with the existing SPA posture. Full token validation is post-v1.11 work (MCPHARD).

### Q2: Which header name should MCP use?

| Option | Description | Selected |
|--------|-------------|----------|
| Reuse X-Kangent-Token | Same header /api/hooks/* reads. Legacy name carried forward. | |
| New X-Kamacu-Token | Matches v1.8 rebrand. Phase 06 also updates /api/hooks/* (or accepts both). | ✓ |
| Both (transition) | Send both during transition. | |
| You decide | Likely X-Kamacu-Token. | |

**User's choice:** New X-Kamacu-Token.

### Q3: Should Phase 06 also rename /api/hooks/* from X-Kangent-Token to X-Kamacu-Token?

| Option | Description | Selected |
|--------|-------------|----------|
| Leave /api/hooks/* alone | Two header names for same token in different code paths. Cleanup is future task. | |
| Also rename /api/hooks/* | Aligns naming across the codebase. Small scope creep outside MCPPROC. | ✓ |
| You decide | Likely leave-alone. | |

**User's choice:** Also rename /api/hooks/*.
**Notes:** User chose coordinated rename across Go + JS + tests. No fallback during transition (the hook token is regenerated fresh per server start, so server+agents are always same-release).

---

## Proof tool choice

### Q1: Which tool should be the architecture proof in Phase 06?

| Option | Description | Selected |
|--------|-------------|----------|
| list_projects (zero endpoint change) | Uses existing GET /api/projects unchanged. Simplest proof. MCPTASK-01's unscoped list_tasks becomes Phase 07. | ✓ |
| list_tasks (needs /api/tasks) | Requires adding GET /api/tasks unscoped endpoint. Slightly more work. | |
| Both list_projects + list_tasks | Two proof tools. SC3 says ONE; over-achieving. | |
| You decide | Likely list_projects. | |

**User's choice:** list_projects.

### Q2: Is the Phase 06 proof tool final code or throwaway?

| Option | Description | Selected |
|--------|-------------|----------|
| Final code, credited in P07 | Same tool ships unchanged in Phase 07. Phase 07 adds 12 more. | ✓ |
| Sketch — may be rebuilt | Phase 06 is just architecture proof; researcher/planner may rebuild in Phase 07. | |
| You decide | | |

**User's choice:** Final code, credited in P07.
**Notes:** Important — Phase 06's list_projects is the production tool Phase 07 inherits unchanged. The work product is the bridge PATTERN, not a throwaway demo.

---

## Tool registration breadth

### Q1: What does tools/list return in Phase 06?

| Option | Description | Selected |
|--------|-------------|----------|
| Just list_projects | ONE tool, matches SC3 literally. Phase 07 adds the other 12. | ✓ |
| All 18 tools (17 stubs) | Stubs return "not implemented until Phase 07+". Early visibility, dead code. | |
| list_projects + a few reads | Middle ground. Arbitrary line; SC3 says ONE. | |
| You decide | Likely just-list_projects. | |

**User's choice:** Just list_projects.

### Q2: SC2 mandates a "malformed-tool regression test". What test shape?

| Option | Description | Selected |
|--------|-------------|----------|
| Unit test only (internal/mcp) | Tests SDK invariant directly at package boundary. No e2e harness. | ✓ |
| Unit test + build-tagged e2e | Closer to SC2 intent but heaviest. MCPHARD-03 deferred to v1.12. | |
| Bridge integration test | Real httptest server; SC2's stdout-pollution guard becomes a unit test on SDK init path. | |
| You decide | Likely unit-test-only. | |

**User's choice:** Unit test only (internal/mcp).

### Q3: How aggressively should Phase 06 enforce stdout cleanliness for the MCP subcommand?

| Option | Description | Selected |
|--------|-------------|----------|
| Rely on stdlib defaults + code review | slog defaults to stderr; internal/mcp/ is fmt.Print*-free by convention. No new infrastructure. | ✓ |
| Active stdout guard wrapper | Belt-and-braces: any future stray print is caught. Heavier. | |
| CI grep check (MCPHARD-02 lite) | Makefile/lint step fails on fmt.Print* in internal/mcp/*. | |
| You decide | Likely rely-on-stdlib. | |

**User's choice:** Rely on stdlib defaults + code review.
**Notes:** Full MCPHARD-02 stdout-pollution guards deferred to v1.12.

---

## the agent's Discretion

The user explicitly left these to researcher/planner:
- google/subcommands registration shape (command group vs. individual commands)
- `internal/mcp/` package file layout (single file vs. split)
- MCP server identification (name + version string for handshake)
- HTTP client construction (timeout, transport, retry behavior)
- Exact malformed-tool unit test cases (which malformations to cover)
- `list_projects` MCP output shape (raw passthrough vs. massaged)
- Stdin / lifecycle behavior (graceful shutdown on stdin close, exit codes)

## Deferred Ideas

Captured as items NOT in Phase 06 scope (see CONTEXT.md `<deferred>` for the full list with rationale):

- MCPAUTO-01..03 — per-task auto-scoping (v1.12)
- MCPREG-01..03 — agent-CLI auto-registration at spawn (v1.12)
- MCPMORE-01..03 — additional tool categories (v1.12)
- MCPHARD-01 — typed JSON-RPC error taxonomy (v1.12)
- MCPHARD-02 — full stdout-pollution guards (v1.12)
- MCPHARD-03 — real-binary e2e harness (v1.12)
- `list_tasks` unscoped endpoint (Phase 07's call)
- Workspace filter on `list_projects` (Phase 07 may want it)
- Server-side token validation middleware on /api/* (post-v1.11)
- External-editor MCP server (Claude Desktop / Cursor / VS Code — separate future milestone)

None pulled into Phase 06 — discussion stayed within the MCPPROC-01/02/03 boundary.

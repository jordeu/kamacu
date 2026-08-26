# Phase 15: Global sessions backend - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-08-26
**Phase:** 15-Global sessions backend
**Areas discussed:** Spawn-gate error posture

---

## Gray-area selection

| Option | Description | Selected |
|--------|-------------|----------|
| Spawn-gate error posture | Unconfigured vs vanished root: same 409 or distinct errors? | ✓ |
| Stop surface shape | Per-session stops vs scope-level stop-all endpoint | |
| Bash tab labels | Task-parity "Bash N" vs scratch-distinct naming | |
| Resume-id capture timing | Parity persist/capture choices for the singleton ids | |

**User's choice:** Spawn-gate error posture only.

---

## Spawn-gate error posture

### Q1: Unconfigured root (root_path='', github_repo=NULL)

| Option | Description | Selected |
|--------|-------------|----------|
| 409 Conflict | Mirrors "task has no worktree" (sessions.go:269); state conflict, not malformed request | ✓ |
| 400 Bad Request | Client-error framing; no precedent among session gates | |

**User's choice:** 409 Conflict → D-28.

### Q2: Root configured but directory vanished since PUT

| Option | Description | Selected |
|--------|-------------|----------|
| Distinct 409 naming the path | "global root no longer exists on disk: <path>" — distinguish misconfig from disk rot | ✓ |
| Same 409 as unconfigured | One gate, one error; sends user to reconfigure when mkdir would fix | |
| 500 parity with tasks | Fall through to Spawn's stat pre-check; least honest | |

**User's choice:** Distinct 409 naming the path → D-29.

### Q3: Which request kinds hit the gate

| Option | Description | Selected |
|--------|-------------|----------|
| All kinds uniformly | Agent, plain bash, tmux, resume — one gate before kind dispatch; no silent $HOME | ✓ |
| Agent only | Only agents dangerous; bash in $HOME would be a silent-fallback lie | |

**User's choice:** All kinds uniformly → D-30.

### Q4: resume:true with no persisted singleton id

| Option | Description | Selected |
|--------|-------------|----------|
| 409 — nothing to resume | Honest refusal; silent fresh spawn pretends to resume | ✓ |
| Silent fresh spawn | Friendlier-looking but a lie | |

**User's choice:** 409 — nothing to resume → D-31.

### Q5: Cross-scope tmux reattach (scope:"global" + task-scoped name)

| Option | Description | Selected |
|--------|-------------|----------|
| 404 — not found in scope | Scoped lookup; foreign row doesn't exist in scope; no info leak | ✓ |
| 409 — scope mismatch | Louder but leaks scope details; adds a branch tasks don't have | |

**User's choice:** 404 — not found in scope → D-32.

### Q6: Gate evaluation order (multiple gates tripping)

| Option | Description | Selected |
|--------|-------------|----------|
| Root gates first | Config-level checks cheapest/most actionable; mirrors task path ordering | ✓ |
| Agent-limit first | Running agent implies root was configured; mixed case rare | |

**User's choice:** Root gates first → D-33.

### Q7: Does an EXITED agent block a new Start?

| Option | Description | Selected |
|--------|-------------|----------|
| No — exited never blocks | SC1 "second concurrent"; fresh spawn replaces — task parity (agents.go:31) | ✓ |
| Yes — exited blocks too | Stricter; adds ceremony tasks don't have | |

**User's choice:** No — exited never blocks → part of D-34.

### Q8: One-agent 409 copy

| Option | Description | Selected |
|--------|-------------|----------|
| Mirror task-gate voice | "global agent already running" — same voice as Phases 13–14 Scratchpad 409s | ✓ |
| the agent's discretion | Only the 409 code locked | |

**User's choice:** Mirror task-gate voice → D-34.

---

## the agent's Discretion

- Engine field spelling (`SpawnOpts.Global` etc.), `ListGlobal`/`StopAllForScope` signatures, counter wiring
- `agentStatusEntry` wire mechanics for the global entry — flagged planning-research spike
- opencode capture re-target mechanics — flagged planning-research spike (host-gated e2e)
- Bash tab label spelling (task-parity "Bash N" default)
- csid persist-failure posture (task `slog.Warn` parity is the default)
- Wave split at planning time (engine+bash, then agent+status — waves, not phases)

## Deferred Ideas

None — discussion stayed within phase scope. Areas presented but not selected (stop surface shape, bash tab labels, resume-id capture timing) were left to planning/discretion, not deferred to future phases. GT-FUT-01..07 remain parked in REQUIREMENTS.md.

# Phase 14: Global config API - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-08-25
**Phase:** 14-Global config API
**Areas discussed:** GET derived state, PUT grammar & clear, Agent-change gating, Folder-overlap policy

---

## GET derived state

### Q1: What should GET /api/global return beyond the stored singleton row?

| Option | Description | Selected |
|--------|-------------|----------|
| Config + live truths | Stored row + root_exists (os.Stat at GET time) + live global session counts + embedded agent summary (id, name, engine) — one round-trip serves Settings and the /global view | ✓ |
| Config only | Just the stored row; Phase 16 composes extra requests | |
| Config + agent only | Row + agent detail, no live checks | |

**User's choice:** Config + live truths
**Notes:** root_exists gives honest vanished-root reporting without violating D-08 (no boot revalidation); counts serve the Phase 16 gate UX; agent summary saves a join on the client.

### Q2: How should the persisted resume session ids surface on the GET wire?

| Option | Description | Selected |
|--------|-------------|----------|
| Internal only | GET never returns claude_session_id / opencode_session_id — Phase-15 spawn-path internals | ✓ |
| Presence booleans | has_resumable flags without leaking ids | |
| Full ids on wire | Raw ids — localhost single-user, maximum curl debuggability | |

**User's choice:** Internal only

### Q3: Should the live-session count fields ship in the Phase 14 wire shape or be added by Phase 15?

| Option | Description | Selected |
|--------|-------------|----------|
| Ship zeros now | Full wire shape from day one; counts (and the 409 gate check) forward-wired, trivially zero/empty until Phase 15 | ✓ |
| Defer counts to P15 | No dead fields, but the wire shape shifts one phase later | |

**User's choice:** Ship zeros now

---

## PUT grammar & clear

### Q1: How should the write surface be shaped?

| Option | Description | Selected |
|--------|-------------|----------|
| Single partial PUT | One PUT /api/global with pointer fields repo/root_path/agent_id; dispatch mirrors projects create (non-empty repo wins, repo+root_path → 400, omitted = untouched) | ✓ |
| Sub-resource routes | PUT /api/global/root + PUT /api/global/agent + clear route — one intent per call but three routes and a divergence from the app idiom | |

**User's choice:** Single partial PUT

### Q2: How is "clear the root" spelled on the wire?

| Option | Description | Selected |
|--------|-------------|----------|
| root_path:"" clears | Explicit empty string resets root_path to '' and github_repo to NULL, behind the same 409 gate — the app-wide ""=clear pointer idiom | ✓ |
| clear_root flag | Dedicated boolean — more discoverable but a second way to say "no root" | |
| DELETE sub-route | DELETE /api/global/root — reads as its own intent but breaks the single-endpoint spelling | |

**User's choice:** root_path:"" clears

### Q3: What should the PUT (and GET) field names be?

| Option | Description | Selected |
|--------|-------------|----------|
| root_path + repo | root_path matches the 00017 column (GET/PUT share one name); repo matches the v1.4 create dispatch field | ✓ |
| repo_path + repo | Mirrors projects create byte-for-byte but gives the same concept two names on one endpoint | |
| root + repo | Shortest but introduces a third name for the folder concept | |

**User's choice:** root_path + repo

### Q4: What does a successful PUT return?

| Option | Description | Selected |
|--------|-------------|----------|
| Return GET shape | Updated config + root_exists + counts + agent summary — one wire type, curl shows the outcome | ✓ |
| 204 + re-GET | One less payload but forces a second round-trip and hides clone/reattach outcomes | |

**User's choice:** Return GET shape

---

## Agent-change gating

### Q1: How should changing the global default agent behave while a global agent session is live?

| Option | Description | Selected |
|--------|-------------|----------|
| Allow anytime | No 409 — live session keeps its agent, change applies at next Start; exactly the per-project pattern (projects.go:616) | ✓ |
| 409 while live | Gate parity with root — stricter than projects, prevents surprise about which agent a Resume continues under | |
| Allow + hint | No gate but an informational note field for "applies at next start" copy | |

**User's choice:** Allow anytime
**Notes:** Root gating exists because a cwd change under a running shell is a lie; an agent-default change mutates nothing under a live session.

### Q2: When the default agent changes (root unchanged), what happens to the persisted resume session ids?

| Option | Description | Selected |
|--------|-------------|----------|
| Keep ids | Agent changes never touch ids; D-16 stays the only clearing path; ids are engine-keyed not agent-keyed | ✓ |
| Clear old engine's id | Hygiene — old conversation never offered again, at the cost of losing a switch-back resume | |
| Clear both ids | Treat an agent switch like a root change — destroys resumable state the gate protects | |

**User's choice:** Keep ids

---

## Folder-overlap policy

### Q1: May the global root be a folder that is also a project's repo_path?

| Option | Description | Selected |
|--------|-------------|----------|
| Allow any valid folder | Git-repo check + $HOME/~/ / footgun block only; mirrors D-03's no-cross-entity-guards stance | ✓ |
| 409 on project overlap | Mirrors the project↔project folder dedup but adds the guard class D-03 rejected, and it can go stale | |
| Allow + expose overlap | Allow but surface root_shared_with_project in GET for a stronger Phase 16 banner | |

**User's choice:** Allow any valid folder

### Q2: Should a folder root inside ~/.kamacu be blocked at configure time?

| Option | Description | Selected |
|--------|-------------|----------|
| Block ~/.kamacu | One HasPrefix check, 400 with clear copy — everything under it is Kamacu-managed machinery (worktrees gated-removed, clones project-owned); extends the D-07 footgun block | ✓ |
| Allow like any folder | User may genuinely want a persisted worktree; vanish risk covered by D-08 spawn-time honesty | |

**User's choice:** Block ~/.kamacu

---

## the agent's Discretion

- Clone-failure status code (v1.4 500 + inline error is the default mirror)
- Empty-body PUT {} behavior (workspaces "nothing to update" 400 precedent)
- Same-value agent_id re-PUT (no-op success fine)
- Exact JSON key spelling of agent summary and count fields
- reattachManaged reuse verbatim vs thin wrapper
- How the forward-wired live check queries until Phase 15's ListGlobal() lands

## Deferred Ideas

None — discussion stayed within phase scope. GT-FUT-01..07 remain parked in REQUIREMENTS.md.

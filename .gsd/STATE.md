# GSD State

**Active Milestone:** M001: Configurable Agents
**Active Slice:** S02: Agent management UI and per-project selection
**Phase:** planning
**Requirements Status:** 12 active · 6 validated · 5 deferred · 0 out of scope

## Milestone Registry
- 🔄 **M001:** Configurable Agents

## Recent Decisions
- D008 (v1.4 (Phases 14–15)): Standard delivery shape: backend-capability phase then UI-driver phase -> Split a user-facing feature into two phases: (1) prove the engine/backend with tests and a typed wire contract, (2) a thin UI layer over it. Verify the seam with an integration audit rather than assuming it.
- D009 (v1.2 (internal/tmux), v1.3 (internal/github)): Leaf-package-per-external-tool, extended not scattered -> Every verb for an external tool lives in one leaf package (internal/tmux, internal/github, internal/migrate) and is extended there, not scattered as raw shell calls across handlers. Shared services are constructed once in main.go and injected via narrow interface seams (e.g. PRStateGetter) to avoid import cycles.
- D010 (v1.7 (Phase 18)): Small fixed lookup sets shared between Go and TS -> For a small, fixed, shared lookup (e.g. the project color palette), keep the source of truth in Go and mirror it byte-for-byte as a TS const with a provenance comment — no endpoint round-trip.
- D011 (v1.1 (Phase 6)): Settings storage: KV with absent-row-as-default, managers DB-free -> Settings live in a SQLite KV store with NO seeded rows — defaults live in code, so an absent row = the default. Managers (session, worktree) take values via SpawnOpts/locals and never import the store.
- D012 (v1.5, strongly re-confirmed v1.7): Human-verify gate authority over design -> A human-verify gate is allowed to CHANGE a design, not just bless it. When the gate flips a visual decision, revise the design contract (CONTEXT/UI-SPEC) alongside the code and re-verify against what shipped. For net-new visual primitives, produce a cheap visual mock during ui-phase before building.

## Blockers
- None

## Next Action
Slice S02 has no DB tasks. Plan slice tasks before execution.

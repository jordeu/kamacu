# Project Retrospective

*A living document updated after each milestone. Lessons feed forward into future planning.*

## Milestone: v1.0 — MVP

**Shipped:** 2026-06-11
**Phases:** 5 | **Plans:** 28 | **Tasks:** 74

### What Was Built
- Single Go binary serving an embedded React SPA at localhost: projects sidebar, four-column kanban board, task CRUD, SQLite persistence (Phase 1)
- Server-owned PTY session engine with ring-buffer replay, binary WebSocket bridge, xterm.js terminal, detach/reattach, and loopback-only security (Phase 2)
- Worktree-per-task isolation (`task/<slug>-<id>` under `~/.kangent/worktrees/`), bash tabs inside the worktree, gated confirmed cleanup that always keeps the branch (Phase 3)
- Real `claude` CLI in the worktree PTY via Start button, invisible additive status hooks, dispatcher board with status dots and sidebar waiting chips (Phase 4)
- Restart reconciliation (zero schema change), `claude --resume <uuid>` recovery, and a read-only merge-base diff tab (Phase 5)

### What Worked
- **Risk-front-loaded roadmap.** Phase 2 (the PTY/WebSocket session manager — the only subsystem with no off-the-shelf solution) was built and proven against plain bash before anything depended on it. Every later phase reused that proven stack instead of discovering its risk late.
- **Empirical phase research over training-data assumptions.** Each phase verified version-specific behavior against the actually-installed binaries (git 2.43, claude v2.1.170→173). This caught real surprises before planning: claude does NOT use the alt-screen buffer (the feared reattach problem evaporated), `--settings` overlays merge additively, `--resume` reuses the same session ID (no DB mutation needed), the worktree branch-leak footgun, and the `git diff --no-index` exit-1-as-success quirk.
- **Interface-first wave parallelism.** Plans that published exact Go signatures / REST contracts in their `<interfaces>` blocks let backend and frontend tracks run fully parallel with disjoint file ownership — both waves of most phases ran two agents at once with no merge conflicts.
- **Headless-Chrome pre-checks before every human checkpoint.** The orchestrator drove the real app in headless Chrome before handing each checkpoint to the user — this caught a tree-crashing `TooltipProvider` bug (blank page) and the collapsed-sidebar overlap in Phase 1 before the user wasted time, and pre-verified the diff tab in Phase 5.
- **fake-claude stub for CI.** Agent-session tests never touched the real claude binary (no API quota, deterministic), while the real binary was reserved for the human checkpoint.

### What Was Inefficient
- **Stray verification servers blocked the user's port.** Twice (Phases 2 and 5 setup) an orchestrator/executor verification instance was left holding port 7333, blocking the user's own `./bin/kangent`. Fixed by an explicit "kill any server before returning" instruction to checkpoint executors — should be a standing rule from the start.
- **Smoke tests gave false confidence.** Phase 1's restart-persistence smoke test passed while the UI was a blank page (the Tooltip crash) because it only asserted HTTP responses, not browser rendering. Pure-HTTP verification is necessary but not sufficient for UI phases.
- **Parallel-executor commit interleaving.** Wave-parallel agents occasionally raced on commits (an `--amend` collided with a sibling's commit; `go.mod` ownership was ambiguous between two plans). Recovered each time, but tightening `files_modified` ownership and avoiding `--amend` under parallelism would prevent it.
- **Shell cwd drift.** The orchestrator's shell repeatedly reset to `web/` or a temp dir, causing several `gsd-tools` calls to fail with "not found" until re-run from the repo root. Absolute paths or explicit `cd` guards would have avoided the churn.

### Patterns Established
- **The `TabDef[]` seam.** Phase 1 shipped the task-view tab strip as a data-driven array with just a Description tab; Phases 2–5 appended Agent, Bash, and Diff tabs to the same seam with zero rework. Designing the extensibility point in the first phase paid off four times.
- **Reserved color budget.** Phases 1–3 deliberately kept the palette neutral so status color (Phase 4's amber/green/gray dots) and diff color (Phase 5's green/red) could own their meaning. The UI-SPEC inheritance chain enforced this across phases.
- **Checkpoint-feedback loop is real design input.** Phase 4's exited-agent UX was reworked from user feedback (dimmed terminal, code-free banner, "Reset session" not "Start again"), and the CONTEXT/UI-SPEC docs were updated to match so the change persisted — user feedback overrode the previously-approved spec cleanly.
- **Decisions persisted by ID (D-NN).** Every discuss-phase decision got a stable ID referenced verbatim in plan actions and grep-able acceptance criteria, making decision fidelity checkable rather than aspirational.

### Key Lessons
1. **Verify external-tool behavior empirically, per version, before planning** — training-data assumptions about claude/git cost nothing to check and the checks repeatedly changed the plan (alt-screen, --resume, --settings merge, diff exit codes).
2. **Build the riskiest, least-off-the-shelf subsystem first** against a stand-in (bash before claude) so the rest of the project composes proven parts.
3. **For UI phases, drive a real browser before declaring done** — HTTP/test-level green does not mean the page renders.
4. **Make checkpoint executors clean up their own servers/processes** — a standing teardown rule, not a per-checkpoint reminder.
5. **A data-driven extensibility seam in phase 1 compounds** — the tab strip absorbed four later phases without rework.

### Cost Observations
- Model mix: phases planned/executed predominantly on Sonnet (inherit profile); orchestration spanned Fable and later Opus 4.8.
- Notable: wave-parallel execution (2 agents/wave for most phases) roughly halved wall-clock per wave; the fake-claude stub kept agent-session testing at zero API cost.

---

## Milestone: v1.1 — Settings & Polish

**Shipped:** 2026-06-11
**Phases:** 1 | **Plans:** 4

### What Was Built
- SQLite-backed settings KV store (migration 00004) with code defaults (absent row = default), per-key validation carrying the UI-SPEC canonical error copy, branch-template expansion with git ref validation, quote-aware extra-params tokenizer, and the GET/PUT `/api/settings` surface (06-01)
- All four settings wired read-at-use into their existing v1.0 call sites — claude extra-params on every spawn (fresh + resume), LookPath-resolved bash-tab shell, template-driven branches under the settings worktree base — managers stayed DB-free, so "applies at next spawn, no restart" is structural (06-02)
- Full `/settings` page with per-field commit (blur/Enter, Esc revert, 2s Saved flash, Reset to default, verbatim inline server errors), sidebar gear, API-driven shell select, and the UI-01 full-width task header (06-03)
- Phase gate: clean release binary, full suite green across 8 packages, 8/8 verbatim copy audit, restart-persistence smoke, all seven success criteria approved live (06-04)

### What Worked
- **Coarse single-phase milestone.** Twelve requirements in one phase with 4 plans kept planning overhead proportionate to integration-only work — no artificial phase boundaries inside what was really one feature.
- **Integration-over-net-new framing held.** Every setting landed in an existing v1.0 call site; zero new Go modules and zero new npm deps vs the v1.0 tag, verified by diffing manifests against the tag in the gate.
- **Grep-able copy contract.** The UI-SPEC's canonical error strings were asserted verbatim by tests and re-audited at the gate (8/8) — copy fidelity stayed checkable, not aspirational.
- **Backend/frontend wave parallelism again.** Disjoint file ownership let 06-02 and 06-03 run concurrently in the same checkout with interleaved commits and no conflicts.
- **Checkpoint → continuation-agent flow.** The 06-04 gate ran its automated half, returned structured state, and a fresh continuation agent closed the plan after approval — no resume fragility.

### What Was Inefficient
- **One cross-plan test-harness clash.** 06-01's `TestSettingsGetAllDefaults` collided with 06-02's planned seed row; auto-fixed in-flight, but explicit harness/seed ownership in plan frontmatter would have prevented it.
- **gsd-tools key-link checker false negatives.** 5 link checks failed on path-suffix parsing and a regex-escaping bug; each had to be manually re-verified with grep during verification.
- **Checkpoint asked for two things, got one.** The gate requested approval *plus* the carried plan-mode-amber observation; the user replied only "approved", so the carried v1.0 UAT item (research OQ1) remains open. Observations should be separate, explicit questions.

### Patterns Established
- **Settings KV with absent-row-as-default.** No seeded rows; defaults live in code, so new settings need no migration and `Reset to default` is a row delete.
- **Read-at-use settings reads in handlers.** Managers take values via `SpawnOpts`/locals and never import the store — next-spawn semantics by construction.

### Key Lessons
1. **Make checkpoint questions atomic** — a combined "approve + report observation" prompt yields partial answers; carried UAT items need their own explicit question.
2. **Declare test-harness/seed ownership across plans** — parallel plans sharing a test DB schema need the same `files_modified`-style discipline for fixtures.
3. **Diff dependency manifests against the previous tag at the gate** — cheap, mechanical proof of the zero-new-deps discipline.

### Cost Observations
- Single execution session (orchestrated on Fable 5, inherit profile): 6 subagents (~830k subagent tokens) across 3 waves + verifier; 19 commits.
- Notable: wave 2 ran backend and frontend executors concurrently in the shared checkout (harness worktree isolation unavailable); zero conflicts thanks to disjoint ownership.

---

## Cross-Milestone Trends

### Process Evolution

| Milestone | Phases | Plans | Key Change |
|-----------|--------|-------|------------|
| v1.0 | 5 | 28 | Established the discuss → ui-spec → research → plan → execute → verify loop with human checkpoints; risk-front-loaded roadmap; interface-first wave parallelism |
| v1.1 | 1 | 4 | Coarse single-phase milestone for integration-only scope; checkpoint → fresh continuation-agent flow; manifest-diff-vs-tag as the zero-dep gate |

### Cumulative Quality

| Milestone | Go packages tested | Frontend | Zero-Dep Discipline |
|-----------|--------------------|----------|--------------------|
| v1.0 | 6 (api, diff, session, store, worktree, ws) | tsc + vite build green | Held throughout — only sanctioned deps added (xterm set, dnd-kit, radix collapsible); no go.mod surprises |
| v1.1 | 7 (+settings) | tsc + vite build green | Held — zero new Go modules and zero new npm deps, proven by manifest diff against the v1.0 tag |

### Top Lessons (Verified Across Milestones)

1. **Empirical per-version tool research before planning** — exercised lightly in v1.1 (integration-only scope; call sites verified against the actual v1.0 code rather than assumptions). Held.
2. *(still to be re-tested — v1.1 had no novel-risk subsystem)* Build the riskiest subsystem first against a stand-in.
3. **Interface-first wave parallelism with disjoint file ownership** — confirmed across both milestones; zero merge conflicts in any parallel wave.

---
phase: 16-global-view-settings-bar-integration
verified: 2026-08-27T14:05:00Z
status: passed
score: 10/12 must-haves verified
behavior_unverified: 2 # truths present + wired, runtime behavior not exercised by any test
behavior_unverified_items:

  - truth: "The Agent tab starts the configured default agent in the global root and bash tabs spawn with the same shell options as tasks — through the Phase-15 session surface (SC2)"
    test: "With a configured folder root, open /global, click Start agent, then the trailing + for a bash tab"
    expected: "Agent terminal streams a session whose cwd is the global root; a bash tab spawns with task-parity shell options (plain bash + invisible tmux); the running agent's ⋯ menu shows only Stop; Stop ends the session; a second concurrent agent spawn 409s"
    why_human: "The click→mutate→POST→PTY→WS-attach chain is an interactive runtime flow; grep proves both halves wired (AgentTab global branch → useSpawnGlobalAgent/useSpawnGlobalSession → POST /api/sessions {scope:'global'} handlers) and the backend halves are API-tested (TestGlobalSessionPlainBashSpawn et al., all passing), but no test drives the browser flow"

  - truth: "A live global agent session appears as a row in the global Active Sessions bar with click-through to /global and current-page highlight (SC5 / GINT-01)"
    test: "Spawn a global agent (curl POST /api/sessions {\"scope\":\"global\",\"kind\":\"agent\"} or via Start), expand the Active Sessions bar, click the Global · Scratchpad row"
    expected: "A standard bar row renders (synthesized server label), clicking it navigates to /global and collapses the bar, and the row stays highlighted while location.pathname === '/global'"
    why_human: "Live row appearance, click-through, and highlight are runtime rendering of the 5s status feed; wiring is fully present (sessionId keying, source===\"global\" nav + pathname-highlight branches, server Source:\"global\" synthesis shipped and tested in Phase 15) but observable only in a running app"
human_verification:

  - test: "Live bar row + click-through + highlight (GINT-01)"
    expected: "A live global agent session renders as a Global · Scratchpad bar row; clicking navigates to /global; the row highlights while /global is the current page; task rows keep their existing navigation/highlight"
    why_human: "Runtime rendering of the status feed in a running app; the plans defer this smoke to end-of-phase UAT (16-01 D4 rationale)"

  - test: "/global interactive session parity (GVIEW-01 behavior half)"
    expected: "Start agent spawns the configured default agent in the global root (terminal streams); ⋯ menu renders exactly Stop (destructive, no Insert items, no separator); Stop works; trailing + spawns bash tabs with task-parity options; 409 on a second concurrent agent"
    why_human: "Interactive PTY/WS flow — no automated test drives the browser click path"

  - test: "7-state matrix walkthrough (GVIEW-04)"
    expected: "unconfigured → D-39 hero with Open Settings; folder root → shell + persistent D-38 banner; repo root → shell with NO banner; root renamed on disk while live → D-42 advisory banner with terminals still streaming; all stopped + vanished → distinct D-40 hero naming the path in mono"
    why_human: "State transitions driven by real config/disk changes against a running server"

  - test: "Settings Scratchpad flows (GCONF-05 observable closure)"
    expected: "Open Scratchpad reaches /global; default-agent Select instant-saves (hint 'Applies at the next Start.'); Change root with a repo shows the blocking Cloning spinner then the managed badge; with a live session, Save root/Clear root surface the 409 lead sentence + mono reasons list with the dialog still open; Clear root (all stopped) returns to 'No root configured yet.'"
    why_human: "Dialog interaction flows against real clone/409 failures; 16-03 D3 explicitly defers to end-of-phase UAT"
---

# Phase 16: Global view, Settings & bar integration — Verification Report

**Phase Goal:** The user-facing global scratchpad — a `/global` route rendering the TaskPage-derived shell with agent + bash tabs only, a Settings Global section to configure root and default agent, and a live row in the Active Sessions bar with click-through. Scratch is a first-class citizen built from the same machinery, not a degraded task.
**Verified:** 2026-08-27T14:05:00Z
**Status:** human_needed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | SC1 (GVIEW-01): `/global` renders the full TaskPage-derived shell with ONLY the Agent tab + bash tabs — no description tab, no diff tab, never on a kanban board — with explicit Start and ⋯ Stop parity | ✓ VERIFIED | `App.tsx:141` route inside AppLayout (plain sibling of /settings, NOT in BoardWorkspaceSync, no params, plain import); `GlobalTaskPage.tsx:165-168,339-394` TabDef list is exactly `[agent, ...visibleSessions]`; grep gates all 0 (`Description`, `keydown`, `BoardWorkspaceSync`, `dangerouslySetInnerHTML` in GlobalTaskPage.tsx); AgentTab global branch: D-41 Start CTA (`AgentTab.tsx:187-204`) + D-36 Stop-only ⋯ menu (insert flags forced false via `isTask`, separator conditional at :248) |
| 2 | SC2: The Agent tab starts the configured default agent in the global root and bash tabs spawn with the same shell options — through the Phase-15 session surface | ⚠️ PRESENT_BEHAVIOR_UNVERIFIED | Both halves wired: `AgentTab.tsx:75-78` global spawn/resume hooks → `useSpawnGlobalAgent()` posts `{scope:"global",kind:"agent"}` (`sessions.ts:207-221`); `GlobalTaskPage.tsx:330-337` handleSpawn → `useSpawnGlobalSession()`; backend handlers + cwd semantics shipped and API-tested in Phase 15 (`TestGlobalSessionPlainBashSpawn`, `TestGlobalSessionUnconfiguredRoot409`, GVIEW-02/03 complete). Browser click-through unexercised — see Human Verification |
| 3 | SC3 (GVIEW-04): With no root configured, the global view shows an honest unconfigured state pointing to Settings — never a silent `$HOME` or cwd fallback | ✓ VERIFIED | Deterministic branch `GlobalTaskPage.tsx:216-232` (`config.root_path === ""` → D-39 hero + Open Settings); distinct D-40 vanished-root hero at :242-260 names root_path in a mono span; no fallback code anywhere in the view; backend driver behaviorally tested — `TestGetGlobalUnconfigured` (passed) asserts `root_path = ""` when unconfigured |
| 4 | SC4 (GCONF-05): Settings Global section configures root + default agent with server errors surfaced inline, clone failures leave the dialog open (no half-configured state), and provides an Open Scratchpad affordance | ✓ VERIFIED | `SettingsPage.tsx:160-163` mounts ScratchpadSection directly after AgentsSection in the max-w-[640px] column; `ScratchpadSection.tsx` — root row server-truth (:64-77: managed owner/name + Badge / folder mono+truncate+title / `No root configured yet.`), instant-save Select controlled by cached agent_id with snap-back + inline error (:85-111), primary `Open Scratchpad` → navigate("/global") (:116); dialog: `onOpenChange(false)` sits ONLY after the awaited `mutateAsync` succeeds (:213-218, :231-233) — close is lexically unreachable on failure; verbatim 409 rendering: sentenceCase lead + `reasons[].target` mono list (:198-207, :252-266); Clear root destructive, only-when-configured, PUT `{root_path:""}`, no nested confirm (:229-237, :341-351) |
| 5 | SC5 (GINT-01): A live global agent session appears as a row in the global Active Sessions bar (synthesized label, non-nullable wire contract) with click-through to /global and current-page highlight | ⚠️ PRESENT_BEHAVIOR_UNVERIFIED | Bar wiring complete: `ActiveSessionsBar.tsx:137` key={entry.sessionId} (taskId keying removed), :145-149 global branch navigate("/global")+collapse, :140-142 pathname highlight (task rows unchanged at :142), useLocation imported+used (:2,:77); server synthesis shipped Phase 15 (`agents.go:192,280` Source:"global"); live appearance/click-through needs a running app — see Human Verification |
| 6 | Wire layer (16-01): source union widened, TermSession.global?: boolean, ApiError carries structured reasons | ✓ VERIFIED | `agents.ts:17` `"manual" \| "github_pr" \| "global"`; `sessions.ts:26` `global?: boolean`; `client.ts:6-24,34-49` ApiErrorReason + reasons populated in the !res.ok path — matches the actual 409 body at `global.go:340-345` (`{error, reasons}`); tsc -b green in build |
| 7 | The ONE shared ["global"] query + PUT-to-cache (D-17/D-23) | ✓ VERIFIED | `global.ts:32-57` — useGlobal (queryKey ["global"], 5000ms), useSaveGlobal partial-body PUT `{root_path?\|repo?\|agent_id?}` with setQueryData(["global"], response), no refetch; consumed by BOTH GlobalTaskPage and ScratchpadSection |
| 8 | Six scope-aware session hooks keyed ["sessions","global"] with exact Phase-15 POST grammar and the setQueryData-then-invalidate contract | ✓ VERIFIED | `sessions.ts:185-280` — useGlobalSessions (GET ?scope=global ×1), useSpawnGlobalSession, useSpawnGlobalAgent (+agent-statuses invalidation), useResumeGlobalAgent, useReattachGlobalTmux, useRenameGlobalSession; every onSuccess does setQueryData FIRST then prefix invalidation; bodies match `sessions.go:332-393` grammar |
| 9 | Bar GINT-01 edits are surgical: SessionRow untouched, exactly 2 `source === "global"` sites, task-row behavior unchanged | ✓ VERIFIED | `ActiveSessionsBar.tsx` — 2 runtime global branches (:140,:145) + pre-existing PR badge (:287) only; SessionRow signature/body unchanged (entry/isCurrent/onOpen); repo-wide `source ===` audit shows only sanctioned sites (bar + AgentTab/GlobalTaskPage lookups + pre-existing github_pr checks) |
| 10 | AgentTab loosened behind AgentTabScope — task path provably unchanged, global D-36/D-41 branches, resumable server-driven | ✓ VERIFIED | `AgentTab.tsx:37-39` exported union; git diff TaskPage.tsx = the AgentTab call site ONLY (scope + description props); task worktree gating byte-identical (:137-181); global resumable from `find(e => e.source === "global")?.resumable` (:97-101) — never inferred; `projectId` count 0; eslint clean |
| 11 | 7-state view matrix: distinct D-39/D-40 heroes, exactly-one banner in states 5/6/7 (D-42/D-38/none), QuotaIndicator iff engine==="claude" off the ["global"] query, static Scratchpad h1, tmux reattach one-shot guard | ✓ VERIFIED | `GlobalTaskPage.tsx:190-212` (pending/error), :214-260 (heroes), :268-294 (banner ternary chain — exactly one of warn/isolation/null), :437,:448 (engine gate, NOT TaskPage's pattern predicate), :447 (static h1, no back-arrow/menu), :128-136 (reattachedRef guard), :421-429 (409-verbatim error span) |
| 12 | Change-root dialog (D-43/D-44/D-45): AddProjectDialog-derived, prevOpen reset, gated segmented tabs, blocking Cloning spinner, every-failure-keeps-open, verbatim reasons | ✓ VERIFIED | `ScratchpadSection.tsx:157-183` (prevOpen adjust-during-render, repo-default when integration on, folder-only otherwise), :248,:329-338 (Cloning spinner), :275-286 (integration-gated Tabs), :209-237 (close only on 2xx), :252-266 (sentenceCase + mono reasons list); zero `dangerouslySetInnerHTML`, zero href-anchors; package.json/components.json untouched (diff stat confirms) |

**Score:** 10/12 truths verified (2 present, behavior-unverified)

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `web/src/api/global.ts` | NEW: GlobalConfig + useGlobal + useSaveGlobal | ✓ VERIFIED | 58 lines, substantive, consumed by GlobalTaskPage + ScratchpadSection + AgentTab |
| `web/src/pages/GlobalTaskPage.tsx` | NEW: trimmed TaskPage shell + 7-state matrix | ✓ VERIFIED | 465 lines, wired to route + all hooks |
| `web/src/components/settings/ScratchpadSection.tsx` | NEW: summary card + selector + CTA + dialog | ✓ VERIFIED | 368 lines, mounted in SettingsPage |
| `web/src/api/client.ts` | ApiError.reasons widening | ✓ VERIFIED | Populated in !res.ok path; named ApiErrorReason export |
| `web/src/api/agents.ts` | source union widened | ✓ VERIFIED | `"manual" \| "github_pr" \| "global"` |
| `web/src/api/sessions.ts` | TermSession.global + six global hooks | ✓ VERIFIED | All six present with exact keys/bodies |
| `web/src/components/layout/ActiveSessionsBar.tsx` | sessionId re-key + nav + highlight | ✓ VERIFIED | 3 surgical edits; SessionRow untouched |
| `web/src/components/task/AgentTab.tsx` | AgentTabScope discriminator | ✓ VERIFIED | Single file serves both scopes; task path byte-identical |
| `web/src/pages/TaskPage.tsx` | AgentTab call-site change ONLY | ✓ VERIFIED | git diff = 3-line JSX change; QuotaIndicator gating untouched |
| `web/src/App.tsx` | /global route | ✓ VERIFIED | AppLayout sibling, no BoardWorkspaceSync |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|----|--------|---------|
| Bar global row | /global route | navigate("/global") in onOpen | ✓ WIRED | Route registered App.tsx:141 |
| ScratchpadSection CTA | /global route | navigate("/global") | ✓ WIRED | Same route |
| GlobalTaskPage / ScratchpadSection / AgentTab | ["global"] cache | shared useGlobal() | ✓ WIRED | One query, three consumers |
| GlobalTaskPage / AgentTab | scoped sessions | useGlobalSessions/useSpawn*/useResume*/useReattach*/useRename* | ✓ WIRED | All hooks consumed |
| ScratchpadSection errorBox | ApiError.reasons | imports ApiErrorReason from client.ts | ✓ WIRED | Renders kind/target verbatim |
| Frontend hooks | Go backend | GET/PUT /api/global, ?scope=global, POST grammar | ✓ WIRED | Handlers registered (global.go:75-76, sessions.go:70,332-393); agents.go synthesizes Source:"global" |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|--------------|--------|--------------------|--------|
| GlobalTaskPage | config / sessions / agentEntry | GET /api/global + GET /api/sessions?scope=global + /api/agents/status | Yes — registered handlers, DB-backed, Phase-15 API tests passing | ✓ FLOWING |
| ScratchpadSection | global / agents | GET /api/global + useAgents + PUT /api/global | Yes — same handlers; 409 body verified to carry reasons | ✓ FLOWING |
| ActiveSessionsBar | live rows | /api/agents/status (Source:"global" synthesized server-side) | Yes — agents.go:192,280 | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Frontend compiles + builds | `npm --prefix web run build` | tsc -b + vite green (2803 modules) | ✓ PASS |
| New/edited files lint clean | `npx eslint` on GlobalTaskPage/AgentTab/ScratchpadSection/ActiveSessionsBar | No output (clean) | ✓ PASS |
| Backend regression suite | `go test ./internal/...` | All packages ok except `TestCustomEngineDoesNotGetHookEnv` | ✓ PASS (environmental) |
| Environmental failure isolated | `go test ./internal/session -run TestCustomEngineDoesNotGetHookEnv` with ambient KAMACU_HOOK_BASE/KAMACU_HOOK_TOKEN stripped | ok — passes without the Kamacu-injected env; zero Go files changed this phase (diff stat: web/src only) | ✓ PASS (not a regression) |
| Unconfigured wire driver (SC3) | `TestGetGlobalUnconfigured` (in passing api package run) | Asserts root_path "" when unconfigured — the exact value the D-39 branch keys on | ✓ PASS |

### Probe Execution

No probes declared in PLAN/SUMMARY and no `scripts/*/tests/probe-*.sh` exist — correctly skipped (UI-only phase; verification rides build + grep gates + UAT).

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|---------------------|----------|
| GCONF-05 | 16-03 | "Open Global Task" affordance makes the view reachable when idle | ✓ SATISFIED | Open Scratchpad primary CTA → /global (ScratchpadSection.tsx:116); marked [x] Complete in REQUIREMENTS.md |
| GVIEW-01 | 16-01, 16-02 | /global route with ONLY Agent + bash tabs, Start/⋯-Stop parity | ✓ SATISFIED (implementation) | Truths 1, 10, 11; REQUIREMENTS.md checkbox deliberately left [ ] pending end-of-phase/Phase 17 UAT — documented deferral in 16-01/16-02 SUMMARYs, consistent with Phase 17's goal ("Closes the verification halves of the earlier phases' requirements") |
| GVIEW-04 | 16-02 | Honest unconfigured state pointing to Settings, never a silent fallback | ✓ SATISFIED (implementation) | Truth 3; same deliberate checkbox deferral |
| GINT-01 | 16-01, 16-02 | Live global session bar row with click-through + highlight | ✓ SATISFIED (wiring) — behavior pending UAT | Truths 5, 9; same deliberate checkbox deferral |

No orphaned requirements: all four phase-mapped IDs (GCONF-05, GVIEW-01, GVIEW-04, GINT-01) are claimed by plan frontmatter. The unchecked REQUIREMENTS.md boxes are a documented, intentional deferral of observable-behavior closure to end-of-phase UAT / Phase 17 — not missing work.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| (none) | — | Zero TBD/FIXME/XXX/TODO/HACK/placeholder markers across all 11 phase files; zero console.log-only handlers; zero raw-HTML injection | — | — |

Notable non-findings (checked, clean): the empty-state branches render server truth (`No root configured yet.`, heroes), not stubs; `globalConfig?.root_path ?? ""` in AgentTab's pre-start is a mono-span placeholder for pending data, immediately populated by the shared cached query.

### Human Verification Required

Four UAT bundles (detailed in frontmatter `human_verification`): (1) live bar row + click-through + highlight, (2) /global interactive Start/Stop/bash parity, (3) 7-state matrix walkthrough incl. D-38/D-42/D-40 transitions, (4) Settings dialog flows incl. verbatim 409-with-live-session and Cloning spinner. All are the flows the plans' verification sections explicitly deferred to end-of-phase manual UAT; Phase 17 additionally owns lifecycle-gate closure.

### Gaps Summary

No gaps. Every artifact exists, is substantive, and is wired to real, handler-registered endpoints; every prohibition audited clean (no unsanctioned `source ===` sites, TaskPage touched only at the call site, no BoardWorkspaceSync/params/Escape on /global, no client re-wording, no anchor routing, zero package/registry drift, no client-side liveness/resumability inference). Build, isolated eslint, and the backend suite are green (the single Go failure is the documented Kamacu-ambient-env caveat, reproduced-passing with the vars stripped; no Go files changed this phase).

The two behavior-unverified truths (SC2 interactive session parity, SC5 live bar row) are present and wired on both frontend and backend halves — their observable closure was deliberately routed by the plans to end-of-phase/Phase 17 manual UAT, which is exactly what this report's `human_needed` status surfaces.

---

_Verified: 2026-08-27T14:05:00Z_
_Verifier: the agent (gsd-verifier)_

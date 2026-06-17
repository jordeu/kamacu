# Phase 16: Sharper Review Column - Context

**Gathered:** 2026-06-17
**Status:** Ready for planning

<domain>
## Phase Boundary

Make the per-project Review column convey two **independent** at-a-glance signals per PR — *your* agent's session state and the PR's CI state — without the two being confused for each other, and stop losing PRs from view after you review them by adding a "Recently reviewed" section that holds reviewed PRs until they merge or close.

In scope: the agent-state dot + open-session border on PR cards; CI status rendered as icons (replacing the colored dot); the second `reviewed-by:@me` query + cache + dedup; the "Recently reviewed" labeled subsection. Delivers SIGNL-01/02/03, CHECK-01/02, REVWD-01/02/03/04.

Out of scope (other phases / deferred): in-app review actions, time-windowed/capped reviewed list, richer PR card fields, cross-project review surfaces.

</domain>

<decisions>
## Implementation Decisions

### CI status icons (CHECK-01/02)
- **D-01:** Replace the current colored CI dot in `PRCard.tsx` with **bare lucide glyph icons** (no enclosing ring): pass → `Check` in green (`text-green-500`), fail → `X` in red (`text-red-500`), running/pending → a thin `Circle` outline in orange/amber (`text-amber-500`). Static — **no spinner** for running.
- **D-02:** `none` (no checks) renders **nothing** — no icon, no reserved gutter, no layout shift. Preserves the existing dotless behavior (the `pr.checks !== "none"` conditional render stays; only the rendered element changes from dot → icon).
- **D-03:** Icon size ~14px (`size-3.5`), keeping a tooltip ("Checks passing/failing/running") like the current dot. The server-side `checks` reduction (`pass|fail|pending|none` from `reduceChecks`) is unchanged — this is a pure frontend swap of the rendered element.

### Agent-state on PR cards — left rail, NOT a dot (SIGNL-01/02/03)

> **Checkpoint redesign (2026-06-17, human-verify gate):** the original design put the reused `StatusDot` (a filled dot) next to the line-art CI glyph. At the gate the user found the dot+glyph pairing visually clashing (different styles, slight misalignment). Chosen fix: **drop the dot; the left rail carries the full agent state via its color.** The two signals now live on fully separate channels — left edge = your agent, right glyph = the PR's CI. D-04/D-06/D-07 below are the redesigned decisions; they supersede the dot.

- **D-04:** A PR card shows agent state as a colored **left rail**, not a `StatusDot`. Rail color mirrors `dotMeta`'s palette: working `bg-green-500`, waiting `bg-amber-400` (the rail **pulses** — the old dot's `animate-pulse` moved here), idle `bg-blue-500/60`, exited `bg-zinc-600`. Any rail = the PR has a linked review session (an agent-status entry for its `source='github_pr'` task); no entry → no rail, no gutter, identical to a plain card. State is exposed accessibly via the card's `aria-label`/`title` (the rail is `aria-hidden`).
- **D-05:** The rail applies to PR cards in **both** the awaiting-review and Recently-reviewed sections (SIGNL-03) — it is a property of the `PRCard`, so both sections get it for free.
- **D-06:** **Always-on** rail for any open session *regardless of agent state*, so open-session PRs stand out at a glance even when the agent is idle/working. This intentionally **diverges from the task-card D-44 rule** (which tints the border amber only while waiting); here the rail is the sole agent signal and encodes the full state by color.
- **D-07:** Implemented as a 3px `absolute inset-y-0 left-0` span on a `relative overflow-hidden` card (a positioned rail, not a CSS border) so that **only the rail pulses** when waiting (`animate-pulse motion-reduce:animate-none`), never the whole card. "waiting = amber" is preserved; the rail amber is `amber-400` while the CI 'running' glyph is `amber-500` (a half-step deeper) so the two ambers never read as one.
- **D-08:** The rail keys on presence of the linked agent entry and applies in both sections (a property of `PRCard`).

### Recently Reviewed section (REVWD-01/02/03/04)
- **D-09:** A **"Recently reviewed" labeled subheader/divider** rendered **below** the awaiting-review list, inside the **same** scroll area of the existing Review column — **not** independently collapsible (the whole column already collapses as one). Cards are the same `PRCard` (click to open/reattach the review).
- **D-10:** Order: **most-recently-updated first** (reuse the existing `UpdatedAt` lexical sort). Drafts **excluded**. **No cap / no time window** for v1 (bounded naturally by open state — leaves on merge/close).
- **D-11:** The section is **quietly omitted** when it holds no PRs (no blank subheader, no fabricated count) — same quiet-empty discipline as the column today (D-07/D-14 of Phase 11). The column header count stays the **awaiting-review** count (unchanged); the Recently-reviewed subheader MAY carry its own small count.

### Data + wiring (REVWD-01/03, SIGNL-01) — "extend existing surfaces"
- **D-12:** **Reviewed query:** add a second `gh pr list -R <repo> --search "reviewed-by:@me draft:false" --state open --json <same field set>` to `internal/github` (clone of the existing `user-review-requested:@me` path in `prlist.go` — same `prRaw`/`PRSummary` shape, same `reduceChecks`, same degrade classification). Run it in the **same** per-repo TTL cache `Service` refresh cycle as the existing call (one cached gh cycle, not a separate poll).
- **D-13:** **Endpoint shape:** **extend** the existing always-200 `GET /api/projects/{id}/pull-requests[?refresh=1]` response with a **second array** (e.g. `reviewed: PRSummary[] | null`) alongside `prs`, sharing the same `state`/`stale`/`fetchedAt`. Do **not** add a new endpoint. `usePullRequests` consumes both lists from one query.
- **D-14:** **Single-section dedup (REVWD-03):** done **server-side** — any PR number present in the awaiting-review list (`prs`) is **removed** from the `reviewed` list before returning (top precedence). A re-requested PR therefore reappears in the top section and never duplicates below.
- **D-15:** **Open-session detection (SIGNL-01):** **extend the shared `/api/agents/status` entries** with `pr_number` (nullable) and `source`, so `AgentStatusEntry` carries enough to map a PR → its session. `PRCard` reuses the **same `useAgentStatuses()` 5s shared poll** that task cards use and finds its entry via `data.find(e => e.projectId === projectId && e.prNumber === pr.number)`. No new endpoint, no second poll loop. ("Open session" = an agent-status entry exists for the PR's linked `source='github_pr'` task; reuses the same presence test task cards use, including the `exited` state.)

### Claude's Discretion
- Exact Tailwind class values for the icons and the blue/amber left border (intent is locked in D-01/D-07).
- Card row element ordering (redesigned): `[title flex-1] · [CI icon if checks≠none] · [↗ external link]` — the agent dot was removed; agent state is the left rail (D-04). Matches the redesign preview `┃ title  ✓ ↗`.
- The exact JSON field name for the reviewed array (`reviewed` suggested) and the agent-entry field names (`prNumber`/`source` suggested).
- Whether the Recently-reviewed subheader shows its own count.
- **Lint cleanup (non-blocking):** the two carried `Date.now()`-in-render advisories live in exactly the touched files (`PRCard.tsx`/`ReviewColumn.tsx`) — clean them up while here if cheap; do NOT let them block the phase (gating build is already green).

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

No external specs/ADRs — this project keeps its source of truth in `.planning/` and the existing code. The authoritative references for this phase:

### Requirements & scope
- `.planning/REQUIREMENTS.md` — the 9 REQ-IDs delivered here (SIGNL-01/02/03, CHECK-01/02, REVWD-01/02/03/04), plus Out-of-Scope and Future (REVWD-FUT-*, GHCARD-*)
- `.planning/ROADMAP.md` (Phase 16 "Sharper Review Column" section) — goal, 5 success criteria, scope notes
- `.planning/PROJECT.md` — Current Milestone v1.5 block (milestone-time locked decisions) + Key Decisions (D-44 waiting border, StatusDot palette, the Phase 11–13 review-column/cache/reaper history)

### Existing code that defines the patterns to replicate (see code_context for how)
- `internal/github/prlist.go`, `internal/github/service.go` — the gh list path + reducer + per-repo TTL cache to clone/extend
- `web/src/components/board/PRCard.tsx`, `web/src/components/board/ReviewColumn.tsx` — the cards/column to modify
- `web/src/components/StatusDot.tsx`, `web/src/components/board/TaskCard.tsx` — the dot + D-44 border language to reuse/diverge from
- `web/src/api/agents.ts`, `web/src/api/pullRequests.ts` — the hooks/types to extend

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- **`StatusDot` / `dotMeta`** (`web/src/components/StatusDot.tsx`): reused verbatim for the PR agent dot — takes a `Pick<AgentStatusEntry,'status'|'exitCode'|'stopRequested'>`. No changes needed to the component.
- **`useAgentStatuses()`** (`web/src/api/agents.ts`): the single shared `["agent-statuses"]` query (`GET /api/agents/status`, 5s poll) that already drives task-card dots; PRCard reuses it. `AgentStatusEntry` must gain `prNumber`/`source` (D-15).
- **`internal/github` list path** (`prlist.go` `runGH`/`reduceChecks`/`prRaw`, `service.go` TTL cache `Service`): the `reviewed-by:@me` query is a near-clone in the same `Service` (D-12).
- **`usePullRequests` / `PRSummary` / `PullRequestsResponse`** (`web/src/api/pullRequests.ts`): extend the response with the `reviewed` array (D-13); `PRCard` already takes a `PRSummary`.
- **`useOpenReview`** (`web/src/api/pullRequests.ts`): the `source='github_pr'` task↔PR open/reattach link — Recently-reviewed cards reuse it unchanged.
- **Reaper merge/close detection** (Phase 13, `internal/github` `PRState` + reaper): already removes merged/closed PR worktrees; the `--state open` reviewed query means merged/closed PRs naturally drop out of the list (REVWD-02), no new reaper work required.

### Established Patterns
- **Dotless-card rule:** task cards and PR cards never reserve a gutter when there's no dot/icon (no layout shift) — CHECK-02 and the no-session case must preserve this.
- **D-44 waiting border** (`TaskCard.tsx`): `entry?.status === "waiting" ? "border-amber-400/40" : "border-border"`. Phase 16 **intentionally diverges** for PR cards (D-06/D-07: always-on blue session border, amber when waiting).
- **Always-200 / degrade-don't-break** (`prlist.go`, `usePullRequests`): the reviewed query rides the same classified `state`/`stale` contract; a gh failure degrades both lists together, never throws.
- **Server-side check reduction (D-00d):** raw `statusCheckRollup` never reaches the browser; `checks` stays the 4-value reduction — CI icons are a pure frontend render swap.

### Integration Points
- `internal/github`: new `reviewed-by:@me` runner + the `Service` refresh doing both gh calls per cycle + the server-side dedup.
- The `/api/projects/{id}/pull-requests` handler: assemble `{prs, reviewed, state, stale, fetchedAt}`.
- The `/api/agents/status` handler/payload: add `pr_number`/`source` to each entry (join from the `tasks` row).
- `PRCard.tsx`: agent dot (via `useAgentStatuses`), CI icon swap, blue/amber left border.
- `ReviewColumn.tsx`: render the `reviewed` list under a "Recently reviewed" subheader; keep the existing loading/empty/degraded states.

</code_context>

<specifics>
## Specific Ideas

- Card visual target (from the milestone preview): `┃ Fix flaky CI test   ● ✓ ↗` / `  Other PR   ✓ ↗` — left bar = open-session border, `●` = agent dot, `✓` = CI icon.
- "Bare check / cross" was explicitly chosen over circle-badge icons and over a spinning running indicator — keep CI lightweight; running is a static orange circle outline.
- The blue-for-session / amber-for-waiting split was explicitly chosen so "which PRs am I working on" (blue) stays distinct from "which need my input" (amber).

</specifics>

<deferred>
## Deferred Ideas

- Time window or count cap on Recently Reviewed (REVWD-FUT-01) — v1.5 bounds by open state only.
- Review-decision badge (approved vs changes-requested) on reviewed cards (REVWD-FUT-02).
- Richer PR card fields — diff size, fork pill, head→base line, labels (GHCARD-01..04).
- Independent collapse for the Recently-reviewed subsection — considered, deferred (the whole column collapses as one for v1).

</deferred>

---

*Phase: 16-sharper-review-column*
*Context gathered: 2026-06-17*

---
status: complete
phase: 17-hardening-e2e
source: [17-04-PLAN.md, 17-UI-SPEC.md §UAT Walkthrough Copy Contract, 17-CONTEXT.md D-59..D-61]
started: 2026-08-28T11:55:00Z
updated: 2026-08-28T16:20:00Z
---

## Current Test

[testing complete]

## Preamble — read before running

This is the SC5 walkthrough: the folder-mode Scratchpad driven end-to-end in a
browser against a real repo checkout, plus the restart-resume proofs no
automation can judge (D-53's "it remembers"), the interlock and sentinel-leak
sanity rows, and the D-12 settlement (D-61). Run the app the way you normally
do (installed binary or `go run ./cmd/kamacu serve`) and open it in the
browser at the served localhost URL.

**Already verified live — do NOT re-verify (16-UAT tests 2–4):** the bar
row + click-through + highlight, the `/global` interactive session parity
(`Start agent` / `Stop` / bash tabs / concurrent 409), the 7-state view
matrix, and the Settings Scratchpad flows (instant-save Select, Change-root
dialog, 409 error box). Phase 17 replays the *lifecycle gates* through those
surfaces; the Phase-16 UX itself is settled.

**Already machine-proven at API level (17-01/17-04 E2E), quoted here only as
reference strings:** the refusal edges `no global claude session to resume`
and `no global opencode session to resume` (the D-54 clear/stale-id
edges) and the concurrent-spawn 409 `global agent already running` are
asserted over real HTTP by `TestE2EGlobalReconfigureGate`,
`TestE2EGlobalRestartResume` and `TestE2EGlobalOpencodeRestartResume`. The
browser half of those gates surfaces the same wire strings inline; you
should not need to force them.

## Safety posture (D-60 — read this one seriously)

The Scratchpad agent runs **directly in the checkout you pick** — no
worktree, no branch, no safety net. Concretely: a
`--dangerously-skip-permissions` agent (the claude default since v1.10) can
write, stage, and commit straight into that repo, and none of the task-mode
protections exist here — **no worktree net** to discard, **no diff tab** to
review what it did, **no gated cleanup** to undo it. If you point the
Scratchpad at a repo you care about (e.g. the kamacu checkout itself,
dogfooding), you are one prompt away from an agent editing your working
tree. If that gives you pause, clone the repo somewhere disposable
(`git clone <repo> /tmp/scratchpad-uat`) and run this walkthrough against
the clone — same flows, zero risk. This is a suggestion, not a requirement.

## Tests

### 1. Folder-mode lifecycle against a real repo (SC5 / D-59)

expected: Settings → Scratchpad → `Configure root…` with your picked repo folder (the root summary shows the mono path; folder roots show the persistent amber banner, repo roots do not) → on `/global` the agent tab shows the `Start agent` CTA → clicking it streams a live agent terminal in the root; the trailing `+` spawns a bash tab labeled `Bash <n>`; navigating away and back reattaches both with prior scrollback (bash tab: invisible reattach, label byte-for-byte, no resume affordance); the agent's ⋯ menu renders exactly `Stop` (destructive) and stopping works; a second `Start agent` while live surfaces `global agent already running` inline; finally, with everything stopped, Clear root returns the summary to `No root configured yet.` with the button flipped back to `Configure root…` — reconfigure before test 2
result: pass

### 2. Claude same-conversation resume across a restart (D-53 / SC2 claude leg)

expected: with the default Claude agent live in the Scratchpad, ask it something identifiable (e.g. "remember the passphrase is blue-kangent-17", get its reply); stop the serve process (Ctrl-C/kill — hard death is the restart semantic) and start it again on the same DB; the bar's `Global · Scratchpad` row is absent until resume (the bar's live filter drops the DB-derived exited entry — locked, not a bug); on `/global` the agent tab shows the heading `Agent session ended.` with the `Reset session` / `Resume session` pair; clicking `Resume session` shows `Resuming…` then streams the SAME conversation, and the agent remembers the identifiable thing when asked (this memory judgment is the point of the flow — a fresh terminal that forgot it is a fail)
result: pass

### 3. Opencode same-conversation resume across a restart (D-53 / SC2 opencode leg)

expected: Settings → Scratchpad → default-agent Select set to OpenCode (instant-save, applies at next Start); same flow as test 2 with the opencode agent — identifiable thing, stop/restart the serve process, heading `Agent session ended.`, `Resume session` → `Resuming…` → same conversation, agent remembers (the engine-branched `-s <session>` resume path proven at API level by 17-04's E2E; this is its browser half)
result: pass

### 4. Interlock sanity — project delete keeps the managed Scratchpad root (SC3 / D-58)

expected: with a managed project clone AND a managed Scratchpad root of the SAME repo (`owner/name` in both places), delete the project from the board → the Settings Scratchpad root row still shows the mono `owner/name` + `managed` badge unchanged, `/global` still loads and its terminals still work; then clearing the Scratchpad root (all stopped) leaves the project-side clone alone in the other direction
result: pass

### 5. Sentinel-leak visual sanity — the global appears nowhere it shouldn't (SC4 / D-55)

expected: with a live global agent + live global bash tab: kanban columns show no foreign card, the project sidebar lists no new project, an empty workspace still deletes, Activity lists/stats show no `Scratchpad`/`Global` rows; the global appears ONLY on the Active Sessions bar row (`Global · Scratchpad`), `/global`, Settings → Scratchpad, and — if you have an MCP client attached — the MCP session tools listing it with its honest `Scratchpad`/`Global` labels
result: pass

### 6. D-12 bar-row settlement with the live bar (D-61)

expected: with a task row AND the global row live simultaneously in the expanded Active Sessions bar, confirm the text-only `Global · Scratchpad` row is distinguishable from a task's project · task pair (scope slot says `Global`, name slot says `Scratchpad`); record the keep-or-flip decision below — the default is KEEP (no globe badge, no icon, no tint; any flip is future work riding existing Badge/lucide idioms, never a new hue)
result: pass
decision: KEEP (settled with the live bar — text-only `Global · Scratchpad` row distinguishable as-is)

## Summary

total: 6
passed: 6
issues: 0
pending: 0
skipped: 0
blocked: 0

## Gaps

(none recorded yet — append any flow that could not be executed as-is, with the reason)

# Phase 13: Global data foundation & safety net - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-08-25
**Phase:** 13-Global data foundation & safety net
**Areas discussed:** Managed-root namespace, Folder-root validation, UI naming (Global vs Scratchpad), "Live" definition for the reconfigure gate

---

## Managed-root namespace

| Option | Description | Selected |
|--------|-------------|----------|
| Separate namespace | Own namespace — structurally unrepresentable collision with project gated deletes (P8) | ✓ |
| Shared repos/ + dedup 409 | One clone on disk if same repo is both project + global; cross-entity 409 guards on every create/delete path | |

| Option | Description | Selected |
|--------|-------------|----------|
| repos/global/<owner>/<name> | Inside the existing repos/ tree; 'global' as reserved pseudo-owner | ✓ |
| ~/.kamacu/global/repo | Dedicated top-level dir for everything global | |

| Option | Description | Selected |
|--------|-------------|----------|
| Allow both | Separate entities by design; zero extra guard code | ✓ |
| Allow + warn | Configure succeeds; inline note in Settings | |
| Refuse with 409 | Forces a different repo or removing the project | |

| Option | Description | Selected |
|--------|-------------|----------|
| Keep on disk | Reconfiguring never deletes; same-repo reconfigure reattaches free | ✓ |
| Gated removal on switch | Managed-project delete gates run against the old clone | |

**User's choice:** Separate namespace at `~/.kamacu/repos/global/<owner>/<name>`; same-repo overlap allowed with no guard; old clones kept on disk forever (reattach stays free).
**Notes:** Resolves the STACK-vs-PITFALLS researcher disagreement in favor of PITFALLS P8's "structurally safe beats guarded".

---

## Folder-root validation

| Option | Description | Selected |
|--------|-------------|----------|
| Git repo required | Reuses validateRepoPath semantics; git = recovery story for a skip-permissions agent | ✓ |
| Any dir + non-git warning | Most permissive; warning carries the whole safety posture | |

| Option | Description | Selected |
|--------|-------------|----------|
| Persistent banner | One-line notice in the global view for folder-mode roots (P3 safety element) | ✓ |
| Configure-time only | Warn once in Settings, nothing in the view | |
| No warning | Trust the explicit choice | |

| Option | Description | Selected |
|--------|-------------|----------|
| Block $HOME and / | Clear 400 at configure time; one equality check vs the P3 worst case | ✓ |
| Trust the user | Any existing git repo passes | |

| Option | Description | Selected |
|--------|-------------|----------|
| Spawn-time honesty | Validate once at PUT; honest spawn error if dir vanishes later | ✓ |
| Boot-time revalidation | Startup marks config degraded if the dir is gone | |

**User's choice:** Git repo required; persistent un-isolation banner in the view; `$HOME`/`/` blocked; no boot-time revalidation.
**Notes:** The banner is deliberately distinct from the deselected GT-FUT-01/02 header lines — it's the P3 mitigation, not decoration.

---

## UI naming (Global vs Scratchpad)

| Option | Description | Selected |
|--------|-------------|----------|
| Scratchpad | Avoids v1.12 Activity 'Global' scope-label collision; internals untouched | ✓ |
| Global Task | Matches milestone/requirements language; accepts the mild collision | |

| Option | Description | Selected |
|--------|-------------|----------|
| Global · Scratchpad | projectName="Global" (scope), taskTitle="Scratchpad" (name) | ✓ |
| Scratchpad · Scratchpad | Both slots carry the name; reads redundant | |
| Kamacu · Scratchpad | Brands the row as app-level | |

| Option | Description | Selected |
|--------|-------------|----------|
| Keep 'global' internals | scope:'global', /global route, global_task table, kamacu-global-* | ✓ |
| Rename internals too | One term everywhere but churns the locked architecture | |

| Option | Description | Selected |
|--------|-------------|----------|
| Settle at UAT | Strings locked now; visuals deferred to Phase 17 per research | ✓ |
| Globe badge now | Decide the visual treatment in advance | |
| Text-only now | Decide the visual treatment in advance | |

**User's choice:** "Scratchpad" user-facing; synthesized strings projectName:"Global" / taskTitle:"Scratchpad"; internals stay 'global'; bar visuals settle at UAT.
**Notes:** Resolves the research-gap naming collision; label strings start emitting in Phase 15, so this had to be decided once, early.

---

## "Live" definition for the reconfigure gate

| Option | Description | Selected |
|--------|-------------|----------|
| Any global session | Agent + plain-bash + tmux tabs all block (ListGlobal() nonempty) | ✓ |
| Agent only | Bash/tmux tabs keep running with a stale cwd | |

| Option | Description | Selected |
|--------|-------------|----------|
| Don't block | Only live processes gate; resume ids cleared on success instead | ✓ |
| Block while resumable | Exited-but-resumable agent also gates | |

| Option | Description | Selected |
|--------|-------------|----------|
| Reasons list | {error, reasons:[...]} — mirrors the gated-delete 409 grammar app-wide | ✓ |
| Flat message | Single-sentence 409, no enumeration | |

| Option | Description | Selected |
|--------|-------------|----------|
| Clear on any change | Uniform GCONF-04 contract; no no-op-re-PUT exception | ✓ |
| Skip no-op re-PUTs | Same-root re-PUT preserves ids | |

**User's choice:** Any live global PTY blocks; exited/resumable never blocks; 409 reasons list; resume ids clear on any successful change.
**Notes:** Locks the semantics now; the gate itself enforces in Phase 14 (GCONF-04) and is E2E-verified in Phase 17.

---

## the agent's Discretion

- Migration split shape (one combined vs 00017+00018) — plan-phase detail per research.
- Scope-discriminator column spelling and singleton resume-id column granularity.
- Sweep known-set query rewrite shape (LEFT JOIN vs UNION).

## Deferred Ideas

- Bar-row visual treatment (globe badge vs text-only) — settles at UAT in Phase 17.
- GT-FUT-01..07 remain parked in REQUIREMENTS.md.

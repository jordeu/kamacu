# Phase 20: Kamacu Rebrand & Brand - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-07-01
**Phase:** 20-kamacu-rebrand-brand
**Areas discussed:** Logo & mark, Sidebar brand lockup, README, Rename boundary calls

---

## Logo & mark

### Q1 — Logo form
| Option | Description | Selected |
|--------|-------------|----------|
| 'K' monogram mark | Stylized 'K' tile echoing the app's rounded monogram project avatars | |
| Abstract symbol | A distinct icon representing the product/concept | ✓ |
| Wordmark only | "Kamacu" in a typeface, no separate mark | |

### Q2 — What the symbol represents
| Option | Description | Selected |
|--------|-------------|----------|
| A creature/animal | Stylized animal/mascot mark (octocat / Rust-crab style) | ✓ |
| The workflow/agents | Abstract mark evoking parallel agents / board / worktrees | |
| Pure geometric | Non-representational geometric glyph, no literal meaning | |

### Q3 — Which creature
| Option | Description | Selected |
|--------|-------------|----------|
| You pick / suggest | Claude proposes creature options to choose from | |
| Let name inspire it | Claude interprets the name "Kamacu" into a creature/mood | ✓ |

### Q4 — Creature direction to lock (name-inspired)
| Option | Description | Selected |
|--------|-------------|----------|
| Andean camelid | Alpaca/vicuña; herd = parallel agents; ties to Quechua *kamaq* | |
| Nocturnal watcher | Owl/fox; "always-on" agent watching your tasks | |
| Abstract spark/creature | *kamaq* = animating force → a small glowing spark/wisp | ✓ |

### Q5 — Mascot or pure element
| Option | Description | Selected |
|--------|-------------|----------|
| Hint of life | Subtle character (tiny eyes / gaze) — a little being | |
| Pure element | Purely elemental spark/wisp/comet — no face | ✓ |

### Q6 — Color treatment
| Option | Description | Selected |
|--------|-------------|----------|
| Single brand accent | One distinctive glow, deliberately NOT amber | |
| Warm ember | Warm amber/orange ember glow | ✓ |
| Monochrome + glow | White/near-white spark with luminous halo | |

**User's choice:** Abstract, pure-elemental **warm-ember spark/wisp** mark embodying *kamaq* (the animating life-force).
**Notes:** Claude flagged that warm ember overlaps the app's amber "agent waiting" status dot; recorded as a hard design constraint (D-04) — the mark must be a fuller multi-tone glow/shape, never a plain dot.

---

## Sidebar brand lockup

### Q1 — Expanded header
| Option | Description | Selected |
|--------|-------------|----------|
| Mark + wordmark | Ember spark mark next to "Kamacu" text | ✓ |
| Wordmark only | Just "Kamacu" text, no mark | |
| Mark only | Just the spark mark, no text | |

### Q2 — Collapsed icon rail
| Option | Description | Selected |
|--------|-------------|----------|
| Spark mark | Compact ember spark at top of the rail, above avatars | ✓ |
| Keep empty | Leave as today — only the toggle | |

**User's choice:** Expanded = mark + "Kamacu" wordmark (capital K); collapsed rail = spark mark at top.
**Notes:** Replaces the current lowercase text `kangent` at `ProjectSidebar.tsx:49`, which is hidden when collapsed.

---

## README

### Q1 — Audience
| Option | Description | Selected |
|--------|-------------|----------|
| Shareable open-source | Written so a stranger could discover, build, run it | ✓ |
| Personal / dev-facing | Concise personal reference, minimal onboarding | |

### Q2 — Depth / visual
| Option | Description | Selected |
|--------|-------------|----------|
| Concise + screenshot | Tight intro + one screenshot/GIF + build/run + workflow | ✓ |
| Fuller docs | Adds architecture, stack rationale, worktree/session model | |
| Text-only minimal | No screenshots, prose + code blocks only | |

**User's choice:** Shareable open-source framing; concise with one screenshot/GIF of board + task terminal.
**Notes:** Screenshot must be captured after the rebrand lands so it shows Kamacu branding (execution note D-12).

---

## Rename boundary calls

### Q1 — Thoroughness
| Option | Description | Selected |
|--------|-------------|----------|
| Full sweep | Also rename cmd/kangent/ dir, comments, log wording, identifiers | ✓ |
| Minimal | Only module/binary/imports/UI; leave dir/comments/logs | |

### Q2 — What stays for Phase 21
| Option | Description | Selected |
|--------|-------------|----------|
| Keep all runtime-adjacent | Keep ~/.kangent paths, -L kangent socket, session prefix, localStorage keys, AND kangent-tmux.conf filename | ✓ |
| Also rename tmux-conf file | Rename kangent-tmux.conf now, keep the rest | |

**User's choice:** Full cosmetic sweep; keep ALL runtime-adjacent strings (paths, socket, prefix, localStorage, tmux-conf filename) for Phase 21.
**Notes:** Reconciliation recorded (D-15): rename the product-name word in log/comment prose but never a string literal that is/builds a real `~/.kangent` path/socket/key — tests assert `.kangent/repos`, so `go test ./...` must stay green.

---

## Claude's Discretion

- Exact spark geometry/animation, stroke vs fill, precise ember hue and glow radius (within D-01..D-05).
- README section ordering and prose.
- Whether the sidebar mark and favicon share one SVG source or two optimized variants.

## Deferred Ideas

- BRAND-FUT-01 — light/dark or animated logo variants (parked in REQUIREMENTS.md § Future).
- Renaming the GitHub repo/remote — explicitly Out of Scope.

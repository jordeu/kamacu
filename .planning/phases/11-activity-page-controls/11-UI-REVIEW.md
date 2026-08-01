# Phase 11 — UI Review

**Audited:** 2026-08-01
**Baseline:** `11-UI-SPEC.md` (approved design contract) + abstract 6-pillar standards
**Screenshots:** not captured (code-only audit)

> **Screenshot note:** `playwright` is not a project dependency (downloaded on-demand via `npx`, requires `npx playwright install` for browsers). The live servers on `:7333`/`:7334` are production Go binaries serving the embedded SPA shell, not the Vite dev server the CLI screenshot approach targets (`:3000`/`:5173`/`:8080`). Audit is therefore code-only against the UI-SPEC. `11-UAT.md` records 7/7 human-verified browser tests passing on `:7334`, so the rendered result is independently confirmed — this review scores the *code* against the *contract*.

---

## Pillar Scores

| Pillar | Score | Key Finding |
|--------|-------|-------------|
| 1. Copywriting | 4/4 | Every literal matches the UI-SPEC copy contract verbatim, all as grep-able template literals |
| 2. Visuals | 2/4 | Reviews entries omit the spec-mandated `ExternalLink` affordance icon; entry titles render muted instead of "primary" |
| 3. Color | 4/4 | Textbook monochrome execution — zero primary/accent/destructive in the activity components; accent correctly reserved for sidebar entry + controls |
| 4. Typography | 2/4 | `text-2xl font-semibold` on the StatsStrip count numbers breaks the locked 4-size / 2-weight scale — only such usage in the entire app |
| 5. Spacing | 3/4 | Core scale (gap-1/2/3/6, p-4, mt-6) and the `max-w-[640px]` SettingsPage shell are verbatim; two undeclared arbitrary width values |
| 6. Experience Design | 4/4 | Full state coverage (loading skeletons, error+retry, empty/silent degrade), 3-layer null defense, dual-state sidebar reachability, no toasts |

**Overall: 19/24**

---

## Top 3 Priority Fixes

1. **Reviews-done entries have no external-link affordance** (`web/src/components/activity/ReviewsList.tsx:83-100`) — users can't visually distinguish an external-open entry from an internal-navigate (task) entry. The UI-SPEC (line 119) mandates an `ExternalLink` icon (`size-3.5 text-muted-foreground hover:text-foreground`) mirroring `PRCard.tsx:175`, and the plan omitted it. **Fix:** add `<ExternalLink className="size-3.5 text-muted-foreground hover:text-foreground" />` inside the existing `<a>` (after the title span), reusing the already-imported `ExternalLink` from `lucide-react`.

2. **StatsStrip count numbers use `text-2xl font-semibold`** (`web/src/components/activity/StatsStrip.tsx:43,49`) — this is the **only** `text-2xl` and the **only** `font-semibold` in the entire app, violating the UI-SPEC's locked "exactly four sizes (12/14/16/mono) and two weights (400/500)" (Typography §55, §69) and contradicting the offered count-label styling (`text-sm font-medium`, §114). It also fights the page's stated "quiet read-out" intent — the counts become the loudest element. **Fix:** downgrade to `text-sm font-medium tabular-nums` (UI-SPEC option 1), OR amend the UI-SPEC to bless the hero-number treatment if the team genuinely wants louder stats.

3. **Activity entry titles render `text-muted-foreground`** (`web/src/components/activity/ActivityList.tsx:69`, `web/src/components/activity/ReviewsList.tsx:92`) — the UI-SPEC entry-copy contract (lines 118–119) specifies the title as "primary" (`text-sm`) with only the time caption muted. The muted-until-hover treatment flattens the title/caption hierarchy and makes both lists harder to scan. **Fix:** render titles in default foreground (`text-sm`) and keep only the time caption `text-muted-foreground` — restoring the spec's two-tier row rhythm — OR, if the documented "quiet read-out" intent wins, update the UI-SPEC entry-copy rows to bless muted titles so the contract matches the code.

---

## Detailed Findings

### Pillar 1: Copywriting (4/4)

Every UI string is a grep-able template literal (Phase-3 convention restated in UI-SPEC §129) and matches the copy contract verbatim:

| Spec element | Spec copy | Implementation | Verdict |
|---|---|---|---|
| Page heading (§106) | `Activity` | `ActivityPage.tsx:41` `` `Activity` `` | ✓ |
| Stat section heading (§108) | `Statistics` | `StatsStrip.tsx:40` `` `Statistics` `` | ✓ |
| Tasks section heading (§109) | `Tasks completed` | `ActivityList.tsx:53` `` `Tasks completed` `` | ✓ |
| Reviews section heading (§110) | `Reviews completed` | `ReviewsList.tsx:76` `` `Reviews completed` `` | ✓ |
| Stat row labels (§111) | `Cycle` / `In progress` / `In review` | `StatsStrip.tsx:56-58` | ✓ |
| Stat row format (§112) | `min · median · max` then `over N tasks` | `StatsStrip.tsx:26,31` | ✓ |
| Count nouns (§114) | `tasks` / `reviews` | `StatsStrip.tsx:46,52` | ✓ |
| Scope trigger aria-label (§115) | `Scope: {name}` | `ScopeSelector.tsx:64` | ✓ |
| Scope sections (§116) | `Global` / `Workspaces` / `Projects` + `All activity` | `ScopeSelector.tsx:75,80,84,96` | ✓ |
| Window labels (§117) | `Week` / `Month` | `WindowToggle.tsx:27-28` | ✓ |
| Tasks-done entry (§118) | `{title}` + relative time | `ActivityList.tsx:70,73` | ✓ (text — see P2 for color) |
| Reviews-done entry (§119) | `#{number} {title}` + aria-label `Open PR #{number} on GitHub` | `ReviewsList.tsx:89,93` | ✓ (text — see P2 for missing icon) |
| Empty tasks (§121) | `No tasks completed in this window.` | `ActivityList.tsx:55` | ✓ |
| Empty reviews hard-degrade (§122) | render NOTHING | `ReviewsList.tsx:57` returns `null` | ✓ |
| Transport error (§124) | `Couldn't load activity.` + `Retry loading` | `ActivityPage.tsx:30,32` | ✓ |
| No primary CTA (§105) | none | (none) | ✓ |
| No toasts (§128) | inline/muted only | (no sonner) | ✓ |

No generic labels (`Submit`/`OK`/`Cancel`/`Save`/`Click Here`) found anywhere in the activity surface. The `over 0 tasks` caption renders on `n===0` em-dash rows (`StatsStrip.tsx:30-32`) — explicitly permitted by UI-SPEC §113/D-11 ("may render or be suppressed — agent picks"). Contract is met in full.

### Pillar 2: Visuals (2/4)

**Finding V-1 (WARNING) — Missing external-link affordance on reviews-done entries.**
UI-SPEC §119 explicitly requires: *"External-link affordance: an `ExternalLink` icon (`size-3.5 text-muted-foreground hover:text-foreground`) mirroring `PRCard.tsx:166-176`."* `ReviewsList.tsx:83-100` renders no such icon — the entry is just `#{number} {title}` + time. The established app idiom is `PRCard.tsx:175` (`<ExternalLink className="size-3.5" />`). Impact: a reviews-done row looks identical to a tasks-done row, but the former opens an **external** GitHub tab while the latter navigates **internally**. With no visual cue, users discover the behavior only after clicking. The `aria-label` + `target="_blank"` provide programmatic signals but no scan-time affordance. The plan (11-02-PLAN §183) omitted the icon; the UI-SPEC did not.

**Finding V-2 (WARNING) — Entry titles render `text-muted-foreground` instead of "primary".**
UI-SPEC §118/§119 specify the entry title as "primary" (`text-sm`) — the prominent row element — with only the time caption muted. The implementation renders the title in `text-muted-foreground` that brightens only on hover (`ActivityList.tsx:69`, `ReviewsList.tsx:92`). This flattens the two-tier row hierarchy the spec defines. The 11-02-SUMMARY documents this as a deliberate "quiet read-out" choice, so the *intent* is defensible — but the code diverges from the literal contract.

**Finding V-3 (positive) — Focal hierarchy is otherwise clean.** The `text-2xl` count lead numbers (P4 aside) do establish the stats as the page focal point; the `text-base` page heading, `text-xs uppercase` section headings, and `text-sm` body form a clear 4-step visual ladder. Icon-only buttons carry aria-labels (`ScopeSelector.tsx:64`, `ProjectSidebar.tsx:196`) and tooltips (`ProjectSidebar.tsx:201`); decorative glyphs are `aria-hidden` (`ScopeSelector.tsx:70`).

### Pillar 3: Color (4/4)

Textbook execution of the UI-SPEC monochrome contract. Class inventory across the activity components:

| Class | Count | Role per spec |
|---|---|---|
| `text-muted-foreground` | 17 | Captions, help copy, empty states, section subheadings (§83) — correct |
| `text-foreground` | 4 (all `hover:`) | Row-hover brighten — correct |
| `text-2xl` / `text-base` / `text-sm` / `text-xs` | 2 / 1 / 4 / 10 | Size tokens (see P4) |
| `bg-muted` | 1 | Tabs segmented-control track — correct |
| `text-primary` / `bg-primary` / `border-primary` | 0 | Reserved for inverted CTA — correctly UNUSED (§81: no primary CTA this phase) |
| `bg-sidebar-accent` / `text-sidebar-accent-foreground` | 0 in activity files | Used correctly in `ProjectSidebar.tsx:192-193` for the Activity entry active state |
| `text-destructive` / `text-amber-*` / `text-red-*` | 0 | No errors/destructive content on this read-only page — correct |

No hardcoded `#hex` or `rgb()` anywhere in the activity surface. The 60/30/10 distribution holds: `--background` dominant (page shell), `--card`/`--sidebar`/`--popover` secondary (sidebar + dropdown content), accent (`--sidebar-accent`/`--accent`/`--primary`) reserved to exactly the four declared surfaces (sidebar entry active, scope trigger hover, segmented-control pill, and the intentionally-unused primary CTA). No saturated color was introduced.

### Pillar 4: Typography (2/4)

**Finding T-1 (WARNING) — `text-2xl font-semibold` on StatsStrip count numbers violates the locked scale.**
`StatsStrip.tsx:43` and `:49` render `taskCount`/`reviewCount` as `<span className="text-2xl font-semibold tabular-nums">`. UI-SPEC §55 states: *"The app declares two weights only: regular (400) and medium (500). No font-semibold/font-bold appears in the page shells."* §69 states: *"This phase uses exactly four sizes (12 / 14 / 16 + mono) and two weights (400 / 500)."* §114 offered two count-label treatments: "`text-sm font-medium` for the number" OR "a single `text-sm` line." Neither is `text-2xl font-semibold`.

This is not an app-wide convention the phase is matching — a whole-`web/src` grep confirms these **two lines are the only `text-2xl` and the only `font-semibold` in the entire codebase**. The phase *introduces* both a 5th size (24px) and a 3rd weight (600) that the contract and the rest of the app do not use. It also works against the page's stated "quiet read-out" intent — the counts become the loudest element on the page.

**Finding T-2 (positive) — Everything else is exactly on-scale.** Page heading `text-base font-medium` (`ActivityPage.tsx:41`); section headings `text-xs font-medium tracking-wide uppercase` (the locked idiom, `StatsStrip.tsx:40` + both lists); body `text-sm`; captions `text-xs`; stat values + `#N` PR numbers in `font-mono` (`StatsStrip.tsx:23,25`, `ReviewsList.tsx:93`). Two weights (400 default + 500 `font-medium`) and `font-mono` family override are the only typography outside the body default — precisely the spec.

### Pillar 5: Spacing (3/4)

**Finding S-1 (positive) — Core scale and page shell are verbatim SettingsPage.**
The page shell `h-full overflow-y-auto p-4` + `max-w-[640px]` column + `mt-6 flex flex-col gap-6` section stack (`ActivityPage.tsx:39-52`) mirror `SettingsPage.tsx:140-148` exactly. Section-internal `flex flex-col gap-3` (`StatsStrip.tsx:39`, both lists) is the locked sm+ (12px) idiom. Frequency: `gap-2` ×8, `gap-3` ×6, `gap-6` ×3, `gap-1` ×3, `mt-6` ×2, `p-4` ×1 — **all multiples of 4** (4/8/12/16/24), all in the declared scale. No off-scale gap/padding tokens.

**Finding S-2 (WARNING) — Two undeclared arbitrary width values.** UI-SPEC §49 states "Exceptions: none," but the implementation adds:
- `min-w-[5.5rem]` (`StatsStrip.tsx:21`) — aligns the three TimeStat-row label columns so the `min · median · max` values line up vertically. Legitimate alignment purpose, but `5.5rem` is an undeclared arbitrary value.
- `max-w-[10rem]` (`ScopeSelector.tsx:67`) — truncates long workspace/project names in the trigger so the control bar width stays bounded. Legitimate, but undeclared.

Both are width/alignment constraints rather than gap/padding tokens, and both solve real layout problems — so the deviation is defensible. Flagged because the spec declares "no exceptions" and these are arbitrary `[bracket]` values not present in the locked scale. The page-column `max-w-[640px]` (`ActivityPage.tsx:40`) is explicitly spec-locked (§47) — not a finding.

### Pillar 6: Experience Design (4/4)

Full state coverage and interaction contract met:

| Contract | Implementation | Verdict |
|---|---|---|
| Loading state (UI-SPEC §140) | 4 Skeleton rows when `isLoading \|\| !data` (`ActivityPage.tsx:42-50`), mirroring SettingsPage | ✓ |
| Error + retry (UI-SPEC §124) | `isError` → muted "Couldn't load activity." + outline "Retry loading" button calling `refetch` (`ActivityPage.tsx:26-36`) | ✓ |
| Tasks empty (UI-SPEC §121) | `No tasks completed in this window.` single muted line, no icon/CTA (`ActivityList.tsx:55`) | ✓ |
| Reviews hard-degrade (UI-SPEC §122, ACT-04, D-12) | `HARD_DEGRADE.has(state) && prs.length===0` → `return null` — entire section suppressed, no heading/hint/toast (`ReviewsList.tsx:56-58`) | ✓ |
| Reviews ok+empty (UI-SPEC §123) | `groups.length===0` → `return null` (`ReviewsList.tsx:68-70`) | ✓ |
| Wire-contract null defense (gap-closure 11-03) | 3 layers: `groupByProject` accepts `T[] \| null \| undefined` (`ActivityList.tsx:32-36`); `ReviewsList` normalizes `prs = reviews.prs ?? []` (`ReviewsList.tsx:52`); `ActivityPage` guards both props (`ActivityPage.tsx:58-59`) | ✓ |
| External link mechanism (UI-SPEC §168, D-14) | Plain `<a href={url} target="_blank" rel="noreferrer">` — never `window.open`, never router `<Link>` (`ReviewsList.tsx:84-88`) | ✓ |
| Tasks internal nav (UI-SPEC §167, D-15) | `<Link to={`/projects/${task.projectId}/tasks/${task.id}`}>` (`ActivityList.tsx:64-66`) | ✓ |
| Sidebar dual-state reachability (UI-SPEC §162, D-02) | Footer's wholesale `group-data-[collapsible=icon]:hidden` removed and re-applied per-child; Activity button carries `group-data-[collapsible=icon]:size-8 rounded-md` instead (`ProjectSidebar.tsx:169-202`) | ✓ (UAT Test 1) |
| Active state (UI-SPEC §161, D-01) | `location.pathname === "/activity"` → `bg-sidebar-accent text-sidebar-accent-foreground` (`ProjectSidebar.tsx:192-193`) | ✓ |
| Scope/window persistence (UI-SPEC §163-166, D-05/D-06) | `useActivityView` validates scope against parseScope grammar, adopts active-workspace default on first open without overwriting saved scope (Pitfall 6 guard at `useActivityView.ts:72-79`) | ✓ |
| No `refetchInterval` (UI-SPEC §173) | query has no poll — occasional view, not live dashboard | ✓ |
| No toasts (UI-SPEC §128) | Degrade is silent; no sonner import | ✓ |

**Minor observation (not score-affecting):** `HARD_DEGRADE` includes `"error"` (`ReviewsList.tsx:38`), which UI-SPEC §169 lists in the *render* set (`ok`/`partial`/`error → render`). This is **dead code given the later `groups.length===0 → return null` guard** (`ReviewsList.tsx:68-70`) — the observable behavior is identical whether or not `error` is in the set, because error+empty suppresses either way. It is defensible as defense-in-depth (survives a future removal of the groups guard) but is a literal spec-vs-code membership mismatch worth a one-line comment or a spec amendment.

---

## Files Audited

- `web/src/pages/ActivityPage.tsx` (page shell, control bar, section composition)
- `web/src/components/activity/ScopeSelector.tsx` (Global/Workspaces/Projects dropdown)
- `web/src/components/activity/WindowToggle.tsx` (Week/Month segmented control)
- `web/src/components/activity/StatsStrip.tsx` (counts + 3 TimeStat rows, em-dash guard)
- `web/src/components/activity/ActivityList.tsx` (tasks-done grouped by project + `groupByProject` util)
- `web/src/components/activity/ReviewsList.tsx` (reviews-done, hard-degrade silence, external `<a>`)
- `web/src/components/sidebar/ProjectSidebar.tsx` (Activity sidebar entry — dual-state reachability)
- `web/src/lib/useActivityView.ts` (scope/window localStorage hook)
- `web/src/App.tsx` (`/activity` bare route registration)
- `web/src/pages/SettingsPage.tsx` (parity reference for the page-shell contract)
- `web/src/lib/time.ts` (`formatDuration` formatter — behavior verified in 11-01)
- `web/src/components/board/PRCard.tsx` (reference for the `ExternalLink` external-link idiom)

**Registry audit:** `web/components.json` present; UI-SPEC §Registry Safety declares "Third-party registries: None declared" and "No `npx shadcn view` / `npx shadcn add` invocations occur in this phase." → **0 third-party blocks checked, no flags.**

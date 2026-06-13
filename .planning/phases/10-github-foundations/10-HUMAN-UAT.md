---
status: partial
phase: 10-github-foundations
source: [10-VERIFICATION.md]
started: 2026-06-13T00:00:00.000Z
updated: 2026-06-13T00:00:00.000Z
---

## Current Test

[awaiting optional human click-through — phase verified passed in code]

## Tests

### 1. Live gh-gated toggle behavior (GHSET-01 / GHSET-03)
expected: With `gh` ABSENT (current host state) the /settings GitHub integration Switch shows OFF (never on, even against a stored "on" row); the help line reads `Install the GitHub CLI (gh) before enabling GitHub integration.`; clicking the Switch to enable does NOT turn it on and surfaces that install message. Restore `gh` on PATH and reload: the toggle behaves normally (reflects stored on/off, enable commits, help reverts to `Show GitHub features across Kangent. Turn off to hide all GitHub UI.`). No server restart needed (Available() is call-time).
result: [pending] — gate/guard/effective-state/copy/endpoint all verified in code; gh-absent degrade exercised live via TestGithubStatus. Only the browser click-through remains.

### 2. OFF cascade click-through (GHSET-02)
expected: Turn the GitHub integration Switch OFF on /settings; open a project's ⋯ → Project settings: the GitHub repository field is gone, Description remains; no Review column or other GitHub UI appears anywhere; board/tasks/sessions behave as before v1.3.
result: [pending] — gate verified in code (repo field is `{integrationOn && ...}`); whole-app visual assertion left for human.

### 3. Origin prefill on dialog open (GHPRJ-03 / D-08)
expected: Open Project settings for an unlinked project whose git origin is a GitHub remote: the repository field prefills with the canonicalized owner/name; for a non-GitHub or absent origin the field stays empty; the origin endpoint is hit only on open, never on the project list.
result: [pending] — endpoint + query verified in code and by TestGithubOriginSuggestion; live prefill UX left for human.

## Summary

total: 3
passed: 0
issues: 0
pending: 3
skipped: 0
blocked: 0

## Gaps

- **Gap 1 — gh-gated default + enable guard (GHSET-01/GHSET-03):** status: resolved — closed by gap-closure plans 10-04 (GET /api/github/status) + 10-05 (gh-aware toggle). Verified in code; degrade path exercised live (gh absent on host).
- **Gap 2 — toggle copy (GHSET-02):** status: resolved — closed by 10-05; help text is now exactly `Show GitHub features across Kangent. Turn off to hide all GitHub UI.` (over-claiming clause removed).

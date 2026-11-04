# PR work context

The existing terminal-monitor classifier emits `work_summary`, `work_evidence`,
`work_uncertainty` and `pr_links` alongside its attention decision. There is no
additional monitoring loop. Work summaries cover objective, progress, blockers,
decisions and next steps. The server validates excerpts against the stable snapshot’s current screen or retained
history before persisting. History retains its original line timestamps, so
rediscovery cannot make old evidence appear new. PR links require a full PR URL in the cited evidence;
the classifier distinguishes implementation/review from incidental references.
Terminal claims are observations, never verification of GitHub status.

Migration 35 backfills review workspaces and adds many-to-many task links,
session links and durable session summaries. Repository identity is lowercase
`owner/repo` on github.com, paired with the PR number and rendered as a full URL.
New review workspaces also record their explicit association. All task agents,
including Scratchpad agents, use the same classifier contract. Shell-only tabs
are available for deeper terminal inspection; they do not run an extra classifier.

Updates carry source, observation time, evidence and uncertainty. The timestamp of
the cited screen/history evidence orders inferred updates. Older/equal evidence cannot
replace newer records. Explicit link corrections are protected from future
inference; marking a link incidental rejects an incorrect association without
letting monitoring recreate it. Summaries retain the last supported observation
and become stale after five minutes; a saved working state is not proof of a live
process. Migration 36 gives lifecycle state its own observation timestamp:
`state_observed_at` and `state_stale` are independent of summary `observed_at` and
`stale`. Idle/working classifications update state even without a new summary;
the existing monitor scheduler records observed exits and marks missing process
identities unknown after restart. PR links remain discoverable without summaries.
Live session responses and terminal output establish current activity.

MCP tools:

- `get_task_pr_context(task_id)`: inspect links, corrections, summaries and sessions.
- `get_pr_context(repository, number)`: read GitHub facts and related task/session
  observations. `repository` also accepts a full PR URL without `number`.
- `set_task_pr_link(task_id, repository, number, role, evidence, uncertainty)`:
  explicitly correct a local association; role is implementation, review or
  incidental. No GitHub write occurs.

HTTP equivalents are GET `/api/tasks/{id}/pr-context`, GET
`/api/pr-context?repository=owner%2Frepo&number=7`, and PUT
`/api/tasks/{id}/pr-links`. Existing `get_task` and `get_session` responses also
carry saved observations and links. Full terminal evidence remains available
through `get_session_output`.

Each PR lookup fetches current GitHub metadata under a 30-second deadline:
state, draft, head, requested reviewers/teams, authenticated viewer, and viewer
review history across all pages (up to 100 pages). Earlier approvals remain visible
independently of the current head. `viewer_ever_approved: null` means unknown;
`reviews_complete: false` discloses a partial scan. Positive approval evidence is
retained even when a later page fails. Recent commits are bounded to the last
page of ten commit subjects; failures have a separate `changes_error`. Unavailable
metadata is explicitly marked unavailable/stale, never synthesized as open or
unapproved. Facts have their own source and fetch timestamp and are refreshed
on every lookup rather than serving an unmarked cache.

Notification subject decoration contains `pr_facts`, `related_work`, and
`related_sessions` outside the truncatable `context` excerpt. Both lookup tools
are permitted in read-only notification triage. Work decoration strips transcript
evidence, bounds summary text, and represents each session once. Serialized work
arrays are capped at 8 KB per subject and share a 32 KB budget across a processing
batch; omissions set `work_context_truncated` and provide `work_context_lookup`.
Optional commit/review detail arrays share a separate 32 KB batch budget.
Review counts and truncation flags distinguish a shortened decoration from
incomplete GitHub fetching; viewer identity, approval, and status stay intact.
Full evidence remains available through MCP. User notification preferences
remain prose interpreted by the LLM; this feature adds no approval-based filter.

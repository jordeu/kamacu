# Terminal monitoring and inbox

Kamacu observes each running agent's terminal, including custom agents and Scratchpad. Terminal activity drives the working indicator. A configured LLM detects questions, permission dialogs, blocking errors, and completed responses. No status hook or feedback tool call is required. Process exit is still determined directly from the process.

Messages relay the agent’s response directly in its own voice. The compact preview highlights the concrete result or decision; opening the message shows the relevant description, rationale, links, verification, and limitations needed to respond. PR descriptions are preserved as message content, including details available in recent terminal history, instead of being replaced with narration about what the agent is doing.

## Setup

1. Open **Settings → Terminal monitoring → Add monitoring model**.
2. Choose **Installed Claude Code login** or **Installed Codex login** to reuse the CLI's authentication on the Kamacu server. Sign in using that CLI first. Leave Model empty to use its default, or specify a model available to your account. Kamacu invokes the unmodified CLI; it does not copy OAuth tokens. Existing environment authentication overrides and provider subscription limits still apply.
3. Alternatively choose **OpenAI-compatible API or local endpoint**, or **Anthropic API**. Enter a base URL including its API version, such as `http://localhost:11434/v1` or `https://api.anthropic.com/v1`, and a model ID. For authenticated endpoints, name an environment variable containing the API key on the Kamacu server. Leave it empty for unauthenticated local endpoints. Keys are not stored in these settings or returned to the browser.
4. Use **Test connection**, save, and select the global default. Scratchpad uses this default. Choose a workspace override under **Workspace models** or **Manage workspaces → Monitoring model**.

A model must be configured before starting new agents. A workspace without an override inherits the global default. A failing explicit override stays unavailable; Kamacu never silently sends its terminal text to another provider. Changes apply to running sessions. The selected model receives rendered terminal text, so choose its endpoint accordingly.

OpenAI-compatible endpoints must support Chat Completions with JSON-schema response formatting. Compatibility varies by server/model; test the actual endpoint. Monitoring is independent of the coding agent's provider.

## Notifications and replies

- The inbox sidebar and Pending tab list messages in queue order, oldest first. New messages join the end, and monitoring updates preserve a message's position. **Move to end** puts a pending message at the back of the queue without answering or dismissing it. Queue order survives refreshes and restarts. Archived messages remain newest first.
- Watching a terminal suppresses its notifications, while status continues updating. Viewing means that exact terminal is selected in a visible, focused browser window. Any fresh window viewing it counts.
- An unanswered question appears after leaving, without requiring more terminal output. Returning hides its card and attention count; leaving restores the same card. Dismissal is permanent for that episode.
- A completion produced while watching is considered seen, even if inference finishes after leaving. Unrelated screen repaints preserve the evidence's original time.
- Explicit choices in conversational questions appear as clickable answer buttons. When no choices are offered, the monitor suggests two or three relevant replies based on the question and context. Suggestions do not invent missing user information and are never sent automatically. Clicking one sends its displayed text; you can also write your own answer. Updated choices invalidate older buttons.
- Ordinary conversational answers are saved and sent as plain terminal text. Recognized Claude Code single-select question dialogs show the original choices and descriptions as buttons. Choosing one navigates with arrow keys, checks the rendered selection after each step, and presses Enter. Multi-question dialogs can advance one question at a time; additional review screens require terminal interaction. Multi-select, preview, free-text entry and unrecognized layouts use Open task / Open Scratchpad. Native approval menus offer **Open task** / **Open Scratchpad** so the user handles them in the terminal.
- Replies check the screen revision and input generation before writing. If delivery is uncertain, the saved answer remains in Archived for manual recovery. After a server restart, interrupted delivery is explicitly marked uncertain. Other message writers and keyboard input cannot interleave between a reply’s paste and submit key. Kamacu never automatically resends it: a successful PTY write is not proof that the agent processed it.
- Completion cards always offer **Close session**, Open task (or Open Scratchpad), and Dismiss. Manual tasks also offer every legal forward board move: **Move to review and close session** from Todo/In progress, and **Move to done and close session** from Todo/In progress/Review. These controls are available even when the model makes no recommendation. Model recommendations are labeled **Recommended** using the scoped objective, project/repository, and terminal evidence. Scratchpad, PR-review sessions, and tasks already Done still offer Close session, without board moves. Closing alone leaves the task status unchanged.
- Published reviews complete review sessions even when they include nonblocking comments or optional follow-up offers. A resolved Renovate batch can recommend Done without waiting for informational post-merge CI; explicitly required validation or extra requested work remains outstanding and is explained in the message.
- Completion actions run only when clicked. Kamacu rechecks terminal freshness before closing. For a board move, it also rechecks the message and task status, saves the move and message answer together, and then closes the originating session. Failed or stale moves leave the session open. Open task and Dismiss archive the message and keep the session running. Inbox/MCP exits do not open another task-status dialog; only an exit from the actively viewed agent terminal offers that dialog.

The sidebar and full Inbox share pending counts. Hidden cards do not count. Model rewording updates the same attention episode rather than reopening a dismissed card. Fresh request evidence after user input starts a new episode, even if the question text repeats. Questions superseded by observed progress and questions belonging to stopped sessions are archived as unanswered, without inventing a user response. Existing inbox history and compatibility tools remain available for recovering older messages; there is no legacy status-monitoring mode.

Presence updates on terminal selection, navigation, focus, and visibility, with five-second heartbeats. Each document has its own identity and ordered beacons, so duplicated tabs and delayed requests cannot overwrite a newer viewing state. Page departure sends an explicit away beacon. If a browser disappears without a final update, its presence expires after fifteen seconds.

## Operation

The Go server renders terminal bytes continuously, independently of browser attachment. Input and screen revisions guard delayed model results and replies. The renderer uses bounded screen/history storage and preserves split UTF-8/ANSI sequences. It matches terminal dimensions down to a single cell; terminals larger than 300 columns or 100 rows show monitoring unavailable rather than trusting clipped output. Resize invalidates pending replies. Rendering is synchronous to avoid dropped bytes; model inference runs outside the PTY/session locks.

The scheduler checks once a second, debounces stable output for two seconds, and limits calls to at most one per session every five seconds. Continuously changing output is reconsidered after ten seconds. Four workers prioritize the oldest attempt. User input or model configuration changes cancel obsolete calls; changed screens reject old results. Each call has a thirty-second timeout, bounded input/output, and provider failures back off. Stable unchanged screens require no repeated calls. There is no spending cutoff.

The active-session bar lists open agent terminals until their processes exit, independently of classification. Meaningful terminal output or user input marks an agent working immediately, with a 15-second activity window. Identical repaints and resizing do not count as activity. Without recent activity, the terminal is idle; this does not mean the task is complete. A current model decision can identify waiting, errors, or an idle prompt. New activity invalidates that decision. This works for every agent engine, including custom agents. Provider/auth/quota failures remain monitoring diagnostics and do not hide sessions or introduce an unknown status. Settings show calls, failures, timeouts, last latency, and tokens when the adapter receives usage data; these counters reset on server restart. Codex's current final-text adapter does not expose token counts.

Installed-login inference uses a fresh temporary directory, disables project instructions, action tools, hooks and MCP, and removes inherited Kamacu task identity. It runs outside the ordinary session manager so the monitor cannot monitor itself. API adapters have no tools. Terminal contents are treated as untrusted data and model decisions must cite current visible evidence.

On upgrade, new agent launches stop injecting Claude status hooks or selecting the Codex hook profile. Unchanged former Kamacu Codex/OpenCode hook files are retired; user-edited files are preserved with a diagnostic. Optional artifact/MCP tools and user configuration remain available. Restart agents to load updated MCP configuration.

## Recovery and current limits

Agent terminals are direct PTYs, not persistent tmux sessions; a server restart ends those agents. Shell tmux tabs retain their existing recovery behavior. Inbox history and episode decisions are durable, and a new agent session cannot inherit another session's pending decision.

Claude retains its explicit conversation ID. OpenCode retains its existing session-list discovery. Codex's normal exit footer supplies its resume ID without hooks, and saved IDs resume directly. If no ID was saved, Resume asks Codex for the latest interactive session in the task worktree or Scratchpad root. If several conversations share that directory, Codex may select a different one; if none exists, Codex reports that it cannot resume. The Resume option can therefore appear for a configured Codex directory even before a conversation exists.

Strict screen-generation checks favor avoiding stale notifications. A continuously repainting UI or a slow/limited provider can exceed the intended 10–15 second response time. The implementation has synthetic installed-login checks and a recorded Codex renderer replay; broad live-terminal accuracy and 1/5/10-session load benchmarks remain necessary before treating that latency as a production guarantee.

## Verification

```sh
npm ci --prefix web
npm run build --prefix web
npm test --prefix web
go test ./...
KAMACU_MONITOR_CORPUS=1 go test ./internal/monitor -run TestInstalledLoginCorpus -v
KAMACU_BROWSER_TEST=1 KAMACU_PLAYWRIGHT_PACKAGE=/absolute/path/to/playwright \
  go test ./internal/api -run '^TestInboxBrowser$' -count=1
```

The live corpus is opt-in and submits only synthetic fixtures to the current Claude/Codex logins. The deterministic PTY test covers question → deferred inbox → plain reply → completion, including completion inference finishing after departure. Browser checks cover desktop/mobile inbox, task and Scratchpad navigation, model selection/inheritance, and optional integrations. Set `KAMACU_BROWSER_EXECUTABLE` to an installed Chromium executable if needed. Race detection additionally requires a C compiler and `CGO_ENABLED=1`.

## External notification sources

Add GitHub under Settings → Notifications. Collection uses the server's bound
`gh` account and repositories linked to registered projects. Polling, collection,
and manual archiving do not invoke a model or change GitHub read state. Pausing a
source retains its checkpoint; disconnecting retains its history.

Choose a notification processing model independently of terminal monitoring.
**Process pending** captures the pending event revisions, rules and Inbox history.
One batch agent reads the plain numbered rules and context, with bounded read-only
lookups when needed. It decides relevance, grouping, task ownership and wording
in one workflow. Rules have stable IDs and save directly; there is no compiler,
held category, or automatic uncertainty-rule bypass. Preview runs the same agent
without publishing messages or starting work.

Before publishing, Kamacu checks whether any source discussion changed during
processing. It refreshes only changed discussions and supplies them as final
notes alongside the previous proposal and existing read results. The agent can
revise the complete response once, preserving unaffected work. Routine bot
updates do not themselves require feedback. This check does not restart the
batch repeatedly; saved actions still validate current state when executed.
Refreshed evidence receives new IDs; obsolete source reads cannot be cited in
the final response. Large source bodies appear once in the prompt. The batch
trace retains the refreshed evidence, original reads and previous proposal,
including when the final revision fails.

### Saved option actions

Each actionable notification option carries its own versioned sequence of typed
application operations. Different buttons can target different PR subsets or
existing tasks. The browser sends only the option ID and question revision;
Kamacu atomically saves that exact option action with the answer. Selecting a
button does not invoke the orchestration model. Clarification choices are marked
as replies and are interpreted like free text.

Supported operations are `start_reviews`, `run_recurring_task`, `continue_task`,
`create_investigation`, `send_to_session`, `close_session`, `complete_task`,
`update_task`, `associate_pr`, `ignore`, and `open_task`. Sequences have 1–10
operations and up to 20 PRs per operation. A task reference can refer to an earlier
task-producing operation instead of inventing an ID. Operations cannot execute
arbitrary MCP tools, shell commands, or GitHub mutations. Task-agent instructions
are saved and previewable before the user selects an option.

Review actions open or reuse proper PR review tasks, verify current PR state and
account identity, resolve the configured review prompt, start/resume an agent if
needed, wait for its input readiness, and submit the prompt with bracketed paste
followed by a separate Enter key. Existing initialized review agents receive the
saved follow-up context; initial prompts with uncertain delivery are never resent.
Starting an agent is not counted as successful handoff until submission succeeds.

Task continuation and recurring operations reuse current task sessions or resume
stored conversations and deliver the saved instructions. Investigation operations
create a task and deliver their instructions. Completion operations stop only the
explicit captured session, check for new input or replacement sessions, and use
the board's existing transactional move/recurrence behavior. Ignore and navigation
require no model or source enrichment.

### Free text, progress and recovery

A free-text reply uses one bounded notification-model planning call to produce the
same typed operations, or a focused clarification. The executor then runs without
further model calls. It does not ask a model to choose each tool, inspect tool
success, or write a final completion message. Task agents still use their own
models for actual work.

The action worker is separate from collection and triage. Each execution records
its exact action, resolved task/session IDs, per-step intent, arguments and result.
**Execution steps** in the Inbox exposes the durable trace. Failed/blocked actions
create a visible pending follow-up with task links and the original execution
trace. Failure notices are recorded durably and delivered after restart or a
transient publication failure, without replacing another pending question. When every attempted mutation
has a definite outcome, **Retry unfinished steps** reuses recorded successes
without another model call. Uncertain writes offer inspection instead of retry. Grouped reviews track
independent PR handoffs without a model-step ceiling. Overall outcomes include
succeeded, blocked, failed, partial and uncertain; submission means PTY delivery,
not completion of the task agent's work.

Before side effects, Kamacu checks current identity/source gates, rule revision,
task roles and relevant session/task/prompt preconditions. Busy sessions wait for an idle input prompt by default. An explicitly labelled
“send now, even if working” alternative can submit to a working input surface;
permission/choice dialogs are never answered automatically. Changed targets
produce specific attention/progress states instead of guessing or interrupting
work. Completed steps are reusable after restart. Interrupted mutations remain
uncertain and are never automatically repeated; interrupted reads are blocked for
inspection. Task and session links survive partial execution.

Migration preserves old history and uncertain executions, retires older pending
notification cards, and returns unexecuted inputs to pending for regeneration.
It does not replay previously answered or uncertain work.

When a saved action cannot finish, its notice names the stopped step and affected
tasks, distinguishes prompts that were sent from those still pending, and gives
concrete recovery instructions. Readiness failures offer **Retry sending review
prompts** (or instructions) when safe, reusing recorded setup and successful handoffs.
If retry is unavailable, the notice asks for inspection and new instructions.
Uncertain mutations require inspection and never offer automatic retry.
**Dismiss this notice** closes the notice without cancelling existing work.

The task's recorded agent startup can advance its own `todo` guard to
`in_progress` when the same agent session still owns the task. A completed,
deleted or otherwise changed target of unfinished work invalidates the saved action; retrying the
unchanged action is not offered. For a completed recurring task, a uniquely
matching pending instance can be proposed with a fresh saved action, requiring
an explicit choice before switching tasks.

Failures without a clear deterministic explanation receive one read-only model
diagnosis using the saved action, journal and current task state. Diagnosis
cannot execute work or override guards. Its result is persisted so publishing a
notice again does not repeat inference, and is not reused for another failed
run. If diagnosis is unavailable, the notice shows the concrete recorded error
in its visible text instead of a generic "action could not finish" question.

Retry guards apply to unfinished operations; a completed task does not block
unrelated remaining work. A session started by the action retains its input
guard, so manually entered instructions cannot be followed by a duplicate
handoff on retry. After entering input in its terminal, provide fresh instructions
instead of retrying the old sequence. Optional recurrence metadata cannot hide
the underlying failure notice.

### Automatic recording recovery

A readiness check or other read-only operation cannot create uncertain side
effects. Temporary database failures retry the same journal write before any
work is repeated. Lost read results can be read again. Successful side effects
are reused, while unconfirmed side effects remain blocked for inspection.

Recovery uses persisted attempts and backoff (five automatic resumptions), and
updates the original message's execution progress without creating a question.
Task, session and input guards are checked again before continuing. Startup
reconciles older read-only recording failures only when all mutations are known
and the user has not handled the failure notice or started a retry. Superseded
notices are archived while recovery runs and refreshed if a real problem remains.

If the database remains unavailable, the final notice is an **Operational
issue**, with no required reply or free-text answer form. A bounded read-only
model diagnosis can explain the failure, but cannot authorize a repeated side
effect. Execution history is derived from the journal; a history-cache or UI
progress failure does not invalidate confirmed work.

Database diagnostics report connection-pool waits and journal transaction time
separately. Agent-status polling releases its database cursor before inspecting
transcripts and scans the transcript tree once per request.

### Pending instruction reconciliation

When an Inbox action cannot establish a safe agent input prompt, Kamacu records
its exact unsent instruction, task, session, input generation, blocker and terminal
snapshot separately from the execution journal. Session lifecycle changes wake a
reconciliation worker; a five-second poll also discovers terminal and work-summary
changes. Evidence must settle for two seconds. Assessments are at least 30 seconds
apart per handoff, and the same evidence is assessed at most once, including after
restart. Redraw counters and summary timestamps alone do not trigger another call.
The worker uses the notification model, independently of notification collection
and action execution. Assessment inputs, results and errors are retained for audit.

A read-only assessment distinguishes work already handled, work underway, a
specific decision, an outstanding instruction and insufficient evidence. Completion
requires a quote from current terminal/work evidence, rather than task status or
an input counter. New evidence invalidates a response produced against an older
snapshot. No assessment submits input, answers an agent question or changes a task.

Work already handled closes the notice as **Reconciled**. Work underway appears in
history as **Work in progress** and remains tracked. A later relevant decision
reopens that same notice with its concrete question. Outstanding instructions can
be offered as an explicit, freshly guarded handoff to the same session (review
handoffs retain the review workflow and PR checks). Unknown outcomes offer task
inspection, not a broad retry. Multi-task requests stay visible while any handoff
needs attention. Answers and dismissed notices remain immutable history.
Existing pending readiness notices are imported; handled notices are not reopened.

### Incremental GitHub notification evidence

Notification collection still polls GitHub threads and durably saves their raw
versions before advancing the collection checkpoint. Processing now keeps a
separate activity cache per registered source/account and PR or issue. The cache
stores immutable, monotonically numbered versions of comments and reviews. Body
or review-state changes create a version; a timestamp-only update does not.
Restoring an older body after another edit creates a new version too.

Ordinary comments and inline review comments use `since` with a five-minute
overlap after a complete fetch. The review catalog is compared against saved
versions because GitHub's REST review-list endpoint has no `since` parameter.
That same catalog supplies current approval facts within the read scope. Every
newly submitted review brings in its inline comments, including comments drafted
before the checkpoint. Current PR state, head, reviewers and approval facts stay
in every processing input. Complete reconciliation scans occur when processing a
subject whose last full scan is at least 24 hours old.

Fetched and processed checkpoints are independent. Complete fetches may advance
the retrieval cursor before inference succeeds because all undecided versions
remain in the durable cache. Cached undecided entries absent from a later delta
are revalidated using their individual GitHub resource endpoints. Deleted or
inaccessible entries are represented as availability metadata; their old text is
not replayed as a current request. Deleted latest-comment pointers do not block
processing when the authoritative comment and review streams are complete.
First scans use the source registration time as the unseen-activity boundary,
not the latest notification timestamp. Existing descriptions and activity before
that boundary remain context. Legacy snapshots seed only each resource's latest
acknowledged content; restoring an earlier body remains new activity.

The model receives new activity, the current description, parent context when
cached, related work and Inbox history. Large generated coverage tables from
`github-actions[bot]` are represented by their coverage header and all text outside
the table. Reports with a direct mention are kept in full. All original versions
remain available through the read-only `read_activity` tool. No bot category is
automatically ignored: the numbered text rules still determine relevance.

Review streams and ordinary comment streams have independent retrieval limits,
instead of competing for one 96 KB allowance. Missing required pages, resources
or exceeded retrieval limits leave notifications pending with a processing error.
A manifest lists every new activity version. Each must be assigned exactly once
to a model decision. If the input uses a compact index, required activity text
must be supplied through complete read pages before a final response is accepted.
Current evidence membership comes from the durable delta, not proximity to the
GitHub notification timestamp.

Inbox publication, raw-notification archiving and acknowledgement of the exact
activity versions share one transaction. Newer fetched versions are not marked
processed by an older result. The bounded final freshness pass merges additional
activity even if the PR metadata timestamp did not change. Processing details
record new/unchanged items, retrieved text bytes, current evidence bytes, actual
GitHub requests, decision rounds and read-only lookups. Decision rounds count
application-level model requests, not retries internal to a model provider.

**Check for missed notifications** previews the latest ignored decisions made
with incomplete evidence in the last seven days. Nothing is changed by that
preview. Select specific entries and choose **Move selected to pending**, then
use **Process pending** to reassess them. Repair revalidates the preview revisions,
forces a complete retrieval, preserves past messages/decisions, and widens the
activity window so older missed feedback is not treated as baseline history.
No task, terminal action or model call starts merely by queuing a repair.
The corresponding endpoints are `GET /api/notifications/repair` and
`POST /api/notifications/repair` with the selected preview objects in `items`.

Explicitly moving an archived notification back to pending and running a rule
preview deliberately replays the selected activity window, including handled
versions. These inputs are labelled `activity_delta.replay`; prior receipts and
Inbox history are preserved. New issue bodies and release announcements remain
activity, so direct mentions in their primary content are not lost as historical
description context. Bootstrap widens its window when recent legacy decisions
have recorded retrieval gaps; explicit repair can promote previously unseen
context without replaying versions already acknowledged by completed decisions.

`read_thread` provides older cached discussion, optionally limited to one review.
Its pages and full `read_activity` lookups are restricted to batch subjects and
versions present at the scan receipt; a concurrent later edit cannot enter an
earlier decision through a history lookup. Historical results are explicitly
labelled, and source refresh invalidates older thread/activity read results.

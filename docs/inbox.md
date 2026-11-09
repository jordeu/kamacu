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

- The inbox sidebar lists messages from oldest to newest. New messages join the end, and monitoring updates preserve a message's original position.
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

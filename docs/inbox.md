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
- Completion cards always offer Open task (or Open Scratchpad) and Dismiss. The monitoring model may also offer **Move to review and close session**, **Move to done and close session**, both, or neither, based on the task title, description, current status, and terminal evidence. Scratchpad and PR-review sessions have no board-move actions. A task already in Review can only be offered Done; a task already Done gets neither.
- Completion actions run only when clicked. Kamacu rechecks the message and task status, saves the board move and message answer together, and then closes the originating session. Failed or stale moves leave the session open. Open task and Dismiss archive the message and keep the session running. Inbox/MCP exits do not open another task-status dialog; only an exit from the actively viewed agent terminal offers that dialog.

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

Claude retains its explicit conversation ID. OpenCode retains its existing session-list discovery. Codex's normal exit footer supplies its resume ID without hooks, and already saved IDs resume normally. A fresh Codex process killed before emitting that footer may have no saved resume ID; automatic crash recovery of that conversation is a remaining limitation. Kamacu deliberately does not guess from unrelated conversations in the same directory.

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

Under **Settings → Notification sources**, add GitHub to collect new activity in
repositories linked to Kamacu projects. Collection uses the server's current
`gh` login and the GitHub notifications API. Install and authenticate `gh` with
access to notifications first. This first provider supports github.com; the
provider registry also supports adding other source implementations.

Collection starts at registration time, without importing an old unread backlog.
GitHub read/done state stays unchanged. Polling starts at one-minute intervals
and respects GitHub's advertised polling interval. Failed or incomplete scans
retain their checkpoint. The source is bound to the first successfully observed
account: switching `gh` accounts produces a diagnostic instead of mixing account
history. Account identity is checked again before enrichment and every notification
action, independently of polling. A mismatch stops processing before further
actions. Disconnect and add the source again to start with another account.
Disconnection preserves inbox history. Pausing retains the checkpoint and catches
up on activity after resuming. Turning off GitHub integration pauses collection
and notification actions too.

Collected notifications first appear in the **Notifications** view, with separate
**Pending** and **Archived** lists. Browsing this view and manually archiving or
restoring items never invokes the model. Collection does not automatically enrich,
classify, group, or discard new notifications. Older revisions remain available
for comparison with newer activity.

The Inbox sidebar shows a separate pending-notification counter, a link to the raw
list, and **Process pending**, available in both expanded and collapsed layouts.
The same processing control is available in Notifications. Clicking it captures
all currently pending notifications in one durable snapshot. Repeated clicks join
the active run; notifications collected afterward wait for the next run.

Processing uses the global monitoring model and global **Notification instructions**;
workspace model overrides do not apply. The model reads the whole snapshot with
current subject state, bounded comment/review excerpts, matching projects/tasks,
and related pending inbox questions. It groups related activity into one question
per concrete action, can propose several distinct actions from one thread, and
discards obsolete activity such as a review request for a merged PR. Existing
pending questions can be preserved or explicitly superseded. The full related
pending-question set is checked before inference and again during publication;
new follow-ups, edits, answers, or dismissals invalidate a stale plan and leave
its raw notifications pending for another explicit run. Exact duplicate actions
are deduplicated without merging different actions for the same notification.

On success, every raw notification in the snapshot moves to Archived, including
notifications represented by inbox actions. Inbox tracks the resulting decisions
independently. Publishing the actions, superseding old questions, archiving raw
items, and completing the run commit together. Failed or interrupted processing
leaves all snapshot notifications pending; retry requires another explicit click.
Missing subjects or deleted comments are supplied as unavailable-context warnings,
not as evidence that work is resolved. Missing optional comments preserve any
verified subject state and other excerpts. Resulting action cards disclose missing
context, while authentication, identity, rate-limit and transport failures still
stop the run without consuming notifications.

An oversized snapshot fails visibly instead of silently processing only part of
the pending list. Notifications in an active snapshot cannot be manually archived
or restored until the run finishes.

Notification questions belong to their source rather than an agent session and
survive server restarts. Replies are saved and queued atomically. A bounded
execution loop exposes the full registered Kamacu MCP tool catalog after a user
reply, including creating tasks, moving tasks, deleting resources, and posting
reviews when requested. The reply determines the target project, task, or session;
targets may be outside the notification repositories. Grouped actions and their
follow-up questions retain every contributing notification. Source content is
treated as untrusted data; the user's reply supplies action authorization.
During triage, the model can use read-only MCP tools to inspect active sessions
and recent terminal output before proposing actions, avoiding duplicate review
requests when a review is already underway. Mutations require an inbox reply.
Agent-scoped legacy feedback tools keep their API identity requirements; this worker has no agent
identity and uses notification follow-up questions and outcomes instead.

Execution results appear with the saved answer in **Archived**. **Reply again**
opens another question without replacing earlier answers. Reply processing keeps
the displayed summary, context, choices, earlier replies, and tool results so that
references to previous sessions or decisions remain available. Ambiguous instructions
can produce a follow-up question directly. Failed or interrupted actions are never
automatically repeated; an uncertain result asks you to check the target task or
session before requesting another attempt. Execution history is stored locally.
Settings show collection health, queued notifications, and processing errors.

Notification action cards route follow-up work to task agents. New PR comments,
feedback, questions, and commits should offer to resume the responsible
implementation task and ask its agent to inspect the latest comments and commits.
A clear match in the project task list can identify the owner even without an
explicit PR link; a separate review task does not take precedence over it. If no
owner exists, offer to create a task and start its agent. The inbox delegates the
work rather than asking the user to resolve technical feedback or compose a PR
reply there. After the user accepts, execution checks existing sessions, starts
or resumes the agent as needed, and sends the follow-up instructions with the
full PR URL. Ambiguous ownership requires choosing a task.

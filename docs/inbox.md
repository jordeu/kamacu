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

Collected notifications appear in **Notifications**, with **Pending**, **Held**, and
**Archived** lists. Collection and manual archiving do not invoke a model.
**Process pending** captures a durable snapshot of pending inputs and the active
rule-set revision. New arrivals wait for the next run. Repeated clicks join the
active run. Processing requires an active rule set.

### Notification rules

Under notification settings, write one instruction per rule. Each rule has a stable
ID and revision. Saving creates a draft; **Prepare rules** queues a model job that
converts changed instructions into validated conditions and effects. Unchanged
compiled instructions can be reused. The compiler identifies filtering, grouping,
action, and presentation clauses. Each rule shows **Compiled**, **LLM evaluated · not
compiled**, or **Not compiled · awaiting preparation**. When compilation cannot
represent a rule faithfully, that rule falls back to LLM evaluation; its explanation
and suggested alternative rules appear beside its text. The suggestions are never
applied automatically. Other rules can still compile. You can also explicitly choose
**Use LLM evaluation**, or choose automatic compilation again to retry a rewrite.
Review each rule's mode and compiled interpretation, including
its missing-evidence behavior and priority, then explicitly activate the revision.
The global monitoring model handles compilation and notification processing.

Filtering conditions on repository and verified subject facts run before activity
classification. Conditions evaluate to true, false, or unknown. Unknown evidence
holds affected activity outside the inbox by default; continuing requires an explicit
rule instruction. A direct-mention exception belongs to its own rule and cannot
bypass unrelated exclusions. A team review request does not prove an individual
assignment. Only verified individual assignment or a current directly addressed
request can produce a personal review request.

The model classifies independent activities using cited source evidence. Factual
conditions run in code; semantic conditions use a bounded model check that must
cover every condition and cite supplied evidence. LLM-evaluated rules are interpreted
from their full text for each activity, including filtering, grouping, routing and
presentation. Every rule must receive a result with source-backed effects; ambiguous
meaning or missing evidence holds the activity. Compiled exclusions cannot be bypassed
by a fallback rule. Interpretation failures leave inputs pending for retry.
Suppression wins over action
selection. Conflicting action or grouping choices hold the activity for inspection.
Grouping runs after filtering and action planning, so different owners or workflows
are not combined. The writer describes the resulting plan and cannot override it. Its input contains
only admitted citations, their source identity, PR authorship, latest loaded review-state
metadata for cited reviewers, and resolved action plans; project
task catalogs, full discussions, and excluded activities are omitted. Writer inputs
are limited to 32 KB, splitting large inputs between existing cards while preserving
each card's grouping. `notification writer input` logs report prompt bytes, card count,
and subject count for each batch/project.

Session routing checks the linked review or implementation owner against the live
process manager and refreshes local routing context before publication, even when
GitHub has not changed. Without a live lookup, bounded history can prove a current
session exists but cannot prove its absence. Unknown or stale lifecycle evidence
remains unknown; it is not guessed by a model. The previously compiled new-session condition also uses this factual path,
so it does not require recompilation. GitHub reads are reused only within one subject
retrieval phase. Revalidation starts with a fresh cache and identity checks are never
cached.
Merged or closed PRs do not produce obsolete review requests.

**Test on recent notifications** runs the selected compiled revision without
publishing inbox cards or archiving inputs. It shows decisions and proposed cards,
including activities previously delivered to the inbox. Normal processing instead
deduplicates previously delivered activity. Preview and compilation jobs expose
queued/running/completed or failed states and diagnostics.

Each processed activity records its rule-set revision, clause results, facts,
missing evidence, and source-backed semantic judgments. Expand a notification's
decisions to inspect why it was admitted, suppressed, held, or deduplicated.
Held activities remain available in **Held** and can be explicitly retried after
fixing missing context. Other independent activities from the same input may still
produce inbox cards without being repeated on retry. Activating new rules releases
held inputs for reevaluation, retires stale pending cards, and requeues their inputs.
Publication and execution reject plans captured under a different active revision.

This replaces the former prompt and structured notification preferences. On upgrade,
existing development preferences are imported into an inactive draft for review,
compilation, and activation. There is no runtime fallback. Pending cards and queued
replies from the former path are invalidated and their inputs are requeued; tasks
and inbox history are retained.

### Processing and execution

Independent projects use up to three workers. Retrieval and revalidation share
limits of four subject jobs and four GitHub requests. Results publish atomically
in stable project order, together with decisions, card plans, and raw-item status.
A failed or interrupted run leaves its inputs available for an explicit retry.
Missing comments preserve other verified facts and become unknown evidence where
necessary. Authentication, account, rate-limit, and transport failures stop the run.
Oversized snapshots fail visibly rather than silently truncating pending inputs.
Inputs in an active snapshot cannot be manually archived or restored.

Server logs record `notification stage finished` with stage, duration, batch, and
repository where applicable. Stages include retrieval, classification, semantic
rules, action planning, drafting, revalidation, and publication. Compilation has its
own stage. Overlapping durations must not be summed as total batch time;
`notification batch finished` records total duration and errors.

Each card stores a typed action plan separately from its displayed text. Review
requests default to PR-review workspaces rather than ordinary Todo tasks. An explicit
rule can instead select an existing named recurring task for grouped reviews.
Other activities
can route to an implementation owner, an explicitly named recurring task, or a new
investigation task. Ambiguous targets require clarification. Cards offer one planned
action and dismissal. No task mutation occurs before an inbox reply.

Clicking the proposed action executes its saved plan. A free-text reply is separately
interpreted and validated; changing the workflow or target requires explicit user
instructions. Execution restricts mutations to the plan's workflow, projects, tasks,
and PRs, and checks the active rules again before each mutation. Creating a task or
review workspace alone cannot count as completing an action that requires starting
its agent. Follow-ups preserve the latest validated execution plan and source evidence.
Clarification answers are replanned even when selected using a button.

Notification questions survive restarts. Replies are saved and queued atomically;
results appear with the answer in **Archived**. **Reply again** preserves previous
answers and execution history. Failed or interrupted actions are never automatically
repeated; uncertain delivery requires checking the target before another attempt.

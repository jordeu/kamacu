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

- Watching a terminal suppresses its notifications, while status continues updating. Viewing means that exact terminal is selected in a visible, focused browser window. Any fresh window viewing it counts.
- An unanswered question appears after leaving, without requiring more terminal output. Returning hides its card and attention count; leaving restores the same card. Dismissal is permanent for that episode.
- A completion produced while watching is considered seen, even if inference finishes after leaving. Unrelated screen repaints preserve the evidence's original time.
- Explicit choices in conversational questions appear as clickable answer buttons. When no choices are offered, the monitor suggests two or three relevant replies based on the question and context. Suggestions do not invent missing user information and are never sent automatically. Clicking one sends its displayed text; you can also write your own answer. Updated choices invalidate older buttons.
- Ordinary conversational answers are saved and sent as plain terminal text. Native approval menus offer **Open task** / **Open Scratchpad** so the user handles them in the terminal.
- Replies check the screen revision and input generation before writing. If delivery is uncertain, the saved answer remains in Archived for manual recovery. After a server restart, interrupted delivery is explicitly marked uncertain. Other message writers and keyboard input cannot interleave between a reply’s paste and submit key. Kamacu never automatically resends it: a successful PTY write is not proof that the agent processed it.
- Completion cards support Open task, Dismiss, and OK, exit session. Exiting from the inbox or an MCP request does not open the task-status dialog. Only an exit triggered from the actively viewed agent terminal offers to move the task. Tasks are never automatically moved to Done.

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

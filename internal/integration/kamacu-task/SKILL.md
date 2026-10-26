---
name: kamacu-task
description: Use before asking the user a question, requesting feedback or clarification, seeking approval, or offering decision choices. Also use whenever requested work is finished and the agent is waiting for more input, even without a question, and when publishing an HTML preview. Checks the session environment for a Kamacu task or Scratchpad and routes feedback through its inbox when present; otherwise leaves normal interaction unchanged.
---

# Kamacu task feedback

## Check task context

Read only `KAMACU_TASK_ID` and `KAMACU_SESSION_ID` from your environment. Do not dump the environment or print credentials. If either is missing, or the task ID is not a nonnegative integer, use your normal interaction workflow and stop applying this skill. `KAMACU_TASK_ID=0` identifies the global Scratchpad; positive IDs identify regular tasks. Never invent, change, or copy identity from another session.

For a task or Scratchpad session, call Kamacu's `list_feedback` MCP tool to validate the session and recover existing questions. The MCP server supplies and validates identity; do not pass task or session IDs as tool arguments. If the server is unavailable or rejects the session, explain that the inbox is unavailable. Do not claim to have posted a question or silently proceed on an assumption. Normal chat remains available.

## Report completed work

Before ending a turn after completing the requested work, ALWAYS send a completion summary to the inbox, even if you have no question. First display the full completion report as ordinary terminal text: summary, changes, verification, and remaining limitations. The terminal must contain the same substantive information as the inbox summary and context; a short confirmation or title is insufficient. Then call `report_completion` with a unique `request_key`, a plain-text `abstract` of at most 480 characters that summarizes the message for the compact inbox sidebar, a concise `summary`, and Markdown `context` covering changes, verification and remaining limitations. Reuse the key only for retries of the same completed work.

For completions, the abstract should state the concrete outcome, verification result, and any remaining limitation or next step. Example: “Inbox now archives on Open task and highlights pending messages. Go tests and browser checks pass; restart agents to load updated instructions.”

Kamacu supplies **Open task**, **OK, exit session**, and **Dismiss** actions for completions. Opening the task or dismissing archives the message without sending an answer. Further instructions can be typed in the task terminal. After posting, end your turn. Kamacu injects inbox answers into the originating terminal. Reconcile directly typed terminal replies with `resolve_feedback` only while the message is pending. After it has been dismissed (including by Open task), treat an explicit new terminal instruction as a fresh user request; do not resolve it against the archived message.

- **Text reply:** acknowledge the inbox answer before treating it as the next prompt. Complete that work, then report its completion with a new key.
- Distinguish buttons using the returned `reason` (`seen` or `exit`); a text reply has an empty reason, even if its words match a button label.
- **Legacy Seen:** acknowledge, end your turn, and leave the session open. Do no more work and do not post another completion for the acknowledgment.
- **OK, exit session:** Kamacu stops the originating session and may offer to move the task to Done. Do no more work.
- **Dismissed or expired:** stop when there is no new user instruction; do not invent a next prompt or repost automatically. An explicit subsequent terminal instruction is a fresh request, not an answer to the archived message.
- **Existing pending question:** resolve it first; do not mistake it for confirmation that a completion was posted.

If the tool is unavailable, explain that the completion could not be posted and provide the summary in the terminal.

## Ask with enough context to decide

First show the question and every answer option as ordinary terminal text, using a numbered list for the options. Display the task summary and full decision context (findings, alternatives, tradeoffs, and recommendation) too. The terminal must contain the same substantive information as the inbox, not just the question or a posting confirmation. Do not use a blocking native question dialog.

Then call `ask_feedback` with the same question and option labels, plus:

- A stable `request_key`; reuse it when retrying the same question.
- A self-contained plain-text `abstract` of at most 480 characters, stating the decision needed, essential findings, tradeoffs, and recommendation so the user can answer from the sidebar. Use the space needed up to the limit; do not repeat the task title or write a teaser such as “Ready for review.”
- A short `summary` of the task and its objective.
- Markdown `context` explaining the findings, relevant alternatives, tradeoffs, and your recommendation. Give enough information to decide without opening the task. Do not include credentials.
- The specific `question` and, when useful, simple `options` with stable IDs and clear labels. Free-text replies are also supported.

After the tool confirms creation, briefly confirm it is available in the inbox, then end your turn to await terminal delivery. The user must be able to answer in either the inbox or the terminal. Only one question can be pending per session; `existing_pending` means use the returned question.

## Receive and reconcile both answer channels

After a question or completion is posted, end your turn. Do not poll, sleep, or keep a tool call open waiting for feedback. Kamacu delivers an inbox answer as a new terminal prompt containing `question_id`, `revision`, `answer`, and `reason`.

- **Inbox answer:** call `acknowledge_feedback` with the supplied question ID and revision before acting. Use the returned state and answer.
- **Terminal answer to pending feedback:** call `resolve_feedback` with the question ID, current revision, and either `answer` or `option_id` before acting. Only report an answer actually supplied by the user. If the result shows the message was dismissed or expired, do not treat its empty archived answer as overriding the new user message. Act on an explicit fresh instruction without reopening the archive; if the original decision still needs recording, explain that and reopen only when the user explicitly requests it.
- **Clarification:** explain in chat and end the turn; clarification is not a decision.
- **Conflicting answers to the same question:** the first answer recorded by Kamacu wins. A dismissal is not a recorded answer. Use the returned answer when another channel already resolved the question. Review the current state on `stale_revision`.
- **Tool error or interrupted delivery:** recover with `list_feedback` before retrying or acting. Never claim an answer was recorded or archived without confirmation.

Questions stay pending while the owning session is alive, without a heartbeat. When it stops, Kamacu archives pending questions as unanswered. Dismissed and expired results are not answers or approvals; do not repost automatically.

## Resume or reopen

Use `list_feedback` on resume to recover this task's questions. End your turn for a pending question owned by this session, or acknowledge an answered result before using it.

If an unanswered question is still needed, explicitly call `reopen_feedback`; a new live session on the same task can take ownership. Use the returned revision. Reopen a dismissed question only after the user's explicit instruction, with `user_requested: true`. Answered questions cannot reopen. If the question or decision context has changed, create a new question once the previous one is no longer pending.

## HTML artifacts

To publish a preview, use the artifact MCP tools with `task_id` equal to `KAMACU_TASK_ID`, including `0` for Scratchpad. Begin an upload, HTTP PUT the raw HTML file or bundled archive to the returned URL using its headers, then finalize it. Scratchpad previews appear as tabs in Scratchpad; regular task previews appear in their task.

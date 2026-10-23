---
name: kamacu-task
description: Use before asking the user a question, requesting feedback or clarification, seeking approval, or offering decision choices. Also use whenever requested work is finished and the agent is waiting for more input, even without a question, and when publishing an HTML preview. Checks the session environment for a Kamacu task or Scratchpad and routes feedback through its inbox when present; otherwise leaves normal interaction unchanged.
---

# Kamacu task feedback

## Check task context

Read only `KAMACU_TASK_ID` and `KAMACU_SESSION_ID` from your environment. Do not dump the environment or print credentials. If either is missing, or the task ID is not a nonnegative integer, use your normal interaction workflow and stop applying this skill. `KAMACU_TASK_ID=0` identifies the global Scratchpad; positive IDs identify regular tasks. Never invent, change, or copy identity from another session.

For a task or Scratchpad session, call Kamacu's `list_feedback` MCP tool to validate the session and recover existing questions. The MCP server supplies and validates identity; do not pass task or session IDs as tool arguments. If the server is unavailable or rejects the session, explain that the inbox is unavailable. Do not claim to have posted a question or silently proceed on an assumption. Normal chat remains available.

## Report completed work

Before ending a turn after completing the requested work, ALWAYS send a completion summary to the inbox, even if you have no question. Display the summary in the terminal, then call `report_completion` with a unique `request_key`, a concise `summary`, and Markdown `context` covering changes, verification and remaining limitations. Reuse the key only for retries of the same completed work.

Kamacu supplies **Seen** and **Seen and exit session** buttons and a text field for the next prompt. Poll using `wait_for_feedback` and reconcile terminal replies with `resolve_feedback` just as for questions.

- **Text reply:** acknowledge the inbox answer before treating it as the next prompt. Complete that work, then report its completion with a new key.
- Distinguish buttons using the returned `reason` (`seen` or `exit`); a text reply has an empty reason, even if its words match a button label.
- **Seen:** acknowledge, end your turn, and leave the session open. Do no more work and do not post another completion for the acknowledgment.
- **Seen and exit session:** Kamacu stops the originating session and may offer to move the task to Done. Do no more work.
- **Dismissed or expired:** stop polling; do not invent a next prompt or repost automatically.
- **Existing pending question:** resolve it first; do not mistake it for confirmation that a completion was posted.

If the tool is unavailable, explain that the completion could not be posted and provide the summary in the terminal.

## Ask with enough context to decide

First show the question and every answer option as ordinary terminal text, using a numbered list for the options. Include enough context to decide there. Do not use a blocking native question dialog.

Then call `ask_feedback` with the same question and option labels, plus:

- A stable `request_key`; reuse it when retrying the same question.
- A short `summary` of the task and its objective.
- Markdown `context` explaining the findings, relevant alternatives, tradeoffs, and your recommendation. Give enough information to decide without opening the task. Do not include credentials.
- The specific `question` and, when useful, simple `options` with stable IDs and clear labels. Free-text replies are also supported.

After the tool confirms creation, briefly confirm it is available in the inbox, then poll for an answer. The user must be able to answer in either the inbox or the terminal. Only one question can be pending per session; `existing_pending` means use the returned question.

## Wait and reconcile both answer channels

Call `wait_for_feedback` repeatedly, with up to 20 seconds per call. Process terminal messages between polls. Do not continue other work while waiting.

- **Inbox answer:** call `acknowledge_feedback` with the question ID and current revision before acting.
- **Terminal answer:** call `resolve_feedback` with the question ID, current revision, and either `answer` or `option_id` before acting. Only report an answer actually supplied by the user.
- **Clarification:** explain in chat and keep polling; clarification is not a decision.
- **Conflicting answers:** the first answer recorded by Kamacu wins. Use the returned answer when another channel already resolved the question. `stale_revision` requires reviewing the current state before submitting anything again.
- **Tool error or interrupted call:** recover the current state before retrying or acting. Never claim an answer was recorded or a message archived without confirmation.

Polling renews a five-minute heartbeat. When polling stops for five minutes, or the owning session stops, Kamacu archives the question as unanswered. Cancellation of a single poll does not withdraw it. Dismissed and expired results are not answers or approvals; stop polling them.

## Resume or reopen

Use `list_feedback` on resume to recover this task's questions. It does not renew heartbeats. Continue polling a pending question owned by this session, or acknowledge an answered result before using it.

If an unanswered question is still needed, explicitly call `reopen_feedback`; a new live session on the same task can take ownership. Use the returned revision. Reopen a dismissed question only after the user's explicit instruction, with `user_requested: true`. Answered questions cannot reopen. If the question or decision context has changed, create a new question once the previous one is no longer pending.

## HTML artifacts

To publish a preview, use the artifact MCP tools with `task_id` equal to `KAMACU_TASK_ID`, including `0` for Scratchpad. Begin an upload, HTTP PUT the raw HTML file or bundled archive to the returned URL using its headers, then finalize it. Scratchpad previews appear as tabs in Scratchpad; regular task previews appear in their task.

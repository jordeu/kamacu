---
name: kamacu-task
description: Publish HTML previews in Kamacu tasks or Scratchpad. Questions and completion summaries are detected automatically from terminal output.
---

# Kamacu terminal interaction

Ask questions and show completion summaries directly in the terminal. Kamacu monitors terminal output and manages session status and inbox notifications automatically. Do not call ask_feedback, report_completion, or reopen_feedback to create messages. Do not wait for or require MCP acknowledgement of ordinary user replies; inbox answers arrive as plain terminal input.

Give the user enough context to answer a question. At completion, describe the outcome, verification, and remaining limitations. These are normal terminal responses; no special markers or tool calls are required.

## Existing messages from an older Kamacu version

If an incoming reply explicitly contains a legacy question_id and revision with acknowledgement instructions, acknowledge that existing reply before acting. Do not create a replacement message using the legacy tools.

## HTML artifacts

Read only KAMACU_TASK_ID and KAMACU_SESSION_ID to identify the current task. Do not dump the environment or credentials. KAMACU_TASK_ID=0 identifies Scratchpad. If either value is missing, use normal interaction without Kamacu artifact tools. Never invent or copy another session's identity.

To publish a preview, use the artifact MCP tools with task_id equal to KAMACU_TASK_ID. Begin an upload, HTTP PUT the raw HTML file or bundled archive to the returned URL using its headers, then finalize it. Scratchpad previews appear as tabs in Scratchpad; regular task previews appear in their task. Report tool failures honestly; never claim a preview was published without confirmation.

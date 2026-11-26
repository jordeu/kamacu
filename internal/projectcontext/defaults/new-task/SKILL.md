---
name: new-task
description: Create a task on the correct Kamacu project when the user explicitly asks to add one.
---

# Create a Kamacu task

1. Use the project ID the user supplied, or use the `find-project` skill to
   resolve it. For work spanning projects, create a task for each relevant
   project when the request calls for separate work.
2. Call Kamacu’s `list_tasks` with the project ID and check titles and
   descriptions for the same work. If an open task already covers it, point
   the user to that task. If a completed task is related, mention it in the
   new description and explain what remains.
3. Write a short imperative title and a description that another agent can
   use without this conversation. Include the problem, relevant files or
   links, and observable completion criteria. Include scope limits the user
   gave.
4. Call `create_task` with `project_id`, `title` and `description`. Report
   the task ID, title, project and the returned `worktree_path` or
   `worktree_error`. If the creation result is uncertain, check the board
   before retrying. Start the agent only when the user also requests it.

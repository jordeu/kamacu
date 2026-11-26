---
name: find-project
description: Identify the Kamacu project a request refers to when its project ID or repository is unclear.
---

# Find the project

Use Kamacu’s `get_project` when the user supplies an ID. Otherwise call
`list_projects` and match the request against project names, descriptions,
GitHub repositories and checkout paths. When a workspace name would
disambiguate the match, use `list_workspaces` to identify its ID.

If multiple projects fit, ask the user to choose and include the repository
or workspace that distinguishes each candidate. For a request spanning
multiple projects, identify each one.

Use the selected project ID for subsequent Kamacu calls. When the task needs
the default branch, look up `refs/remotes/origin/HEAD` in the returned
`repo_path` with `git symbolic-ref --short`. If it is missing, report the
branch as unknown until you can verify it.

# M001: Configurable Agents

**Vision:** Make agents first-class configurable: define custom agents (name + command template) in global Settings, mark a default, and choose per-project which agent runs in the task Agent tab. The Claude Code integration stays byte-for-byte unchanged (hooks, resume, quota, working/waiting/idle); custom agents run their command in the worktree PTY with running/exited status. Uses a uniform agents table (claude is a pre-seeded, non-deletable default row) and an engine column that cleanly separates the claude capability tier from the generic one.

## Success Criteria

- A user can add a custom agent (name + command template) in global Settings and it persists.
- A user can set any agent as the global default; exactly one default always exists.
- A user can choose, per-project, which agent runs; existing projects default to the Claude agent with no migration action.
- Spawning a task whose project uses a custom agent runs that agent's command in the worktree PTY and shows running/exited status; terminal detach/reattach works.
- Spawning a task whose project uses the Claude agent is byte-for-byte unchanged: working/waiting/idle status, --resume recovery, and the quota indicator all work with zero regression (fake-claude tests green).
- An agent in use by any project cannot be deleted (block-until-unassigned); the claude seed is non-deletable.
- Zero new Go modules or npm dependencies (D007); frontend gate (tsc -b && vite build) and Go suite (go test/vet, make test) green.
- Human-verify gate passes: a custom agent runs end-to-end AND a claude project shows no regression.

## Slices

- [x] **S01: Agent data foundation and spawn engine** `risk:medium` `depends:[]`
  > After this: After this: a custom agent row can be created via API, a project assigned to it, and spawning that project's task runs the agent's command in the worktree PTY with running/exited status; a claude project still shows working/waiting + resume.

- [x] **S02: Agent management UI and per-project selection** `risk:low` `depends:[S01]`
  > After this: After this: the user adds a custom agent in Settings, picks it on a project, opens a task, and sees that agent run in the Agent tab with running/exited status and working terminal reattach; switching the project back to Claude restores working/waiting + resume with no UI regression.

## Boundary Map

| Boundary | S01 (backend) owns | S02 (frontend) owns |\n|---|---|---|\n| agents table + migration + backfill | yes | no |\n| /api/agents CRUD + set-default | yes | no |\n| spawn engine branch (claude/custom) | yes | no |\n| command-template render + substitution | yes | no |\n| Settings Agents section UI | no | yes |\n| Project Settings agent selector | no | yes |\n| Agent tab status dot (engine-aware) | status source in S01 | render in S02 |\n| quota indicator (engine-aware) | data in S01 | show/hide in S02 |

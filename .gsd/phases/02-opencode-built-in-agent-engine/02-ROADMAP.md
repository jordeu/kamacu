# M002: opencode built-in agent engine

**Vision:** Add opencode as a first-class built-in agent alongside claude, with the same terminal UX and running/waiting/idle status detection, reusing the existing claude hook-receiver path via a gated global opencode plugin (D013).

## Success Criteria

- A task can be assigned the opencode agent and spawn a real opencode session in the terminal
- opencode task status reflects working/waiting/idle/exited driven by opencode events (not just activity heuristics)
- opencode sessions resume after a kamacu restart
- Engine-matrix, fake-opencode argv regression, and real-binary status matrix tests are green

## Slices

- [ ] **S01: opencode engine spawn and activity-based status** `risk:low` `depends:[]`
  > After this: After this: create a task with the opencode agent, watch it launch in the terminal, status toggles working/idle as it runs.

- [ ] **S02: Gated status plugin unlocks waiting and idle** `risk:high` `depends:[S01]`
  > After this: After this: an opencode permission prompt flips the task status to waiting; turn end flips it to idle — same UX as claude.

- [ ] **S03: Session resume and argv regression hardening** `risk:medium` `depends:[S01,S02]`
  > After this: After this: restart kamacu, an opencode task's session resumes to the same conversation; the fake-opencode test guards the exact argv we build.

## Boundary Map

Not provided.

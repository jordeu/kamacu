## Noted during 03-03 execution (2026-06-10)

- `web/dist/index.html` carries uncommitted local modifications: a previous
  local `make build` overwrote the committed placeholder with real Vite build
  output (hashed asset references). Pre-existing before 03-03 started; out of
  plan scope. Phase 01 decision says real build output is never committed —
  restore with `git checkout -- web/dist/index.html` or leave for the next
  frontend plan's build cycle to handle.

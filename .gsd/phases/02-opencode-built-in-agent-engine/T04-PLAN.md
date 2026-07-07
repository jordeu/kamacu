---
estimated_steps: 13
estimated_files: 1
skills_used: []
---

# T04: Integration test: hook receiver drives working/waiting/idle for an opencode-engine session

WHY: This is the cross-cutting integration proof. hooks.go is UNCHANGED and engine-agnostic, but we must prove that an opencode-engine session (made full-heuristics by T02) actually transitions working/waiting/idle when the UNCHANGED hook receiver fires — exactly the loop the plugin (T03) drives in production. This closes the Go-side status contract without needing the real opencode binary.

DO:
1. New file internal/api/opencode_hook_status_test.go (package api). Build a real test mux via api.HookRoutes(mux, mgr, token) (mirror the scaffolding in internal/api/hooks_test.go: httptest.NewRequest + mux.ServeHTTP + the X-Kangent-Token header). Configure mgr.SetAgentConfig with the SAME token. Spawn an opencode-engine session: mgr.Spawn(session.SpawnOpts{Kind:session.KindAgent, AgentEngine:"opencode", AgentArgs:[]string{"sleep","10"}, Cwd:t.TempDir(), TaskID:1}). Assert throughout that Info().Engine == "opencode" (genuinely exercising the opencode branch). Then:
   a. working — immediately after spawn, assert Info().AgentStatus == "working" (lastActivity is set at spawn for KindAgent; timing-robust since agentQuietThreshold >> test elapsed time).
   b. waiting — POST {hook_event_name:"Notification"} to /api/hooks/sessions/<id> with the token header → assert response 204 and Info().AgentStatus == "waiting".
   c. idle — POST {hook_event_name:"Stop"} → assert 204 and Info().AgentStatus == "idle".
   d. reject — POST {hook_event_name:"Notification"} with a WRONG token → assert 401 and AgentStatus UNCHANGED (still the pre-POST value), proving the token gate holds for opencode sessions too.
   e. SessionStart — POST {hook_event_name:"SessionStart"} → assert 204 (the hooksAlive canary flips; observable indirectly — assert no error and session still reachable). If straightforward, additionally assert that after SessionStart a notification still works (proves the receiver stays healthy).
   Use defer s.Stop(). Read the session id from sess.Info().ID for the URL path.
2. Reference internal/api/hooks_test.go for the exact httptest pattern and how the hook payload is sent (raw JSON body). Do NOT modify hooks.go — it is the unchanged seam this test validates.

CONSTRAINTS: no real opencode (sleep stub); do not modify hooks.go/session.go/manager.go; pure black-box test through the real hook handler + real manager.

DONE WHEN: an opencode-engine session transitions working(spawn)→waiting(Notification)→idle(Stop) under the UNCHANGED hook receiver, the wrong-token POST is rejected with status unchanged, all assertions hold with Engine=="opencode", and `go test ./internal/api/ -run OpencodeHook -count=1` is green. The live opencode→plugin→hook loop is deferred to UAT (needs opencode installed).

Skills: go, testing, http.

## Inputs

- `internal/api/hooks.go`
- `internal/api/hooks_test.go`
- `internal/session/manager.go`

## Expected Output

- `internal/api/opencode_hook_status_test.go`

## Verification

go test ./internal/api/ -run OpencodeHook -count=1

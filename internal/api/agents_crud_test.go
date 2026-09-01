package api

import (
	"net/http"
	"testing"
)

// defaultAgentID returns the id of the default (is_default=true) agent from
// GET /api/agents. On a fresh migrated DB that is the Claude seed (id 1).
func defaultAgentID(t *testing.T, srvURL string) int64 {
	t.Helper()
	status, list := doJSONList(t, srvURL+"/api/agents")
	if status != http.StatusOK {
		t.Fatalf("GET /api/agents: status=%d", status)
	}
	for _, a := range list {
		if def, _ := a["is_default"].(bool); def {
			return int64(a["id"].(float64))
		}
	}
	t.Fatalf("no default agent in %v", list)
	return 0
}

// TestAgentList proves GET /api/agents returns the seeded Claude agent with
// is_default + is_system true and engine='claude' on a fresh migrated DB. A
// fresh DB now seeds TWO system agents (claude + opencode, M002 migration 00015),
// so the test locates the CLAUDE seed by engine rather than assuming list[0].
func TestAgentList(t *testing.T) {
	srv, _, _ := newTestServer(t)

	status, list := doJSONList(t, srv.URL+"/api/agents")
	if status != http.StatusOK {
		t.Fatalf("status=%d, want 200", status)
	}
	if len(list) < 1 {
		t.Fatalf("agents = %d, want >= 1 (seeded system agents)", len(list))
	}
	var a map[string]any
	for _, x := range list {
		if x["engine"] == "claude" {
			a = x
			break
		}
	}
	if a == nil {
		t.Fatalf("no claude agent in list %v", list)
	}
	if a["name"] != "Claude Code" {
		t.Errorf("name = %v, want \"Claude Code\"", a["name"])
	}
	if a["engine"] != "claude" {
		t.Errorf("engine = %v, want \"claude\"", a["engine"])
	}
	if def, _ := a["is_default"].(bool); !def {
		t.Errorf("is_default = %v, want true", a["is_default"])
	}
	if sys, _ := a["is_system"].(bool); !sys {
		t.Errorf("is_system = %v, want true", a["is_system"])
	}
}

// TestAgentCreate proves a custom agent is created with engine='custom',
// is_default=false, is_system=false.
func TestAgentCreate(t *testing.T) {
	srv, _, _ := newTestServer(t)

	status, a := doJSON(t, "POST", srv.URL+"/api/agents",
		map[string]string{"name": "Gemini", "command": "gemini"})
	if status != http.StatusCreated {
		t.Fatalf("status=%d, want 201", status)
	}
	if a["engine"] != "custom" {
		t.Errorf("engine = %v, want \"custom\" (new agents are always custom)", a["engine"])
	}
	if def, _ := a["is_default"].(bool); def {
		t.Errorf("is_default = %v, want false", a["is_default"])
	}
	if sys, _ := a["is_system"].(bool); sys {
		t.Errorf("is_system = %v, want false", a["is_system"])
	}
	if a["command"] != "gemini" {
		t.Errorf("command = %v, want \"gemini\"", a["command"])
	}

	// The list now has the system seeds (claude + opencode) + the new custom
	// agent = 3.
	_, list := doJSONList(t, srv.URL+"/api/agents")
	if len(list) != 3 {
		t.Errorf("agents after create = %d, want 3", len(list))
	}
}

// TestAgentCreateDuplicate proves a case-insensitive name collision is 409.
func TestAgentCreateDuplicate(t *testing.T) {
	srv, _, _ := newTestServer(t)
	// "Claude Code" already exists (seed) -- a case variant must collide.
	status, _ := doJSON(t, "POST", srv.URL+"/api/agents",
		map[string]string{"name": "claude code", "command": "x"})
	if status != http.StatusConflict {
		t.Fatalf("status=%d, want 409 (case-insensitive dup)", status)
	}
}

// TestAgentCreateEmpty proves name + command are both required.
func TestAgentCreateEmpty(t *testing.T) {
	srv, _, _ := newTestServer(t)
	if status, _ := doJSON(t, "POST", srv.URL+"/api/agents",
		map[string]string{"name": "", "command": "x"}); status != http.StatusBadRequest {
		t.Errorf("empty name: status=%d, want 400", status)
	}
	if status, _ := doJSON(t, "POST", srv.URL+"/api/agents",
		map[string]string{"name": "X", "command": ""}); status != http.StatusBadRequest {
		t.Errorf("empty command: status=%d, want 400", status)
	}
}

// TestAgentUpdateCustom proves a custom agent's name, command, and engine are
// all editable.
func TestAgentUpdateCustom(t *testing.T) {
	srv, _, _ := newTestServer(t)
	_, created := doJSON(t, "POST", srv.URL+"/api/agents",
		map[string]string{"name": "Aider", "command": "aider"})
	id := int64(created["id"].(float64))

	status, a := doJSON(t, "PATCH", srv.URL+"/api/agents/"+itoa(id),
		map[string]any{"name": "Aider2", "command": "aider --model sonnet"})
	if status != http.StatusOK {
		t.Fatalf("PATCH custom: status=%d, want 200", status)
	}
	if a["name"] != "Aider2" || a["command"] != "aider --model sonnet" {
		t.Errorf("updated = name=%v command=%v", a["name"], a["command"])
	}
}

// TestAgentUpdateSystemEngineLocked proves the claude seed's engine is immutable
// (a PATCH attempting to change it is 400), while its name IS editable.
func TestAgentUpdateSystemEngineLocked(t *testing.T) {
	srv, _, _ := newTestServer(t)
	id := defaultAgentID(t, srv.URL)

	// Engine change on the system agent -> 400.
	status, _ := doJSON(t, "PATCH", srv.URL+"/api/agents/"+itoa(id),
		map[string]any{"engine": "custom"})
	if status != http.StatusBadRequest {
		t.Errorf("system engine change: status=%d, want 400", status)
	}

	// Name change on the system agent -> 200 (the binary-path name is editable).
	status, a := doJSON(t, "PATCH", srv.URL+"/api/agents/"+itoa(id),
		map[string]any{"name": "Claude"})
	if status != http.StatusOK {
		t.Fatalf("system rename: status=%d, want 200", status)
	}
	if a["name"] != "Claude" {
		t.Errorf("system rename: name=%v, want \"Claude\"", a["name"])
	}
	if a["engine"] != "claude" {
		t.Errorf("system engine drifted: %v, want \"claude\"", a["engine"])
	}
}

// TestAgentDeleteUnused proves an unused custom agent is deleted (204).
func TestAgentDeleteUnused(t *testing.T) {
	srv, _, _ := newTestServer(t)
	_, created := doJSON(t, "POST", srv.URL+"/api/agents",
		map[string]string{"name": "Tmp", "command": "x"})
	id := int64(created["id"].(float64))

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/agents/"+itoa(id), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("delete unused custom: status=%d, want 204", resp.StatusCode)
	}

	// Back to the two system seeds (claude + opencode).
	_, list := doJSONList(t, srv.URL+"/api/agents")
	if len(list) != 2 {
		t.Errorf("agents after delete = %d, want 2 (system seeds only)", len(list))
	}
}

// TestAgentDeleteSystem proves the claude seed (is_system=1) is non-deletable.
func TestAgentDeleteSystem(t *testing.T) {
	srv, _, _ := newTestServer(t)
	id := defaultAgentID(t, srv.URL)

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/agents/"+itoa(id), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE system: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("delete system agent: status=%d, want 409", resp.StatusCode)
	}
}

// TestAgentDeleteInUse proves an agent referenced by a project is blocked 409
// (block-until-unassigned). Seeds a project via the projects API, points it at
// the custom agent, then attempts delete.
func TestAgentDeleteInUse(t *testing.T) {
	srv, db, _ := newTestServer(t)
	_, created := doJSON(t, "POST", srv.URL+"/api/agents",
		map[string]string{"name": "Used", "command": "x"})
	agentID := int64(created["id"].(float64))

	// Insert a project row directly and point it at the custom agent (the
	// projects API path through create is heavier; a direct insert isolates the
	// in-use guard, mirroring how the workspaces in-use test seeds).
	if _, err := db.Exec(
		`INSERT INTO projects (name, repo_path, workspace_id, agent_id) VALUES (?, ?, 1, ?)`,
		"P", "/tmp/agent-inuse", agentID,
	); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/agents/"+itoa(agentID), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE in-use: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("delete in-use agent: status=%d, want 409", resp.StatusCode)
	}
}

// TestAgentDeleteInUseByGlobal proves the global_task.agent_id reference
// blocks agent deletion 409 with the locked D-09 string (count-free — the
// singleton references exactly one agent, always), and that moving the
// reference away unblocks the delete (block-until-reassigned). The friendly
// COUNT guard runs ahead of the 00017 ON DELETE RESTRICT backstop.
func TestAgentDeleteInUseByGlobal(t *testing.T) {
	srv, db, _ := newTestServer(t)
	_, created := doJSON(t, "POST", srv.URL+"/api/agents",
		map[string]string{"name": "Scratch", "command": "x"})
	agentID := int64(created["id"].(float64))

	// Point the singleton at the custom agent directly (no global config API
	// exists until Phase 14; the FK row is the delete handler's only input).
	if _, err := db.Exec(`UPDATE global_task SET agent_id = ? WHERE id = 1`, agentID); err != nil {
		t.Fatalf("point global_task at custom agent: %v", err)
	}

	// Referenced -> 409 with the exact D-09 message.
	status, out := doJSON(t, http.MethodDelete, srv.URL+"/api/agents/"+itoa(agentID), nil)
	if status != http.StatusConflict {
		t.Errorf("delete agent referenced by global_task: status=%d, want 409", status)
	}
	if msg, _ := out["error"].(string); msg != "reassign the Scratchpad agent first" {
		t.Errorf("409 body error = %q, want the D-09 string %q", msg, "reassign the Scratchpad agent first")
	}

	// Reassigned back to the default agent -> the same delete succeeds 204
	// (block-until-reassigned semantics).
	if _, err := db.Exec(
		`UPDATE global_task SET agent_id = (SELECT id FROM agents WHERE is_default = 1) WHERE id = 1`,
	); err != nil {
		t.Fatalf("reassign global_task to default agent: %v", err)
	}
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/agents/"+itoa(agentID), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE after reassign: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("delete after reassign: status=%d, want 204", resp.StatusCode)
	}
}

// TestAgentSetDefault proves the default flag moves to the target and the
// exactly-one invariant holds.
func TestAgentSetDefault(t *testing.T) {
	srv, _, _ := newTestServer(t)
	_, created := doJSON(t, "POST", srv.URL+"/api/agents",
		map[string]string{"name": "New", "command": "x"})
	newID := int64(created["id"].(float64))

	status, a := doJSON(t, "POST", srv.URL+"/api/agents/"+itoa(newID)+"/default", nil)
	if status != http.StatusOK {
		t.Fatalf("set-default: status=%d, want 200", status)
	}
	if def, _ := a["is_default"].(bool); !def {
		t.Errorf("returned agent is_default=%v, want true", a["is_default"])
	}

	// Exactly-one invariant: count is_default agents.
	_, list := doJSONList(t, srv.URL+"/api/agents")
	defCount := 0
	for _, x := range list {
		if d, _ := x["is_default"].(bool); d {
			defCount++
		}
	}
	if defCount != 1 {
		t.Errorf("is_default count after set-default = %d, want 1", defCount)
	}

	// The old default (claude seed) is no longer default. Scan the list we
	// already fetched (the seed is id=1 on a fresh DB).
	seedID := int64(1)
	if seedID == newID {
		t.Fatalf("new agent id == 1, test setup wrong")
	}
	for _, x := range list {
		if int64(x["id"].(float64)) == seedID {
			if d, _ := x["is_default"].(bool); d {
				t.Errorf("claude seed still is_default after set-default")
			}
		}
	}
}

// TestRenderAgentCommand covers the custom-engine template render: placeholder
// substitution + settings.Tokenize shell-splitting.
func TestRenderAgentCommand(t *testing.T) {
	cases := []struct {
		name       string
		template   string
		worktree   string
		sessionID  string
		wantBin    string
		wantArgs   []string
	}{
		{
			name:      "bare command",
			template:  "gemini",
			wantBin:   "gemini",
			wantArgs:  nil,
		},
		{
			name:      "command with flags",
			template:  "aider --model sonnet",
			wantBin:   "aider",
			wantArgs:  []string{"--model", "sonnet"},
		},
		{
			name:      "quoted arg with spaces",
			template:  `aider --model "claude 3.5 sonnet"`,
			wantBin:   "aider",
			wantArgs:  []string{"--model", "claude 3.5 sonnet"},
		},
		{
			name:      "worktree placeholder substituted",
			template:  "myagent --cwd {{worktree}}",
			worktree:  "/tmp/path with spaces/wt",
			wantBin:   "myagent",
			wantArgs:  []string{"--cwd", "/tmp/path with spaces/wt"},
		},
		{
			name:      "session_id placeholder substituted",
			template:  "myagent --session {{session_id}}",
			sessionID: "abc-123",
			wantBin:   "myagent",
			wantArgs:  []string{"--session", "abc-123"},
		},
		{
			name:      "unknown placeholder left literal (degrade-don't-break)",
			template:  "myagent --foo {{bar}}",
			wantBin:   "myagent",
			wantArgs:  []string{"--foo", "{{bar}}"},
		},
		{
			name:      "empty template yields nil",
			template:  "   ",
			wantBin:   "",
			wantArgs:  nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := renderAgentCommand(tc.template, tc.worktree, tc.sessionID)
			var gotBin string
			var gotArgs []string
			if len(got) > 0 {
				gotBin = got[0]
				gotArgs = got[1:]
			}
			if gotBin != tc.wantBin {
				t.Errorf("bin = %q, want %q", gotBin, tc.wantBin)
			}
			if len(gotArgs) != len(tc.wantArgs) {
				t.Fatalf("args = %v, want %v", gotArgs, tc.wantArgs)
			}
			for i, w := range tc.wantArgs {
				if gotArgs[i] != w {
					t.Errorf("args[%d] = %q, want %q", i, gotArgs[i], w)
				}
			}
		})
	}
}

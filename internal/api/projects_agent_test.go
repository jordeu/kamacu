package api

import (
	"net/http"
	"testing"
)

// TestProjectCreateDefaultAgent proves a folder project created WITHOUT an
// explicit agent_id lands on the global default (the Claude seed, id 1 on a
// fresh DB) — R020 read-at-use default.
func TestProjectCreateDefaultAgent(t *testing.T) {
	srv, _, _ := newTestServer(t)
	repo := gitRepo(t)

	status, p := doJSON(t, "POST", srv.URL+"/api/projects",
		map[string]string{"repo_path": repo})
	if status != http.StatusCreated {
		t.Fatalf("create: status=%d, want 201", status)
	}
	agentID, _ := p["agent_id"].(float64)
	if agentID != 1 {
		t.Errorf("created project agent_id = %v, want 1 (default Claude seed)", p["agent_id"])
	}
}

// TestProjectCreateExplicitAgent proves a supplied agent_id is honored.
func TestProjectCreateExplicitAgent(t *testing.T) {
	srv, _, _ := newTestServer(t)
	repo := gitRepo(t)
	// Create a custom agent first.
	_, created := doJSON(t, "POST", srv.URL+"/api/agents",
		map[string]string{"name": "Gemini", "command": "gemini"})
	customID := int64(created["id"].(float64))

	status, p := doJSON(t, "POST", srv.URL+"/api/projects",
		map[string]any{"repo_path": repo, "agent_id": customID})
	if status != http.StatusCreated {
		t.Fatalf("create: status=%d, want 201", status)
	}
	agentID, _ := p["agent_id"].(float64)
	if int64(agentID) != customID {
		t.Errorf("created project agent_id = %v, want %d (explicit)", p["agent_id"], customID)
	}
}

// TestProjectCreateUnknownAgent proves a supplied non-existent agent_id is 400,
// and no project row is created.
func TestProjectCreateUnknownAgent(t *testing.T) {
	srv, _, _ := newTestServer(t)
	repo := gitRepo(t)

	status, _ := doJSON(t, "POST", srv.URL+"/api/projects",
		map[string]any{"repo_path": repo, "agent_id": 999})
	if status != http.StatusBadRequest {
		t.Errorf("create with unknown agent: status=%d, want 400", status)
	}
	_, list := doJSONList(t, srv.URL+"/api/projects")
	if len(list) != 0 {
		t.Errorf("projects after failed create = %d, want 0 (no row)", len(list))
	}
}

// TestProjectPatchAgent proves PATCH /api/projects/{id} changes the agent and
// validates the target id.
func TestProjectPatchAgent(t *testing.T) {
	srv, _, _ := newTestServer(t)
	repo := gitRepo(t)
	_, p := doJSON(t, "POST", srv.URL+"/api/projects",
		map[string]string{"repo_path": repo})
	pid := itoa(int64(p["id"].(float64)))
	// Create a custom agent.
	_, created := doJSON(t, "POST", srv.URL+"/api/agents",
		map[string]string{"name": "Aider", "command": "aider"})
	customID := int64(created["id"].(float64))

	// Patch the project to the custom agent.
	status, updated := doJSON(t, "PATCH", srv.URL+"/api/projects/"+pid,
		map[string]any{"agent_id": customID})
	if status != http.StatusOK {
		t.Fatalf("PATCH agent: status=%d, want 200", status)
	}
	if got, _ := updated["agent_id"].(float64); int64(got) != customID {
		t.Errorf("after PATCH agent_id = %v, want %d", updated["agent_id"], customID)
	}

	// Patch to a non-existent agent -> 400, unchanged.
	status, _ = doJSON(t, "PATCH", srv.URL+"/api/projects/"+pid,
		map[string]any{"agent_id": 999})
	if status != http.StatusBadRequest {
		t.Errorf("PATCH unknown agent: status=%d, want 400", status)
	}
	_, after := doJSONList(t, srv.URL+"/api/projects")
	for _, row := range after {
		if int64(row["id"].(float64)) == int64(p["id"].(float64)) {
			if got, _ := row["agent_id"].(float64); int64(got) != customID {
				t.Errorf("agent_id after failed PATCH = %v, want %d (unchanged)", row["agent_id"], customID)
			}
		}
	}
}

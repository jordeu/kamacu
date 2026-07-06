package api

import (
	"bytes"
	"net/http"
	"os/exec"
	"slices"
	"testing"

	"kamacu/internal/settings"
)

func TestSettingsGetAllDefaults(t *testing.T) {
	srv, db, _ := newTestServer(t)
	// This test asserts the pristine all-defaults shape and provisions no
	// worktrees — drop the harness's safety seed (see seedWorktreeBase).
	if _, err := db.Exec(`DELETE FROM settings WHERE key = ?`, settings.KeyWorktreeBase); err != nil {
		t.Fatalf("clear worktree_base seed: %v", err)
	}

	status, body := doJSON(t, http.MethodGet, srv.URL+"/api/settings", nil)
	if status != http.StatusOK {
		t.Fatalf("GET /api/settings = %d, want 200", status)
	}
	for key, def := range settings.Defaults {
		entry, ok := body[key].(map[string]any)
		if !ok {
			t.Fatalf("GET body missing key %q: %v", key, body)
		}
		if entry["value"] != def {
			t.Errorf("%s value = %v, want default %q", key, entry["value"], def)
		}
		if entry["default"] != def {
			t.Errorf("%s default = %v, want %q", key, entry["default"], def)
		}
	}
	// Shell entry additionally carries options mirroring AllowedShells()
	// (PATH truth at call time — exactly ["bash"] plus "tmux" when present).
	shell := body["shell"].(map[string]any)
	if got, want := optionStrings(t, shell), settings.AllowedShells(); !slices.Equal(got, want) {
		t.Errorf("shell options = %v, want %v (AllowedShells PATH truth)", got, want)
	}
	// Non-shell entries omit options.
	if _, has := body["branch_template"].(map[string]any)["options"]; has {
		t.Error("branch_template entry has options, want omitted")
	}
}

func TestSettingsPutValidTemplate(t *testing.T) {
	srv, db, _ := newTestServer(t)

	status, body := doJSON(t, http.MethodPut, srv.URL+"/api/settings/branch_template",
		map[string]string{"value": "task/{slug}-{id}"})
	if status != http.StatusOK {
		t.Fatalf("PUT = %d, want 200 (body %v)", status, body)
	}
	if body["value"] != "task/{slug}-{id}" {
		t.Errorf("value = %v, want task/{slug}-{id}", body["value"])
	}
	if body["default"] != "task/{slug}-{id}" {
		t.Errorf("default = %v, want task/{slug}-{id}", body["default"])
	}

	// SET-02: persisted in SQLite (survives restart by construction).
	var stored string
	if err := db.QueryRow(`SELECT value FROM settings WHERE key = ?`, settings.KeyBranchTemplate).Scan(&stored); err != nil {
		t.Fatalf("SELECT stored value: %v", err)
	}
	if stored != "task/{slug}-{id}" {
		t.Errorf("stored value = %q, want task/{slug}-{id}", stored)
	}
}

func TestSettingsPutInvalidTemplateStoresNothing(t *testing.T) {
	srv, db, _ := newTestServer(t)

	// Store a known-good value first so "previous value" is observable.
	status, _ := doJSON(t, http.MethodPut, srv.URL+"/api/settings/branch_template",
		map[string]string{"value": "wip/{slug}-{id}"})
	if status != http.StatusOK {
		t.Fatalf("setup PUT = %d, want 200", status)
	}

	status, body := doJSON(t, http.MethodPut, srv.URL+"/api/settings/branch_template",
		map[string]string{"value": "task/{slugg}"})
	if status != http.StatusBadRequest {
		t.Fatalf("PUT invalid = %d, want 400 (body %v)", status, body)
	}
	want := "Unknown token {slugg}. Use {slug}, {id}, or {title}."
	if body["error"] != want {
		t.Errorf("error = %v, want %q (canonical copy verbatim)", body["error"], want)
	}

	// Nothing stored: GET still shows the previous value.
	status, all := doJSON(t, http.MethodGet, srv.URL+"/api/settings", nil)
	if status != http.StatusOK {
		t.Fatalf("GET = %d, want 200", status)
	}
	entry := all["branch_template"].(map[string]any)
	if entry["value"] != "wip/{slug}-{id}" {
		t.Errorf("value after rejected PUT = %v, want previous wip/{slug}-{id}", entry["value"])
	}
	var stored string
	if err := db.QueryRow(`SELECT value FROM settings WHERE key = ?`, settings.KeyBranchTemplate).Scan(&stored); err != nil {
		t.Fatalf("SELECT: %v", err)
	}
	if stored != "wip/{slug}-{id}" {
		t.Errorf("stored = %q, want previous wip/{slug}-{id}", stored)
	}
}

func TestSettingsPutUnknownShell(t *testing.T) {
	srv, _, _ := newTestServer(t)

	status, body := doJSON(t, http.MethodPut, srv.URL+"/api/settings/shell",
		map[string]string{"value": "zsh"})
	if status != http.StatusBadRequest {
		t.Fatalf("PUT zsh = %d, want 400", status)
	}
	if body["error"] != "Unknown shell." {
		t.Errorf("error = %v, want %q", body["error"], "Unknown shell.")
	}
}

func TestSettingsPutShellResponseIncludesOptions(t *testing.T) {
	srv, _, _ := newTestServer(t)

	status, body := doJSON(t, http.MethodPut, srv.URL+"/api/settings/shell",
		map[string]string{"value": "bash"})
	if status != http.StatusOK {
		t.Fatalf("PUT bash = %d, want 200", status)
	}
	// The PUT response must include options so the frontend cache replace
	// keeps the dropdown populated.
	if got, want := optionStrings(t, map[string]any(body)), settings.AllowedShells(); !slices.Equal(got, want) {
		t.Errorf("PUT shell response options = %v, want %v (AllowedShells PATH truth)", got, want)
	}
}

// TestSettingsShellOptionsTrackPath asserts TMUX-01 end-to-end at the HTTP
// layer: the dropdown options array and the save acceptance both follow
// exec.LookPath("tmux") at call time, from the same AllowedShells function.
func TestSettingsShellOptionsTrackPath(t *testing.T) {
	srv, _, _ := newTestServer(t)

	if _, err := exec.LookPath("tmux"); err == nil {
		// PATH-present: tmux is offered AND accepted.
		status, body := doJSON(t, http.MethodGet, srv.URL+"/api/settings", nil)
		if status != http.StatusOK {
			t.Fatalf("GET /api/settings = %d, want 200", status)
		}
		opts := optionStrings(t, body["shell"].(map[string]any))
		if !slices.Contains(opts, "tmux") {
			t.Errorf("shell options with tmux on PATH = %v, want to contain tmux", opts)
		}
		status, putBody := doJSON(t, http.MethodPut, srv.URL+"/api/settings/shell",
			map[string]string{"value": "tmux"})
		if status != http.StatusOK {
			t.Fatalf("PUT shell=tmux with tmux on PATH = %d, want 200 (body %v)", status, putBody)
		}
	}

	// PATH-scrubbed: the option vanishes and re-saving tmux is honestly
	// rejected — same LookPath truth on both sides. t.Setenv auto-restores.
	t.Setenv("PATH", t.TempDir())
	status, body := doJSON(t, http.MethodGet, srv.URL+"/api/settings", nil)
	if status != http.StatusOK {
		t.Fatalf("GET /api/settings = %d, want 200", status)
	}
	opts := optionStrings(t, body["shell"].(map[string]any))
	if !slices.Equal(opts, []string{"bash"}) {
		t.Errorf(`shell options with scrubbed PATH = %v, want ["bash"]`, opts)
	}
	status, putBody := doJSON(t, http.MethodPut, srv.URL+"/api/settings/shell",
		map[string]string{"value": "tmux"})
	if status != http.StatusBadRequest {
		t.Fatalf("PUT shell=tmux with scrubbed PATH = %d, want 400 (body %v)", status, putBody)
	}
	if putBody["error"] != "Unknown shell." {
		t.Errorf("error = %v, want %q", putBody["error"], "Unknown shell.")
	}
}

// optionStrings extracts a settings entry's options array as []string.
func optionStrings(t *testing.T, entry map[string]any) []string {
	t.Helper()
	raw, ok := entry["options"].([]any)
	if !ok {
		t.Fatalf("entry options missing or not an array: %v", entry["options"])
	}
	out := make([]string, len(raw))
	for i, v := range raw {
		s, ok := v.(string)
		if !ok {
			t.Fatalf("options[%d] = %v, want string", i, v)
		}
		out[i] = s
	}
	return out
}

func TestSettingsPutUnknownKey(t *testing.T) {
	srv, _, _ := newTestServer(t)

	status, _ := doJSON(t, http.MethodPut, srv.URL+"/api/settings/nope",
		map[string]string{"value": "x"})
	if status != http.StatusNotFound {
		t.Fatalf("PUT unknown key = %d, want 404", status)
	}
}

func TestSettingsPutInvalidJSON(t *testing.T) {
	srv, _, _ := newTestServer(t)

	req, err := http.NewRequest(http.MethodPut, srv.URL+"/api/settings/shell",
		bytes.NewReader([]byte("{not json")))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("PUT invalid JSON = %d, want 400", resp.StatusCode)
	}
}

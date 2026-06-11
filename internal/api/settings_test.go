package api

import (
	"bytes"
	"net/http"
	"testing"

	"kangent/internal/settings"
)

func TestSettingsGetAllDefaults(t *testing.T) {
	srv, _, _ := newTestServer(t)

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
	// Shell entry additionally carries options ["bash"].
	shell := body["shell"].(map[string]any)
	opts, ok := shell["options"].([]any)
	if !ok || len(opts) != 1 || opts[0] != "bash" {
		t.Errorf(`shell options = %v, want ["bash"]`, shell["options"])
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

func TestSettingsPutEmptyExtraParams(t *testing.T) {
	srv, _, _ := newTestServer(t)

	status, body := doJSON(t, http.MethodPut, srv.URL+"/api/settings/agent_extra_params",
		map[string]string{"value": ""})
	if status != http.StatusOK {
		t.Fatalf("PUT empty = %d, want 200 (body %v)", status, body)
	}
	if v, ok := body["value"].(string); !ok || v != "" {
		t.Errorf("PUT response value = %v, want \"\"", body["value"])
	}

	// Empty ≠ absent: GET shows value "" with the skip-permissions default.
	_, all := doJSON(t, http.MethodGet, srv.URL+"/api/settings", nil)
	entry := all["agent_extra_params"].(map[string]any)
	if v, ok := entry["value"].(string); !ok || v != "" {
		t.Errorf("GET value = %v, want \"\" (NOT the default)", entry["value"])
	}
	if entry["default"] != "--dangerously-skip-permissions" {
		t.Errorf("GET default = %v, want --dangerously-skip-permissions", entry["default"])
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
	opts, ok := body["options"].([]any)
	if !ok || len(opts) != 1 || opts[0] != "bash" {
		t.Errorf(`PUT shell response options = %v, want ["bash"]`, body["options"])
	}
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

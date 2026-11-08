package monitor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kamacu/internal/diagnostics"
	"kamacu/internal/observation"
)

func TestHTTPTranscripts(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		classify   bool
	}{
		{"notification", `{"choices":[{"message":{"content":"{\"ok\":true}"}}],"usage":{"prompt_tokens":12}}`, 200, false},
		{"invalid classification", `{"choices":[{"message":{"content":"invalid"}}]}`, 200, true},
		{"provider error", `{"error":"quota exceeded"}`, 429, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); w.Write([]byte(tc.body)) }))
			defer srv.Close()
			dir := t.TempDir()
			rec, err := diagnostics.NewRecorder(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer rec.Close()
			t.Setenv("KAMACU_TEST_TRANSCRIPT_KEY", "secret-key-never-recorded")
			b := Backend{Transcripts: rec}
			p := Profile{Backend: "openai_compatible", Endpoint: srv.URL, Model: "fixture", CredentialEnv: "KAMACU_TEST_TRANSCRIPT_KEY"}
			if tc.classify {
				_, err = b.Classify(context.Background(), p, observation.Snapshot{Screen: "hello"})
			} else {
				_, err = b.GenerateJSON(context.Background(), p, "test prompt", map[string]string{"type": "object"})
			}
			if (err != nil) != (tc.status != 200 || tc.classify) {
				t.Fatalf("error %v", err)
			}
			data, e := readTranscript(dir)
			if e != nil {
				t.Fatal(e)
			}
			if strings.Contains(string(data), "secret-key-never-recorded") {
				t.Fatal("credential recorded")
			}
			var v struct {
				Purpose, Prompt, Response, Error string
				Provider                         providerCapture
			}
			if e = json.Unmarshal(data, &v); e != nil {
				t.Fatal(e)
			}
			if v.Provider.Output != tc.body || v.Provider.HTTPStatus != tc.status || v.Prompt == "" {
				t.Fatalf("incomplete transcript: %s", data)
			}
			if tc.classify && (v.Purpose != "monitoring" || v.Error == "") {
				t.Fatal(string(data))
			}
		})
	}
}

func TestCLITranscripts(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	for _, backend := range []string{"claude_cli", "codex_cli"} {
		t.Run(backend, func(t *testing.T) {
			dir := t.TempDir()
			bin := filepath.Join(dir, "fixture")
			body := "#!/bin/sh\ncat >/dev/null\necho diagnostic >&2\necho '{\"ok\":true}'\n"
			if backend == "claude_cli" {
				body = "#!/bin/sh\ncat >/dev/null\necho diagnostic >&2\necho '{\"structured_output\":{\"ok\":true},\"usage\":{\"input_tokens\":5}}'\n"
			}
			if err := os.WriteFile(bin, []byte(body), 0700); err != nil {
				t.Fatal(err)
			}
			rec, err := diagnostics.NewRecorder(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer rec.Close()
			raw, err := (Backend{Transcripts: rec}).GenerateJSON(context.Background(), Profile{Backend: backend, Executable: bin}, "CLI prompt", map[string]string{"type": "object"})
			if err != nil || !strings.Contains(string(raw), `"ok":true`) {
				t.Fatalf("%s %v", raw, err)
			}
			data, err := readTranscript(dir)
			if err != nil {
				t.Fatal(err)
			}
			var v struct {
				Prompt   string
				Provider providerCapture
			}
			if err = json.Unmarshal(data, &v); err != nil {
				t.Fatal(err)
			}
			if v.Prompt != "CLI prompt" || strings.TrimSpace(v.Provider.Stderr) != "diagnostic" || v.Provider.Output == "" {
				t.Fatal(string(data))
			}
		})
	}
}

// Reconstruct the fields checked by adapter tests from the published folder.
func readTranscript(root string) ([]byte, error) {
	var dir string
	err := filepath.WalkDir(filepath.Join(root, "llm"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Name() == "metadata.json" {
			dir = filepath.Dir(path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(dir, "metadata.json"))
	if err != nil {
		return nil, err
	}
	var fields map[string]any
	if err = json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	data, err = os.ReadFile(filepath.Join(dir, "provider.json"))
	if err != nil {
		return nil, err
	}
	fields["provider"] = json.RawMessage(data)
	data, err = os.ReadFile(filepath.Join(dir, "prompt.md"))
	if err != nil {
		return nil, err
	}
	fields["prompt"] = string(data)
	return json.Marshal(fields)
}

package diagnostics

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestRotationAndPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "test.log")
	w, err := NewRotatingWriter(path, 4, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	for _, s := range []string{"aaaa", "bbbb", "cccc", "dddd"} {
		if _, err = w.Write([]byte(s)); err != nil {
			t.Fatal(err)
		}
	}
	for name, want := range map[string]string{path: "dddd", path + ".1": "cccc", path + ".2": "bbbb"} {
		got, err := os.ReadFile(name)
		if err != nil || string(got) != want {
			t.Fatalf("%s: %q %v", name, got, err)
		}
		st, _ := os.Stat(name)
		if st.Mode().Perm() != 0600 {
			t.Fatalf("permissions %v", st.Mode())
		}
	}
	if _, err := os.Stat(path + ".3"); !os.IsNotExist(err) {
		t.Fatalf("extra backup: %v", err)
	}
}
func TestConcurrentTranscripts(t *testing.T) {
	dir := t.TempDir()
	r, err := NewRecorder(dir)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Go(func() { r.Record(map[string]any{"prompt": "hello\nworld", "response": "ok"}) })
	}
	wg.Wait()
	r.Close()
	f, err := os.Open(filepath.Join(dir, "llm", "interactions.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	for i := 0; i < 30; i++ {
		var v map[string]any
		if err := dec.Decode(&v); err != nil {
			t.Fatal(err)
		}
		if v["prompt"] != "hello\nworld" {
			t.Fatal(v)
		}
	}
}
func TestLoggingLevels(t *testing.T) {
	for _, debug := range []bool{false, true} {
		dir := t.TempDir()
		close, err := Configure(dir, debug)
		if err != nil {
			t.Fatal(err)
		}
		slog.Debug("debug marker")
		slog.Info("info marker")
		close()
		data, err := os.ReadFile(filepath.Join(dir, "kamacu.log"))
		if err != nil {
			t.Fatal(err)
		}
		hasDebug := strings.Contains(string(data), "debug marker")
		if hasDebug != debug {
			t.Fatalf("debug=%v log=%s", debug, data)
		}
	}
}

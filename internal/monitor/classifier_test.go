package monitor

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestCLISharedWorkingDirectory(t *testing.T) {
	// Exercise a symlinked cache path, as macOS temporary paths commonly use
	// /var while the child process reports the physical /private/var path.
	cachePath := filepath.Join(t.TempDir(), "cache")
	if err := os.Symlink(t.TempDir(), cachePath); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", cachePath)
	t.Setenv("XDG_CACHE_HOME", cachePath)
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	wantDir := filepath.Join(cacheDir, "kamacu", "classifier")
	bin := filepath.Join(t.TempDir(), "fixture")
	// Report the actual cwd, settings, and schema seen by the child process.
	const script = `#!/bin/sh
set -eu
backend=codex
schema_path=
settings=null
while [ "$#" -gt 0 ]; do
  case "$1" in
    -p) backend=claude ;;
    --settings) shift; settings=$1 ;;
    --json-schema) shift; schema=$1 ;;
    --output-schema) shift; schema_path=$1 ;;
  esac
  shift
done
cat >/dev/null
if [ -n "$schema_path" ]; then
  schema=$(cat "$schema_path")
fi
if [ "$backend" = claude ]; then
  printf '{"structured_output":'
fi
printf '{"cwd":"%s","schema_path":"%s","schema":%s,"settings":%s}' "$PWD" "$schema_path" "$schema" "$settings"
if [ "$backend" = claude ]; then
  printf '}'
fi
`
	if err := os.WriteFile(bin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	schemaPaths := make(chan string, 24)
	// Two batches check that the directory survives calls as well as supporting
	// concurrent creation and use by both backends with different schemas.
	for range 2 {
		var wg sync.WaitGroup
		for _, backend := range []string{"claude_cli", "codex_cli"} {
			for i := range 12 {
				wg.Go(func() {
					var usage *Usage
					raw, err := runCLIJSON(ctx, Profile{Backend: backend, Executable: bin}, "prompt", map[string]int{"call": i}, &usage)
					if err != nil {
						t.Errorf("%s call %d: %v", backend, i, err)
						return
					}
					var got struct {
						CWD        string `json:"cwd"`
						SchemaPath string `json:"schema_path"`
						Schema     map[string]int
						Settings   map[string]bool
					}
					if err := json.Unmarshal(raw, &got); err != nil {
						t.Errorf("%s: %s: %v", backend, raw, err)
						return
					}
					wantInfo, err := os.Stat(wantDir)
					if err != nil {
						t.Error(err)
						return
					}
					gotInfo, err := os.Stat(got.CWD)
					if err != nil || !os.SameFile(wantInfo, gotInfo) {
						t.Errorf("%s call %d: unexpected cwd %q, want %q (%v)", backend, i, got.CWD, wantDir, err)
					}
					if got.Schema["call"] != i || len(got.Schema) != 1 {
						t.Errorf("%s call %d: unexpected schema: %s", backend, i, raw)
					}
					if backend == "claude_cli" {
						memory, present := got.Settings["autoMemoryEnabled"]
						if !got.Settings["disableAllHooks"] || !present || memory {
							t.Errorf("Claude settings: %v", got.Settings)
						}
					} else {
						if got.SchemaPath == "" || filepath.Dir(got.SchemaPath) == wantDir {
							t.Errorf("schema must be outside cwd: %q", got.SchemaPath)
						}
						if _, err := os.Stat(got.SchemaPath); !os.IsNotExist(err) {
							t.Errorf("schema not removed after call: %s (%v)", got.SchemaPath, err)
						}
						schemaPaths <- got.SchemaPath
					}
				})
			}
		}
		wg.Wait()
		entries, err := os.ReadDir(wantDir)
		if err != nil || len(entries) != 0 {
			t.Fatalf("shared cwd should remain empty: %v (%v)", entries, err)
		}
	}
	close(schemaPaths)
	seen := map[string]bool{}
	for path := range schemaPaths {
		if seen[path] {
			t.Errorf("schema path reused: %s", path)
		}
		seen[path] = true
	}
}

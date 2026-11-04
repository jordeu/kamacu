package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/subcommands"
)

func TestStartupDiagnosticsPersist(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		remote, legacy, both bool
		want                 string
	}{
		{name: "invalid address", want: "refusing to start"},
		{name: "remote warning", remote: true, want: "NO authentication"},
		{name: "legacy invalid address", legacy: true, want: "refusing to start"},
		{name: "both data directories", remote: true, legacy: true, both: true, want: "creating db directory"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			logDir := filepath.Join(home, ".kamacu")
			if tc.legacy {
				if err := os.Mkdir(filepath.Join(home, ".kangent"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			if tc.both {
				if err := os.Mkdir(logDir, 0700); err != nil {
					t.Fatal(err)
				}
			}
			// Force an early database directory failure before starting any workers.
			blocked := filepath.Join(home, "blocked")
			if err := os.WriteFile(blocked, []byte("file"), 0600); err != nil {
				t.Fatal(err)
			}
			c := serveCmd{addr: "0.0.0.0:7333", db: filepath.Join(blocked, "db"), insecureAllowRemote: tc.remote, noIntegrationSync: true}
			if got := c.Execute(context.Background(), nil); got != subcommands.ExitFailure {
				t.Fatalf("exit %v", got)
			}
			data, err := os.ReadFile(filepath.Join(logDir, "kamacu.log"))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(data), tc.want) {
				t.Fatalf("missing %q in %s", tc.want, data)
			}
			if tc.legacy {
				if _, err := os.Stat(filepath.Join(home, ".kangent", "kamacu.log")); !os.IsNotExist(err) {
					t.Fatalf("legacy directory was modified: %v", err)
				}
			}
		})
	}
}

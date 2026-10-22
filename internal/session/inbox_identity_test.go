package session

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestAgentInboxIdentityOverridesInheritedEnvironment(t *testing.T) {
	t.Setenv("KAMACU_TASK_ID", "999")
	t.Setenv("KAMACU_SESSION_ID", "stale")
	for _, taskID := range []int64{0, 7} {
		for _, engine := range []string{"claude", "codex", "opencode", "custom"} {
			t.Run(engine+"/"+strconv.FormatInt(taskID, 10), func(t *testing.T) {
				dump := filepath.Join(t.TempDir(), "environment")
				stub := filepath.Join(t.TempDir(), "agent")
				if err := os.WriteFile(stub, []byte("#!/bin/sh\nenv > '"+dump+"'\necho STUB_READY >> '"+dump+"'\nsleep 30\n"), 0700); err != nil {
					t.Fatal(err)
				}
				mgr := NewManager()
				mgr.SetAgentConfig(AgentConfig{ClaudeBin: stub, Token: "token", BaseURL: "http://127.0.0.1:7333"})
				sess := spawnForTestOpts(t, mgr, SpawnOpts{Kind: KindAgent, TaskID: taskID, Global: taskID == 0, Cwd: t.TempDir(), AgentEngine: engine, AgentArgs: []string{stub}})
				waitForStubReady(t, dump)
				if value, _ := envValue(t, dump, "KAMACU_TASK_ID"); value != strconv.FormatInt(taskID, 10) {
					t.Fatal("incorrect task identity", value)
				}
				if value, _ := envValue(t, dump, "KAMACU_SESSION_ID"); value != sess.Info().ID {
					t.Fatal("incorrect session identity", value)
				}
			})
		}
	}

}

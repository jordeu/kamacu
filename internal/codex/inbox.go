package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"time"
)

// FeedbackEnvironmentArgs forwards identity to registered local Kamacu MCP
// servers. Codex filters child environments; only variable names belong in
// argv, never credentials or session values. User configuration stays untouched.
func FeedbackEnvironmentArgs(bin, dir string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "mcp", "list", "--json")
	cmd.Dir = dir
	cmd.WaitDelay = 100 * time.Millisecond
	data, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("inspect Codex MCP registrations: %w", err)
	}
	return feedbackEnvironmentArgs(data)
}

func feedbackEnvironmentArgs(data []byte) ([]string, error) {
	var entries []struct {
		Name      string `json:"name"`
		Enabled   bool   `json:"enabled"`
		Transport struct {
			Type    string   `json:"type"`
			Command string   `json:"command"`
			Args    []string `json:"args"`
			EnvVars []string `json:"env_vars"`
		} `json:"transport"`
	}
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("invalid Codex MCP registration response")
	}
	var args []string
	for _, entry := range entries {
		t := entry.Transport
		isKamacu := entry.Name == "kamacu" || (filepath.Base(t.Command) == "kamacu" && slices.Contains(t.Args, "mcp") && slices.Contains(t.Args, "serve"))
		if !entry.Enabled || t.Type != "stdio" || !isKamacu {
			continue
		}
		vars := slices.Clone(t.EnvVars)
		for _, name := range []string{"KAMACU_TASK_ID", "KAMACU_SESSION_ID", "KAMACU_HOOK_TOKEN", "KAMACU_HOOK_BASE"} {
			if !slices.Contains(vars, name) {
				vars = append(vars, name)
			}
		}
		// Codex splits -c keys on dots and keeps quotes literally, so the name
		// must stay bare; Codex already restricts server names to [A-Za-z0-9_-].
		value, _ := json.Marshal(vars)
		args = append(args, "-c", "mcp_servers."+entry.Name+".env_vars="+string(value))
	}
	return args, nil
}

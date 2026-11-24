package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"kamacu/internal/settings"
)

func validateModelConfig(models []string, argument string) (string, error) {
	if argument != "" {
		tokens := settings.Tokenize(argument)
		valid := false
		if len(tokens) == 1 {
			flag, value, inline := strings.Cut(tokens[0], "=")
			valid = inline && strings.HasPrefix(flag, "-") && !strings.Contains(flag, "{{model}}") && strings.Contains(value, "{{model}}")
		} else if len(tokens) == 2 {
			valid = strings.HasPrefix(tokens[0], "-") && !strings.ContainsAny(tokens[0], "={}") && strings.Contains(tokens[1], "{{model}}")
		}
		if !valid || strings.Count(argument, "{{model}}") != 1 || strings.ContainsRune(argument, 0) {
			return "", errors.New("model argument must be a flag with one {{model}} value, such as --model {{model}} or --model={{model}}")
		}
	}

	if len(models) > 200 {
		return "", errors.New("at most 200 models are allowed")
	}
	clean := []string{}
	seen := map[string]bool{}
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" || len(model) > 256 || strings.ContainsAny(model, "\x00\r\n") {
			return "", errors.New("model IDs must be nonempty and at most 256 characters")
		}
		if !seen[model] {
			clean = append(clean, model)
			seen[model] = true
		}
	}
	b, err := json.Marshal(clean)
	return string(b), err
}

type taskAgentValidationError string

func (e taskAgentValidationError) Error() string { return string(e) }

func sameAgentOverride(a, b *int64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// A task with an explicit model pins its agent too, so changing a project's
// default later cannot send that model to a different agent. Existing clients
// that omit both fields keep inheriting the project's agent.
func validateTaskAgent(db *sql.DB, projectID int64, agentID *int64, model string) (*int64, string, error) {
	model = strings.TrimSpace(model)
	if agentID == nil && model == "" {
		return nil, "", nil
	}
	var id int64
	if agentID != nil {
		id = *agentID
	} else if err := db.QueryRow(`SELECT agent_id FROM projects WHERE id = ?`, projectID).Scan(&id); err != nil {
		return nil, "", err
	}
	var modelsJSON, argument string
	if err := db.QueryRow(`SELECT models, model_argument FROM agents WHERE id = ?`, id).Scan(&modelsJSON, &argument); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, "", taskAgentValidationError("agent not found")
		}
		return nil, "", err
	}
	if model != "" {
		if argument == "" {
			return nil, "", taskAgentValidationError("the selected agent has no model argument configured")
		}
		var models []string
		if err := json.Unmarshal([]byte(modelsJSON), &models); err != nil {
			return nil, "", err
		}
		found := false
		for _, candidate := range models {
			found = found || candidate == model
		}
		if !found {
			return nil, "", taskAgentValidationError("model is not in the selected agent's configured list")
		}
	}
	return &id, model, nil
}

// PATCH accepts null to restore the project default, and an empty model to
// restore the CLI default. Changing agents without a model clears the previous
// model; model-only edits keep the task's existing agent.
func resolveTaskAgentUpdate(db *sql.DB, task Task, rawAgent json.RawMessage, model *string) (*int64, string, bool, error) {
	agentID, nextModel := task.AgentID, task.Model
	if len(rawAgent) > 0 {
		var replacement *int64
		if err := json.Unmarshal(rawAgent, &replacement); err != nil {
			return nil, "", false, taskAgentValidationError("agent_id must be an integer or null")
		}
		agentID = replacement
	}
	var projectAgent int64
	if err := db.QueryRow(`SELECT agent_id FROM projects WHERE id = ?`, task.ProjectID).Scan(&projectAgent); err != nil {
		return nil, "", false, err
	}
	previous, next := projectAgent, projectAgent
	if task.AgentID != nil {
		previous = *task.AgentID
	}
	if agentID != nil {
		next = *agentID
	}
	if model != nil {
		nextModel = strings.TrimSpace(*model)
	} else if previous != next {
		nextModel = ""
	}
	changed := previous != next || task.Model != nextModel
	if changed {
		resolved, validated, err := validateTaskAgent(db, task.ProjectID, agentID, nextModel)
		return resolved, validated, true, err
	}
	// Retain a saved model even if it has since been removed from suggestions.
	if agentID == nil && nextModel != "" {
		agentID = &next
	}
	return agentID, nextModel, false, nil
}

// Split the template before substitution: a model ID is always literal argv,
// including spaces and shell metacharacters. Replace existing occurrences of
// the configured flag rather than relying on a CLI's duplicate-flag behavior.
func applyModelArgument(args []string, template, model, engine string) []string {
	if model == "" || template == "" {
		return args
	}
	tokens := settings.Tokenize(template)
	if len(tokens) == 0 {
		return args
	}
	flag, pattern, templateInline := strings.Cut(tokens[0], "=")
	if !templateInline {
		if len(tokens) != 2 {
			return args
		}
		pattern = tokens[1]
	}
	prefix, suffix, _ := strings.Cut(pattern, "{{model}}")
	matchesValue := func(value string) bool {
		return len(value) >= len(prefix)+len(suffix) && strings.HasPrefix(value, prefix) && strings.HasSuffix(value, suffix)
	}
	out := make([]string, 0, len(args)+len(tokens))
	var positional []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--" {
			positional = args[i:]
			break
		}
		key, value, inline := strings.Cut(args[i], "=")
		match := key == flag
		if (engine == "claude" || engine == "codex" || engine == "opencode") && flag == "--model" {
			match = match || key == "-m"
		}
		if !inline && i+1 < len(args) && args[i+1] != "--" {
			value = args[i+1]
		}
		if match && matchesValue(value) {
			if !inline && i+1 < len(args) && args[i+1] != "--" {
				i++
			}
			continue
		}
		out = append(out, args[i])
	}

	for _, token := range tokens {
		out = append(out, strings.ReplaceAll(token, "{{model}}", model))
	}
	return append(out, positional...)
}

// Discovery is deliberately separate from configuration: this endpoint never
// writes agents.models. The user chooses which suggestions to save.
func (h *agentCRUDHandlers) discoverModels(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Engine           string `json:"engine"`
		Command          string `json:"command"`
		DiscoveryCommand string `json:"model_discovery_command"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	args := settings.Tokenize(req.Command)
	if strings.TrimSpace(req.DiscoveryCommand) != "" {
		args = settings.Tokenize(req.DiscoveryCommand)
	} else {
		if len(args) == 0 {
			writeError(w, http.StatusBadRequest, "agent command is required")
			return
		}
		// Do not silently discard provider/profile options or command wrappers:
		// the resulting catalog could describe a different configuration.
		if len(args) != 1 {
			writeError(w, http.StatusBadRequest, "agent command has launch arguments; configure a discovery command for the intended provider and settings")
			return
		}
		switch req.Engine {
		case "codex":
			args = append(args, "debug", "models")
		case "opencode":
			args = append(args, "models")
		case "claude":
			// The initialization control request is the CLI protocol underlying
			// Agent SDK supportedModels(). No user prompt or inference is sent.
			args = append(args, "--print", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--no-session-persistence", "--strict-mcp-config", "--mcp-config", `{"mcpServers":{}}`)
		default:
			writeError(w, http.StatusBadRequest, "configure a discovery command for this custom agent, or add models manually")
			return
		}
	}
	if len(args) == 0 {
		writeError(w, http.StatusBadRequest, "discovery command is required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	// Match agent launch isolation: custom catalog tools must not inherit
	// another Kamacu instance's hook credentials. Claude discovery also works
	// when Kamacu was started from a Claude Code terminal.
	for _, value := range os.Environ() {
		key, _, _ := strings.Cut(value, "=")
		if !strings.HasPrefix(key, "KAMACU_") && key != "CLAUDECODE" {
			cmd.Env = append(cmd.Env, value)
		}
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = time.Second
	if req.Engine == "claude" && strings.TrimSpace(req.DiscoveryCommand) == "" {
		cmd.Stdin = strings.NewReader("{\"type\":\"control_request\",\"request_id\":\"models\",\"request\":{\"subtype\":\"initialize\"}}\n")
	}
	var output catalogOutput
	cmd.Stdout = &output
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			writeError(w, http.StatusGatewayTimeout, "model discovery timed out; you can still add models manually")
		} else {
			writeError(w, http.StatusBadGateway, "model discovery failed: "+err.Error())
		}
		return
	}
	models, err := parseDiscoveredModels(output.Bytes())
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": models})
}

// Bound subprocess output before buffering it, including for custom commands.
type catalogOutput struct{ bytes.Buffer }

func (b *catalogOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 2<<20 {
		return 0, errors.New("model catalog exceeds 2 MiB")
	}
	return b.Buffer.Write(p)
}

func parseDiscoveredModels(data []byte) ([]string, error) {
	// Codex's catalog and user discovery commands may return JSON. OpenCode
	// and custom commands may instead return one model ID per line.
	var catalog struct {
		Models []struct {
			Slug       string `json:"slug"`
			Visibility string `json:"visibility"`
		} `json:"models"`
	}
	var models []string
	if json.Unmarshal(data, &models) == nil && models != nil {
		// A custom discovery command returning a JSON array of model IDs.
	} else if json.Unmarshal(data, &catalog) == nil && catalog.Models != nil {
		models = []string{}
		for _, m := range catalog.Models {
			if m.Visibility == "" || m.Visibility == "list" {
				models = append(models, m.Slug)
			}
		}
	} else {
		models = []string{}
		for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if strings.HasPrefix(line, "{") {
				var event struct {
					Type     string `json:"type"`
					Response struct {
						Response struct {
							Models []struct {
								Value string `json:"value"`
							} `json:"models"`
						} `json:"response"`
					} `json:"response"`
				}
				if err := json.Unmarshal([]byte(line), &event); err != nil {
					return nil, errors.New("invalid model discovery response")
				}
				if event.Type == "control_response" {
					for _, m := range event.Response.Response.Models {
						models = append(models, m.Value)
					}
				}
			} else if strings.HasPrefix(line, "[") || strings.ContainsAny(line, " \t\x1b") || line == "null" || line == "true" || line == "false" {
				return nil, errors.New("discovery must return model IDs as a JSON array or one per line")
			} else {
				models = append(models, line)
			}
		}
	}
	if len(models) == 0 {
		return nil, errors.New("no models were discovered; you can add models manually")
	}
	// Discovery may return far more models than the curated list allows.
	// Do not prevent browsing a provider's catalog to choose a small subset.
	if len(models) > 10000 {
		return nil, errors.New("model catalog is too large")
	}
	clean := []string{}
	seen := map[string]bool{}
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" || len(model) > 256 || strings.ContainsAny(model, "\x00\r\n") {
			return nil, errors.New("invalid discovered model ID")
		}
		if !seen[model] {
			clean = append(clean, model)
			seen[model] = true
		}
	}
	return clean, nil
}

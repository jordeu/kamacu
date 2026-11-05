package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"kamacu/internal/monitor"
	"kamacu/internal/settings"
)

type replayInference struct {
	backend   monitor.Backend
	prompts   []string
	responses []json.RawMessage
}

func (m *replayInference) GenerateValidatedJSON(ctx context.Context, profile monitor.Profile, prompt string, schema any, validate func([]byte) error) ([]byte, error) {
	m.prompts = append(m.prompts, prompt)
	raw, err := m.backend.GenerateValidatedJSON(ctx, profile, prompt, schema, validate)
	m.responses = append(m.responses, append(json.RawMessage(nil), raw...))
	return raw, err
}

// Opt-in replay uses frozen local traces and never calls GitHub. Model calls
// are disabled unless KAMACU_NOTIFICATION_REPLAY_MODEL is explicitly supplied.
// That mode sends trace excerpts to the configured Claude CLI provider.
// KAMACU_NOTIFICATION_REPLAY_POLICY_FILE supplies an absolute path to local preferences;
// otherwise use empty preferences. An optional REPLAY_VIEWER is used when the
// snapshot lacks verified viewer identity. Private traces stay outside fixtures.
// It exercises the production policy, windowing, prompt and output validator.
// The synthetic needs_context answer measures input shape, not model accuracy.
func TestNotificationTraceReplay(t *testing.T) {
	root := os.Getenv("KAMACU_NOTIFICATION_TRACE_ROOT")
	if root == "" {
		t.Skip("set KAMACU_NOTIFICATION_TRACE_ROOT for offline replay")
	}
	s, model, _, _ := fixture(t)
	respondWithIntent(model, "needs_context")
	prompts := &model.prompts
	var live *replayInference
	if requested := os.Getenv("KAMACU_NOTIFICATION_REPLAY_MODEL"); requested != "" {
		if _, err := s.DB.Exec(`UPDATE monitor_profiles SET backend='claude_cli',model=? WHERE id=1`, requested); err != nil {
			t.Fatal(err)
		}
		live = &replayInference{}
		s.Model = live
		prompts = &live.prompts
	}
	policy := settings.NotificationPolicy{}
	if path := os.Getenv("KAMACU_NOTIFICATION_REPLAY_POLICY_FILE"); path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		policy, err = settings.ParseNotificationPolicy(string(raw))
		if err != nil {
			t.Fatal(err)
		}
	}
	result := struct {
		Mode            string            `json:"mode"`
		Events          int               `json:"events"`
		Subjects        int               `json:"subjects"`
		PolicyReasons   map[string]int    `json:"policy_reasons"`
		ClassifierCalls int               `json:"classifier_calls"`
		PromptBytes     int               `json:"prompt_bytes"`
		LargestPrompt   int               `json:"largest_prompt_bytes"`
		ModelInference  bool              `json:"model_inference"`
		InferenceModel  string            `json:"inference_model,omitempty"`
		Responses       []json.RawMessage `json:"responses,omitempty"`
	}{Mode: "production-code replay with synthetic inspection answers; not accuracy evaluation", PolicyReasons: map[string]int{}}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Name() != "prompt.md" {
			return nil
		}
		text, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		response, err := os.ReadFile(filepath.Join(filepath.Dir(path), "response.md"))
		if err != nil {
			return err
		}
		// Only final group decisions, not repeated read-tool steps or reply execution.
		decisionText := strings.TrimSpace(string(response))
		decisionText = strings.TrimPrefix(decisionText, "```json\n")
		decisionText = strings.TrimSuffix(decisionText, "```")
		var decision struct {
			ToolName string `json:"tool_name"`
		}
		if err = json.Unmarshal([]byte(decisionText), &decision); err != nil {
			return err
		}
		if decision.ToolName != "" {
			return nil
		}
		prefix, raw, ok := strings.Cut(string(text), "SNAPSHOT (data):\n")
		if !ok {
			return nil
		}
		var snapshot struct {
			Notifications []RawNotification `json:"notifications"`
			Subjects      map[string]struct {
				Title    string   `json:"title"`
				State    string   `json:"state"`
				Excerpt  string   `json:"excerpt"`
				Facts    *PRFacts `json:"pr_facts"`
				Warnings []string `json:"context_warnings"`
			} `json:"subject_context"`
		}
		if err = json.NewDecoder(strings.NewReader(raw)).Decode(&snapshot); err != nil {
			return err
		}
		_, custom, _ := strings.Cut(prefix, "GLOBAL NOTIFICATION INSTRUCTIONS:\n")
		groups := map[string]*triageSubject{}
		for _, raw := range snapshot.Notifications {
			key := subjectKey(raw.SourceID, raw.Event)
			subject := groups[key]
			if subject == nil {
				fresh := raw.Event
				context := snapshot.Subjects[key]
				fresh.Context = context.Excerpt
				fresh.SubjectTitle = context.Title
				fresh.SubjectState = context.State
				fresh.PRFacts = context.Facts
				fresh.ContextWarnings = context.Warnings
				viewer := os.Getenv("KAMACU_NOTIFICATION_REPLAY_VIEWER")
				if fresh.PRFacts != nil && fresh.PRFacts.Viewer != "" {
					viewer = fresh.PRFacts.Viewer
				}
				subject = &triageSubject{fresh: fresh, viewer: viewer}
				groups[key] = subject
			}
			subject.events = append(subject.events, raw)
		}
		keys := make([]string, 0, len(groups))
		for key := range groups {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		for _, key := range keys {
			subject := groups[key]
			result.Subjects++
			result.Events += len(subject.events)
			reason := policyReason(policy, subject.events, subject.fresh, subject.viewer)
			if reason == "" {
				reason = "semantic"
				if err = s.classifySubject(context.Background(), subject, custom); err != nil {
					return err
				}
				applyIntentPolicy(subject, policy)
			}
			result.PolicyReasons[reason] += len(subject.events)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if live != nil {
		result.Mode = "production-code replay with live model inference; recorded outputs are not human gold labels"
		result.ModelInference = true
		result.InferenceModel = os.Getenv("KAMACU_NOTIFICATION_REPLAY_MODEL")
		result.Responses = live.responses
	}
	for _, prompt := range *prompts {
		result.ClassifierCalls++
		result.PromptBytes += len(prompt)
		if len(prompt) > result.LargestPrompt {
			result.LargestPrompt = len(prompt)
		}
		if len(prompt) > projectPromptLimit || strings.Contains(prompt, "AVAILABLE READ-ONLY MCP TOOLS") {
			t.Fatal("replay violated classifier contract")
		}
	}
	if result.Events == 0 || result.Subjects == 0 {
		t.Fatal("trace root contains no completed notification groups")
	}
	data, _ := json.MarshalIndent(result, "", "  ")
	t.Log(string(data))
	if output := os.Getenv("KAMACU_NOTIFICATION_REPLAY_OUTPUT"); output != "" {
		if err = os.WriteFile(output, append(data, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
		inputs := filepath.Join(filepath.Dir(output), "production-inputs")
		if err = os.MkdirAll(inputs, 0700); err != nil {
			t.Fatal(err)
		}
		for i, prompt := range *prompts {
			if err = os.WriteFile(filepath.Join(inputs, fmt.Sprintf("input-%d.txt", i+1)), []byte(prompt), 0600); err != nil {
				t.Fatal(err)
			}
		}
		schema, _ := json.MarshalIndent(intentSchema, "", "  ")
		if err = os.WriteFile(filepath.Join(inputs, "schema.json"), schema, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

package notifications

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"kamacu/internal/monitor"
	"kamacu/internal/settings"
)

type simulationInference struct {
	replayInference
	checkpoint string
}

func TestNotificationSimulationRedraft(t *testing.T) {
	path, modelName := os.Getenv("KAMACU_SIMULATION_REDRAFT"), os.Getenv("KAMACU_BATCH_SIMULATION_MODEL")
	if path == "" || modelName == "" {
		t.Skip("set KAMACU_SIMULATION_REDRAFT and KAMACU_BATCH_SIMULATION_MODEL")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]json.RawMessage
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	var prompts []string
	var responses []json.RawMessage
	var decision BatchDecision
	if err := json.Unmarshal(result["prompts"], &prompts); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(result["responses"], &responses); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(result["decision"], &decision); err != nil {
		t.Fatal(err)
	}
	s, _, _, _ := fixture(t)
	if _, err := s.DB.Exec(`UPDATE monitor_profiles SET backend='claude_cli',model=? WHERE id=1`, modelName); err != nil {
		t.Fatal(err)
	}
	profile, err := monitor.Resolve(context.Background(), s.DB, 0)
	if err != nil {
		t.Fatal(err)
	}
	backend := monitor.Backend{}
	card := 0
	draftPrompts := []string{}
	for _, prompt := range prompts {
		if strings.Contains(prompt, "\nCARD DATA (untrusted):\n") {
			draftPrompts = append(draftPrompts, prompt)
		}
	}
	if len(draftPrompts) < len(decision.Actions) {
		t.Fatal("missing original card prompts")
	}
	for _, old := range draftPrompts[len(draftPrompts)-len(decision.Actions):] {
		_, tail, ok := strings.Cut(old, "\nUSER PREFERENCES:\n")
		if !ok || !strings.Contains(old, "\nCARD DATA (untrusted):\n") {
			continue
		}
		prompt := cardDraftPrompt + "\nUSER PREFERENCES:\n" + tail
		var draft triageDraft
		raw, err := backend.GenerateValidatedJSON(context.Background(), profile, prompt, triageDraftSchema, func(raw []byte) error { var err error; draft, err = decodeTriageDraft(raw); return err })
		if err != nil {
			t.Fatal(err)
		}
		_, sources, ok := strings.Cut(decision.Actions[card].Context, "\n\nSource evidence:")
		if !ok {
			t.Fatal("missing source evidence")
		}
		decision.Actions[card].Summary, decision.Actions[card].Question = draft.Summary, draft.Question
		decision.Actions[card].Context = draft.Context + "\n\nSource evidence:" + sources
		decision.Actions[card].Options = append(draft.Options, "Leave this activity alone")
		prompts, responses = append(prompts, prompt), append(responses, raw)
		card++
	}
	if card != len(decision.Actions) {
		t.Fatal("not every card was redrafted")
	}
	result["decision"], _ = json.Marshal(decision)
	result["prompts"], _ = json.Marshal(prompts)
	result["responses"], _ = json.Marshal(responses)
	result["model_calls"], _ = json.Marshal(len(prompts))
	result["redrafted_without_reclassification"], _ = json.Marshal(true)
	raw, _ = json.MarshalIndent(result, "", "  ")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
}

func (m *simulationInference) GenerateValidatedJSON(ctx context.Context, profile monitor.Profile, prompt string, schema any, validate func([]byte) error) ([]byte, error) {
	raw, err := m.replayInference.GenerateValidatedJSON(ctx, profile, prompt, schema, validate)
	detail := ""
	if err != nil {
		detail = err.Error()
	}
	checkpoint, _ := json.MarshalIndent(map[string]any{"prompts": m.prompts, "responses": m.responses, "last_error": detail}, "", "  ")
	if writeErr := os.WriteFile(m.checkpoint, checkpoint, 0600); writeErr != nil {
		return nil, writeErr
	}
	return raw, err
}

// The input is a local export of the original raw notifications plus captured
// enrichment, not a copy of the application's writable database.
func TestNotificationBatchSimulation(t *testing.T) {
	input, output, modelName := os.Getenv("KAMACU_BATCH_SIMULATION_INPUT"), os.Getenv("KAMACU_BATCH_SIMULATION_OUTPUT"), os.Getenv("KAMACU_BATCH_SIMULATION_MODEL")
	if input == "" || output == "" || modelName == "" {
		t.Skip("set KAMACU_BATCH_SIMULATION_INPUT, OUTPUT and MODEL for isolated batch simulation")
	}
	var snapshot struct {
		BatchID     string                      `json:"batch_id"`
		Preferences string                      `json:"preferences"`
		Policy      settings.NotificationPolicy `json:"policy"`
		Limitations []string                    `json:"limitations"`
		Subjects    []struct {
			Viewer string            `json:"viewer"`
			Events []RawNotification `json:"events"`
			Fresh  Event             `json:"fresh"`
		} `json:"subjects"`
	}
	raw, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		t.Fatal(err)
	}
	s, _, _, _ := fixture(t)
	if _, err := s.DB.Exec(`UPDATE monitor_profiles SET backend='claude_cli',model=? WHERE id=1`, modelName); err != nil {
		t.Fatal(err)
	}
	model := &simulationInference{checkpoint: output + ".calls.json"}
	s.Model, s.Tools = model, nil
	policy := applyGroupingInstruction(snapshot.Policy, snapshot.Preferences)
	sort.SliceStable(snapshot.Subjects, func(i, j int) bool {
		return snapshot.Subjects[i].Fresh.Repository < snapshot.Subjects[j].Fresh.Repository
	})
	subjects := []*triageSubject{}
	notifications := 0
	for _, item := range snapshot.Subjects {
		subject := &triageSubject{viewer: item.Viewer, events: item.Events, fresh: item.Fresh}
		notifications += len(item.Events)
		t.Logf("classifying %s (%d notifications)", item.Fresh.URL, len(item.Events))
		subject.reason = policyReason(policy, subject.events, subject.fresh, subject.viewer)
		if strings.HasPrefix(subject.reason, "needs_context:") {
			subject.findings = []intentFinding{{Intent: "needs_context"}}
		} else if subject.reason == "" {
			if err := s.classifySubject(context.Background(), subject, snapshot.Preferences); err != nil {
				t.Fatal(err)
			}
			applyIntentPolicy(subject, policy)
		}
		subjects = append(subjects, subject)
	}
	decision, err := s.renderTriage(context.Background(), subjects, nil, policy)
	if err != nil {
		t.Fatal(err)
	}
	beforeDraft := len(model.prompts)
	if err := s.draftTriageCards(context.Background(), &decision, subjects, snapshot.Preferences); err != nil {
		t.Fatal(err)
	}
	if len(model.prompts)-beforeDraft != len(decision.Actions) {
		t.Fatal("not every card reached the model drafter")
	}
	for _, action := range decision.Actions {
		if !strings.Contains(action.Context, "\n\nSource evidence:") {
			t.Fatal("card drafting fell back instead of producing a model response")
		}
	}
	allowed := map[int64]RawNotification{}
	for _, subject := range subjects {
		for _, event := range subject.events {
			allowed[event.ID] = event
		}
	}
	for i := range decision.Actions {
		if err := fitTriageContext(&decision.Actions[i], allowed); err != nil {
			t.Fatal(err)
		}
	}
	all := []RawNotification{}
	for _, subject := range subjects {
		all = append(all, subject.events...)
	}
	if _, _, err := validateBatch(decision, all, nil); err != nil {
		t.Fatal(err)
	}
	type disposition struct {
		URL           string          `json:"url"`
		Notifications int             `json:"notifications"`
		Findings      []intentFinding `json:"findings"`
		Reason        string          `json:"reason"`
	}
	dispositions := []disposition{}
	for _, subject := range subjects {
		dispositions = append(dispositions, disposition{subject.fresh.URL, len(subject.events), subject.findings, subject.reason})
	}
	result, _ := json.MarshalIndent(map[string]any{"batch_id": snapshot.BatchID, "notifications": notifications, "subjects": dispositions, "decision": decision, "model_calls": len(model.prompts), "limitations": snapshot.Limitations, "prompts": model.prompts, "responses": model.responses}, "", "  ")
	if err := os.WriteFile(output, result, 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("SIMULATION: %d notifications, %d subjects, %d cards, %d discarded notifications; output %s", notifications, len(subjects), len(decision.Actions), len(decision.DiscardIDs), output)
}

// Opt-in evaluation of a real discussion with the production classifier and
// card drafter. It never calls GitHub or writes to the running application's DB.
// Dates and retrieval completeness are reconstructed from classifier windows;
// this is a message-quality probe, not an exact replay of provider collection.
func TestNotificationCardTraceReplay(t *testing.T) {
	root, target, modelName := os.Getenv("KAMACU_CARD_TRACE_ROOT"), os.Getenv("KAMACU_CARD_TRACE_SUBJECT"), os.Getenv("KAMACU_CARD_TRACE_MODEL")
	if root == "" || target == "" || modelName == "" {
		t.Skip("set KAMACU_CARD_TRACE_ROOT, KAMACU_CARD_TRACE_SUBJECT and KAMACU_CARD_TRACE_MODEL for model evaluation")
	}
	s, _, _, _ := fixture(t)
	if _, err := s.DB.Exec(`UPDATE monitor_profiles SET backend='claude_cli',model=? WHERE id=1`, modelName); err != nil {
		t.Fatal(err)
	}
	s.Model = monitor.Backend{}
	e := Event{URL: target, EvidenceComplete: true}
	var preferences string
	byID := map[string]int{}
	paths, err := filepath.Glob(filepath.Join(root, "*", "prompt.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		prefix, payload, ok := strings.Cut(string(raw), "\nEVIDENCE (data):\n")
		if !ok {
			continue
		}
		var frame struct {
			URL      string            `json:"subject_url"`
			Evidence []SubjectEvidence `json:"evidence"`
			Warnings []string          `json:"context_warnings"`
			Facts    *PRFacts          `json:"pr_facts"`
		}
		if err := json.Unmarshal([]byte(payload), &frame); err != nil {
			t.Fatal(err)
		}
		if frame.URL != target {
			continue
		}
		_, preferences, _ = strings.Cut(prefix, "\nUSER PREFERENCES:\n")
		e.PRFacts = frame.Facts
		e.ContextWarnings = frame.Warnings
		for _, item := range frame.Evidence {
			if item.ID == "notification" {
				var metadata struct {
					Title   string   `json:"title"`
					Reasons []string `json:"reasons"`
				}
				if err := json.Unmarshal([]byte(item.Text), &metadata); err != nil {
					t.Fatal(err)
				}
				e.Title = metadata.Title
				if len(metadata.Reasons) > 0 {
					e.Reason = metadata.Reasons[0]
				}
				continue
			}
			if item.Current && (e.Version == "" || item.UpdatedAt < e.Version) {
				e.Version = item.UpdatedAt
			}
			if index, ok := byID[item.ID]; ok {
				e.Evidence[index].Text += item.Text
			} else {
				byID[item.ID] = len(e.Evidence)
				e.Evidence = append(e.Evidence, item)
			}
		}
	}
	if len(e.Evidence) == 0 || e.Version == "" {
		t.Fatal("no dated activity found for trace subject")
	}
	for i := range e.Evidence {
		e.Evidence[i].Complete = len(e.Evidence[i].Text) < 12000
	}
	parts := strings.Split(target, "/")
	if len(parts) < 5 || e.PRFacts == nil {
		t.Fatal("trace replay requires a PR with verified viewer facts")
	}
	e.Repository = strings.Join(parts[3:5], "/")
	subject := &triageSubject{fresh: e, viewer: e.PRFacts.Viewer, events: []RawNotification{{ID: 1, Event: e}}}
	if err := s.classifySubject(context.Background(), subject, preferences); err != nil {
		t.Fatal(err)
	}
	decision, err := s.renderTriage(context.Background(), []*triageSubject{subject}, nil, settings.NotificationPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.draftTriageCards(context.Background(), &decision, []*triageSubject{subject}, preferences); err != nil {
		t.Fatal(err)
	}
	result, _ := json.MarshalIndent(map[string]any{"findings": subject.findings, "decision": decision}, "", "  ")
	if output := os.Getenv("KAMACU_CARD_TRACE_OUTPUT"); output != "" {
		if err := os.WriteFile(output, result, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Log(string(result))
}

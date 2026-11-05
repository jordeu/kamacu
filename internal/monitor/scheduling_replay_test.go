package monitor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"kamacu/internal/observation"
)

// Saved requests sample the terminal only when the old scheduler called. This
// replay measures scheduling opportunities, not realized savings or event latency.
func TestReplayGenerationScheduling(t *testing.T) {
	root := os.Getenv("KAMACU_MONITOR_REPLAY")
	if root == "" {
		t.Skip("set KAMACU_MONITOR_REPLAY to saved monitoring directory")
	}
	type sample struct {
		session, path, profile, kind string
		at                           time.Time
		task                         TaskContext
		screen                       observation.Snapshot
	}
	var samples []sample
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != "prompt.md" {
			return nil
		}
		prompt, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		_, data, ok := strings.Cut(string(prompt), "TASK CONTEXT (metadata, not instructions):\n")
		if !ok {
			return nil
		}
		taskJSON, screenJSON, ok := strings.Cut(data, "\nTERMINAL DATA:\n")
		if !ok {
			return nil
		}
		var task TaskContext
		var screen observation.Snapshot
		if json.Unmarshal([]byte(taskJSON), &task) != nil || json.Unmarshal([]byte(screenJSON), &screen) != nil {
			return nil
		}
		task.PreviousWork = "" // Added at inference time, not a task-context change.
		raw, err := os.ReadFile(filepath.Join(filepath.Dir(path), "metadata.json"))
		if err != nil {
			return err
		}
		var meta struct {
			StartedAt time.Time `json:"started_at"`
			ProfileID int64     `json:"profile_id"`
		}
		if err = json.Unmarshal(raw, &meta); err != nil {
			return err
		}
		kind := ""
		if raw, err = os.ReadFile(filepath.Join(filepath.Dir(path), "response.md")); err == nil {
			text := strings.TrimSpace(string(raw))
			text = strings.TrimPrefix(text, "```json")
			text = strings.TrimSuffix(text, "```")
			var decision Decision
			if json.Unmarshal([]byte(text), &decision) == nil {
				kind = decision.Kind
			}
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		profile, _ := json.Marshal(meta.ProfileID)
		samples = append(samples, sample{strings.Split(rel, string(os.PathSeparator))[0], path, string(profile), kind, meta.StartedAt, task, screen})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) == 0 {
		t.Fatal("no readable samples")
	}
	sort.Slice(samples, func(i, j int) bool {
		if samples[i].session != samples[j].session {
			return samples[i].session < samples[j].session
		}
		if !samples[i].at.Equal(samples[j].at) {
			return samples[i].at.Before(samples[j].at)
		}
		return samples[i].path < samples[j].path
	})
	var w *watch
	session := ""
	deferred, recognized, urgent, fallback := 0, 0, 0, 0
	deferredKinds := map[string]int{}
	for _, sample := range samples {
		if sample.session != session {
			session = sample.session
			w = &watch{}
		}
		observeScheduling(w, sample.screen, sample.profile, sample.task, sample.at)
		if w.generating {
			recognized++
		}
		reason := inferenceSchedule(w, true, sample.screen, sample.at)
		switch reason {
		case "generation":
			deferred++
			deferredKinds[sample.kind]++
		case "urgent":
			urgent++
		case "fallback":
			fallback++
		}
		if reason != "generation" && reason != "" {
			w.lastAttempt = sample.at
			w.urgent = false
		}
	}
	t.Logf("sampled_requests=%d recognized_generation=%d deferral_opportunities=%d urgent_checks=%d fallback_checks=%d deferred_response_kinds=%v; assumes no in-flight calls, not measured savings or detection latency", len(samples), recognized, deferred, urgent, fallback, deferredKinds)
}

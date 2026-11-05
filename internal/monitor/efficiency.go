package monitor

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"kamacu/internal/observation"
)

// Only the complete, recognized Codex progress footer is normalized. Numbers,
// clocks and spinner-like characters in arbitrary agent output are evidence.
var generationFooter = regexp.MustCompile(`(?m)^([ \t]*• Working \()[0-9]+(?:m [0-9]+s|s)( • esc to interrupt\)[ \t]*)$`)

// Claude's present-tense status has an ellipsis; its completed "Worked for"
// footer deliberately does not match. Keep the verb and effort text meaningful.
var claudeGeneration = regexp.MustCompile(`^([✻✽✶✳✢·]) ([\p{L}]+)… \(([0-9]+m )?[0-9]+s(?: · [↓↑↕] [0-9]+(?:\.[0-9]+)?[km]? tokens)?( · thinking with [a-z]+ effort)?\)$`)

// Only the last matching status for this engine is UI. Earlier status-shaped
// lines may be quoted examples or command output and must remain meaningful.
func generationLine(screen, engine string) int {
	lines := strings.Split(screen, "\n")
	last := -1
	fenced := false
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") {
			fenced = !fenced
			continue
		}
		if fenced || i < max(0, len(lines)-8) {
			continue
		}
		if (engine == "codex" && generationFooter.MatchString(line)) || (engine == "claude" && claudeGeneration.MatchString(line)) {
			last = i
		}
	}
	return last
}

func normalizedScreen(screen, engine string) string {
	index := generationLine(screen, engine)
	if index < 0 {
		return screen
	}
	lines := strings.Split(screen, "\n")
	line := lines[index]
	if engine == "codex" {
		lines[index] = generationFooter.ReplaceAllString(line, "${1}<elapsed>${2}")
	} else if m := claudeGeneration.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
		lines[index] = "<spinner> " + m[2] + "… (<progress>" + m[4] + ")"
	}
	return strings.Join(lines, "\n")
}

func activeGeneration(screen, engine string) bool { return generationLine(screen, engine) >= 0 }

// These signals only accelerate inference, never determine state or authorize
// input. False positives cost a call; false negatives retain a periodic fallback.
var attentionSignal = regexp.MustCompile(`(?i)\?|enter to select|esc to cancel|\b(permission|approval|allow|approve|failed|error|completed|finished)\b|\bdone\b`)

type attentionEvidence struct{ current, history string }

func (a attentionEvidence) present() bool { return a.current != "" || a.history != "" }

// Removal and history expiry never preempt inference. A signal newly visible
// on screen is urgent even if it was previously present only in history.
func newAttention(old, current attentionEvidence) bool {
	hasNew := func(before, after string) bool {
		seen := map[string]bool{}
		for _, line := range strings.Split(before, "\n") {
			seen[line] = true
		}
		for _, line := range strings.Split(after, "\n") {
			if line != "" && !seen[line] {
				return true
			}
		}
		return false
	}
	return hasNew(old.current, current.current) || hasNew(old.current+"\n"+old.history, current.history)
}

func attentionFingerprint(s observation.Snapshot) attentionEvidence {
	var current []string
	visible := map[string]bool{}
	for _, line := range strings.Split(s.Screen, "\n") {
		if attentionSignal.MatchString(line) {
			current = append(current, line)
			visible[line] = true
		}
	}
	// Repeated repaints must not create new urgency by duplicating historical
	// question lines. Keep current and historical signals distinct so a question
	// that scrolls into view is still a new scheduling event.
	historical := map[string]bool{}
	for _, screen := range s.History {
		for _, line := range strings.Split(screen, "\n") {
			if attentionSignal.MatchString(line) && !visible[line] {
				historical[line] = true
			}
		}
	}
	var history []string
	for line := range historical {
		history = append(history, line)
	}
	sort.Strings(history)
	return attentionEvidence{strings.Join(current, "\n"), strings.Join(history, "\n")}
}

const generationFallback = 30 * time.Second

// observeScheduling runs even while inference is busy, retaining urgent changes
// until a worker actually starts. Meaningful quiet time ignores known UI clocks.
func observeScheduling(w *watch, s observation.Snapshot, profile string, task TaskContext, now time.Time) {
	if w.observedRevision != 0 && w.observedRevision == s.Revision && w.observedInput == s.Input && w.observedProfile == profile && w.observedTask == task {
		return
	}
	content := meaningfulFingerprint(s, task.Engine)
	active := activeGeneration(s.Screen, task.Engine)
	signals := attentionFingerprint(s)
	if w.observedRevision == 0 {
		w.observationStarted = now
		w.lastChange = now
		w.urgent = signals.present()
	} else {
		if content != w.observedContent {
			w.lastChange = now
		}
		if s.Input != w.observedInput || profile != w.observedProfile || task != w.observedTask {
			w.urgent = true
			w.retryAfter = time.Time{}
		}
		if (w.generating && !active) || newAttention(w.observedSignals, signals) {
			w.urgent = true
		}
	}
	w.observedRevision = s.Revision
	w.observedInput, w.observedProfile, w.observedTask = s.Input, profile, task
	w.observedContent, w.observedSignals, w.generating = content, signals, active
}

func inferenceSchedule(w *watch, stale bool, s observation.Snapshot, now time.Time) string {
	if w.busy || s.UnsupportedSize || s.Screen == "" || now.Before(w.retryAfter) {
		return ""
	}
	if w.urgent {
		return "urgent"
	}
	elapsed := now.Sub(w.lastAttempt)
	// Existing attention evidence keeps the ordinary retry cadence even if a
	// generation footer remains visible beside the dialog.
	attentionKind := w.decision.Kind == "question" || w.decision.Kind == "permission" || w.decision.Kind == "error"
	// Keep an unread completion responsive to a cosmetic repaint, but do not
	// let a retained old completion force calls throughout new generation.
	if w.decision.Kind == "completion" && meaningfulFingerprint(w.evidence, w.observedTask.Engine) == meaningfulFingerprint(s, w.observedTask.Engine) {
		attentionKind = true
	}
	attention := attentionKind && w.input == s.Input && w.profile == w.observedProfile && w.taskContext == w.observedTask && strings.TrimSpace(w.decision.Evidence) != "" && strings.Contains(s.Screen, w.decision.Evidence)
	if w.generating && !(stale && attention) {
		baseline := w.lastAttempt
		if baseline.IsZero() {
			baseline = w.observationStarted
		}
		if now.Sub(baseline) >= generationFallback {
			return "fallback"
		}
		return "generation"
	}
	if !stale {
		if w.decision.State == "unknown" && elapsed >= generationFallback {
			return "fallback"
		}
		return ""
	}
	if elapsed > 5*time.Second && (now.Sub(w.lastChange) > 2*time.Second || elapsed > 10*time.Second) {
		return "settled"
	}
	return ""
}

func meaningfulFingerprint(s observation.Snapshot, engine string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(normalizedScreen(s.Screen, engine))))
}

// promptSnapshot removes only contiguous suffix/prefix overlaps between
// consecutive screens. Arbitrary repeated lines may be headings, choices or
// code delimiters whose context must remain intact. Originals remain available
// to validate excerpts and resolve their observation times.
func promptSnapshot(s observation.Snapshot) observation.Snapshot {
	out := s
	out.History = nil
	out.HistoryAt = nil
	out.HistoryLineTimes = nil
	for i, screen := range s.History {
		next := s.Screen
		if i+1 < len(s.History) {
			next = s.History[i+1]
		}
		lines := strings.Split(screen, "\n")
		newer := strings.Split(next, "\n")
		overlap := 0
		for n := min(len(lines), len(newer)); n > 0; n-- {
			// A single blank or generic label is not sufficient evidence of overlap.
			if n < 2 && screen != next {
				continue
			}
			if strings.Join(lines[len(lines)-n:], "\n") == strings.Join(newer[:n], "\n") {
				overlap = n
				break
			}
		}
		remaining := strings.Join(lines[:len(lines)-overlap], "\n")
		if remaining == "" {
			continue
		}
		out.History = append(out.History, remaining)
		at := s.At
		if i < len(s.HistoryAt) {
			at = s.HistoryAt[i]
		}
		out.HistoryAt = append(out.HistoryAt, at)
	}
	return out
}

// Cosmetic reuse is limited to successfully persisted routine decisions. Only
// recognized progress evidence may be rebased onto an actual current line.
func cosmeticCandidate(w *watch, s observation.Snapshot, profile string, task TaskContext) bool {
	if s.UnsupportedSize || w.revision == 0 || w.revision == s.Revision || !w.reusable || w.input != s.Input || w.profile != profile || w.taskContext != task {
		return false
	}
	_, reusable := rebaseRoutineDecision(w.decision, w.evidence, s, task.Engine)
	return reusable
}

func rebaseRoutineDecision(d Decision, old, current observation.Snapshot, engine string) (Decision, bool) {
	if old.UnsupportedSize || current.UnsupportedSize || old.Input != current.Input {
		return d, false
	}
	if old.Screen != current.Screen && (!activeGeneration(old.Screen, engine) || !activeGeneration(current.Screen, engine)) {
		return d, false
	}
	if d.Kind != "none" || (d.State != "working" && d.State != "idle") || strings.TrimSpace(d.Evidence) == "" || meaningfulFingerprint(old, engine) != meaningfulFingerprint(current, engine) || !historyCovered(old, current, engine) {
		return d, false
	}
	if strings.Contains(current.Screen, d.Evidence) {
		return d, true
	}
	// Rebase only the actual selected UI line in both snapshots. An earlier
	// quoted status must never become the replacement detection evidence.
	oldIndex, currentIndex := generationLine(old.Screen, engine), generationLine(current.Screen, engine)
	if oldIndex >= 0 && currentIndex >= 0 && d.Evidence == strings.Split(old.Screen, "\n")[oldIndex] {
		line := strings.Split(current.Screen, "\n")[currentIndex]
		if normalizedScreen(line, engine) == normalizedScreen(d.Evidence, engine) {
			d.Evidence = line
			return d, true
		}
	}
	return d, false
}

// A transient question/error may scroll away between observations even when
// the visible screen returns to the same content. New history is substantive.
func historyCovered(old, current observation.Snapshot, engine string) bool {
	sources := append([]string{old.Screen}, old.History...)
	for _, h := range current.History {
		normalized := normalizedScreen(h, engine)
		covered := false
		for _, source := range sources {
			if strings.Contains(normalizedScreen(source, engine), normalized) {
				covered = true
				break
			}
		}
		if !covered {
			return false
		}
	}
	return true
}

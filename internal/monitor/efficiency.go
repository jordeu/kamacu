package monitor

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"

	"kamacu/internal/observation"
)

// Only the complete, recognized Codex progress footer is normalized. Numbers,
// clocks and spinner-like characters in arbitrary agent output are evidence.
var generationFooter = regexp.MustCompile(`(?m)^([ \t]*• Working \()[0-9]+(?:m [0-9]+s|s)( • esc to interrupt\)[ \t]*)$`)

func meaningfulFingerprint(s observation.Snapshot) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(generationFooter.ReplaceAllString(s.Screen, "${1}<elapsed>${2}"))))
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

// Cosmetic candidates never reuse attention or uncertain decisions. Requiring
// an exact evidence match prevents a changing footer from becoming evidence.
func cosmeticCandidate(w *watch, s observation.Snapshot, profile string, task TaskContext) bool {
	return !s.UnsupportedSize && w.revision != 0 && w.revision != s.Revision && w.input == s.Input && w.profile == profile && w.taskContext == task && w.decision.Kind == "none" && (w.decision.State == "working" || w.decision.State == "idle") && strings.Contains(s.Screen, w.decision.Evidence) && w.decision.Evidence != "" && meaningfulFingerprint(w.evidence) == meaningfulFingerprint(s) && historyCovered(w.evidence, s)
}

// A transient question/error may scroll away between observations even when
// the visible screen returns to the same content. New history is substantive.
func historyCovered(old, current observation.Snapshot) bool {
	sources := append([]string{old.Screen}, old.History...)
	for _, h := range current.History {
		normalized := generationFooter.ReplaceAllString(h, "${1}<elapsed>${2}")
		covered := false
		for _, source := range sources {
			if strings.Contains(generationFooter.ReplaceAllString(source, "${1}<elapsed>${2}"), normalized) {
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

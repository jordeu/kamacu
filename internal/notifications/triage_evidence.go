package notifications

import "time"

type evidenceRecency uint8

const (
	evidenceUnknown evidenceRecency = iota
	evidenceHistorical
	evidenceCurrent
)

// GitHub reasons and latest-comment pointers persist across thread updates.
// They cannot establish when a particular question or mention happened.
func evidenceSince(events []RawNotification) (time.Time, bool) {
	var since time.Time
	for _, raw := range events {
		at, err := time.Parse(time.RFC3339, raw.Event.Version)
		if err != nil {
			return time.Time{}, false
		}
		if since.IsZero() || at.Before(since) {
			since = at
		}
	}
	return since, !since.IsZero()
}

func scopeEvidence(events []RawNotification, subject Event, item SubjectEvidence) evidenceRecency {
	since, ok := evidenceSince(events)
	if !ok {
		return evidenceUnknown
	}
	at, err := time.Parse(time.RFC3339, item.UpdatedAt)
	if err != nil {
		at, err = time.Parse(time.RFC3339, item.CreatedAt)
	}
	if err != nil {
		return evidenceUnknown
	}
	if at.Before(since) {
		// Thread notification timestamps can lag their comments. A nearby older
		// timestamp is uncertain, not proof that the text is historical. Keep it
		// for inspection instead of silently dropping it on a clock boundary.
		if since.Sub(at) <= time.Minute {
			return evidenceUnknown
		}
		return evidenceHistorical
	}
	if item.URL == subject.URL {
		// A subject's updated_at also changes when comments or PR state change.
		// Only creation establishes current body text without a body-edit event.
		// A latest-comment pointer can still reference an old subject body.
		created, err := time.Parse(time.RFC3339, item.CreatedAt)
		if err != nil || created.Before(since) {
			return evidenceUnknown
		}
	}
	return evidenceCurrent
}

func classifierEvidence(events []RawNotification, subject Event) ([]SubjectEvidence, bool) {
	evidence := []SubjectEvidence{}
	complete := subject.EvidenceComplete && len(subject.ContextWarnings) == 0
	for _, item := range subject.Evidence {
		recency := scopeEvidence(events, subject, item)
		if recency == evidenceHistorical {
			continue
		}
		// Unknown dates cannot justify dropping an otherwise relevant question.
		// Include issue bodies too; their URL is the subject URL, not a comment URL.
		if recency == evidenceUnknown {
			complete = false
		}
		item.Current = recency == evidenceCurrent
		evidence = append(evidence, item)
	}
	if len(subject.Evidence) == 0 && subject.Context != "" {
		evidence = append(evidence, SubjectEvidence{ID: "context", Text: subject.Context, URL: subject.URL})
	}
	return evidence, complete
}

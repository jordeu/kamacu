package notifications

import (
	"strings"
	"time"
)

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

func textMentionsViewer(text, viewer string) bool {
	if viewer == "" {
		return false
	}
	text = strings.ToLower(text)
	needle := "@" + strings.ToLower(viewer)
	for at := strings.Index(text, needle); at >= 0; {
		end := at + len(needle)
		if end == len(text) || !strings.ContainsRune("abcdefghijklmnopqrstuvwxyz0123456789_-", rune(text[end])) {
			return true
		}
		next := strings.Index(text[end:], needle)
		if next < 0 {
			break
		}
		at = end + next
	}
	return false
}

// mentionStatus is tri-state. Negative evidence requires complete discussion
// retrieval, or a full-text scan performed before clipping model excerpts.
func mentionStatus(events []RawNotification, e Event, viewer string) *bool {
	yes, no := true, false
	if viewer == "" {
		return nil
	}
	uncertainMention := false
	scanned := e.MentionScanComplete
	for _, item := range e.Evidence {
		recency := scopeEvidence(events, e, item)
		if recency == evidenceHistorical {
			continue
		}
		mentioned := textMentionsViewer(item.Text, viewer)
		if item.ViewerMention != nil && strings.EqualFold(item.MentionViewer, viewer) {
			mentioned = *item.ViewerMention
		} else {
			scanned = false
		}
		if mentioned {
			if recency == evidenceCurrent {
				return &yes
			}
			uncertainMention = true
		}
	}
	if !uncertainMention && (e.EvidenceComplete || scanned) && len(e.ContextWarnings) == 0 {
		return &no
	}
	return nil
}

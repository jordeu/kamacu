package notifications

import (
	"crypto/sha256"
	"encoding/base64"
	"strings"
)

type triageEvidence struct {
	Scope  string `json:"scope"`
	Intent string `json:"intent"`
	Quote  string `json:"quote"`
}

func triageScope(sourceID, url string) string { return sourceID + "\x00" + strings.ToLower(url) }

func subjectScope(subject *triageSubject) string {
	sourceID := ""
	if len(subject.events) > 0 {
		sourceID = subject.events[0].SourceID
	}
	return triageScope(sourceID, subject.fresh.URL)
}

func evidenceFingerprint(text string) string {
	hash := sha256.Sum256([]byte(text))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}

func findingEvidence(subject *triageSubject, finding intentFinding) triageEvidence {
	return triageEvidence{Scope: evidenceFingerprint(subjectScope(subject)), Intent: finding.Intent, Quote: evidenceFingerprint(finding.Quote)}
}

func planHasIntent(plan ActionPlan, intent string) bool {
	for _, record := range plan.Evidence {
		if record.Intent == intent {
			return true
		}
	}
	return false
}

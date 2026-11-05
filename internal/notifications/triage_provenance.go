package notifications

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
)

const triageEvidencePrefix = "<!-- kamacu-triage-evidence:"

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

// Read only the application-owned prefix, never markers in generated prose or
// quoted repository text. A malformed prefix cannot fall back to broad matches.
func readTriageEvidence(context string) ([]triageEvidence, string, bool) {
	if !strings.HasPrefix(context, triageEvidencePrefix) {
		return nil, context, false
	}
	end := strings.Index(context, " -->")
	if end < 0 {
		return nil, context, true
	}
	var records []triageEvidence
	if err := json.Unmarshal([]byte(context[len(triageEvidencePrefix):end]), &records); err != nil {
		return nil, context, true
	}
	return records, strings.TrimPrefix(context[end+len(" -->"):], "\n\n"), true
}

func withTriageEvidence(records []triageEvidence, context string) string {
	if len(records) == 0 {
		return context
	}
	_, body, _ := readTriageEvidence(context)
	raw, _ := json.Marshal(records)
	return triageEvidencePrefix + string(raw) + " -->\n\n" + body
}

func hasFindingEvidence(context string, subject *triageSubject, finding intentFinding) (bool, bool) {
	records, _, present := readTriageEvidence(context)
	if !present {
		return false, false
	}
	want := findingEvidence(subject, finding)
	for _, record := range records {
		if record == want {
			return true, true
		}
	}
	return false, true
}

package notifications

import (
	"bytes"
	"encoding/json"
)

func reviewRequestStatusReason(subject Event) string {
	if facts := subject.PRFacts; facts != nil && facts.Available && !facts.Stale && (facts.State == "merged" || facts.State == "closed") {
		return "obsolete_review_request"
	}
	return ""
}
func refreshIntentRules(subject *triageSubject, previous Event, rules RuleSet) {
	subject.reason = preflightRules(rules, subject)
	before, beforeComplete := classifierEvidence(subject.events, previous)
	after, afterComplete := classifierEvidence(subject.events, subject.fresh)
	a, _ := json.Marshal(before)
	b, _ := json.Marshal(after)
	subject.classificationStale = subject.classificationStale || !bytes.Equal(a, b) || beforeComplete != afterComplete || previous.SubjectTitle != subject.fresh.SubjectTitle || !equalBool(mentionStatus(subject.events, previous, subject.viewer), mentionStatus(subject.events, subject.fresh, subject.viewer)) || classificationFactsChanged(previous.PRFacts, subject.fresh.PRFacts) || len(subject.classifiedFindings) == 0
	if subject.reason != "" {
		subject.findings = nil
		return
	}
	if subject.classificationStale {
		subject.findings = []intentFinding{{Intent: "needs_context"}}
		return
	}
	subject.findings = append([]intentFinding(nil), subject.classifiedFindings...)
}

func classificationFactsChanged(before, after *PRFacts) bool {
	if before == nil || after == nil {
		return before != after
	}
	fields := func(f *PRFacts) map[string]any {
		values := compactTriageFacts(f).(map[string]any)
		delete(values, "observed_at")
		values["head_commit"], values["updated_at"] = f.HeadCommit, f.UpdatedAt
		return values
	}
	a, _ := json.Marshal(fields(before))
	b, _ := json.Marshal(fields(after))
	return !bytes.Equal(a, b)
}

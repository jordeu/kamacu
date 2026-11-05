package notifications

import (
	"bytes"
	"encoding/json"
	"strings"

	"kamacu/internal/settings"
)

// This function is called only for an already classified review_request.
// A thread's review_requested reason cannot establish the latest activity's
// intent: comments and feedback on the same thread may remain actionable.
func reviewPolicyReason(policy settings.NotificationPolicy, subject Event, mention *bool) string {
	if mention != nil && *mention {
		return ""
	}
	facts := subject.PRFacts
	if facts == nil || !facts.Available || facts.Stale {
		return ""
	}
	reason := ""
	switch {
	case facts.State == "merged" || facts.State == "closed":
		reason = "obsolete_review_request"
	case policy.IgnoreDrafts && facts.Draft != nil && *facts.Draft:
		reason = "draft_review_request"
	case policy.IgnoreApproved && facts.ViewerEverApproved != nil && *facts.ViewerEverApproved:
		reason = "already_approved"
	}
	if reason != "" && mention == nil {
		return "needs_context:direct_mention"
	}
	return reason
}

func refreshIntentPolicy(subject *triageSubject, previous Event, policy settings.NotificationPolicy) {
	subject.reason = policyReason(policy, subject.events, subject.fresh, subject.viewer)
	if subject.reason != "" && !strings.HasPrefix(subject.reason, "needs_context:") {
		return
	}
	before, beforeComplete := classifierEvidence(subject.events, previous)
	after, afterComplete := classifierEvidence(subject.events, subject.fresh)
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	changed := !bytes.Equal(beforeJSON, afterJSON) || beforeComplete != afterComplete || previous.SubjectTitle != subject.fresh.SubjectTitle
	if classificationFactsChanged(previous.PRFacts, subject.fresh.PRFacts) {
		changed = true
	}
	subject.classificationStale = subject.classificationStale || changed || len(subject.classifiedFindings) == 0
	if subject.classificationStale || strings.HasPrefix(subject.reason, "needs_context:") {
		subject.findings = []intentFinding{{Intent: "needs_context"}}
		return
	}
	// Reapply changed status/approval facts to the original classification, not to
	// a previously filtered copy. This also handles reopening a merged PR.
	subject.findings = append([]intentFinding(nil), subject.classifiedFindings...)
	applyIntentPolicy(subject, policy)
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

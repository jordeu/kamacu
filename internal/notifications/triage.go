package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"
	"time"

	"kamacu/internal/inbox"
	"kamacu/internal/settings"
)

type intentFinding struct {
	Intent      string `json:"intent"`
	EvidenceID  string `json:"evidence_id"`
	Quote       string `json:"quote"`
	PolicyQuote string `json:"policy_quote"`
}
type intentResponse struct {
	Findings []intentFinding `json:"findings"`
}

var intentSchema = map[string]any{
	"type": "object", "additionalProperties": false, "required": []string{"findings"},
	"properties": map[string]any{"findings": map[string]any{"type": "array", "minItems": 1, "maxItems": 6, "items": map[string]any{
		"type": "object", "additionalProperties": false, "required": []string{"intent", "evidence_id", "quote", "policy_quote"},
		"properties": map[string]any{"intent": map[string]any{"type": "string", "enum": []string{"review_request", "review_feedback", "direct_question", "routine", "policy_ignore", "needs_context"}}, "evidence_id": map[string]string{"type": "string"}, "quote": map[string]string{"type": "string"}, "policy_quote": map[string]string{"type": "string"}},
	}}}}

type triageSubject struct {
	viewer              string
	events              []RawNotification
	fresh               Event
	findings            []intentFinding
	reason              string
	classifiedFindings  []intentFinding
	classificationStale bool
}

// Match the full username, so @alice-other does not count as @alice.
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

// policyReason never suppresses direct mentions. Own activity requires an exact
// current-comment timestamp match; the PR author alone says nothing about actor.
func policyReason(p settings.NotificationPolicy, events []RawNotification, e Event, viewer string) string {
	mention := mentionStatus(events, e, viewer)
	if mention != nil && *mention {
		return ""
	}
	excluded := false
	for _, repo := range p.ExcludedRepositories {
		excluded = excluded || strings.EqualFold(repo, e.Repository)
	}
	if excluded {
		if mention == nil {
			return "needs_context:direct_mention"
		}
		return "repository_policy"
	}
	if p.IgnoreOwnActivity && mention != nil && !*mention && viewer != "" {
		own := len(events) > 0
		for _, raw := range events {
			found := false
			for _, item := range e.Evidence {
				found = found || (item.Current && item.Complete && item.UpdatedAt == raw.Event.Version && strings.EqualFold(item.Actor, viewer))
			}
			own = own && found
		}
		if own {
			return "own_activity"
		}
	}
	return ""
}

func decodeIntent(raw []byte, evidence []SubjectEvidence, custom string) ([]intentFinding, error) {
	var response intentResponse
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&response); err != nil {
		return nil, err
	}
	if d.Decode(new(any)) != io.EOF || len(response.Findings) == 0 || len(response.Findings) > 6 {
		return nil, errors.New("invalid intent classification")
	}
	byID := map[string]string{}
	for _, item := range evidence {
		byID[item.ID] = item.Text
	}
	for _, f := range response.Findings {
		switch f.Intent {
		case "review_request", "review_feedback", "direct_question", "routine", "policy_ignore", "needs_context":
		default:
			return nil, errors.New("unknown notification intent")
		}
		if strings.TrimSpace(f.Quote) == "" || len(f.Quote) > 2000 || !strings.Contains(byID[f.EvidenceID], f.Quote) {
			return nil, errors.New("notification intent requires source-backed evidence")
		}
		if f.Intent == "policy_ignore" {
			if strings.TrimSpace(f.PolicyQuote) == "" || !strings.Contains(custom, f.PolicyQuote) {
				return nil, errors.New("policy ignore requires an exact user preference")
			}
		} else if f.PolicyQuote != "" {
			return nil, errors.New("unexpected policy evidence")
		}
	}
	// Keep repeated intents until project classification applies recency and
	// policy checks to each citation, then deduplicates the resulting intents.
	return response.Findings, nil
}

// GitHub can report a dependency review as "subscribed". Apply review-specific
// rules again after intent classification without suppressing other findings on
// the same subject (for example feedback after a merge).
func applyIntentPolicy(subject *triageSubject, policy settings.NotificationPolicy) {
	mention := mentionStatus(subject.events, subject.fresh, subject.viewer)
	if mention != nil && *mention {
		return
	}
	reason := reviewPolicyReason(policy, subject.fresh, mention)
	if reason == "" {
		return
	}
	filtered := []intentFinding{}
	for _, finding := range subject.findings {
		if finding.Intent != "review_request" {
			filtered = append(filtered, finding)
			continue
		}
		if strings.HasPrefix(reason, "needs_context:") {
			finding.Intent = "needs_context"
			filtered = append(filtered, finding)
		}
	}
	if len(filtered) == 0 {
		subject.reason = reason
	}
	subject.findings = filtered
}

func compactTriageFacts(f *PRFacts) any {
	if f == nil {
		return nil
	}
	return map[string]any{"state": f.State, "draft": f.Draft, "available": f.Available, "stale": f.Stale, "observed_at": f.ObservedAt, "viewer": f.Viewer, "viewer_ever_approved": f.ViewerEverApproved, "reviews_complete": f.ReviewsComplete, "requested_reviewers": f.RequestedReviewers}
}

func (s *Service) triageGroup(ctx context.Context, batchID string, events []RawNotification, plan *notificationPlan) error {
	runCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	rawPolicy, err := settings.Get(s.DB, settings.KeyNotificationPolicy)
	if err != nil {
		return err
	}
	policy, err := settings.ParseNotificationPolicy(rawPolicy)
	if err != nil {
		return err
	}
	custom, err := s.notificationPrompt()
	if err != nil {
		return err
	}
	sources := map[string]Source{}
	subjects := []*triageSubject{}
	indexes := map[string]int{}
	for _, raw := range events {
		if _, ok := sources[raw.SourceID]; !ok {
			source, e := scanSource(s.DB.QueryRowContext(runCtx, `SELECT `+sourceColumns+` FROM notification_sources WHERE id=?`, raw.SourceID))
			if e != nil {
				return e
			}
			if e = s.sourceEnabled(runCtx, source.ID); e != nil {
				return e
			}
			if e = s.checkIdentity(runCtx, source); e != nil {
				return e
			}
			sources[raw.SourceID] = source
		}
		key := subjectKey(raw.SourceID, raw.Event)
		index, ok := indexes[key]
		if !ok {
			index = len(subjects)
			indexes[key] = index
			subjects = append(subjects, &triageSubject{})
		}
		subjects[index].events = append(subjects[index].events, raw)
		// Input order is event ID order; keep the newest notification timestamp.
		if subjects[index].fresh.URL == "" || raw.Event.Version >= subjects[index].fresh.Version {
			subjects[index].fresh = raw.Event
		}
	}
	for _, subject := range subjects {
		source := sources[subject.events[0].SourceID]
		subject.viewer = source.Account
		subject.fresh, err = s.enrichSubject(runCtx, source, subject.fresh)
		if err != nil {
			return err
		}
		subject.reason = policyReason(policy, subject.events, subject.fresh, source.Account)
	}
	if err = s.classifyProject(runCtx, subjects, policy, custom); err != nil {
		return err
	}

	plan.groups = append(plan.groups, preparedTriageGroup{subjects: subjects, sources: sources, events: events, policy: policy, custom: custom})
	return nil
}

type preparedTriageGroup struct {
	subjects []*triageSubject
	sources  map[string]Source
	events   []RawNotification
	policy   settings.NotificationPolicy
	custom   string
}

func (s *Service) checkTriagePreferences(group preparedTriageGroup) error {
	raw, err := settings.Get(s.DB, settings.KeyNotificationPolicy)
	if err != nil {
		return err
	}
	policy, err := settings.ParseNotificationPolicy(raw)
	if err != nil {
		return err
	}
	custom, err := s.notificationPrompt()
	if err != nil {
		return err
	}
	before, _ := json.Marshal(group.policy)
	after, _ := json.Marshal(policy)
	if string(before) != string(after) || custom != group.custom {
		return errors.New("notification preferences changed during processing; process pending again")
	}
	return nil
}

func (s *Service) finalizeTriage(ctx context.Context, id string, plan *notificationPlan) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	for _, group := range plan.groups {
		subjects, sources, events, policy := group.subjects, group.sources, group.events, group.policy
		// A settings change invalidates the whole prepared plan, just like an inbox edit.
		if err := s.checkTriagePreferences(group); err != nil {
			return err
		}
		var err error
		// Re-fetch every GitHub fact that may affect a decision immediately before
		// planning publication. External state is not covered by inbox revision locks.
		for _, subject := range subjects {
			source := sources[subject.events[0].SourceID]
			if reader, ok := s.Providers[source.Provider].(interface {
				SubjectVersion(context.Context, Source, Event) (string, error)
			}); ok && subject.fresh.SubjectUpdatedAt != "" {
				version, e := reader.SubjectVersion(ctx, source, subject.fresh)
				if e != nil && !errors.Is(e, ErrContextUnavailable) {
					return e
				}
				if e != nil || version != subject.fresh.SubjectUpdatedAt {
					previous := subject.fresh
					subject.fresh, err = s.enrichSubject(ctx, source, subject.fresh)
					if err != nil {
						return err
					}
					refreshIntentPolicy(subject, previous, policy)
				}
			}
			if reader, ok := s.Providers[source.Provider].(interface {
				PRContext(context.Context, Source, string, int64) PRFacts
			}); ok && subject.fresh.PRFacts != nil {
				old := subject.fresh.PRFacts
				fresh := reader.PRContext(ctx, source, old.Repository, old.Number)
				before, _ := json.Marshal(compactTriageFacts(old))
				after, _ := json.Marshal(compactTriageFacts(&fresh))
				// Observation time is expected to change. Compare only decision fields.
				if old.Available != fresh.Available || old.Stale != fresh.Stale || old.State != fresh.State || !equalBool(old.Draft, fresh.Draft) || !equalBool(old.ViewerEverApproved, fresh.ViewerEverApproved) || old.ReviewsComplete != fresh.ReviewsComplete || old.HeadCommit != fresh.HeadCommit || old.UpdatedAt != fresh.UpdatedAt || old.Viewer != fresh.Viewer || !slices.Equal(old.RequestedReviewers, fresh.RequestedReviewers) {
					slog.Info("notification facts changed before publication", "url", subject.fresh.URL, "before", string(before), "after", string(after))
					// Re-fetch prose too: an update can introduce a new direct mention.
					previous := subject.fresh
					subject.fresh, err = s.enrichSubject(ctx, source, subject.fresh)
					if err != nil {
						return err
					}
					refreshIntentPolicy(subject, previous, policy)
				}
			}
		}
		if slices.ContainsFunc(subjects, func(subject *triageSubject) bool { return subject.classificationStale }) {
			for _, subject := range subjects {
				subject.reason = policyReason(policy, subject.events, subject.fresh, subject.viewer)
			}
			if err = s.classifyProject(ctx, subjects, policy, group.custom); err != nil {
				return err
			}
		}
		questions, err := s.groupQuestions(ctx, id, events)
		if err != nil {
			return err
		}
		decision, err := s.renderTriage(ctx, subjects, questions, policy)
		if err != nil {
			return err
		}
		if err = s.draftProjectCards(ctx, &decision, subjects, policy, group.custom); err != nil {
			return err
		}
		allowed := map[int64]RawNotification{}
		for _, raw := range events {
			allowed[raw.ID] = raw
		}
		for i := range decision.Actions {
			if err = fitTriageContext(&decision.Actions[i], allowed); err != nil {
				return err
			}
		}
		for _, source := range sources {
			if err = s.sourceEnabled(ctx, source.ID); err != nil {
				return err
			}
			if err = s.checkIdentity(ctx, source); err != nil {
				return err
			}
		}
		actions, refs, err := validateBatch(decision, events, questions)
		if err != nil {
			return err
		}
		for i := range actions {
			noneSeen := false
			options := actions[i].Request.Options[:0]
			for j := range actions[i].Request.Options {
				option := &actions[i].Request.Options[j]
				if option.Label == "Leave this activity alone" || option.Label == "I'll handle these reviews myself" {
					if noneSeen {
						continue
					}
					option.ID = "triage_noop"
					noneSeen = true
				}
				options = append(options, *option)
			}
			actions[i].Request.Options = options
		}
		plan.actions = append(plan.actions, actions...)
		plan.refs = append(plan.refs, refs...)

	}
	// Model calls may take time. Also validate preferences after the last model
	// response, before publishing any prepared group's cards.
	for _, group := range plan.groups {
		if err := s.checkTriagePreferences(group); err != nil {
			return err
		}
	}
	return nil
}

func equalBool(a, b *bool) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }

var triageQuestions = map[string]string{
	"review_request":  "How should I handle these review requests?",
	"review_feedback": "How should I handle this review feedback?",
	"direct_question": "How would you like to respond to this question?",
	"needs_context":   "Would you like to inspect this activity before deciding?",
}

func (s *Service) renderTriage(ctx context.Context, subjects []*triageSubject, questions []inbox.Question, policy settings.NotificationPolicy) (BatchDecision, error) {
	decision := BatchDecision{}
	scopes := map[string]map[string]bool{}
	for _, q := range questions {
		related, err := s.Inbox.RelatedNotificationIDs(ctx, q.ID)
		if err != nil {
			return decision, err
		}
		urls := map[string]bool{}
		for _, id := range related {
			var payload, sourceID string
			if err = s.DB.QueryRowContext(ctx, `SELECT payload,source_id FROM notification_events WHERE id=?`, id).Scan(&payload, &sourceID); err != nil {
				return decision, err
			}
			var e Event
			if err = json.Unmarshal([]byte(payload), &e); err != nil {
				return decision, err
			}
			urls[triageScope(sourceID, e.URL)] = true
		}
		scopes[q.ID] = urls
	}
	// Only retire our review template when every linked subject is conclusively
	// obsolete in this snapshot. A merged PR may still have an unrelated question.
	for _, q := range questions {
		if !isTriageQuestion(q, "review_request") {
			continue
		}
		obsolete := len(scopes[q.ID]) > 0
		for url := range scopes[q.ID] {
			found := false
			for _, subject := range subjects {
				if subjectScope(subject) == url && (subject.reason == "obsolete_review_request" || subject.reason == "draft_review_request" || subject.reason == "already_approved") {
					found = true
				}
			}
			obsolete = obsolete && found
		}
		if obsolete {
			decision.SupersedeIDs = append(decision.SupersedeIDs, q.ID)
		}
	}
	// A resolved classification replaces an earlier inspection card. Keep grouped
	// questions if any linked subject is missing or still uncertain.
	for _, q := range questions {
		if !isTriageQuestion(q, "needs_context") {
			continue
		}
		resolved := len(scopes[q.ID]) > 0
		for url := range scopes[q.ID] {
			found := false
			for _, subject := range subjects {
				if subjectScope(subject) != url {
					continue
				}
				found = subject.reason != "" && !strings.HasPrefix(subject.reason, "needs_context:")
				if subject.reason == "" && len(subject.findings) > 0 {
					found = !slices.ContainsFunc(subject.findings, func(f intentFinding) bool { return f.Intent == "needs_context" })
				}
			}
			resolved = resolved && found
		}
		if resolved {
			decision.SupersedeIDs = append(decision.SupersedeIDs, q.ID)
		}
	}
	allowed := map[int64]RawNotification{}
	for _, subject := range subjects {
		for _, raw := range subject.events {
			allowed[raw.ID] = raw
		}
	}
	grouped := map[string]*BatchAction{}
	var order []*BatchAction
	for _, subject := range subjects {
		actionable := []intentFinding{}
		if subject.reason == "" || strings.HasPrefix(subject.reason, "needs_context:") {
			for _, f := range subject.findings {
				if f.Intent != "routine" && f.Intent != "policy_ignore" {
					actionable = append(actionable, f)
				}
			}
		}
		ids := []int64{}
		for _, raw := range subject.events {
			ids = append(ids, raw.ID)
		}
		if len(actionable) == 0 {
			decision.DiscardIDs = append(decision.DiscardIDs, ids...)
			slog.Info("notification decision", "url", subject.fresh.URL, "event_ids", ids, "reason", subject.reason, "findings", subject.findings)
			continue
		}
		for _, f := range actionable {
			// Only known template questions are deduplicated; arbitrary old questions
			// might represent a different action on the same PR and remain untouched.
			duplicate := false
			for _, q := range questions {
				if !isTriageQuestion(q, f.Intent) {
					continue
				}
				urls := scopes[q.ID]
				if !urls[subjectScope(subject)] {
					continue
				}
				match, scoped := hasFindingEvidence(q.Context, subject, f)
				if !scoped && len(urls) == 1 {
					match = f.Quote == "" || strings.Contains(q.Context, f.Quote)
				}
				if match {
					duplicate = true
					continue
				}
				// A single-subject card can be replaced with the new evidence. Preserve
				// grouped cards when only part of their scope is present in this snapshot.
				if len(urls) == 1 && !slices.Contains(decision.SupersedeIDs, q.ID) {
					decision.SupersedeIDs = append(decision.SupersedeIDs, q.ID)
				}
			}
			if duplicate {
				continue
			}

			key := subject.fresh.URL
			action := grouped[key]
			if action != nil {
				candidate := append(append([]int64(nil), action.EventIDs...), ids...)
				if len(batchActionDetails(candidate, allowed)) > triageProvenanceLimit {
					action = nil
				}
			}
			if action == nil {
				action = &BatchAction{Summary: triageTitle(f.Intent) + ": " + subject.fresh.Title, Question: triageQuestions[f.Intent], Options: triageOptions(f.Intent)}
				grouped[key] = action
				order = append(order, action)
			}
			for _, id := range ids {
				if !slices.Contains(action.EventIDs, id) {
					action.EventIDs = append(action.EventIDs, id)
				}
			}
			action.evidence = append(action.evidence, findingEvidence(subject, f))
			title := subject.fresh.SubjectTitle
			if title == "" {
				title = subject.fresh.Title
			}
			if len(title) > 600 {
				title = strings.ToValidUTF8(title[:600], "") + "…"
			}
			action.Context += fmt.Sprintf("\n\n<!-- kamacu-triage:%s -->\n**%s**\n%s", f.Intent, title, subject.fresh.URL)
			if f.Quote != "" {
				action.Context += "\n\nSource excerpt:\n> " + strings.ReplaceAll(f.Quote, "\n", "\n> ")
			}
			if f.Intent == "needs_context" {
				for _, warning := range subject.fresh.ContextWarnings {
					action.Context += "\n\nContext limitation: " + warning
				}
				if subject.classificationStale {
					action.Context += "\n\nThe discussion changed after classification; verify the latest comments before choosing an action."
				} else if subject.reason == "needs_context:direct_mention" {
					action.Context += "\n\nThe preference would exclude this activity, but the available evidence cannot establish whether you were directly mentioned."
				}
			}
			if f.Intent == "review_request" {
				coverage, err := s.reviewCoverage(ctx, subject)
				if err != nil {
					return decision, err
				}
				action.Context += "\n\n" + coverage

			}
		}
	}
	covered := map[int64]bool{}
	for _, a := range order {
		a.Context = withTriageEvidence(a.evidence, a.Context)
		if err := fitTriageContext(a, allowed); err != nil {
			return decision, err
		}
		for _, id := range a.EventIDs {
			covered[id] = true
		}
		decision.Actions = append(decision.Actions, *a)
	}
	for _, subject := range subjects {
		for _, raw := range subject.events {
			if !covered[raw.ID] && !slices.Contains(decision.DiscardIDs, raw.ID) {
				decision.DiscardIDs = append(decision.DiscardIDs, raw.ID)
			}
		}
	}
	return decision, nil
}

func triageTitle(kind string) string {
	switch kind {
	case "review_request":
		return "Review requested"
	case "review_feedback":
		return "New review feedback"
	case "direct_question":
		return "Question to consider"
	default:
		return "Activity needs inspection"
	}
}
func triageOptions(kind string) []string {
	switch kind {
	case "review_request":
		return []string{"Review these PRs, checking existing work before starting another session", "I'll handle these reviews myself"}
	case "review_feedback":
		return []string{"Resume the responsible implementation task agent to check new comments and commits; create a task if none exists"}
	case "direct_question":
		return []string{"Ask the responsible task agent to investigate and answer; create a task if none exists"}
	default:
		return []string{"Ask the responsible task agent to inspect this activity; create a task if none exists", "Leave this activity alone"}
	}
}

func (s *Service) reviewCoverage(ctx context.Context, subject *triageSubject) (string, error) {
	for _, work := range subject.fresh.RelatedSessions {
		if work.State == "working" && !work.StateStale && !work.Stale && work.Uncertainty == "" && work.Evidence != "" {
			return "A related session has recent work evidence. Check whether it covers this review before starting another session.", nil
		}
	}
	if s.Tools == nil {
		return "Active review coverage is unknown because session discovery is unavailable.", nil
	}
	var projectID int64
	if err := s.DB.QueryRowContext(ctx, `SELECT id FROM projects WHERE lower(github_repo)=lower(?) ORDER BY id LIMIT 1`, subject.fresh.Repository).Scan(&projectID); err != nil {
		return "Active review coverage is unknown: no registered project could be resolved.", nil
	}
	args, _ := json.Marshal(map[string]any{"project_id": projectID})
	output, err := s.Tools.Call(ctx, "list_sessions", args)
	if err != nil {
		return "Active review coverage is unknown because the session lookup failed.", nil
	}
	var sessions []json.RawMessage
	if len(output) > ToolResultLimit || json.Unmarshal([]byte(output), &sessions) != nil {
		return "Active review coverage is unknown because the session lookup was incomplete.", nil
	}
	if len(sessions) > 0 {
		return "This project has live sessions, but their coverage of this PR is unconfirmed. Check existing work before starting another review.", nil
	}
	return "No live project sessions were returned by the current lookup. Recheck before starting a review.", nil
}

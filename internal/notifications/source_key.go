package notifications

func subjectKey(sourceID string, event Event) string {
	subject := event.SubjectURL
	if subject == "" {
		subject = event.URL
	}
	if subject == "" {
		subject = event.ExternalID
	}
	return sourceID + ":" + subject
}

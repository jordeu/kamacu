package settings

import "testing"

func TestNotificationPolicyValidation(t *testing.T) {
	for _, raw := range []string{`{}`, `{"excluded_repositories":["acme/repo"],"ignore_drafts":true,"batch_review_task":"renovate"}`} {
		if _, err := ParseNotificationPolicy(raw); err != nil {
			t.Fatal(err)
		}
	}
	for _, raw := range []string{`null`, `[]`, `{} {}`, `{"unknown":true}`, `{"excluded_repositories":["https://github.com/acme/repo"]}`, `{"ignore_drafts":"yes"}`} {
		if _, err := ParseNotificationPolicy(raw); err == nil {
			t.Fatal("accepted", raw)
		}
	}
}

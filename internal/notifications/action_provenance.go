package notifications

// ActionEvidence preserves historical source provenance in saved action plans.
type ActionEvidence struct {
	Scope  string `json:"scope"`
	Intent string `json:"intent"`
	Quote  string `json:"quote"`
}

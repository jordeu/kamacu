package notifications

import (
	"errors"
	"fmt"
	"strings"
)

const triageCardContextLimit = 24000
const inboxCardContextLimit = 32000

// Reserve space for explanatory text; unusually large groups can span cards,
// but every subject's revisions stay together and every source URL is retained.
const triageProvenanceLimit = 12000

func batchActionDetails(ids []int64, allowed map[int64]RawNotification) string {
	linked, warned := map[string]bool{}, map[string]bool{}
	var links, availability strings.Builder
	for _, id := range ids {
		event := allowed[id].Event
		if !linked[event.URL] {
			fmt.Fprintf(&links, "\n- %s (%s)", event.URL, event.Reason)
			linked[event.URL] = true
		}
		if len(event.ContextWarnings) > 0 && !warned[event.URL] && availability.Len() < 3000 {
			fmt.Fprintf(&availability, "\n- %s: Some current source context could not be loaded. Verify the linked notification before acting.", event.URL)
			warned[event.URL] = true
		}
	}
	detail := "\n\nRelated notifications:" + links.String()
	if availability.Len() > 0 {
		detail += "\n\nContext availability:" + availability.String()
	}
	return detail
}

func fitTriageContext(action *BatchAction, allowed map[int64]RawNotification) error {
	budget := min(triageCardContextLimit, inboxCardContextLimit-len(batchActionDetails(action.EventIDs, allowed)))
	const omitted = "\n\nAdditional detail omitted to fit this card. All related subjects are linked below."
	if budget < len(omitted) {
		return errors.New("notification subject links exceed the card size limit")
	}
	if len(action.Context) > budget {
		action.Context = strings.ToValidUTF8(action.Context[:budget-len(omitted)], "") + omitted
	}
	return nil
}

package inbox

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Reuse the persisted summary field so older messages and API clients retain
// their source text. This contract applies only to newly generated headlines.
const MaxHeadlineCharacters = 160

const HeadlineInstructions = `summary: this field is the inbox headline. Write one short, plain-text line saying what happened or what is new. Aim for 6–12 simple words, at most 160 characters. Lead with the concrete event or result, not a question, an instruction, a list of choices, or a generic task title. Keep a qualifier that changes the meaning, such as merged but not released, or a check still failing. Approval is not proof of merge readiness; a merge is not deployment. Put reasons, details, next steps and alternatives in explanation/context, and put the actual request in question. Do not invent an event or overstate success. Do not add Markdown headings, bullets or links. These examples illustrate wording only, not facts about the current message: "PR approval received"; "Fix merged; release still pending"; "Disk cost estimate changed"; "Task instructions could not be delivered". This instruction applies to the inbox summary field, not work_summary or summaries of source evidence.`

func ValidateGeneratedHeadline(text string) error {
	if strings.TrimSpace(text) == "" || !utf8.ValidString(text) || utf8.RuneCountInString(text) > MaxHeadlineCharacters || strings.ContainsAny(text, "\r\n") {
		return fmt.Errorf("headline (summary) must be a nonempty single line of at most %d characters", MaxHeadlineCharacters)
	}
	return nil
}

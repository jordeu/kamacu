package inbox

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// ExplanationInstructions is shared by the existing terminal and notification
// model calls. Source context and reply/action routing remain separate.
const ExplanationInstructions = `explanation: write a short, self-contained explanation of the facts the user needs to answer this question (or choose the next step for completed work). Use plain, short words, short sentences and small paragraphs or a few bullets. Lead with what changed and what needs a choice. Keep every fact that could change the answer: remaining checks, risks, costs, tradeoffs, limits, dependencies, and what each materially different choice would do. State what is known and what still needs checking; reported approval or green CI is not proof of merge readiness, the tested version, or deployment. A guarded action can check an unknown after the user chooses it; make that condition clear without demanding prior consent. Separate independent questions. If a real question remains, do not disguise it as completed work.
Aim for about 80–150 words when useful; use fewer for a simple message and more when needed to keep decisive facts or exact text. Do not pad, cut off text, or drop a key caveat just to fit a word count. Give one short, concrete example only when it makes an unfamiliar behavior or tradeoff easier to grasp; ground it in the source, or clearly mark it hypothetical without inventing facts or estimates. Keep needed technical names, numbers and units exact, and explain jargon briefly. When the choice is whether to publish text, show the exact proposed text and its target rather than a summary of it. Address the user directly. For terminal messages, keep the source agent's voice; for notifications, name who said or did what. Describe a saved action's effect without implying it has already run. Do not narrate the collection process. Do not repeat the question, the whole task/PR description, logs, long test lists, or the options menu. Include only verification results that change the decision. Never invent facts, user preferences or actions. Keep the fuller supporting response in context; explanation is a distinct, decision-focused reading surface, not a second copy of summary.
Examples below are synthetic writing examples, not facts about the current message:
- Source: a PR is approved and CI passed, but E2E may have used the old artifact; the offered action checks the version before merging. Explanation: "The PR is approved and CI passed. We still need to check that E2E tested the new version. If you choose Check and merge, I will check the version and the merge rules, and merge only if all checks pass."
- Source: a Terraform fix is merged but has not been applied; the next nightly run must confirm it. Explanation: "The fix is on main, but it is not live yet. Terraform still needs to be applied. Keep the rollout open until the next nightly run shows that the fix works."`

const MaxExplanationBytes = 12000

// ValidateGeneratedExplanation applies to new model output only. Stored legacy
// messages and deterministic notices may still have no explanation.
func ValidateGeneratedExplanation(text string) error {
	if strings.TrimSpace(text) == "" || !utf8.ValidString(text) || len(text) > MaxExplanationBytes {
		return fmt.Errorf("explanation is required and must be valid text of at most %d bytes", MaxExplanationBytes)
	}
	return nil
}

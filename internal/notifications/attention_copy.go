package notifications

import (
	"encoding/json"
	"fmt"
	"strings"

	"kamacu/internal/inboxaction"
)

type attentionCopy struct {
	summary, context, question, retryLabel, retrySummary string
}

// Describe actual journal outcomes. "Partial" is an internal run state; it does
// not tell the user whether a task exists, a prompt was sent, or work started.
func actionAttentionCopy(run ActionRun, uncertain, canRetry bool, result string) attentionCopy {
	out := attentionCopy{
		summary:      "Your requested action could not finish",
		question:     "Check the problem below. Would you like to retry the unfinished steps?",
		retryLabel:   "Retry unfinished steps",
		retrySummary: "Recheck the current state and retry unfinished steps. Reuse recorded successful steps without repeating them.",
	}
	latest := map[string]int{}
	for i, step := range run.Steps {
		key, _, _ := strings.Cut(step.Key, "/recheck_")
		latest[key] = i
	}
	failures := []ActionStep{}
	sent := 0
	for i, step := range run.Steps {
		if step.Kind == "diagnose_failure" {
			continue
		}
		key, _, _ := strings.Cut(step.Key, "/recheck_")
		if latest[key] != i {
			continue
		}
		if step.Kind == "send_session_message" && step.State == "succeeded" {
			sent++
		}
		if step.State == "failed" || step.State == "blocked" || step.State == "uncertain" || step.State == "running" {
			failures = append(failures, step)
		}
	}
	onlyReady := len(failures) > 0
	lines := []string{}
	for _, step := range failures {
		onlyReady = onlyReady && step.Kind == "wait_ready"
		var args struct {
			TaskID int64 `json:"task_id"`
			PR     int64 `json:"pr_number"`
		}
		_ = json.Unmarshal(step.Arguments, &args)
		target := ""
		if args.TaskID > 0 {
			target = fmt.Sprintf("Task %d: ", args.TaskID)
		} else if args.PR > 0 {
			target = fmt.Sprintf("PR %d: ", args.PR)
		}
		phase := map[string]string{"wait_ready": "could not confirm a safe agent input prompt", "start_task_agent": "could not start the agent", "open_review": "could not open the review workspace", "verify_review": "could not verify the current PR state", "send_session_message": "could not confirm instruction delivery", "create_task": "could not create the task", "move_task": "could not move the task", "close_session": "could not confirm session shutdown", "update_task": "could not update the task", "set_task_pr_link": "could not link the PR"}[step.Kind]
		if step.Kind == "send_session_message" && step.State == "blocked" {
			phase = "instructions were not sent"
		}
		if phase == "" {
			phase = "could not complete the requested step"
		}
		reason := strings.TrimPrefix(step.Result, ErrActionBlocked.Error()+": ")
		if len(reason) > 1200 {
			reason = strings.ToValidUTF8(reason[:1200], "") + "…"
		}
		if len(lines) < 8 {
			lines = append(lines, target+phase+". "+reason)
		}
	}
	if len(failures) > len(lines) {
		lines = append(lines, fmt.Sprintf("%d more stopped steps are listed in Original execution steps.", len(failures)-len(lines)))
	}
	if onlyReady && !uncertain {
		noun := "instruction"
		out.retryLabel = "Retry sending instructions"
		reviewsOnly := true
		seq := runSequence(run)
		for _, a := range seq.Actions {
			reviewsOnly = reviewsOnly && a.Kind == "start_reviews"
		}
		if reviewsOnly && len(seq.Actions) > 0 {
			noun = "review prompt"
			out.retryLabel = "Retry sending review prompts"
		}
		pending := len(failures)
		out.summary = fmt.Sprintf("Could not send %s: agent readiness was not confirmed", attentionCount(pending, noun))
		out.context = fmt.Sprintf("Kamacu stopped before sending %s. Existing tasks and recorded startup steps are preserved.", attentionCount(pending, noun))
		if sent > 0 {
			out.context += fmt.Sprintf(" %s succeeded; those will be skipped on retry.", attentionCount(sent, "other instruction delivery"))
		}
		out.context += "\n\nOpen the linked task terminals. If an agent asks you to trust a workspace, sign in, or choose an option, handle that in its terminal. If you enter anything there, provide fresh instructions in your reply so Kamacu can reassess the changed session."
		if canRetry {
			out.context += " If you only waited for startup and entered no input, retry once the normal input prompt is visible. Kamacu will recheck readiness and reuse tasks and sessions that are still available."
		} else {
			out.context += " Once the terminal is ready, provide new instructions. This saved action cannot be safely retried automatically."
		}
		out.question = "Check the linked terminals, then retry sending the pending instructions?"
		out.retrySummary = "Recheck the existing agents and send only instructions not already delivered. Reuse saved tasks and workspaces; do not create duplicate tasks or resend successful handoffs."
	} else {
		if len(failures) == 1 && len(lines) > 0 {
			out.summary = strings.TrimSuffix(strings.SplitN(lines[0], ". ", 2)[0], ".")
		}
		out.context = "Check the problem below and Original execution steps. Inspect any linked tasks to understand the current state."
		if canRetry {
			out.context += " Address the reported condition before retrying. Successful steps will be skipped."
		} else {
			out.context += " This saved action cannot be safely retried automatically; provide new instructions after resolving the problem."
		}
	}
	if !canRetry {
		out.question = "Inspect the problem and any linked tasks, then provide new instructions when it is resolved."
	}
	if uncertain {
		out.summary = "An action's outcome could not be confirmed"
		out.question = "Open the linked tasks to check what happened before requesting any further action."
		out.context = "A step may already have taken effect. Check the linked terminals and Original execution steps to confirm whether instructions were delivered or a task changed. Automatic retry is unavailable because it could repeat an action."
	}
	if len(lines) > 0 {
		out.context += "\n\nStopped steps:\n" + strings.Join(lines, "\n")
	} else {
		out.context += "\n\n" + result
	}
	return out
}

func runSequence(run ActionRun) (seq inboxaction.Sequence) {
	_ = json.Unmarshal(run.Action, &seq)
	return
}
func attentionCount(n int, noun string) string {
	if n != 1 {
		noun += "s"
	}
	return fmt.Sprintf("%d %s", n, noun)
}

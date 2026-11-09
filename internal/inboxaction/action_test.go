package inboxaction

import "testing"

func TestSequencesRejectCyclesDuplicateIDsAndUnsupportedOperations(t *testing.T) {
	good := Action{ID: "create", Kind: "create_investigation", ProjectID: 1, TaskName: "Investigate", Instructions: "Check the issue"}
	cases := []Sequence{
		{Version: 1, Actions: []Action{{ID: "danger", Kind: "shell"}}},
		{Version: 1, Actions: []Action{{ID: "later", Kind: "continue_task", ProjectID: 1, TaskRef: "create", Instructions: "Continue"}, good}},
		{Version: 1, Actions: []Action{good, good}},
		{Version: 1, Actions: []Action{{ID: "one", Kind: "start_reviews", ProjectID: 1, PRNumbers: []int64{7, 7}}}},
		{Version: 1, Actions: []Action{{ID: "one", Kind: "start_reviews", ProjectID: 1, PRNumbers: []int64{-1}}}},
		{Version: 1, Actions: []Action{{ID: "one", Kind: "ignore"}, {ID: "two", Kind: "ignore"}}},
		{Version: 1, Actions: []Action{{ID: "create", Kind: "complete_task", ProjectID: 1, TaskID: 1, Status: "todo"}}},
	}
	for _, seq := range cases {
		if err := seq.Validate(); err == nil {
			t.Fatal("invalid sequence accepted", seq)
		}
	}
	seq := Sequence{Version: 1, Actions: []Action{good, {ID: "update", Kind: "update_task", ProjectID: 1, TaskRef: "create", Description: "Source reference"}}}
	if err := seq.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestOperationIDsCannotCollideWithWorkflowStepPaths(t *testing.T) {
	for _, id := range []string{"reviews/pr/7", "run/submit", "op\nname", "op.name"} {
		seq := Sequence{Version: 1, Actions: []Action{{ID: id, Kind: "continue_task", ProjectID: 1, TaskID: 1, Instructions: "Inspect the change"}}}
		if err := seq.Validate(); err == nil {
			t.Fatal("unsafe operation ID accepted", id)
		}
	}
}

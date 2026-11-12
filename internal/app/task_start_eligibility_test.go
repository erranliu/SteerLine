package app

import (
	"testing"

	ports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func TestTaskStartEligibility(t *testing.T) {
	for _, test := range []struct {
		name, status, mode, action, code string
		active                           int
		merged                           bool
	}{
		{name: "manual first start", status: "open", mode: "manual", action: "start_task"},
		{name: "auto first start", status: "open", mode: "auto", action: "start_task", code: "wrong_mode"},
		{name: "explicit candidate in auto mode", status: "in_progress", mode: "auto", action: "start_candidate", active: 4},
		{name: "candidate limit", status: "in_progress", mode: "manual", action: "start_candidate", active: 5, code: "candidate_limit_reached"},
		{name: "observed merge", status: "in_progress", mode: "manual", action: "start_candidate", merged: true, code: "candidate_pr_merged"},
		{name: "discarded task", status: "discarded", mode: "manual", action: "start_task", code: "task_discarded"},
	} {
		t.Run(test.name, func(t *testing.T) {
			view := taskStartEligibility(test.mode, ports.NativeTaskDetail{
				TaskRecord: taskmodel.TaskRecord{Status: test.status}, CurrentMaterialCandidates: test.active, ObservedMergedCandidate: test.merged,
			}, true)
			if view.Action != test.action {
				t.Fatalf("action = %s, want %s", view.Action, test.action)
			}
			if test.code == "" {
				if len(view.Blockers) != 0 {
					t.Fatalf("unexpected blockers: %+v", view.Blockers)
				}
			} else if len(view.Blockers) != 1 || view.Blockers[0].Code != test.code || view.Blockers[0].Diagnostic == "" {
				t.Fatalf("expected structured %s blocker, got %+v", test.code, view.Blockers)
			}
		})
	}
}

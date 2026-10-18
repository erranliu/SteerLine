package app

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/erranliu/SteerLine/internal/hostcontract"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/product/query"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"github.com/erranliu/SteerLine/internal/taskrunner"
)

func TestCandidateReadFactsAcrossViews(t *testing.T) {
	for _, test := range []struct {
		name, participation, submit, observed, task, delivery string
		stop, async                                           bool
	}{
		{name: "pending_without_observation", participation: "active", submit: "pending"},
		{name: "unknown_without_observation", participation: "active", submit: "unknown"},
		{name: "ready_open_task", participation: "active", submit: "accepted", observed: "pr_ready", task: "in_progress"},
		{name: "abandoned_still_running", participation: "abandoned", submit: "accepted", observed: "running", stop: true},
		{name: "winner_still_running", participation: "completed", submit: "accepted", observed: "running", task: "completed", delivery: "winner"},
		{name: "completed_history_without_delivery", participation: "completed", submit: "accepted", observed: "running"},
		{name: "stop_pending_async_question", participation: "active", submit: "accepted", observed: "running", stop: true, async: true},
		{name: "unknown_runner_closed_task", participation: "active", submit: "accepted", observed: "future_status", task: "closed"},
		{name: "unknown_runner_discarded_task", participation: "abandoned", submit: "accepted", observed: "future_status", task: "discarded"},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := runnerstate.Candidate{ID: "candidate", TaskID: "task", RunID: "runner", Number: 2, Participation: test.participation, SubmitState: test.submit, StopRequested: test.stop}
			if test.observed != "" {
				c.Projection = &runnercontract.Run{RunID: "runner", Status: test.observed, Step: "step", Revision: 7, Controls: runnercontract.Controls{CanReply: true}, Execution: runnercontract.Execution{AttemptID: "original"}}
				if test.async {
					c.Projection.Question = &runnercontract.Question{ID: "original-question"}
				}
			}
			before, err := json.Marshal(c)
			if err != nil {
				t.Fatal(err)
			}
			graph := storageports.TaskGraphSnapshot{ByID: map[string]taskmodel.TaskRecord{"task": {TaskID: "task", Status: test.task}}}
			if test.delivery != "" {
				graph.Deliveries = map[string]taskmodel.TaskDeliveryRecord{"task": {TaskID: "task", SelectedWorkflowRunID: "candidate"}}
			}
			rt := taskrunner.Project(storageports.RunnerSnapshot{Graph: graph, Candidates: []runnerstate.Candidate{c}}, hostcontract.ServiceCheckpoint{})
			page := productServicePage(&rt)
			if len(page.Runs) != 1 {
				t.Fatalf("runs: %+v", page.Runs)
			}
			detail := page.Runs[0]
			historyPage := query.ProjectPage(storageports.ProjectView{Candidates: []runnerstate.Candidate{c}}, "", 0, api.ObservationSnapshot{}, api.ServiceConnection{}, nil, nil)
			taskPage := query.TaskDetail(storageports.NativeTaskDetail{Candidates: []runnerstate.Candidate{c}})
			if historyPage.History == nil || len(historyPage.History.WorkflowRuns) != 1 || len(taskPage.WorkflowRuns) != 1 {
				t.Fatal("sampled history or task candidates were not projected")
			}
			history := historyPage.History.WorkflowRuns[0]
			taskCandidate := taskPage.WorkflowRuns[0]
			wantStatus := test.observed
			if wantStatus == "" {
				wantStatus = test.submit
			}
			if detail.WorkflowStatus != wantStatus || detail.ExecutionStatus != wantStatus || history.WorkflowStatus != wantStatus || taskCandidate.WorkflowStatus != wantStatus {
				t.Fatalf("observation changed: detail=%+v history=%+v", detail, history)
			}
			if detail.Participation != test.participation || history.Participation != test.participation || detail.SubmissionStatus != test.submit || history.SubmissionStatus != test.submit || detail.StopRequested != test.stop || history.StopRequested != test.stop || taskCandidate.Participation != history.Participation || taskCandidate.SubmissionStatus != history.SubmissionStatus || taskCandidate.StopRequested != history.StopRequested {
				t.Fatalf("caller facts lost: detail=%+v history=%+v", detail, history)
			}
			if detail.RunID != c.ID || detail.ExecutionID != c.RunID || history.WorkflowRunID != c.ID || detail.WorkflowStage != history.WorkflowStage || detail.DeliveryOutcome != test.delivery {
				t.Fatalf("identity/stage/delivery mismatch: detail=%+v history=%+v", detail, history)
			}
			if test.stop && detail.Decision != nil {
				t.Fatal("stop request exposed answer qualification")
			}
			after, err := json.Marshal(c)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("read conversion mutated original facts")
			}
		})
	}
}

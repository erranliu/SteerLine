package query

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	backend "github.com/erranliu/SteerLine/internal/execution/backend"
	model "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func TestTM05PageProjection(t *testing.T) {
	t.Run("task_page_excludes_fixed_contract_and_private_delivery_evidence", func(t *testing.T) {
		input := contracts.NativeTaskDetail{TaskRecord: taskmodel.TaskRecord{TaskID: "77", Title: "中文任务", Version: 6, IdentityMapDigest: "private-map", ContractID: "private-contract"}, Contract: taskmodel.TaskContract{SpecRef: taskmodel.FileReference{Path: "docs/spec/task.spec.md", SHA256: "secret-document-hash"}, PlanRef: taskmodel.FileReference{Path: "docs/spec/task.plan.md"}}, Delivery: &model.TaskDeliveryRecord{SelectedWorkflowRunID: "winning-workflow", PRURL: "https://example.invalid/pr/7", Evidence: []byte(`{"private":true}`)}}
		page := TaskDetail(input)
		encoded, err := json.Marshal(page)
		if err != nil {
			t.Fatal(err)
		}
		if page.Version != 6 || page.SpecPath != input.Contract.SpecRef.Path || page.Delivery.WorkflowRunID != "winning-workflow" {
			t.Fatalf("lost displayed fact or version: %+v", page)
		}
		for _, forbidden := range []string{"task.plan.md", "secret-document-hash", "private-map", "private-contract", `"private"`, "acceptance_refs"} {
			if strings.Contains(string(encoded), forbidden) {
				t.Fatalf("private contract or evidence reached UI: %s", encoded)
			}
		}
	})
	t.Run("unique_delivery_projects_winner_and_loser_without_choosing_by_merge", func(t *testing.T) {
		rt := &contracts.Runtime{Runs: []*contracts.Run{{TaskID: "77", RunID: "late-merged", PR: model.PRState{State: "merged"}}, {TaskID: "77", RunID: "winner", PR: model.PRState{State: "merged"}}}}
		if page := ServicePage(rt); page.Runs[0].DeliveryOutcome != "" || page.Runs[1].DeliveryOutcome != "" {
			t.Fatal("merge observations became a unique delivery")
		}
		rt.Deliveries = []model.TaskDeliveryRecord{{TaskID: "77", SelectedWorkflowRunID: "winner"}}
		page := ServicePage(rt)
		if page.Runs[0].DeliveryOutcome != "loser" || page.Runs[1].DeliveryOutcome != "winner" {
			t.Fatalf("lost authoritative sole delivery: %+v", page.Runs)
		}
	})
	t.Run("conversation_projects_exact_body_raw_and_source_without_checkpoint", func(t *testing.T) {
		input := contracts.ConversationEvent{ProjectID: "project", WorkflowID: "workflow-b", ExecutionID: "execution-b", AttemptID: "attempt-b", SessionID: "session-b", Source: "canonical", SourceSegment: "segment", SourceCursor: "private-source-checkpoint", ExternalRef: "private-envelope", Body: &contracts.ConversationBodyRef{ExecutionID: "execution-b", AttemptID: "attempt-b", ItemKey: "item", Revision: 3, SHA256: "body-digest", Completeness: contracts.BodyComplete}, Raw: &contracts.ConversationRawRef{ExecutionID: "execution-b", AttemptID: "attempt-b", ID: "raw", SHA256: "raw-digest"}, Attributes: map[string]any{"reason": "中文😀", "private": "private-native-envelope"}}
		page, err := ConversationEvent(input)
		if err != nil {
			t.Fatal(err)
		}
		if page.Body.Revision != 3 || page.Raw.ID != "raw" || page.SessionID != input.SessionID || page.SourceSegment != input.SourceSegment {
			t.Fatalf("lost exact source/body/raw target: %+v", page)
		}
		page.Body.ItemKey = "changed"
		page.EndReason = "changed"
		if input.Body.ItemKey != "item" || input.Attributes["reason"] != "中文😀" {
			t.Fatal("page mutation changed canonical source")
		}
		body, err := json.Marshal(page)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), "private-source-checkpoint") || strings.Contains(string(body), "private-envelope") || strings.Contains(string(body), "private-native-envelope") {
			t.Fatalf("private source fields reached UI: %s", body)
		}
		input.Attributes["invalid"] = make(chan int)
		if _, err := ConversationEvent(input); err == nil {
			t.Fatal("invalid JSON attributes silently lost")
		}
	})
	t.Run("candidate_excludes_private_execution_and_copies_page_values", func(t *testing.T) {
		now := time.Now().UTC()
		path, summary := "workspace-b", "中文😀"
		r := &contracts.Run{RunID: "workflow-b", ExecutionID: "execution-b", AttemptID: "attempt-b", SessionID: "session-b", ControlRevision: 9, WorktreePath: &path, Summary: &summary, CandidateEndedAt: &now, Process: &backend.ProcessIdentity{PID: 41, Terminal: "terminal-b"}, Decision: &model.DecisionState{ID: "decision-b", Version: 3, Request: model.DecisionRequest{Question: "继续？", Options: []backend.DecisionOption{{ID: "yes", Label: "继续", Action: "private-authority"}}}}, IdentityMapDigest: "private-map", WorkflowConfigDigest: "private-config"}
		page := Candidate(r)
		if page.RunID != r.RunID || page.ExecutionID != r.ExecutionID || page.SessionID != r.SessionID || page.ControlRevision != 9 || page.Process.Terminal != "terminal-b" {
			t.Fatalf("lost exact target: %+v", page)
		}
		*page.WorktreePath = "changed"
		page.Decision.Request.Options[0].Label = "changed"
		if path != "workspace-b" || r.Decision.Request.Options[0].Label != "继续" {
			t.Fatal("page mutation changed internal state")
		}
		body, err := json.Marshal(page)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"private-authority", "private-map", "private-config", `"pid"`, "question_route", "implementation_data"} {
			if strings.Contains(string(body), forbidden) {
				t.Fatalf("private state escaped page: %s", body)
			}
		}
	})
	t.Run("history_retains_complete_candidate_numbering_without_private_records", func(t *testing.T) {
		input := contracts.DomainSnapshot{WorkflowRuns: []model.WorkflowRunRecord{{WorkflowRunID: "exited", TaskID: "77", Phase: "abandoned"}, {WorkflowRunID: "active", TaskID: "77", Phase: "running"}}, Attempts: []model.StepAttemptRecord{{ExecutionID: "execution-b", AttemptID: "attempt-b", AttemptNumber: 2}}, Decisions: []model.DecisionRecord{{DecisionID: "decision-b", WorkflowRunID: "active", Request: []byte(`{"question":"继续？","options":[]}`)}, {DecisionID: "bad-original", Request: []byte("invalid")}}}
		page := History(input)
		if len(page.WorkflowRuns) != 2 || page.WorkflowRuns[0].WorkflowRunID != "exited" || page.Attempts[0].AttemptNumber != 2 || page.Decisions[0].Request.Question != "继续？" || page.Decisions[1].Diagnostic == "" {
			t.Fatalf("history guessed or lost canonical facts: %+v", page)
		}
	})
}

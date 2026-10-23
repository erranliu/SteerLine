package query

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	backend "github.com/erranliu/SteerLine/internal/execution/backend"
	model "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/hostcontract"
	api "github.com/erranliu/SteerLine/internal/product/api"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func TestTM05PageProjection(t *testing.T) {
	t.Run("task_page_excludes_fixed_contract_and_private_delivery_evidence", func(t *testing.T) {
		input := storageports.NativeTaskDetail{TaskRecord: taskmodel.TaskRecord{TaskID: "77", Title: "中文任务", Version: 6, IdentityMapDigest: "private-map", ContractID: "private-contract"}, Contract: taskmodel.TaskContract{SpecRef: taskmodel.FileReference{Path: "SteerlineFlow/spec/task.spec.md", SHA256: "secret-document-hash"}, PlanRef: taskmodel.FileReference{Path: "SteerlineFlow/plans/task.plan.md"}}, Delivery: &taskmodel.TaskDeliveryRecord{SelectedWorkflowRunID: "winning-workflow", PRURL: "https://example.invalid/pr/7", Evidence: []byte(`{"private":true}`)}}
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
		rt := &ServiceView{Runs: []*CandidateView{{TaskID: "77", RunID: "late-merged", PR: model.PRState{State: "merged"}}, {TaskID: "77", RunID: "winner", PR: model.PRState{State: "merged"}}}}
		if page := ServicePage(rt); page.Runs[0].DeliveryOutcome != "" || page.Runs[1].DeliveryOutcome != "" {
			t.Fatal("merge observations became a unique delivery")
		}
		rt.Deliveries = []taskmodel.TaskDeliveryRecord{{TaskID: "77", SelectedWorkflowRunID: "winner"}}
		page := ServicePage(rt)
		if page.Runs[0].DeliveryOutcome != "loser" || page.Runs[1].DeliveryOutcome != "winner" {
			t.Fatalf("lost authoritative sole delivery: %+v", page.Runs)
		}
	})
	t.Run("conversation_projects_exact_body_raw_and_source_without_checkpoint", func(t *testing.T) {
		input := model.ConversationEvent{ProjectID: "project", WorkflowID: "workflow-b", ExecutionID: "execution-b", AttemptID: "attempt-b", SessionID: "session-b", Source: "canonical", SourceSegment: "segment", SourceCursor: "private-source-checkpoint", ExternalRef: "private-envelope", Body: &model.ConversationBodyRef{ExecutionID: "execution-b", AttemptID: "attempt-b", ItemKey: "item", Revision: 3, SHA256: "body-digest", Completeness: model.BodyComplete}, Attributes: map[string]any{"reason": "中文😀", "private": "private-native-envelope"}}
		page, err := ConversationEvent(input)
		if err != nil {
			t.Fatal(err)
		}
		if page.Body.Revision != 3 || page.SessionID != input.SessionID || page.SourceSegment != input.SourceSegment {
			t.Fatalf("lost exact source/body target: %+v", page)
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
		r := &CandidateView{RunID: "workflow-b", ExecutionID: "execution-b", AttemptID: "attempt-b", SessionID: "session-b", ControlRevision: 9, WorktreePath: &path, Summary: &summary, CandidateEndedAt: &now, Process: &hostcontract.ProcessIdentity{PID: 41, Terminal: "terminal-b"}, Decision: &model.DecisionState{ID: "decision-b", Version: 3, Request: model.DecisionRequest{Question: "继续？", Options: []backend.DecisionOption{{ID: "yes", Label: "继续", Action: "private-authority"}}}}}
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

}

func TestCommandReceiptProjectsTypedDiscardResult(t *testing.T) {
	discard := api.TaskDiscardReceipt{RequestID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", TaskID: "8", Status: "discarded", GraphRevision: 12}
	result, err := json.Marshal(discard)
	if err != nil {
		t.Fatal(err)
	}
	command := CommandReceipt(storageports.CommandRecord{ProjectIdentity: "project", CommandID: discard.RequestID, Kind: "discard_task",
		TargetID: discard.TaskID, ExpectedVersion: 4, RequestedBy: "operator", Status: "accepted", Result: result})
	if command.Discard == nil || command.Discard.RequestID != discard.RequestID || command.Discard.GraphRevision != 12 || command.ResultCode != "" {
		t.Fatalf("discard command lost its typed receipt: %+v", command)
	}
	other := CommandReceipt(storageports.CommandRecord{ProjectIdentity: "project", CommandID: discard.RequestID, Kind: "answer_decision", Result: result})
	if other.Discard != nil {
		t.Fatalf("non-discard command exposed an unrelated result: %+v", other)
	}
}

// This original client assertion follows the unique ready-candidate projector.
func TestNativeCandidateSnapshotKeepsDispatchOrderBeyondDetailPage(t *testing.T) {
	at := time.Now().UTC()
	ready := storageports.NativeReadyTaskSnapshot{SampledAt: at}
	for i := 501; i >= 1; i-- {
		ready.Tasks = append(ready.Tasks, taskmodel.TaskRecord{TaskID: fmt.Sprint(i), Title: "ready", Type: "task", Status: "open"})
	}
	snapshot := ReadyCandidates(ready, at, api.ServiceConnection{State: api.ConnectionConnected}, []string{"task"})
	if len(snapshot.Tasks) != 501 || snapshot.Tasks[0].TaskID != "501" || snapshot.Tasks[500].TaskID != "1" {
		t.Fatalf("ready candidates were truncated or reordered by task ID: %d", len(snapshot.Tasks))
	}
}

func TestCommandReceiptProjectsTypedRequestIdentity(t *testing.T) {
	answer := api.DecisionAnswer{RunID: "original-run", DecisionID: "original-decision", OptionID: "yes", Note: "original text", Answers: map[string][]string{"q": {"choice", "remarks"}}, ExpectedDecisionVersion: 3, ExpectedRunRevision: 7, SourceAttemptID: "original-attempt", ExpectedRecoveryVersion: 2, ServiceInstanceID: "original-instance", AnsweredBy: "operator"}
	body, _ := json.Marshal(answer)
	receipt := CommandReceipt(storageports.CommandRecord{ProjectIdentity: "project", CommandID: "command", Kind: "answer_decision", TargetID: answer.DecisionID, ExpectedVersion: 3, Payload: body})
	if receipt.DecisionAnswer == nil || receipt.DecisionAnswer.ServiceInstanceID != answer.ServiceInstanceID || receipt.DecisionAnswer.SourceAttemptID != answer.SourceAttemptID || receipt.DecisionAnswer.ExpectedRecoveryVersion != 2 || receipt.DecisionAnswer.Answers["q"][1] != "remarks" {
		t.Fatalf("lost original answer identities/content: %+v", receipt)
	}
	receipt.DecisionAnswer.Answers["q"][1] = "changed"
	if string(body) == "changed" || answer.Answers["q"][1] != "remarks" {
		t.Fatal("receipt mutation changed authority input")
	}
	discard := api.DiscardInput{TaskID: "8", Reason: "original reason", RequestedBy: "operator", ExpectedVersion: 4}
	body, _ = json.Marshal(discard)
	receipt = CommandReceipt(storageports.CommandRecord{ProjectIdentity: "project", CommandID: "command", Kind: "discard_task", TargetID: "8", ExpectedVersion: 4, Payload: body})
	if receipt.DiscardRequest == nil || receipt.DiscardRequest.RequestID != "command" || receipt.DiscardRequest.Reason != discard.Reason || receipt.DiscardRequest.ExpectedVersion != 4 {
		t.Fatalf("lost discard request: %+v", receipt)
	}
	receipt = CommandReceipt(storageports.CommandRecord{Kind: "answer_decision", Payload: []byte("invalid")})
	if receipt.DecisionAnswer != nil {
		t.Fatal("malformed request was treated as verified content")
	}
}

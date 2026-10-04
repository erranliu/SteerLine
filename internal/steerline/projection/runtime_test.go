package projection

import (
	"encoding/json"
	"testing"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func projectionFixture(t *testing.T) (contracts.DomainSnapshot, Metadata) {
	t.Helper()
	at := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	implementationData, err := executionbackend.NewImplementationDataEnvelope(executionbackend.ImplementationDataEnvelope{CodecVersion: 1, Purpose: executionbackend.ImplementationConfigPurpose, ImplementationKey: "codex.worker-profile", ProjectID: "project", ProfileID: "default", AcceptedConfigVersion: "accepted-v2"}, []byte(`{"args":["original"]}`))
	if err != nil {
		t.Fatal(err)
	}
	request, err := json.Marshal(executionmodel.DecisionRequest{FormatVersion: executionmodel.DecisionRequestFormatVersion, Question: "Continue?", AllowFreeText: true})
	if err != nil {
		t.Fatal(err)
	}
	answer, err := json.Marshal(executionmodel.DecisionAnswer{DecisionID: "decision", RunID: "run", Note: " continue ", AnsweredBy: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := contracts.DomainSnapshot{
		ProjectIdentity: "project",
		Tasks:           []taskmodel.TaskRecord{{TaskID: "task", Title: "Task", Status: "in_progress"}},
		WorkflowRuns: []executionmodel.WorkflowRunRecord{{WorkflowRunID: "run", TaskID: "task", WorkflowVersion: executionmodel.Version, SelectedResult: "execution", Phase: "waiting_decision", ControlRevision: 42,
			ExecutionProfile: &executionmodel.FixedExecutionProfile{Version: 5, Host: "local", ImplementationData: []executionbackend.ImplementationDataEnvelope{implementationData}}}},
		Executions: []executionmodel.ExecutionRecord{{ExecutionID: "execution", WorkflowRunID: "run", CurrentStep: "worker", CurrentAttemptID: "attempt", Backend: string(executionbackend.BackendLocal), Protocol: string(executionbackend.ProtocolLocalCodexAppServer), Agent: "codex", Status: "active", DecisionID: "decision",
			Process: &executionbackend.ProcessIdentity{Kind: "codex", PID: 12}, WorkspaceIntent: &executionbackend.WorkspaceIntent{Path: "/work/tree", Branch: "codex/task"}, TurnStartedAt: &at}},
		Attempts:  []executionmodel.StepAttemptRecord{{AttemptID: "attempt", ExecutionID: "execution", StepID: "worker", AttemptNumber: 1}},
		Decisions: []executionmodel.DecisionRecord{{DecisionID: "decision", QuestionID: "decision", ProjectIdentity: "project", WorkflowRunID: "run", ExecutionID: "execution", AttemptID: "attempt", Version: 2, Status: "answer_saved", Request: request, Answer: answer}},
	}
	metadata := Metadata{Checkpoint: contracts.ServiceCheckpoint{SchemaVersion: contracts.WorkerFlowRuntimeSchema, Revision: 7, HeartbeatAt: at,
		Repository: contracts.Repository{Identity: "project"}, Service: contracts.ServiceState{Capabilities: []string{"answer_decision"}},
		Operation: &contracts.ControlOperation{RequestID: "control", Payload: json.RawMessage(`{"task":"task"}`)}},
		DispatchMode: "manual", DispatchPaused: true, AuthorityRevision: 9}
	return snapshot, metadata
}

func TestBuildOwnsRecordsAndMetadataAcrossRefreshes(t *testing.T) {
	snapshot, metadata := projectionFixture(t)
	first, err := Build(snapshot, metadata)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Build(snapshot, metadata)
	if err != nil {
		t.Fatal(err)
	}
	if first.Control.Revision != 9 || first.Revision != 7 || first.Runs[0].ControlRevision != 42 || first.Runs[0].Decision.Note != "continue" || first.Runs[0].Host != "local" {
		t.Fatalf("authority, hidden control revision or decision projection changed: %+v", first)
	}
	if first.Runs[0] == second.Runs[0] || first.Runs[0].Process == snapshot.Executions[0].Process || first.Runs[0].WorkspaceIntent == first.Executions[0].WorkspaceIntent {
		t.Fatal("Run or nested input identity leaked between views")
	}
	first.Runs[0].Process.PID = 999
	first.Runs[0].WorkspaceIntent.Path = "view-only"
	first.Runs[0].Decision.Request.Question = "view-only question"
	first.Executions[0].WorkspaceIntent.Path = "row-only"
	first.WorkflowRuns[0].ExecutionProfile.ImplementationData[0].Payload[0] = '['
	first.Service.Capabilities[0] = "changed"
	first.Control.Operation.Payload[0] = 'x'
	if snapshot.Executions[0].Process.PID != 12 || snapshot.Executions[0].WorkspaceIntent.Path != "/work/tree" || snapshot.WorkflowRuns[0].ExecutionProfile.ImplementationData[0].Payload[0] != '{' || metadata.Checkpoint.Service.Capabilities[0] != "answer_decision" || metadata.Checkpoint.Operation.Payload[0] != '{' {
		t.Fatal("view mutation changed snapshot or metadata")
	}
	if second.Runs[0].Process.PID != 12 || second.Runs[0].WorkspaceIntent.Path != "/work/tree" || second.Runs[0].Decision.Request.Question != "Continue?" {
		t.Fatal("view mutation changed a separate refresh")
	}
	snapshot.Executions[0].Process.PID = 123
	if second.Runs[0].Process.PID != 12 || second.Executions[0].Process.PID != 12 {
		t.Fatal("later input mutation changed an existing sample")
	}
}

func TestProjectionRejectsBrokenBindingsWithoutReusingOldViews(t *testing.T) {
	snapshot, metadata := projectionFixture(t)
	for _, mutation := range []func(*contracts.DomainSnapshot){
		func(s *contracts.DomainSnapshot) { s.WorkflowRuns[0].SelectedResult = "another-execution" },
		func(s *contracts.DomainSnapshot) {
			s.Decisions[0].Answer = []byte(`{"decision_id":"decision","run_id":"another-run","note":"continue","answered_by":"operator"}`)
		},
	} {
		broken, err := CloneDomain(snapshot)
		if err != nil {
			t.Fatal(err)
		}
		mutation(&broken)
		if _, err := Build(broken, metadata); err == nil {
			t.Fatal("broken execution or saved-answer identity accepted")
		}
	}
	old := contracts.Runtime{Runs: []*contracts.Run{{RunID: "old", ExecutionID: "orphan", ExecutionStatus: "running"}}}
	view, err := Build(contracts.DomainSnapshot{}, MetadataOf(old))
	if err != nil || len(view.Runs) != 0 || view.SlotOccupied {
		t.Fatalf("old client view filled missing authority: %+v %v", view, err)
	}
}

func TestProjectionRetainsDeliveryAndReservationRules(t *testing.T) {
	snapshot, metadata := projectionFixture(t)
	snapshot.Tasks[0].Status = "closed"
	snapshot.Deliveries = []contracts.TaskDeliveryRecord{{TaskID: "task", SelectedWorkflowRunID: "run", PRURL: "https://example.invalid/pull/1", MergeCommit: "merged"}}
	view, err := Build(snapshot, metadata)
	if err != nil || view.Runs[0].ExecutionStatus != "succeeded" || view.Runs[0].PR.State != "merged" || *view.Runs[0].PR.MergeCommit != "merged" {
		t.Fatalf("delivery facts changed: %+v %v", view, err)
	}
	snapshot.Tasks[0].Status, snapshot.WorkflowRuns[0].Phase = "discarded", "abandoned"
	view, err = Build(snapshot, Metadata{Checkpoint: metadata.Checkpoint})
	if err != nil || len(view.Runs) != 0 {
		t.Fatalf("settled discarded run retained: %+v %v", view, err)
	}
	reserved := "unclaimed"
	metadata.Checkpoint.ReservedTask = &reserved
	view, err = Build(contracts.DomainSnapshot{}, metadata)
	if err != nil || !view.SlotOccupied || view.CurrentTask == nil || *view.CurrentTask != reserved || view.CurrentTask == &reserved {
		t.Fatalf("reservation lost or aliased input: %+v %v", view, err)
	}
}

func TestExecutionProjectionKeepsRemotePathAndWhitespaceSemantics(t *testing.T) {
	execution := executionmodel.ExecutionRecord{Backend: string(executionbackend.BackendOrca), Protocol: string(executionbackend.ProtocolOrcaSupervised), Target: executionbackend.ExecutionTarget{ConnectionID: "peer", ExternalRuntime: "remote"}, WorkspaceIntent: &executionbackend.WorkspaceIntent{Path: "service-local-path", Branch: "codex/task"}, Status: "idle", LastTurnOutcome: " "}
	run := Execution(contracts.Run{}, execution)
	if run.WorktreePath != nil || *run.WorktreeBranch != "codex/task" || executionViewStatus(execution) != " " || workflowExecutionHost(executionmodel.WorkflowRunRecord{ExecutionProfile: &executionmodel.FixedExecutionProfile{Host: "orca_supervised"}}) != "orca_supervised" {
		t.Fatal("remote intent was shown as an actual local workspace or whitespace fallback changed")
	}
}

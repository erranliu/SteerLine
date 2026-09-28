package steerline

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestSupervisedShutdownDetachesObserverWithoutStoppingDispatch(t *testing.T) {
	detached := make(chan struct{})
	worker := &Worker{
		cancelObservation: func() { close(detached) },
		observationDone:   detached,
	}
	run := &Run{
		RunID:           newID(),
		Protocol:        string(ProtocolOrcaSupervised),
		ExecutionStatus: "running",
		Process:         &ProcessIdentity{Kind: "orca_supervised", Dispatch: "original-dispatch"},
	}
	s := &Service{Config: Config{ShutdownGrace: "1s"}, active: run, worker: worker}
	s.shutdownWorker()
	if s.worker != nil || run.ExecutionStatus != "running" || run.Process == nil || run.Process.Dispatch != "original-dispatch" {
		t.Fatalf("shutdown changed the live remote execution instead of detaching observation: %+v", run)
	}
	select {
	case <-detached:
	default:
		t.Fatal("supervised observer remained attached after shutdown")
	}
}

func TestUnknownMinimalStepPersistsOriginalBindingAndBlocksOnlyItsRun(t *testing.T) {
	f := newFixture(t)
	s := f.service(t)
	t.Cleanup(func() { _ = s.Close() })
	authority, err := s.domainStore.nativeAuthority(context.Background(), f.c.Identity())
	if err != nil {
		t.Fatal(err)
	}
	batch, err := json.Marshal(TaskBatchInput{SchemaVersion: "task-batch/v1", ProjectID: f.c.Identity(), Tasks: []TaskBatchTask{{LocalKey: "task", TaskID: 101, Title: "Unknown outcome", Type: "task", Contract: nativeContract(t, f.c, f.commit, "contract-101")}}, Dependencies: []TaskBatchEdge{}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = (CommandService{Store: s.domainStore, Project: f.c.Identity()}).CreateTaskBatch(context.Background(), f.c, batch, newID(), authority.AuthorityVersion, "test"); err != nil {
		t.Fatal(err)
	}
	page, err := (QueryService{Store: s.domainStore, Project: f.c.Identity()}).ShowNativeTask(context.Background(), "101")
	if err != nil || len(page.Tasks) != 1 {
		t.Fatalf("created native Task cannot be read: %+v, %v", page, err)
	}
	s.State.Tasks = append(s.State.Tasks, page.Tasks[0].TaskRecord)
	when := time.Now().UTC()
	runID, executionID, attemptID := newID(), newID(), newID()
	identity := &ProcessIdentity{Kind: "orca_supervised", Dispatch: "original-dispatch"}
	run := &Run{TaskID: "101", DomainTaskID: "101", RunID: runID, ExecutionID: executionID, AttemptID: attemptID, WorkflowVersion: MinimalWorkflowVersion, Protocol: string(ProtocolOrcaSupervised), ExecutionStatus: "running", FinishTaskStatus: "pending", Process: identity}
	workflow := WorkflowRunRecord{ProjectIdentity: f.c.Identity(), WorkflowRunID: runID, TaskID: "101", WorkflowVersion: MinimalWorkflowVersion, Phase: "executing", CreatedAt: when, UpdatedAt: when}
	execution := ExecutionRecord{ProjectIdentity: f.c.Identity(), ExecutionID: executionID, WorkflowRunID: runID, Status: "active", CurrentStep: "verify", Process: identity, CreatedAt: when, UpdatedAt: when}
	attempt := StepAttemptRecord{ProjectIdentity: f.c.Identity(), AttemptID: attemptID, ExecutionID: executionID, StepID: "verify", AttemptNumber: 1, Status: "running", CreatedAt: when, UpdatedAt: when}
	s.State.Runs = append(s.State.Runs, run)
	s.State.WorkflowRuns = append(s.State.WorkflowRuns, workflow)
	s.State.Executions = append(s.State.Executions, execution)
	s.State.Attempts = append(s.State.Attempts, attempt)
	worker := &Worker{Events: make(chan WorkerEvent), Done: make(chan error, 1)}
	close(worker.Events)
	s.active, s.worker = run, worker
	otherID, otherWorker := newID(), &Worker{}
	s.workers = map[string]*Worker{runID: worker, otherID: otherWorker}
	cause := &BackendError{Code: BackendIdentityMismatch, Diagnostic: "original Dispatch identity changed"}
	if err = s.finishWorkerInLoop(cause); err != nil {
		t.Fatalf("durably blocked unknown outcome stopped the service loop: %v", err)
	}
	if run.ExecutionStatus != "unknown" || run.FinishTaskStatus != "blocked" || run.Process == nil || run.Process.Dispatch != "original-dispatch" || s.State.Attempts[len(s.State.Attempts)-1].Status != "outcome_unknown" || !s.State.SlotOccupied || s.State.CurrentTask == nil || *s.State.CurrentTask != "101" || len(s.workers) != 1 || s.workers[otherID] != otherWorker {
		t.Fatalf("unknown result lost its original binding or execution fence: run=%+v attempt=%+v", run, s.State.Attempts[len(s.State.Attempts)-1])
	}
	saved, loadErr := LoadDatabaseRuntime(f.c)
	if loadErr != nil || saved == nil || len(saved.Runs) == 0 || !strings.Contains(deref(saved.Runs[len(saved.Runs)-1].Error), "original Dispatch identity changed") {
		t.Fatalf("unknown result was not durably saved: %v, %+v", loadErr, saved)
	}
}

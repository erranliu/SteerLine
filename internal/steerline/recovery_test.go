package steerline

import (
	"context"
	"strings"
	"testing"
)

func TestMarkStoppedKeepsServiceRunningAndRetainsOtherUnknownSlot(t *testing.T) {
	f, service, run := failedStartup(t)
	defer service.Close()
	records := executionRecords{root: f.c.RunDir}
	start, err := records.loadOperation(run.RunID, run.AttemptID, attemptOperationID(run.RunID, run.AttemptID))
	require(t, err)
	start.Status = OperationUnknown
	require(t, records.saveOperation(start))
	run.ExecutionStatus = "unknown"
	otherID := newID()
	other := &Run{TaskID: "other-unknown", RunID: otherID, AttemptID: otherID, Claimant: f.c.Claimant, Host: "local", Backend: string(BackendLocal), Protocol: string(ProtocolLocalProcess), WorkflowVersion: "legacy-v1", ExecutionID: "legacy-execution:" + otherID, ExecutionStatus: "unknown", FinishTaskStatus: "blocked", PR: PRState{State: "unknown"}}
	service.State.Runs = append(service.State.Runs, other)
	service.State.CurrentTask, service.State.SlotOccupied = ptr(run.TaskID), true
	require(t, service.save())
	statusBefore := service.State.Service.Status
	require(t, service.markStoppedCore(run.RunID, "operator", "I checked the original local process tree and confirmed it is stopped."))
	if run.ExecutionStatus != "failed" || run.FinishTaskStatus != "blocked" || run.ManualStop == nil || run.ManualStop.Basis == "" {
		t.Fatalf("manual stop did not retain its audited failed/blocked result: %+v", run)
	}
	if service.State.Service.Status != statusBefore || service.State.Service.Status == "stopped" {
		t.Fatalf("mark-stopped changed the live service lifecycle: %s -> %s", statusBefore, service.State.Service.Status)
	}
	if !service.State.SlotOccupied || service.State.CurrentTask == nil || *service.State.CurrentTask != other.TaskID {
		t.Fatalf("resolving one unknown cleared another slot reservation: slot=%v current=%v", service.State.SlotOccupied, service.State.CurrentTask)
	}
	stored, err := records.loadOperation(run.RunID, run.AttemptID, start.ID)
	require(t, err)
	if stored.Status != OperationUnknown {
		t.Fatalf("mark-stopped rewrote the original uncertain operation: %s", stored.Status)
	}
}

func TestInFlightRetryCommandIsNeverReplayedAndCanBeResolved(t *testing.T) {
	f, service, run := failedStartup(t)
	defer service.Close()
	service.State.Service.Status, service.State.Service.Ready = "running", true
	require(t, service.save())
	commands := CommandService{Store: service.domainStore, Project: service.Config.identity}
	originalID := newID()
	originalBody := mustJSON(RecoveryCommandBody{ConfirmedProcessStopped: true, ConfirmationBasis: "operator checked the original process"})
	originalPayload := mustJSON(ServiceCommandPayload{ServiceInstanceID: service.State.Service.InstanceID, ExpectedRevision: run.ControlRevision, RunAttemptID: run.AttemptID, Body: originalBody})
	original := CommandRecord{CommandID: originalID, Kind: "retry_run", TargetID: run.RunID, ExpectedVersion: run.ControlRevision, IdempotencyKey: originalID, RequestedBy: "operator", Payload: originalPayload, Status: "in_flight", CreatedAt: now()}
	if _, created, err := commands.Submit(context.Background(), original); err != nil || !created {
		t.Fatalf("publish in-flight retry evidence: created=%v err=%v", created, err)
	}
	startsBefore := readFake(f.db).Starts[run.TaskID]
	require(t, service.processPendingCommands(context.Background()))
	if service.worker != nil || readFake(f.db).Starts[run.TaskID] != startsBefore || run.ExecutionStatus != "unknown" || !service.State.SlotOccupied {
		t.Fatalf("in-flight retry was replayed or its slot released: worker=%v starts=%d run=%s slot=%v", service.worker != nil, readFake(f.db).Starts[run.TaskID], run.ExecutionStatus, service.State.SlotOccupied)
	}
	record, err := service.domainStore.Command(context.Background(), service.Config.identity, originalID)
	require(t, err)
	if record.Status != "indeterminate" {
		t.Fatalf("in-flight retry did not retain an indeterminate receipt: %+v", record)
	}
	if len(service.State.Control.Blockers) != 1 || service.State.Control.Blockers[0].RequestID != originalID {
		t.Fatalf("in-flight retry blocker was not scoped to the original command: %+v", service.State.Control.Blockers)
	}
	resolutionID := newID()
	resolutionPayload := mustJSON(ServiceCommandPayload{ServiceInstanceID: service.State.Service.InstanceID, ExpectedRevision: service.State.Control.Revision, Body: mustJSON(RecoveryCommandBody{ConfirmationBasis: "operator confirmed the original retry execution is stopped"})})
	resolution := CommandRecord{CommandID: resolutionID, Kind: "resolve_command", TargetID: originalID, ExpectedVersion: service.State.Control.Revision, IdempotencyKey: resolutionID, RequestedBy: "operator", Payload: resolutionPayload, Status: "pending", CreatedAt: now()}
	if _, created, err := commands.Submit(context.Background(), resolution); err != nil || !created {
		t.Fatalf("publish blocker resolution: created=%v err=%v", created, err)
	}
	require(t, service.processPendingCommands(context.Background()))
	if len(service.State.Control.Blockers) != 0 || run.ExecutionStatus != "failed" || run.ManualStop == nil || service.State.Service.Status == "stopped" {
		t.Fatalf("resolution did not record the stopped execution and preserve service state: blocker=%+v run=%+v service=%+v", service.State.Control.Blockers, run, service.State.Service)
	}
	original, err = service.domainStore.Command(context.Background(), service.Config.identity, originalID)
	require(t, err)
	resolved, err := service.domainStore.Command(context.Background(), service.Config.identity, resolutionID)
	require(t, err)
	if original.Status != "indeterminate" || resolved.Status != "accepted" || !strings.Contains(string(resolved.Result), "original_request_id") {
		t.Fatalf("resolution receipts lost their linkage: original=%+v resolution=%+v", original, resolved)
	}
}

func TestInspectRecoveryIsReadOnlyAndBindsRunRevision(t *testing.T) {
	f, service, run := failedStartup(t)
	defer service.Close()
	before, err := LoadDatabaseRuntime(f.c)
	require(t, err)
	startsBefore := readFake(f.db).Starts[run.TaskID]
	report, err := InspectRecovery(context.Background(), f.c, run.RunID)
	require(t, err)
	if report.RunID != run.RunID || report.RunRevision != run.ControlRevision || report.SampledAt.IsZero() {
		t.Fatalf("recovery report is not bound to the displayed run version: %+v", report)
	}
	after, err := LoadDatabaseRuntime(f.c)
	require(t, err)
	if after.Service.InstanceID != before.Service.InstanceID || after.Revision != before.Revision || after.Runs[0].ControlRevision != before.Runs[0].ControlRevision || readFake(f.db).Starts[run.TaskID] != startsBefore {
		t.Fatal("read-only recovery inspection mutated service or execution state")
	}
}

func TestRecoveryResultDigestRejectsChangedBytesBeforeAnyStateTransition(t *testing.T) {
	_, service, run := failedStartup(t)
	defer service.Close()
	run.ExecutionStatus, run.FinishTaskStatus = "unknown", "blocked"
	require(t, service.save())
	if err := service.acceptResultBytes(context.Background(), run.RunID, newID(), "operator", "I confirmed the process is stopped", []byte("result"), strings.Repeat("0", 64)); err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("changed result bytes reached recovery validation: %v", err)
	}
	if run.FinishTaskStatus != "blocked" || run.ExecutionStatus != "unknown" || run.PR.URL != nil {
		t.Fatalf("digest rejection mutated the run: %+v", run)
	}
}

func TestResolveControlCommandClearsOnlyVerifiedPreClaimBlocker(t *testing.T) {
	f := newFixture(t, task("target"))
	s := f.service(t)
	defer s.Close()
	req := controlRequestFor(s, "start_task", map[string]string{"task_id": "target"})
	publishForService(t, s, req)
	digest := rawRequestDigest(controlRequestPath(s.Config.RunDir, req.RequestID))
	receipt := s.baseReceipt(req, digest, "indeterminate", "indeterminate", "restart before claim reconciliation")
	receipt.CompletedAt = ptr(now())
	require(t, s.writeReceipt(receipt))
	runID := newID()
	s.State.Control.Blockers = append(s.State.Control.Blockers, ControlBlocker{RequestID: req.RequestID, Action: "start_task", TaskID: "target", RunID: runID, AttemptID: runID, Reason: "unknown start"})
	require(t, s.save())
	result, err := s.resolveControlBlocker(context.Background(), req.RequestID, "operator", "I verified the task remains open and unclaimed; no worker or worktree exists.")
	require(t, err)
	if len(s.State.Control.Blockers) != 0 || s.recoveryTarget(runID) != nil || result["original_request_id"] != req.RequestID {
		t.Fatalf("pre-claim blocker resolution changed another identity or reconstructed a run: blockers=%+v result=%+v", s.State.Control.Blockers, result)
	}
	original, err := ReadVerifiedControlReceipt(s.Config, req.RequestID)
	require(t, err)
	if original.Status != "indeterminate" {
		t.Fatalf("original uncertain receipt was rewritten: %+v", original)
	}
}

func TestResolveClaimOrphanPreservesPreallocatedIdentityAndMissingWorkspace(t *testing.T) {
	f := newFixture(t, task("owned"))
	s := f.service(t)
	defer s.Close()
	fake := readFake(f.db)
	fake.Tasks[0].Status, fake.Tasks[0].Assignee = "in_progress", f.c.Claimant
	writeFake(f.db, fake)
	req := controlRequestFor(s, "start_task", map[string]string{"task_id": "owned"})
	publishForService(t, s, req)
	digest := rawRequestDigest(controlRequestPath(s.Config.RunDir, req.RequestID))
	receipt := s.baseReceipt(req, digest, "indeterminate", "indeterminate", "claim result unknown")
	receipt.CompletedAt = ptr(now())
	require(t, s.writeReceipt(receipt))
	runID := newID()
	attemptID := newID()
	s.State.Control.Blockers = append(s.State.Control.Blockers, ControlBlocker{RequestID: req.RequestID, Action: "start_task", TaskID: "owned", RunID: runID, AttemptID: attemptID, Reason: "claim outcome unknown"})
	require(t, s.save())
	_, err := s.resolveControlBlocker(context.Background(), req.RequestID, "operator", "I confirmed the original execution is stopped; preserve the claimed task for manual repair.")
	require(t, err)
	recovered := s.find("owned")
	if recovered == nil || recovered.RunID != runID || recovered.AttemptID != attemptID || recovered.WorktreePath != nil || recovered.WorkspaceIntent != nil || recovered.ExecutionStatus != "failed" || recovered.FinishTaskStatus != "blocked" {
		t.Fatalf("orphan recovery fabricated workspace or lost its preallocated identity: %+v", recovered)
	}
	s.State.Control.DispatchMode = "manual"
	require(t, s.save())
	if err = s.startRetry(context.Background(), recovered.RunID); err == nil || !strings.Contains(err.Error(), "preparation intent") {
		t.Fatalf("orphan record gained retry eligibility without workspace evidence: %v", err)
	}
}

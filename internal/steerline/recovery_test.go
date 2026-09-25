package steerline

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
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
	service.State.Control.DispatchMode = "manual"
	require(t, service.save())
	commands := CommandService{Store: service.domainStore, Project: service.Config.identity}
	originalID := newID()
	originalBody := mustJSON(RecoveryCommandBody{ConfirmedProcessStopped: true, ConfirmationBasis: "operator checked the original process"})
	originalPayload := mustJSON(ServiceCommandPayload{ServiceInstanceID: service.State.Service.InstanceID, ExpectedRevision: run.ControlRevision, RunAttemptID: run.AttemptID, Body: originalBody})
	original := CommandRecord{CommandID: originalID, Kind: "retry_run", TargetID: run.RunID, ExpectedVersion: run.ControlRevision, IdempotencyKey: originalID, RequestedBy: "operator", Payload: originalPayload, Status: "in_flight", CreatedAt: now()}
	if _, created, err := commands.Submit(context.Background(), original); err != nil || !created {
		t.Fatalf("publish in-flight retry evidence: created=%v err=%v", created, err)
	}
	pendingID := newID()
	pending := CommandRecord{CommandID: pendingID, Kind: "retry_run", TargetID: run.RunID, ExpectedVersion: run.ControlRevision, IdempotencyKey: pendingID, RequestedBy: "operator", Payload: originalPayload, Status: "pending", CreatedAt: now()}
	if _, created, err := commands.Submit(context.Background(), pending); err != nil || !created {
		t.Fatalf("publish newer pending retry: created=%v err=%v", created, err)
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
	newer, err := service.domainStore.Command(context.Background(), service.Config.identity, pendingID)
	require(t, err)
	if newer.Status != "rejected" {
		t.Fatalf("newer retry bypassed the older in-flight operation: %+v", newer)
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

func TestInFlightMarkStoppedCannotBorrowPriorStopConfirmation(t *testing.T) {
	_, service, run := failedStartup(t)
	defer service.Close()
	run.ExecutionStatus, run.FinishTaskStatus = "unknown", "blocked"
	require(t, service.save())
	require(t, service.markStoppedCore(run.RunID, "first-operator", "the original process stopped"))
	commandID := newID()
	command := CommandRecord{CommandID: commandID, Kind: "mark_stopped", TargetID: run.RunID, ExpectedVersion: run.ControlRevision, IdempotencyKey: commandID, RequestedBy: "second-operator", Payload: mustJSON(ServiceCommandPayload{ServiceInstanceID: service.State.Service.InstanceID, ExpectedRevision: run.ControlRevision, RunAttemptID: run.AttemptID, Body: mustJSON(RecoveryCommandBody{ConfirmedProcessStopped: true, ConfirmationBasis: "another stop confirmation"})}), Status: "in_flight", CreatedAt: now()}
	commands := CommandService{Store: service.domainStore, Project: service.Config.identity}
	if _, created, err := commands.Submit(context.Background(), command); err != nil || !created {
		t.Fatalf("publish in-flight stop command: created=%v err=%v", created, err)
	}
	require(t, service.processPendingCommands(context.Background()))
	stored, err := service.domainStore.Command(context.Background(), service.Config.identity, commandID)
	require(t, err)
	if stored.Status != "indeterminate" || len(service.State.Control.Blockers) != 1 || run.ManualStop.ConfirmedBy != "first-operator" {
		t.Fatalf("second command borrowed a prior stop confirmation: command=%+v blockers=%+v stop=%+v", stored, service.State.Control.Blockers, run.ManualStop)
	}
}

func TestInFlightResolveCannotInferSuccessFromMissingBlocker(t *testing.T) {
	f := newFixture(t)
	service := f.service(t)
	defer service.Close()
	commandID, targetID := newID(), newID()
	command := CommandRecord{CommandID: commandID, Kind: "resolve_command", TargetID: targetID, ExpectedVersion: service.State.Control.Revision, IdempotencyKey: commandID, RequestedBy: "operator", Payload: mustJSON(ServiceCommandPayload{ServiceInstanceID: service.State.Service.InstanceID, ExpectedRevision: service.State.Control.Revision, Body: mustJSON(RecoveryCommandBody{ConfirmationBasis: "blocker was removed elsewhere"})}), Status: "in_flight", CreatedAt: now()}
	commands := CommandService{Store: service.domainStore, Project: service.Config.identity}
	if _, created, err := commands.Submit(context.Background(), command); err != nil || !created {
		t.Fatalf("publish in-flight resolve command: created=%v err=%v", created, err)
	}
	require(t, service.processPendingCommands(context.Background()))
	stored, err := service.domainStore.Command(context.Background(), service.Config.identity, commandID)
	require(t, err)
	if stored.Status != "indeterminate" || len(service.State.Control.Blockers) != 0 {
		t.Fatalf("missing blocker was mistaken for this command's receipt: command=%+v blockers=%+v", stored, service.State.Control.Blockers)
	}
}

func TestRetryServiceCommandRequiresOperatorStopConfirmation(t *testing.T) {
	f, service, run := failedStartup(t)
	defer service.Close()
	service.State.Service.Status, service.State.Service.Ready = "running", true
	service.State.Control.DispatchMode = "manual"
	require(t, service.save())
	startsBefore := readFake(f.db).Starts[run.TaskID]
	commandID := newID()
	command := CommandRecord{CommandID: commandID, Kind: "retry_run", TargetID: run.RunID, ExpectedVersion: run.ControlRevision, RequestedBy: "operator", Payload: mustJSON(ServiceCommandPayload{ServiceInstanceID: service.State.Service.InstanceID, ExpectedRevision: run.ControlRevision, RunAttemptID: run.AttemptID, Body: mustJSON(RecoveryCommandBody{})})}
	status, result := service.applyServiceCommand(context.Background(), command)
	if status != "rejected" || !strings.Contains(string(result), "confirmation_required") || service.worker != nil || readFake(f.db).Starts[run.TaskID] != startsBefore {
		t.Fatalf("retry without operator confirmation started an execution: status=%s result=%s starts=%d/%d", status, result, readFake(f.db).Starts[run.TaskID], startsBefore)
	}
}

func TestOversizedRecoveryConfirmationIsRejectedIntact(t *testing.T) {
	_, service, run := failedStartup(t)
	defer service.Close()
	run.ExecutionStatus, run.FinishTaskStatus = "unknown", "blocked"
	require(t, service.save())
	basis := strings.Repeat("界", 2001)
	if err := service.markStoppedCore(run.RunID, "operator", basis); err == nil || !strings.Contains(err.Error(), "2000 character") {
		t.Fatalf("oversized confirmation was accepted or silently clipped: %v", err)
	}
	if run.ManualStop != nil || run.ExecutionStatus != "unknown" || len([]rune(basis)) != 2001 {
		t.Fatalf("oversized confirmation mutated the run or input: run=%+v", run)
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

func TestInFlightAcceptedResultRechecksProviderHeadBeforeReceipt(t *testing.T) {
	f, service, run := failedStartup(t)
	defer service.Close()
	ctx := context.Background()
	run.ExecutionStatus, run.FinishTaskStatus = "unknown", "blocked"
	require(t, service.save())
	require(t, service.markStoppedCore(run.RunID, "operator", "the original execution stopped"))
	headBytes, err := gitRunner(f.c, *run.WorktreePath).Output(ctx, f.c.Git, "rev-parse", "HEAD")
	require(t, err)
	head := strings.TrimSpace(string(headBytes))
	f.mu.Lock()
	f.prHeadRevision = head
	f.mu.Unlock()
	resultBytes := mustJSON(Result{TaskID: run.TaskID, RunID: run.RunID, Outcome: "pr_submitted", PRURL: ptr(f.server.URL + "/owner/repo/pulls/1"), WorktreePath: run.WorktreePath, BaseCommit: run.BaseCommit, Summary: ptr("recovered result")})
	sum := sha256.Sum256(resultBytes)
	digest := hex.EncodeToString(sum[:])
	commandID, priorRevision, priorAttempt := newID(), run.ControlRevision, run.AttemptID
	require(t, service.acceptResultBytes(ctx, run.RunID, commandID, "operator", "the original execution stopped", resultBytes, digest))
	if run.FinishTaskStatus != "waiting_pr_merge" {
		t.Fatalf("fixture result was not accepted: %+v", run)
	}
	command := CommandRecord{CommandID: commandID, Kind: "accept_result", TargetID: run.RunID, ExpectedVersion: priorRevision, IdempotencyKey: commandID, RequestedBy: "operator", Payload: mustJSON(ServiceCommandPayload{ServiceInstanceID: service.State.Service.InstanceID, ExpectedRevision: priorRevision, RunAttemptID: priorAttempt, Body: mustJSON(RecoveryCommandBody{ConfirmedProcessStopped: true, ConfirmationBasis: "the original execution stopped", ResultBase64: base64.StdEncoding.EncodeToString(resultBytes), ResultSHA256: digest})}), Status: "in_flight", CreatedAt: now()}
	commands := CommandService{Store: service.domainStore, Project: service.Config.identity}
	if _, created, err := commands.Submit(ctx, command); err != nil || !created {
		t.Fatalf("publish in-flight result command: created=%v err=%v", created, err)
	}
	f.mu.Lock()
	f.prHeadRevision = strings.Repeat("f", 40)
	f.mu.Unlock()
	require(t, service.processPendingCommands(ctx))
	stored, err := service.domainStore.Command(ctx, service.Config.identity, commandID)
	require(t, err)
	if stored.Status != "indeterminate" || len(service.State.Control.Blockers) != 1 || run.FinishTaskStatus != "waiting_pr_merge" {
		t.Fatalf("changed PR head was treated as a verified result receipt: command=%+v blockers=%+v run=%+v", stored, service.State.Control.Blockers, run)
	}
	if _, err = service.resolveControlBlocker(ctx, commandID, "operator", "I checked the provider head"); err == nil || !strings.Contains(err.Error(), "head") {
		t.Fatalf("resolver accepted the changed PR head: %v", err)
	}
	if len(service.State.Control.Blockers) != 1 {
		t.Fatal("failed PR verification cleared the recovery blocker")
	}
	f.pr("1", "merged")
	require(t, service.pollPRs(ctx))
	if run.FinishTaskStatus != "blocked" || readFake(f.db).Tasks[0].Status == "closed" {
		t.Fatalf("changed recovery PR head completed the task: run=%+v task=%+v", run, readFake(f.db).Tasks[0])
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

func TestResolveControlCommandRejectsConflictingRequestAndRunIdentity(t *testing.T) {
	_, service, run := failedStartup(t)
	defer service.Close()
	run.ExecutionStatus, run.FinishTaskStatus = "unknown", "blocked"
	require(t, service.save())
	req := controlRequestFor(service, "start_task", map[string]string{"task_id": "different-task"})
	publishForService(t, service, req)
	digest := rawRequestDigest(controlRequestPath(service.Config.RunDir, req.RequestID))
	receipt := service.baseReceipt(req, digest, "indeterminate", "indeterminate", "start outcome unknown")
	receipt.CompletedAt = ptr(now())
	require(t, service.writeReceipt(receipt))
	service.State.Control.Blockers = append(service.State.Control.Blockers, ControlBlocker{RequestID: req.RequestID, Action: "start_task", TaskID: run.TaskID, RunID: run.RunID, AttemptID: run.AttemptID, Reason: "unknown start"})
	require(t, service.save())
	if _, err := service.resolveControlBlocker(context.Background(), req.RequestID, "operator", "the original execution stopped"); err == nil || !strings.Contains(err.Error(), "identity conflict") {
		t.Fatalf("mismatched request cleared a run blocker: %v", err)
	}
	if run.ExecutionStatus != "unknown" || run.ManualStop != nil || len(service.State.Control.Blockers) != 1 {
		t.Fatalf("mismatched request mutated the run or blocker: run=%+v blockers=%+v", run, service.State.Control.Blockers)
	}
}

func TestResolveControlCommandFinishesAfterSavedManualStop(t *testing.T) {
	_, service, run := failedStartup(t)
	defer service.Close()
	run.ExecutionStatus, run.FinishTaskStatus = "unknown", "blocked"
	require(t, service.save())
	req := controlRequestFor(service, "start_task", map[string]string{"task_id": run.TaskID})
	publishForService(t, service, req)
	digest := rawRequestDigest(controlRequestPath(service.Config.RunDir, req.RequestID))
	receipt := service.baseReceipt(req, digest, "indeterminate", "indeterminate", "start outcome unknown")
	receipt.CompletedAt = ptr(now())
	require(t, service.writeReceipt(receipt))
	service.State.Control.Blockers = append(service.State.Control.Blockers, ControlBlocker{RequestID: req.RequestID, Action: "start_task", TaskID: run.TaskID, RunID: run.RunID, AttemptID: run.AttemptID, Reason: "unknown start"})
	require(t, service.save())
	require(t, service.markStoppedCore(run.RunID, "first-operator", "the original process is stopped"))
	if run.ExecutionStatus != "failed" || !manuallyStopped(run) || len(service.State.Control.Blockers) != 1 {
		t.Fatalf("fixture did not persist a partial recovery boundary: run=%+v blockers=%+v", run, service.State.Control.Blockers)
	}
	result, err := service.resolveControlBlocker(context.Background(), req.RequestID, "second-operator", "I verified the saved stop confirmation for this attempt")
	require(t, err)
	if len(service.State.Control.Blockers) != 0 || run.ExecutionStatus != "failed" || run.ManualStop.ConfirmedBy != "first-operator" || !strings.Contains(result["resolution"].(string), "reconciled") {
		t.Fatalf("durable stop could not finish resolving the original blocker: run=%+v blockers=%+v result=%+v", run, service.State.Control.Blockers, result)
	}
}

func TestResolveServiceCommandRejectsConflictingBlockerIdentity(t *testing.T) {
	_, service, run := failedStartup(t)
	defer service.Close()
	run.ExecutionStatus, run.FinishTaskStatus = "unknown", "blocked"
	require(t, service.save())
	commandID := newID()
	command := CommandRecord{CommandID: commandID, Kind: "mark_stopped", TargetID: run.RunID, ExpectedVersion: run.ControlRevision, IdempotencyKey: commandID, RequestedBy: "operator", Payload: mustJSON(ServiceCommandPayload{ServiceInstanceID: service.State.Service.InstanceID, ExpectedRevision: run.ControlRevision, RunAttemptID: run.AttemptID, Body: mustJSON(RecoveryCommandBody{ConfirmedProcessStopped: true, ConfirmationBasis: "original execution stopped"})}), Status: "indeterminate", CreatedAt: now()}
	commands := CommandService{Store: service.domainStore, Project: service.Config.identity}
	if _, created, err := commands.Submit(context.Background(), command); err != nil || !created {
		t.Fatalf("publish indeterminate command: created=%v err=%v", created, err)
	}
	service.State.Control.Blockers = append(service.State.Control.Blockers, ControlBlocker{RequestID: commandID, Action: "retry_run", TaskID: run.TaskID, RunID: run.RunID, AttemptID: run.AttemptID, Reason: "wrong action"})
	require(t, service.save())
	if _, err := service.resolveControlBlocker(context.Background(), commandID, "operator", "original execution stopped"); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("mismatched service blocker was resolved: %v", err)
	}
	if run.ExecutionStatus != "unknown" || run.ManualStop != nil || len(service.State.Control.Blockers) != 1 {
		t.Fatalf("mismatched blocker mutated the run or blocker: run=%+v blockers=%+v", run, service.State.Control.Blockers)
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

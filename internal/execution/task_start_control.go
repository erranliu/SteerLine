package execution

import (
	"context"
	"errors"

	productapi "github.com/erranliu/SteerLine/internal/product/api"
)

type taskStartCheckResult struct {
	request  productapi.ControlRequest
	digest   string
	prepared preparedTaskClaim
	err      error
}

// Only external preflight runs in the background. Task claims and receipts
// remain owned by the service actor, independently of other starting tasks.
func (s *Service) processTaskStartControl(ctx context.Context, req productapi.ControlRequest, payload productapi.ControlPayload, digest string) error {
	if s.State.Control.DispatchMode != "manual" {
		return s.rejectControl(req, digest, "wrong_mode", errors.New("start_task requires manual dispatch mode"))
	}
	if s.State.Control.DispatchPaused {
		return s.rejectControl(req, digest, "dispatch_paused", errors.New("dispatch is paused"))
	}
	for _, pending := range s.startChecks {
		p, _ := DecodeControlPayload(pending)
		if p.TaskID == payload.TaskID {
			return s.rejectControl(req, digest, "already_managed", errAlreadyManaged)
		}
	}
	if err := s.validateReadyTarget(ctx, payload.TaskID); err != nil {
		code := "task_not_ready"
		if errors.Is(err, errAlreadyManaged) {
			code = "already_managed"
		}
		return s.rejectControl(req, digest, code, err)
	}
	profileID := payload.ExecutionProfileID
	if profileID == "" {
		if s.runtime.Profiles.HasMultipleProfiles() {
			return s.rejectControl(req, digest, "invalid_request", errors.New("start_task requires an execution profile selection"))
		}
		profileID = s.runtime.Profiles.DefaultProfileID()
	}
	config, profile, err := s.runtime.Profiles.ResolveExecutionProfile(profileID, payload.ExecutionProfileDigest)
	if err != nil {
		return s.rejectControl(req, digest, "stale_profile", err)
	}
	graph, err := s.commandStore.TaskGraphSnapshot(ctx, s.Config.identity)
	if err != nil {
		return err
	}
	task, found := graph.ByID[payload.TaskID]
	if !found {
		return s.rejectControl(req, digest, "task_not_ready", errTaskNotReady)
	}
	expectedTaskVersion := payload.ExpectedTaskVersion
	if expectedTaskVersion != 0 && expectedTaskVersion != task.Version {
		return s.rejectControl(req, digest, "stale_version", nativeErr("stale_version", "start_task task version changed before admission"))
	}
	if expectedTaskVersion == 0 {
		expectedTaskVersion = task.Version
	}
	req.Payload = append([]byte(nil), req.Payload...)
	commands := s.commandService(s.Config.identity)
	options := TaskClaimOptions{TaskID: payload.TaskID, RequestID: req.RequestID, WorkflowRunID: req.RequestID, ExpectedTaskVersion: expectedTaskVersion, ExpectedAuthorityVersion: req.ExpectedControlRevision, RequestedBy: req.RequestedBy, Explicit: true, ExecutionProfile: &profile, ServiceInstanceID: req.InstanceID, controlRequestDigest: digest}
	check := func(checkCtx context.Context) taskStartCheckResult {
		prepared, err := commands.prepareNativeTaskClaim(checkCtx, config, options)
		return taskStartCheckResult{request: req, digest: digest, prepared: prepared, err: err}
	}
	if s.startChecks == nil {
		s.startChecks = make(map[string]productapi.ControlRequest)
	}
	s.startChecks[req.RequestID] = req
	s.controlSeen[req.RequestID] = digest
	if s.signalQueue == nil {
		return s.applyTaskStartCheck(check(ctx))
	}
	queue := s.signalQueue
	s.forwarders.Add(1)
	go func() {
		defer s.forwarders.Done()
		checkCtx, cancel := context.WithDeadline(ctx, req.ExpiresAt)
		defer cancel()
		result := check(checkCtx)
		select {
		case queue <- Signal{startCheck: &result}:
		case <-ctx.Done():
		}
	}()
	return nil
}

func (s *Service) applyTaskStartCheck(result taskStartCheckResult) error {
	req := result.request
	pending, found := s.startChecks[req.RequestID]
	if !found || RequestDigest(pending) != RequestDigest(req) {
		return errors.New("task start preflight differs from its original request")
	}
	delete(s.startChecks, req.RequestID)
	// Until the receipt is published, an I/O failure must leave the immutable
	// inbox eligible for retry/recovery of the same committed reservation.
	delete(s.controlSeen, req.RequestID)
	if s.host.ControlRequestDigest(req.RequestID) != result.digest {
		return s.rejectControl(req, result.digest, "idempotency_conflict", errors.New("task start request changed during preflight"))
	}
	if code, err := ValidateControlRequest(req, s.State.Service.InstanceID, s.Config.identity, s.State.Control.Revision, now()); err != nil {
		return s.rejectControl(req, result.digest, code, err)
	}
	if !s.State.Service.Ready || s.State.Service.Status != "running" && s.State.Service.Status != "error" || s.runContext != nil && s.runContext.Err() != nil {
		return s.rejectControl(req, result.digest, "stale_instance", errors.New("service stopped before task start admission"))
	}
	if result.err != nil {
		return s.rejectControl(req, result.digest, controlFailureCode(result.err), result.err)
	}
	ctx := context.Background()
	if s.runContext != nil {
		ctx = s.runContext
	}
	commands := s.commandService(s.Config.identity)
	receipt, err := commands.commitPreparedTaskClaim(ctx, result.prepared)
	if err != nil {
		return s.rejectControl(req, result.digest, controlFailureCode(err), err)
	}
	// The immutable command and its request digest were saved in the claim
	// transaction. A crash before publishing this receipt recovers that claim.
	s.injectControlFault("after_start_reservation")
	if err := s.refreshNativeSnapshot(ctx); err != nil {
		return err
	}
	applying := s.baseReceipt(req, result.digest, "applying", "claim_reserved", "native task reserved; original worker start awaits its persisted identity")
	applying.AcceptedAt = ptr(now())
	if result.prepared.Commit != nil {
		applying.AcceptedAt = ptr(result.prepared.Commit.Command.CreatedAt)
	}
	applying.RunID, applying.AttemptID = receipt.WorkflowRunID, receipt.AttemptID
	applying.ControlRevision = s.State.Control.Revision
	if run := s.find(receipt.WorkflowRunID); run != nil {
		applying.RunRevision = run.ControlRevision
	}
	if err := s.writeReceipt(applying); err != nil {
		return err
	}
	s.controlSeen[req.RequestID] = result.digest
	return s.publishCheckpoint()
}

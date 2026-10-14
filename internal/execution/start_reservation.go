package execution

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	m "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/reconstruction"
	api "github.com/erranliu/SteerLine/internal/product/api"
	p "github.com/erranliu/SteerLine/internal/storage/ports"
)

// Admission persists identity before any external readiness or Git probe.
func (s *Service) reserveControlStart(ctx context.Context, req api.ControlRequest, payload api.ControlPayload, candidate bool) error {
	profileID := payload.ExecutionProfileID
	if profileID == "" {
		profileID = s.runtime.Profiles.DefaultProfileID()
	}
	config, profile, err := s.runtime.Profiles.ResolveExecutionProfile(profileID, payload.ExecutionProfileDigest)
	digest := s.host.ControlRequestDigest(req.RequestID)
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
	version := payload.ExpectedTaskVersion
	if version == 0 {
		version = task.Version
	}
	// A second confirmation can arrive before the console observes its own
	// first claim. Only that exact, same-instance claim permits the one version
	// advance; all other stale task edits remain rejected.
	if candidate && task.Version == version+1 {
		for _, w := range s.domain.WorkflowRuns {
			if w.TaskID != task.TaskID {
				continue
			}
			old, exists, lookupErr := s.commandStore.LookupCommand(ctx, s.Config.identity, w.WorkflowRunID)
			if lookupErr != nil {
				return lookupErr
			}
			if exists && old.Kind == "claim_task" && old.ExpectedVersion == version && old.Status == "accepted" {
				for _, e := range s.domain.Executions {
					if e.ExecutionID == w.SelectedResult && e.OwnerServiceInstanceID == req.InstanceID {
						version = task.Version
					}
				}
			}
		}
	}
	options := TaskClaimOptions{TaskID: payload.TaskID, RequestID: req.RequestID, WorkflowRunID: req.RequestID,
		ExpectedTaskVersion: version, ExpectedGraphRevision: payload.ExpectedGraphRevision,
		ExpectedAuthorityVersion: req.ExpectedControlRevision, RequestedBy: req.RequestedBy, Explicit: true,
		ExecutionProfile: &profile, ServiceInstanceID: req.InstanceID, controlRequestDigest: digest,
		reserveOnly: true, startCandidate: candidate}
	if candidate && options.ExpectedGraphRevision == 0 {
		options.ExpectedGraphRevision = graph.Authority.GraphRevision
	}
	receipt, err := s.commandService(s.Config.identity).ClaimNativeTask(ctx, config, options)
	if err != nil {
		return s.rejectControl(req, digest, controlFailureCode(err), err)
	}
	s.injectControlFault("after_start_reservation")
	if err = s.refreshNativeSnapshot(ctx); err != nil {
		return err
	}
	result := s.baseReceipt(req, digest, "applying", "candidate_reserved", "candidate retained; preparation pending")
	result.AcceptedAt, result.RunID, result.AttemptID = ptr(now()), receipt.WorkflowRunID, receipt.AttemptID
	result.ControlRevision = s.State.Control.Revision
	if err = s.writeReceipt(result); err != nil {
		return err
	}
	if s.controlSeen == nil {
		s.controlSeen = map[string]string{}
	}
	s.controlSeen[req.RequestID] = digest
	return s.prepareReservedStart(ctx, s.find(req.RequestID))
}

func (s *Service) prepareReservedStart(ctx context.Context, run *reconstruction.RunView) error {
	w, e := s.workflowRecords(run)
	if w == nil || e == nil || !w.StartPending || s.candidateBusinessClosed(run) || run.TaskStatus == "closed" || run.TaskStatus == "discarded" {
		return nil
	}
	if _, busy := s.startChecks[run.RunID]; busy {
		return nil
	}
	// Establish the common base once. Later starts can prepare independently
	// as soon as that prerequisite is frozen, without waiting for a worker.
	base, baseErr := m.CandidateCommonBase(s.domain.WorkflowRuns, s.domain.Executions, run.TaskID)
	if baseErr != nil && !isNativeErrorCode(baseErr, "task_not_ready") {
		return baseErr
	}
	if base == "" {
		for _, sibling := range s.domain.WorkflowRuns {
			if sibling.TaskID == run.TaskID && sibling.StartPending && sibling.Status != "abandoned" && sibling.CandidateNumber < w.CandidateNumber {
				return nil
			}
		}
	}
	req, err := s.host.ReadControlRequest(run.RunID)
	if err != nil {
		return err
	}
	config, profile, err := s.runtime.Profiles.ResolveExecutionProfile(w.ExecutionProfile.ID, w.ExecutionProfile.Digest)
	if err != nil {
		return s.failReservedStart(run, err.Error())
	}
	graph, err := s.commandStore.TaskGraphSnapshot(ctx, s.Config.identity)
	if err != nil {
		return err
	}
	task := graph.ByID[run.TaskID]
	reservation := p.TaskCandidateSnapshot{WorkflowRuns: []m.WorkflowRunRecord{*w}, Executions: []m.ExecutionRecord{*e}}
	options := TaskClaimOptions{TaskID: run.TaskID, RequestID: run.RunID, WorkflowRunID: run.RunID,
		ExpectedTaskVersion: task.Version, ExpectedAuthorityVersion: graph.Authority.AuthorityVersion,
		ExpectedGraphRevision: graph.Authority.GraphRevision, RequestedBy: req.RequestedBy, Explicit: true,
		ExecutionProfile: &profile, ServiceInstanceID: s.State.Service.InstanceID, startCandidate: true, reservation: &reservation}
	commands := s.commandService(s.Config.identity)
	digest := s.host.ControlRequestDigest(req.RequestID)
	check := func() taskStartCheckResult {
		checkCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		prepared, err := commands.prepareNativeTaskClaim(checkCtx, config, options)
		return taskStartCheckResult{request: req, digest: digest, prepared: prepared, err: err}
	}
	if s.startChecks == nil {
		s.startChecks = map[string]api.ControlRequest{}
	}
	s.startChecks[run.RunID] = req
	if s.signalQueue == nil {
		return s.applyTaskStartCheck(check())
	}
	queue := s.signalQueue
	s.forwarders.Add(1)
	go func() {
		defer s.forwarders.Done()
		result := check()
		select {
		case queue <- Signal{startCheck: &result}:
		case <-ctx.Done():
		}
	}()
	return nil
}

func (c CommandService) finishReservedStart(ctx context.Context, commit p.ClaimCommit) error {
	if err := verifyClaimConfigFiles(c.runtime.ConfigVerifier); err != nil {
		return err
	}
	return c.Store.TransactCandidate(ctx, func(tx p.CandidateTx) error {
		w, err := tx.Workflow(ctx, c.Project, commit.Workflow.WorkflowRunID, commit.Before.TaskID)
		if err != nil {
			return err
		}
		if !w.StartPending || w.Status != "queued" {
			return m.ErrTurnStale
		}
		graph, err := tx.TaskGraphSnapshot(ctx, c.Project)
		if err != nil {
			return err
		}
		if graph.Authority.AuthorityVersion != commit.AuthorityVersion || graph.Authority.GraphRevision != commit.GraphRevision || !bytes.Equal(mustJSON(graph.ByID[commit.Before.TaskID]), mustJSON(commit.Before)) {
			return nativeErr("stale_version", "task or authority changed during candidate preparation")
		}
		snapshot, err := tx.ReadTaskCandidateSnapshot(ctx, c.Project, commit.Before.TaskID)
		if err != nil {
			return err
		}
		if m.CandidatePRBindings(snapshot.WorkflowRuns, commit.Before.TaskID) != commit.ExpectedPRBindings {
			return nativeErr("stale_version", "known PR bindings changed during preparation")
		}
		base, err := m.CandidateCommonBase(snapshot.WorkflowRuns, snapshot.Executions, commit.Before.TaskID)
		if err != nil && !isNativeErrorCode(err, "task_not_ready") {
			return err
		}
		if base != "" && base != commit.Execution.Workspace.Base {
			return nativeErr("contract_mismatch", "candidate common base changed during preparation")
		}
		old, found, err := tx.LookupCommand(ctx, c.Project, w.WorkflowRunID)
		if err != nil {
			return err
		}
		if !found || old.Status != "accepted" {
			return errors.New("candidate has no original reservation command")
		}
		if err = tx.PutWorkflowRun(commit.Workflow); err != nil {
			return err
		}
		if err = tx.PutExecution(commit.Execution); err != nil {
			return err
		}
		if err = tx.PutStepAttempt(commit.Attempt); err != nil {
			return err
		}
		var receipt m.TaskClaimReceipt
		if err = json.Unmarshal(old.Result, &receipt); err != nil {
			return err
		}
		receipt.BaseCommit = commit.Execution.Workspace.Base
		old.Result = mustJSON(receipt)
		return tx.UpdateCommand(old)
	})
}

func (s *Service) failReservedStart(run *reconstruction.RunView, reason string) error {
	w, e := s.workflowRecords(run)
	if w == nil || e == nil {
		return errors.New("failed start has no persistent candidate")
	}
	if !m.StopCandidate(w, "startup_failed", "system", now()) {
		return nil
	}
	w.StepID, w.StageDiagnostic = "start.failed", clip(reason, 2000)
	e.Status, e.Error, e.UpdatedAt, e.TurnEndedAt = "failed", ptr(clip(reason, 2000)), now(), ptr(now())
	if a := s.workflowAttempt(run, run.AttemptID); a != nil {
		a.Status, a.UpdatedAt = "failed", now()
	}
	s.refreshRun(run)
	return s.commitWorker(run)
}

package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func (s *Service) startRetry(ctx context.Context, id string) error {
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(id) {
		return errors.New("invalid retry run ID")
	}
	if s.worker != nil {
		return errors.New("execution slot occupied")
	}
	if s.Config.hostName() == "orca" {
		return errors.New("retry is not supported for the orca host yet; use manual recovery")
	}
	var selected *Run
	for _, r := range s.State.Runs {
		if r.ExecutionStatus == "unknown" || r.ExecutionStatus == "running" || r.ExecutionStatus == "pending" {
			return errors.New("resolve uncertain executions before retry")
		}
		if s.inspectRunProcess(r) == "matching process still exists" {
			return errors.New("recorded worker is still alive")
		}
		if r.RunID == id {
			selected = r
		}
	}
	if selected == nil {
		return fmt.Errorf("run %s not found", id)
	}
	r := selected
	if r.FinishTaskStatus != "blocked" || (r.ExecutionStatus != "failed" && r.ExecutionStatus != "succeeded") || r.PR.URL != nil {
		return errors.New("retry requires a blocked run without a bound PR")
	}
	if err := verifyRunWorkspace(ctx, s.Config, r); err != nil {
		return err
	}
	if err := verifyWorkspace(ctx, s.Config, r.WorkspaceIntent, true); err != nil {
		return errors.New("retry is limited to startup failures; worktree HEAD must still equal base_commit")
	}
	out, err := gitRunner(s.Config, *r.WorktreePath).Output(ctx, s.Config.Git, "status", "--porcelain")
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(out)) != "" {
		return errors.New("worktree has changes; preserve and repair the existing work manually")
	}
	source, err := s.beads.Context(ctx)
	if err != nil {
		return err
	}
	if r.BeadsContext == nil || !source.Same(*r.BeadsContext) {
		return errors.New("Beads identity changed since dispatch")
	}
	task, err := s.beads.Show(ctx, r.TaskID)
	if err != nil {
		return err
	}
	if task.Status != "in_progress" || task.Assignee != r.Claimant {
		return errors.New("retry task claim is no longer owned by this service")
	}
	// Keep all old files at their original paths. The snapshot links to the previous
	// attempt; a crash after the pending save is recovered as unknown, never rerun.
	attemptID := newID()
	dir := filepath.Join(s.Config.RunDir, "runs", r.RunID, "attempts", attemptID)
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	previous, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err = AtomicWrite(filepath.Join(dir, "previous-run.json"), previous); err != nil {
		return err
	}
	r.LogPath = ptr(filepath.Join(dir, "events.jsonl"))
	r.ResultPath = ptr(filepath.Join(dir, "result.json"))
	r.AttemptID = attemptID
	r.ExecutionStatus = "pending"
	r.FinishTaskStatus = "pending"
	r.BeadsStatus = task.Status
	r.SessionID = nil
	r.Process = nil
	r.StartedAt = nil
	r.EndedAt = nil
	r.LastEvent = nil
	r.Summary = nil
	r.Error = nil
	s.State.CurrentTask = ptr(r.TaskID)
	s.State.SlotOccupied = true
	if err = s.save(); err != nil {
		return err
	}
	s.note(r, "steer", "retry", "started", "retry attempt started", r.TaskID, nil)
	if err = ctx.Err(); err != nil {
		return s.preparationFailed(r, err.Error())
	}
	backend := backendFor(s.Config, source)
	kind, protocol := backend.Identity()
	if kind != BackendLocal || protocol != ProtocolLocalProcess {
		return unsupported("retry for this backend/protocol")
	}
	if r.WorkspaceIntent == nil || r.WorktreePath == nil || r.BaseCommit == nil {
		return errors.New("retry has no verified fixed workspace binding")
	}
	intent := *r.WorkspaceIntent
	target, agent := targetFor(s.Config), agentFor(s.Config)
	opID := attemptOperationID(r.RunID, r.AttemptID)
	spec := AttemptSpec{ProjectID: s.Config.identity, TaskID: task.ID, RunID: r.RunID, AttemptID: r.AttemptID, OperationID: opID, ConfigDigest: s.Config.configDigest, Repository: s.Config.Repository, BaseCommit: intent.BaseCommit, Workspace: intent, Agent: agent, Protocol: protocol, Target: target, Task: task, Run: cloneRun(*r), Beads: source}
	records := executionRecords{root: s.Config.RunDir}
	op := Operation{Version: 1, ID: opID, RunID: r.RunID, AttemptID: r.AttemptID, Backend: kind, Protocol: protocol, Target: target, Action: "start", PayloadDigest: digestValue(spec), Status: OperationPending, StartedAt: now(), UpdatedAt: now()}
	capabilities, probeErr := backend.Probe(ctx, target, agent)
	if probeErr != nil {
		return probeErr
	}
	if err = records.saveCapabilityReport(capabilities); err != nil {
		return err
	}
	if old, loadErr := records.loadOperation(r.RunID, r.AttemptID, opID); loadErr == nil {
		if old.PayloadDigest != op.PayloadDigest {
			return errors.New("retry start operation conflicts with its fixed input")
		}
		return fmt.Errorf("retry operation is already %s; refusing duplicate start", old.Status)
	} else if !os.IsNotExist(loadErr) {
		return loadErr
	}
	if err = records.saveOperation(op); err != nil {
		return err
	}
	op.Status, op.UpdatedAt = OperationInFlight, now()
	if err = records.saveOperation(op); err != nil {
		return err
	}
	started, err := backend.Start(ctx, spec)
	if err != nil {
		op.ResidualResources = append([]string(nil), started.ResidualResources...)
		var backendErr *BackendError
		if errors.As(err, &backendErr) && backendErr.Code == BackendOutcomeUnknown {
			op.Status, op.Diagnostic = OperationUnknown, err.Error()
			r.ExecutionStatus = "unknown"
			s.State.SlotOccupied = true
			s.State.CurrentTask = ptr(r.TaskID)
			s.blocked(r, "Retry start outcome unknown; refusing replacement attempt: "+err.Error())
			if saveErr := records.saveOperation(op); saveErr != nil {
				return errors.Join(err, saveErr)
			}
			return errors.Join(err, s.save())
		}
		op.Status, op.Diagnostic, op.UpdatedAt = OperationRejected, err.Error(), now()
		if saveErr := records.saveOperation(op); saveErr != nil {
			return errors.Join(err, saveErr)
		}
		return s.preparationFailed(r, "Retry startup failed: "+err.Error())
	}
	if started.Disposition != "accepted" || started.Worker == nil {
		op.Status, op.Diagnostic, op.UpdatedAt = OperationUnknown, "retry backend returned no verifiable accepted worker", now()
		op.ResidualResources = append([]string(nil), started.ResidualResources...)
		if saveErr := records.saveOperation(op); saveErr != nil {
			return saveErr
		}
		r.ExecutionStatus = "unknown"
		if started.Worker != nil {
			identity := started.Worker.proc.Identity()
			r.Process = &identity
			s.worker, s.active = started.Worker, r
			r.StartedAt = ptr(now())
		}
		s.blocked(r, "Retry start receipt is incomplete; execution slot remains reserved")
		s.State.SlotOccupied, s.State.CurrentTask = true, ptr(r.TaskID)
		return s.save()
	}
	ref := started.Ref
	if ref.RunID != r.RunID || ref.AttemptID != r.AttemptID || ref.Backend != kind || ref.Protocol != protocol || ref.Target != target {
		identity := started.Worker.proc.Identity()
		r.Process = &identity
		r.ExecutionStatus = "unknown"
		s.worker, s.active = started.Worker, r
		op.Status, op.Diagnostic, op.UpdatedAt = OperationUnknown, "retry backend returned a mismatched execution identity", now()
		opSaveErr := records.saveOperation(op)
		s.blocked(r, "Retry execution identity mismatch; retaining unknown execution")
		s.State.SlotOccupied, s.State.CurrentTask = true, ptr(r.TaskID)
		return errors.Join(errors.New(op.Diagnostic), opSaveErr, s.save())
	}
	r.WorktreePath = started.Run.WorktreePath
	r.WorktreeBranch = started.Run.WorktreeBranch
	r.BaseCommit = started.Run.BaseCommit
	r.Process = &ref.External
	s.worker = started.Worker
	s.active = r
	r.ExecutionStatus = "running"
	r.StartedAt = ptr(now())
	if err = records.saveBinding(ref); err != nil {
		op.Status, op.Diagnostic, op.UpdatedAt = OperationUnknown, "retry binding persistence failed: "+err.Error(), now()
		opSaveErr := records.saveOperation(op)
		r.ExecutionStatus = "unknown"
		s.blocked(r, "Retry started but binding persistence failed; retaining unknown execution")
		s.State.SlotOccupied, s.State.CurrentTask = true, ptr(r.TaskID)
		return errors.Join(err, opSaveErr, s.save())
	}
	op.Status, op.Ref, op.UpdatedAt = OperationAccepted, &ref, now()
	if err = records.saveOperation(op); err != nil {
		op.Status, op.Diagnostic, op.UpdatedAt = OperationUnknown, "retry operation receipt persistence failed: "+err.Error(), now()
		opSaveErr := records.saveOperation(op)
		r.ExecutionStatus = "unknown"
		s.blocked(r, "Retry started but operation receipt persistence failed; retaining unknown execution")
		s.State.SlotOccupied, s.State.CurrentTask = true, ptr(r.TaskID)
		return errors.Join(err, opSaveErr, s.save())
	}
	s.note(r, "steer", "worker_start", "started", "worker started", "", map[string]any{"pid": ref.External.PID, "retry": true})
	if err = s.save(); err != nil {
		return err
	}
	op.Status, op.UpdatedAt = OperationConfirmed, now()
	if err = records.saveOperation(op); err != nil {
		return err
	}
	return s.save()
}

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
		if inspectProcessWithConfig(s.Config, r.Process) == "matching process still exists" {
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
	if r.WorkflowVersion == MinimalWorkflowVersion {
		return errors.New("minimal-v1 cannot use the single-workflow retry path; reconcile its current step attempt")
	}
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
	workerConfig, err := workerForWorkspace(ctx, s.Config, r, source)
	if err != nil {
		return err
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
	r.DomainAttemptNumber++
	if r.DomainAttemptNumber < 1 {
		r.DomainAttemptNumber = 1
	}
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
	worker, err := StartWorker(workerConfig, task, r)
	if err != nil {
		return s.preparationFailed(r, "Retry startup failed: "+err.Error())
	}
	s.worker = worker
	s.active = r
	r.ExecutionStatus = "running"
	r.StartedAt = ptr(now())
	procID := worker.proc.Identity()
	r.Process = &procID
	s.note(r, "steer", "worker_start", "started", "worker started", "", map[string]any{"pid": procID.PID, "retry": true})
	return s.save()
}

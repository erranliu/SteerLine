package steerline

import (
	"fmt"
	"os"
)

func manuallyStopped(run *Run) bool {
	return run.ManualStop != nil && run.ManualStop.AttemptID == run.AttemptID && !run.ManualStop.ConfirmedAt.IsZero()
}

// reconcileRunExecutionRecord compares the two durable execution records with
// the runtime projection. It never queries or restarts an external worker.
func (s *Service) reconcileRunExecutionRecord(run *Run) error {
	if !stableExecutionID(run.RunID) || !stableExecutionID(run.AttemptID) {
		run.ExecutionStatus = "unknown"
		s.blocked(run, "Run has no current execution identity; preserve state without migration")
		s.State.SlotOccupied = true
		return nil
	}
	records := executionRecords{root: s.Config.RunDir}
	op, opErr := records.loadOperation(run.RunID, run.AttemptID, attemptOperationID(run.RunID, run.AttemptID))
	if opErr != nil && !os.IsNotExist(opErr) {
		return fmt.Errorf("invalid execution operation for run %s: %w", run.RunID, opErr)
	}
	ref, refErr := records.loadBinding(run.RunID, run.AttemptID)
	if refErr != nil && !os.IsNotExist(refErr) {
		return fmt.Errorf("invalid execution binding for run %s: %w", run.RunID, refErr)
	}
	var bound *ExecutionRef
	if refErr == nil {
		bound = &ref
	}
	if err := s.reconcileAttemptOperations(run, bound); err != nil {
		return err
	}
	if os.IsNotExist(opErr) {
		if refErr == nil {
			return fmt.Errorf("run %s has an execution binding without its start operation", run.RunID)
		}
		if !manuallyStopped(run) && (run.Process != nil || run.ExecutionStatus != "pending" || run.WorkspaceIntent != nil) {
			run.ExecutionStatus = "unknown"
			s.blocked(run, "Run lacks a current start operation; preserve state without migration")
			s.State.SlotOccupied = true
		}
		return nil
	}
	if op.Action != "start" || string(op.Backend) != run.Host {
		return fmt.Errorf("run %s start operation conflicts with its recorded backend", run.RunID)
	}
	if os.IsNotExist(refErr) {
		if op.Ref != nil || op.Status == OperationAccepted || op.Status == OperationConfirmed {
			return fmt.Errorf("run %s has an accepted start without its execution binding", run.RunID)
		}
		if !manuallyStopped(run) && (op.Status == OperationInFlight || op.Status == OperationUnknown) {
			run.ExecutionStatus = "unknown"
			s.blocked(run, "Start operation has no binding; outcome remains unknown and cannot be replayed")
			s.State.SlotOccupied = true
		}
		return nil
	}
	if op.Status == OperationPending || op.Status == OperationRejected {
		return fmt.Errorf("run %s has a binding for an unsent or rejected start", run.RunID)
	}
	if op.Backend != ref.Backend || op.Protocol != ref.Protocol || op.Target != ref.Target {
		return fmt.Errorf("run %s operation and binding disagree", run.RunID)
	}
	if op.Ref != nil && digestValue(*op.Ref) != digestValue(ref) {
		return fmt.Errorf("run %s operation receipt and binding disagree", run.RunID)
	}
	if run.Process != nil && *run.Process != ref.External {
		return fmt.Errorf("run %s runtime process identity conflicts with execution binding", run.RunID)
	}
	if run.WorktreePath != nil && canonical(*run.WorktreePath) != canonical(ref.WorkspacePath) || run.WorktreeBranch != nil && *run.WorktreeBranch != ref.WorkspaceBranch || run.BaseCommit != nil && *run.BaseCommit != ref.BaseCommit {
		return fmt.Errorf("run %s runtime workspace conflicts with execution binding", run.RunID)
	}
	projectionMissing := run.Process == nil || run.WorktreePath == nil || run.WorktreeBranch == nil || run.BaseCommit == nil || run.WorkspaceIntent == nil || canonical(run.WorkspaceIntent.Path) != canonical(ref.WorkspacePath) || run.WorkspaceIntent.Branch != ref.WorkspaceBranch || run.WorkspaceIntent.BaseCommit != ref.BaseCommit
	if projectionMissing {
		process := ref.External
		path, branch, base := ref.WorkspacePath, ref.WorkspaceBranch, ref.BaseCommit
		run.Process, run.WorktreePath, run.WorktreeBranch, run.BaseCommit = &process, &path, &branch, &base
		run.WorkspaceIntent = &WorkspaceIntent{Path: path, Branch: branch, BaseCommit: base}
		run.ExecutionStatus = "unknown"
		s.blocked(run, "Runtime projection rebuilt from the fixed execution binding; manual recovery required")
		s.State.SlotOccupied = true
	}
	if !manuallyStopped(run) && (op.Status == OperationInFlight || op.Status == OperationUnknown) {
		run.ExecutionStatus = "unknown"
		s.blocked(run, "Start operation remains outcome_unknown; no replacement worker will be started")
		s.State.SlotOccupied = true
	}
	return nil
}

// Reconcile every side effect for the current attempt, not just its launch.
// An older runtime projection cannot erase a sent reply or an uncertain stop.
func (s *Service) reconcileAttemptOperations(run *Run, ref *ExecutionRef) error {
	records := executionRecords{root: s.Config.RunDir}
	ops, err := records.attemptOperations(run.RunID, run.AttemptID)
	if err != nil {
		return err
	}
	for _, op := range ops {
		id := op.ID
		if op.Action == "start" {
			if id != attemptOperationID(run.RunID, run.AttemptID) {
				return fmt.Errorf("run %s has a conflicting start operation", run.RunID)
			}
			continue
		}
		if ref == nil || op.Ref == nil || digestValue(*op.Ref) != digestValue(*ref) {
			return fmt.Errorf("run %s operation %s lacks its matching fixed binding", run.RunID, id)
		}
		if !manuallyStopped(run) && (op.Status == OperationInFlight || op.Status == OperationUnknown || op.Action == "stop" && op.Status == OperationAccepted) {
			run.ExecutionStatus = "unknown"
			s.blocked(run, "Execution operation "+id+" remains unresolved; no replacement execution or replay is allowed")
			s.State.SlotOccupied = true
		}
	}
	return nil
}

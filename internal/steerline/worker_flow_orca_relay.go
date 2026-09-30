package steerline

import (
	"context"
	"errors"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
	"path/filepath"
	"strings"
)

// Orca's Windows named pipe is inaccessible to commands in Codex's
// workspace-write sandbox. The worker can write this one bounded request in
// its own checkout; the center forwards it only for the bound Dispatch.
type nativeOrcaDoneRequest = orcaworker.DoneRequest

func nativeOrcaRelayInstruction(runID, turnID string) string {
	return orcaworker.RelayInstruction(runID, turnID)
}

// Keep the short-lived capability file out of git add -A without changing the
// project's tracked .gitignore. Git's shared exclude file covers every linked
// worktree of this one repository.
func ensureNativeOrcaRelayIgnored(ctx context.Context, c Config, worktree string) error {
	if c.identity == "" || !filepath.IsAbs(worktree) {
		return errors.New("native Orca relay ignore has no fixed repository/worktree")
	}
	out, err := gitRunner(c, worktree).Output(ctx, c.Git, "rev-parse", "--path-format=absolute", "--git-path", "info/exclude")
	if err != nil {
		return err
	}
	if canonical(strings.TrimSpace(string(out))) != canonical(filepath.Join(c.identity, "info", "exclude")) {
		return errors.New("native Orca relay exclude is outside the fixed repository")
	}
	return orcaworker.EnsureRelayIgnored(c.identity)
}

func nativeOrcaRelayPath(worktree, runID, turnID string) (string, error) {
	return orcaworker.RelayPath(worktree, runID, turnID)
}

func removeNativeOrcaRelayRequest(worktree, runID, turnID string) {
	orcaworker.RemoveRelayRequest(worktree, runID, turnID)
}

func readNativeOrcaDoneRequest(path, worktree string, run *Run, turn *OrcaNativeTurn) (*nativeOrcaDoneRequest, error) {
	return orcaworker.ReadDoneRequest(path, worktree, run.RunID, turn.TurnID, turn.TaskID, turn.DispatchID)
}

func (s *Service) relayNativeOrcaDone(ctx context.Context, run *Run, execution *ExecutionRecord, turn *OrcaNativeTurn) error {
	path, err := nativeOrcaRelayPath(execution.Workspace.Worktree, run.RunID, turn.TurnID)
	if err != nil {
		return err
	}
	key := "orca-native-relay-done:" + execution.ExecutionID + ":" + turn.TurnID
	if s.nativeOrcaOperation(key) != nil {
		return &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca result relay was already attempted, but the original Run inbox has no worker_done; preserve the Dispatch for reconciliation"}
	}
	request, err := readNativeOrcaDoneRequest(path, execution.Workspace.Worktree, run, turn)
	if err != nil || request == nil {
		return err
	}
	if err := s.verifyNativeOrcaWorker(ctx, run, execution); err != nil {
		return err
	}
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return err
	}
	status, err := orcaworker.VerifyActiveDispatch(ctx, orcaWorkerMetaCaller(fixed.config), orcaworker.DispatchIdentity{
		DispatchID: turn.DispatchID, TaskID: turn.TaskID, RunID: execution.Target.OrchestrationRun,
		RuntimeID: execution.Process.RuntimeID, Terminal: execution.Process.Terminal,
	})
	if err != nil {
		return err
	}
	if status != "dispatched" {
		return nil // The original worker_done may still be in transit to the inbox.
	}
	intent := *request
	intent.DispatchCapability = ""
	opID, err := s.startWorkerOperation(run, "orca_native_relay_done", key, intent)
	if err != nil {
		return err
	}
	// Keep the original bounded request until its native result and settlement
	// have been stored. Unknown delivery never deletes the only worker evidence.
	receipt, err := orcaworker.SendDone(ctx, orcaWorkerMetaCaller(fixed.config), *request, execution.Target.OrchestrationRun, execution.Process.Terminal, execution.Process.RuntimeID)
	if err != nil {
		_ = s.changeWorkerOperation(run, opID, "outcome_unknown", "", nil, err.Error())
		return err
	}
	if err := s.changeWorkerOperation(run, opID, "accepted", receipt.RequestID, receipt.Payload, ""); err != nil {
		return err
	}
	s.note(run, "worker", "orca_relay", "accepted", "Orca accepted the worker's relay request", receipt.ID, map[string]any{"dispatch_id": turn.DispatchID})
	if err := s.commitWorker(run); err != nil {
		return err
	}
	return nil
}

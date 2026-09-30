package steerline

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
)

func showWorkerOrcaTerminal(ctx context.Context, c Config, handle string) (orcaTerminal, error) {
	terminal, err := orcaworker.ShowTerminal(ctx, orcaWorkerMetaCaller(c), handle)
	return terminal, workerOrcaBackendError(err)
}

func verifyWorkerOrcaTerminal(term orcaTerminal, run *Run) error {
	if run == nil || run.WorktreePath == nil || run.WorktreeBranch == nil || term.Handle == "" || term.RuntimeID == "" || term.IncarnationID == "" ||
		canonical(filepath.FromSlash(term.WorktreePath)) != canonical(*run.WorktreePath) ||
		strings.TrimPrefix(term.Branch, "refs/heads/") != *run.WorktreeBranch {
		return &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "Orca terminal changed the fixed worker workspace or branch"}
	}
	return nil
}

func workerOrcaIdentity(term orcaTerminal, c Config) executionapi.ProcessIdentity {
	return executionapi.ProcessIdentity{Kind: "orca", Terminal: term.Handle, RuntimeID: term.RuntimeID, Incarnation: term.IncarnationID, Executable: c.Orca.Executable}
}

func workerOrcaTerminalIdentity(identity executionapi.ProcessIdentity) orcaworker.TerminalIdentity {
	return orcaworker.TerminalIdentity{Kind: identity.Kind, Handle: identity.Terminal, RuntimeID: identity.RuntimeID, Incarnation: identity.Incarnation}
}

func (s *Service) closeNativeOrcaTerminal(ctx context.Context, run *Run, execution *ExecutionRecord, identity executionapi.ProcessIdentity, role string) error {
	if execution == nil {
		return errors.New("Orca terminal has no fixed execution")
	}
	config, configErr := s.executionConfigForWorkflowID(execution.WorkflowRunID)
	if configErr != nil {
		return configErr
	}
	fixedConfig := copyWorkerFlowConfig(config)
	if identity.Terminal == "" || identity.Incarnation == "" || identity.RuntimeID == "" || identity.RuntimeID != execution.Process.RuntimeID {
		return &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "Orca terminal cleanup lacks its original identity"}
	}
	key := "orca-native-close:" + execution.ExecutionID + ":" + role
	if op := s.operationByKey(key); op != nil && op.Status == "confirmed" {
		return nil
	}
	term, err := showWorkerOrcaTerminal(ctx, fixedConfig, identity.Terminal)
	if err != nil {
		if op := s.operationByKey(key); op != nil {
			selector := "path:" + filepath.ToSlash(fixedConfig.Repository)
			if role == "agent" {
				selector = "id:" + identity.WorktreeID
			}
			terms, runtimeID, listErr := nativeOrcaTerminals(ctx, fixedConfig, selector)
			if listErr == nil && runtimeID == identity.RuntimeID {
				for _, candidate := range terms {
					if candidate.Handle == identity.Terminal {
						return err
					}
				}
				return s.changeWorkerOperation(run, op.OperationID, "confirmed", op.PlatformRequestID, mustJSON(map[string]string{"closed_handle": identity.Terminal, "proof": "absent_on_original_runtime"}), "")
			}
		}
		return err
	}
	if term.RuntimeID != identity.RuntimeID || term.IncarnationID != identity.Incarnation || role == "agent" && term.WorktreeID != identity.WorktreeID || role == "coordinator" && canonical(filepath.FromSlash(term.WorktreePath)) != canonical(fixedConfig.Repository) {
		return &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "Orca cleanup terminal was replaced or moved"}
	}
	op := s.operationByKey(key)
	opID := ""
	if op == nil {
		opID, err = s.startWorkerOperation(run, "orca_native_terminal_close", key, map[string]string{"role": role, "handle": identity.Terminal, "incarnation": identity.Incarnation, "runtime": identity.RuntimeID})
		if err != nil {
			return err
		}
	} else {
		opID = op.OperationID
	}
	receipt, err := orcaworker.CloseTerminal(ctx, orcaWorkerMetaCaller(fixedConfig), identity.Terminal, identity.RuntimeID)
	if err != nil {
		var callErr *orcaworker.CallError
		if errors.As(err, &callErr) {
			_ = s.changeWorkerOperation(run, opID, "outcome_unknown", "", nil, err.Error())
		}
		return workerOrcaBackendError(err)
	}
	return s.changeWorkerOperation(run, opID, "confirmed", "", receipt, "")
}

func (s *Service) finishNativeOrcaCleanup(ctx context.Context, run *Run) error {
	fixed, configErr := s.workerFlowContext(run)
	if configErr != nil {
		return configErr
	}
	fixedConfig := fixed.config
	workflow, execution := s.workflowRecords(run)
	if run == nil || workflow == nil || execution == nil || execution.OrcaNative == nil || execution.Process == nil || execution.Process.Kind != "orca-native" || workflow.Phase != "completed" {
		return nil
	}
	state := execution.OrcaNative
	if execution.PendingTurnID != "" || execution.CurrentTurnID == "" || execution.CurrentTurnID != execution.LastStoppedTurnID || state.DeliveryID != "" {
		return nil // Delivery succeeded, but the external worker or inbox is not yet safe to close.
	}
	if err := s.closeNativeOrcaTerminal(ctx, run, execution, *execution.Process, "agent"); err != nil {
		return err
	}
	if state.Coordinator.Terminal == "" || execution.Target.OrchestrationRun == "" {
		return nil
	}
	delivery, err := orcaworker.CheckInbox(ctx, orcaWorkerMetaCaller(fixedConfig), state.Coordinator.Terminal, execution.Target.OrchestrationRun)
	if err != nil {
		return err
	}
	if delivery.RuntimeID != state.Coordinator.RuntimeID || delivery.RunID != execution.Target.OrchestrationRun || delivery.DeliveryID != "" || len(delivery.Messages) != 0 {
		return &executionapi.BackendError{Code: executionapi.BackendOutcomeUnknown, Diagnostic: "Orca coordinator inbox is not drained after delivery"}
	}
	return s.closeNativeOrcaTerminal(ctx, run, execution, state.Coordinator, "coordinator")
}

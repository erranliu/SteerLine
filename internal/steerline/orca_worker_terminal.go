package steerline

import (
	"context"
	"errors"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
)

func showWorkerOrcaTerminal(ctx context.Context, c Config, handle string) (orcaTerminal, error) {
	terminal, err := orcaworker.ShowTerminal(ctx, orcaWorkerMetaCaller(c), handle)
	return terminal, workerOrcaBackendError(err)
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
	absent, err := orcaworker.InspectCleanupTerminal(ctx, orcaWorkerMetaCaller(fixedConfig), identity, fixedConfig.Repository, role, s.operationByKey(key) != nil)
	if err != nil {
		return workerOrcaBackendError(err)
	}
	if absent {
		op := s.operationByKey(key)
		return s.changeWorkerOperation(run, op.OperationID, "confirmed", op.PlatformRequestID, mustJSON(map[string]string{"closed_handle": identity.Terminal, "proof": "absent_on_original_runtime"}), "")
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
	state, stateErr := decodeOrcaExecutionState(execution)
	if stateErr != nil {
		return stateErr
	}
	if run == nil || workflow == nil || execution == nil || state == nil || execution.Process == nil || execution.Process.Kind != "orca-native" || workflow.Phase != "completed" {
		return nil
	}
	if execution.PendingTurnID != "" || execution.CurrentTurnID == "" || execution.CurrentTurnID != execution.LastStoppedTurnID || state.DeliveryID != "" {
		return nil // Delivery succeeded, but the external worker or inbox is not yet safe to close.
	}
	if err := s.closeNativeOrcaTerminal(ctx, run, execution, *execution.Process, "agent"); err != nil {
		return err
	}
	if state.Coordinator.Terminal == "" || execution.Target.OrchestrationRun == "" {
		return nil
	}
	if err := orcaworker.InboxDrained(ctx, orcaWorkerMetaCaller(fixedConfig), state.Coordinator.Terminal, state.Coordinator.RuntimeID, execution.Target.OrchestrationRun); err != nil {
		return &executionapi.BackendError{Code: executionapi.BackendOutcomeUnknown, Diagnostic: err.Error(), Cause: err}
	}
	return s.closeNativeOrcaTerminal(ctx, run, execution, state.Coordinator, "coordinator")
}

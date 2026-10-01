package steerline

import (
	"context"
	"errors"
	"reflect"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
)

// Native Orca calls below are retained for the host-evidence boundary assigned to #65.
func orcaWorkerCaller(c Config) orcaworker.Call         { return nativeOrcaClient(c).Call }
func orcaWorkerMetaCaller(c Config) orcaworker.MetaCall { return nativeOrcaClient(c).MetaCall }

func workerOrcaTerminalIdentity(identity executionapi.ProcessIdentity) orcaworker.TerminalIdentity {
	return orcaworker.TerminalIdentity{Kind: identity.Kind, Handle: identity.Terminal, RuntimeID: identity.RuntimeID, Incarnation: identity.Incarnation}
}

func nativeOrcaGeneration(execution *ExecutionRecord) string {
	if execution == nil {
		return ""
	}
	if state, err := decodeOrcaExecutionState(execution); err == nil && state != nil && state.Coordinator.RuntimeID != "" {
		return state.Coordinator.RuntimeID
	}
	if execution.Process != nil && execution.Process.Incarnation != "" {
		return execution.Process.Incarnation
	}
	return ""
}

func (s *Service) verifyHostExecutionEvidence(ctx context.Context, run *Run, execution *ExecutionRecord) error {
	if execution == nil || execution.Process == nil {
		return errors.New("host execution has no original terminal")
	}
	if environment := execution.Environment; environment != nil {
		if execution.PolicyResources == nil || environment.Runtime != execution.Process.RuntimeID || environment.LaunchCommand == "" ||
			!reflect.DeepEqual(environment.WriteRoots, execution.PolicyResources.WriteRoots) || len(environment.WriteRoots) == 0 ||
			canonical(environment.WriteRoots[0]) != canonical(execution.Workspace.Worktree) {
			return &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "managed Orca resource evidence differs from its committed policy snapshot"}
		}
	}
	lifecycle, fixed, err := s.lifecycleForRun(run)
	if err != nil {
		return err
	}
	snapshot, err := s.lifecycleSnapshot(run, fixed.workflow, fixed.execution, fixed.attempt, nil)
	if err != nil {
		return err
	}
	evidence, err := lifecycle.VerifyManualRecovery(ctx, snapshot)
	if err != nil {
		return err
	}
	if !evidence.ManagedProcess {
		return &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "native Orca terminal is not managed by its original execution"}
	}
	return nil
}

// These native state readers support the host-evidence boundary assigned to #65.
func nativeOrcaCurrentTurn(execution *ExecutionRecord) *implementationTurnView {
	state, err := decodeOrcaExecutionState(execution)
	if err != nil || state == nil || len(state.Turns) == 0 {
		return nil
	}
	return &state.Turns[len(state.Turns)-1]
}

func persistNativeOrcaTurn(execution *ExecutionRecord, turn *implementationTurnView) error {
	if turn == nil {
		return errors.New("native turn is required")
	}
	state, err := decodeOrcaExecutionState(execution)
	if err != nil {
		return err
	}
	if state == nil {
		return errors.New("native execution state is unavailable")
	}
	for i := range state.Turns {
		if state.Turns[i].TurnID == turn.TurnID {
			state.Turns[i] = *turn
			return storeOrcaExecutionState(execution, state)
		}
	}
	return errors.New("native turn is not part of the fixed execution state")
}

func (s *Service) readNativeOrcaOutput(ctx context.Context, run *Run, execution *ExecutionRecord, turn *implementationTurnView) error {
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return err
	}
	state, err := decodeOrcaExecutionState(execution)
	if err != nil || state == nil || turn == nil {
		return errors.Join(err, errors.New("Orca output has no verified coordinator or turn binding"))
	}
	attempt := s.workflowAttempt(run, run.AttemptID)
	if attempt == nil {
		return errors.New("worker output has no attempt")
	}
	result, resetCursor, err := orcaworker.ReadWorkerOutput(ctx, orcaWorkerMetaCaller(fixed.config), turn.DispatchID, turn.OutputCursor, state.Coordinator.RuntimeID)
	if err != nil {
		return err
	}
	cursor := result.NextCursor
	if cursor == "" {
		cursor = result.Cursor
	}
	if turn.Purpose != "readiness" {
		if resetCursor {
			attempt.OutputCursor = ""
		}
		attempt.OutputSource, attempt.OutputCursor = result.Source, cursor
	}
	turn.OutputSource, turn.OutputCursor, turn.OutputComplete = result.Source, cursor, result.ContentComplete
	if turn.Purpose == "readiness" && execution.Environment != nil {
		preparation := execution.Environment.Preparation
		if resetCursor {
			preparation.Evidence = orcaworker.ProbeToolEvidence{}
		}
		preparation.Evidence, err = orcaworker.ObserveProbe(result, preparation.Command, preparation.Nonce, preparation.ScriptDigest, preparation.Evidence)
		if err != nil {
			return err
		}
		if preparation.Evidence.FailureCode != "" {
			return s.blockWorkerEnvironment(run, orcaworker.HostFact{Reason: preparation.Evidence.FailureCode, Confidence: "confirmed", Source: "orca.transcript.tool-result", Turn: turn.TurnID})
		}
	}
	if turn.Purpose == "business" && execution.Environment != nil {
		if resetCursor {
			turn.ToolContext = orcaworker.ToolContext{}
		}
		var failed bool
		turn.ToolContext, failed, err = orcaworker.ObserveToolInitialization(result, turn.TaskID, turn.IntentAt.UnixMilli(), turn.ToolContext)
		if err != nil {
			return err
		}
		if failed {
			return s.blockWorkerEnvironment(run, orcaworker.HostFact{Reason: "tool_initialization_failed", Confidence: "confirmed", Source: "orca.transcript.tool-result", Turn: turn.TurnID})
		}
	}
	turn.OutputClipping, turn.OutputFallbackReason = append([]string(nil), result.Clipping...), result.FallbackReason
	content := result.DisplayContent()
	if content != "" {
		digest := digestValue(struct{ Source, Content string }{result.Source, content})
		if digest != turn.OutputDigest {
			turn.OutputDigest = digest
			status := "bounded"
			if result.ContentComplete && len(result.Clipping) == 0 {
				status = "complete"
			}
			s.note(run, "worker", "output", status, "Orca worker output", clip(content, 7000), map[string]any{"source": result.Source, "fallback_reason": result.FallbackReason, "clipping": result.Clipping, "dispatch_id": turn.DispatchID})
		}
	}
	if result.SourceChanged || len(result.Clipping) > 0 || !result.ContentComplete {
		execution.LastEvent = ptr("Orca output is bounded (" + result.Source + "; " + result.FallbackReason + ")")
		s.refreshRun(run)
	}
	if err := persistNativeOrcaTurn(execution, turn); err != nil {
		return err
	}
	return s.commitWorker(run)
}

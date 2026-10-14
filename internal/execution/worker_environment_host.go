package execution

import (
	"context"
	"errors"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	"reflect"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

func (s *Service) hostEnvironmentForRun(run *reconstructiontypes.RunView) (executionbackend.HostEnvironmentPort, executionbackend.LifecyclePort, workerRecords, executionbackend.LifecycleSnapshot, error) {
	lifecycle, fixed, err := s.lifecycleForRun(run)
	if err != nil {
		return nil, nil, fixed, executionbackend.LifecycleSnapshot{}, err
	}
	host, ok := lifecycle.(executionbackend.HostEnvironmentPort)
	if !ok {
		return nil, lifecycle, fixed, executionbackend.LifecycleSnapshot{}, errors.New("selected implementation does not provide host evidence")
	}
	snapshot, err := s.lifecycleSnapshot(run, fixed.workflow, fixed.execution, fixed.attempt, nil)
	if err != nil {
		return nil, lifecycle, fixed, executionbackend.LifecycleSnapshot{}, err
	}
	return host, lifecycle, fixed, snapshot, nil
}

func (s *Service) currentHostTurn(run *reconstructiontypes.RunView) (*executionbackend.HostTurnFacts, error) {
	host, _, _, snapshot, err := s.hostEnvironmentForRun(run)
	if err != nil {
		return nil, err
	}
	return host.CurrentTurn(snapshot)
}

func (s *Service) resetHostTurnOutput(run *reconstructiontypes.RunView, turnID string) error {
	host, _, fixed, snapshot, err := s.hostEnvironmentForRun(run)
	if err != nil {
		return err
	}
	envelope, err := host.ResetTurnOutput(snapshot, turnID)
	if err != nil {
		return err
	}
	if err = mergeLifecycleExecutionReceipt(fixed.execution, executionbackend.LifecycleReceipt{Implementation: []executionbackend.ImplementationDataEnvelope{envelope}}); err != nil {
		return err
	}
	return s.commitWorker(run)
}

func (s *Service) executeHostAction(ctx context.Context, run *reconstructiontypes.RunView, lifecycle executionbackend.LifecyclePort, input executionbackend.HostActionInput) error {
	candidate := executionbackend.LifecycleCandidate{Action: executionbackend.LifecycleHostAction, Input: executionbackend.LifecycleInput{Host: &input}}
	return s.executeLifecycleStep(ctx, run, lifecycle, candidate)
}

func (s *Service) readHostOutput(ctx context.Context, run *reconstructiontypes.RunView) error {
	host, _, fixed, snapshot, err := s.hostEnvironmentForRun(run)
	if err != nil {
		return err
	}
	attempt := fixed.attempt
	if attempt == nil {
		return errors.New("host output has no original attempt")
	}
	input := executionbackend.HostReadInput{}
	if environment := fixed.execution.Environment; environment != nil && environment.Preparation != nil {
		p := environment.Preparation
		input.Probe = &executionbackend.HostProbeObservation{Command: p.Command, Nonce: p.Nonce, ScriptDigest: p.ScriptDigest, Evidence: p.Evidence}
	}
	output, envelope, err := host.ReadWorkerOutput(ctx, snapshot, input)
	if err != nil {
		return err
	}
	if err = mergeLifecycleExecutionReceipt(fixed.execution, executionbackend.LifecycleReceipt{Implementation: []executionbackend.ImplementationDataEnvelope{envelope}}); err != nil {
		return err
	}
	if output.ProbeEvidence != nil {
		if snapshot.PreparationProbe == nil || fixed.execution.Environment == nil || fixed.execution.Environment.Preparation == nil || fixed.execution.Environment.Preparation.Probe != *snapshot.PreparationProbe {
			return executionmodel.ErrTurnStale
		}
		fixed.execution.Environment.Preparation.Evidence = *output.ProbeEvidence
	}
	if output.UpdateAttemptCursor {
		if output.SourceChanged {
			attempt.OutputCursor = ""
		}
		attempt.OutputSource, attempt.OutputCursor = output.Source, output.Cursor
	}
	if output.Fact != nil {
		if output.Fact.Reason != "output_bounded" {
			return s.blockWorkerEnvironment(run, *output.Fact)
		}
		fixed.execution.LastEvent = ptr("Worker output is bounded (" + output.Source + "; " + output.FallbackReason + ")")
		s.refreshRun(run)
	}
	if output.Content != "" && output.ContentChanged {
		status := "bounded"
		if output.ContentComplete && len(output.Clipping) == 0 {
			status = "complete"
		}
		s.note(run, "worker", "output", status, "Worker output", clip(output.Content, 7000), map[string]any{"source": output.Source, "fallback_reason": output.FallbackReason, "clipping": output.Clipping})
	}
	return s.commitWorker(run)
}

func (s *Service) verifyHostExecutionEvidence(ctx context.Context, run *reconstructiontypes.RunView, execution *executionmodel.ExecutionRecord) error {
	if execution == nil || execution.Process == nil {
		return errors.New("host execution has no original terminal")
	}
	if environment := execution.Environment; environment != nil {
		if execution.PolicyResources == nil || environment.Runtime != execution.Process.RuntimeID || environment.LaunchCommand == "" ||
			!reflect.DeepEqual(environment.WriteRoots, execution.PolicyResources.WriteRoots) || len(environment.WriteRoots) == 0 ||
			canonical(environment.WriteRoots[0]) != canonical(execution.Workspace.Worktree) {
			return &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "managed host resource evidence differs from its committed policy snapshot"}
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
		return &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "host process is not managed by its original execution"}
	}
	return nil
}

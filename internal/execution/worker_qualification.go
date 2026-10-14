package execution

import (
	"context"
	"github.com/erranliu/SteerLine/internal/execution/reliable"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
)

type workerQualificationResult struct {
	runID, attemptID, generation, commandID, turnID string
	pendingCommandID, pendingTurnID                 string
	version                                         uint64
	evidence                                        executionbackend.LifecycleRecoveryEvidence
	err                                             error
}

// An attached observer need not be replaced just because an earlier gate probe
// failed. Qualification is retried through the worker, without sending input.
func (s *Service) recheckWorkerQualification(ctx context.Context, run *reconstructiontypes.RunView) error {
	if s.workerRecovering[run.RunID] || s.lifecycleEffectPending(run.RunID) {
		return nil
	}
	port, fixed, err := s.lifecycleForRun(run)
	if err != nil {
		return err
	}
	snapshot, err := s.lifecycleSnapshot(run, fixed.workflow, fixed.execution, fixed.attempt, nil)
	if err != nil {
		return err
	}
	e := fixed.execution
	result := workerQualificationResult{runID: run.RunID, attemptID: run.AttemptID, generation: e.Generation, commandID: e.CurrentCommandID, turnID: e.CurrentTurnID, pendingCommandID: e.PendingCommandID, pendingTurnID: e.PendingTurnID, version: e.RecoveryVersion}
	probe := func(parent context.Context) workerQualificationResult {
		probeCtx, cancel := context.WithTimeout(parent, reliable.QualificationTimeout(duration(fixed.config.CommandTimeout)))
		defer cancel()
		result.evidence, result.err = port.VerifyManualRecovery(probeCtx, snapshot)
		return result
	}
	if s.signalQueue == nil {
		return s.applyWorkerQualification(probe(ctx))
	}
	if s.workerRecovering == nil {
		s.workerRecovering = make(map[string]bool)
	}
	s.workerRecovering[run.RunID] = true
	s.forwarders.Add(1)
	messages, parent := s.signalQueue, s.runContext
	if parent == nil {
		parent = ctx
	}
	go func() {
		defer s.forwarders.Done()
		value := probe(parent)
		select {
		case messages <- Signal{runID: run.RunID, qualification: &value}:
		case <-parent.Done():
		}
	}()
	return nil
}

func (s *Service) applyWorkerQualification(result workerQualificationResult) error {
	run := s.find(result.runID)
	if run == nil || s.candidateBusinessClosed(run) {
		return nil
	}
	e := s.executionRecord(run)
	if e == nil || !e.RecoveryRequired || !reliable.CurrentObservation(reliableObservation(e), reliable.ObservationIdentity{ExecutionID: run.ExecutionID, AttemptID: result.attemptID, Generation: result.generation, RecoveryVersion: result.version, CommandID: result.commandID, TurnID: result.turnID, PendingCommandID: result.pendingCommandID, PendingTurnID: result.pendingTurnID}) {
		return nil
	}
	parent := s.runContext
	if parent == nil {
		parent = context.Background()
	}
	return s.acceptWorkerRecoveryQualification(parent, nativeTaskRecord(s.domain.Tasks, run.DomainTaskID), run, result.evidence, result.err)
}

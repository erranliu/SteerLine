package execution

import (
	"context"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
)

type workerQualificationResult struct {
	runID, attemptID, generation, commandID, turnID string
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
	result := workerQualificationResult{runID: run.RunID, attemptID: run.AttemptID, generation: e.Generation, commandID: e.CurrentCommandID, turnID: e.CurrentTurnID, version: e.RecoveryVersion}
	probe := func(parent context.Context) workerQualificationResult {
		probeCtx, cancel := context.WithTimeout(parent, min(positiveDuration(fixed.config.CommandTimeout, 30*time.Second), 10*time.Second))
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
	if e == nil || !e.RecoveryRequired || run.AttemptID != result.attemptID || e.Generation != result.generation || e.RecoveryVersion != result.version || e.CurrentCommandID != result.commandID || e.CurrentTurnID != result.turnID {
		return nil
	}
	if result.err != nil {
		return nil
	} // Retain uncertainty; a later tick can retry.
	before := recordFingerprint(*e)
	facts := executionmodel.RecoveryFacts{WorkerLive: result.evidence.ManagedProcess && !result.evidence.ProcessAbsent, WorkerAbsent: result.evidence.ProcessAbsent, WritersAbsent: result.evidence.WritersAbsent, Completed: settledOriginalWorkerTurn(e), QuestionPending: run.Decision != nil && run.Decision.Status == "pending" && run.Decision.Request.QuestionRoute != nil}
	s.setWorkerRecoveryFacts(run, facts, result.evidence.SessionAvailable)
	if before == recordFingerprint(*e) {
		return nil
	}
	s.stampPendingRecoveryQuestion(run)
	s.refreshRun(run)
	return s.commitWorker(run)
}

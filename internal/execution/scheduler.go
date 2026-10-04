package execution

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
)

func (s *Service) BeginActor(ctx context.Context) error {
	s.runContext = ctx
	s.signalQueue = make(chan Signal, 128)
	for runID, worker := range s.workers {
		s.forwardWorker(ctx, runID, worker)
	}
	s.State.Service.Mode = "scheduler"
	if err := s.refreshNativeSnapshot(ctx); err != nil {
		return err
	}
	if err := s.adoptOrGateServiceExecutions(); err != nil {
		return err
	}
	s.State.Service.Ready = false
	s.State.Service.Status = "running"
	s.State.Service.HeartbeatIntervalMS = duration(s.Config.Heartbeat).Milliseconds()
	if e := s.publishCheckpoint(); e != nil {
		return e
	}
	s.conversationReplayRequests = make(chan []conversationJournalSource, 16)
	s.conversationReplayErrors = make(chan error, 16)
	replayDone := make(chan struct{})
	s.replayDone = replayDone
	replaySources := s.conversationJournalSources()
	go func() {
		defer close(replayDone)
		s.runConversationReplayQueue(ctx, s.conversationReplayRequests, s.conversationReplayErrors)
	}()
	if len(replaySources) > 0 {
		s.conversationReplayRequests <- replaySources
	}
	s.historyRequests = make(chan []contracts.ConversationHistorySource, 1)
	historyDone := make(chan struct{})
	s.historyDone = historyDone
	go func() {
		defer close(historyDone)
		s.runConversationHistoryCollector(ctx, s.historyRequests, s.conversationReplayErrors)
	}()
	historySources, err := s.discoverConversationHistory(ctx)
	if err != nil {
		return err
	}
	s.historyRequests <- historySources

	return nil
}
func (s *Service) step(ctx context.Context, onlyWhenChanged bool, fn func(context.Context) error) error {
	var before []byte
	var beforeDatabaseID string
	var beforeDatabaseRevision uint64
	if onlyWhenChanged {
		var snapshotErr, versionErr error
		before, snapshotErr = json.Marshal(s.State)
		beforeDatabaseID, beforeDatabaseRevision, versionErr = s.domainStore.DatabaseVersion(ctx)
		if snapshotErr != nil || versionErr != nil {
			onlyWhenChanged = false // Fall back to the existing durable publication path.
		}
	}
	e := fn(ctx)
	if s.store.Err() != nil {
		return s.store.Err()
	}
	if e != nil {
		s.setError(e)
	} else {
		s.State.Service.Status = "running"
		s.State.Service.Error = nil
	}
	if onlyWhenChanged && e == nil {
		after, snapshotErr := json.Marshal(s.State)
		databaseID, databaseRevision, versionErr := s.domainStore.DatabaseVersion(ctx)
		if snapshotErr == nil && versionErr == nil && bytes.Equal(before, after) &&
			beforeDatabaseID == databaseID && beforeDatabaseRevision == databaseRevision {
			return nil
		}
	}
	if e := s.publishCheckpoint(); e != nil {
		return e
	}
	if s.State.Service.Ready {
		return s.publishStartupReady()
	}
	return nil
}

func (s *Service) Initialize(ctx context.Context, cancel context.CancelFunc) error {
	// Recovery observes the original workers before accepting new input.
	if e := s.step(ctx, false, s.pollPRs); e != nil {
		return e
	}
	if e := s.reconcile(ctx); e != nil {
		s.setError(e)
		if saveErr := s.publishCheckpoint(); saveErr != nil {
			return saveErr
		}
	}
	if e := s.step(ctx, false, s.resumeConfiguredWork); e != nil {
		return e
	}
	if e := s.step(ctx, false, s.processCommandsAndDecisions); e != nil {
		return e
	}
	candidateErr := s.refreshCandidates(ctx)
	if candidateErr != nil {
		s.setError(candidateErr)
		if saveErr := s.publishCheckpoint(); saveErr != nil {
			return saveErr
		}
	}
	s.State.Service.Ready = false
	if e := s.publishCheckpoint(); e != nil {
		return e
	}
	if e := s.pollControl(ctx, cancel); e != nil {
		return e
	}
	s.State.Service.Ready = candidateErr == nil && ctx.Err() == nil && s.State.Control.Operation == nil
	if e := s.publishCheckpoint(); e != nil {
		return e
	}
	if e := s.publishStartupReady(); e != nil {
		return e
	}
	if s.State.Control.Operation == nil {
		if e := s.step(ctx, false, s.dispatch); e != nil {
			return e
		}
	}

	return nil
}

func (s *Service) RecoverAdmission() (bool, error) {
	if s.State.Control.Operation == nil {
		return true, nil
	}
	if err := s.recoverControlOperation(); err != nil {
		if err = s.retainControlPublicationFailure(err); err != nil {
			return false, err
		}
	}
	return s.State.Control.Operation == nil, nil
}
func (s *Service) SetStartupMetadata(id string) { s.State.Service.StartupID = ptr(id) }
func (s *Service) Heartbeat() error             { return s.store.Heartbeat() }
func (s *Service) Signals() <-chan Signal       { return s.signalQueue }
func (s *Service) ReplayErrors() <-chan error   { return s.conversationReplayErrors }
func (s *Service) HandleSignal(message Signal) error {
	err := s.handleWorkerMessage(message)
	if errors.Is(err, executionmodel.ErrTurnStale) {
		return nil
	}
	return err
}
func (s *Service) HandleReplayError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	return s.step(ctx, false, func(context.Context) error { return err })
}
func (s *Service) PollHistory(ctx context.Context) {
	sources, err := s.discoverConversationHistory(ctx)
	if err != nil {
		select {
		case s.conversationReplayErrors <- err:
		default:
		}
		return
	}
	select {
	case s.historyRequests <- sources:
	default:
	}
}
func (s *Service) TickDecision(ctx context.Context) error {
	if s.State.Control.Operation != nil {
		return nil
	}
	return s.step(ctx, true, s.processCommandsAndDecisions)
}
func (s *Service) TickTask(ctx context.Context, cancel context.CancelFunc) error {
	if err := s.step(ctx, true, s.processCommandsAndDecisions); err != nil {
		return err
	}
	if err := s.step(ctx, true, s.reconcile); err != nil {
		return err
	}
	if err := s.pollControl(ctx, cancel); err != nil {
		return err
	}
	if s.State.Control.Operation != nil {
		return nil
	}
	if err := s.step(ctx, true, s.resumeConfiguredWork); err != nil {
		return err
	}
	if len(s.workers) != 0 && s.State.Control.DispatchMode == "auto" {
		return nil
	}
	return s.step(ctx, true, s.dispatch)
}
func (s *Service) TickControl(ctx context.Context, cancel context.CancelFunc) error {
	if err := s.step(ctx, true, s.processPendingCommands); err != nil {
		return err
	}
	return s.pollControl(ctx, cancel)
}
func (s *Service) TickPR(ctx context.Context) error {
	if s.State.Control.Operation != nil {
		return nil
	}
	return s.step(ctx, true, s.pollPRs)
}
func (s *Service) EndActor(stop *productapi.ControlRequest, acceptedAt *time.Time, prior error) error {
	if s.stopControl == nil && stop != nil {
		s.stopControl = stop
		s.stopAcceptedAt = acceptedAt
	}
	s.State.Service.Status = "stopping"
	s.State.Service.Ready = false
	prior = errors.Join(prior, s.markServiceRecoveryGate())
	// Preserve the original best-effort stopping publication and ensure the stop receipt before owned resources detach.
	s.publishCheckpoint()
	prior = errors.Join(prior, s.ensureStopApplyingReceipt())
	s.shutdown()
	if s.State.Control.Operation != nil {
		prior = errors.Join(prior, s.recoverControlOperation())
	}
	s.State.Service.Status = "stopped"
	if err := s.store.Err(); err != nil {
		prior = err
	}
	if prior != nil {
		s.setError(prior)
	}
	if err := s.publishCheckpoint(); err != nil {
		prior = err
	}
	prior = errors.Join(prior, s.finalizeStopControl())
	if s.replayDone != nil {
		<-s.replayDone
	}
	if s.historyDone != nil {
		<-s.historyDone
	}
	s.conversationReplayRequests = nil
	s.conversationReplayErrors = nil
	return prior
}
func (s *Service) InstanceID() string { return s.State.Service.InstanceID }
func (s *Service) WaitSources() {
	if s.replayDone != nil {
		<-s.replayDone
	}
	if s.historyDone != nil {
		<-s.historyDone
	}
	s.conversationReplayRequests = nil
	s.conversationReplayErrors = nil
}

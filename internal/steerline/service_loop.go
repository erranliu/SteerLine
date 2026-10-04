package steerline

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
)

func (s *Service) Run(ctx context.Context) (retErr error) {
	return s.RunWithDispatch(ctx, "auto")
}

func (s *Service) RunWithDispatch(ctx context.Context, mode string) error {
	if mode != "manual" && mode != "auto" {
		return errors.New("dispatch mode must be manual or auto")
	}
	return s.run(ctx, mode)
}

func (s *Service) run(ctx context.Context, dispatchMode string) (retErr error) {
	ctx, cancel := context.WithCancel(ctx)
	s.runContext = ctx
	s.workerMessages = make(chan workerMessage, 128)
	for runID, worker := range s.workers {
		s.forwardWorker(ctx, runID, worker)
	}
	var conversationReplayDone <-chan struct{}
	var historyCollectorDone <-chan struct{}
	defer func() {
		cancel()
		if conversationReplayDone != nil {
			<-conversationReplayDone
		}
		if historyCollectorDone != nil {
			<-historyCollectorDone
		}
		s.conversationReplayRequests = nil
		s.conversationReplayErrors = nil
	}()
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
	conversationReplayDone = replayDone
	replaySources := s.conversationJournalSources()
	go func() {
		defer close(replayDone)
		s.runConversationReplayQueue(ctx, s.conversationReplayRequests, s.conversationReplayErrors)
	}()
	if len(replaySources) > 0 {
		s.conversationReplayRequests <- replaySources
	}
	historyRequests := make(chan []contracts.ConversationHistorySource, 1)
	historyDone := make(chan struct{})
	historyCollectorDone = historyDone
	go func() {
		defer close(historyDone)
		s.runConversationHistoryCollector(ctx, historyRequests, s.conversationReplayErrors)
	}()
	historySources, err := s.discoverConversationHistory(ctx)
	if err != nil {
		return err
	}
	historyRequests <- historySources
	historyTick := time.NewTicker(2 * time.Second)
	defer historyTick.Stop()
	heartDone := make(chan struct{})
	acceptedStops := make(chan stopAcceptance, 1)
	instanceID := s.State.Service.InstanceID
	go func() {
		defer close(heartDone)
		t := time.NewTicker(duration(s.Config.Heartbeat))
		defer t.Stop()
		stopTick := time.NewTicker(500 * time.Millisecond)
		defer stopTick.Stop()
		stopSeen := map[string]bool{}
		for {
			select {
			case <-ctx.Done():
				return
			case <-stopTick.C:
				stopReq, _, checkedAt := findStopRequestCached(s.Config, instanceID, stopSeen)
				if stopReq != nil {
					if stopReq != nil {
						acceptedStops <- stopAcceptance{Request: *stopReq, At: checkedAt}
					}
					cancel()
					return
				}
			case <-t.C:
				if e := s.store.Heartbeat(); e != nil {
					cancel()
					return
				}
			}
		}
	}()
	defer func() {
		cancel()
		<-heartDone
		if s.stopControl == nil {
			select {
			case accepted := <-acceptedStops:
				s.stopControl = &accepted.Request
				s.stopAcceptedAt = &accepted.At
			default:
			}
		}
		s.State.Service.Status = "stopping"
		s.State.Service.Ready = false
		if err := s.markServiceRecoveryGate(); err != nil {
			retErr = errors.Join(retErr, err)
		}
		s.publishCheckpoint()
		if e := s.ensureStopApplyingReceipt(); e != nil {
			retErr = errors.Join(retErr, e)
		}
		s.shutdown()
		if s.State.Control.Operation != nil {
			if e := s.recoverControlOperation(); e != nil {
				retErr = errors.Join(retErr, e)
			}
		}
		s.State.Service.Status = "stopped"
		if e := s.store.Err(); e != nil {
			retErr = e
		}
		if retErr != nil {
			s.setError(retErr)
		}
		if e := s.publishCheckpoint(); e != nil {
			retErr = e
		}
		if e := s.finalizeStopControl(); e != nil {
			retErr = errors.Join(retErr, e)
		}
	}()
	taskTick := time.NewTicker(duration(s.Config.TaskPoll))
	defer taskTick.Stop()
	prTick := time.NewTicker(duration(s.Config.PRPoll))
	defer prTick.Stop()
	decisionTick := time.NewTicker(2 * time.Second)
	defer decisionTick.Stop()
	controlTick := time.NewTicker(500 * time.Millisecond)
	defer controlTick.Stop()
	stepWithChange := func(fn func(context.Context) error, onlyWhenChanged bool) error {
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
	step := func(fn func(context.Context) error) error { return stepWithChange(fn, false) }
	stepIfChanged := func(fn func(context.Context) error) error { return stepWithChange(fn, true) }
	for s.State.Control.Operation != nil {
		if e := s.recoverControlOperation(); e != nil {
			if e = s.retainControlPublicationFailure(e); e != nil {
				return e
			}
			select {
			case <-ctx.Done():
				return nil
			case <-controlTick.C:
			}
		}
	}
	// Recovery observes the original workers before accepting new input.
	if e := step(s.pollPRs); e != nil {
		return e
	}
	if e := s.reconcile(ctx); e != nil {
		s.setError(e)
		if saveErr := s.publishCheckpoint(); saveErr != nil {
			return saveErr
		}
	}
	if e := step(s.resumeConfiguredWork); e != nil {
		return e
	}
	if e := step(s.processCommandsAndDecisions); e != nil {
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
		if e := step(s.dispatch); e != nil {
			return e
		}
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-historyTick.C:
			historySources, discoverErr := s.discoverConversationHistory(ctx)
			if discoverErr != nil {
				select {
				case s.conversationReplayErrors <- discoverErr:
				default:
				}
				continue
			}
			select {
			case historyRequests <- historySources:
			default:
			}
		case replayErr := <-s.conversationReplayErrors:
			if replayErr != nil {
				if e := step(func(context.Context) error { return replayErr }); e != nil {
					return e
				}
			}
		case message := <-s.workerMessages:
			if err := s.handleWorkerMessage(message); err != nil && !errors.Is(err, executionmodel.ErrTurnStale) {
				return err
			}
		case <-decisionTick.C:
			if s.State.Control.Operation != nil {
				continue
			}
			if e := stepIfChanged(s.processCommandsAndDecisions); e != nil {
				return e
			}
		case <-taskTick.C:
			if e := stepIfChanged(s.processCommandsAndDecisions); e != nil {
				return e
			}
			if e := stepIfChanged(s.reconcile); e != nil {
				return e
			}
			if e := s.pollControl(ctx, cancel); e != nil {
				return e
			}
			if s.State.Control.Operation != nil {
				continue
			}
			if e := stepIfChanged(s.resumeConfiguredWork); e != nil {
				return e
			}
			if len(s.workers) != 0 && s.State.Control.DispatchMode == "auto" {
				continue
			}
			if e := stepIfChanged(s.dispatch); e != nil {
				return e
			}
		case <-controlTick.C:
			if e := stepIfChanged(s.processPendingCommands); e != nil {
				return e
			}
			if e := s.pollControl(ctx, cancel); e != nil {
				return e
			}
		case <-prTick.C:
			if s.State.Control.Operation != nil {
				continue
			}
			if e := stepIfChanged(s.pollPRs); e != nil {
				return e
			}
		}
	}
}

func (s *Service) shutdown() {
	defer func() {
		for runID, owned := range s.ownedProcesses {
			s.interruptOwnedLocalTurn(runID, owned)
			owned.Close()
			delete(s.ownedProcesses, runID)
		}
	}()
	if s.runContext != nil && s.runContext.Err() != nil {
		s.forwarders.Wait()
		s.drainWorkerMessages()
	}
	for _, messages := range s.lifecycleMessages {
		for _, message := range messages {
			if message.event != nil {
				rejectUndeliveredWorkerEvent(*message.event, context.Canceled)
			}
		}
	}
	s.lifecycleMessages, s.lifecycleEffects = nil, nil
	// The actor has stopped. Final owned-resource work still uses the same
	// background result path, awaited here because no message loop remains.
	s.workerMessages = nil
	for len(s.workers) != 0 {
		var runID string
		for id := range s.workers {
			runID = id
			break
		}
		var run *Run
		for _, candidate := range s.runs {
			if candidate != nil && candidate.RunID == runID {
				run = candidate
				break
			}
		}
		if run == nil {
			s.setError(errors.New("active worker has no fixed workflow run"))
			return
		}
		config, err := s.executionConfigForRun(run)
		if err != nil {
			s.setError(err)
			return // Preserve the original worker identity for restart reconciliation.
		}
		stoppingWorker := s.workers[runID]
		ctx, cancel := context.WithTimeout(context.Background(), duration(config.ShutdownGrace))
		err = stoppingWorker.detachLocalObservation(ctx)
		cancel()
		if err != nil {
			s.setError(fmt.Errorf("original worker observation could not be detached: %w", err))
			return
		}
		if s.workers[runID] == stoppingWorker {
			delete(s.workers, runID)
		}
	}
}

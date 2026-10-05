package execution

import (
	"context"
	"errors"
	"fmt"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	"path/filepath"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	api "github.com/erranliu/SteerLine/internal/product/api"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

const maxConversationCursorBytes = 1 << 20

func (s *Service) discoverConversationHistory(ctx context.Context) ([]ConversationHistorySource, error) {
	if s.conversationSources == nil {
		return nil, errors.New("conversation source port is required")
	}
	discovery := ConversationDiscovery{ProjectID: s.Config.identity, RunDirectory: s.Config.RunDir, Repository: s.Config.Repository}
	for _, run := range s.runs {
		if run == nil || run.RunID == "" || run.ExecutionID == "" || run.DomainTaskID == "" {
			continue
		}
		base := executionmodel.ConversationEvent{Version: executionmodel.ConversationVersion, ProjectID: s.Config.identity, TaskID: run.DomainTaskID, WorkflowID: run.RunID, AttemptID: run.AttemptID,
			ExecutionID: run.ExecutionID, Backend: executionbackend.BackendKind(run.Backend), Protocol: executionbackend.ExecutionProtocol(run.Protocol), Agent: run.Agent, TargetID: run.Target.ID,
			SessionQuality: executionmodel.IdentityUnknown, TurnQuality: executionmodel.IdentityUnknown, ItemQuality: executionmodel.IdentityUnknown}
		runSource := ConversationRunSource{Base: base, RunDirectory: filepath.Join(s.Config.RunDir, "runs", run.RunID), Workspace: deref(run.WorktreePath), WorkerFlow: run.WorkflowVersion == executionmodel.Version, Process: conversationProcessSnapshot(run.Process)}
		for _, attempt := range s.domain.Attempts {
			if attempt.ExecutionID != run.ExecutionID || attempt.EventPath == "" {
				continue
			}
			ended := attempt.AttemptID != run.AttemptID || run.EndedAt != nil
			if run.WorkflowVersion == executionmodel.Version && attempt.AttemptID == run.AttemptID {
				ended = run.TaskStatus == "closed" || run.TaskStatus == "discarded"
			}
			runSource.Attempts = append(runSource.Attempts, ConversationAttemptSource{AttemptID: attempt.AttemptID, EventPath: attempt.EventPath, Ended: ended})
		}
		discovery.Runs = append(discovery.Runs, runSource)
	}
	result, err := s.conversationSources.DiscoverHistory(ctx, discovery)
	if err != nil {
		return nil, err
	}
	for _, write := range result.Writes {
		if err = validateDiscoveredConversationOwner(discovery, write.Event); err != nil {
			return nil, err
		}
	}
	for _, source := range result.Sources {
		if err = validateDiscoveredConversationOwner(discovery, source.Event); err != nil {
			return nil, err
		}
	}
	if len(result.Writes) > 0 {
		if _, err = s.conversationStore.CommitConversation(ctx, result.Writes, executionmodel.ConversationCheckpoint{}); err != nil {
			return nil, err
		}
	}
	return result.Sources, nil
}

func validateDiscoveredConversationOwner(discovery ConversationDiscovery, event executionmodel.ConversationEvent) error {
	if event.ProjectID != discovery.ProjectID {
		return errors.New("conversation discovery changed its original project")
	}
	for _, run := range discovery.Runs {
		if run.Base.WorkflowID != event.WorkflowID || run.Base.ExecutionID != event.ExecutionID {
			continue
		}
		known := run.Base.AttemptID != "" && run.Base.AttemptID == event.AttemptID
		for _, attempt := range run.Attempts {
			known = known || attempt.AttemptID != "" && attempt.AttemptID == event.AttemptID
		}
		if !known {
			return errors.New("conversation discovery changed its original attempt")
		}
		expected := run.Base
		expected.AttemptID = event.AttemptID
		return validateConversationSourceWrites(expected, []executionmodel.ConversationWrite{{Event: event}})
	}
	return errors.New("conversation discovery returned an unknown original execution")
}

func (s *Service) collectConversationHistoryPage(ctx context.Context, source ConversationHistorySource) (bool, error) {
	if s.conversationSources == nil || s.Config.ProcessHost == nil {
		return false, errors.New("conversation source port and managed process host are required")
	}
	event := source.Event
	if event.ProjectID != s.Config.identity || event.ExecutionID == "" || event.AttemptID == "" || event.Source == "" || event.SourceSegment == "" || event.SourceIdentity == "" {
		return false, errors.New("conversation history source lost its fixed identity")
	}
	previousCursor := ""
	checkpoint, err := s.conversationStore.ConversationCheckpoint(ctx, event.ProjectID, event.ExecutionID, event.AttemptID, event.Source, event.SourceSegment)
	hadCheckpoint := err == nil
	if err == nil {
		if len(checkpoint.Cursor) > maxConversationCursorBytes {
			return false, errors.New("persisted conversation source cursor exceeds the read budget")
		}
		previousCursor = checkpoint.Cursor
	} else if !errors.Is(err, storageports.ErrNotFound) {
		return false, err
	}
	platformVersion := ""
	if source.RequiresPlatformVersion {
		var run *reconstructiontypes.RunView
		for _, candidate := range s.runs {
			if candidate != nil && candidate.RunID == event.WorkflowID && candidate.ExecutionID == event.ExecutionID {
				run = candidate
				break
			}
		}
		if run == nil {
			return false, errors.New("host history source has no fixed worker-flow binding")
		}
		host, _, _, _, hostErr := s.hostEnvironmentForRun(run)
		if hostErr != nil {
			return false, hostErr
		}
		platformVersion, err = host.PlatformVersion(ctx)
		if err != nil {
			return false, err
		}
	}
	batch, err := s.conversationSources.ReadHistory(ctx, ConversationHistoryReadRequest{Source: source, Cursor: previousCursor, HasCheckpoint: hadCheckpoint,
		Repository: s.Config.Repository, RunDirectory: s.Config.RunDir, PlatformVersion: platformVersion}, s.Config.ProcessHost.StartManaged)
	if err != nil {
		return false, err
	}
	if err = validateConversationSourceWrites(event, batch.Writes); err != nil {
		return false, err
	}
	if len(batch.Cursor) > maxConversationCursorBytes {
		return false, errors.New("conversation source cursor exceeds the persisted budget")
	}
	newCheckpoint := executionmodel.ConversationCheckpoint{ProjectID: event.ProjectID, ExecutionID: event.ExecutionID, AttemptID: event.AttemptID,
		Source: event.Source, SourceSegment: event.SourceSegment, SourceIdentity: event.SourceIdentity,
		Generation: event.SessionID, Cursor: batch.Cursor, LocalSeq: batch.Sequence}
	writes := batch.Writes
	for len(writes) > 128 {
		if _, err = s.conversationStore.CommitConversation(ctx, writes[:128], executionmodel.ConversationCheckpoint{}); err != nil {
			return false, err
		}
		writes = writes[128:]
	}
	if len(writes) > 0 || batch.Cursor != previousCursor {
		if _, err = s.conversationStore.CommitConversation(ctx, writes, newCheckpoint); err != nil {
			return false, err
		}
	}
	if batch.Capabilities != nil {
		report := *batch.Capabilities
		reports, reportErr := s.conversationStore.ConversationCapabilityReports(ctx, report.Backend, report.Protocol, report.Target.ID, 100)
		if reportErr != nil {
			return false, reportErr
		}
		var previous *api.ConversationCapabilityReport
		for i := range reports {
			if reports[i].ExecutionID == report.ExecutionID && reports[i].AttemptID == report.AttemptID && reports[i].SourceIdentity == report.SourceIdentity {
				previous = &reports[i]
				break
			}
		}
		if previous != nil && previous.AgentVersion == report.AgentVersion && previous.PlatformVersion == report.PlatformVersion {
			for i := range report.Capabilities {
				for _, old := range previous.Capabilities {
					if old.Name != report.Capabilities[i].Name {
						continue
					}
					oldStatus := executionbackend.CapabilityStatus(old.Status)
					if report.Capabilities[i].Status == executionbackend.CapabilityNotTested && oldStatus != executionbackend.CapabilityNotTested || oldStatus == executionbackend.CapabilityVerified && report.Capabilities[i].Status != executionbackend.CapabilityVerified {
						report.Capabilities[i].Status = oldStatus
						report.Capabilities[i].Evidence = old.Evidence
					}
				}
			}
			same := len(report.Capabilities) == len(previous.Capabilities)
			for i, capability := range report.Capabilities {
				if !same || capability.Name != previous.Capabilities[i].Name || capability.Status != executionbackend.CapabilityStatus(previous.Capabilities[i].Status) || capability.Evidence != previous.Capabilities[i].Evidence {
					same = false
					break
				}
			}
			if same {
				return !batch.More && batch.Settled, nil
			}
		}
		if err = s.conversationStore.SaveConversationCapabilityReport(ctx, report); err != nil {
			return false, err
		}
	}
	return !batch.More && batch.Settled, nil
}

func (s *Service) runConversationHistoryCollector(ctx context.Context, requests <-chan []ConversationHistorySource, errorsOut chan<- error) {
	finished := map[string]bool{}
	activeIndex, historicalIndex := 0, 0
	for {
		select {
		case <-ctx.Done():
			return
		case sources, ok := <-requests:
			if !ok {
				return
			}
			active, historical := []ConversationHistorySource{}, []ConversationHistorySource{}
			for _, source := range sources {
				if source.Ended {
					historical = append(historical, source)
				} else {
					active = append(active, source)
				}
			}
			for i, j := 0, len(historical)-1; i < j; i, j = i+1, j-1 {
				historical[i], historical[j] = historical[j], historical[i]
			}
			pick := func(group []ConversationHistorySource, next *int) (ConversationHistorySource, bool) {
				for i := 0; i < len(group); i++ {
					index := (*next + i) % len(group)
					source := group[index]
					if !finished[source.Identity] {
						*next = index + 1
						return source, true
					}
				}
				return ConversationHistorySource{}, false
			}
			selected := make([]ConversationHistorySource, 0, 2)
			if source, ok := pick(active, &activeIndex); ok {
				selected = append(selected, source)
			}
			if source, ok := pick(historical, &historicalIndex); ok {
				selected = append(selected, source)
			}
			for _, source := range selected {
				readCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
				settled, readErr := s.collectConversationHistoryPage(readCtx, source)
				cancel()
				if readErr != nil {
					if ctx.Err() != nil {
						return
					}
					event := source.Event
					event.Type, event.Status = executionmodel.ConversationGap, "source_unavailable"
					event.SourceEventID = "read-error:" + source.Identity
					event.Diagnostic = readErr.Error()
					_, _ = s.conversationStore.CommitConversation(ctx, []executionmodel.ConversationWrite{{Event: event}}, executionmodel.ConversationCheckpoint{})
					select {
					case errorsOut <- fmt.Errorf("conversation history: %w", readErr):
					default:
					}
				} else if source.Ended && settled {
					finished[source.Identity] = true
				}
			}
		}
	}
}

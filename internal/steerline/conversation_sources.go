package steerline

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

func (s *Service) discoverConversationHistory(ctx context.Context) ([]contracts.ConversationHistorySource, error) {
	if s.conversationSources == nil {
		return nil, errors.New("conversation source port is required")
	}
	discovery := contracts.ConversationDiscovery{ProjectID: s.Config.identity, RunDirectory: s.Config.RunDir, Repository: s.Config.Repository}
	for _, run := range s.runs {
		if run == nil || run.RunID == "" || run.ExecutionID == "" || run.DomainTaskID == "" {
			continue
		}
		base := ConversationEvent{Version: ConversationVersion, ProjectID: s.Config.identity, TaskID: run.DomainTaskID, WorkflowID: run.RunID, AttemptID: run.AttemptID,
			ExecutionID: run.ExecutionID, Backend: executionapi.BackendKind(run.Backend), Protocol: executionapi.ExecutionProtocol(run.Protocol), Agent: run.Agent, TargetID: run.Target.ID,
			SessionQuality: IdentityUnknown, TurnQuality: IdentityUnknown, ItemQuality: IdentityUnknown}
		runSource := contracts.ConversationRunSource{Base: base, RunDirectory: filepath.Join(s.Config.RunDir, "runs", run.RunID), Workspace: deref(run.WorktreePath), WorkerFlow: run.WorkflowVersion == WorkerFlowVersion, Process: conversationProcessSnapshot(run.Process)}
		for _, attempt := range s.domain.Attempts {
			if attempt.ExecutionID != run.ExecutionID || attempt.EventPath == "" {
				continue
			}
			ended := attempt.AttemptID != run.AttemptID || run.EndedAt != nil
			if run.WorkflowVersion == WorkerFlowVersion && attempt.AttemptID == run.AttemptID {
				ended = run.TaskStatus == "closed" || run.TaskStatus == "discarded"
			}
			runSource.Attempts = append(runSource.Attempts, contracts.ConversationAttemptSource{AttemptID: attempt.AttemptID, EventPath: attempt.EventPath, Ended: ended})
		}
		discovery.Runs = append(discovery.Runs, runSource)
	}
	result, err := s.conversationSources.DiscoverHistory(ctx, discovery)
	if err != nil {
		return nil, err
	}
	if len(result.Writes) > 0 {
		if _, err = s.domainStore.CommitConversation(ctx, result.Writes, ConversationCheckpoint{}); err != nil {
			return nil, err
		}
	}
	return result.Sources, nil
}

func (s *Service) collectConversationHistoryPage(ctx context.Context, source contracts.ConversationHistorySource) (bool, error) {
	event := source.Event
	if event.ProjectID != s.Config.identity || event.ExecutionID == "" || event.AttemptID == "" || event.Source == "" || event.SourceSegment == "" || event.SourceIdentity == "" {
		return false, errors.New("conversation history source lost its fixed identity")
	}
	previousCursor := ""
	checkpoint, err := s.domainStore.ConversationCheckpoint(ctx, event.ProjectID, event.ExecutionID, event.AttemptID, event.Source, event.SourceSegment)
	hadCheckpoint := err == nil
	if err == nil {
		if len(checkpoint.Cursor) > MaxEventPageBytes {
			return false, errors.New("persisted conversation source cursor exceeds the read budget")
		}
		previousCursor = checkpoint.Cursor
	} else if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	platformVersion := ""
	if source.RequiresPlatformVersion {
		var run *Run
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
	batch, err := s.conversationSources.ReadHistory(ctx, contracts.ConversationHistoryReadRequest{Source: source, Cursor: previousCursor, HasCheckpoint: hadCheckpoint,
		Repository: s.Config.Repository, RunDirectory: s.Config.RunDir, PlatformVersion: platformVersion}, localManagedProcessHost{timeout: duration(s.Config.CommandTimeout)}.StartManaged)
	if err != nil {
		return false, err
	}
	if err = validateConversationSourceWrites(event, batch.Writes); err != nil {
		return false, err
	}
	if len(batch.Cursor) > MaxEventPageBytes {
		return false, errors.New("conversation source cursor exceeds the persisted budget")
	}
	newCheckpoint := ConversationCheckpoint{ProjectID: event.ProjectID, ExecutionID: event.ExecutionID, AttemptID: event.AttemptID,
		Source: event.Source, SourceSegment: event.SourceSegment, SourceIdentity: event.SourceIdentity,
		Generation: event.SessionID, Cursor: batch.Cursor, LocalSeq: batch.Sequence}
	writes := batch.Writes
	for len(writes) > 128 {
		if _, err = s.domainStore.CommitConversation(ctx, writes[:128], ConversationCheckpoint{}); err != nil {
			return false, err
		}
		writes = writes[128:]
	}
	if len(writes) > 0 || batch.Cursor != previousCursor {
		if _, err = s.domainStore.CommitConversation(ctx, writes, newCheckpoint); err != nil {
			return false, err
		}
	}
	if batch.Capabilities != nil {
		report := *batch.Capabilities
		reports, reportErr := s.domainStore.ConversationCapabilityReports(ctx, report.Backend, report.Protocol, report.Target.ID, 100)
		if reportErr != nil {
			return false, reportErr
		}
		var previous *ConversationCapabilityReport
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
					if report.Capabilities[i].Status == executionapi.CapabilityNotTested && old.Status != executionapi.CapabilityNotTested || old.Status == executionapi.CapabilityVerified && report.Capabilities[i].Status != executionapi.CapabilityVerified {
						report.Capabilities[i] = old
					}
				}
			}
			if sameConversationCapabilities(report.Capabilities, previous.Capabilities) {
				return !batch.More && batch.Settled, nil
			}
		}
		if err = s.domainStore.SaveConversationCapabilityReport(ctx, report); err != nil {
			return false, err
		}
	}
	return !batch.More && batch.Settled, nil
}

func sameConversationCapabilities(a, b []ConversationCapability) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (s *Service) runConversationHistoryCollector(ctx context.Context, requests <-chan []contracts.ConversationHistorySource, errorsOut chan<- error) {
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
			active, historical := []contracts.ConversationHistorySource{}, []contracts.ConversationHistorySource{}
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
			pick := func(group []contracts.ConversationHistorySource, next *int) (contracts.ConversationHistorySource, bool) {
				for i := 0; i < len(group); i++ {
					index := (*next + i) % len(group)
					source := group[index]
					if !finished[source.Identity] {
						*next = index + 1
						return source, true
					}
				}
				return contracts.ConversationHistorySource{}, false
			}
			selected := make([]contracts.ConversationHistorySource, 0, 2)
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
					event.Type, event.Status = ConversationGap, "source_unavailable"
					event.SourceEventID = "read-error:" + source.Identity
					event.Diagnostic = readErr.Error()
					_, _ = s.domainStore.CommitConversation(ctx, []ConversationWrite{{Event: event}}, ConversationCheckpoint{})
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

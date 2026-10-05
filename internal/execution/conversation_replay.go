package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	"io"
	"os"
	"path/filepath"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

type conversationJournalSource struct {
	Path           string
	RunID          string
	TaskID         string
	WorkflowID     string
	ExecutionID    string
	AttemptID      string
	Backend        executionbackend.BackendKind
	Protocol       executionbackend.ExecutionProtocol
	Agent          string
	TargetID       string
	ExternalRef    string
	ProjectID      string
	Ended          bool
	FileID         string
	ExpectedFileID string
	Diagnostic     string
	Workspace      string
}

func (s *Service) conversationJournalSources() []conversationJournalSource {
	if s.conversationSources == nil {
		return nil
	}
	discovery := ConversationDiscovery{ProjectID: s.Config.identity, RunDirectory: s.Config.RunDir, Repository: s.Config.Repository}
	for _, run := range s.runs {
		if run == nil || run.RunID == "" || run.ExecutionID == "" || run.DomainTaskID == "" {
			continue
		}
		base := executionmodel.ConversationEvent{Version: executionmodel.ConversationVersion, ProjectID: s.Config.identity, TaskID: run.DomainTaskID, WorkflowID: run.RunID, AttemptID: run.AttemptID,
			ExecutionID: run.ExecutionID, Backend: executionbackend.BackendKind(run.Backend), Protocol: executionbackend.ExecutionProtocol(run.Protocol), Agent: run.Agent, TargetID: run.Target.ID}
		runSource := ConversationRunSource{Base: base, RunDirectory: filepath.Join(s.Config.RunDir, "runs", run.RunID), Workspace: deref(run.WorktreePath),
			WorkerFlow: run.WorkflowVersion == executionmodel.Version, Process: conversationProcessSnapshot(run.Process)}
		for _, attempt := range s.domain.Attempts {
			if attempt.ExecutionID != run.ExecutionID || attempt.EventPath == "" || !executionbackend.StableExecutionID(attempt.AttemptID) {
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
	identify := func(path string) (string, error) {
		file, identity, err := openJournal(path)
		if err != nil {
			return "", err
		}
		return identity, file.Close()
	}
	found, err := s.conversationSources.DiscoverJournals(context.Background(), discovery, identify)
	if err != nil {
		s.setError(err)
		return nil
	}
	sources := make([]conversationJournalSource, 0, len(found))
	for _, value := range found {
		event := value.Event
		sources = append(sources, conversationJournalSource{Path: value.Path, RunID: event.WorkflowID, TaskID: event.TaskID, WorkflowID: event.WorkflowID,
			ExecutionID: event.ExecutionID, AttemptID: value.AttemptID, Backend: event.Backend, Protocol: event.Protocol, Agent: event.Agent, TargetID: event.TargetID,
			ExternalRef: event.ExternalRef, ProjectID: event.ProjectID, Ended: value.Ended, FileID: value.FileID, ExpectedFileID: value.ExpectedFileID,
			Diagnostic: value.Diagnostic, Workspace: value.Workspace})
	}
	return sources
}

func (s *Service) runConversationReplayQueue(ctx context.Context, requests <-chan []conversationJournalSource, errorsOut chan<- error) {
	for {
		select {
		case <-ctx.Done():
			return
		case sources, ok := <-requests:
			if !ok {
				return
			}
			for _, source := range sources {
				if err := ctx.Err(); err != nil {
					return
				}
				if source.Diagnostic != "" {
					select {
					case errorsOut <- fmt.Errorf("conversation source %s/%s: %s", source.RunID, source.AttemptID, source.Diagnostic):
					case <-ctx.Done():
						return
					}
					continue
				}
				if err := s.replayConversationJournal(ctx, source); err != nil && !errors.Is(err, context.Canceled) {
					select {
					case errorsOut <- fmt.Errorf("conversation replay %s/%s: %w", source.RunID, source.AttemptID, err):
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}
}

func (s *Service) reconcileConversationRun(run *reconstructiontypes.RunView) error {
	if run == nil {
		return nil
	}
	if s.conversationReplayRequests == nil {
		return nil
	}
	sources := []conversationJournalSource{}
	for _, source := range s.conversationJournalSources() {
		if source.RunID != run.RunID || source.AttemptID != nonEmpty(run.AttemptID, run.RunID) {
			continue
		}
		source.Ended = true
		sources = append(sources, source)
	}
	if len(sources) == 0 {
		return nil
	}
	select {
	case s.conversationReplayRequests <- sources:
	case <-time.After(50 * time.Millisecond):
		return errors.New("conversation replay queue is full; raw source remains available for startup replay")
	}
	return nil
}

func (s *Service) replayConversationJournal(ctx context.Context, source conversationJournalSource) error {
	const batchRecords = 256
	readCursor := JournalCursor{}
	if s.conversationSources == nil {
		return errors.New("conversation source port is required")
	}
	journalSource := s.conversationSources.JournalSource(source.Backend, source.Protocol)
	if journalSource == "" {
		return errors.New("conversation implementation does not support journal replay for this execution")
	}
	mapper := s.conversationSources.RestoreMapper(executionmodel.ConversationMapperSnapshot{})
	var sourceSeq uint64
	checkpoints, err := s.conversationStore.ConversationSourceCheckpoints(ctx, source.ProjectID, source.ExecutionID, source.AttemptID, journalSource)
	if err != nil {
		return err
	}
	segment := s.conversationSources.JournalSegment(source.Backend, source.Protocol, source.FileID)
	generation := source.FileID
	knownSegment := false
	var prior *executionmodel.ConversationCheckpoint
	var matching *executionmodel.ConversationCheckpoint
	for i := range checkpoints {
		checkpoint := checkpoints[i]
		if checkpoint.SourceIdentity == source.FileID && (matching == nil || checkpoint.UpdatedAt.After(matching.UpdatedAt)) {
			knownSegment = true
			segment, generation = checkpoint.SourceSegment, checkpoint.Generation
			copy := checkpoint
			matching = &copy
		}
		if prior == nil || checkpoint.UpdatedAt.After(prior.UpdatedAt) {
			copy := checkpoint
			prior = &copy
		}
	}
	if matching != nil {
		position, decodeErr := executionmodel.DecodeConversationJournalPosition(*matching)
		if decodeErr != nil {
			return decodeErr
		}
		readCursor = journalCursorOf(position, source.Path)
		if stat, statErr := os.Stat(source.Path); statErr == nil && stat.Size() == position.FileSize && stat.ModTime().UnixNano() != position.ModifiedNS && position.ByteOffset > 0 {
			// Same-size rewrites can preserve both byte anchors. Force the
			// existing journal reset path to open a visible source_changed segment.
			readCursor.HeaderHash = "modified-since-checkpoint"
		}
		sourceSeq = position.SourceSeq
		snapshot, snapshotErr := s.conversationStore.ConversationMapperSnapshot(ctx, source.ProjectID, source.ExecutionID, source.AttemptID, journalSource, segment)
		if snapshotErr != nil {
			return snapshotErr
		}
		mapper = s.conversationSources.RestoreMapper(snapshot)
	}
	if !knownSegment && (prior != nil || source.ExpectedFileID != "" && source.ExpectedFileID != source.FileID) {
		previousIdentity := source.ExpectedFileID
		if previousIdentity == "" && prior != nil {
			previousIdentity = prior.SourceIdentity
		}
		gap := executionmodel.ConversationEvent{
			Version: executionmodel.ConversationVersion, ProjectID: source.ProjectID, TaskID: source.TaskID,
			WorkflowID: source.WorkflowID, ExecutionID: source.ExecutionID, AttemptID: source.AttemptID,
			Backend: source.Backend, Protocol: source.Protocol, Agent: source.Agent, TargetID: source.TargetID,
			ExternalRef: source.ExternalRef, Source: journalSource, SourceRole: executionmodel.ConversationSourceFallback, SourceSegment: segment,
			SourceIdentity: source.FileID, SessionQuality: executionmodel.IdentityUnknown, TurnQuality: executionmodel.IdentityUnknown,
			ItemQuality: executionmodel.IdentityUnknown, SourceEventID: "source-changed:" + previousIdentity + ":" + source.FileID,
			SourceEventQuality: executionmodel.IdentityDerived, Type: executionmodel.ConversationGap, Phase: executionmodel.MessagePhaseUnknown,
			Status: "source_changed", ReceivedAt: now(),
			Diagnostic: "events.jsonl file identity changed; a new source segment was opened",
			Evidence:   map[string]string{"source_record": "events.jsonl", "source_changed": "true", "previous_identity": previousIdentity, "current_identity": source.FileID},
		}
		position, positionErr := readConversationJournalPosition(source.Path, source.FileID, 0, 0, 1, nil)
		if positionErr != nil {
			return positionErr
		}
		checkpointCursor, positionErr := marshalConversationJournalPosition(position)
		if positionErr != nil {
			return positionErr
		}
		checkpoint := executionmodel.ConversationCheckpoint{ProjectID: source.ProjectID, ExecutionID: source.ExecutionID, AttemptID: source.AttemptID, Source: gap.Source, SourceSegment: segment, SourceIdentity: source.FileID, Generation: generation, Cursor: checkpointCursor, UpdatedAt: now()}
		if _, err = s.conversationStore.CommitConversation(ctx, []executionmodel.ConversationWrite{{Event: gap}}, checkpoint); err != nil {
			return err
		}
	}
	for {
		if err = ctx.Err(); err != nil {
			return err
		}
		wasInitialized := readCursor.Initialized
		batch, readErr := ReadJournalBatch(source.Path, readCursor, JournalOptions{FromStart: true, KeepCR: true, MaxRecords: batchRecords, MaxBytes: JournalMaxBytes})
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return nil
			}
			return readErr
		}
		if wasInitialized && batch.Reset {
			previousIdentity := source.FileID
			if batch.Cursor.FileID != "" {
				source.FileID = batch.Cursor.FileID
			}
			generation = fmt.Sprintf("%s:%d", source.FileID, batch.Cursor.Generation)
			segment = s.conversationSources.JournalSegment(source.Backend, source.Protocol, generation)
			mapper, sourceSeq = s.conversationSources.RestoreMapper(executionmodel.ConversationMapperSnapshot{}), 0
			gap := executionmodel.ConversationEvent{
				Version: executionmodel.ConversationVersion, ProjectID: source.ProjectID, TaskID: source.TaskID,
				WorkflowID: source.WorkflowID, ExecutionID: source.ExecutionID, AttemptID: source.AttemptID,
				Backend: source.Backend, Protocol: source.Protocol, Agent: source.Agent, TargetID: source.TargetID,
				ExternalRef: source.ExternalRef, Source: journalSource, SourceRole: executionmodel.ConversationSourceFallback, SourceSegment: segment,
				SourceIdentity: source.FileID, SessionQuality: executionmodel.IdentityUnknown, TurnQuality: executionmodel.IdentityUnknown,
				ItemQuality: executionmodel.IdentityUnknown, SourceEventID: "source-changed:" + previousIdentity + ":" + source.FileID,
				SourceEventQuality: executionmodel.IdentityDerived, Type: executionmodel.ConversationGap, Phase: executionmodel.MessagePhaseUnknown,
				Status: "source_changed", ReceivedAt: now(),
				Diagnostic: "events.jsonl changed while being reconciled; a new source segment was opened",
				Evidence:   map[string]string{"source_record": "events.jsonl", "source_changed": "true", "previous_identity": previousIdentity, "current_identity": source.FileID},
			}
			position, positionErr := readConversationJournalPosition(source.Path, source.FileID, 0, 0, batch.Cursor.Generation, nil)
			if positionErr != nil {
				return positionErr
			}
			checkpointCursor, positionErr := marshalConversationJournalPosition(position)
			if positionErr != nil {
				return positionErr
			}
			checkpoint := executionmodel.ConversationCheckpoint{ProjectID: source.ProjectID, ExecutionID: source.ExecutionID, AttemptID: source.AttemptID, Source: gap.Source, SourceSegment: segment, SourceIdentity: source.FileID, Generation: generation, Cursor: checkpointCursor, UpdatedAt: now()}
			if _, err = s.conversationStore.CommitConversation(ctx, []executionmodel.ConversationWrite{{Event: gap}}, checkpoint); err != nil {
				return err
			}
		}
		readCursor = batch.Cursor
		writes := make([]executionmodel.ConversationWrite, 0, len(batch.Records))
		for _, record := range batch.Records {
			sourceSeq++
			end := record.Offset + record.Length
			cursor, _ := json.Marshal(struct {
				ByteOffset int64  `json:"byte_offset"`
				ByteLength int64  `json:"byte_length"`
				SourceSeq  uint64 `json:"source_seq"`
			}{end, record.Length, sourceSeq})
			base := executionmodel.ConversationEvent{
				Version: executionmodel.ConversationVersion, ProjectID: source.ProjectID, TaskID: source.TaskID,
				WorkflowID: source.WorkflowID, ExecutionID: source.ExecutionID, AttemptID: source.AttemptID,
				Backend: source.Backend, Protocol: source.Protocol, Agent: source.Agent, TargetID: source.TargetID,
				ExternalRef: source.ExternalRef, Source: journalSource, SourceRole: executionmodel.ConversationSourceFallback, SourceSegment: segment,
				SourceIdentity: source.FileID, SourceCursor: string(cursor),
				SessionQuality: executionmodel.IdentityUnknown, TurnQuality: executionmodel.IdentityUnknown, ItemQuality: executionmodel.IdentityUnknown,
				SourceEventID: fmt.Sprintf("bytes:%d:%d", record.Offset, end), SourceEventQuality: executionmodel.IdentityDerived,
				SourceSeq: sourceSeq, Phase: executionmodel.MessagePhaseUnknown, ReceivedAt: now(),
				Evidence: map[string]string{"source_record": "events.jsonl", "byte_start": fmt.Sprint(record.Offset), "byte_end": fmt.Sprint(end), "identity_rule": "source-file-byte-range-v1", "replay": "true"},
			}
			if record.Oversized {
				base.Type, base.Status, base.Diagnostic = executionmodel.ConversationGap, "gap", fmt.Sprintf("source record exceeds %d bytes", JournalMaxRecord)
				writes = append(writes, executionmodel.ConversationWrite{Event: base})
				continue
			}
			raw := append(append([]byte(nil), record.Data...), '\n')
			mapped := mapper.MapRecord(base, raw)
			writes = append(writes, mapped)
		}
		if len(writes) > 0 {
			position, positionErr := currentConversationJournalPosition(ctx, s.conversationStore, source.ProjectID, source.ExecutionID, source.AttemptID, journalSource, segment, batch.Cursor.Generation, source.Path, source.FileID, batch.Cursor.Offset, sourceSeq)
			if positionErr != nil {
				return positionErr
			}
			checkpointCursor, positionErr := marshalConversationJournalPosition(position)
			if positionErr != nil {
				return positionErr
			}
			checkpoint := executionmodel.ConversationCheckpoint{ProjectID: source.ProjectID, ExecutionID: source.ExecutionID, AttemptID: source.AttemptID, Source: journalSource, SourceSegment: segment, SourceIdentity: source.FileID, Generation: generation, Cursor: checkpointCursor, LocalSeq: sourceSeq, AllowReplay: true, UpdatedAt: now()}
			if _, err = s.conversationStore.CommitConversation(ctx, writes, checkpoint); err != nil {
				return err
			}
		}
		if !batch.More {
			if source.Ended && len(batch.Cursor.Pending) > 0 {
				if err = s.replayConversationTail(ctx, source, segment, generation, mapper, batch.Cursor.Pending, batch.Cursor.ScanOffset-int64(len(batch.Cursor.Pending)), sourceSeq); err != nil {
					return err
				}
			}
			if !batch.Cursor.Dropping {
				return nil
			}
			if source.Ended {
				return s.replayConversationTailGap(ctx, source, segment, generation, batch.Cursor.DroppingStart, batch.Cursor.ScanOffset, sourceSeq+1)
			}
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (s *Service) replayConversationTail(ctx context.Context, source conversationJournalSource, segment, generation string, mapper ConversationMapper, data []byte, start int64, oldSeq uint64) error {
	journalSource := s.conversationSources.JournalSource(source.Backend, source.Protocol)
	sequence := oldSeq + 1
	raw := append([]byte(nil), data...)
	end := start + int64(len(raw))
	positionGeneration, err := s.conversationSources.JournalGeneration(source.FileID, segment)
	if err != nil {
		return err
	}
	position, err := currentConversationJournalPosition(ctx, s.conversationStore, source.ProjectID, source.ExecutionID, source.AttemptID, journalSource, segment, positionGeneration, source.Path, source.FileID, end, sequence)
	if err != nil {
		return err
	}
	cursor, err := marshalConversationJournalPosition(position)
	if err != nil {
		return err
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		event := executionmodel.ConversationEvent{
			Version: executionmodel.ConversationVersion, ProjectID: source.ProjectID, TaskID: source.TaskID,
			WorkflowID: source.WorkflowID, ExecutionID: source.ExecutionID, AttemptID: source.AttemptID,
			Backend: source.Backend, Protocol: source.Protocol, Agent: source.Agent, TargetID: source.TargetID,
			ExternalRef: source.ExternalRef, Source: journalSource, SourceRole: executionmodel.ConversationSourceFallback, SourceSegment: segment,
			SourceIdentity: source.FileID, SourceCursor: string(cursor), SessionQuality: executionmodel.IdentityUnknown,
			TurnQuality: executionmodel.IdentityUnknown, ItemQuality: executionmodel.IdentityUnknown, SourceEventID: fmt.Sprintf("bytes:%d:%d", start, end),
			SourceEventQuality: executionmodel.IdentityDerived, SourceSeq: sequence, Type: executionmodel.ConversationGap, Phase: executionmodel.MessagePhaseUnknown,
			Status: "gap", ReceivedAt: now(), Diagnostic: "unterminated final source row is not a complete JSON object",
			Evidence: map[string]string{"source_record": "events.jsonl", "byte_start": fmt.Sprint(start), "byte_end": fmt.Sprint(end), "replay": "true", "unterminated_final_record": "true"},
		}
		checkpoint := executionmodel.ConversationCheckpoint{ProjectID: source.ProjectID, ExecutionID: source.ExecutionID, AttemptID: source.AttemptID, Source: event.Source, SourceSegment: segment, SourceIdentity: source.FileID, Generation: generation, Cursor: string(cursor), LocalSeq: sequence, AllowReplay: true, UpdatedAt: now()}
		_, err := s.conversationStore.CommitConversation(ctx, []executionmodel.ConversationWrite{{Event: event, Raw: raw}}, checkpoint)
		return err
	}
	base := executionmodel.ConversationEvent{
		Version: executionmodel.ConversationVersion, ProjectID: source.ProjectID, TaskID: source.TaskID,
		WorkflowID: source.WorkflowID, ExecutionID: source.ExecutionID, AttemptID: source.AttemptID,
		Backend: source.Backend, Protocol: source.Protocol, Agent: source.Agent, TargetID: source.TargetID,
		ExternalRef: source.ExternalRef, Source: journalSource, SourceRole: executionmodel.ConversationSourceFallback, SourceSegment: segment,
		SourceIdentity: source.FileID, SourceCursor: string(cursor), SessionQuality: executionmodel.IdentityUnknown,
		TurnQuality: executionmodel.IdentityUnknown, ItemQuality: executionmodel.IdentityUnknown, SourceEventID: fmt.Sprintf("bytes:%d:%d", start, end),
		SourceEventQuality: executionmodel.IdentityDerived, SourceSeq: sequence, Phase: executionmodel.MessagePhaseUnknown, ReceivedAt: now(),
		Evidence: map[string]string{"source_record": "events.jsonl", "byte_start": fmt.Sprint(start), "byte_end": fmt.Sprint(end), "identity_rule": "source-file-byte-range-v1", "replay": "true", "unterminated_final_record": "true"},
	}
	write := mapper.MapRecord(base, raw)
	checkpoint := executionmodel.ConversationCheckpoint{ProjectID: source.ProjectID, ExecutionID: source.ExecutionID, AttemptID: source.AttemptID, Source: journalSource, SourceSegment: segment, SourceIdentity: source.FileID, Generation: generation, Cursor: string(cursor), LocalSeq: sequence, AllowReplay: true, UpdatedAt: now()}
	_, err = s.conversationStore.CommitConversation(ctx, []executionmodel.ConversationWrite{write}, checkpoint)
	return err
}

func (s *Service) replayConversationTailGap(ctx context.Context, source conversationJournalSource, segment, generation string, start, end int64, sequence uint64) error {
	journalSource := s.conversationSources.JournalSource(source.Backend, source.Protocol)
	if end < start {
		return errors.New("incomplete conversation source range is invalid")
	}
	positionGeneration, err := s.conversationSources.JournalGeneration(source.FileID, segment)
	if err != nil {
		return err
	}
	position, err := currentConversationJournalPosition(ctx, s.conversationStore, source.ProjectID, source.ExecutionID, source.AttemptID, journalSource, segment, positionGeneration, source.Path, source.FileID, end, sequence)
	if err != nil {
		return err
	}
	cursor, err := marshalConversationJournalPosition(position)
	if err != nil {
		return err
	}
	event := executionmodel.ConversationEvent{
		Version: executionmodel.ConversationVersion, ProjectID: source.ProjectID, TaskID: source.TaskID,
		WorkflowID: source.WorkflowID, ExecutionID: source.ExecutionID, AttemptID: source.AttemptID,
		Backend: source.Backend, Protocol: source.Protocol, Agent: source.Agent, TargetID: source.TargetID,
		ExternalRef: source.ExternalRef, Source: journalSource, SourceRole: executionmodel.ConversationSourceFallback, SourceSegment: segment,
		SourceIdentity: source.FileID, SourceCursor: string(cursor), SessionQuality: executionmodel.IdentityUnknown,
		TurnQuality: executionmodel.IdentityUnknown, ItemQuality: executionmodel.IdentityUnknown, SourceEventID: fmt.Sprintf("bytes:%d:%d", start, end),
		SourceEventQuality: executionmodel.IdentityDerived, SourceSeq: sequence, Type: executionmodel.ConversationGap, Phase: executionmodel.MessagePhaseUnknown,
		Status: "gap", ReceivedAt: now(), Diagnostic: "unterminated oversized source record at end of a completed attempt",
		Evidence: map[string]string{"source_record": "events.jsonl", "byte_start": fmt.Sprint(start), "byte_end": fmt.Sprint(end), "replay": "true"},
	}
	checkpoint := executionmodel.ConversationCheckpoint{ProjectID: source.ProjectID, ExecutionID: source.ExecutionID, AttemptID: source.AttemptID, Source: event.Source, SourceSegment: segment, SourceIdentity: source.FileID, Generation: generation, Cursor: string(cursor), LocalSeq: sequence, AllowReplay: true, UpdatedAt: now()}
	_, err = s.conversationStore.CommitConversation(ctx, []executionmodel.ConversationWrite{{Event: event}}, checkpoint)
	return err
}

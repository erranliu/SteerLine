package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

type conversationJournalSource struct {
	Path           string
	RunID          string
	TaskID         string
	WorkflowID     string
	ExecutionID    string
	AttemptID      string
	Backend        BackendKind
	Protocol       ExecutionProtocol
	Agent          string
	TargetID       string
	ExternalRef    string
	ProjectID      string
	Ended          bool
	FileID         string
	ExpectedFileID string
	Diagnostic     string
	Workspace      string
	ReaderCommand  Command
	CodexHome      string
}

func (s *Service) conversationJournalSources() []conversationJournalSource {
	sources := []conversationJournalSource{}
	seen := map[string]bool{}
	for _, run := range s.State.Runs {
		if run == nil || run.RunID == "" || !(run.Backend == string(BackendLocal) && run.Protocol == string(ProtocolLocalProcess) || run.WorkflowVersion == WorkerFlowVersion && run.Backend == string(BackendOrca) && (run.Protocol == string(ProtocolOrcaTerminalV2) || run.Protocol == string(ProtocolOrcaSupervised))) {
			continue
		}
		root := filepath.Join(s.Config.RunDir, "runs", run.RunID)
		add := func(path, attempt string) {
			if path == "" || !stableExecutionID(attempt) || !stableExecutionID(run.ExecutionID) || run.DomainTaskID == "" {
				return
			}
			clean := filepath.Clean(path)
			if filepath.Base(clean) != "events.jsonl" || !inside(root, clean) || seen[clean] {
				return
			}
			markerPath := filepath.Join(filepath.Dir(clean), ConversationSourceMarkerName)
			markerBytes, markerErr := os.ReadFile(markerPath)
			if os.IsNotExist(markerErr) {
				// Raw logs without the current source marker belong to an earlier
				// development format; leave them available to the existing raw reader.
				return
			}
			f, fileID, err := openJournal(clean)
			if err != nil {
				return
			}
			_ = f.Close()
			seen[clean] = true
			source := conversationJournalSource{
				Path: clean, RunID: run.RunID, TaskID: run.DomainTaskID, WorkflowID: run.RunID,
				ExecutionID: run.ExecutionID, AttemptID: attempt, Backend: BackendKind(run.Backend),
				Protocol: ExecutionProtocol(run.Protocol), Agent: run.Agent, TargetID: run.Target.ID,
				ExternalRef: conversationExternalRef(run.Process), ProjectID: s.Config.identity,
				Ended: run.EndedAt != nil, FileID: fileID,
				Workspace: deref(run.WorktreePath), ReaderCommand: codexReaderCommand(s.Config),
			}
			if markerErr != nil {
				source.Diagnostic = fmt.Sprintf("source marker is unreadable: %v", markerErr)
			} else {
				var marker ConversationSourceMarker
				if decodeErr := json.Unmarshal(markerBytes, &marker); decodeErr != nil {
					source.Diagnostic = fmt.Sprintf("source marker is invalid: %v", decodeErr)
				} else if validateErr := marker.validate(); validateErr != nil {
					source.Diagnostic = validateErr.Error()
				} else if marker.ProjectID != s.Config.identity || marker.WorkflowID != run.RunID || marker.ExecutionID != run.ExecutionID || marker.AttemptID != attempt || marker.Backend != BackendKind(run.Backend) || marker.Protocol != ExecutionProtocol(run.Protocol) || marker.Agent != run.Agent || marker.TargetID != run.Target.ID {
					source.Diagnostic = "source marker identity does not match the fixed run and attempt"
				} else {
					source.ExpectedFileID = marker.SourceIdentity
					if marker.ReaderCommand.Executable != "" {
						source.ReaderCommand = marker.ReaderCommand
					}
					source.CodexHome = marker.CodexHome
				}
			}
			sources = append(sources, source)
		}
		for _, attempt := range s.State.Attempts {
			if run.ExecutionID != "" && attempt.ExecutionID == run.ExecutionID && attempt.EventPath != "" {
				add(attempt.EventPath, attempt.AttemptID)
			}
		}
		if run.WorkflowVersion == WorkerFlowVersion {
			turnRoot := filepath.Join(root, "turns")
			if entries, err := os.ReadDir(turnRoot); err == nil && len(entries) <= 4096 {
				for _, entry := range entries {
					if entry.IsDir() && stableExecutionID(entry.Name()) {
						add(filepath.Join(turnRoot, entry.Name(), "events.jsonl"), run.AttemptID)
					}
				}
			}
		}
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

func (s *Service) reconcileConversationRun(run *Run) error {
	if run == nil || !(run.Backend == string(BackendLocal) && run.Protocol == string(ProtocolLocalProcess) || run.WorkflowVersion == WorkerFlowVersion && run.Backend == string(BackendOrca) && (run.Protocol == string(ProtocolOrcaTerminalV2) || run.Protocol == string(ProtocolOrcaSupervised))) {
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
	mapper := &ConversationMapper{}
	var sourceSeq uint64
	checkpoints, err := s.domainStore.ConversationSourceCheckpoints(ctx, source.ProjectID, source.ExecutionID, source.AttemptID, "codex-jsonl")
	if err != nil {
		return err
	}
	segmentPrefix := "local-jsonl-v1:"
	if source.Backend == BackendOrca {
		segmentPrefix = "orca-jsonl-v1:"
	}
	segment := segmentPrefix + source.FileID
	generation := source.FileID
	knownSegment := false
	var prior *ConversationCheckpoint
	var matching *ConversationCheckpoint
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
		position, decodeErr := decodeConversationJournalPosition(*matching)
		if decodeErr != nil {
			return decodeErr
		}
		readCursor = position.journalCursor(source.Path)
		if stat, statErr := os.Stat(source.Path); statErr == nil && stat.Size() == position.FileSize && stat.ModTime().UnixNano() != position.ModifiedNS && position.ByteOffset > 0 {
			// Same-size rewrites can preserve both byte anchors. Force the
			// existing journal reset path to open a visible source_changed segment.
			readCursor.HeaderHash = "modified-since-checkpoint"
		}
		sourceSeq = position.SourceSeq
		mapper, err = s.domainStore.RestoreConversationMapper(ctx, source.ProjectID, source.ExecutionID, source.AttemptID, "codex-jsonl", segment)
		if err != nil {
			return err
		}
	}
	if !knownSegment && (prior != nil || source.ExpectedFileID != "" && source.ExpectedFileID != source.FileID) {
		previousIdentity := source.ExpectedFileID
		if previousIdentity == "" && prior != nil {
			previousIdentity = prior.SourceIdentity
		}
		gap := ConversationEvent{
			Version: ConversationVersion, ProjectID: source.ProjectID, TaskID: source.TaskID,
			WorkflowID: source.WorkflowID, ExecutionID: source.ExecutionID, AttemptID: source.AttemptID,
			Backend: source.Backend, Protocol: source.Protocol, Agent: source.Agent, TargetID: source.TargetID,
			ExternalRef: source.ExternalRef, Source: "codex-jsonl", SourceSegment: segment,
			SourceIdentity: source.FileID, SessionQuality: IdentityUnknown, TurnQuality: IdentityUnknown,
			ItemQuality: IdentityUnknown, SourceEventID: "source-changed:" + previousIdentity + ":" + source.FileID,
			SourceEventQuality: IdentityDerived, Type: ConversationGap, Phase: MessagePhaseUnknown,
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
		checkpoint := ConversationCheckpoint{ProjectID: source.ProjectID, ExecutionID: source.ExecutionID, AttemptID: source.AttemptID, Source: gap.Source, SourceSegment: segment, SourceIdentity: source.FileID, Generation: generation, Cursor: checkpointCursor, UpdatedAt: now()}
		if _, err = s.domainStore.CommitConversation(ctx, []ConversationWrite{{Event: gap}}, checkpoint); err != nil {
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
			segment = segmentPrefix + generation
			mapper, sourceSeq = &ConversationMapper{}, 0
			gap := ConversationEvent{
				Version: ConversationVersion, ProjectID: source.ProjectID, TaskID: source.TaskID,
				WorkflowID: source.WorkflowID, ExecutionID: source.ExecutionID, AttemptID: source.AttemptID,
				Backend: source.Backend, Protocol: source.Protocol, Agent: source.Agent, TargetID: source.TargetID,
				ExternalRef: source.ExternalRef, Source: "codex-jsonl", SourceSegment: segment,
				SourceIdentity: source.FileID, SessionQuality: IdentityUnknown, TurnQuality: IdentityUnknown,
				ItemQuality: IdentityUnknown, SourceEventID: "source-changed:" + previousIdentity + ":" + source.FileID,
				SourceEventQuality: IdentityDerived, Type: ConversationGap, Phase: MessagePhaseUnknown,
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
			checkpoint := ConversationCheckpoint{ProjectID: source.ProjectID, ExecutionID: source.ExecutionID, AttemptID: source.AttemptID, Source: gap.Source, SourceSegment: segment, SourceIdentity: source.FileID, Generation: generation, Cursor: checkpointCursor, UpdatedAt: now()}
			if _, err = s.domainStore.CommitConversation(ctx, []ConversationWrite{{Event: gap}}, checkpoint); err != nil {
				return err
			}
		}
		readCursor = batch.Cursor
		writes := make([]ConversationWrite, 0, len(batch.Records))
		for _, record := range batch.Records {
			sourceSeq++
			end := record.Offset + record.Length
			cursor, _ := json.Marshal(struct {
				ByteOffset int64  `json:"byte_offset"`
				ByteLength int64  `json:"byte_length"`
				SourceSeq  uint64 `json:"source_seq"`
			}{end, record.Length, sourceSeq})
			base := ConversationEvent{
				Version: ConversationVersion, ProjectID: source.ProjectID, TaskID: source.TaskID,
				WorkflowID: source.WorkflowID, ExecutionID: source.ExecutionID, AttemptID: source.AttemptID,
				Backend: source.Backend, Protocol: source.Protocol, Agent: source.Agent, TargetID: source.TargetID,
				ExternalRef: source.ExternalRef, Source: "codex-jsonl", SourceSegment: segment,
				SourceIdentity: source.FileID, SourceCursor: string(cursor),
				SessionQuality: IdentityUnknown, TurnQuality: IdentityUnknown, ItemQuality: IdentityUnknown,
				SourceEventID: fmt.Sprintf("bytes:%d:%d", record.Offset, end), SourceEventQuality: IdentityDerived,
				SourceSeq: sourceSeq, Phase: MessagePhaseUnknown, ReceivedAt: now(),
				Evidence: map[string]string{"source_record": "events.jsonl", "byte_start": fmt.Sprint(record.Offset), "byte_end": fmt.Sprint(end), "identity_rule": "source-file-byte-range-v1", "replay": "true"},
			}
			if record.Oversized {
				base.Type, base.Status, base.Diagnostic = ConversationGap, "gap", fmt.Sprintf("source record exceeds %d bytes", JournalMaxRecord)
				writes = append(writes, ConversationWrite{Event: base})
				continue
			}
			raw := append(append([]byte(nil), record.Data...), '\n')
			mapped := mapper.MapCodexJSONL(base, raw)
			if mapped.Event.Type == ConversationSession && codexThreadID.MatchString(mapped.Event.SessionID) {
				binding := codexHistoryBinding{Version: 1, ProjectID: source.ProjectID, TaskID: source.TaskID, WorkflowID: source.WorkflowID, ExecutionID: source.ExecutionID, AttemptID: source.AttemptID, Backend: source.Backend, Protocol: source.Protocol, TargetID: source.TargetID, ExternalRef: source.ExternalRef, Workspace: source.Workspace, SessionID: mapped.Event.SessionID, Command: source.ReaderCommand, Home: source.CodexHome}
				if err = writeCodexHistoryBinding(localCodexHistoryBindingPath(filepath.Dir(source.Path), binding.SessionID), binding); err != nil {
					return err
				}
			}
			writes = append(writes, mapped)
		}
		if len(writes) > 0 {
			position, positionErr := currentConversationJournalPosition(ctx, s.domainStore, source.ProjectID, source.ExecutionID, source.AttemptID, "codex-jsonl", segment, source.Path, source.FileID, batch.Cursor.Offset, sourceSeq)
			if positionErr != nil {
				return positionErr
			}
			checkpointCursor, positionErr := marshalConversationJournalPosition(position)
			if positionErr != nil {
				return positionErr
			}
			checkpoint := ConversationCheckpoint{ProjectID: source.ProjectID, ExecutionID: source.ExecutionID, AttemptID: source.AttemptID, Source: "codex-jsonl", SourceSegment: segment, SourceIdentity: source.FileID, Generation: generation, Cursor: checkpointCursor, LocalSeq: sourceSeq, AllowReplay: true, UpdatedAt: now()}
			if _, err = s.domainStore.CommitConversation(ctx, writes, checkpoint); err != nil {
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

func (s *Service) replayConversationTail(ctx context.Context, source conversationJournalSource, segment, generation string, mapper *ConversationMapper, data []byte, start int64, oldSeq uint64) error {
	sequence := oldSeq + 1
	raw := append([]byte(nil), data...)
	end := start + int64(len(raw))
	position, err := currentConversationJournalPosition(ctx, s.domainStore, source.ProjectID, source.ExecutionID, source.AttemptID, "codex-jsonl", segment, source.Path, source.FileID, end, sequence)
	if err != nil {
		return err
	}
	cursor, err := marshalConversationJournalPosition(position)
	if err != nil {
		return err
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		event := ConversationEvent{
			Version: ConversationVersion, ProjectID: source.ProjectID, TaskID: source.TaskID,
			WorkflowID: source.WorkflowID, ExecutionID: source.ExecutionID, AttemptID: source.AttemptID,
			Backend: source.Backend, Protocol: source.Protocol, Agent: source.Agent, TargetID: source.TargetID,
			ExternalRef: source.ExternalRef, Source: "codex-jsonl", SourceSegment: segment,
			SourceIdentity: source.FileID, SourceCursor: string(cursor), SessionQuality: IdentityUnknown,
			TurnQuality: IdentityUnknown, ItemQuality: IdentityUnknown, SourceEventID: fmt.Sprintf("bytes:%d:%d", start, end),
			SourceEventQuality: IdentityDerived, SourceSeq: sequence, Type: ConversationGap, Phase: MessagePhaseUnknown,
			Status: "gap", ReceivedAt: now(), Diagnostic: "unterminated final source row is not a complete JSON object",
			Evidence: map[string]string{"source_record": "events.jsonl", "byte_start": fmt.Sprint(start), "byte_end": fmt.Sprint(end), "replay": "true", "unterminated_final_record": "true"},
		}
		checkpoint := ConversationCheckpoint{ProjectID: source.ProjectID, ExecutionID: source.ExecutionID, AttemptID: source.AttemptID, Source: event.Source, SourceSegment: segment, SourceIdentity: source.FileID, Generation: generation, Cursor: string(cursor), LocalSeq: sequence, AllowReplay: true, UpdatedAt: now()}
		_, err := s.domainStore.CommitConversation(ctx, []ConversationWrite{{Event: event, Raw: raw}}, checkpoint)
		return err
	}
	base := ConversationEvent{
		Version: ConversationVersion, ProjectID: source.ProjectID, TaskID: source.TaskID,
		WorkflowID: source.WorkflowID, ExecutionID: source.ExecutionID, AttemptID: source.AttemptID,
		Backend: source.Backend, Protocol: source.Protocol, Agent: source.Agent, TargetID: source.TargetID,
		ExternalRef: source.ExternalRef, Source: "codex-jsonl", SourceSegment: segment,
		SourceIdentity: source.FileID, SourceCursor: string(cursor), SessionQuality: IdentityUnknown,
		TurnQuality: IdentityUnknown, ItemQuality: IdentityUnknown, SourceEventID: fmt.Sprintf("bytes:%d:%d", start, end),
		SourceEventQuality: IdentityDerived, SourceSeq: sequence, Phase: MessagePhaseUnknown, ReceivedAt: now(),
		Evidence: map[string]string{"source_record": "events.jsonl", "byte_start": fmt.Sprint(start), "byte_end": fmt.Sprint(end), "identity_rule": "source-file-byte-range-v1", "replay": "true", "unterminated_final_record": "true"},
	}
	write := mapper.MapCodexJSONL(base, raw)
	checkpoint := ConversationCheckpoint{ProjectID: source.ProjectID, ExecutionID: source.ExecutionID, AttemptID: source.AttemptID, Source: "codex-jsonl", SourceSegment: segment, SourceIdentity: source.FileID, Generation: generation, Cursor: string(cursor), LocalSeq: sequence, AllowReplay: true, UpdatedAt: now()}
	_, err = s.domainStore.CommitConversation(ctx, []ConversationWrite{write}, checkpoint)
	return err
}

func (s *Service) replayConversationTailGap(ctx context.Context, source conversationJournalSource, segment, generation string, start, end int64, sequence uint64) error {
	if end < start {
		return errors.New("incomplete conversation source range is invalid")
	}
	position, err := currentConversationJournalPosition(ctx, s.domainStore, source.ProjectID, source.ExecutionID, source.AttemptID, "codex-jsonl", segment, source.Path, source.FileID, end, sequence)
	if err != nil {
		return err
	}
	cursor, err := marshalConversationJournalPosition(position)
	if err != nil {
		return err
	}
	event := ConversationEvent{
		Version: ConversationVersion, ProjectID: source.ProjectID, TaskID: source.TaskID,
		WorkflowID: source.WorkflowID, ExecutionID: source.ExecutionID, AttemptID: source.AttemptID,
		Backend: source.Backend, Protocol: source.Protocol, Agent: source.Agent, TargetID: source.TargetID,
		ExternalRef: source.ExternalRef, Source: "codex-jsonl", SourceSegment: segment,
		SourceIdentity: source.FileID, SourceCursor: string(cursor), SessionQuality: IdentityUnknown,
		TurnQuality: IdentityUnknown, ItemQuality: IdentityUnknown, SourceEventID: fmt.Sprintf("bytes:%d:%d", start, end),
		SourceEventQuality: IdentityDerived, SourceSeq: sequence, Type: ConversationGap, Phase: MessagePhaseUnknown,
		Status: "gap", ReceivedAt: now(), Diagnostic: "unterminated oversized source record at end of a completed attempt",
		Evidence: map[string]string{"source_record": "events.jsonl", "byte_start": fmt.Sprint(start), "byte_end": fmt.Sprint(end), "replay": "true"},
	}
	checkpoint := ConversationCheckpoint{ProjectID: source.ProjectID, ExecutionID: source.ExecutionID, AttemptID: source.AttemptID, Source: event.Source, SourceSegment: segment, SourceIdentity: source.FileID, Generation: generation, Cursor: string(cursor), LocalSeq: sequence, AllowReplay: true, UpdatedAt: now()}
	_, err = s.domainStore.CommitConversation(ctx, []ConversationWrite{{Event: event}}, checkpoint)
	return err
}

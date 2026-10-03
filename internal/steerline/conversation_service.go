package steerline

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

func (s *Service) recordConversationEvent(run *Run, source WorkerEvent) error {
	if run == nil || len(source.Raw) == 0 && !source.SourceGap && !source.InputWait || s.conversationStore == nil {
		return nil
	}
	if !executionapi.StableExecutionID(run.ExecutionID) {
		return nativeErr("contract_mismatch", "conversation event has no fixed execution identity")
	}
	attemptID := source.AttemptID
	if attemptID == "" {
		attemptID = run.AttemptID
	}
	if !executionapi.StableExecutionID(attemptID) {
		return nativeErr("contract_mismatch", "conversation event has no fixed attempt identity")
	}
	base := ConversationEvent{Version: ConversationVersion, ProjectID: s.Config.identity, TaskID: run.DomainTaskID, WorkflowID: run.RunID,
		ExecutionID: run.ExecutionID, AttemptID: attemptID, Backend: executionapi.BackendKind(run.Backend), Protocol: executionapi.ExecutionProtocol(run.Protocol),
		Agent: run.Agent, TargetID: run.Target.ID, SourceEventID: source.SourceEventID, SessionQuality: IdentityUnknown, TurnQuality: IdentityUnknown, ItemQuality: IdentityUnknown}
	input := contracts.ConversationInput{Event: base, Process: run.Process, Source: source.Source, Segment: source.SourceSegment, Identity: source.SourceIdentity,
		Cursor: source.SourceCursor, Generation: processGeneration(run.Process), Session: source.Session, SourceSeq: source.SourceSeq,
		SourceOffset: source.SourceOffset, SourceLength: source.SourceLength, Raw: append([]byte(nil), source.Raw...), Diagnostic: source.Diagnostic,
		Gap: source.SourceGap, InputWait: source.InputWait, SourceChanged: source.SourceChanged}
	if input.Process != nil {
		process := *input.Process
		input.Process = &process
	}
	if s.conversationSources == nil {
		return errors.New("conversation source port is required")
	}
	journalSource := s.conversationSources.JournalSource(base.Backend, base.Protocol)
	segment := source.SourceSegment
	if segment == "" {
		segment = s.conversationSources.JournalSegment(base.Backend, base.Protocol, source.SourceIdentity)
	}
	if journalSource != "" && segment != "" {
		snapshot, snapshotErr := s.conversationStore.ConversationMapperSnapshot(context.Background(), s.Config.identity, run.ExecutionID, attemptID, journalSource, segment)
		if snapshotErr != nil {
			return snapshotErr
		}
		input.Mapper = snapshot
	}
	var mapper contracts.ConversationInputMapper = s.conversationSources
	if lifecycle, _, lifecycleErr := s.lifecycleForRun(run); lifecycleErr == nil {
		if implementationMapper, ok := lifecycle.(contracts.ConversationInputMapper); ok {
			mapper = implementationMapper
		}
	} else if run.WorkflowVersion == WorkerFlowVersion {
		return lifecycleErr
	}
	batch, err := mapper.MapConversation(input)
	if err != nil {
		return err
	}
	if len(batch.Writes) == 0 {
		return nil
	}
	if err = validateConversationSourceWrites(base, batch.Writes); err != nil {
		return err
	}
	checkpoint := batch.Checkpoint
	if checkpoint.ProjectID != base.ProjectID || checkpoint.ExecutionID != base.ExecutionID || checkpoint.AttemptID != base.AttemptID {
		return errors.New("conversation mapper checkpoint changed the original execution identity")
	}
	if batch.VerifySourcePosition && source.SourceIdentity != "" {
		paths := []string{}
		for _, attempt := range s.domain.Attempts {
			if attempt.AttemptID == attemptID && attempt.ExecutionID == run.ExecutionID && attempt.EventPath != "" {
				paths = append(paths, attempt.EventPath)
			}
		}
		path, pathErr := s.conversationSources.LiveJournalPath(context.Background(), contracts.ConversationJournalPathRequest{
			RunDirectory: filepath.Join(s.Config.RunDir, "runs", run.RunID), WorkflowID: run.RunID, ExecutionID: run.ExecutionID, AttemptID: attemptID,
			SourceIdentity: source.SourceIdentity, LogPath: deref(run.LogPath), EventPaths: paths, WorkerFlow: run.WorkflowVersion == WorkerFlowVersion})
		if pathErr != nil {
			return pathErr
		}
		event := batch.Writes[len(batch.Writes)-1].Event
		generation, generationErr := s.conversationSources.JournalGeneration(source.SourceIdentity, event.SourceSegment)
		if generationErr != nil {
			return generationErr
		}
		position, positionErr := currentConversationJournalPosition(context.Background(), s.conversationStore, s.Config.identity, run.ExecutionID, attemptID, event.Source, event.SourceSegment, generation, path, source.SourceIdentity, source.SourceOffset+source.SourceLength, source.SourceSeq)
		if positionErr == nil && !source.SourceGap && len(source.Raw) > 0 {
			positionErr = verifyConversationSourceBytes(path, source.SourceIdentity, source.SourceOffset, source.SourceLength, source.Raw)
		}
		if positionErr != nil {
			gap := event
			gap.Type, gap.Phase, gap.Status = ConversationGap, MessagePhaseUnknown, "source_unavailable"
			gap.SourceEventID = "source-unavailable:" + event.SourceEventID
			gap.Diagnostic = positionErr.Error()
			gap.SessionQuality, gap.TurnQuality, gap.ItemQuality = IdentityUnknown, IdentityUnknown, IdentityUnknown
			if _, gapErr := s.conversationStore.CommitConversation(context.Background(), []ConversationWrite{{Event: gap, Raw: append([]byte(nil), source.Raw...)}}, ConversationCheckpoint{}); gapErr != nil {
				return errors.Join(positionErr, gapErr)
			}
			return positionErr
		}
		checkpoint.Cursor, positionErr = marshalConversationJournalPosition(position)
		if positionErr != nil {
			return positionErr
		}
		checkpoint.SourceIdentity, checkpoint.Generation, checkpoint.LocalSeq, checkpoint.AllowReplay = source.SourceIdentity, source.SourceIdentity, source.SourceSeq, true
	}
	_, err = s.conversationStore.CommitConversation(context.Background(), batch.Writes, checkpoint)
	return err
}

func verifyConversationSourceBytes(path, expectedFileID string, offset, length int64, raw []byte) error {
	if offset < 0 || length != int64(len(raw)) {
		return errors.New("conversation source byte range is invalid")
	}
	f, fileID, err := openJournal(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if fileID != expectedFileID {
		return errors.New("conversation source file changed before committing its record")
	}
	stored := make([]byte, len(raw))
	if _, err = f.ReadAt(stored, offset); err != nil {
		return err
	}
	if !bytes.Equal(stored, raw) {
		return errors.New("conversation source bytes changed before committing their checkpoint")
	}
	return nil
}

func processGeneration(process *executionapi.ProcessIdentity) string {
	if process == nil {
		return ""
	}
	return process.Incarnation
}

func conversationProcessSnapshot(process *executionapi.ProcessIdentity) *executionapi.ProcessIdentity {
	if process == nil {
		return nil
	}
	identity := *process
	return &identity
}

func validateConversationSourceWrites(expected ConversationEvent, writes []ConversationWrite) error {
	for _, write := range writes {
		event := write.Event
		if event.ProjectID != expected.ProjectID || event.TaskID != expected.TaskID || event.WorkflowID != expected.WorkflowID || event.ExecutionID != expected.ExecutionID || event.AttemptID != expected.AttemptID || event.Backend != expected.Backend || event.Protocol != expected.Protocol || event.Agent != expected.Agent || event.TargetID != expected.TargetID {
			return errors.New("conversation source changed its fixed project, execution or attempt owner")
		}
		if expected.Source != "" && (event.Source != expected.Source || event.SourceSegment != expected.SourceSegment || event.SourceIdentity != expected.SourceIdentity) {
			return errors.New("conversation source changed its discovered source identity")
		}
	}
	return nil
}

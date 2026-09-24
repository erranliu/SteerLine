package steerline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

func (s *Service) recordConversationEvent(run *Run, source WorkerEvent) (retErr error) {
	if run == nil || len(source.Raw) == 0 && !source.SourceGap && !source.InputWait || s.domainStore == nil {
		return nil
	}
	if source.Source == "orca-terminal-v2" {
		return s.recordOrcaTerminalEvent(run, source)
	}
	if err := s.bindLocalCodexHistory(run, source); err != nil {
		return err
	}
	if source.Source != "" && source.Source != "codex-jsonl" {
		return nil
	}
	executionID := run.ExecutionID
	if executionID == "" {
		executionID = legacyExecutionID(run.RunID)
	}
	attemptID := source.AttemptID
	if attemptID == "" {
		attemptID = run.AttemptID
	}
	if attemptID == "" {
		attemptID = run.RunID
	}
	segment := nonEmpty(source.SourceSegment, "local-jsonl-v1:"+source.SourceIdentity)
	mapperKey := run.RunID + ":" + attemptID + ":" + segment
	if s.conversationMappers == nil {
		s.conversationMappers = map[string]*ConversationMapper{}
	}
	mapper := s.conversationMappers[mapperKey]
	if mapper == nil {
		var err error
		mapper, err = s.domainStore.RestoreConversationMapper(context.Background(), s.Config.identity, executionID, attemptID, "codex-jsonl", segment)
		if err != nil {
			return err
		}
		s.conversationMappers[mapperKey] = mapper
	}
	previousMapper := *mapper
	if mapper.turnFinalSeen != nil {
		previousMapper.turnFinalSeen = make(map[string]bool, len(mapper.turnFinalSeen))
		for key, seen := range mapper.turnFinalSeen {
			previousMapper.turnFinalSeen[key] = seen
		}
	}
	defer func() {
		if retErr != nil {
			*mapper = previousMapper
		}
	}()
	checkpointCursor, err := json.Marshal(struct {
		ByteOffset int64  `json:"byte_offset"`
		ByteLength int64  `json:"byte_length"`
		SourceSeq  uint64 `json:"source_seq"`
	}{source.SourceOffset, source.SourceLength, source.SourceSeq})
	if err != nil {
		return err
	}
	sourceIdentity := nonEmpty(source.SourceIdentity, "local-events:"+run.RunID+":"+attemptID)
	evidence := map[string]string{
		"source_record": "events.jsonl",
		"byte_start":    fmt.Sprint(source.SourceOffset),
		"byte_end":      fmt.Sprint(source.SourceOffset + source.SourceLength),
		"identity_rule": "source-file-byte-range-v1",
	}
	if source.Diagnostic != "" {
		evidence["collector_diagnostic"] = source.Diagnostic
	}
	event := ConversationEvent{
		Version: ConversationVersion, ProjectID: s.Config.identity, TaskID: run.DomainTaskID,
		WorkflowID: run.RunID, ExecutionID: executionID, AttemptID: attemptID,
		Backend: BackendKind(run.Backend), Protocol: ExecutionProtocol(run.Protocol), Agent: run.Agent,
		TargetID: run.Target.ID, ExternalRef: conversationExternalRef(run.Process),
		Source: "codex-jsonl", SourceSegment: segment, SourceIdentity: sourceIdentity, SourceCursor: string(checkpointCursor), SessionQuality: IdentityUnknown,
		TurnQuality: IdentityUnknown, ItemQuality: IdentityUnknown, SourceEventQuality: IdentityDerived,
		SourceEventID: fmt.Sprintf("bytes:%d:%d", source.SourceOffset, source.SourceOffset+source.SourceLength),
		SourceSeq:     source.SourceSeq, ReceivedAt: now(), Evidence: evidence,
	}
	var write ConversationWrite
	if source.SourceGap {
		event.Type, event.Phase, event.Status = ConversationGap, MessagePhaseUnknown, "gap"
		event.Diagnostic = source.Diagnostic
		write = ConversationWrite{Event: event}
	} else {
		if source.Session != "" {
			event.SessionID, event.SessionQuality = source.Session, IdentityNative
		}
		write = mapper.MapCodexJSONL(event, source.Raw)
	}
	checkpoint := ConversationCheckpoint{
		ProjectID: s.Config.identity, ExecutionID: executionID, AttemptID: attemptID,
		Source: event.Source, SourceSegment: segment,
		SourceIdentity: sourceIdentity,
		Generation:     sourceIdentity, Cursor: string(checkpointCursor), LocalSeq: source.SourceSeq,
	}
	if source.Source == "codex-jsonl" && source.SourceIdentity != "" {
		path := s.localConversationLogPath(run, attemptID)
		if path == "" {
			return errors.New("Codex conversation source path is unavailable")
		}
		position, positionErr := currentConversationJournalPosition(context.Background(), s.domainStore, s.Config.identity, executionID, attemptID, event.Source, segment, path, source.SourceIdentity, source.SourceOffset+source.SourceLength, source.SourceSeq)
		if positionErr != nil {
			gap := event
			gap.Type, gap.Phase, gap.Status = ConversationGap, MessagePhaseUnknown, "source_unavailable"
			gap.SourceEventID = "source-unavailable:" + event.SourceEventID
			gap.Diagnostic = positionErr.Error()
			gap.SessionQuality, gap.TurnQuality, gap.ItemQuality = IdentityUnknown, IdentityUnknown, IdentityUnknown
			if _, gapErr := s.domainStore.CommitConversation(context.Background(), []ConversationWrite{{Event: gap, Raw: append([]byte(nil), source.Raw...)}}, ConversationCheckpoint{}); gapErr != nil {
				return errors.Join(positionErr, gapErr)
			}
			return positionErr
		}
		checkpoint.Cursor, positionErr = marshalConversationJournalPosition(position)
		if positionErr != nil {
			return positionErr
		}
	}
	_, err = s.domainStore.CommitConversation(context.Background(), []ConversationWrite{write}, checkpoint)
	return err
}

func (s *Service) recordOrcaTerminalEvent(run *Run, source WorkerEvent) error {
	executionID := run.ExecutionID
	if executionID == "" {
		executionID = legacyExecutionID(run.RunID)
	}
	attemptID := source.AttemptID
	if attemptID == "" {
		attemptID = run.AttemptID
	}
	if attemptID == "" {
		attemptID = run.RunID
	}
	segment := nonEmpty(source.SourceSegment, "orca-terminal-v2-unsegmented")
	eventID := source.SourceEventID
	quality := IdentityDerived
	if eventID == "" {
		eventID, quality = "unidentified:"+newID(), IdentityUnknown
	}
	event := ConversationEvent{
		Version: ConversationVersion, ProjectID: s.Config.identity, TaskID: run.DomainTaskID,
		WorkflowID: run.RunID, ExecutionID: executionID, AttemptID: attemptID,
		Backend: BackendOrca, Protocol: ProtocolOrcaTerminalV2, Agent: run.Agent,
		TargetID: run.Target.ID, ExternalRef: conversationExternalRef(run.Process),
		Source: "orca-terminal-v2", SourceSegment: segment, SourceIdentity: source.SourceIdentity,
		SourceCursor: source.SourceCursor, SessionID: source.Session, SessionQuality: IdentityUnknown, TurnQuality: IdentityUnknown,
		SourceEventID: eventID, SourceEventQuality: quality, SourceSeq: source.SourceSeq,
		Phase: MessagePhaseUnknown, Status: "observed", ReceivedAt: now(),
		Evidence:   map[string]string{"capture": "terminal-read", "turn_boundary": "unknown", "message_phase": "unknown", "source_changed": fmt.Sprint(source.SourceChanged)},
		Diagnostic: source.Diagnostic,
	}
	if source.Session != "" {
		event.SessionQuality = IdentityDerived
	}
	write := ConversationWrite{Event: event}
	if source.SourceGap {
		event.Type, event.Status = ConversationGap, "gap"
		if len(source.Raw) > 0 {
			event.ItemID, event.ItemQuality = event.SourceEventID, IdentityDerived
			event.ItemKey = conversationItemKey(event, event.ItemID)
			event.Attributes = map[string]any{"content_kind": "terminal_observation", "source_changed": source.SourceChanged}
			write = ConversationWrite{Event: event, Body: append([]byte(nil), source.Raw...), BodyMode: "snapshot", Raw: append([]byte(nil), source.Raw...), BodySource: "orca-terminal-stream"}
		} else {
			write.Event = event
		}
	} else if source.InputWait {
		event.Type, event.Status = ConversationInputWait, "waiting"
		event.Attributes = map[string]any{"wait_origin": "host", "business_decision": false}
		event.Evidence["wait_source"] = "orca-terminal-show.agentWait"
		write.Event = event
	} else {
		event.Type, event.ItemID, event.ItemQuality = ConversationObservation, eventID, IdentityDerived
		event.ItemKey = conversationItemKey(event, event.ItemID)
		write = ConversationWrite{Event: event, Body: append([]byte(nil), source.Raw...), BodyMode: "snapshot", Raw: append([]byte(nil), source.Raw...), BodySource: "orca-terminal-stream"}
	}
	generation := ""
	if run.Process != nil {
		generation = run.Process.Incarnation
	}
	checkpoint := ConversationCheckpoint{
		ProjectID: s.Config.identity, ExecutionID: executionID, AttemptID: attemptID,
		Source: event.Source, SourceSegment: segment, SourceIdentity: source.SourceIdentity,
		Generation: generation, Cursor: source.SourceCursor, LocalSeq: source.SourceSeq,
	}
	_, err := s.domainStore.CommitConversation(context.Background(), []ConversationWrite{write}, checkpoint)
	return err
}

func (s *Service) recordOrcaTurnStarted(run *Run, ref ExecutionRef, operation Operation, receipt ReplyReceipt) error {
	if run == nil || s.domainStore == nil || ref.Backend != BackendOrca || !receipt.InputAccepted || !receipt.TurnStarted {
		return nil
	}
	executionID := run.ExecutionID
	if executionID == "" {
		executionID = legacyExecutionID(run.RunID)
	}
	attemptID := run.AttemptID
	if attemptID == "" {
		attemptID = run.RunID
	}
	sessionID := "terminal:" + ref.External.Terminal + ":" + ref.External.Incarnation
	segment := "orca-terminal-session-v1:" + digestValue(ref.External)
	turnID := "reply:" + operation.ID
	event := ConversationEvent{
		Version: ConversationVersion, ProjectID: s.Config.identity, TaskID: run.DomainTaskID,
		WorkflowID: run.RunID, ExecutionID: executionID, AttemptID: attemptID,
		Backend: ref.Backend, Protocol: ref.Protocol, Agent: ref.Agent.Name, TargetID: ref.Target.ID,
		ExternalRef: conversationExternalRef(&ref.External), Source: "orca-reply-receipt", SourceSegment: segment,
		SourceIdentity: "terminal:" + ref.External.RuntimeID + ":" + ref.External.Terminal + ":" + ref.External.Incarnation,
		SessionID:      sessionID, SessionQuality: IdentityDerived, TurnID: turnID, TurnQuality: IdentityDerived,
		SourceEventID: "operation:" + operation.ID, SourceEventQuality: IdentityDerived,
		Type: ConversationTurnStarted, Phase: MessagePhaseUnknown, FinalAvailability: FinalUnknown,
		ReceivedAt: now(), Attributes: map[string]any{"input_accepted": true, "turn_started": true},
		Evidence: map[string]string{"turn_boundary": "reply-receipt.prompt.stages", "derivation": "orca-terminal-reply-operation-v1"},
	}
	_, err := s.domainStore.CommitConversation(context.Background(), []ConversationWrite{{Event: event}}, ConversationCheckpoint{})
	return err
}

func conversationExternalRef(identity *ProcessIdentity) string {
	if identity == nil {
		return ""
	}
	if identity.Kind == "orca" && identity.Terminal != "" {
		return "terminal:" + identity.Terminal + "@" + identity.Incarnation
	}
	if identity.Kind == "os" && identity.PID > 0 {
		return fmt.Sprintf("process:%d@%s", identity.PID, identity.Created)
	}
	return ""
}

func (s *Service) recordExecutionResult(run *Run, result Result, path string) error {
	if run == nil || s.domainStore == nil {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(raw)
	digest := hex.EncodeToString(sum[:])
	executionID := run.ExecutionID
	if executionID == "" {
		executionID = legacyExecutionID(run.RunID)
	}
	attemptID := run.AttemptID
	if attemptID == "" {
		attemptID = run.RunID
	}
	segment := "result-file-v1:" + digest
	event := ConversationEvent{
		Version: ConversationVersion, ProjectID: s.Config.identity, TaskID: run.DomainTaskID,
		WorkflowID: run.RunID, ExecutionID: executionID, AttemptID: attemptID,
		Backend: BackendKind(run.Backend), Protocol: ExecutionProtocol(run.Protocol), Agent: run.Agent,
		TargetID: run.Target.ID, ExternalRef: conversationExternalRef(run.Process),
		Source: "steerline-result", SourceSegment: segment,
		SessionQuality: IdentityUnknown, TurnQuality: IdentityUnknown,
		ItemID: "execution-summary", ItemQuality: IdentityDerived,
		SourceEventID: "sha256:" + digest, SourceEventQuality: IdentityDerived,
		Type: ConversationExecutionResult, Phase: MessagePhaseUnknown,
		ReceivedAt: now(), Raw: &ConversationRawRef{},
		Attributes: map[string]any{"outcome": result.Outcome, "summary_available": result.Summary != nil, "content_kind": "execution_summary"},
		Evidence:   map[string]string{"source_record": "result.json", "result_sha256": digest, "authority": "structured-result-only"},
	}
	write := ConversationWrite{Event: event, Raw: raw, BodySource: "structured-execution-result"}
	if result.Summary != nil {
		write.Body = []byte(*result.Summary)
		write.BodyMode = "complete"
		event.ItemKey = conversationItemKey(event, event.ItemID)
		write.Event = event
	}
	checkpoint := ConversationCheckpoint{ProjectID: event.ProjectID, ExecutionID: executionID, AttemptID: attemptID, Source: event.Source, SourceSegment: segment, SourceIdentity: "result-file:" + run.RunID + ":" + attemptID, Generation: digest, Cursor: digest, LocalSeq: 1}
	_, err = s.domainStore.CommitConversation(context.Background(), []ConversationWrite{write}, checkpoint)
	return err
}

func (s *Service) recordMinimalExecutionResult(run *Run, envelope MinimalStepResultEnvelope, path string) error {
	if run == nil || s.domainStore == nil {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(raw)
	digest := hex.EncodeToString(sum[:])
	segment := "step-result-v1:" + digest
	event := ConversationEvent{
		Version: ConversationVersion, ProjectID: s.Config.identity, TaskID: envelope.TaskID,
		WorkflowID: envelope.WorkflowRunID, ExecutionID: envelope.ExecutionID, AttemptID: envelope.AttemptID,
		Backend: BackendKind(run.Backend), Protocol: ExecutionProtocol(run.Protocol), Agent: run.Agent,
		TargetID: run.Target.ID, ExternalRef: conversationExternalRef(run.Process),
		Source: "steerline-step-result", SourceSegment: segment, SourceIdentity: "step-result:" + envelope.ExecutionID + ":" + envelope.AttemptID,
		SessionQuality: IdentityUnknown, TurnQuality: IdentityUnknown, ItemQuality: IdentityUnknown,
		SourceEventID: "sha256:" + digest, SourceEventQuality: IdentityDerived,
		Type: ConversationExecutionResult, Phase: MessagePhaseUnknown, ReceivedAt: now(),
		Attributes: map[string]any{"step_id": envelope.Result.StepID, "succeeded": envelope.Result.Succeeded, "content_kind": "structured_step_result", "result_id": envelope.Result.ResultID},
		Evidence:   map[string]string{"source_record": "step-result.json", "result_sha256": digest, "authority": "structured-result-only"},
	}
	checkpoint := ConversationCheckpoint{ProjectID: event.ProjectID, ExecutionID: event.ExecutionID, AttemptID: event.AttemptID, Source: event.Source, SourceSegment: segment, SourceIdentity: event.SourceIdentity, Generation: digest, Cursor: digest, LocalSeq: 1}
	_, err = s.domainStore.CommitConversation(context.Background(), []ConversationWrite{{Event: event, Raw: raw}}, checkpoint)
	return err
}

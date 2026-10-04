package codexworker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

const maxConversationCursorBytes = 64 << 10

const historyBindingFile = "codex-history.v1.json"

type ConversationAdapter struct {
}

type codexHistoryCursor struct {
	Cursor      string             `json:"cursor"`
	Sequence    uint64             `json:"sequence"`
	LastTurnID  string             `json:"last_turn_id,omitempty"`
	LastDigest  string             `json:"last_digest,omitempty"`
	Pending     []codexPendingTurn `json:"pending,omitempty"`
	NextPending int                `json:"next_pending,omitempty"`
	ReadPending bool               `json:"read_pending,omitempty"`
}

type codexPendingTurn struct {
	ID     string `json:"id"`
	Cursor string `json:"cursor"`
	Digest string `json:"digest"`
}

type historyPositionEnvelope struct {
	Version           int                `json:"version"`
	ImplementationKey string             `json:"implementation_key"`
	CodecVersion      int                `json:"codec_version"`
	Purpose           string             `json:"purpose"`
	ProjectID         string             `json:"project_id"`
	ExecutionID       string             `json:"execution_id"`
	AttemptID         string             `json:"attempt_id"`
	SourceIdentity    string             `json:"source_identity"`
	BindingDigest     string             `json:"binding_digest"`
	Digest            string             `json:"digest"`
	Payload           codexHistoryCursor `json:"payload"`
}

func historyPosition(binding HistoryBinding, cursor codexHistoryCursor) historyPositionEnvelope {
	return historyPositionEnvelope{Version: 1, ImplementationKey: "codex.history-position", CodecVersion: 1, Purpose: "source-position",
		ProjectID: binding.ProjectID, ExecutionID: binding.ExecutionID, AttemptID: binding.AttemptID, SourceIdentity: binding.Identity(), BindingDigest: digestValue(binding), Payload: cursor}
}

func encodeHistoryPosition(binding HistoryBinding, cursor codexHistoryCursor) (string, error) {
	value := historyPosition(binding, cursor)
	value.Digest = digestValue(value)
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	if len(raw) > maxConversationCursorBytes {
		return "", errors.New("history source cursor exceeds the persisted budget")
	}
	return string(raw), nil
}

func decodeHistoryPosition(binding HistoryBinding, raw string) (codexHistoryCursor, error) {
	var value historyPositionEnvelope
	if len(raw) > maxConversationCursorBytes {
		return codexHistoryCursor{}, errors.New("persisted history source cursor exceeds the read budget")
	}
	if err := executionapi.DecodeStrictJSON([]byte(raw), &value, "conversation source binding has trailing data"); err != nil {
		return codexHistoryCursor{}, err
	}
	digest := value.Digest
	value.Digest = ""
	expected := historyPosition(binding, value.Payload)
	if value.Version != expected.Version || value.ImplementationKey != expected.ImplementationKey || value.CodecVersion != expected.CodecVersion || value.Purpose != expected.Purpose || value.ProjectID != expected.ProjectID || value.ExecutionID != expected.ExecutionID || value.AttemptID != expected.AttemptID || value.SourceIdentity != expected.SourceIdentity || value.BindingDigest != expected.BindingDigest || digest == "" || digest != digestValue(value) {
		return codexHistoryCursor{}, errors.New("history position differs from its versioned original source binding")
	}
	cursor := value.Payload
	if cursor.NextPending < 0 || len(cursor.Pending) > 1024 || len(cursor.Cursor) > MaxHistoryCursorBytes {
		return codexHistoryCursor{}, errors.New("history position exceeds its native cursor limits")
	}
	for _, pending := range cursor.Pending {
		if pending.ID == "" || pending.Cursor == "" || len(pending.Cursor) > MaxHistoryCursorBytes {
			return codexHistoryCursor{}, errors.New("history position has an invalid pending turn anchor")
		}
	}
	return cursor, nil
}

func NewConversationAdapter() contracts.ConversationSourcePort {
	return &ConversationAdapter{}
}

func (a *ConversationAdapter) RestoreMapper(snapshot contracts.ConversationMapperSnapshot) contracts.ConversationMapper {
	return RestoreConversationMapper(snapshot)
}

func (a *ConversationAdapter) MapConversation(input contracts.ConversationInput) (contracts.ConversationMappedBatch, error) {
	if input.Source != "" && input.Source != ConversationJSONLSource {
		return contracts.ConversationMappedBatch{}, nil
	}
	batch, err := a.MapRealtime(input)
	if err == nil {
		batch.VerifySourcePosition = input.Event.Backend == executionapi.BackendLocal
	}
	return batch, err
}

func (l *ExecutionLifecycle) MapConversation(input contracts.ConversationInput) (contracts.ConversationMappedBatch, error) {
	return (&ConversationAdapter{}).MapConversation(input)
}

func (a *ConversationAdapter) JournalSources() []string { return []string{ConversationJSONLSource} }

func (a *ConversationAdapter) JournalSource(backend executionapi.BackendKind, protocol executionapi.ExecutionProtocol) string {
	return JournalSource(backend, protocol)
}

func (a *ConversationAdapter) JournalSegment(backend executionapi.BackendKind, protocol executionapi.ExecutionProtocol, identity string) string {
	return JournalSegment(backend, protocol, identity)
}

func (a *ConversationAdapter) JournalGeneration(fileIdentity, segment string) (uint64, error) {
	return JournalGeneration(fileIdentity, segment)
}

func (a *ConversationAdapter) MapRealtime(input contracts.ConversationInput) (contracts.ConversationMappedBatch, error) {
	if input.Event.ProjectID == "" || input.Event.ExecutionID == "" || input.Event.AttemptID == "" {
		return contracts.ConversationMappedBatch{}, errors.New("conversation record has no fixed execution identity")
	}
	segment := input.Segment
	if segment == "" {
		segment = "local-jsonl-v1:" + input.Identity
	}
	identity := input.Identity
	if identity == "" {
		identity = "local-events:" + input.Event.WorkflowID + ":" + input.Event.AttemptID
	}
	cursor, err := json.Marshal(struct {
		ByteOffset int64  `json:"byte_offset"`
		ByteLength int64  `json:"byte_length"`
		SourceSeq  uint64 `json:"source_seq"`
	}{input.SourceOffset, input.SourceLength, input.SourceSeq})
	if err != nil {
		return contracts.ConversationMappedBatch{}, err
	}
	event := input.Event
	event.Version, event.Source, event.SourceRole, event.SourceSegment, event.SourceIdentity = contracts.ConversationVersion, ConversationJSONLSource, contracts.ConversationSourceFallback, segment, identity
	event.ExternalRef = conversationExternalRef(input.Process)
	event.SourceCursor = string(cursor)
	event.SessionQuality, event.TurnQuality, event.ItemQuality = contracts.IdentityUnknown, contracts.IdentityUnknown, contracts.IdentityUnknown
	event.SourceEventQuality, event.SourceSeq, event.ReceivedAt = contracts.IdentityDerived, input.SourceSeq, time.Now().UTC()
	event.SourceEventID = fmt.Sprintf("bytes:%d:%d", input.SourceOffset, input.SourceOffset+input.SourceLength)
	event.Evidence = map[string]string{"source_record": "events.jsonl", "byte_start": fmt.Sprint(input.SourceOffset), "byte_end": fmt.Sprint(input.SourceOffset + input.SourceLength), "identity_rule": "source-file-byte-range-v1"}
	if input.Diagnostic != "" {
		event.Evidence["collector_diagnostic"] = input.Diagnostic
	}
	var write contracts.ConversationWrite
	if input.Gap {
		event.Type, event.Phase, event.Status, event.Diagnostic = contracts.ConversationGap, contracts.MessagePhaseUnknown, "gap", input.Diagnostic
		write.Event = event
	} else {
		if input.Session != "" {
			event.SessionID, event.SessionQuality = input.Session, contracts.IdentityNative
		}
		write = a.RestoreMapper(input.Mapper).MapRecord(event, input.Raw)
	}
	checkpoint := contracts.ConversationCheckpoint{ProjectID: event.ProjectID, ExecutionID: event.ExecutionID, AttemptID: event.AttemptID,
		Source: event.Source, SourceSegment: segment, SourceIdentity: identity, Generation: identity, Cursor: string(cursor), LocalSeq: input.SourceSeq}
	return contracts.ConversationMappedBatch{Writes: []contracts.ConversationWrite{write}, Checkpoint: checkpoint}, nil
}

func (a *ConversationAdapter) LiveJournalPath(ctx context.Context, request contracts.ConversationJournalPathRequest) (string, error) {
	if request.RunDirectory == "" || request.WorkflowID == "" || request.ExecutionID == "" || request.AttemptID == "" {
		return "", errors.New("live conversation source has no fixed run identity")
	}
	if request.WorkerFlow {
		root := filepath.Join(request.RunDirectory, "turns")
		entries, err := os.ReadDir(root)
		if err != nil {
			return "", err
		}
		if len(entries) > 4096 {
			return "", errors.New("worker flow has too many source directories")
		}
		for _, entry := range entries {
			if !entry.IsDir() || !validExecutionID(entry.Name()) {
				continue
			}
			dir := filepath.Join(root, entry.Name())
			markerData, readErr := readConversationSourceFile(filepath.Join(dir, ConversationSourceMarkerName))
			if readErr != nil {
				continue
			}
			var marker ConversationSourceMarker
			if executionapi.DecodeStrictJSON(markerData, &marker, "conversation source binding has trailing data") == nil && marker.Validate() == nil && marker.SourceIdentity == request.SourceIdentity &&
				marker.WorkflowID == request.WorkflowID && marker.ExecutionID == request.ExecutionID && marker.AttemptID == request.AttemptID {
				return filepath.Join(dir, "events.jsonl"), nil
			}
		}
		return "", errors.New("worker flow source marker was not found for the fixed identity")
	}
	candidates := append([]string(nil), request.EventPaths...)
	if request.LogPath != "" {
		candidates = append(candidates, request.LogPath)
	}
	for _, candidate := range candidates {
		if candidate != "" && filepath.Base(candidate) == "events.jsonl" && insidePath(request.RunDirectory, candidate) {
			return filepath.Clean(candidate), nil
		}
	}
	return "", errors.New("conversation source path is unavailable")
}

func (a *ConversationAdapter) DiscoverHistory(ctx context.Context, discovery contracts.ConversationDiscovery) (contracts.ConversationDiscoveryResult, error) {
	if discovery.ProjectID == "" || discovery.RunDirectory == "" {
		return contracts.ConversationDiscoveryResult{}, errors.New("conversation source discovery lacks a project or run directory")
	}
	result := contracts.ConversationDiscoveryResult{}
	seen := map[string]bool{}
	for _, run := range discovery.Runs {
		if err := ctx.Err(); err != nil {
			return contracts.ConversationDiscoveryResult{}, err
		}
		base := run.Base
		if base.Agent != "codex" || base.ProjectID != discovery.ProjectID || base.WorkflowID == "" || base.ExecutionID == "" || run.RunDirectory == "" {
			continue
		}
		attempts := append([]contracts.ConversationAttemptSource(nil), run.Attempts...)
		if run.WorkerFlow {
			entries, readErr := os.ReadDir(filepath.Join(run.RunDirectory, "turns"))
			if readErr == nil && len(entries) <= 4096 {
				ended := false
				for _, attempt := range run.Attempts {
					if attempt.AttemptID == base.AttemptID {
						ended = attempt.Ended
					}
				}
				for _, entry := range entries {
					if entry.IsDir() && validExecutionID(entry.Name()) {
						attempts = append(attempts, contracts.ConversationAttemptSource{AttemptID: base.AttemptID, EventPath: filepath.Join(run.RunDirectory, "turns", entry.Name(), "events.jsonl"), Ended: ended})
					}
				}
			}
		}
		for _, attempt := range attempts {
			if attempt.EventPath == "" || !validExecutionID(attempt.AttemptID) || !insidePath(run.RunDirectory, attempt.EventPath) {
				continue
			}
			dir := filepath.Dir(filepath.Clean(attempt.EventPath))
			if seen[dir] {
				continue
			}
			seen[dir] = true
			entries, err := os.ReadDir(dir)
			if err != nil {
				if !os.IsNotExist(err) {
					result.Writes = append(result.Writes, bindingGap(base, attempt.AttemptID, dir, err))
				}
				continue
			}
			if len(entries) > 2048 {
				result.Writes = append(result.Writes, bindingGap(base, attempt.AttemptID, dir, errors.New("too many files to scan for bounded history bindings")))
				continue
			}
			for _, entry := range entries {
				name := entry.Name()
				if name != historyBindingFile {
					if !strings.HasPrefix(name, "codex-history.") || !strings.HasSuffix(name, ".v1.json") {
						continue
					}
					session := strings.TrimSuffix(strings.TrimPrefix(name, "codex-history."), ".v1.json")
					if !ValidNativeID(session) {
						continue
					}
				}
				bindingPath := filepath.Join(dir, name)
				info, infoErr := entry.Info()
				if infoErr != nil || !info.Mode().IsRegular() {
					result.Writes = append(result.Writes, bindingGap(base, attempt.AttemptID, bindingPath, errors.New("history binding is not a regular file")))
					continue
				}
				binding, readErr := ReadHistoryBinding(bindingPath)
				if readErr != nil || !bindingMatches(binding, base, run.Workspace, attempt.AttemptID, name, dir) {
					cause := readErr
					if cause == nil {
						cause = errors.New("binding identity differs from recorded execution")
					}
					result.Writes = append(result.Writes, bindingGap(base, attempt.AttemptID, bindingPath, cause))
					continue
				}
				if binding.Backend == executionapi.BackendOrca && binding.ExternalRef == "" {
					continue
				}
				binding.Ended = attempt.Ended
				identity := binding.Identity() + ":" + binding.AttemptID
				result.Sources = append(result.Sources, contracts.ConversationHistorySource{Identity: identity, BindingPath: bindingPath, Workspace: binding.Workspace, Event: binding.Event(), Ended: binding.Ended,
					RequiresPlatformVersion: binding.Backend == executionapi.BackendOrca})
			}
		}
	}
	return result, nil
}

func (a *ConversationAdapter) DiscoverJournals(ctx context.Context, discovery contracts.ConversationDiscovery, identify func(string) (string, error)) ([]contracts.ConversationJournalSource, error) {
	if identify == nil {
		return nil, errors.New("conversation journal identity reader is required")
	}
	sources := []contracts.ConversationJournalSource{}
	seen := map[string]bool{}
	for _, run := range discovery.Runs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if JournalSource(run.Base.Backend, run.Base.Protocol) == "" {
			continue
		}
		candidates := append([]contracts.ConversationAttemptSource(nil), run.Attempts...)
		if run.WorkerFlow {
			turnRoot := filepath.Join(run.RunDirectory, "turns")
			if entries, err := os.ReadDir(turnRoot); err == nil && len(entries) <= 4096 {
				attempt := run.Base.AttemptID
				ended := false
				for _, item := range run.Attempts {
					if item.AttemptID == attempt {
						ended = item.Ended
						break
					}
				}
				for _, entry := range entries {
					if entry.IsDir() && validExecutionID(entry.Name()) {
						candidates = append(candidates, contracts.ConversationAttemptSource{AttemptID: attempt, EventPath: filepath.Join(turnRoot, entry.Name(), "events.jsonl"), Ended: ended})
					}
				}
			}
		}
		root := run.RunDirectory
		if root == "" {
			root = filepath.Join(discovery.RunDirectory, "runs", run.Base.WorkflowID)
		}
		for _, candidate := range candidates {
			path := filepath.Clean(candidate.EventPath)
			if path == "" || filepath.Base(path) != "events.jsonl" || !insidePath(root, path) || seen[path] || !validExecutionID(candidate.AttemptID) {
				continue
			}
			markerPath := filepath.Join(filepath.Dir(path), ConversationSourceMarkerName)
			markerData, markerErr := readConversationSourceFile(markerPath)
			if os.IsNotExist(markerErr) {
				continue
			}
			fileID, fileErr := identify(path)
			if fileErr != nil {
				continue
			}
			seen[path] = true
			source := contracts.ConversationJournalSource{Path: path, AttemptID: candidate.AttemptID, FileID: fileID, Workspace: run.Workspace, Ended: candidate.Ended}
			base := run.Base
			base.AttemptID = candidate.AttemptID
			base.ExternalRef = conversationExternalRef(run.Process)
			source.Event = base
			if markerErr != nil {
				source.Diagnostic = fmt.Sprintf("source marker is unreadable: %v", markerErr)
				sources = append(sources, source)
				continue
			}
			var marker ConversationSourceMarker
			if err := executionapi.DecodeStrictJSON(markerData, &marker, "conversation source binding has trailing data"); err != nil {
				source.Diagnostic = fmt.Sprintf("source marker is invalid: %v", err)
			} else if err = marker.Validate(); err != nil {
				source.Diagnostic = err.Error()
			} else if marker.ProjectID != base.ProjectID || marker.WorkflowID != base.WorkflowID || marker.ExecutionID != base.ExecutionID || marker.AttemptID != candidate.AttemptID || marker.Backend != base.Backend || marker.Protocol != base.Protocol || marker.Agent != base.Agent || marker.TargetID != base.TargetID {
				source.Diagnostic = "source marker identity does not match the fixed run and attempt"
			} else {
				source.ExpectedFileID = marker.SourceIdentity
			}
			sources = append(sources, source)
		}
	}
	return sources, nil
}

func (a *ConversationAdapter) ReadHistory(ctx context.Context, request contracts.ConversationHistoryReadRequest, start func(*exec.Cmd) (executionapi.StopHandle, error)) (contracts.ConversationHistoryBatch, error) {
	binding, err := ReadHistoryBinding(request.Source.BindingPath)
	event := request.Source.Event
	if err != nil || request.Source.Identity != binding.Identity()+":"+binding.AttemptID || event.ProjectID != binding.ProjectID || event.TaskID != binding.TaskID || event.WorkflowID != binding.WorkflowID || event.ExecutionID != binding.ExecutionID || event.AttemptID != binding.AttemptID || event.Backend != binding.Backend || event.Protocol != binding.Protocol || event.TargetID != binding.TargetID || event.ExternalRef != binding.ExternalRef || event.SessionID != binding.SessionID || event.SourceIdentity != binding.Identity() || event.SourceSegment != binding.Identity() || canonicalPath(request.Source.Workspace) != canonicalPath(binding.Workspace) {
		return contracts.ConversationHistoryBatch{}, errors.New("history source is no longer bound to its discovered execution")
	}
	var cursor codexHistoryCursor
	if request.Cursor != "" {
		cursor, err = decodeHistoryPosition(binding, request.Cursor)
		if err != nil {
			return contracts.ConversationHistoryBatch{}, err
		}
	}
	readerDir := binding.Workspace
	if !directoryExists(readerDir) {
		readerDir = request.Repository
		if !directoryExists(readerDir) {
			readerDir = request.RunDirectory
			if !directoryExists(readerDir) {
				readerDir = ""
			}
		}
	}
	client, err := OpenHistory(ctx, binding.Command, readerDir, binding.Home, start)
	if err != nil {
		return contracts.ConversationHistoryBatch{}, err
	}
	defer client.Close()
	if err = client.ReadMetadata(ctx, binding.SessionID, binding.Workspace); err != nil {
		return contracts.ConversationHistoryBatch{}, err
	}
	readCursor := cursor.Cursor
	pendingIndex := -1
	if len(cursor.Pending) > 0 && cursor.ReadPending {
		pendingIndex = cursor.NextPending % len(cursor.Pending)
		readCursor = cursor.Pending[pendingIndex].Cursor
		cursor.NextPending++
		cursor.ReadPending = false
	} else if len(cursor.Pending) > 0 {
		cursor.ReadPending = true
	}
	page, err := client.ReadPage(ctx, binding.SessionID, readCursor)
	if err != nil {
		return contracts.ConversationHistoryBatch{}, err
	}
	if pendingIndex >= 0 && (len(page.Data) != 1 || page.Data[0].ID != cursor.Pending[pendingIndex].ID) {
		return contracts.ConversationHistoryBatch{}, errors.New("history cursor changed its native turn identity")
	}
	base := binding.Event()
	writes := []contracts.ConversationWrite{}
	if cursor.Sequence == 0 && len(page.Data) > 0 {
		event := base
		event.Type, event.Status = contracts.ConversationGap, "source_changed"
		event.SourceEventID = "native-source-selected"
		event.Diagnostic = "Verified history is the canonical conversation source; raw source projections remain available through an explicit source filter"
		event.Attributes = map[string]any{"supersedes_sources": []string{"codex-jsonl", "orca-terminal-v2", "orca-reply-receipt"}}
		writes = append(writes, contracts.ConversationWrite{Event: event})
		event = base
		event.Type, event.SourceEventID = contracts.ConversationSession, "session:"+binding.SessionID
		writes = append(writes, contracts.ConversationWrite{Event: event})
	}
	for _, turn := range page.Data {
		digest := digestValue(turn)
		unchanged := pendingIndex >= 0 && cursor.Pending[pendingIndex].Digest == digest || pendingIndex < 0 && cursor.LastTurnID == turn.ID && cursor.LastDigest == digest
		if !unchanged {
			mapped, mapErr := MapHistoryTurn(base, turn, cursor.Sequence+uint64(len(writes))+1)
			if mapErr != nil {
				return contracts.ConversationHistoryBatch{}, mapErr
			}
			writes = append(writes, mapped...)
		}
		native, inspectErr := InspectHistoryTurn(turn)
		if inspectErr != nil {
			return contracts.ConversationHistoryBatch{}, inspectErr
		}
		if pendingIndex >= 0 && native.HasFinal {
			cursor.Pending = append(cursor.Pending[:pendingIndex], cursor.Pending[pendingIndex+1:]...)
			if len(cursor.Pending) == 0 {
				cursor.NextPending, cursor.ReadPending = 0, false
			}
		} else if pendingIndex >= 0 {
			cursor.Pending[pendingIndex].Digest = digest
		} else if native.Terminal && !native.HasFinal {
			if page.BackwardsCursor == nil || *page.BackwardsCursor == "" {
				return contracts.ConversationHistoryBatch{}, errors.New("history omitted the anchor needed to reconcile a final-less turn")
			}
			known := false
			for _, pending := range cursor.Pending {
				if pending.ID == turn.ID {
					known = true
					break
				}
			}
			if !known {
				if len(cursor.Pending) >= 1024 {
					return contracts.ConversationHistoryBatch{}, errors.New("history has too many unresolved turns for a bounded cursor")
				}
				cursor.Pending = append(cursor.Pending, codexPendingTurn{ID: turn.ID, Cursor: *page.BackwardsCursor, Digest: digest})
			}
		}
		if pendingIndex < 0 {
			cursor.LastTurnID, cursor.LastDigest = turn.ID, digest
		}
	}
	if pendingIndex < 0 {
		if page.NextCursor != nil && *page.NextCursor != "" {
			cursor.Cursor = *page.NextCursor
		} else if page.BackwardsCursor != nil {
			cursor.Cursor = *page.BackwardsCursor
		}
	}
	cursor.Sequence += uint64(len(writes))
	encoded, err := encodeHistoryPosition(binding, cursor)
	if err != nil {
		return contracts.ConversationHistoryBatch{}, err
	}
	settled := false
	if pendingIndex < 0 && len(cursor.Pending) == 0 && len(page.Data) > 0 {
		native, inspectErr := InspectHistoryTurn(page.Data[0])
		if inspectErr != nil {
			return contracts.ConversationHistoryBatch{}, inspectErr
		}
		settled = native.Terminal
	}
	version := clip(client.UserAgent(), 256)
	platformVersion := version
	if binding.Backend == executionapi.BackendOrca {
		platformVersion = strings.TrimSpace(request.PlatformVersion)
		if platformVersion == "" {
			return contracts.ConversationHistoryBatch{}, errors.New("host implementation version is unavailable for capability reporting")
		}
	}
	report := &contracts.ConversationCapabilityReport{Version: 1, Backend: binding.Backend, Protocol: binding.Protocol, Agent: "codex", AgentVersion: version,
		PlatformVersion: platformVersion, Target: executionapi.ExecutionTarget{ID: binding.TargetID}, ExecutionID: binding.ExecutionID, AttemptID: binding.AttemptID,
		Source: "codex-app-server", SourceIdentity: binding.Identity(), CheckedAt: time.Now().UTC()}
	report.Capabilities = historyCapabilities(binding, page, request.HasCheckpoint)
	return contracts.ConversationHistoryBatch{Writes: writes, Cursor: encoded, Sequence: cursor.Sequence, Settled: settled, More: page.NextCursor != nil && *page.NextCursor != "", Capabilities: report}, nil
}

func historyCapabilities(binding HistoryBinding, page HistoryPage, hadCheckpoint bool) []contracts.ConversationCapability {
	values := map[string]contracts.ConversationCapability{
		"turn_boundaries":      {Name: "turn_boundaries", Status: executionapi.CapabilityNotTested, Evidence: "No completed native turn observed yet"},
		"message_phase":        {Name: "message_phase", Status: executionapi.CapabilityNotTested, Evidence: "No phase-bearing assistant message observed yet"},
		"final_content":        {Name: "final_content", Status: executionapi.CapabilityNotTested, Evidence: "No complete native final message observed yet"},
		"stable_item_identity": {Name: "stable_item_identity", Status: executionapi.CapabilityNotTested, Evidence: "No native item observed yet"},
		"incremental_read":     {Name: "incremental_read", Status: executionapi.CapabilityNotTested, Evidence: "A saved turn cursor has not been resumed yet"},
		"history_replay":       {Name: "history_replay", Status: executionapi.CapabilityNotTested, Evidence: "A persisted checkpoint has not been replayed yet"},
		"input_wait":           {Name: "input_wait", Status: executionapi.CapabilityNotTested, Evidence: "A host input-wait object has not been observed"},
	}
	if binding.Backend == executionapi.BackendLocal {
		values["input_wait"] = contracts.ConversationCapability{Name: "input_wait", Status: executionapi.CapabilityUnsupported, Evidence: "local_process Reply is unsupported by the execution backend"}
	}
	for _, turn := range page.Data {
		if turn.CompletedAt != nil {
			values["turn_boundaries"] = contracts.ConversationCapability{Name: "turn_boundaries", Status: executionapi.CapabilityVerified, Evidence: "Native turn ID, terminal status and completedAt were observed through full turn history"}
		}
		if len(turn.Items) > 0 {
			values["stable_item_identity"] = contracts.ConversationCapability{Name: "stable_item_identity", Status: executionapi.CapabilityVerified, Evidence: "Native session, turn and item IDs were observed through full turn history"}
		}
		native, err := InspectHistoryTurn(turn)
		if err == nil && native.HasPhase {
			values["message_phase"] = contracts.ConversationCapability{Name: "message_phase", Status: executionapi.CapabilityVerified, Evidence: "Native agentMessage.phase was observed through full turn history"}
		}
		if err == nil && native.CompleteFinal {
			values["final_content"] = contracts.ConversationCapability{Name: "final_content", Status: executionapi.CapabilityVerified, Evidence: "A completed native final_answer item has a persisted full body reference"}
		}
	}
	if hadCheckpoint && (page.NextCursor != nil || page.BackwardsCursor != nil) {
		values["incremental_read"] = contracts.ConversationCapability{Name: "incremental_read", Status: executionapi.CapabilityVerified, Evidence: "A persisted opaque turn cursor was resumed with a bounded full-item page"}
	}
	if hadCheckpoint && len(page.Data) > 0 {
		values["history_replay"] = contracts.ConversationCapability{Name: "history_replay", Status: executionapi.CapabilityVerified, Evidence: "A new public app-server process replayed native history from the persisted source checkpoint"}
	}
	names := []string{"turn_boundaries", "message_phase", "final_content", "stable_item_identity", "incremental_read", "history_replay", "input_wait"}
	result := make([]contracts.ConversationCapability, 0, len(names))
	for _, name := range names {
		result = append(result, values[name])
	}
	return result
}

func bindingMatches(b HistoryBinding, base contracts.ConversationEvent, workspace, attempt, filename, directory string) bool {
	if b.ProjectID != base.ProjectID || b.WorkflowID != base.WorkflowID || b.ExecutionID != base.ExecutionID || b.AttemptID != attempt || b.TargetID != base.TargetID || b.Backend != base.Backend || b.Protocol != base.Protocol || canonicalPath(b.Workspace) != canonicalPath(workspace) {
		return false
	}
	if b.Backend == executionapi.BackendOrca {
		return filename == historyBindingFile
	}
	return filename == filepath.Base(localConversationHistoryBindingPath(directory, b.SessionID))
}

func localConversationHistoryBindingPath(dir, session string) string {
	return filepath.Join(dir, "codex-history."+session+".v1.json")
}

func bindingGap(base contracts.ConversationEvent, attempt, path string, cause error) contracts.ConversationWrite {
	event := base
	event.AttemptID, event.Source, event.SourceRole = attempt, "codex-app-server", contracts.ConversationSourceCanonical
	event.SourceSegment = "invalid-binding-v1:" + attempt
	event.SourceEventID = "binding:" + digestValue(struct{ Path, Reason string }{path, cause.Error()})
	event.Type, event.Status, event.Phase = contracts.ConversationGap, "source_unavailable", contracts.MessagePhaseUnknown
	event.SessionQuality, event.TurnQuality, event.ItemQuality = contracts.IdentityUnknown, contracts.IdentityUnknown, contracts.IdentityUnknown
	event.SourceEventQuality, event.ReceivedAt = contracts.IdentityDerived, time.Now().UTC()
	event.Diagnostic = fmt.Sprintf("conversation source binding for %s/%s: %v", base.WorkflowID, attempt, cause)
	return contracts.ConversationWrite{Event: event}
}

func insidePath(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func validExecutionID(value string) bool { return len(value) == 32 && isHex(value) }

func isHex(value string) bool {
	for _, char := range value {
		if char < '0' || char > '9' && (char < 'a' || char > 'f') && (char < 'A' || char > 'F') {
			return false
		}
	}
	return value != ""
}

func directoryExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func conversationExternalRef(identity *executionapi.ProcessIdentity) string {
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

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

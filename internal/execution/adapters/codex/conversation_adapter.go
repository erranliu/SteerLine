package codexworker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	conversationtypes "github.com/erranliu/SteerLine/internal/execution/conversation"
	executionvalues "github.com/erranliu/SteerLine/internal/execution/model"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
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
	if err := executionbackend.DecodeStrictJSON([]byte(raw), &value, "conversation source binding has trailing data"); err != nil {
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

func NewConversationAdapter() conversationtypes.ConversationSourcePort {
	return &ConversationAdapter{}
}

func (a *ConversationAdapter) DiscoverHistory(ctx context.Context, discovery conversationtypes.ConversationDiscovery) (conversationtypes.ConversationDiscoveryResult, error) {
	if discovery.ProjectID == "" || discovery.RunDirectory == "" {
		return conversationtypes.ConversationDiscoveryResult{}, errors.New("conversation source discovery lacks a project or run directory")
	}
	result := conversationtypes.ConversationDiscoveryResult{}
	seen := map[string]bool{}
	for _, run := range discovery.Runs {
		if err := ctx.Err(); err != nil {
			return conversationtypes.ConversationDiscoveryResult{}, err
		}
		base := run.Base
		if base.Agent != "codex" || base.ProjectID != discovery.ProjectID || base.WorkflowID == "" || base.ExecutionID == "" || run.RunDirectory == "" {
			continue
		}
		attempts := append([]conversationtypes.ConversationAttemptSource(nil), run.Attempts...)
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
						attempts = append(attempts, conversationtypes.ConversationAttemptSource{AttemptID: base.AttemptID, EventPath: filepath.Join(run.RunDirectory, "turns", entry.Name(), "events.jsonl"), Ended: ended})
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
				if binding.Backend == executionbackend.BackendOrca && binding.ExternalRef == "" {
					continue
				}
				binding.Ended = attempt.Ended
				identity := binding.Identity() + ":" + binding.AttemptID
				result.Sources = append(result.Sources, conversationtypes.ConversationHistorySource{Identity: identity, BindingPath: bindingPath, Workspace: binding.Workspace, Event: binding.Event(), Ended: binding.Ended,
					RequiresPlatformVersion: binding.Backend == executionbackend.BackendOrca})
			}
		}
	}
	return result, nil
}

func (a *ConversationAdapter) ReadHistory(ctx context.Context, request conversationtypes.ConversationHistoryReadRequest, start func(*exec.Cmd) (executionbackend.StopHandle, error)) (conversationtypes.ConversationHistoryBatch, error) {
	binding, err := ReadHistoryBinding(request.Source.BindingPath)
	event := request.Source.Event
	if err != nil || request.Source.Identity != binding.Identity()+":"+binding.AttemptID || event.ProjectID != binding.ProjectID || event.TaskID != binding.TaskID || event.WorkflowID != binding.WorkflowID || event.ExecutionID != binding.ExecutionID || event.AttemptID != binding.AttemptID || event.Backend != binding.Backend || event.Protocol != binding.Protocol || event.TargetID != binding.TargetID || event.ExternalRef != binding.ExternalRef || event.SessionID != binding.SessionID || event.SourceIdentity != binding.Identity() || event.SourceSegment != binding.Identity() || canonicalPath(request.Source.Workspace) != canonicalPath(binding.Workspace) {
		return conversationtypes.ConversationHistoryBatch{}, errors.New("history source is no longer bound to its discovered execution")
	}
	var cursor codexHistoryCursor
	if request.Cursor != "" {
		cursor, err = decodeHistoryPosition(binding, request.Cursor)
		if err != nil {
			return conversationtypes.ConversationHistoryBatch{}, err
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
		return conversationtypes.ConversationHistoryBatch{}, err
	}
	defer client.Close()
	if err = client.ReadMetadata(ctx, binding.SessionID, binding.Workspace); err != nil {
		return conversationtypes.ConversationHistoryBatch{}, err
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
		return conversationtypes.ConversationHistoryBatch{}, err
	}
	if pendingIndex >= 0 && (len(page.Data) != 1 || page.Data[0].ID != cursor.Pending[pendingIndex].ID) {
		return conversationtypes.ConversationHistoryBatch{}, errors.New("history cursor changed its native turn identity")
	}
	base := binding.Event()
	writes := []executionvalues.ConversationWrite{}
	if cursor.Sequence == 0 && len(page.Data) > 0 {
		event := base
		event.Type, event.SourceEventID = executionvalues.ConversationSession, "session:"+binding.SessionID
		writes = append(writes, executionvalues.ConversationWrite{Event: event})
	}
	for _, turn := range page.Data {
		digest := digestValue(turn)
		unchanged := pendingIndex >= 0 && cursor.Pending[pendingIndex].Digest == digest || pendingIndex < 0 && cursor.LastTurnID == turn.ID && cursor.LastDigest == digest
		if !unchanged {
			mapped, mapErr := MapHistoryTurn(base, turn, cursor.Sequence+uint64(len(writes))+1)
			if mapErr != nil {
				return conversationtypes.ConversationHistoryBatch{}, mapErr
			}
			writes = append(writes, mapped...)
		}
		native, inspectErr := InspectHistoryTurn(turn)
		if inspectErr != nil {
			return conversationtypes.ConversationHistoryBatch{}, inspectErr
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
				return conversationtypes.ConversationHistoryBatch{}, errors.New("history omitted the anchor needed to reconcile a final-less turn")
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
					return conversationtypes.ConversationHistoryBatch{}, errors.New("history has too many unresolved turns for a bounded cursor")
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
		return conversationtypes.ConversationHistoryBatch{}, err
	}
	settled := false
	if pendingIndex < 0 && len(cursor.Pending) == 0 && len(page.Data) > 0 {
		native, inspectErr := InspectHistoryTurn(page.Data[0])
		if inspectErr != nil {
			return conversationtypes.ConversationHistoryBatch{}, inspectErr
		}
		settled = native.Terminal
	}
	version := clip(client.UserAgent(), 256)
	platformVersion := version
	if binding.Backend == executionbackend.BackendOrca {
		platformVersion = strings.TrimSpace(request.PlatformVersion)
		if platformVersion == "" {
			return conversationtypes.ConversationHistoryBatch{}, errors.New("host implementation version is unavailable for capability reporting")
		}
	}
	report := &executionvalues.ConversationCapabilityReport{Version: 1, Backend: binding.Backend, Protocol: binding.Protocol, Agent: "codex", AgentVersion: version,
		PlatformVersion: platformVersion, Target: executionbackend.ExecutionTarget{ID: binding.TargetID}, ExecutionID: binding.ExecutionID, AttemptID: binding.AttemptID,
		Source: "codex-app-server", SourceIdentity: binding.Identity(), CheckedAt: time.Now().UTC()}
	report.Capabilities = historyCapabilities(binding, page, request.HasCheckpoint)
	return conversationtypes.ConversationHistoryBatch{Writes: writes, Cursor: encoded, Sequence: cursor.Sequence, Settled: settled, More: page.NextCursor != nil && *page.NextCursor != "", Capabilities: report}, nil
}

func historyCapabilities(binding HistoryBinding, page HistoryPage, hadCheckpoint bool) []executionvalues.ConversationCapability {
	values := map[string]executionvalues.ConversationCapability{
		"turn_boundaries":      {Name: "turn_boundaries", Status: executionbackend.CapabilityNotTested, Evidence: "No completed native turn observed yet"},
		"message_phase":        {Name: "message_phase", Status: executionbackend.CapabilityNotTested, Evidence: "No phase-bearing assistant message observed yet"},
		"final_content":        {Name: "final_content", Status: executionbackend.CapabilityNotTested, Evidence: "No complete native final message observed yet"},
		"stable_item_identity": {Name: "stable_item_identity", Status: executionbackend.CapabilityNotTested, Evidence: "No native item observed yet"},
		"incremental_read":     {Name: "incremental_read", Status: executionbackend.CapabilityNotTested, Evidence: "A saved turn cursor has not been resumed yet"},
		"history_replay":       {Name: "history_replay", Status: executionbackend.CapabilityNotTested, Evidence: "A persisted checkpoint has not been replayed yet"},
		"input_wait":           {Name: "input_wait", Status: executionbackend.CapabilityNotTested, Evidence: "A host input-wait object has not been observed"},
	}
	if binding.Backend == executionbackend.BackendLocal {
		values["input_wait"] = executionvalues.ConversationCapability{Name: "input_wait", Status: executionbackend.CapabilityUnsupported, Evidence: "local_process Reply is unsupported by the execution backend"}
	}
	for _, turn := range page.Data {
		if turn.CompletedAt != nil {
			values["turn_boundaries"] = executionvalues.ConversationCapability{Name: "turn_boundaries", Status: executionbackend.CapabilityVerified, Evidence: "Native turn ID, terminal status and completedAt were observed through full turn history"}
		}
		if len(turn.Items) > 0 {
			values["stable_item_identity"] = executionvalues.ConversationCapability{Name: "stable_item_identity", Status: executionbackend.CapabilityVerified, Evidence: "Native session, turn and item IDs were observed through full turn history"}
		}
		native, err := InspectHistoryTurn(turn)
		if err == nil && native.HasPhase {
			values["message_phase"] = executionvalues.ConversationCapability{Name: "message_phase", Status: executionbackend.CapabilityVerified, Evidence: "Native agentMessage.phase was observed through full turn history"}
		}
		if err == nil && native.CompleteFinal {
			values["final_content"] = executionvalues.ConversationCapability{Name: "final_content", Status: executionbackend.CapabilityVerified, Evidence: "A completed native final_answer item has a persisted full body reference"}
		}
	}
	if hadCheckpoint && (page.NextCursor != nil || page.BackwardsCursor != nil) {
		values["incremental_read"] = executionvalues.ConversationCapability{Name: "incremental_read", Status: executionbackend.CapabilityVerified, Evidence: "A persisted opaque turn cursor was resumed with a bounded full-item page"}
	}
	if hadCheckpoint && len(page.Data) > 0 {
		values["history_replay"] = executionvalues.ConversationCapability{Name: "history_replay", Status: executionbackend.CapabilityVerified, Evidence: "A new public app-server process replayed native history from the persisted source checkpoint"}
	}
	names := []string{"turn_boundaries", "message_phase", "final_content", "stable_item_identity", "incremental_read", "history_replay", "input_wait"}
	result := make([]executionvalues.ConversationCapability, 0, len(names))
	for _, name := range names {
		result = append(result, values[name])
	}
	return result
}

func bindingMatches(b HistoryBinding, base executionvalues.ConversationEvent, workspace, attempt, filename, directory string) bool {
	if b.ProjectID != base.ProjectID || b.WorkflowID != base.WorkflowID || b.ExecutionID != base.ExecutionID || b.AttemptID != attempt || b.TargetID != base.TargetID || b.Backend != base.Backend || b.Protocol != base.Protocol || canonicalPath(b.Workspace) != canonicalPath(workspace) {
		return false
	}
	if b.Backend == executionbackend.BackendOrca {
		return filename == historyBindingFile
	}
	return filename == filepath.Base(localConversationHistoryBindingPath(directory, b.SessionID))
}

func localConversationHistoryBindingPath(dir, session string) string {
	return filepath.Join(dir, "codex-history."+session+".v1.json")
}

func bindingGap(base executionvalues.ConversationEvent, attempt, path string, cause error) executionvalues.ConversationWrite {
	event := base
	event.AttemptID, event.Source, event.SourceRole = attempt, "codex-app-server", executionvalues.ConversationSourceCanonical
	event.SourceSegment = "invalid-binding-v1:" + attempt
	event.SourceEventID = "binding:" + digestValue(struct{ Path, Reason string }{path, cause.Error()})
	event.Type, event.Status, event.Phase = executionvalues.ConversationGap, "source_unavailable", executionvalues.MessagePhaseUnknown
	event.SessionQuality, event.TurnQuality, event.ItemQuality = executionvalues.IdentityUnknown, executionvalues.IdentityUnknown, executionvalues.IdentityUnknown
	event.SourceEventQuality, event.ReceivedAt = executionvalues.IdentityDerived, time.Now().UTC()
	event.Diagnostic = fmt.Sprintf("conversation source binding for %s/%s: %v", base.WorkflowID, attempt, cause)
	return executionvalues.ConversationWrite{Event: event}
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

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

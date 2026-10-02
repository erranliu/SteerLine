package codexworker

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

const ConversationSourceMarkerName = "conversation-source.v1.json"
const ConversationJSONLSource = "codex-jsonl"
const LocalCommandOutputSource = "local-command-output"

func LocalProcessJournalSegment(identity string) string {
	return JournalSegment(executionapi.BackendLocal, executionapi.ProtocolLocalProcess, identity)
}

func LocalCommandJournalSegment(identity string) string { return "local-command-v1:" + identity }

func JournalSource(backend executionapi.BackendKind, protocol executionapi.ExecutionProtocol) string {
	if backend == executionapi.BackendLocal && (protocol == executionapi.ProtocolLocalProcess || protocol == executionapi.ProtocolLocalCodexAppServer) ||
		backend == executionapi.BackendOrca && (protocol == executionapi.ProtocolOrcaTerminalV2 || protocol == executionapi.ProtocolOrcaSupervised) {
		return ConversationJSONLSource
	}
	return ""
}

func JournalSegment(backend executionapi.BackendKind, protocol executionapi.ExecutionProtocol, identity string) string {
	prefix := ""
	if backend == executionapi.BackendLocal && (protocol == executionapi.ProtocolLocalProcess || protocol == executionapi.ProtocolLocalCodexAppServer) {
		prefix = "local-jsonl-v1:"
	} else if backend == executionapi.BackendOrca && (protocol == executionapi.ProtocolOrcaTerminalV2 || protocol == executionapi.ProtocolOrcaSupervised) {
		prefix = "orca-jsonl-v1:"
	}
	if prefix == "" {
		return ""
	}
	return prefix + identity
}

func JournalGeneration(fileIdentity, segment string) (uint64, error) {
	prefix := JournalSegment(executionapi.BackendLocal, executionapi.ProtocolLocalProcess, fileIdentity)
	if strings.HasPrefix(segment, JournalSegment(executionapi.BackendOrca, executionapi.ProtocolOrcaTerminalV2, fileIdentity)) {
		prefix = JournalSegment(executionapi.BackendOrca, executionapi.ProtocolOrcaTerminalV2, fileIdentity)
	}
	if prefix == "" || segment == prefix {
		if segment == prefix {
			return 1, nil
		}
		return 0, errors.New("conversation source segment has an unsupported identity")
	}
	if !strings.HasPrefix(segment, prefix+":") {
		return 0, errors.New("conversation source segment and file identity differ")
	}
	generation, err := strconv.ParseUint(strings.TrimPrefix(segment, prefix+":"), 10, 64)
	if err != nil || generation < 1 {
		return 0, errors.New("conversation source generation is invalid")
	}
	return generation, nil
}

type ConversationSourceMarkerInput struct {
	ProjectID, WorkflowID, ExecutionID, AttemptID string
	Agent, TargetID, SourceIdentity               string
	ReaderCommand                                 contracts.Command
	CodexHome                                     string
	CreatedAt                                     time.Time
}

type ConversationSourceMarker struct {
	Version        int                            `json:"v"`
	ProjectID      string                         `json:"project_id"`
	WorkflowID     string                         `json:"workflow_id"`
	ExecutionID    string                         `json:"execution_id"`
	AttemptID      string                         `json:"attempt_id"`
	Backend        executionapi.BackendKind       `json:"backend"`
	Protocol       executionapi.ExecutionProtocol `json:"protocol"`
	Agent          string                         `json:"agent"`
	TargetID       string                         `json:"target_id"`
	Source         string                         `json:"source"`
	SourceSegment  string                         `json:"source_segment"`
	SourceIdentity string                         `json:"source_identity"`
	RecordName     string                         `json:"record_name"`
	CreatedAt      time.Time                      `json:"created_at"`
	ReaderCommand  contracts.Command              `json:"reader_command,omitempty"`
	CodexHome      string                         `json:"codex_home,omitempty"`
}

func (m ConversationSourceMarker) Validate() error {
	local := m.Backend == executionapi.BackendLocal && m.Protocol == executionapi.ProtocolLocalProcess && m.SourceSegment == JournalSegment(m.Backend, m.Protocol, m.SourceIdentity)
	orca := m.Backend == executionapi.BackendOrca && (m.Protocol == executionapi.ProtocolOrcaTerminalV2 || m.Protocol == executionapi.ProtocolOrcaSupervised) && m.SourceSegment == JournalSegment(m.Backend, m.Protocol, m.SourceIdentity)
	if m.Version != 1 || m.ProjectID == "" || m.WorkflowID == "" || m.ExecutionID == "" || m.AttemptID == "" || !local && !orca || m.Agent == "" || m.TargetID == "" || m.Source != ConversationJSONLSource || m.SourceIdentity == "" || m.RecordName != "events.jsonl" || m.CreatedAt.IsZero() {
		return errors.New("conversation source marker is incomplete or unsupported")
	}
	return nil
}

func WriteConversationSourceMarker(path string, input ConversationSourceMarkerInput, writeAtomic func(string, []byte) error) error {
	marker := ConversationSourceMarker{Version: 1, ProjectID: input.ProjectID, WorkflowID: input.WorkflowID, ExecutionID: input.ExecutionID, AttemptID: input.AttemptID,
		Backend: executionapi.BackendLocal, Protocol: executionapi.ProtocolLocalProcess, Agent: input.Agent, TargetID: input.TargetID, Source: ConversationJSONLSource,
		SourceSegment: LocalProcessJournalSegment(input.SourceIdentity), SourceIdentity: input.SourceIdentity, RecordName: "events.jsonl",
		CreatedAt: input.CreatedAt, ReaderCommand: input.ReaderCommand, CodexHome: input.CodexHome}
	if marker.CreatedAt.IsZero() {
		marker.CreatedAt = time.Now().UTC()
	}
	if err := marker.Validate(); err != nil {
		return err
	}
	data, err := json.Marshal(marker)
	if err != nil {
		return err
	}
	return writeAtomic(path, append(data, '\n'))
}

type HistoryBinding struct {
	Version     int                            `json:"v"`
	ProjectID   string                         `json:"project_id"`
	TaskID      string                         `json:"task_id"`
	WorkflowID  string                         `json:"workflow_id"`
	ExecutionID string                         `json:"execution_id"`
	AttemptID   string                         `json:"attempt_id"`
	Backend     executionapi.BackendKind       `json:"backend"`
	Protocol    executionapi.ExecutionProtocol `json:"protocol"`
	TargetID    string                         `json:"target_id"`
	ExternalRef string                         `json:"external_ref"`
	Workspace   string                         `json:"workspace"`
	SessionID   string                         `json:"session_id"`
	Command     contracts.Command              `json:"command"`
	Home        string                         `json:"codex_home,omitempty"`
	Ended       bool                           `json:"-"`
}

func (b HistoryBinding) Validate() error {
	if b.Version != 1 || b.ProjectID == "" || b.ExecutionID == "" || b.AttemptID == "" || b.WorkflowID == "" || !filepath.IsAbs(b.Workspace) || b.Command.Executable == "" || !ValidNativeID(b.SessionID) {
		return errors.New("Codex conversation binding is incomplete")
	}
	if b.Backend != executionapi.BackendLocal && b.Backend != executionapi.BackendOrca {
		return errors.New("Codex history backend is unsupported")
	}
	if b.Backend == executionapi.BackendLocal && b.Protocol != executionapi.ProtocolLocalProcess && b.Protocol != executionapi.ProtocolLocalCodexAppServer || b.Backend == executionapi.BackendOrca && b.Protocol != executionapi.ProtocolOrcaTerminalV2 {
		return errors.New("Codex history protocol does not match backend")
	}
	return nil
}
func (b HistoryBinding) Identity() string {
	return "codex-app-server-v1:" + digestValue(struct {
		Session, Home, Workspace string
		Command                  contracts.Command
	}{b.SessionID, b.Home, canonicalPath(b.Workspace), b.Command})
}
func (b HistoryBinding) Event() contracts.ConversationEvent {
	return contracts.ConversationEvent{Version: contracts.ConversationVersion, ProjectID: b.ProjectID, TaskID: b.TaskID, WorkflowID: b.WorkflowID, ExecutionID: b.ExecutionID, AttemptID: b.AttemptID, Backend: b.Backend, Protocol: b.Protocol, Agent: "codex", TargetID: b.TargetID, ExternalRef: b.ExternalRef, Source: "codex-app-server", SourceRole: contracts.ConversationSourceCanonical, SourceSegment: b.Identity(), SourceIdentity: b.Identity(), SessionID: b.SessionID, SessionQuality: contracts.IdentityNative, TurnQuality: contracts.IdentityUnknown, ItemQuality: contracts.IdentityUnknown, SourceEventQuality: contracts.IdentityDerived, Phase: contracts.MessagePhaseUnknown, ReceivedAt: time.Now().UTC()}
}
func WriteHistoryBinding(path string, b HistoryBinding, writeAtomic func(string, []byte) error) error {
	if old, err := os.ReadFile(path); err == nil {
		var previous HistoryBinding
		if json.Unmarshal(old, &previous) != nil || previous.Identity() != b.Identity() || previous.ProjectID != b.ProjectID || previous.WorkflowID != b.WorkflowID || previous.ExecutionID != b.ExecutionID || previous.AttemptID != b.AttemptID || previous.Backend != b.Backend || previous.Protocol != b.Protocol || previous.TargetID != b.TargetID {
			return errors.New("Codex session changed within the same attempt")
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := b.Validate(); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(path, append(raw, '\n'))
}

func ReadHistoryBinding(path string) (HistoryBinding, error) {
	var b HistoryBinding
	raw, err := os.ReadFile(path)
	if err != nil {
		return b, err
	}
	if err = json.Unmarshal(raw, &b); err != nil {
		return b, err
	}
	return b, b.Validate()
}

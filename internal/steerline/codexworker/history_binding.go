package codexworker

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
)

const ConversationSourceMarkerName = "conversation-source.v1.json"

const ConversationJSONLSource = "codex-jsonl"

func LocalProcessJournalSegment(identity string) string {
	return JournalSegment(executionbackend.BackendLocal, executionbackend.ProtocolLocalProcess, identity)
}

func JournalSource(backend executionbackend.BackendKind, protocol executionbackend.ExecutionProtocol) string {
	if backend == executionbackend.BackendLocal && (protocol == executionbackend.ProtocolLocalProcess || protocol == executionbackend.ProtocolLocalCodexAppServer) ||
		backend == executionbackend.BackendOrca && (protocol == executionbackend.ProtocolOrcaTerminalV2 || protocol == executionbackend.ProtocolOrcaSupervised) {
		return ConversationJSONLSource
	}
	return ""
}

func JournalSegment(backend executionbackend.BackendKind, protocol executionbackend.ExecutionProtocol, identity string) string {
	prefix := ""
	if backend == executionbackend.BackendLocal && (protocol == executionbackend.ProtocolLocalProcess || protocol == executionbackend.ProtocolLocalCodexAppServer) {
		prefix = "local-jsonl-v1:"
	} else if backend == executionbackend.BackendOrca && (protocol == executionbackend.ProtocolOrcaTerminalV2 || protocol == executionbackend.ProtocolOrcaSupervised) {
		prefix = "orca-jsonl-v1:"
	}
	if prefix == "" {
		return ""
	}
	return prefix + identity
}

func JournalGeneration(fileIdentity, segment string) (uint64, error) {
	prefix := JournalSegment(executionbackend.BackendLocal, executionbackend.ProtocolLocalProcess, fileIdentity)
	if strings.HasPrefix(segment, JournalSegment(executionbackend.BackendOrca, executionbackend.ProtocolOrcaTerminalV2, fileIdentity)) {
		prefix = JournalSegment(executionbackend.BackendOrca, executionbackend.ProtocolOrcaTerminalV2, fileIdentity)
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

type ConversationSourceMarker struct {
	Version        int                                `json:"v"`
	ProjectID      string                             `json:"project_id"`
	WorkflowID     string                             `json:"workflow_id"`
	ExecutionID    string                             `json:"execution_id"`
	AttemptID      string                             `json:"attempt_id"`
	Backend        executionbackend.BackendKind       `json:"backend"`
	Protocol       executionbackend.ExecutionProtocol `json:"protocol"`
	Agent          string                             `json:"agent"`
	TargetID       string                             `json:"target_id"`
	Source         string                             `json:"source"`
	SourceSegment  string                             `json:"source_segment"`
	SourceIdentity string                             `json:"source_identity"`
	RecordName     string                             `json:"record_name"`
	CreatedAt      time.Time                          `json:"created_at"`
	ReaderCommand  executionbackend.Command           `json:"reader_command,omitempty"`
	CodexHome      string                             `json:"codex_home,omitempty"`
}

func (m ConversationSourceMarker) Validate() error {
	local := m.Backend == executionbackend.BackendLocal && m.Protocol == executionbackend.ProtocolLocalProcess && m.SourceSegment == JournalSegment(m.Backend, m.Protocol, m.SourceIdentity)
	orca := m.Backend == executionbackend.BackendOrca && (m.Protocol == executionbackend.ProtocolOrcaTerminalV2 || m.Protocol == executionbackend.ProtocolOrcaSupervised) && m.SourceSegment == JournalSegment(m.Backend, m.Protocol, m.SourceIdentity)
	if m.Version != 1 || m.ProjectID == "" || m.WorkflowID == "" || m.ExecutionID == "" || m.AttemptID == "" || !local && !orca || m.Agent == "" || m.TargetID == "" || m.Source != ConversationJSONLSource || m.SourceIdentity == "" || m.RecordName != "events.jsonl" || m.CreatedAt.IsZero() {
		return errors.New("conversation source marker is incomplete or unsupported")
	}
	return nil
}

type HistoryBinding struct {
	Version     int                                `json:"v"`
	ProjectID   string                             `json:"project_id"`
	TaskID      string                             `json:"task_id"`
	WorkflowID  string                             `json:"workflow_id"`
	ExecutionID string                             `json:"execution_id"`
	AttemptID   string                             `json:"attempt_id"`
	Backend     executionbackend.BackendKind       `json:"backend"`
	Protocol    executionbackend.ExecutionProtocol `json:"protocol"`
	TargetID    string                             `json:"target_id"`
	ExternalRef string                             `json:"external_ref"`
	Workspace   string                             `json:"workspace"`
	SessionID   string                             `json:"session_id"`
	Command     executionbackend.Command           `json:"command"`
	Home        string                             `json:"codex_home,omitempty"`
	Ended       bool                               `json:"-"`
}

func (b HistoryBinding) Validate() error {
	if b.Version != 1 || b.ProjectID == "" || b.ExecutionID == "" || b.AttemptID == "" || b.WorkflowID == "" || !filepath.IsAbs(b.Workspace) || b.Command.Executable == "" || !ValidNativeID(b.SessionID) {
		return errors.New("Codex conversation binding is incomplete")
	}
	if b.Backend != executionbackend.BackendLocal && b.Backend != executionbackend.BackendOrca {
		return errors.New("Codex history backend is unsupported")
	}
	if b.Backend == executionbackend.BackendLocal && b.Protocol != executionbackend.ProtocolLocalProcess && b.Protocol != executionbackend.ProtocolLocalCodexAppServer || b.Backend == executionbackend.BackendOrca && b.Protocol != executionbackend.ProtocolOrcaTerminalV2 {
		return errors.New("Codex history protocol does not match backend")
	}
	return nil
}

func (b HistoryBinding) Identity() string {
	return "codex-app-server-v1:" + digestValue(struct {
		Session, Home, Workspace string
		Command                  executionbackend.Command
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
	raw, err := readConversationSourceFile(path)
	if err != nil {
		return b, err
	}
	if err = executionbackend.DecodeStrictJSON(raw, &b, "conversation source binding has trailing data"); err != nil {
		return b, err
	}
	return b, b.Validate()
}

func readConversationSourceFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxConversationCursorBytes {
		return nil, errors.New("conversation source binding must be a regular file within 64 KiB")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxConversationCursorBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxConversationCursorBytes {
		return nil, errors.New("conversation source binding exceeds 64 KiB")
	}
	return raw, nil
}

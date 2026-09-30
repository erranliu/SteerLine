package codexworker

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

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
	return contracts.ConversationEvent{Version: 1, ProjectID: b.ProjectID, TaskID: b.TaskID, WorkflowID: b.WorkflowID, ExecutionID: b.ExecutionID, AttemptID: b.AttemptID, Backend: b.Backend, Protocol: b.Protocol, Agent: "codex", TargetID: b.TargetID, ExternalRef: b.ExternalRef, Source: "codex-app-server", SourceSegment: b.Identity(), SourceIdentity: b.Identity(), SessionID: b.SessionID, SessionQuality: contracts.IdentityNative, TurnQuality: contracts.IdentityUnknown, ItemQuality: contracts.IdentityUnknown, SourceEventQuality: contracts.IdentityDerived, Phase: contracts.MessagePhaseUnknown, ReceivedAt: time.Now().UTC()}
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

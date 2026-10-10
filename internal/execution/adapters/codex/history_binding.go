package codexworker

import (
	"encoding/json"
	"errors"
	executionvalues "github.com/erranliu/SteerLine/internal/execution/model"
	"io"
	"os"
	"path/filepath"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

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
	if b.Backend != executionbackend.BackendLocal {
		return errors.New("Codex history backend is unsupported")
	}
	if b.Protocol != executionbackend.ProtocolLocalProcess && b.Protocol != executionbackend.ProtocolLocalCodexAppServer {
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

func (b HistoryBinding) Event() executionvalues.ConversationEvent {
	return executionvalues.ConversationEvent{Version: executionvalues.ConversationVersion, ProjectID: b.ProjectID, TaskID: b.TaskID, WorkflowID: b.WorkflowID, ExecutionID: b.ExecutionID, AttemptID: b.AttemptID, Backend: b.Backend, Protocol: b.Protocol, Agent: "codex", TargetID: b.TargetID, ExternalRef: b.ExternalRef, Source: "codex-app-server", SourceRole: executionvalues.ConversationSourceCanonical, SourceSegment: b.Identity(), SourceIdentity: b.Identity(), SessionID: b.SessionID, SessionQuality: executionvalues.IdentityNative, TurnQuality: executionvalues.IdentityUnknown, ItemQuality: executionvalues.IdentityUnknown, SourceEventQuality: executionvalues.IdentityDerived, Phase: executionvalues.MessagePhaseUnknown, ReceivedAt: time.Now().UTC()}
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

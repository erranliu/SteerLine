package orca_remote_worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/erranliu/SteerLine/internal/execution/adapters/orca"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

type Settings struct{ ConnectionID, EnvironmentID, RepositorySelector, CoordinatorTerminal, ExpectedCodexVersion, PollInterval string }

type Config struct {
	ExpectedRemoteRuntime                                                string
	Settings                                                             Settings
	Repository, ProjectID, Agent, Model, ReasoningEffort, CommandTimeout string
	Call                                                                 func(context.Context, time.Duration, ...string) ([]byte, error)
	Origin                                                               func(context.Context) ([]byte, error)
}

func (c Config) Validate() error {
	if c.Call == nil || c.Origin == nil || c.Settings.ConnectionID == "" || c.Settings.EnvironmentID == "" || c.Settings.RepositorySelector == "" {
		return errors.New("remote Orca requires fixed connection, environment and repository")
	}
	return nil
}

type Client struct{ config Config }

func New(c Config) Client { return Client{config: c} }

type Message = orcaworker.Message

type Ref struct {
	Backend                                           executionbackend.BackendKind
	Protocol                                          executionbackend.ExecutionProtocol
	RunID, WorkspacePath, WorkspaceBranch, BaseCommit string
	Target                                            executionbackend.ExecutionTarget
	External                                          executionbackend.ProcessIdentity
}

type Workspace struct {
	Repository, Worktree, Branch, Base, Head string
	Dirty                                    bool
	CheckedAt                                time.Time
}

type Error struct {
	Code       executionbackend.BackendErrorCode
	Diagnostic string
	Cause      error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Diagnostic, e.Cause)
	}
	return string(e.Code) + ": " + e.Diagnostic
}

func (e *Error) Unwrap() error { return e.Cause }

var commitHash = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

func now() time.Time { return time.Now().UTC() }

func positiveDuration(s string, d time.Duration) time.Duration {
	if value, e := time.ParseDuration(s); e == nil && value > 0 {
		return value
	}
	return d
}

func duration(s string) time.Duration { v, _ := time.ParseDuration(s); return v }

var stableID = regexp.MustCompile(`^[0-9a-f]{32}$`)

func stableExecutionID(s string) bool { return stableID.MatchString(s) }

func validateSupervisedRef(c Config, ref Ref) error {
	if ref.Backend != executionbackend.BackendOrca || ref.Protocol != executionbackend.ProtocolOrcaSupervised || ref.Target.ID != "orca-supervised:"+c.Settings.ConnectionID || ref.Target.ConnectionID != c.Settings.ConnectionID || ref.Target.ExternalRuntime != c.Settings.EnvironmentID || ref.Target.OrchestrationRun != ref.External.OrchestrationRun || ref.Target.ExternalTask != ref.External.Task || ref.Target.Dispatch != ref.External.Dispatch || ref.Target.ExecutionHost != ref.External.ExecutionHost || ref.Target.WorktreeID != ref.External.WorktreeID || ref.External.Kind != "orca_supervised" || ref.External.RuntimeID == "" || ref.External.CoordinatorRuntimeID == "" || ref.External.Task == "" || ref.External.Dispatch == "" || ref.External.ExecutionHost != c.Settings.EnvironmentID || ref.External.WorktreeID == "" {
		return &Error{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "remote Orca reference differs from its fixed target/Dispatch"}
	}
	return nil
}

type orcaEnvelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    string          `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data,omitempty"`
	} `json:"error"`
	Meta struct {
		RuntimeID string `json:"runtimeId"`
	} `json:"_meta"`
}

func parseEnvelope(b []byte) (orcaEnvelope, error) {
	b = bytes.TrimPrefix(b, []byte{0xef, 0xbb, 0xbf})
	var e orcaEnvelope
	if err := json.Unmarshal(b, &e); err != nil {
		return e, err
	}
	if !e.OK {
		msg := "orca command failed"
		if e.Error != nil && e.Error.Message != "" {
			msg = e.Error.Message
		}
		return e, errors.New(msg)
	}
	return e, nil
}

const MaxMessageBytes = 8 << 10

const maxSupervisedMessageBytes = MaxMessageBytes

type PreflightResult struct {
	CenterVersion string
	RemoteVersion string
	CenterRuntime string
	RemoteRuntime string
	EnvironmentID string
	RepositoryID  string
	RepositoryURL string
	Coordinator   string
}

type StartReceipt struct {
	RunID             string
	TaskID            string
	DispatchID        string
	WorktreeID        string
	CenterRuntimeID   string
	PlatformRequestID string
	State             string
	Stage             string
}

type WorkerObservation struct {
	Identity       executionbackend.ProcessIdentity
	Liveness       string
	DispatchState  string
	AgentWaitState string
	WorktreePath   string
	Branch         string
	ObservedAt     time.Time
}

type Delivery struct {
	ID       string
	Messages []Message
	TimedOut bool
}

type orcaRuntimeStatus struct {
	State        string   `json:"state"`
	Reachable    bool     `json:"reachable"`
	RuntimeID    string   `json:"runtimeId"`
	AppVersion   string   `json:"appVersion"`
	Capabilities []string `json:"capabilities"`
}

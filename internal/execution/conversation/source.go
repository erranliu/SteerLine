package conversation

import (
	"context"
	"os/exec"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

type ConversationSourcePort interface {
	DiscoverHistory(context.Context, ConversationDiscovery) (ConversationDiscoveryResult, error)
	ReadHistory(context.Context, ConversationHistoryReadRequest, func(*exec.Cmd) (executionbackend.StopHandle, error)) (ConversationHistoryBatch, error)
}

type ConversationDiscovery struct {
	ProjectID, RunDirectory, Repository string
	Runs                                []ConversationRunSource
}

type ConversationRunSource struct {
	Base         executionmodel.ConversationEvent
	RunDirectory string
	Workspace    string
	WorkerFlow   bool
	Process      *executionbackend.ProcessIdentity
	Attempts     []ConversationAttemptSource
}

type ConversationAttemptSource struct {
	AttemptID string
	EventPath string
	FileID    string
	Ended     bool
}

type ConversationHistorySource struct {
	Identity                string
	BindingPath             string
	Workspace               string
	Event                   executionmodel.ConversationEvent
	Ended                   bool
	RequiresPlatformVersion bool
}

type ConversationDiscoveryResult struct {
	Sources []ConversationHistorySource
	Writes  []executionmodel.ConversationWrite
}

type ConversationHistoryReadRequest struct {
	Source          ConversationHistorySource
	Cursor          string
	HasCheckpoint   bool
	Repository      string
	RunDirectory    string
	PlatformVersion string
}

type ConversationHistoryBatch struct {
	Writes       []executionmodel.ConversationWrite
	Cursor       string
	Sequence     uint64
	Settled      bool
	More         bool
	Capabilities *executionmodel.ConversationCapabilityReport
}

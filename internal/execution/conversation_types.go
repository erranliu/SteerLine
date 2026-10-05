package execution

import (
	"context"
	"os/exec"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

type ConversationMapper interface {
	MapRecord(executionmodel.ConversationEvent, []byte) executionmodel.ConversationWrite
}

type ConversationInputMapper interface {
	MapConversation(executionmodel.ConversationInput) (executionmodel.ConversationMappedBatch, error)
}

type ConversationSourcePort interface {
	ConversationInputMapper
	RestoreMapper(executionmodel.ConversationMapperSnapshot) ConversationMapper
	JournalSources() []string
	JournalSource(executionbackend.BackendKind, executionbackend.ExecutionProtocol) string
	JournalSegment(executionbackend.BackendKind, executionbackend.ExecutionProtocol, string) string
	JournalGeneration(fileIdentity, segment string) (uint64, error)
	LiveJournalPath(context.Context, ConversationJournalPathRequest) (string, error)
	DiscoverHistory(context.Context, ConversationDiscovery) (ConversationDiscoveryResult, error)
	DiscoverJournals(context.Context, ConversationDiscovery, func(string) (string, error)) ([]ConversationJournalSource, error)
	ReadHistory(context.Context, ConversationHistoryReadRequest, func(*exec.Cmd) (executionbackend.StopHandle, error)) (ConversationHistoryBatch, error)
}

type ConversationJournalPathRequest struct {
	RunDirectory, WorkflowID, ExecutionID, AttemptID, SourceIdentity, LogPath string
	EventPaths                                                                []string
	WorkerFlow                                                                bool
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

type ConversationJournalSource struct {
	Path, AttemptID, FileID, ExpectedFileID, Diagnostic, Workspace string
	Event                                                          executionmodel.ConversationEvent
	Ended                                                          bool
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

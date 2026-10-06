package execution

import (
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

// Config is the immutable execution value consumed by claim, worker and
// recovery operations. It contains one selected profile and never owns the
// application catalog, task admission rules, host timers or runtime adapters.
type Config struct {
	PublicationPolicy     executionbackend.PublicationPolicy            `json:"publication_policy"`
	ExecutionPolicy       executionbackend.ExecutionPolicy              `json:"execution_policy"`
	ExecutionRequirements executionbackend.ExecutionRequirements        `json:"execution_requirements,omitempty"`
	Repository            string                                        `json:"repository"`
	Remote                string                                        `json:"remote"`
	Branch                string                                        `json:"branch"`
	RunDir                string                                        `json:"run_dir"`
	WorktreeRoot          string                                        `json:"worktree_root"`
	Git                   executionbackend.Command                      `json:"git"`
	ImplementationData    []executionbackend.ImplementationDataEnvelope `json:"-"`
	ImplementationDisplay []string                                      `json:"-"`
	WorkflowProtocol      string                                        `json:"workflow_protocol,omitempty"`
	CommandTimeout        string                                        `json:"command_timeout"`
	PR                    executionmodel.PRProviderConfig               `json:"pr"`
	Host                  string                                        `json:"host"`
	Agent                 string                                        `json:"agent"`
	WorkspaceBranchPrefix string                                        `json:"-"`

	identity               string
	fixedProfileDigest     string
	acceptedConfigVersion  string
	selectedProfileID      string
	publicationRemoteURL   string
	frozenExecutionConfig  bool
	requirementsEvaluation *executionbackend.RequirementsEvaluation
}

// RuntimeDependencies are injected when an execution service is composed.
// They are not copied into workflow or worker snapshots.
type RuntimeDependencies struct {
	Directory      *ExecutionDirectory
	ProcessHost    executionbackend.ManagedProcessHost
	Profiles       ProfilePort
	ConfigVerifier ConfigVerifier
}

// ProfilePort resolves one profile by identity and digest. Implementations are
// app-owned; execution receives only the selected frozen value.
type ProfilePort interface {
	DefaultProfileID() string
	HasMultipleProfiles() bool
	ResolveExecutionProfile(string, string) (Config, executionmodel.FixedExecutionProfile, error)
}

// ConfigVerifier checks that the app-loaded project/global config bytes are
// still the bytes accepted by the selected profile.
type ConfigVerifier interface {
	VerifyCurrentConfig() error
}

// RuntimeSettings is intentionally limited to app/host facts used to publish
// the service checkpoint. Scheduling itself stays in the host package.
type RuntimeSettings struct {
	Claimant      string
	Heartbeat     time.Duration
	ShutdownGrace time.Duration
}

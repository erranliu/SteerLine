package execution

import (
	"context"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	contracts "github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/internal/steerline/persistence"
	"github.com/erranliu/SteerLine/internal/steerline/publication"
)

type Config struct {
	ReadRuntime             func(context.Context, Config) (*contracts.Runtime, error) `json:"-"`
	Resources               ResourcePort                                              `json:"-"`
	Persistence             persistence.Factory                                       `json:"-"`
	ExecutionDirectory      *ExecutionDirectory                                       `json:"-"`
	ProcessHost             executionbackend.ManagedProcessHost                       `json:"-"`
	WorkspaceBranchPrefix   string                                                    `json:"-"`
	StateRoot               string                                                    `json:"-"`
	RepositoryStateKey      string                                                    `json:"-"`
	fixedProfileDigest      string
	frozenExecutionConfig   bool
	requirementsEvaluation  *executionbackend.RequirementsEvaluation
	executionConfigPath     string
	publicationRemoteURL    string
	selectedProfileID       string
	PublicationPolicy       executionbackend.PublicationPolicy     `json:"publication_policy"`
	ExecutionPolicy         executionbackend.ExecutionPolicy       `json:"execution_policy"`
	ExecutionRequirements   executionbackend.ExecutionRequirements `json:"execution_requirements,omitempty"`
	policySource            string
	Repository              string                                        `json:"repository"`
	TaskAuthority           string                                        `json:"task_authority,omitempty"`
	Remote                  string                                        `json:"remote"`
	Branch                  string                                        `json:"branch"`
	RunDir                  string                                        `json:"run_dir"`
	WorktreeRoot            string                                        `json:"worktree_root"`
	Claimant                string                                        `json:"claimant"`
	Git                     executionbackend.Command                      `json:"git"`
	ImplementationData      []executionbackend.ImplementationDataEnvelope `json:"-"`
	ImplementationDisplay   []string                                      `json:"-"`
	WorkflowProtocol        string                                        `json:"workflow_protocol,omitempty"`
	ExecutableTypes         []string                                      `json:"executable_types"`
	TaskPoll                string                                        `json:"task_poll"`
	PRPoll                  string                                        `json:"pr_poll"`
	Heartbeat               string                                        `json:"heartbeat"`
	CommandTimeout          string                                        `json:"command_timeout"`
	ShutdownGrace           string                                        `json:"shutdown_grace"`
	Agent                   string                                        `json:"agent"`
	Host                    string                                        `json:"host"`
	ExecutionProfiles       []ExecutionProfile                            `json:"execution_profiles,omitempty"`
	DefaultExecutionProfile string                                        `json:"default_execution_profile,omitempty"`
	StallTimeout            string                                        `json:"stall_timeout"`
	PR                      publication.Config                            `json:"pr"`
	identity                string
	workerEnv               map[string]string
	configDigest            string
	globalExecution         bool
}

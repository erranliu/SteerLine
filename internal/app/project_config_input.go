package app

import (
	host "github.com/erranliu/SteerLine/internal/app/host"
	execution "github.com/erranliu/SteerLine/internal/execution"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

// ProjectConfigInput is the app-owned on-disk project format. Runtime ports,
// storage handles, execution directories and host process facts are composed
// after decoding and never enter the serialized input value.
type ProjectConfigInput struct {
	PublicationPolicy executionbackend.PublicationPolicy `json:"publication_policy"`

	Repository              string                          `json:"repository"`
	TaskAuthority           string                          `json:"task_authority,omitempty"`
	Remote                  string                          `json:"remote"`
	Branch                  string                          `json:"branch"`
	RunDir                  string                          `json:"run_dir"`
	WorktreeRoot            string                          `json:"worktree_root"`
	Claimant                string                          `json:"claimant"`
	Git                     executionbackend.Command        `json:"git"`
	WorkflowProtocol        string                          `json:"workflow_protocol,omitempty"`
	ExecutableTypes         []string                        `json:"executable_types"`
	TaskPoll                string                          `json:"task_poll"`
	PRPoll                  string                          `json:"pr_poll"`
	Heartbeat               string                          `json:"heartbeat"`
	CommandTimeout          string                          `json:"command_timeout"`
	ShutdownGrace           string                          `json:"shutdown_grace"`
	Agent                   string                          `json:"agent"`
	Host                    string                          `json:"host"`
	ExecutionProfiles       []execution.ExecutionProfile    `json:"execution_profiles,omitempty"`
	DefaultExecutionProfile string                          `json:"default_execution_profile,omitempty"`
	StallTimeout            string                          `json:"stall_timeout"`
	PR                      executionmodel.PRProviderConfig `json:"pr"`
}

// Config is the app-owned loaded/composed project configuration. Project files
// decode through ProjectConfigInput; catalog, directory, process host and file
// identity are added by app composition.
type Config struct {
	runtimeHost *host.Config // fixed only by the owning ProjectRuntime after admission

	ConfigPath string `json:"-"`

	PublicationPolicy executionbackend.PublicationPolicy `json:"publication_policy"`

	Repository              string                              `json:"repository"`
	TaskAuthority           string                              `json:"task_authority,omitempty"`
	Remote                  string                              `json:"remote"`
	Branch                  string                              `json:"branch"`
	RunDir                  string                              `json:"run_dir"`
	WorktreeRoot            string                              `json:"worktree_root"`
	Claimant                string                              `json:"claimant"`
	Git                     executionbackend.Command            `json:"git"`
	WorkflowProtocol        string                              `json:"workflow_protocol,omitempty"`
	ExecutableTypes         []string                            `json:"executable_types"`
	TaskPoll                string                              `json:"task_poll"`
	PRPoll                  string                              `json:"pr_poll"`
	Heartbeat               string                              `json:"heartbeat"`
	CommandTimeout          string                              `json:"command_timeout"`
	ShutdownGrace           string                              `json:"shutdown_grace"`
	Agent                   string                              `json:"agent"`
	Host                    string                              `json:"host"`
	ExecutionProfiles       []execution.ExecutionProfile        `json:"execution_profiles,omitempty"`
	DefaultExecutionProfile string                              `json:"default_execution_profile,omitempty"`
	StallTimeout            string                              `json:"stall_timeout"`
	PR                      executionmodel.PRProviderConfig     `json:"pr"`
	ExecutionDirectory      *execution.ExecutionDirectory       `json:"-"`
	ProcessHost             executionbackend.ManagedProcessHost `json:"-"`
	projectPath             string
	executionPath           string
	configDigest            string
	identity                string
	globalExecution         bool
	selectedProfileID       string
	activeProfile           *execution.ExecutionProfile
}

func configFromProjectInput(input ProjectConfigInput) Config {
	return Config{
		PublicationPolicy: input.PublicationPolicy,
		Repository:        input.Repository, TaskAuthority: input.TaskAuthority, Remote: input.Remote, Branch: input.Branch, RunDir: input.RunDir,
		WorktreeRoot: input.WorktreeRoot, Claimant: input.Claimant, Git: input.Git, WorkflowProtocol: input.WorkflowProtocol,
		ExecutableTypes: input.ExecutableTypes, TaskPoll: input.TaskPoll, PRPoll: input.PRPoll, Heartbeat: input.Heartbeat,
		CommandTimeout: input.CommandTimeout, ShutdownGrace: input.ShutdownGrace, Agent: input.Agent, Host: input.Host,
		ExecutionProfiles: input.ExecutionProfiles, DefaultExecutionProfile: input.DefaultExecutionProfile, StallTimeout: input.StallTimeout, PR: input.PR,
	}
}

func defaultProjectConfigInput() ProjectConfigInput {
	return ProjectConfigInput{
		Remote: "origin", TaskPoll: "10s", PRPoll: "30s", Heartbeat: "5s", CommandTimeout: "30s", ShutdownGrace: "10s",
		Git: executionbackend.Command{Executable: "git"}, ExecutableTypes: []string{"task", "bug", "feature", "chore"}, StallTimeout: "5m",
	}
}

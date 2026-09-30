package steerline

import (
	"errors"
	"fmt"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

func (s *Service) projectExecutionConfig() Config {
	if s.baseConfig.Repository != "" {
		return s.baseConfig
	}
	return s.Config // Test fixtures that construct a Service directly.
}

func (s *Service) selectDispatchProfile(id, digest string) error {
	base := s.projectExecutionConfig()
	selected, profile, err := base.ResolveExecutionProfile(id, digest)
	if err != nil {
		return err
	}
	selected = copyWorkerFlowConfig(selected)
	s.dispatchConfig = &selected
	s.dispatchProfile = &profile
	return nil
}

func (s *Service) selectDefaultDispatchProfile() error {
	base := s.projectExecutionConfig()
	id := base.DefaultProfileID()
	if id == "" {
		return errors.New("automatic dispatch requires default_execution_profile")
	}
	return s.selectDispatchProfile(id, "")
}

func (s *Service) executionConfigForRun(run *Run) (Config, error) {
	base := s.projectExecutionConfig()
	if run == nil {
		return base, nil
	}
	for _, workflow := range s.domain.WorkflowRuns {
		if workflow.WorkflowRunID != run.RunID {
			continue
		}
		if workflow.ExecutionProfile == nil {
			return Config{}, errors.New("execution snapshot v3 is required; preserve original state and external session")
		}
		fixed := *workflow.ExecutionProfile
		if fixed.ID == "" || !verifyFixedExecutionProfile(fixed) || workflow.ConfigDigest != fixed.AcceptedConfigVersion {
			return Config{}, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "fixed execution profile identity is incomplete"}
		}
		config := base.ApplyFixedExecutionProfile(fixed)
		if err := validateWorkflowConfig(&config, nil); err != nil {
			return Config{}, err
		}
		for _, e := range s.domain.Executions {
			if e.WorkflowRunID == workflow.WorkflowRunID && e.PolicyResources == nil && (e.Process != nil || e.SessionID != "") {
				return Config{}, errors.New("bound execution lost its original policy resource snapshot")
			}
			if e.WorkflowRunID == workflow.WorkflowRunID && e.PolicyResources != nil {
				if err := verifyPolicyResources(e.PolicyResources, fixed.Digest, e.Workspace.Worktree); err != nil {
					return Config{}, err
				}
				config.Codex.Executable = e.PolicyResources.Settings.Executable
			}
		}
		config.WorkflowProtocol = WorkerFlowVersion
		config.Repository, config.Branch = workflow.Repository, fixed.Publication.BaseBranch
		if fixed.Publication.RemoteURL == "" {
			return Config{}, errors.New("fixed publication remote URL is missing; preserve original state")
		}
		if fixed.Host == "orca_supervised" && run.Target.CoordinatorTerminal != "" {
			if config.Orca.Supervised == nil {
				return Config{}, errors.New("fixed supervised profile has no coordinator configuration")
			}
			coordinator := *config.Orca.Supervised
			coordinator.CoordinatorTerminal = run.Target.CoordinatorTerminal
			config.Orca.Supervised = &coordinator
		}
		if workflow.WorkflowVersion != WorkerFlowVersion {
			return Config{}, fmt.Errorf("fixed execution profile uses an unsupported workflow version")
		}
		backend, protocol, err := executionapi.ExecutionBackendKind(fixed.Host)
		if workflow.WorkflowVersion == WorkerFlowVersion && backend == string(executionapi.BackendLocal) {
			protocol = string(executionapi.ProtocolLocalCodexAppServer)
		}
		if err != nil || run.Host != fixed.Host || run.Agent != fixed.Agent || run.Backend != string(backend) || run.Protocol != string(protocol) || run.Target.ID != targetFor(config).ID {
			return Config{}, errors.New("fixed execution profile differs from the persisted run identity")
		}
		return config, nil
	}
	return Config{}, errors.New("run has no persisted workflow for its execution profile")
}

func (s *Service) executionConfigForWorkflowID(id string) (Config, error) {
	for _, run := range s.runs {
		if run != nil && run.RunID == id {
			return s.executionConfigForRun(run)
		}
	}
	return Config{}, errors.New("workflow has no persisted run for its execution profile")
}

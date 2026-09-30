package steerline

import (
	"errors"
	"fmt"
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
			if len(base.ExecutionProfiles) > 0 && (run.ExecutionStatus == "pending" || run.ExecutionStatus == "running" || run.ExecutionStatus == "unknown" || run.FinishTaskStatus == "waiting_decision") {
				return Config{}, errors.New("active run has no fixed execution profile; preserve its original service configuration")
			}
			return base, nil
		}
		fixed := *workflow.ExecutionProfile
		if fixed.ID == "" || !fixed.VerifyWorkerFlow() {
			return Config{}, errors.New("fixed execution profile identity is incomplete")
		}
		config := base.ApplyFixedExecutionProfile(fixed)
		config.WorkflowProtocol = WorkerFlowVersion
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
		backend, protocol, err := ExecutionBackendKind(fixed.Host)
		if workflow.WorkflowVersion == WorkerFlowVersion && backend == string(BackendLocal) {
			protocol = string(ProtocolLocalCodexAppServer)
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

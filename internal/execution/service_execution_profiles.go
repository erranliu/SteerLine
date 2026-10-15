package execution

import (
	"errors"
	"fmt"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	"path/filepath"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

func (s *Service) projectExecutionConfig() Config {
	if s.baseConfig.Repository != "" {
		return s.baseConfig
	}
	return s.Config // Test fixtures that construct a Service directly.
}

func (s *Service) selectDispatchProfile(id, digest string) error {
	selected, profile, err := s.runtime.Profiles.ResolveExecutionProfile(id, digest)
	if err != nil {
		return err
	}
	selected = copyWorkerConfig(selected)
	s.dispatchConfig = &selected
	s.dispatchProfile = &profile
	return nil
}

func (s *Service) selectDefaultDispatchProfile() error {
	id := s.runtime.Profiles.DefaultProfileID()
	if id == "" {
		return errors.New("automatic dispatch requires default_execution_profile")
	}
	return s.selectDispatchProfile(id, "")
}

func (s *Service) executionConfigForRun(run *reconstructiontypes.RunView) (Config, error) {
	base := s.projectExecutionConfig()
	if run == nil {
		return base, nil
	}
	for _, workflow := range s.domain.WorkflowRuns {
		if workflow.WorkflowRunID != run.RunID {
			continue
		}
		if workflow.ExecutionProfile == nil {
			return Config{}, errors.New("execution snapshot v5 is required; preserve original state and external session")
		}
		fixed := *workflow.ExecutionProfile
		if fixed.ID == "" || !VerifyFixedExecutionProfile(fixed, s.runtime.Directory) || workflow.ConfigDigest != fixed.AcceptedConfigVersion {
			return Config{}, &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "fixed execution profile identity is incomplete"}
		}
		config := base.ApplyFixedExecutionProfile(fixed, s.runtime.Directory)
		if err := validateWorkflowConfig(config, s.runtime.Directory); err != nil {
			return Config{}, err
		}
		entry, err := registrationFor(config, s.runtime.Directory)
		if err != nil {
			return Config{}, err
		}
		for i := range s.domain.Executions {
			e := &s.domain.Executions[i]
			if e.WorkflowRunID == workflow.WorkflowRunID {
				if e.ProjectIdentity != workflow.ProjectIdentity || e.ConfigurationDigest != fixed.Digest {
					return Config{}, errors.New("execution binding differs from its original project or configuration snapshot")
				}
				if err := validateExecutionImplementationData(e); err != nil {
					return Config{}, &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "fixed implementation binding failed verification", Cause: err}
				}
			}
			if e.WorkflowRunID == workflow.WorkflowRunID && !entry.HostManagedPermissions && e.PolicyResources == nil && (e.Process != nil || e.SessionID != "") && !awaitingRecoveryResources(e) {
				return Config{}, errors.New("bound execution lost its original policy resource snapshot")
			}
			if e.WorkflowRunID == workflow.WorkflowRunID && e.PolicyResources != nil {
				if err := verifyPolicyResources(config, s.runtime.Directory, e.PolicyResources, fixed.Digest, e.Workspace.Worktree, e); err != nil {
					return Config{}, err
				}
			}
		}
		config.WorkflowProtocol = executionmodel.Version
		config.Repository, config.Branch = workflow.Repository, fixed.Publication.BaseBranch
		if run.WorkspaceIntent != nil {
			if !filepath.IsAbs(run.WorkspaceIntent.Path) {
				return Config{}, errors.New("original workspace intent has no absolute resource path")
			}
			config.WorktreeRoot = filepath.Dir(run.WorkspaceIntent.Path)
		}
		unprepared := workflow.StartPending || !executionmodel.CandidateParticipates(workflow.Status) && run.WorkspaceIntent == nil && run.Process == nil && run.SessionID == ""
		if fixed.Publication.RemoteURL == "" && !unprepared {
			return Config{}, errors.New("fixed publication remote URL is missing; preserve original state")
		}
		if workflow.WorkflowVersion != executionmodel.Version {
			return Config{}, fmt.Errorf("fixed execution profile uses an unsupported workflow version")
		}
		entry, err = registrationFor(config, s.runtime.Directory)
		backend, protocol := executionbackend.BackendKind(""), executionbackend.ExecutionProtocol("")
		if err == nil {
			backend, protocol = entry.Key.Backend, entry.Key.Protocol
		}
		if err != nil || run.Host != fixed.Host || run.Agent != fixed.Agent || run.Backend != string(backend) || run.Protocol != string(protocol) || !sameConfiguredExecutionTarget(run.Target, TargetFor(config, s.runtime.Directory)) {
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

package execution

import (
	"context"
	"errors"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	"os"
	"slices"
	"strings"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func queryCredentialRequirement(c Config, r executionbackend.CredentialRequirement, directory *ExecutionDirectory) executionbackend.RequirementEvidence {
	e := executionbackend.RequirementEvidence{ID: r.ID, Site: r.Site, Version: "provider-reference-v1", Target: TargetFor(c, directory), Status: executionbackend.RequirementUnknown, Source: "provider:reference-unavailable", CheckedAt: now()}
	if r.ID != "pr.token_env" || r.Site != "service" {
		return e
	}
	if c.PR.TokenEnv == "" {
		e.Status = executionbackend.RequirementMissing
		return e
	}
	e.Source = "provider:environment-reference:" + c.PR.TokenEnv
	e.Status = executionbackend.RequirementMissing
	if value, ok := os.LookupEnv(c.PR.TokenEnv); ok && strings.TrimSpace(value) != "" {
		e.Status = executionbackend.RequirementPresent
	}
	return e
}

// Evaluation is deliberately separate from the mandatory worker preflight.
// Reports loaded here have already had their native evidence references checked.
func evaluateExecutionRequirements(ctx context.Context, c Config, directory *ExecutionDirectory) (executionbackend.RequirementsEvaluation, error) {
	return evaluateExecutionRequirementsWithRecords(ctx, c, executionRecords{root: c.RunDir}, directory)
}

func evaluateExecutionRequirementsWithRecords(ctx context.Context, c Config, records executionRecords, directory *ExecutionDirectory) (executionbackend.RequirementsEvaluation, error) {
	entry, err := registrationFor(c, directory)
	if err != nil {
		return executionbackend.RequirementsEvaluation{}, err
	}
	if err = c.ExecutionRequirements.Validate(); err != nil {
		return executionbackend.RequirementsEvaluation{}, err
	}
	key := executionbackend.CapabilityReport{Version: 1, Backend: entry.Key.Backend, Protocol: entry.Key.Protocol, Target: entry.Target(c), Agent: AgentFor(c)}
	e := executionbackend.RequirementsEvaluation{Version: 1, Backend: key.Backend, Protocol: key.Protocol, Target: key.Target, Agent: key.Agent, CheckedAt: now()}
	if len(c.ExecutionRequirements.Capabilities) > 0 {
		key, err = entry.ProbeRequirements(ctx, c)
		if err != nil {
			return e, err
		}
		// A version probe establishes the lookup key, never verified capability.
		report, loadErr := records.loadCapabilityReport(key)
		if loadErr != nil {
			return e, errors.New("required capabilities have no current authenticated report: " + loadErr.Error())
		}
		e.Capabilities = &report
	}
	for _, tool := range c.ExecutionRequirements.Tools {
		e.Tools = append(e.Tools, entry.QueryTool(ctx, c, tool.ID))
	}
	for _, ref := range c.ExecutionRequirements.Credentials {
		e.Credentials = append(e.Credentials, queryCredentialRequirement(c, ref, directory))
	}
	return e, c.ExecutionRequirements.Match(e, key)
}

// EvaluateRequirements reads the selected target's requirement evidence using
// the configuration's explicit directory. It grants no mutation authority.
func (c Config) EvaluateRequirements(ctx context.Context, directory *ExecutionDirectory) (executionbackend.RequirementsEvaluation, error) {
	return evaluateExecutionRequirements(ctx, c, directory)
}

func requirementsEvidenceDigest(e *executionbackend.RequirementsEvaluation) string {
	if e == nil {
		return ""
	}
	return digestValue(e)
}

func verifyRequirementsSnapshot(p executionmodel.FixedExecutionProfile, directory *ExecutionDirectory) bool {
	if p.ExecutionRequirements.Validate() != nil || requirementsEvidenceDigest(p.RequirementsEvaluation) != p.RequirementsEvidenceDigest {
		return false
	}
	if p.RequirementsEvaluation == nil {
		return p.ExecutionRequirements.Empty()
	}
	e := *p.RequirementsEvaluation
	projectID := ""
	if len(p.ImplementationData) > 0 {
		projectID = p.ImplementationData[0].ProjectID
	}
	c := Config{Host: p.Host, Agent: p.Agent, selectedProfileID: p.ID, acceptedConfigVersion: p.AcceptedConfigVersion, identity: projectID, ImplementationData: CloneImplementationData(p.ImplementationData), fixedProfileDigest: p.Digest}
	entry, err := registrationFor(c, directory)
	if err != nil {
		return false
	}
	key := executionbackend.CapabilityReport{Version: 1, Backend: entry.Key.Backend, Protocol: entry.Key.Protocol, Target: TargetFor(c, directory), Agent: AgentFor(c)}
	if e.Capabilities != nil {
		key.PlatformVersion = e.Capabilities.PlatformVersion
		key.AgentVersion = e.Capabilities.AgentVersion
		key.CenterVersion = e.Capabilities.CenterVersion
		key.ExpectedAgentVersion = e.Capabilities.ExpectedAgentVersion
	}
	return p.ExecutionRequirements.Match(e, key) == nil
}

func clonePublicationSettings(p executionmodel.PublicationSettings) executionmodel.PublicationSettings {
	p.Provider.Query.Args = slices.Clone(p.Provider.Query.Args)
	p.Provider.Close.Args = slices.Clone(p.Provider.Close.Args)
	p.Git.Args = slices.Clone(p.Git.Args)
	p.AuthenticationRefs = slices.Clone(p.AuthenticationRefs)
	return p
}

// Recheck the exact accepted file bytes inside the claim transaction. A changed
// candidate must be reloaded and evaluated; neither profile nor evidence is reused.
func verifyClaimConfigFiles(verifier ConfigVerifier) error {
	if verifier == nil {
		return nil
	}
	return verifier.VerifyCurrentConfig()
}

func (s *Service) checkCurrentExecutionRequirements(ctx context.Context, fixed workerFlowContext) error {
	if fixed.config.ExecutionRequirements.Empty() {
		return nil
	}
	evaluation, err := evaluateExecutionRequirementsWithRecords(ctx, fixed.config, s.records, s.runtime.Directory)
	if err == nil {
		fixed.execution.CurrentRequirementsEvaluation = &evaluation
		if fixed.execution.CurrentRequirementsError != nil {
			fixed.execution.CurrentRequirementsError = nil
			return s.commitWorker(fixed.run)
		}
		return nil
	}
	reason := "Execution requirements blocked: " + err.Error()
	if fixed.execution.CurrentRequirementsError != nil && *fixed.execution.CurrentRequirementsError == reason {
		return err
	}
	fixed.execution.CurrentRequirementsEvaluation = &evaluation
	fixed.execution.CurrentRequirementsError = ptr(reason)
	// Requirements gate future inputs; they are not a native liveness fact.
	return errors.Join(err, s.commitWorker(fixed.run))
}

func (s *Service) resumeRegisteredWorkerFlow(ctx context.Context, task *taskmodel.TaskRecord, run *reconstructiontypes.RunView, entry ExecutionRegistration) (bool, error) {
	if _, err := s.workerFlowContext(run); err != nil {
		return false, err
	}
	return s.resumeLifecycleWorkerFlow(ctx, task, run)
}

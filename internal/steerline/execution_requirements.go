package steerline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"slices"
	"strings"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func queryCredentialRequirement(c Config, r executionbackend.CredentialRequirement) executionbackend.RequirementEvidence {
	e := executionbackend.RequirementEvidence{ID: r.ID, Site: r.Site, Version: "provider-reference-v1", Target: targetFor(c), Status: executionbackend.RequirementUnknown, Source: "provider:reference-unavailable", CheckedAt: now()}
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
func evaluateExecutionRequirements(ctx context.Context, c Config) (executionbackend.RequirementsEvaluation, error) {
	entry, err := registrationFor(c)
	if err != nil {
		return executionbackend.RequirementsEvaluation{}, err
	}
	if err = c.ExecutionRequirements.Validate(); err != nil {
		return executionbackend.RequirementsEvaluation{}, err
	}
	key := executionbackend.CapabilityReport{Version: 1, Backend: entry.Key.Backend, Protocol: entry.Key.Protocol, Target: entry.Target(c), Agent: agentFor(c)}
	e := executionbackend.RequirementsEvaluation{Version: 1, Backend: key.Backend, Protocol: key.Protocol, Target: key.Target, Agent: key.Agent, CheckedAt: now()}
	if len(c.ExecutionRequirements.Capabilities) > 0 {
		key, err = entry.ProbeRequirements(ctx, c)
		if err != nil {
			return e, err
		}
		// A version probe establishes the lookup key, never verified capability.
		report, loadErr := (executionRecords{root: c.RunDir, persistence: c.Persistence}).loadCapabilityReport(key)
		if loadErr != nil {
			return e, errors.New("required capabilities have no current authenticated report: " + loadErr.Error())
		}
		e.Capabilities = &report
	}
	for _, tool := range c.ExecutionRequirements.Tools {
		e.Tools = append(e.Tools, entry.QueryTool(ctx, c, tool.ID))
	}
	for _, ref := range c.ExecutionRequirements.Credentials {
		e.Credentials = append(e.Credentials, queryCredentialRequirement(c, ref))
	}
	return e, c.ExecutionRequirements.Match(e, key)
}

// EvaluateRequirements reads the selected target's requirement evidence using
// the configuration's explicit directory. It grants no mutation authority.
func (c Config) EvaluateRequirements(ctx context.Context) (executionbackend.RequirementsEvaluation, error) {
	return evaluateExecutionRequirements(ctx, c)
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
	c := Config{ExecutionDirectory: directory, Host: p.Host, Agent: p.Agent, selectedProfileID: p.ID, configDigest: p.AcceptedConfigVersion, identity: projectID, ImplementationData: cloneImplementationData(p.ImplementationData), fixedProfileDigest: p.Digest}
	entry, err := registrationFor(c)
	if err != nil {
		return false
	}
	key := executionbackend.CapabilityReport{Version: 1, Backend: entry.Key.Backend, Protocol: entry.Key.Protocol, Target: targetFor(c), Agent: agentFor(c)}
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
func verifyClaimConfigFiles(c Config) error {
	if c.executionConfigPath == "" {
		return nil
	} // Explicit in-memory configuration.
	project, err := os.ReadFile(c.policySource)
	if err != nil {
		return err
	}
	global, err := os.ReadFile(c.executionConfigPath)
	if err != nil {
		return err
	}
	if loadedConfigDigest(project, global) != c.configDigest {
		return nativeErr("stale_profile", "project or execution configuration changed during claim evaluation")
	}
	return nil
}

func loadedConfigDigest(project, global []byte) string {
	h := sha256.New()
	h.Write(project)
	h.Write([]byte{0})
	h.Write(global)
	return hex.EncodeToString(h.Sum(nil))
}

func (s *Service) checkCurrentExecutionRequirements(ctx context.Context, fixed workerFlowContext) error {
	if fixed.config.ExecutionRequirements.Empty() {
		return nil
	}
	evaluation, err := evaluateExecutionRequirements(ctx, fixed.config)
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

func (s *Service) resumeRegisteredWorkerFlow(ctx context.Context, task *taskmodel.TaskRecord, run *Run, entry ExecutionRegistration) (bool, error) {
	if _, err := s.workerFlowContext(run); err != nil {
		return false, err
	}
	return s.resumeLifecycleWorkerFlow(ctx, task, run)
}

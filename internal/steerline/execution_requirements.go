package steerline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

var requirementToolPatterns = map[string]*regexp.Regexp{
	"git":    regexp.MustCompile(`^git version ([0-9][a-zA-Z0-9.+_-]*)$`),
	"go":     regexp.MustCompile(`^go version go([0-9][a-zA-Z0-9.+_-]*) [a-zA-Z0-9]+/[a-zA-Z0-9]+$`),
	"python": regexp.MustCompile(`^Python ([0-9][a-zA-Z0-9.+_-]*)$`),
}

func queryHostRequirementTool(ctx context.Context, c Config, id string) executionapi.RequirementEvidence {
	e := executionapi.RequirementEvidence{ID: id, Site: "target", Target: targetFor(c), Status: executionapi.RequirementUnknown, Source: "target:inherited-path/version-query", CheckedAt: now()}
	command := Command{Executable: id}
	args := []string{"--version"}
	if id == "git" {
		command = c.Git
		e.Source = "target:frozen-git/version-query"
	}
	if id == "go" {
		args = []string{"version"}
	}
	parser := requirementToolPatterns[id]
	if parser == nil {
		return e
	}
	var environment []string
	if id != "git" {
		environment = workerProcessEnvironment(c, &Run{}, os.Environ())
	}
	executable, lookupErr := requirementExecutable(command.Executable, environment)
	if lookupErr != nil {
		e.Status = executionapi.RequirementMissing
		return e
	}
	command.Executable = executable
	runner := Runner{Dir: c.Repository, Timeout: min(positiveDuration(c.CommandTimeout, 10*time.Second), 10*time.Second)}
	runner.Environment = environment
	if id == "go" {
		// A version check must not download a module-selected Go toolchain.
		runner.Env = map[string]string{"GOTOOLCHAIN": "local"}
		e.Source = "target:inherited-path/local-go-toolchain-version-query"
	}
	out, err := runner.Output(ctx, command, args...)
	if err != nil {
		return e
	}
	match := parser.FindStringSubmatch(strings.TrimSpace(string(out)))
	if len(match) != 2 {
		return e
	}
	e.Status, e.Version = executionapi.RequirementPresent, match[1]
	return e
}
func requirementExecutable(name string, environment []string) (string, error) {
	if environment == nil || strings.ContainsAny(name, `/\`) {
		return exec.LookPath(name)
	}
	path := ""
	for _, entry := range environment {
		key, value, _ := strings.Cut(entry, "=")
		if key == "PATH" || runtime.GOOS == "windows" && strings.EqualFold(key, "PATH") {
			path = value
			break
		}
	}
	if runtime.GOOS == "windows" {
		name += ".exe"
	} // Fixed native tool mapping, no shell shim.
	for _, dir := range filepath.SplitList(path) {
		if !filepath.IsAbs(dir) {
			continue
		}
		candidate, err := exec.LookPath(filepath.Join(dir, name))
		if err == nil {
			return candidate, nil
		}
	}
	return "", exec.ErrNotFound
}
func queryRemoteRequirementTool(_ context.Context, c Config, id string) executionapi.RequirementEvidence {
	return executionapi.RequirementEvidence{ID: id, Site: "target", Target: targetFor(c), Status: executionapi.RequirementUnknown, Source: "remote:fixed-read-only-tool-query-unavailable", CheckedAt: now()}
}
func queryNativeRequirementTool(ctx context.Context, c Config, id string) executionapi.RequirementEvidence {
	// Workspace Git is the frozen host tool used by this target's workspace
	// operations. The terminal's PATH can differ from the service PATH; no native
	// read-only terminal environment query exists for go/python in this version.
	if id == "git" {
		return queryHostRequirementTool(ctx, c, id)
	}
	return executionapi.RequirementEvidence{ID: id, Site: "target", Target: targetFor(c), Status: executionapi.RequirementUnknown, Source: "native:terminal-path-query-unavailable", CheckedAt: now()}
}
func queryCredentialRequirement(c Config, r executionapi.CredentialRequirement) executionapi.RequirementEvidence {
	e := executionapi.RequirementEvidence{ID: r.ID, Site: r.Site, Version: "provider-reference-v1", Target: targetFor(c), Status: executionapi.RequirementUnknown, Source: "provider:reference-unavailable", CheckedAt: now()}
	if r.ID != "pr.token_env" || r.Site != "service" {
		return e
	}
	if c.PR.TokenEnv == "" {
		e.Status = executionapi.RequirementMissing
		return e
	}
	e.Source = "provider:environment-reference:" + c.PR.TokenEnv
	e.Status = executionapi.RequirementMissing
	if value, ok := os.LookupEnv(c.PR.TokenEnv); ok && strings.TrimSpace(value) != "" {
		e.Status = executionapi.RequirementPresent
	}
	return e
}

// Evaluation is deliberately separate from the mandatory worker preflight.
// Reports loaded here have already had their native evidence references checked.
func evaluateExecutionRequirements(ctx context.Context, c Config) (executionapi.RequirementsEvaluation, error) {
	entry, err := registrationFor(c)
	if err != nil {
		return executionapi.RequirementsEvaluation{}, err
	}
	if err = c.ExecutionRequirements.Validate(); err != nil {
		return executionapi.RequirementsEvaluation{}, err
	}
	key := executionapi.CapabilityReport{Version: 1, Backend: entry.Key.Backend, Protocol: entry.Key.Protocol, Target: entry.Target(c), Agent: agentFor(c)}
	e := executionapi.RequirementsEvaluation{Version: 1, Backend: key.Backend, Protocol: key.Protocol, Target: key.Target, Agent: key.Agent, CheckedAt: now()}
	if len(c.ExecutionRequirements.Capabilities) > 0 {
		key, err = entry.ProbeRequirements(ctx, c)
		if err != nil {
			return e, err
		}
		// A version probe establishes the lookup key, never verified capability.
		report, loadErr := (executionRecords{root: c.RunDir}).loadCapabilityReport(key)
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
func requirementsEvidenceDigest(e *executionapi.RequirementsEvaluation) string {
	if e == nil {
		return ""
	}
	return digestValue(e)
}
func verifyRequirementsSnapshot(p FixedExecutionProfile) bool {
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
	c := Config{Host: p.Host, Agent: p.Agent, selectedProfileID: p.ID, configDigest: p.AcceptedConfigVersion, identity: projectID, ImplementationData: cloneImplementationData(p.ImplementationData), fixedProfileDigest: p.Digest}
	entry, err := registrationFor(c)
	if err != nil {
		return false
	}
	key := executionapi.CapabilityReport{Version: 1, Backend: entry.Key.Backend, Protocol: entry.Key.Protocol, Target: targetFor(c), Agent: agentFor(c)}
	if e.Capabilities != nil {
		key.PlatformVersion = e.Capabilities.PlatformVersion
		key.AgentVersion = e.Capabilities.AgentVersion
		key.CenterVersion = e.Capabilities.CenterVersion
		key.ExpectedAgentVersion = e.Capabilities.ExpectedAgentVersion
	}
	return p.ExecutionRequirements.Match(e, key) == nil
}
func clonePublicationSettings(p contracts.PublicationSettings) contracts.PublicationSettings {
	p.Provider.Query.Args = slices.Clone(p.Provider.Query.Args)
	p.Provider.Close.Args = slices.Clone(p.Provider.Close.Args)
	p.Git.Args = slices.Clone(p.Git.Args)
	p.Create.Args = slices.Clone(p.Create.Args)
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

func (s *Service) resumeRegisteredWorkerFlow(ctx context.Context, task *TaskRecord, run *Run, entry *executionRegistration) (bool, error) {
	if _, err := s.workerFlowContext(run); err != nil {
		return false, err
	}
	return s.resumeLifecycleWorkerFlow(ctx, task, run)
}

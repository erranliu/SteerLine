package steerline

import (
	"context"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// Version-query fixtures are test owned. Product construction lives in steerlineapp.
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
func capabilityReport(backend executionapi.BackendKind, protocol executionapi.ExecutionProtocol, target executionapi.ExecutionTarget, agent executionapi.AgentSpec, version string) executionapi.CapabilityReport {
	caps := []executionapi.Capability{{Name: "start", Status: executionapi.CapabilityNotTested}, {Name: "observe", Status: executionapi.CapabilityNotTested}, {Name: "stop", Status: executionapi.CapabilityNotTested}, {Name: "detach_observation", Status: executionapi.CapabilityNotTested}, {Name: "operation_reconciliation", Status: executionapi.CapabilityNotTested}, {Name: "workspace_inspection", Status: executionapi.CapabilityNotTested}, {Name: "output_reading", Status: executionapi.CapabilityNotTested}, {Name: "output_turns", Status: executionapi.CapabilityNotTested}, {Name: "output_final_answer", Status: executionapi.CapabilityNotTested}, {Name: "resource_settlement", Status: executionapi.CapabilityUnsupported}}
	return executionapi.CapabilityReport{Version: 1, Backend: backend, Protocol: protocol, Target: target, Agent: agent, PlatformVersion: version, CheckedAt: now(), Capabilities: caps}
}

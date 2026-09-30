package steerline

import (
	"context"
	"errors"
	"reflect"
	"runtime"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

func policyNativeVersion(ctx context.Context, c Config) (string, error) {
	if err := validateWorkflowConfig(&c, nil); err != nil {
		return "", err
	}
	native, err := codexworker.NativeExecutable(c.Codex.Executable)
	if err != nil {
		return "", err
	}
	raw, err := (Runner{Dir: c.Repository, Timeout: min(positiveDuration(c.CommandTimeout, 10*time.Second), 10*time.Second)}).Output(ctx, Command{Executable: native}, "--version")
	if err != nil {
		return "", err
	}
	version := strings.TrimSpace(string(raw))
	return version, codexworker.VerifyPolicyVersion(version)
}
func policyResourceDigest(p contracts.PolicyResources) string { p.Digest = ""; return digestValue(p) }
func verifyPolicyResources(p *contracts.PolicyResources, profileDigest, workspace string) error {
	if p == nil || p.Version != 2 || p.Digest != policyResourceDigest(*p) || p.ProfileDigest != profileDigest || p.Workspace != workspace || p.Contract != codexworker.PolicyContract || codexworker.VerifyPolicyVersion(p.NativeVersion) != nil || len(p.TemporaryRoots) != 0 {
		return errors.New("execution policy resource snapshot differs from original settings or identity")
	}
	return nil
}
func bindPolicyResources(ctx context.Context, c Config, e *ExecutionRecord, workspace string, linkedArgs []string) error {
	if e.PolicyResources == nil && (e.Process != nil || e.SessionID != "") {
		return errors.New("original execution policy resource snapshot is missing; preserve the native session")
	}
	permissions, err := codexworker.CompilePolicy(c.ExecutionPolicy, runtime.GOOS, c.hostName(), c.agentName(), c.Codex.WindowsSandbox, c.Profile, c.Codex.Args)
	if err != nil {
		return err
	}
	version, err := policyNativeVersion(ctx, c)
	if err != nil {
		return err
	}
	roots, err := codexworker.WritableRoots(linkedArgs)
	if err != nil {
		return err
	}
	var command executionapi.LaunchCommand
	if c.hostName() == "orca" {
		native, nativeErr := codexworker.NativeExecutable(c.Codex.Executable)
		if nativeErr != nil {
			return nativeErr
		}
		command, err = codexworker.ManagedCommand(codexworker.ManagedCommandOptions{Policy: c.ExecutionPolicy, Host: c.hostName(), Agent: c.agentName(), Executable: native, Args: c.Codex.Args, WindowsSandbox: permissions.WindowsSandbox, Sandbox: permissions.Sandbox, Model: c.Model, ReasoningEffort: c.ReasoningEffort, WorkingDirectory: workspace, WriteRoots: roots})
	} else {
		native, nativeErr := codexworker.NativeExecutable(c.Codex.Executable)
		if nativeErr != nil {
			return nativeErr
		}
		command = executionapi.LaunchCommand{Executable: native, Args: append(append([]string(nil), c.Codex.Args...), permissions.Args()...), WorkingDirectory: workspace}
	}
	if err != nil {
		return err
	}
	if permissions.Sandbox == "workspace-write" {
		roots = append([]string{workspace}, roots...)
	}
	evidence := contracts.PolicyResources{Version: 2, ProfileDigest: c.fixedProfileDigest, NativeVersion: version, Contract: permissions.Contract, Workspace: workspace, WriteRoots: roots, TemporaryRoots: []string{}, Settings: command}
	// A named profile retains its selected name/ID in the full configuration digest.
	if evidence.ProfileDigest == "" {
		evidence.ProfileDigest = c.projectExecutionProfile().Digest()
	}
	evidence.Digest = policyResourceDigest(evidence)
	if e.PolicyResources != nil {
		// Windows native resolution may normalize slashes/case. Preserve the
		// original receipt bytes and digest when the executable identity is equal.
		if canonical(e.PolicyResources.Settings.Executable) == canonical(evidence.Settings.Executable) {
			evidence.Settings.Executable = e.PolicyResources.Settings.Executable
			evidence.Digest = policyResourceDigest(evidence)
		}
		if !reflect.DeepEqual(*e.PolicyResources, evidence) {
			return errors.New("native resources/settings expanded or changed after binding")
		}
		return nil
	}
	e.PolicyResources = &evidence
	return nil
}

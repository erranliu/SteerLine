package app

import (
	"context"
	"errors"
	terminal "github.com/erranliu/SteerLine/internal/execution/adapters/orcatransport"
	native "github.com/erranliu/SteerLine/internal/execution/adapters/protocol/orca"
	b "github.com/erranliu/SteerLine/internal/execution/backend"
	api "github.com/erranliu/SteerLine/internal/product/api"
	p "github.com/erranliu/SteerLine/internal/storage/ports"
	"time"
)

type orcaSessionSpec struct {
	profile terminal.Profile
	ref     b.ExecutionRef
}

func resolveOrcaSession(records p.ExecutionSnapshot, project string, target api.NativeSessionTarget) (orcaSessionSpec, error) {
	var spec orcaSessionSpec
	if project == "" || records.ProjectIdentity != project {
		return spec, errors.New("Orca viewer project differs")
	}
	for _, e := range records.Executions {
		if e.ProjectIdentity != project || e.ExecutionID != target.ExecutionID || e.WorkflowRunID != target.RunID {
			continue
		}
		if e.Protocol != string(b.ProtocolOrcaTerminalV3) || e.Backend != string(b.BackendOrca) || e.Process == nil || e.Process.Kind != "orca_terminal" || e.CurrentAttemptID != target.AttemptID || target.SessionID == "" || target.SessionGeneration == "" || e.SessionID != target.SessionID || e.SessionGeneration != target.SessionGeneration || e.SessionID != terminal.SessionID(*e.Process) || e.SessionGeneration != e.Process.Incarnation {
			return spec, errors.New("original Orca session is missing or replaced")
		}
		for _, run := range records.WorkflowRuns {
			if run.ProjectIdentity != project || run.WorkflowRunID != target.RunID || run.ExecutionProfile == nil {
				continue
			}
			profile := run.ExecutionProfile
			if profile.Host != "orca" || profile.Agent != e.Agent {
				return spec, errors.New("frozen Orca profile differs")
			}
			found := false
			for _, value := range profile.ImplementationData {
				if value.ImplementationKey == terminal.ProfileKey {
					if found || value.CodecVersion != terminal.ProfileVersion {
						return spec, errors.New("invalid frozen Orca profile")
					}
					found = true
					if err := value.ValidateProfileIdentity(project, profile.ID, profile.AcceptedConfigVersion); err != nil {
						return spec, err
					}
					var err error
					spec.profile, err = terminal.DecodeProfile(value.Payload)
					if err != nil {
						return spec, err
					}
				}
			}
			if !found {
				return spec, errors.New("original Orca profile missing")
			}
			spec.ref = b.ExecutionRef{Backend: b.BackendOrca, Protocol: b.ProtocolOrcaTerminalV3, RunID: e.WorkflowRunID, AttemptID: e.CurrentAttemptID, Target: e.Target, Agent: b.AgentSpec{Name: e.Agent}, External: *e.Process, WorkspacePath: e.Workspace.Worktree, WorkspaceBranch: e.Workspace.Branch, BaseCommit: e.Workspace.Base}
			return spec, nil
		}
		return spec, errors.New("original Orca workflow missing")
	}
	return spec, errors.New("original Orca candidate missing")
}
func openOrcaSession(ctx context.Context, config Config, records p.ExecutionSnapshot, target api.NativeSessionTarget) error {
	spec, err := resolveOrcaSession(records, config.Identity(), target)
	if err != nil {
		return err
	}
	client := native.New(native.Config{CommandTimeout: "40s", Target: spec.ref.Target, Output: func(ctx context.Context, timeout time.Duration, args ...string) ([]byte, error) {
		return config.ProcessHost.OutputCommand(ctx, b.LaunchCommand{Executable: spec.profile.Executable, Args: append(append(append([]string(nil), spec.profile.Args...), args...), orcaViewerRoute(spec.profile)...), WorkingDirectory: config.Repository, Timeout: timeout, BlockedEnvironment: []string{"ORCA_ENVIRONMENT", "ORCA_PAIRING_CODE", "ORCA_REMOTE_PAIRING"}})
	}})
	verify := func() error {
		t, err := client.InspectBoundTerminal(ctx, spec.ref.Target, spec.ref.External)
		if err != nil {
			return err
		}
		if t.WorktreeID != spec.ref.External.WorktreeID || t.AgentIdentity != spec.ref.Agent.Name || t.Exited() {
			return errors.New("original Orca agent is unavailable")
		}
		return native.ValidateWorkspaceTerminal(t, spec.ref.WorkspacePath, spec.ref.WorkspaceBranch)
	}
	if err = verify(); err != nil {
		return err
	}
	result, err := client.MetaCall(ctx, "terminal", "switch", "--terminal", spec.ref.External.Terminal, "--json")
	if err != nil {
		return err
	}
	if result.RuntimeID != spec.ref.External.RuntimeID {
		return errors.New("Orca runtime changed during focus")
	}
	return verify()
}

func orcaViewerRoute(profile terminal.Profile) []string {
	if profile.Environment != "" {
		return []string{"--environment", profile.Environment}
	}
	return nil
}

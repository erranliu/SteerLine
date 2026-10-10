package app

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/erranliu/SteerLine/internal/execution"
	terminal "github.com/erranliu/SteerLine/internal/execution/adapters/orcatransport"
	native "github.com/erranliu/SteerLine/internal/execution/adapters/protocol/orca"
	b "github.com/erranliu/SteerLine/internal/execution/backend"
)

func terminalExecutionRegistration(host b.ManagedProcessHost, directory func() *execution.ExecutionDirectory) (execution.ImplementationProfileCodec, execution.ExecutionRegistration) {
	settings := func(c execution.Config) (terminal.Profile, error) {
		raw, err := c.DecodeImplementationConfig(terminal.ProfileKey, directory())
		if err != nil {
			return terminal.Profile{}, err
		}
		return terminal.DecodeProfile(raw)
	}
	output := func(c execution.Config, p terminal.Profile) func(context.Context, time.Duration, ...string) ([]byte, error) {
		return func(ctx context.Context, timeout time.Duration, args ...string) ([]byte, error) {
			all := append(append([]string(nil), p.Args...), args...)
			if p.Environment != "" {
				all = append(all, "--environment", p.Environment)
			}
			return host.OutputCommand(ctx, b.LaunchCommand{Executable: p.Executable, Args: all, WorkingDirectory: c.Repository, Timeout: timeout, BlockedEnvironment: []string{"ORCA_ENVIRONMENT", "ORCA_PAIRING_CODE", "ORCA_REMOTE_PAIRING"}})
		}
	}
	codec := execution.ImplementationProfileCodec{Key: terminal.ProfileKey, Version: terminal.ProfileVersion, Normalize: func(data []byte, _, _ string) ([]byte, []string, error) { return terminal.NormalizeProfile(data) }}
	entry := execution.ExecutionRegistration{ServiceStopBehavior: terminal.ServiceStopBehavior, Key: execution.ExecutionRegistrationKey{Backend: b.BackendOrca, Agent: "*", Protocol: b.ProtocolOrcaTerminalV3}, ProfileKey: "orca/terminal", WorkspaceBranchPrefix: "worker", LocalWorkspace: false, HostManagedPermissions: true, ProfileCodecKeys: []string{terminal.ProfileKey}, QuestionCodecKey: "steerline.worker.question", QuestionCodecVersion: 1, Host: "orca",
		Target: func(c execution.Config) b.ExecutionTarget {
			p, err := settings(c)
			if err != nil {
				return b.ExecutionTarget{}
			}
			return p.Target()
		},
		Lifecycle: func(setup b.LifecycleSetup) (b.LifecyclePort, error) {
			setup.ProcessHost = host
			return terminal.NewLifecycle(setup)
		},
		ValidateProfileSelection: func(agent string, payloads map[string][]byte) error {
			if agent == "" || agent == "*" {
				return errors.New("opaque Orca agent identity is required")
			}
			_, err := terminal.DecodeProfile(payloads[terminal.ProfileKey])
			return err
		},
		ValidateConfig: func(_ context.Context, c execution.Config) error { _, err := settings(c); return err },
		PrepareClaim: func(context.Context, execution.Config) (execution.ClaimPreparation, error) {
			return execution.ClaimPreparation{}, nil
		},
		ReadinessRequired:       func(execution.Config) bool { return false },
		LinkedWorktreeWriteArgs: func(context.Context, execution.Config, string) ([]string, error) { return nil, nil },
		ValidTurnID:             b.StableExecutionID, ValidateRef: validateCatalogRef, CleanupCapabilities: []string{"start", "observe", "stop", "workspace_inspection", "output_reading"}, ValidatePublicationPolicy: validateWorkerPublicationPolicy,
	}
	entry.Backend = func(c execution.Config) b.ExecutionBackend {
		p, _ := settings(c)
		return terminal.Backend{Profile: p, Output: output(c, p), Workspace: func(ctx context.Context, ref b.ExecutionRef) (b.WorkspaceInspection, error) {
			client := native.New(native.Config{Target: p.Target(), Output: output(c, p), CommandTimeout: "40s"})
			return terminal.InspectWorkspace(ctx, client, ref, c.Identity())
		},
		}
	}
	entry.Preflight = func(ctx context.Context, c execution.Config) error {
		p, err := settings(c)
		if err != nil {
			return err
		}
		_, err = (terminal.Backend{Profile: p, Output: output(c, p)}).Probe(ctx, p.Target(), b.AgentSpec{Name: c.Agent})
		return err
	}
	entry.BuildPolicyResource = func(_ context.Context, r execution.PolicyResourceBuildRequest) (execution.PolicyResourceBuildResult, error) {
		raw, _ := json.Marshal(struct {
			Workspace, Agent string
			ProfileDigest    string
		}{r.Workspace, r.Agent, shortDigest(r.ProfileData)})
		e, err := b.NewImplementationDataEnvelope(b.ImplementationDataEnvelope{CodecVersion: 2, Purpose: b.ImplementationResourcePurpose, ImplementationKey: "orca.terminal-policy", ProjectID: r.Resource.ProjectID, ExecutionID: r.Resource.ExecutionID, AttemptID: r.Resource.AttemptID, Generation: r.Resource.Generation, ConfigDigest: r.Resource.ConfigDigest}, raw)
		return execution.PolicyResourceBuildResult{Envelope: e, WriteRoots: []string{r.Workspace}}, err
	}
	entry.VerifyPolicyResource = func(e b.ImplementationDataEnvelope, r execution.PolicyResourceIdentity) error {
		if e.ImplementationKey != "orca.terminal-policy" || e.CodecVersion != 2 {
			return errors.New("unsupported terminal permission resource")
		}
		return e.ValidateResourceIdentity(r.ProjectID, r.ExecutionID, r.AttemptID, r.Generation, r.ConfigDigest)
	}
	return codec, entry
}

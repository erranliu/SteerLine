package orca_remote_worker

import (
	"context"
	"errors"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

type Backend struct{ config Config }

func (b Backend) Identity() (executionbackend.BackendKind, executionbackend.ExecutionProtocol) {
	return executionbackend.BackendOrca, executionbackend.ProtocolOrcaSupervised
}

func (b Backend) Probe(ctx context.Context, target executionbackend.ExecutionTarget, agent executionbackend.AgentSpec) (executionbackend.CapabilityReport, error) {
	fixedTarget := b.config.Target()
	if agent.Name != "codex" || !SameBaseTarget(target, fixedTarget) {
		return executionbackend.CapabilityReport{}, &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "supervised Orca probe target or agent differs from fixed configuration"}
	}
	pair, err := Preflight(ctx, b.config)
	if err != nil {
		return executionbackend.CapabilityReport{}, &executionbackend.BackendError{Code: executionbackend.BackendUnreachable, Diagnostic: "supervised Orca target preflight failed", Cause: err}
	}
	return b.report(target, agent, pair), nil
}

func (b Backend) report(target executionbackend.ExecutionTarget, agent executionbackend.AgentSpec, pair PreflightResult) executionbackend.CapabilityReport {
	report := capabilityReport(executionbackend.BackendOrca, executionbackend.ProtocolOrcaSupervised, target, agent, pair.RemoteVersion)
	report.CenterVersion = pair.CenterVersion
	report.ExpectedAgentVersion = b.config.Settings.ExpectedCodexVersion
	report.Capabilities = append(report.Capabilities,
		executionbackend.Capability{Name: "message_inbox", Status: executionbackend.CapabilityNotTested},
		executionbackend.Capability{Name: "remote_cancellation", Status: executionbackend.CapabilityUnsupported, Evidence: "Spec 03 fixes remote_cancel_available=false"},
	)
	for i := range report.Capabilities {
		if report.Capabilities[i].Name == "stop" || report.Capabilities[i].Name == "retry" {
			report.Capabilities[i].Status = executionbackend.CapabilityUnsupported
		}
		if report.Capabilities[i].Name == "resource_settlement" {
			report.Capabilities[i].Status = executionbackend.CapabilityNotTested
		}
	}
	return report
}

func (b Backend) Observe(ctx context.Context, ref executionbackend.ExecutionRef) (result executionbackend.Observation, retErr error) {
	defer func() { retErr = backendError(retErr) }()
	if err := validateSupervisedRef(b.config, FromExecutionRef(ref)); err != nil {
		return executionbackend.Observation{}, err
	}
	show, err := New(b.config).Show(ctx, StartReceipt{RunID: ref.External.OrchestrationRun, TaskID: ref.External.Task, DispatchID: ref.External.Dispatch, WorktreeID: ref.External.WorktreeID, CenterRuntimeID: ref.External.CoordinatorRuntimeID})
	if err != nil {
		return executionbackend.Observation{}, err
	}
	if show.Identity != ref.External {
		return executionbackend.Observation{}, &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "supervised worker observation returned a different exact execution identity"}
	}
	connection := "connected"
	if show.Liveness == "unverifiable" {
		connection = "unverifiable"
	}
	diagnostic := show.DispatchState
	if show.AgentWaitState == "present" {
		diagnostic = "host_interaction_pending"
	}
	return executionbackend.Observation{Liveness: show.Liveness, Connection: connection, ObservedAt: show.ObservedAt, Source: "orca-supervised-worker-show", Diagnostic: diagnostic}, nil
}

func (b Backend) InspectWorkspace(ctx context.Context, ref executionbackend.ExecutionRef) (result executionbackend.WorkspaceInspection, retErr error) {
	defer func() { retErr = backendError(retErr) }()
	if err := validateSupervisedRef(b.config, FromExecutionRef(ref)); err != nil {
		return executionbackend.WorkspaceInspection{}, err
	}
	pair, err := Preflight(ctx, b.config)
	if err != nil {
		return executionbackend.WorkspaceInspection{}, &executionbackend.BackendError{Code: executionbackend.BackendUnreachable, Diagnostic: "supervised workspace preflight failed", Cause: err}
	}
	value, err := New(b.config).InspectWorktree(ctx, pair, ref.External.WorktreeID, ref.WorkspacePath, ref.WorkspaceBranch, ref.BaseCommit, "")
	return executionbackend.WorkspaceInspection(value), err
}

func NewBackend(c Config) Backend { return Backend{config: c} }

func (c Config) Target() executionbackend.ExecutionTarget {
	return executionbackend.ExecutionTarget{ID: "orca-supervised:" + c.Settings.ConnectionID, ConnectionID: c.Settings.ConnectionID, ExternalRuntime: c.Settings.EnvironmentID}
}

func SameBaseTarget(a, b executionbackend.ExecutionTarget) bool {
	return a.ID != "" && a.ID == b.ID && a.ConnectionID == b.ConnectionID && a.ExternalRuntime == b.ExternalRuntime
}

func FromExecutionRef(ref executionbackend.ExecutionRef) Ref {
	return Ref{Backend: ref.Backend, Protocol: ref.Protocol, RunID: ref.RunID, Target: ref.Target, External: ref.External, WorkspacePath: ref.WorkspacePath, WorkspaceBranch: ref.WorkspaceBranch, BaseCommit: ref.BaseCommit}
}

func capabilityReport(backend executionbackend.BackendKind, protocol executionbackend.ExecutionProtocol, target executionbackend.ExecutionTarget, agent executionbackend.AgentSpec, version string) executionbackend.CapabilityReport {
	caps := []executionbackend.Capability{{Name: "start", Status: executionbackend.CapabilityNotTested}, {Name: "observe", Status: executionbackend.CapabilityNotTested}, {Name: "stop", Status: executionbackend.CapabilityNotTested}, {Name: "detach_observation", Status: executionbackend.CapabilityNotTested}, {Name: "operation_reconciliation", Status: executionbackend.CapabilityNotTested}, {Name: "workspace_inspection", Status: executionbackend.CapabilityNotTested}, {Name: "output_reading", Status: executionbackend.CapabilityNotTested}, {Name: "output_turns", Status: executionbackend.CapabilityNotTested}, {Name: "output_final_answer", Status: executionbackend.CapabilityNotTested}, {Name: "resource_settlement", Status: executionbackend.CapabilityUnsupported}}
	return executionbackend.CapabilityReport{Version: 1, Backend: backend, Protocol: protocol, Target: target, Agent: agent, PlatformVersion: version, CheckedAt: now(), Capabilities: caps}
}

func backendError(err error) error {
	var remote *Error
	if errors.As(err, &remote) {
		return &executionbackend.BackendError{Code: remote.Code, Diagnostic: remote.Diagnostic, Cause: remote.Cause}
	}
	return err
}

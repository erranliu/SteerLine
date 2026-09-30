package orca_remote_worker

import (
	"context"
	"errors"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

type Backend struct{ config Config }

func (b Backend) Identity() (executionapi.BackendKind, executionapi.ExecutionProtocol) {
	return executionapi.BackendOrca, executionapi.ProtocolOrcaSupervised
}

func (b Backend) Probe(ctx context.Context, target executionapi.ExecutionTarget, agent executionapi.AgentSpec) (executionapi.CapabilityReport, error) {
	fixedTarget := b.config.Target()
	if agent.Name != "codex" || !SameBaseTarget(target, fixedTarget) {
		return executionapi.CapabilityReport{}, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "supervised Orca probe target or agent differs from fixed configuration"}
	}
	target = fixedTarget
	pair, err := Preflight(ctx, b.config)
	if err != nil {
		return executionapi.CapabilityReport{}, &executionapi.BackendError{Code: executionapi.BackendUnreachable, Diagnostic: "supervised Orca target preflight failed", Cause: err}
	}
	report := capabilityReport(executionapi.BackendOrca, executionapi.ProtocolOrcaSupervised, target, agent, pair.RemoteVersion)
	report.CenterVersion = pair.CenterVersion
	report.ExpectedAgentVersion = b.config.Settings.ExpectedCodexVersion
	report.Capabilities = append(report.Capabilities,
		executionapi.Capability{Name: "message_inbox", Status: executionapi.CapabilityNotTested},
		executionapi.Capability{Name: "remote_cancellation", Status: executionapi.CapabilityUnsupported, Evidence: "Spec 03 fixes remote_cancel_available=false"},
	)
	for i := range report.Capabilities {
		if report.Capabilities[i].Name == "stop" || report.Capabilities[i].Name == "retry" {
			report.Capabilities[i].Status = executionapi.CapabilityUnsupported
		}
		if report.Capabilities[i].Name == "resource_settlement" {
			report.Capabilities[i].Status = executionapi.CapabilityNotTested
		}
	}
	return report, nil
}

func (b Backend) Observe(ctx context.Context, ref executionapi.ExecutionRef) (result executionapi.Observation, retErr error) {
	defer func() { retErr = backendError(retErr) }()
	if err := validateSupervisedRef(b.config, FromExecutionRef(ref)); err != nil {
		return executionapi.Observation{}, err
	}
	show, err := New(b.config).Show(ctx, StartReceipt{RunID: ref.External.OrchestrationRun, TaskID: ref.External.Task, DispatchID: ref.External.Dispatch, WorktreeID: ref.External.WorktreeID, CenterRuntimeID: ref.External.CoordinatorRuntimeID})
	if err != nil {
		return executionapi.Observation{}, err
	}
	if show.Identity != ref.External {
		return executionapi.Observation{}, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "supervised worker observation returned a different exact execution identity"}
	}
	connection := "connected"
	if show.Liveness == "unverifiable" {
		connection = "unverifiable"
	}
	diagnostic := show.DispatchState
	if show.AgentWaitState == "present" {
		diagnostic = "host_interaction_pending"
	}
	return executionapi.Observation{Liveness: show.Liveness, Connection: connection, ObservedAt: show.ObservedAt, Source: "orca-supervised-worker-show", Diagnostic: diagnostic}, nil
}

func (b Backend) InspectWorkspace(ctx context.Context, ref executionapi.ExecutionRef) (result executionapi.WorkspaceInspection, retErr error) {
	defer func() { retErr = backendError(retErr) }()
	if err := validateSupervisedRef(b.config, FromExecutionRef(ref)); err != nil {
		return executionapi.WorkspaceInspection{}, err
	}
	pair, err := Preflight(ctx, b.config)
	if err != nil {
		return executionapi.WorkspaceInspection{}, &executionapi.BackendError{Code: executionapi.BackendUnreachable, Diagnostic: "supervised workspace preflight failed", Cause: err}
	}
	value, err := New(b.config).InspectWorktree(ctx, pair, ref.External.WorktreeID, ref.WorkspacePath, ref.WorkspaceBranch, ref.BaseCommit, "")
	return executionapi.WorkspaceInspection(value), err
}

func NewBackend(c Config) Backend { return Backend{config: c} }
func (c Config) Target() executionapi.ExecutionTarget {
	return executionapi.ExecutionTarget{ID: "orca-supervised:" + c.Settings.ConnectionID, ConnectionID: c.Settings.ConnectionID, ExternalRuntime: c.Settings.EnvironmentID}
}
func SameBaseTarget(a, b executionapi.ExecutionTarget) bool {
	return a.ID != "" && a.ID == b.ID && a.ConnectionID == b.ConnectionID && a.ExternalRuntime == b.ExternalRuntime
}
func FromExecutionRef(ref executionapi.ExecutionRef) Ref {
	return Ref{Backend: ref.Backend, Protocol: ref.Protocol, RunID: ref.RunID, Target: ref.Target, External: ref.External, WorkspacePath: ref.WorkspacePath, WorkspaceBranch: ref.WorkspaceBranch, BaseCommit: ref.BaseCommit}
}
func capabilityReport(backend executionapi.BackendKind, protocol executionapi.ExecutionProtocol, target executionapi.ExecutionTarget, agent executionapi.AgentSpec, version string) executionapi.CapabilityReport {
	caps := []executionapi.Capability{{Name: "start", Status: executionapi.CapabilityNotTested}, {Name: "observe", Status: executionapi.CapabilityNotTested}, {Name: "stop", Status: executionapi.CapabilityNotTested}, {Name: "detach_observation", Status: executionapi.CapabilityNotTested}, {Name: "operation_reconciliation", Status: executionapi.CapabilityNotTested}, {Name: "workspace_inspection", Status: executionapi.CapabilityNotTested}, {Name: "output_reading", Status: executionapi.CapabilityNotTested}, {Name: "output_turns", Status: executionapi.CapabilityNotTested}, {Name: "output_final_answer", Status: executionapi.CapabilityNotTested}, {Name: "resource_settlement", Status: executionapi.CapabilityUnsupported}}
	return executionapi.CapabilityReport{Version: 1, Backend: backend, Protocol: protocol, Target: target, Agent: agent, PlatformVersion: version, CheckedAt: now(), Capabilities: caps}
}

func backendError(err error) error {
	var remote *Error
	if errors.As(err, &remote) {
		return &executionapi.BackendError{Code: remote.Code, Diagnostic: remote.Diagnostic, Cause: remote.Cause}
	}
	return err
}

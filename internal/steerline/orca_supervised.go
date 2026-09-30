package steerline

import (
	"context"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

type orcaSupervisedBackend struct{ config Config }

func (b orcaSupervisedBackend) Identity() (executionapi.BackendKind, executionapi.ExecutionProtocol) {
	return executionapi.BackendOrca, executionapi.ProtocolOrcaSupervised
}

func (b orcaSupervisedBackend) Probe(ctx context.Context, target executionapi.ExecutionTarget, agent executionapi.AgentSpec) (executionapi.CapabilityReport, error) {
	fixedTarget := targetFor(b.config)
	if agent.Name != "codex" || !sameSupervisedBaseTarget(target, fixedTarget) {
		return executionapi.CapabilityReport{}, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "supervised Orca probe target or agent differs from fixed configuration"}
	}
	target = fixedTarget
	pair, err := probeSupervisedOrcaTarget(ctx, b.config)
	if err != nil {
		return executionapi.CapabilityReport{}, &executionapi.BackendError{Code: executionapi.BackendUnreachable, Diagnostic: "supervised Orca target preflight failed", Cause: err}
	}
	report := capabilityReport(executionapi.BackendOrca, executionapi.ProtocolOrcaSupervised, target, agent, pair.RemoteVersion)
	report.CenterVersion = pair.CenterVersion
	report.ExpectedAgentVersion = b.config.Orca.Supervised.ExpectedCodexVersion
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

func (b orcaSupervisedBackend) Observe(ctx context.Context, ref executionapi.ExecutionRef) (executionapi.Observation, error) {
	if err := validateSupervisedRef(b.config, ref); err != nil {
		return executionapi.Observation{}, err
	}
	show, err := b.showWorker(ctx, supervisedStartReceipt{RunID: ref.External.OrchestrationRun, TaskID: ref.External.Task, DispatchID: ref.External.Dispatch, WorktreeID: ref.External.WorktreeID, CenterRuntimeID: ref.External.CoordinatorRuntimeID})
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

func (b orcaSupervisedBackend) InspectWorkspace(ctx context.Context, ref executionapi.ExecutionRef) (executionapi.WorkspaceInspection, error) {
	if err := validateSupervisedRef(b.config, ref); err != nil {
		return executionapi.WorkspaceInspection{}, err
	}
	pair, err := probeSupervisedOrcaTarget(ctx, b.config)
	if err != nil {
		return executionapi.WorkspaceInspection{}, &executionapi.BackendError{Code: executionapi.BackendUnreachable, Diagnostic: "supervised workspace preflight failed", Cause: err}
	}
	return b.inspectSupervisedWorktree(ctx, pair, ref.External.WorktreeID, ref.WorkspacePath, ref.WorkspaceBranch, ref.BaseCommit, "")
}

func validateSupervisedRef(c Config, ref executionapi.ExecutionRef) error {
	if ref.Backend != executionapi.BackendOrca || ref.Protocol != executionapi.ProtocolOrcaSupervised || !sameSupervisedBaseTarget(ref.Target, targetFor(c)) || ref.Target.OrchestrationRun != ref.External.OrchestrationRun || ref.Target.ExternalTask != ref.External.Task || ref.Target.Dispatch != ref.External.Dispatch || ref.Target.ExecutionHost != ref.External.ExecutionHost || ref.Target.WorktreeID != ref.External.WorktreeID || ref.External.Kind != "orca_supervised" || ref.External.RuntimeID == "" || ref.External.CoordinatorRuntimeID == "" || ref.External.OrchestrationRun == "" || ref.External.Task == "" || ref.External.Dispatch == "" || ref.External.ExecutionHost == "" || ref.External.WorktreeID == "" {
		return &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "supervised Orca reference does not match the exact configured target and Dispatch identity"}
	}
	return nil
}

func sameSupervisedBaseTarget(a, b executionapi.ExecutionTarget) bool {
	return a.ID != "" && a.ID == b.ID && a.ConnectionID == b.ConnectionID && a.ExternalRuntime == b.ExternalRuntime
}

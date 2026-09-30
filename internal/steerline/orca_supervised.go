package steerline

import (
	"context"
)

type orcaSupervisedBackend struct{ config Config }

func (b orcaSupervisedBackend) Identity() (BackendKind, ExecutionProtocol) {
	return BackendOrca, ProtocolOrcaSupervised
}

func (b orcaSupervisedBackend) Probe(ctx context.Context, target ExecutionTarget, agent AgentSpec) (CapabilityReport, error) {
	fixedTarget := targetFor(b.config)
	if agent.Name != "codex" || !sameSupervisedBaseTarget(target, fixedTarget) {
		return CapabilityReport{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "supervised Orca probe target or agent differs from fixed configuration"}
	}
	target = fixedTarget
	pair, err := probeSupervisedOrcaTarget(ctx, b.config)
	if err != nil {
		return CapabilityReport{}, &BackendError{Code: BackendUnreachable, Diagnostic: "supervised Orca target preflight failed", Cause: err}
	}
	report := capabilityReport(BackendOrca, ProtocolOrcaSupervised, target, agent, pair.RemoteVersion)
	report.CenterVersion = pair.CenterVersion
	report.ExpectedAgentVersion = b.config.Orca.Supervised.ExpectedCodexVersion
	report.Capabilities = append(report.Capabilities,
		Capability{Name: "message_inbox", Status: CapabilityNotTested},
		Capability{Name: "remote_cancellation", Status: CapabilityUnsupported, Evidence: "Spec 03 fixes remote_cancel_available=false"},
	)
	for i := range report.Capabilities {
		if report.Capabilities[i].Name == "stop" || report.Capabilities[i].Name == "retry" {
			report.Capabilities[i].Status = CapabilityUnsupported
		}
		if report.Capabilities[i].Name == "resource_settlement" {
			report.Capabilities[i].Status = CapabilityNotTested
		}
	}
	return report, nil
}

func (b orcaSupervisedBackend) Observe(ctx context.Context, ref ExecutionRef) (Observation, error) {
	if err := validateSupervisedRef(b.config, ref); err != nil {
		return Observation{}, err
	}
	show, err := b.showWorker(ctx, supervisedStartReceipt{RunID: ref.External.OrchestrationRun, TaskID: ref.External.Task, DispatchID: ref.External.Dispatch, WorktreeID: ref.External.WorktreeID, CenterRuntimeID: ref.External.CoordinatorRuntimeID})
	if err != nil {
		return Observation{}, err
	}
	if show.Identity != ref.External {
		return Observation{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "supervised worker observation returned a different exact execution identity"}
	}
	connection := "connected"
	if show.Liveness == "unverifiable" {
		connection = "unverifiable"
	}
	diagnostic := show.DispatchState
	if show.AgentWaitState == "present" {
		diagnostic = "host_interaction_pending"
	}
	return Observation{Liveness: show.Liveness, Connection: connection, ObservedAt: show.ObservedAt, Source: "orca-supervised-worker-show", Diagnostic: diagnostic}, nil
}

func (b orcaSupervisedBackend) InspectWorkspace(ctx context.Context, ref ExecutionRef) (WorkspaceInspection, error) {
	if err := validateSupervisedRef(b.config, ref); err != nil {
		return WorkspaceInspection{}, err
	}
	pair, err := probeSupervisedOrcaTarget(ctx, b.config)
	if err != nil {
		return WorkspaceInspection{}, &BackendError{Code: BackendUnreachable, Diagnostic: "supervised workspace preflight failed", Cause: err}
	}
	return b.inspectSupervisedWorktree(ctx, pair, ref.External.WorktreeID, ref.WorkspacePath, ref.WorkspaceBranch, ref.BaseCommit, "")
}

func validateSupervisedRef(c Config, ref ExecutionRef) error {
	if ref.Backend != BackendOrca || ref.Protocol != ProtocolOrcaSupervised || !sameSupervisedBaseTarget(ref.Target, targetFor(c)) || ref.Target.OrchestrationRun != ref.External.OrchestrationRun || ref.Target.ExternalTask != ref.External.Task || ref.Target.Dispatch != ref.External.Dispatch || ref.Target.ExecutionHost != ref.External.ExecutionHost || ref.Target.WorktreeID != ref.External.WorktreeID || ref.External.Kind != "orca_supervised" || ref.External.RuntimeID == "" || ref.External.CoordinatorRuntimeID == "" || ref.External.OrchestrationRun == "" || ref.External.Task == "" || ref.External.Dispatch == "" || ref.External.ExecutionHost == "" || ref.External.WorktreeID == "" {
		return &BackendError{Code: BackendIdentityMismatch, Diagnostic: "supervised Orca reference does not match the exact configured target and Dispatch identity"}
	}
	return nil
}

func sameSupervisedBaseTarget(a, b ExecutionTarget) bool {
	return a.ID != "" && a.ID == b.ID && a.ConnectionID == b.ConnectionID && a.ExternalRuntime == b.ExternalRuntime
}

func savedSupervisedTargetConflicts(saved, bound ExecutionTarget) bool {
	return saved.OrchestrationRun != "" && saved.OrchestrationRun != bound.OrchestrationRun ||
		saved.ExternalTask != "" && saved.ExternalTask != bound.ExternalTask ||
		saved.Dispatch != "" && saved.Dispatch != bound.Dispatch ||
		saved.ExecutionHost != "" && saved.ExecutionHost != bound.ExecutionHost ||
		saved.WorktreeID != "" && saved.WorktreeID != bound.WorktreeID
}

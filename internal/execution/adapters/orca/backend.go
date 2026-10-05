package orcaworker

import (
	"context"
	"strings"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func NewBackend(c Config) Backend { return Backend{config: c} }

func now() time.Time { return time.Now().UTC() }

type Backend struct{ config Config }

func (b Backend) Identity() (executionbackend.BackendKind, executionbackend.ExecutionProtocol) {
	return executionbackend.BackendOrca, executionbackend.ProtocolOrcaTerminalV2
}

func (b Backend) Probe(ctx context.Context, target executionbackend.ExecutionTarget, agent executionbackend.AgentSpec) (executionbackend.CapabilityReport, error) {
	if agent.Name == "" {
		return executionbackend.CapabilityReport{}, &executionbackend.BackendError{Code: executionbackend.BackendRejected, Diagnostic: "Orca agent is required"}
	}
	out, err := New(b.config).Raw(ctx, positiveDuration(b.config.CommandTimeout, 30*time.Second), "--version")
	if err != nil {
		return executionbackend.CapabilityReport{}, &executionbackend.BackendError{Code: executionbackend.BackendUnreachable, Diagnostic: "Orca version probe failed", Cause: err}
	}
	version := strings.TrimSpace(string(out))
	if version == "" {
		return executionbackend.CapabilityReport{}, &executionbackend.BackendError{Code: executionbackend.BackendProtocolInvalid, Diagnostic: "Orca version probe returned no version"}
	}
	report := capabilityReport(executionbackend.BackendOrca, executionbackend.ProtocolOrcaTerminalV2, target, agent, version)
	if b.config.AgentVersion != nil {
		version, err := b.config.AgentVersion(ctx)
		if err != nil || strings.TrimSpace(version) == "" {
			return executionbackend.CapabilityReport{}, &executionbackend.BackendError{Code: executionbackend.BackendUnreachable, Diagnostic: "Orca Codex override version probe failed", Cause: err}
		}
		report.AgentVersion = strings.TrimSpace(version)
	}

	return report, nil
}

func (b Backend) Observe(ctx context.Context, ref executionbackend.ExecutionRef) (executionbackend.Observation, error) {
	old := ref.External
	if old.Kind != "orca" || old.Terminal == "" {
		return executionbackend.Observation{}, &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "Orca terminal binding is incomplete"}
	}
	if b.config.Target != ref.Target {
		return executionbackend.Observation{}, &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "configured Orca connection differs from the fixed execution target"}
	}
	if err := ctx.Err(); err != nil {
		return executionbackend.Observation{}, err
	}
	term, e := New(b.config).InspectBoundTerminal(ctx, b.config.Target, old)
	if e != nil {
		return executionbackend.Observation{}, e
	}
	life := "live"
	if term.Exited() {
		life = "exited"
	} else if !term.Connected {
		life = "unverifiable"
	}
	return executionbackend.Observation{Liveness: life, Connection: map[bool]string{true: "connected", false: "unreachable"}[term.Connected], ObservedAt: now(), Source: "orca-terminal-v2", Diagnostic: term.Status}, nil
}

func (b Backend) InspectWorkspace(ctx context.Context, ref executionbackend.ExecutionRef) (executionbackend.WorkspaceInspection, error) {
	if ref.Backend != executionbackend.BackendOrca || ref.Protocol != executionbackend.ProtocolOrcaTerminalV2 || ref.Target != b.config.Target {
		return executionbackend.WorkspaceInspection{}, &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "Orca workspace target differs from the fixed execution"}
	}
	// terminal-v2 worktrees live on this configured host. Future supervised
	// targets must implement their own remote inspection instead of using Git here.
	return b.config.InspectWorkspace(ctx, ref)
}

func (b Backend) StopExecution(ctx context.Context, ref executionbackend.ExecutionRef, _ executionbackend.Operation, handle executionbackend.StopHandle) (executionbackend.StopReceipt, error) {
	obs, err := b.Observe(ctx, ref)
	if err != nil {
		return executionbackend.StopReceipt{}, err
	}
	if obs.Liveness == "exited" {
		return executionbackend.StopReceipt{Disposition: "confirmed", Diagnostic: obs.Diagnostic}, nil
	}
	if obs.Liveness != "live" {
		return executionbackend.StopReceipt{}, &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "Orca terminal is not verifiably live"}
	}
	if handle != nil && handle.Identity() != ref.External {
		return executionbackend.StopReceipt{}, &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "Orca observer handle does not match the fixed terminal identity"}
	}
	closeReceipt, err := New(b.config).CloseTerminalChecked(ctx, ref.External.Terminal, ref.External.RuntimeID)
	if err != nil {
		return executionbackend.StopReceipt{}, err
	}
	if closeReceipt.Disposition == "confirmed" {
		return closeReceipt, nil
	}
	obs, err = b.Observe(ctx, ref)
	if err != nil {
		return executionbackend.StopReceipt{}, &executionbackend.BackendError{Code: executionbackend.BackendOutcomeUnknown, Diagnostic: "terminal close was accepted but stop could not be confirmed", Cause: err}
	}
	if obs.Liveness == "exited" {
		return executionbackend.StopReceipt{Disposition: "confirmed", Diagnostic: obs.Diagnostic}, nil
	}
	return executionbackend.StopReceipt{Disposition: "accepted", Diagnostic: "close accepted; terminal exit is not yet confirmed"}, nil
}

func capabilityReport(backend executionbackend.BackendKind, protocol executionbackend.ExecutionProtocol, target executionbackend.ExecutionTarget, agent executionbackend.AgentSpec, version string) executionbackend.CapabilityReport {
	caps := []executionbackend.Capability{{Name: "start", Status: executionbackend.CapabilityNotTested}, {Name: "observe", Status: executionbackend.CapabilityNotTested}, {Name: "stop", Status: executionbackend.CapabilityNotTested}, {Name: "detach_observation", Status: executionbackend.CapabilityNotTested}, {Name: "operation_reconciliation", Status: executionbackend.CapabilityNotTested}, {Name: "workspace_inspection", Status: executionbackend.CapabilityNotTested}, {Name: "output_reading", Status: executionbackend.CapabilityNotTested}, {Name: "output_turns", Status: executionbackend.CapabilityNotTested}, {Name: "output_final_answer", Status: executionbackend.CapabilityNotTested}, {Name: "resource_settlement", Status: executionbackend.CapabilityUnsupported}}
	return executionbackend.CapabilityReport{Version: 1, Backend: backend, Protocol: protocol, Target: target, Agent: agent, PlatformVersion: version, CheckedAt: now(), Capabilities: caps}
}

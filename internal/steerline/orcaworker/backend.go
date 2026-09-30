package orcaworker

import (
	"context"
	"strings"
	"time"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

func NewBackend(c Config) Backend { return Backend{config: c} }
func now() time.Time              { return time.Now().UTC() }

type Backend struct{ config Config }

func (b Backend) Identity() (executionapi.BackendKind, executionapi.ExecutionProtocol) {
	return executionapi.BackendOrca, executionapi.ProtocolOrcaTerminalV2
}
func (b Backend) Probe(ctx context.Context, target executionapi.ExecutionTarget, agent executionapi.AgentSpec) (executionapi.CapabilityReport, error) {
	if agent.Name == "" {
		return executionapi.CapabilityReport{}, &executionapi.BackendError{Code: executionapi.BackendRejected, Diagnostic: "Orca agent is required"}
	}
	out, err := New(b.config).Raw(ctx, positiveDuration(b.config.CommandTimeout, 30*time.Second), "--version")
	if err != nil {
		return executionapi.CapabilityReport{}, &executionapi.BackendError{Code: executionapi.BackendUnreachable, Diagnostic: "Orca version probe failed", Cause: err}
	}
	version := strings.TrimSpace(string(out))
	if version == "" {
		return executionapi.CapabilityReport{}, &executionapi.BackendError{Code: executionapi.BackendProtocolInvalid, Diagnostic: "Orca version probe returned no version"}
	}
	report := capabilityReport(executionapi.BackendOrca, executionapi.ProtocolOrcaTerminalV2, target, agent, version)
	if b.config.AgentVersion != nil {
		version, err := b.config.AgentVersion(ctx)
		if err != nil || strings.TrimSpace(version) == "" {
			return executionapi.CapabilityReport{}, &executionapi.BackendError{Code: executionapi.BackendUnreachable, Diagnostic: "Orca Codex override version probe failed", Cause: err}
		}
		report.AgentVersion = strings.TrimSpace(version)
	}

	return report, nil
}
func (b Backend) Observe(ctx context.Context, ref executionapi.ExecutionRef) (executionapi.Observation, error) {
	old := ref.External
	if old.Kind != "orca" || old.Terminal == "" {
		return executionapi.Observation{}, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "Orca terminal binding is incomplete"}
	}
	if b.config.Target != ref.Target {
		return executionapi.Observation{}, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "configured Orca connection differs from the fixed execution target"}
	}
	if err := ctx.Err(); err != nil {
		return executionapi.Observation{}, err
	}
	term, e := New(b.config).InspectBoundTerminal(ctx, b.config.Target, old)
	if e != nil {
		return executionapi.Observation{}, e
	}
	life := "live"
	if term.Exited() {
		life = "exited"
	} else if !term.Connected {
		life = "unverifiable"
	}
	return executionapi.Observation{Liveness: life, Connection: map[bool]string{true: "connected", false: "unreachable"}[term.Connected], ObservedAt: now(), Source: "orca-terminal-v2", Diagnostic: term.Status}, nil
}
func (b Backend) InspectWorkspace(ctx context.Context, ref executionapi.ExecutionRef) (executionapi.WorkspaceInspection, error) {
	if ref.Backend != executionapi.BackendOrca || ref.Protocol != executionapi.ProtocolOrcaTerminalV2 || ref.Target != b.config.Target {
		return executionapi.WorkspaceInspection{}, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "Orca workspace target differs from the fixed execution"}
	}
	// terminal-v2 worktrees live on this configured host. Future supervised
	// targets must implement their own remote inspection instead of using Git here.
	return b.config.InspectWorkspace(ctx, ref)
}

func (b Backend) StopExecution(ctx context.Context, ref executionapi.ExecutionRef, _ executionapi.Operation, handle executionapi.StopHandle) (executionapi.StopReceipt, error) {
	obs, err := b.Observe(ctx, ref)
	if err != nil {
		return executionapi.StopReceipt{}, err
	}
	if obs.Liveness == "exited" {
		return executionapi.StopReceipt{Disposition: "confirmed", Diagnostic: obs.Diagnostic}, nil
	}
	if obs.Liveness != "live" {
		return executionapi.StopReceipt{}, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "Orca terminal is not verifiably live"}
	}
	if handle != nil && handle.Identity() != ref.External {
		return executionapi.StopReceipt{}, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "Orca observer handle does not match the fixed terminal identity"}
	}
	closeReceipt, err := New(b.config).CloseTerminalChecked(ctx, ref.External.Terminal, ref.External.RuntimeID)
	if err != nil {
		return executionapi.StopReceipt{}, err
	}
	if closeReceipt.Disposition == "confirmed" {
		return closeReceipt, nil
	}
	obs, err = b.Observe(ctx, ref)
	if err != nil {
		return executionapi.StopReceipt{}, &executionapi.BackendError{Code: executionapi.BackendOutcomeUnknown, Diagnostic: "terminal close was accepted but stop could not be confirmed", Cause: err}
	}
	if obs.Liveness == "exited" {
		return executionapi.StopReceipt{Disposition: "confirmed", Diagnostic: obs.Diagnostic}, nil
	}
	return executionapi.StopReceipt{Disposition: "accepted", Diagnostic: "close accepted; terminal exit is not yet confirmed"}, nil
}

func capabilityReport(backend executionapi.BackendKind, protocol executionapi.ExecutionProtocol, target executionapi.ExecutionTarget, agent executionapi.AgentSpec, version string) executionapi.CapabilityReport {
	caps := []executionapi.Capability{{Name: "start", Status: executionapi.CapabilityNotTested}, {Name: "observe", Status: executionapi.CapabilityNotTested}, {Name: "stop", Status: executionapi.CapabilityNotTested}, {Name: "detach_observation", Status: executionapi.CapabilityNotTested}, {Name: "operation_reconciliation", Status: executionapi.CapabilityNotTested}, {Name: "workspace_inspection", Status: executionapi.CapabilityNotTested}, {Name: "output_reading", Status: executionapi.CapabilityNotTested}, {Name: "output_turns", Status: executionapi.CapabilityNotTested}, {Name: "output_final_answer", Status: executionapi.CapabilityNotTested}, {Name: "resource_settlement", Status: executionapi.CapabilityUnsupported}}
	return executionapi.CapabilityReport{Version: 1, Backend: backend, Protocol: protocol, Target: target, Agent: agent, PlatformVersion: version, CheckedAt: now(), Capabilities: caps}
}

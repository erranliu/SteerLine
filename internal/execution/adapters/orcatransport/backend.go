package orcatransport

import (
	"context"
	"errors"
	"fmt"
	"time"

	native "github.com/erranliu/SteerLine/internal/execution/adapters/protocol/orca"
	b "github.com/erranliu/SteerLine/internal/execution/backend"
)

type Backend struct {
	Profile   Profile
	Output    func(context.Context, time.Duration, ...string) ([]byte, error)
	Workspace func(context.Context, b.ExecutionRef) (b.WorkspaceInspection, error)
}

func (v Backend) Identity() (b.BackendKind, b.ExecutionProtocol) {
	return b.BackendOrca, b.ProtocolOrcaTerminalV3
}
func (v Backend) client() native.Client {
	return native.New(native.Config{Output: v.Output, CommandTimeout: "40s", Target: v.Profile.Target()})
}
func (v Backend) Probe(ctx context.Context, target b.ExecutionTarget, agent b.AgentSpec) (b.CapabilityReport, error) {
	if target.ID != v.Profile.Target().ID || agent.Name == "" {
		return b.CapabilityReport{}, errors.New("Orca probe differs from configured target")
	}
	_, err := v.client().MetaCall(ctx, "status", "--json")
	if err != nil {
		return b.CapabilityReport{}, fmt.Errorf("Orca terminal preflight: %w", err)
	}
	return b.CapabilityReport{Version: 1, Backend: b.BackendOrca, Protocol: b.ProtocolOrcaTerminalV3, Target: target, Agent: agent, CheckedAt: time.Now().UTC(), Capabilities: []b.Capability{{Name: "start", Status: b.CapabilityNotTested}, {Name: "observe", Status: b.CapabilityNotTested}, {Name: "stop", Status: b.CapabilityNotTested}, {Name: "output_reading", Status: b.CapabilityNotTested}}}, nil
}
func (v Backend) Observe(ctx context.Context, ref b.ExecutionRef) (b.Observation, error) {
	if ref.Protocol != b.ProtocolOrcaTerminalV3 || ref.Target.ID != v.Profile.Target().ID || ref.External.Kind != "orca_terminal" {
		return b.Observation{}, errors.New("invalid original Orca terminal binding")
	}
	terminal, err := v.client().InspectBoundTerminal(ctx, ref.Target, ref.External)
	if err != nil {
		return b.Observation{}, err
	}
	life := "live"
	if terminal.Exited() {
		life = "exited"
	} else if !terminal.Connected {
		life = "unverifiable"
	}
	return b.Observation{Liveness: life, Connection: "connected", ObservedAt: time.Now().UTC(), Source: "orca.terminal.show"}, nil
}
func (v Backend) InspectWorkspace(ctx context.Context, ref b.ExecutionRef) (b.WorkspaceInspection, error) {
	if v.Workspace == nil || ref.Protocol != b.ProtocolOrcaTerminalV3 || ref.Target.ID != v.Profile.Target().ID {
		return b.WorkspaceInspection{}, errors.New("unsupported Orca workspace inspection")
	}
	return v.Workspace(ctx, ref)
}
func (v Backend) StopExecution(ctx context.Context, ref b.ExecutionRef, _ b.Operation, _ b.StopHandle) (b.StopReceipt, error) {
	if _, err := v.Observe(ctx, ref); err != nil {
		return b.StopReceipt{}, err
	}
	return v.client().CloseTerminalChecked(ctx, ref.External.Terminal, ref.External.RuntimeID)
}

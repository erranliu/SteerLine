package codexworker

import (
	"context"
	b "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/execution/reliable"
	"testing"
)

func TestPreparationApplicabilitySkipsTerminalLaunch(t *testing.T) {
	l := &ExecutionLifecycle{}
	s := b.LifecycleSnapshot{Workspace: b.WorkspaceIntent{Path: "owned", Branch: "original", BaseCommit: "base"}, PendingTurn: &b.TurnInput{CommandID: "command", Prompt: "saved"}, Session: b.SessionContext{ProjectID: "project", ExecutionID: "execution", AttemptID: "attempt", WorkflowRunID: "association"}, ResourceGeneration: "resource", ConfigurationDigest: "config"}
	facts, err := l.PreparationFacts(context.Background(), s)
	if err != nil || facts.Resource != b.GateNotApplicable || facts.Session != b.GateRequired {
		t.Fatalf("app-server prerequisites: %+v %v", facts, err)
	}
	next, err := reliable.Next(context.Background(), l, s)
	if err != nil || next.Action != b.LifecyclePrepareSession {
		t.Fatalf("app-server planned Orca launch: %+v %v", next, err)
	}
	// An endpoint alone is partial success, not proof that thread preparation
	// completed; it must not authorize the first model input.
	partial, err := NewAppServerBindingEnvelope(l.identity(s), AppServerBinding{Endpoint: "ws://127.0.0.1:12345", Generation: "server-generation"})
	if err != nil {
		t.Fatal(err)
	}
	s.Implementation = []b.ImplementationDataEnvelope{partial}
	if candidate, err := reliable.Next(context.Background(), l, s); err == nil || candidate.Action != "" {
		t.Fatal("partial app-server binding bypassed thread readiness")
	}
	s.Implementation = nil
	// The native session preparation is still mandatory; declared opt-out of
	// terminal launch is never an opt-out of handshake/thread readiness.
	data, err := NewAppServerBindingEnvelope(l.identity(s), AppServerBinding{Endpoint: "ws://127.0.0.1:12345", SessionID: "00000000-0000-0000-0000-000000000001", Generation: "server-generation"})
	if err != nil {
		t.Fatal(err)
	}
	s.Implementation = []b.ImplementationDataEnvelope{data}
	next, err = reliable.Next(context.Background(), l, s)
	if err != nil || next.Action != b.LifecycleStartTurn {
		t.Fatalf("verified session could not start: %+v %v", next, err)
	}
}

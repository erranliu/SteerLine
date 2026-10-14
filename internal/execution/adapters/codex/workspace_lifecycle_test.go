package codexworker

import (
	"context"
	"github.com/erranliu/SteerLine/internal/execution/reliable"
	"strings"
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

type unreachableWorkspaceHost struct {
	executionbackend.ManagedProcessHost
}

func TestWorkspaceLifecyclePrecedesSessionAndRejectsRetargeting(t *testing.T) {
	lifecycle := &ExecutionLifecycle{setup: executionbackend.LifecycleSetup{ProcessHost: unreachableWorkspaceHost{}}}
	intent := executionbackend.WorkspaceIntent{Path: t.TempDir(), Branch: "codex/original", BaseCommit: "0123456789abcdef0123456789abcdef01234567"}
	snapshot := executionbackend.LifecycleSnapshot{WorkspacePlan: intent, PendingTurn: &executionbackend.TurnInput{CommandID: "0123456789abcdef0123456789abcdef", Prompt: "original input"}}
	candidate, err := reliable.Next(context.Background(), lifecycle, snapshot)
	if err != nil || candidate.Action != executionbackend.LifecyclePrepareWorkspace || candidate.Input.Workspace == nil || *candidate.Input.Workspace != intent {
		t.Fatalf("session setup preceded its durable workspace intent: %+v / %v", candidate, err)
	}
	changed := intent
	changed.Branch = "codex/other"
	request := executionbackend.LifecycleRequest{Snapshot: snapshot, Input: executionbackend.LifecycleInput{Workspace: &changed}}
	for _, reconcile := range []bool{false, true} {
		receipt, err := lifecycle.prepareWorkspace(context.Background(), request, reconcile)
		if err == nil || !strings.Contains(err.Error(), "original fixed intent") || receipt.Disposition != executionbackend.OperationRejected || receipt.Workspace != nil {
			t.Fatal("changed workspace intent reached a native Git operation")
		}
	}
	snapshot.Session.SessionID = "original-session"
	if _, err := reliable.Next(context.Background(), lifecycle, snapshot); err == nil {
		t.Fatal("a bound session planned a replacement for its missing original workspace")
	}
}

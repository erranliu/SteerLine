package codexworker

import (
	"context"
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func TestTurnParamsRejectsUnboundCommandBeforeWorkspaceProbe(t *testing.T) {
	called := false
	_, err := TurnParams(context.Background(), TurnParamsInput{
		Worktree: "C:/fixed/worktree", ThreadID: "00000000-0000-0000-0000-000000000001",
		TurnInput: executionbackend.TurnInput{CommandID: "wrong", Prompt: "continue"}, Sandbox: "workspace-write",
	}, func(context.Context, string) ([]string, error) {
		called = true
		return nil, nil
	})
	if err == nil || called {
		t.Fatalf("unbound command reached workspace probe: %v, called=%v", err, called)
	}
}

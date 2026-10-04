package steerline

import (
	"strings"
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

func TestSupervisedPrepareKeepsPlatformWorktreeIdentitySeparateFromPath(t *testing.T) {
	const worktreeID = "repo-test::C:/remote/steerline-run"
	run := &Run{Protocol: string(neutralProtocol), Target: executionbackend.ExecutionTarget{WorktreeID: worktreeID}}
	execution := &executionmodel.ExecutionRecord{Protocol: run.Protocol, Target: run.Target}
	run.Target.WorktreeID = "incorrect-view-only-id"
	workspace := executionmodel.WorkspaceEvidence{Worktree: "C:/remote/steerline-run", Branch: "erranliu/steerline-run", Base: strings.Repeat("a", 40), Head: strings.Repeat("a", 40)}

	applyWorkflowWorkspaceEvidence(run, execution, workspace)

	if execution.Target.WorktreeID != worktreeID {
		t.Fatalf("supervised target lost the exact implementation worktree ID: got %q want %q", execution.Target.WorktreeID, worktreeID)
	}
	if run.Target.WorktreeID != worktreeID {
		t.Fatalf("supervised Run target was not synchronized with its Execution target: got %q want %q", run.Target.WorktreeID, worktreeID)
	}
	if execution.Workspace.Worktree != workspace.Worktree || execution.Target.Branch != workspace.Branch || execution.Target.BaseRevision != workspace.Base {
		t.Fatalf("supervised workspace evidence was not persisted separately: target=%+v workspace=%+v", execution.Target, execution.Workspace)
	}
}

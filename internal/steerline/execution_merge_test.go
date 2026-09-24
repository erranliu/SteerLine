package steerline

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileExecutionStateIsRejectedWithOrWithoutDatabase(t *testing.T) {
	for _, database := range []bool{false, true} {
		dir := t.TempDir()
		if database {
			db, err := OpenSQLiteStore(filepath.Join(dir, "steerline.db"))
			require(t, err)
			require(t, db.Close())
		}
		path := filepath.Join(dir, "execution", "v1", "bindings", newID())
		require(t, os.MkdirAll(path, 0700))
		if err := preflightExistingState(dir, "runtime.json"); err == nil {
			t.Fatal("file-based execution state was accepted")
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatal("old state was removed", err)
		}
	}
}

func TestMinimalStepUsesFixedBindingForConfirmedStop(t *testing.T) {
	f := newFixture(t, task("merge-stop"))
	exe, err := os.Executable()
	require(t, err)
	contract := func(step string) StepContract {
		return StepContract{Provider: "command", Entry: "builtin:" + step, Version: "test-v1", InputSchema: MinimalStepInputSchema, OutputSchema: MinimalStepOutputSchema, ExecutionSite: "local", Timeout: time.Minute, Command: []string{exe, "--fake", "sleep"}}
	}
	f.c.WorkflowDefinition = ptr(DefaultMinimalWorkflowDefinition(false))
	f.c.ProjectSteps = &ProjectSteps{Implement: contract("implement"), Verify: contract("verify"), PublishPR: contract("publish_pr")}
	f.c.ProjectSteps.Verify.RequiredChecks = []string{"unit"}
	s := f.service(t)
	defer s.Close()
	require(t, s.dispatch(context.Background()))
	defer func() {
		if s.worker != nil {
			s.worker.Kill()
		}
	}()
	run := s.State.Runs[0]
	op, err := s.stopActiveExecution(context.Background())
	require(t, err)
	select {
	case <-s.worker.Done:
	case <-time.After(5 * time.Second):
		t.Fatal("step did not stop")
	}
	confirmed, err := s.confirmExecutionStop(op)
	require(t, err)
	if !confirmed || op.AttemptID != run.AttemptID || op.Ref == nil || op.Ref.External != *run.Process {
		t.Fatalf("step stop lost its fixed identity: %+v", op)
	}
	if _, err := os.Stat(filepath.Join(f.c.RunDir, "execution", "v1", "bindings")); !os.IsNotExist(err) {
		t.Fatalf("binding escaped the SQLite authority: %v", err)
	}
}

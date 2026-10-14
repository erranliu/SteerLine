package execution

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

type closedPRQueryCounter struct {
	executionbackend.ManagedProcessHost
	queries atomic.Int32
}

func (p *closedPRQueryCounter) StartManaged(cmd *exec.Cmd) (executionbackend.StopHandle, error) {
	if len(cmd.Args) == 3 && cmd.Args[1] == "api" && cmd.Args[2] == "repos/owner/repo/pulls/8" {
		p.queries.Add(1)
	}
	return p.ManagedProcessHost.StartManaged(cmd)
}

func TestWorkerFlowClosedDeliveryNoQueriesAfterActorRestart(t *testing.T) {
	s, run := neutralAuditService(t)
	counter := &closedPRQueryCounter{ManagedProcessHost: s.runtime.ProcessHost}
	s.runtime.ProcessHost = counter
	ctx := context.Background()
	w, e := s.workflowRecords(run)
	branch, head := e.WorkspaceIntent.Branch, e.WorkspaceIntent.BaseCommit
	url := "https://github.com/owner/repo/pull/8"
	provider := filepath.Join(t.TempDir(), "provider.json")
	t.Setenv("STEERLINE_TEST_HOST_GH_FILE", provider)
	info := executionmodel.PRInfo{URL: url, Repository: "owner/repo", BaseBranch: "main", HeadRepository: "owner/repo", HeadBranch: branch, HeadRevision: head, State: "open"}
	raw, err := json.Marshal(info)
	require(t, err)
	require(t, os.WriteFile(provider, raw, 0600))
	w.StartPending = false
	w.Status, w.StepID, w.PRURL, w.PRState = "waiting_external", executionmodel.StepAwaitMerge, url, "open"
	e.Status, e.Workspace.Branch = "idle", branch
	e.Workspace.Worktree = e.WorkspaceIntent.Path
	setNeutralBinding(t, e, "ws://127.0.0.1:12345", "original-generation", "original-session")
	e.PolicyResources = neutralPolicyReceipt(w.ExecutionProfile.Digest, e.Workspace.Worktree, e)
	require(t, s.commitWorker(run))
	require(t, s.pollPRs(ctx))
	if counter.queries.Load() != 1 {
		t.Fatal("original open candidate did not reach the counting provider")
	}
	at := time.Now().UTC()
	info.State, info.Merged, info.MergedAt, info.MergeCommit = "merged", true, &at, &head
	raw, err = json.Marshal(info)
	require(t, err)
	require(t, os.WriteFile(provider, raw, 0600))
	task := nativeTaskRecord(s.domain.Tasks, "8")
	receipt, err := s.commandService(s.Config.Identity()).CompleteNativeTask(ctx, s.Config, "8", task.Version, run.RunID, publicationClient(s.Config, counter))
	require(t, err)
	if receipt.Status != "closed" || counter.queries.Load() != 2 {
		t.Fatal("native provider merge did not produce a formal delivery")
	}
	require(t, s.refreshNativeSnapshot(ctx))
	assertClosed := func(s *Service) {
		t.Helper()
		task := nativeTaskRecord(s.domain.Tasks, "8")
		current := s.find(run.RunID)
		if task == nil || task.Status != "closed" || current == nil {
			t.Fatal("formal delivery did not retain the closed original task")
		}
		w, e := s.workflowRecords(current)
		if w.Status != "completed" || len(w.StageInputs) != 0 || e.PendingCommandID != "" || e.ExecutionID != run.ExecutionID || len(s.domain.WorkflowRuns) != 1 || len(s.domain.Executions) != 1 || len(s.domain.Attempts) != 1 {
			t.Fatal("closed polling/reconstruction created another candidate, Attempt or input")
		}
		before := recordFingerprint(s.domain)
		for i := 0; i < 3; i++ {
			require(t, s.pollPRs(ctx))
		}
		if counter.queries.Load() != 2 || recordFingerprint(s.domain) != before || len(s.domain.Deliveries) != 1 {
			t.Fatal("closed polling queried provider or changed delivery/input facts")
		}
		t.Logf("service=%s provider_queries=%d deliveries=%d", s.State.Service.InstanceID, counter.queries.Load(), len(s.domain.Deliveries))
	}
	assertClosed(s)
	require(t, s.publishCheckpoint())
	priorInstance, config := s.State.Service.InstanceID, s.Config
	require(t, s.Close())
	restored, err := newNeutralService(config)
	require(t, err)
	t.Cleanup(func() { _ = restored.Close() })
	restored.runtime.ProcessHost = counter
	actorCtx, cancel := context.WithCancel(ctx)
	defer func() {
		cancel()
		if restored.historyDone != nil {
			<-restored.historyDone
		}
	}()
	require(t, restored.BeginActor(actorCtx))
	require(t, restored.Initialize(actorCtx, cancel))
	if !restored.State.Service.Ready || restored.State.Service.InstanceID == priorInstance {
		t.Fatal("restored actor did not enter a distinct ready service epoch")
	}
	assertClosed(restored)
}

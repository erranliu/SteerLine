package steerline

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func completionFixture(t *testing.T) *fixture {
	f := newFixture(t, task("t-1"))
	exe, e := os.Executable()
	require(t, e)
	f.c.Completion = &CompletionConfig{Command: Command{Executable: exe, Args: []string{"--fake", "completion", f.db, "{worktree_path}"}}, Timeout: "5s"}
	return f
}

func TestCompletionPartialSuccessRetriesAfterClosedAndRestart(t *testing.T) {
	f := completionFixture(t)
	s := f.service(t)
	ctx := context.Background()
	require(t, s.dispatch(ctx))
	finishWorker(t, s)
	require(t, s.pollPRs(ctx))
	if readFake(f.db).CompletionCalls != 0 {
		t.Fatal("completion called while PR open")
	}
	f.pr("1", "failure")
	require(t, s.pollPRs(ctx))
	if readFake(f.db).CompletionCalls != 0 {
		t.Fatal("completion called without merge evidence")
	}
	f.pr("1", "merged")
	db := readFake(f.db)
	db.FailCompletion = 1
	writeFake(f.db, db)
	if s.pollPRs(ctx) == nil {
		t.Fatal("expected post-close completion failure")
	}
	r := s.State.Runs[0]
	if r.FinishTaskStatus != "waiting_pr_merge" || r.BeadsStatus != "closed" {
		t.Fatal("partial completion incorrectly finalized")
	}
	out, e := os.ReadFile(filepath.Join(f.c.RunDir, "runs", r.RunID, "completion-result.json"))
	require(t, e)
	if !strings.Contains(string(out), "changed") {
		t.Fatal("partial-write evidence discarded")
	}
	// Removing the setting must not let this already-dispatched run skip its tail.
	f.c.Completion = nil
	s = f.service(t)
	require(t, s.pollPRs(ctx))
	if s.State.Runs[0].FinishTaskStatus != "completed" || readFake(f.db).CompletionCalls != 2 {
		t.Fatal("closed task skipped pinned completion callback")
	}
	var receipt CompletionReceipt
	out, e = os.ReadFile(filepath.Join(f.c.RunDir, "runs", r.RunID, "completion-result.json"))
	require(t, e)
	require(t, json.Unmarshal(out, &receipt))
	if receipt.Status != "completed" {
		t.Fatal("completion receipt not retained")
	}
}

func TestInvalidCompletionReceiptsNeverComplete(t *testing.T) {
	for _, mode := range []string{"wrong-run", "wrong-task", "wrong-pr", "malformed", "no-close"} {
		t.Run(mode, func(t *testing.T) {
			f := completionFixture(t)
			s := f.service(t)
			ctx := context.Background()
			require(t, s.dispatch(ctx))
			finishWorker(t, s)
			db := readFake(f.db)
			db.CompletionMode = mode
			writeFake(f.db, db)
			f.pr("1", "merged")
			if s.pollPRs(ctx) == nil || s.State.Runs[0].FinishTaskStatus == "completed" {
				t.Fatal("invalid completion accepted")
			}
		})
	}
}

func TestCompletionWrongTargetAndOwnerDoNotInvoke(t *testing.T) {
	f := completionFixture(t)
	s := f.service(t)
	ctx := context.Background()
	require(t, s.dispatch(ctx))
	finishWorker(t, s)
	f.pr("1", "wrong-base")
	require(t, s.pollPRs(ctx))
	if readFake(f.db).CompletionCalls != 0 {
		t.Fatal("wrong target invoked completion")
	}
	r := s.State.Runs[0]
	r.FinishTaskStatus = "waiting_pr_merge"
	f.pr("1", "merged")
	db := readFake(f.db)
	db.Tasks[0].Assignee = "other"
	writeFake(f.db, db)
	if s.pollPRs(ctx) == nil || readFake(f.db).CompletionCalls != 0 {
		t.Fatal("wrong owner invoked completion")
	}
}

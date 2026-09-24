package steerline

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixtureGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, e := cmd.CombinedOutput()
	if e != nil {
		t.Fatalf("git %v: %v %s", args, e, out)
	}
	return strings.TrimSpace(string(out))
}

func TestStopDuringWorkspacePreparation(t *testing.T) {
	f := newFixture(t, task("t-1"))
	db := readFake(f.db)
	db.Behavior = "slow-context"
	writeFake(f.db, db)
	s := f.service(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, e := os.Stat(f.db + ".context-started"); e == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("preparation did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	require(t, RequestStop(f.c))
	select {
	case e := <-done:
		require(t, e)
	case <-time.After(3 * time.Second):
		t.Fatal("stop waited for blocked preparation command")
	}
	db = readFake(f.db)
	if len(db.Claims) != 0 || len(db.Starts) != 0 {
		t.Fatal("stop dispatched new work")
	}
}

func TestImplementationCommitsKeepOriginalBase(t *testing.T) {
	f := newFixture(t, task("t-1"))
	s := f.service(t)
	require(t, s.dispatch(context.Background()))
	finishWorker(t, s)
	r := s.State.Runs[0]
	base := *r.BaseCommit
	require(t, os.WriteFile(filepath.Join(*r.WorktreePath, "implementation.txt"), []byte("task change"), 0600))
	fixtureGit(t, *r.WorktreePath, "add", "implementation.txt")
	fixtureGit(t, *r.WorktreePath, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "Implement task")
	require(t, verifyRunWorkspace(context.Background(), s.Config, r))
	head := fixtureGit(t, *r.WorktreePath, "rev-parse", "HEAD")
	if head == base {
		t.Fatal("test did not advance HEAD")
	}
	result, e := ReadResult(*r.ResultPath, r)
	require(t, e)
	result.BaseCommit = &head
	if s.bind(r, result) == nil || *r.BaseCommit != base {
		t.Fatal("implementation HEAD replaced immutable base")
	}
}
func TestWorkspaceTracksFreshRemoteWithoutChangingSource(t *testing.T) {
	f := newFixture(t, task("t-1"), task("t-2"))
	db := readFake(f.db)
	db.Dependencies["t-2"] = "t-1"
	writeFake(f.db, db)
	s := f.service(t)
	ctx := context.Background()
	beforeHead := fixtureGit(t, f.c.Repository, "rev-parse", "HEAD")
	beforeBranch := fixtureGit(t, f.c.Repository, "symbolic-ref", "HEAD")
	require(t, os.WriteFile(filepath.Join(f.c.Repository, "local-only.txt"), []byte("keep original checkout"), 0600))
	beforeStatus := fixtureGit(t, f.c.Repository, "status", "--porcelain")
	require(t, s.dispatch(ctx))
	finishWorker(t, s)
	first := s.State.Runs[0]
	if *first.BaseCommit != beforeHead || canonical(*first.WorktreePath) == canonical(f.c.Repository) {
		t.Fatal("first workspace not isolated at base")
	}
	if fixtureGit(t, *first.WorktreePath, "branch", "--show-current") != *first.WorktreeBranch {
		t.Fatal("wrong worktree branch")
	}
	require(t, s.dispatch(ctx))
	if s.worker != nil {
		t.Fatal("dependent started before predecessor merge")
	}
	// A separate producer advances the remote; the service source checkout stays stale.
	producer := filepath.Join(filepath.Dir(f.c.Repository), "producer")
	fixtureGit(t, filepath.Dir(f.c.Repository), "clone", "--quiet", "--branch", "main", filepath.Join(filepath.Dir(f.c.Repository), "remote.git"), producer)
	fixtureGit(t, producer, "config", "core.hooksPath", filepath.Join(filepath.Dir(f.c.Repository), "no-hooks"))
	require(t, os.WriteFile(filepath.Join(producer, "merged.txt"), []byte("new remote mainline"), 0600))
	fixtureGit(t, producer, "add", "merged.txt")
	fixtureGit(t, producer, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "Remote advances")
	newHead := fixtureGit(t, producer, "rev-parse", "HEAD")
	fixtureGit(t, producer, "push", "--quiet", "origin", "main")
	f.pr("1", "merged")
	require(t, s.pollPRs(ctx))
	require(t, s.dispatch(ctx))
	finishWorker(t, s)
	second := s.State.Runs[1]
	if *second.BaseCommit != newHead || *second.WorktreePath == *first.WorktreePath {
		t.Fatal("new run did not use freshly fetched mainline")
	}
	if _, e := os.Stat(filepath.Join(*second.WorktreePath, "merged.txt")); e != nil {
		t.Fatal(e)
	}
	if fixtureGit(t, f.c.Repository, "rev-parse", "HEAD") != beforeHead || fixtureGit(t, f.c.Repository, "symbolic-ref", "HEAD") != beforeBranch || fixtureGit(t, f.c.Repository, "status", "--porcelain") != beforeStatus {
		t.Fatal("source checkout changed")
	}
	if _, e := os.Stat(*first.WorktreePath); e != nil {
		t.Fatal("waiting worktree removed")
	}
	for _, run := range s.State.Runs {
		prompt, e := os.ReadFile(filepath.Join(f.c.RunDir, "runs", run.RunID, "prompt.txt"))
		require(t, e)
		if !strings.Contains(string(prompt), filepath.Join(*run.WorktreePath, "start.md")) {
			t.Fatal("prompt used original checkout skill")
		}
	}
}

func TestFetchFailureLeavesTaskUnclaimed(t *testing.T) {
	f := newFixture(t, task("t-1"))
	f.c.Branch = "does-not-exist"
	s := f.service(t)
	if s.dispatch(context.Background()) == nil {
		t.Fatal("expected fetch failure")
	}
	db := readFake(f.db)
	if len(db.Claims) != 0 || len(db.Starts) != 0 || len(s.State.Runs) != 0 || s.State.SlotOccupied {
		t.Fatal("fetch failure claimed or launched task")
	}
}

func TestPreparationFailurePreservesClaimAndContents(t *testing.T) {
	for _, behavior := range []string{"path-conflict", "wrong-database", "missing-skill"} {
		t.Run(behavior, func(t *testing.T) {
			f := newFixture(t, task("t-1"))
			db := readFake(f.db)
			db.Behavior = behavior
			writeFake(f.db, db)
			if behavior == "missing-skill" {
				f.c.FinishSkill.Path = "not-in-base.md"
			}
			s := f.service(t)
			require(t, s.dispatch(context.Background()))
			r := s.State.Runs[0]
			db = readFake(f.db)
			if r.FinishTaskStatus != "blocked" || r.ExecutionStatus != "failed" || s.State.SlotOccupied || s.worker != nil || db.Claims["t-1"] != 1 || db.Starts["t-1"] != 0 || db.Tasks[0].Assignee != f.c.Claimant {
				t.Fatal("preparation failure lost claim or launched worker")
			}
			if r.WorktreePath != nil || r.BaseCommit != nil {
				t.Fatal("unverified facts published as prepared")
			}
			if behavior == "path-conflict" {
				b, e := os.ReadFile(filepath.Join(r.WorkspaceIntent.Path, "keep.txt"))
				require(t, e)
				if string(b) != "unrelated existing contents" {
					t.Fatal("existing contents overwritten")
				}
			}
			s = f.service(t)
			require(t, s.dispatch(context.Background()))
			if readFake(f.db).Claims["t-1"] != 1 {
				t.Fatal("failed preparation automatically retried")
			}
		})
	}
}

func TestWorkspaceBranchCollisionIsNotReset(t *testing.T) {
	f := newFixture(t, task("t-1"))
	s := f.service(t)
	ctx := context.Background()
	id := newID()
	base, e := fetchBase(ctx, s.Config, id)
	require(t, e)
	intent, e := planWorkspace(s.Config, "t-1", id, base)
	require(t, e)
	fixtureGit(t, f.c.Repository, "branch", intent.Branch, base)
	source, e := s.beads.Context(ctx)
	require(t, e)
	_, e = prepareWorkspace(ctx, s.Config, &Run{WorkspaceIntent: intent, Claimant: f.c.Claimant, TaskID: "t-1"}, source)
	if e == nil || !strings.Contains(e.Error(), "branch already exists") {
		t.Fatalf("branch conflict: %v", e)
	}
	if fixtureGit(t, f.c.Repository, "rev-parse", "refs/heads/"+intent.Branch) != base {
		t.Fatal("existing branch reset")
	}
	if _, e = os.Stat(intent.Path); !os.IsNotExist(e) {
		t.Fatal("created conflicting workspace")
	}
}

func TestInterruptedPreparationNeverCreatesOrRelaunches(t *testing.T) {
	for _, created := range []bool{false, true} {
		t.Run(map[bool]string{false: "before-create", true: "after-create"}[created], func(t *testing.T) {
			f := newFixture(t, task("t-1"))
			s := f.service(t)
			ctx := context.Background()
			id := newID()
			base, e := fetchBase(ctx, s.Config, id)
			require(t, e)
			intent, e := planWorkspace(s.Config, "t-1", id, base)
			require(t, e)
			ok, e := s.beads.Claim(ctx, "t-1")
			require(t, e)
			if !ok {
				t.Fatal("claim failed")
			}
			r := &Run{TaskID: "t-1", RunID: id, Title: "interrupted", Claimant: f.c.Claimant, BeadsStatus: "in_progress", ExecutionStatus: "pending", FinishTaskStatus: "pending", WorkspaceIntent: intent, PR: PRState{State: "unknown"}}
			s.State.Runs = append(s.State.Runs, r)
			require(t, s.save())
			if created {
				source, e := s.beads.Context(ctx)
				require(t, e)
				_, e = prepareWorkspace(ctx, s.Config, r, source)
				require(t, e)
			}
			restarted := f.service(t)
			if restarted.State.Runs[0].ExecutionStatus != "unknown" || restarted.State.Runs[0].FinishTaskStatus != "blocked" {
				t.Fatal("interrupted intent trusted")
			}
			if restarted.dispatch(ctx) == nil {
				t.Fatal("unknown preparation did not pause dispatch")
			}
			if len(readFake(f.db).Starts) != 0 {
				t.Fatal("crash recovery launched worker")
			}
			_, statErr := os.Stat(intent.Path)
			if created && statErr != nil {
				t.Fatal("lost worktree")
			}
			if !created && !os.IsNotExist(statErr) {
				t.Fatal("recovery recreated worktree")
			}
		})
	}
}

func TestWorkspaceRecoveryRejectsChangedBranch(t *testing.T) {
	f := newFixture(t, task("t-1"))
	s := f.service(t)
	require(t, s.dispatch(context.Background()))
	finishWorker(t, s)
	r := s.State.Runs[0]
	fixtureGit(t, *r.WorktreePath, "switch", "--quiet", "-c", "manual-change")
	restarted := f.service(t)
	if restarted.State.Runs[0].FinishTaskStatus != "blocked" {
		t.Fatal("changed workspace trusted on restart")
	}
	require(t, restarted.pollPRs(context.Background()))
	if readFake(f.db).Tasks[0].Status == "closed" {
		t.Fatal("invalid workspace closed task")
	}
}

func TestWorktreeRootConfigurationAndOldSnapshotRejection(t *testing.T) {
	f := newFixture(t)
	configPath := filepath.Join(f.c.RunDir, "config.json")
	data, _ := json.Marshal(f.c)
	require(t, os.WriteFile(configPath, data, 0600))
	loaded, e := LoadConfig(configPath)
	require(t, e)
	if loaded.WorktreeRoot != filepath.Join(f.c.RunDir, "worktrees") {
		t.Fatal("wrong default worktree_root")
	}
	for _, bad := range []string{f.c.Repository, filepath.Join(f.c.Repository, "nested"), f.c.RunDir, filepath.Join(f.c.RunDir, "runs"), filepath.Join(f.c.RunDir, "runs", "nested")} {
		c := f.c
		c.WorktreeRoot = bad
		if c.prepareWorktreeRoot() == nil {
			t.Fatalf("accepted unsafe root %s", bad)
		}
	}
	s := f.service(t)
	s.State.SchemaVersion = 1
	s.State.Runs = []*Run{{TaskID: "old-1", RunID: "legacy", Claimant: f.c.Claimant, ExecutionStatus: "succeeded", FinishTaskStatus: "waiting_pr_merge", PR: PRState{URL: ptr(f.server.URL + "/owner/repo/pulls/1"), State: "open"}}}
	require(t, s.save())
	if _, err := NewService(f.c); err == nil || !strings.Contains(err.Error(), "unsupported runtime schema") {
		t.Fatalf("old snapshot was accepted: %v", err)
	}
}

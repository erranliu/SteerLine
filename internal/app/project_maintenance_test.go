package app

import (
	"context"
	"fmt"
	backend "github.com/erranliu/SteerLine/internal/execution/backend"
	resourceprocess "github.com/erranliu/SteerLine/internal/execution/process"
	pm "github.com/erranliu/SteerLine/internal/projectmaintenance/model"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type pmStore struct {
	maintenanceStore
	approval pm.Approval
	graph    storageports.TaskGraphSnapshot
	contract taskmodel.TaskContract
}

func (s *pmStore) MaterialApproval(context.Context, string, string) (pm.Approval, error) {
	return s.approval, nil
}
func (s *pmStore) TaskGraphSnapshot(context.Context, string) (storageports.TaskGraphSnapshot, error) {
	return s.graph, nil
}
func (s *pmStore) TaskContract(context.Context, string, taskmodel.TaskRecord) (taskmodel.TaskContract, error) {
	return s.contract, nil
}
func pmGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	b, e := c.CombinedOutput()
	if e != nil {
		t.Fatalf("isolated Git %v: %v %s", args, e, b)
	}
	return strings.TrimSpace(string(b))
}
func TestPMRemoteFreshObservedCommitAndRewrite(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "SteerlineTest")
	remote := filepath.Join(root, "origin.git")
	pmGit(t, root, "init", "--bare", "--initial-branch=main", remote)
	pmGit(t, root, "clone", remote, work)
	pmGit(t, work, "config", "user.name", "PM test")
	pmGit(t, work, "config", "user.email", "pm@example.invalid")
	_ = os.WriteFile(filepath.Join(work, "code.txt"), []byte("one"), 0600)
	pmGit(t, work, "add", ".")
	pmGit(t, work, "-c", "commit.gpgsign=false", "commit", "-qm", "one")
	pmGit(t, work, "push", "origin", "main")
	c := Config{Repository: work, Remote: "origin", Branch: "main", Git: backend.Command{Executable: "git"}, ProcessHost: resourceprocess.New(time.Minute)}
	m := newProjectMaintenance(c, &pmStore{})
	ctx := context.Background()
	first, e := m.check(ctx)
	if e != nil || first.Commit == "" {
		t.Fatal(first, e)
	}
	_ = os.WriteFile(filepath.Join(work, "code.txt"), []byte("two"), 0600)
	pmGit(t, work, "add", ".")
	pmGit(t, work, "-c", "commit.gpgsign=false", "commit", "-qm", "two")
	pmGit(t, work, "push", "origin", "main")
	second, e := m.check(ctx)
	if e != nil || second.Commit == first.Commit || second.Rewritten {
		t.Fatal("fresh fetch not used", second, e)
	}
	pmGit(t, remote, "update-ref", "refs/heads/main", first.Commit)
	rewritten, e := m.check(ctx)
	if e != nil || !rewritten.Rewritten {
		t.Fatal("history rewrite hidden", rewritten, e)
	}
	pmGit(t, remote, "update-ref", "-d", "refs/heads/main")
	failed, e := m.check(ctx)
	if e == nil || failed.Diagnostic == "" || failed.Commit != rewritten.Commit || failed.LastSuccess != rewritten.LastSuccess {
		t.Fatal("failure erased observation", failed, e)
	}
	if got := pmGit(t, work, "rev-parse", "HEAD"); got != second.Commit {
		t.Fatal("observer changed local branch")
	}
}
func TestPMTaskContentIdentityIgnoresOnlyCommit(t *testing.T) {
	a := taskmodel.TaskContract{SpecRef: taskmodel.FileReference{Path: "a.spec.md", Commit: "old", SHA256: "same"}, PlanRef: taskmodel.FileReference{Path: "a.plan.md", Commit: "old", SHA256: "same"}}
	b := a
	b.SpecRef.Commit = "new"
	b.PlanRef.Commit = "new"
	b.Digest = "new digest"
	if contentContract(a) != contentContract(b) {
		t.Fatal("commit movement required task re-approval")
	}
	b.Repository = "other"
	if contentContract(a) == contentContract(b) {
		t.Fatal("repository target not included")
	}
}
func TestPMFreezeRequiresApprovalAndResolvesLatestContent(t *testing.T) {
	root := t.TempDir()
	remote := filepath.Join(root, "origin.git")
	work := filepath.Join(root, "SteerlineTest")
	pmGit(t, root, "init", "--bare", "--initial-branch=main", remote)
	pmGit(t, root, "clone", remote, work)
	pmGit(t, work, "config", "user.name", "PM test")
	pmGit(t, work, "config", "user.email", "pm@example.invalid")
	files := map[string]string{"task.spec.md": "# Spec\n## Acceptance\nKeep material identity\n", "task.plan.md": "# Plan\nTest one\n", "task.tests.json": `{"tests":["one"]}`, "docs/testing.md": "# Testing\nRules\n", "docs/testing/test-list.json": `{"tests":[{"id":"one","name":"One","level":"L1","environment":"isolated","command":"test","check":"passes"}]}`}
	for path, b := range files {
		name := filepath.Join(work, filepath.FromSlash(path))
		if e := os.MkdirAll(filepath.Dir(name), 0700); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(name, []byte(b), 0600); e != nil {
			t.Fatal(e)
		}
	}
	pmGit(t, work, "add", ".")
	pmGit(t, work, "-c", "commit.gpgsign=false", "commit", "-qm", "materials")
	pmGit(t, work, "push", "origin", "main")
	old := pmGit(t, work, "rev-parse", "HEAD")
	c := taskmodel.TaskContract{SpecRef: taskmodel.FileReference{Path: "task.spec.md", Commit: old, SHA256: pm.Digest([]byte(files["task.spec.md"]))}, PlanRef: taskmodel.FileReference{Path: "task.plan.md", Commit: old, SHA256: pm.Digest([]byte(files["task.plan.md"]))}}
	c.Digest, _ = taskmodel.ContractDigest(c)
	store := &pmStore{}
	m := newProjectMaintenance(Config{Repository: work, Remote: "origin", Branch: "main", Git: backend.Command{Executable: "git"}, ProcessHost: resourceprocess.New(time.Minute)}, store)
	w := taskmodel.FixedWork{Contract: c}
	calls := 0
	base := func(context.Context, taskmodel.FixedWork, bool) (string, error) {
		calls++
		return pmGit(t, work, "rev-parse", "HEAD"), nil
	}
	if _, _, e := m.prepare(context.Background(), w, false, base); e == nil {
		t.Fatal("missing review passed start")
	}
	if calls != 0 {
		t.Fatal("unreviewed work reached base reservation")
	}
	a, e := m.assessment(context.Background(), "baseline", "task.plan.md", old)
	if e != nil {
		t.Fatal(e)
	}
	i := a.Items[0]
	if i.Status != "needs_review" {
		t.Fatal(i)
	}
	store.approval = pm.Approval{Revision: 1, Inputs: i.Inputs, Output: i.Output}
	if e = os.WriteFile(filepath.Join(work, "code.txt"), []byte("new source only"), 0600); e != nil {
		t.Fatal(e)
	}
	pmGit(t, work, "add", ".")
	pmGit(t, work, "-c", "commit.gpgsign=false", "commit", "-qm", "source only")
	pmGit(t, work, "push", "origin", "main")
	next := pmGit(t, work, "rev-parse", "HEAD")
	got, b, e := m.prepare(context.Background(), w, false, base)
	if e != nil {
		t.Fatal(e)
	}
	if b != next || got.Contract.PlanRef.Commit != next || got.Contract.SpecRef.Commit != next || got.ConfirmedDigest != c.Digest {
		t.Fatal("mixed commit freeze", got, b)
	}
	if e = os.WriteFile(filepath.Join(work, "task.plan.md"), []byte("# New test requirements\n"), 0600); e != nil {
		t.Fatal(e)
	}
	pmGit(t, work, "add", ".")
	pmGit(t, work, "-c", "commit.gpgsign=false", "commit", "-qm", "changed plan")
	pmGit(t, work, "push", "origin", "main")
	if _, _, e = m.prepare(context.Background(), w, false, base); e == nil {
		t.Fatal("changed Plan ran without Task confirmation")
	}
}

func TestPMRemoteSharesInflightObservation(t *testing.T) {
	// Invalid Git configuration makes any accidental second fetch observable.
	m := newProjectMaintenance(Config{}, nil)
	done := make(chan struct{})
	m.checking = done
	result := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			o, e := m.check(context.Background())
			if e == nil && o.Commit != "shared" {
				e = fmt.Errorf("different observation: %+v", o)
			}
			result <- e
		}()
	}
	m.mu.Lock()
	m.observation = pm.Observation{Commit: "shared"}
	close(done)
	m.mu.Unlock()
	for i := 0; i < 2; i++ {
		if e := <-result; e != nil {
			t.Fatal(e)
		}
	}
}

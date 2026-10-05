package host

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	execution "github.com/erranliu/SteerLine/internal/execution"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

var nativeSeedOnce sync.Once

var nativeSeedRepo, nativeSeedRemote, nativeSeedCommit string

func prepareNativeSeed(t *testing.T) {
	t.Helper()
	nativeSeedOnce.Do(func() {
		root, err := sharedTestArtifactsDir()
		require(t, err)
		nativeSeedRepo = filepath.Join(root, "native-seed", "repo")
		nativeSeedRemote = filepath.Join(root, "native-seed", "origin.git")
		require(t, os.MkdirAll(filepath.Dir(nativeSeedRepo), 0700))
		for _, args := range [][]string{{"init", "--quiet", "-b", "main", nativeSeedRepo}, {"init", "--quiet", "--bare", nativeSeedRemote}, {"-C", nativeSeedRepo, "remote", "add", "origin", nativeSeedRemote}} {
			output, err := exec.Command("git", args...).CombinedOutput()
			if err != nil {
				t.Fatalf("seed git: %v: %s", err, output)
			}
		}
		for name, content := range map[string]string{
			"docs/spec/task-core.spec.md":          "# Task Spec\n\n## Acceptance\n\nPass.\n",
			"docs/spec/task-core.plan.md":          "# Task Plan\n",
			"docs/spec/native-task-constraints.md": "# Shared constraints\n",
		} {
			path := filepath.Join(nativeSeedRepo, filepath.FromSlash(name))
			require(t, os.MkdirAll(filepath.Dir(path), 0700))
			require(t, os.WriteFile(path, []byte(content), 0600))
		}
		for _, args := range [][]string{{"-C", nativeSeedRepo, "add", "."}, {"-C", nativeSeedRepo, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "fixture"}} {
			output, err := exec.Command("git", args...).CombinedOutput()
			if err != nil {
				t.Fatalf("seed git: %v: %s", err, output)
			}
		}
		output, err := exec.Command("git", "-C", nativeSeedRepo, "rev-parse", "HEAD").Output()
		require(t, err)
		nativeSeedCommit = strings.TrimSpace(string(output))
	})
}

func nativeTaskFixture(t *testing.T) (execution.Config, string) {
	t.Helper()
	prepareNativeSeed(t)
	t.Setenv("STEERLINE_HOME", t.TempDir())
	repo := filepath.Join(t.TempDir(), "repo")
	remote := filepath.Join(t.TempDir(), "origin.git")
	require(t, os.CopyFS(repo, os.DirFS(nativeSeedRepo)))
	require(t, os.CopyFS(remote, os.DirFS(nativeSeedRemote)))
	output, err := exec.Command("git", "-C", repo, "remote", "set-url", "origin", remote).CombinedOutput()
	if err != nil {
		t.Fatalf("fixture remote: %v: %s", err, output)
	}
	commit := nativeSeedCommit
	c := execution.Config{PublicationPolicy: executionPublicationFixture(), PR: hostPublicationProviderFixture(), ExecutionPolicy: workspacePolicyFixture(), TaskAuthority: "native", WorkflowProtocol: executionmodel.Version, Repository: repo, Remote: "origin", Branch: "main", TaskPoll: "10s", PRPoll: "30s", Heartbeat: "5s", CommandTimeout: "10s", ShutdownGrace: "10s", ExecutableTypes: []string{"task", "bug", "feature", "chore"}, Git: executionbackend.Command{Executable: "git"}}
	setNeutralProfile(t, &c, neutralSettings{Executable: os.Args[0]})
	key, err := fixtureRepositoryStateKey(c)
	if err != nil {
		t.Fatal(err)
	}
	c.RunDir = filepath.Join(fixtureStateRoot(), key)
	c.WorktreeRoot = filepath.Join(c.RunDir, "worktrees")
	c.Git = executionbackend.Command{Executable: os.Args[0], Args: []string{"--fake", "publication-git", "git"}}
	return c, commit
}

func nativeRef(t *testing.T, c execution.Config, commit, name string) taskmodel.FileReference {
	t.Helper()
	blob, err := exec.Command("git", "-C", c.Repository, "cat-file", "blob", commit+":"+name).Output()
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(blob)
	return taskmodel.FileReference{Path: name, Commit: commit, SHA256: hex.EncodeToString(sum[:])}
}

func nativeContract(t *testing.T, c execution.Config, commit, id string) taskmodel.TaskContract {
	t.Helper()
	spec := nativeRef(t, c, commit, "docs/spec/task-core.spec.md")
	plan := nativeRef(t, c, commit, "docs/spec/task-core.plan.md")
	common := nativeRef(t, c, commit, "docs/spec/native-task-constraints.md")
	target, err := c.TaskRepositoryTarget(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	v := taskmodel.TaskContract{ContractID: id, Repository: target, Remote: c.Remote, Branch: c.Branch, SpecRef: spec, PlanRef: plan, CommonRefs: []taskmodel.FileReference{common}, AcceptanceRefs: []taskmodel.FileReference{spec}}
	v.Digest, err = taskmodel.ContractDigest(v)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// Task creation is not the behavior under test in the execution profile case.
func seedExecutionProfileTasks(t *testing.T, c execution.Config, store *sqliteFixture, payload []byte) (taskmodel.TaskBatchReceipt, error) {
	t.Helper()
	var batch taskmodel.TaskBatchInput
	if err := json.Unmarshal(payload, &batch); err != nil {
		return taskmodel.TaskBatchReceipt{}, err
	}
	authority, err := store.NativeAuthority(context.Background(), c.Identity())
	if err != nil {
		return taskmodel.TaskBatchReceipt{}, err
	}
	stamp := now()
	batchID := newID()
	mapping := map[string]string{}
	for _, v := range batch.Tasks {
		mapping[v.LocalKey] = strconvID(v.TaskID)
	}
	err = store.transactRecords(context.Background(), func(tx *fixtureTransaction) error {
		if err := tx.PutTaskBatch(taskmodel.TaskBatchRecord{ProjectIdentity: c.Identity(), BatchID: batchID, SchemaVersion: taskmodel.BatchSchemaVersion, PayloadDigest: digestCanonical(payload), CommandID: "fixture-" + batchID, CreatedAt: stamp, TaskIDs: mapping}, authority.AuthorityVersion, authority.GraphRevision); err != nil {
			return err
		}
		for _, v := range batch.Tasks {
			priority := 2
			if v.Priority != nil {
				priority = *v.Priority
			}
			record := taskmodel.TaskRecord{ProjectIdentity: c.Identity(), TaskID: strconvID(v.TaskID), BatchID: batchID, Title: v.Title, Type: v.Type, Priority: priority, GoalIDs: append([]string(nil), v.GoalIDs...), ContractID: v.Contract.ContractID, Source: "native", Status: "open", Version: 1, CreatedAt: stamp, UpdatedAt: stamp}
			if err := tx.PutTask(record); err != nil {
				return err
			}
			if err := tx.PutTaskContract(c.Identity(), record.TaskID, v.Contract); err != nil {
				return err
			}
		}
		for _, edge := range taskmodel.ResolveBatchEdges(batch) {
			if err := tx.PutTaskDependency(edge); err != nil {
				return err
			}
		}
		return nil
	})
	return taskmodel.TaskBatchReceipt{TaskIDs: mapping, BatchID: batchID, Status: "fixture"}, err
}

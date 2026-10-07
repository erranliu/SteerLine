package host

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	productquery "github.com/erranliu/SteerLine/internal/product/query"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func TestNativeFixtureSeedAndCopiesAreIndependent(t *testing.T) {
	a, initial := nativeTaskFixture(t)
	b, otherInitial := nativeTaskFixture(t)
	if initial != otherInitial || a.Repository == b.Repository || a.RunDir == b.RunDir {
		t.Fatalf("fixture identities or seed commits overlap: %s %s / %s %s", a.Repository, b.Repository, initial, otherInitial)
	}
	git := func(args ...string) string {
		t.Helper()
		output, err := exec.Command("git", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
		return strings.TrimSpace(string(output))
	}
	aRemote := git("-C", a.Repository, "remote", "get-url", "origin")
	bRemote := git("-C", b.Repository, "remote", "get-url", "origin")
	if aRemote == bRemote || aRemote == nativeSeedRemote || bRemote == nativeSeedRemote {
		t.Fatalf("fixture remotes overlap: %s / %s / %s", aRemote, bRemote, nativeSeedRemote)
	}
	file := filepath.Join(a.Repository, "docs", "spec", "task-core.spec.md")
	require(t, os.WriteFile(file, []byte("# changed only in A\n"), 0600))
	git("-C", a.Repository, "add", ".")
	git("-C", a.Repository, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "isolated change")
	newHead := git("-C", a.Repository, "rev-parse", "HEAD")
	if newHead == initial {
		t.Fatal("A did not advance")
	}
	git("-C", a.Repository, "push", "origin", "main")
	for _, repo := range []string{b.Repository, nativeSeedRepo} {
		if head := git("-C", repo, "rev-parse", "HEAD"); head != initial {
			t.Fatalf("%s HEAD changed to %s", repo, head)
		}
		content, err := os.ReadFile(filepath.Join(repo, "docs", "spec", "task-core.spec.md"))
		require(t, err)
		if string(content) == "# changed only in A\n" {
			t.Fatalf("%s contents changed with A", repo)
		}
	}
	if got := git("--git-dir", aRemote, "rev-parse", "refs/heads/main"); got != newHead {
		t.Fatalf("A remote head=%s, want %s", got, newHead)
	}
	for _, remote := range []string{bRemote, nativeSeedRemote} {
		if output, err := exec.Command("git", "--git-dir", remote, "show-ref", "--verify", "refs/heads/main").CombinedOutput(); err == nil {
			t.Fatalf("%s received A push: %s", remote, output)
		}
	}
	require(t, os.RemoveAll(a.Repository))
	if head := git("-C", b.Repository, "rev-parse", "HEAD"); head != initial {
		t.Fatalf("B broke after A cleanup: %s", head)
	}
}

func TestNativePreflightAllowsItsEmptyRunLockFile(t *testing.T) {
	c, _ := nativeTaskFixture(t)
	store, err := openHostFixtureNativeProject(context.Background(), &c)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(c.RunDir, "steerline.lock")
	if err = os.WriteFile(lockPath, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err = PreflightNativeStatePath(hostConfigForTest(c)); err != nil {
		t.Fatalf("native preflight rejected its own empty OS lock file: %v", err)
	}
	if err = os.WriteFile(lockPath, []byte("unknown"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = PreflightNativeStatePath(hostConfigForTest(c)); err == nil || !strings.Contains(err.Error(), "native service lock path is not an empty regular file") {
		t.Fatalf("native preflight accepted a nonempty lock path: %v", err)
	}
}

func TestNativeTaskQuerySnapshotPagingPersistenceAndIsolation(t *testing.T) {
	ctx := context.Background()
	c, commit := nativeTaskFixture(t)
	reader, err := openHostFixtureNativeProject(ctx, &c)
	if err != nil {
		t.Fatal(err)
	}
	if err = reader.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(c.RunDir, "steerline.db")
	store, err := openSQLiteFixture(path)
	if err != nil {
		t.Fatal(err)
	}
	project := c.Identity()
	contract2 := nativeContract(t, c, commit, "contract-2")
	contract10 := nativeContract(t, c, commit, "contract-10")
	foreignContract := nativeContract(t, c, commit, "foreign-contract")
	foreignContract.Repository = "https://example.invalid/Other/Repo.git"
	foreignContract.Digest, err = taskmodel.ContractDigest(foreignContract)
	if err != nil {
		t.Fatal(err)
	}
	batchDigest := strings.Repeat("a", 64)
	created := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
	staleBatch := taskmodel.TaskBatchRecord{BatchID: "stale-batch", ProjectIdentity: project, SchemaVersion: "task-batch/v1", PayloadDigest: strings.Repeat("f", 64), CreatedAt: created, CommandID: "stale-command", TaskIDs: map[string]string{"stale": "99"}}
	if err = store.transactRecords(ctx, func(tx *fixtureTransaction) error { return tx.PutTaskBatch(staleBatch, 2, 0) }); err == nil || !strings.Contains(err.Error(), "stale_version") {
		t.Fatalf("stale project authority created a batch: %v", err)
	}
	var staleCount int
	if err = store.db.QueryRow(`SELECT COUNT(*) FROM task_batches WHERE project_identity=? AND batch_id=?`, project, "stale-batch").Scan(&staleCount); err != nil || staleCount != 0 {
		t.Fatalf("stale batch transaction left partial state: count=%d err=%v", staleCount, err)
	}
	err = store.transactRecords(ctx, func(tx *fixtureTransaction) error {
		if err := tx.PutTaskBatch(taskmodel.TaskBatchRecord{BatchID: "batch-1", ProjectIdentity: project, SchemaVersion: "task-batch/v1", PayloadDigest: batchDigest, CreatedAt: created, CommandID: "command-1", TaskIDs: map[string]string{"two": "2", "ten": "10"}}, 1, 0); err != nil {
			return err
		}
		for _, entry := range []struct {
			id       string
			contract taskmodel.TaskContract
			priority int
		}{{"2", contract2, 1}, {"10", contract10, 2}} {
			record := taskmodel.TaskRecord{ProjectIdentity: project, TaskID: entry.id, BatchID: "batch-1", Title: "task " + entry.id, Type: "task", Priority: entry.priority, ContractID: entry.contract.ContractID, Source: "native", Status: "open", Version: 1, CreatedAt: created, UpdatedAt: created}
			if err := tx.PutTask(record); err != nil {
				return err
			}
			if err := tx.PutTaskContract(project, entry.id, entry.contract); err != nil {
				return err
			}
		}
		foreignProject := "other-project"
		otherAuthority := executionmodel.ProjectTaskAuthority{ProjectIdentity: foreignProject, Authority: "native", AuthorityVersion: 1, Repository: foreignContract.Repository, Remote: "origin", Branch: "main", DispatchMode: "manual", DispatchPaused: true, CreatedAt: created, UpdatedAt: created}
		if err := tx.PutProjectTaskAuthority(otherAuthority); err != nil {
			return err
		}
		if err := tx.PutTaskBatch(taskmodel.TaskBatchRecord{BatchID: "other-batch", ProjectIdentity: foreignProject, SchemaVersion: "task-batch/v1", PayloadDigest: strings.Repeat("e", 64), CreatedAt: created, CommandID: "other-command", TaskIDs: map[string]string{"other": "2"}}, 1, 0); err != nil {
			return err
		}
		foreign := taskmodel.TaskRecord{ProjectIdentity: foreignProject, TaskID: "2", BatchID: "other-batch", Title: "foreign", Type: "task", ContractID: "foreign-contract", Source: "native", Status: "open", Version: 1, CreatedAt: created, UpdatedAt: created}
		if err := tx.PutTask(foreign); err != nil {
			return err
		}
		if err := tx.PutTaskContract(foreignProject, "2", foreignContract); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	replayedBatch := taskmodel.TaskBatchRecord{BatchID: "batch-1", ProjectIdentity: project, SchemaVersion: "task-batch/v1", PayloadDigest: batchDigest, CreatedAt: created, CommandID: "command-1", TaskIDs: map[string]string{"two": "2", "ten": "10"}}
	if err = store.transactRecords(ctx, func(tx *fixtureTransaction) error { return tx.PutTaskBatch(replayedBatch, 1, 0) }); err != nil {
		t.Fatalf("identical batch retry was not idempotent: %v", err)
	}
	query := productquery.NewService(store, project)
	first, err := query.ListNativeTasks(ctx, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if first.Protocol != "native-tasks/v1" || first.ProjectID != project || first.AuthorityVersion != 1 || first.GraphRevision != 1 || first.DatabaseRevision == 0 || first.SampledAt.IsZero() || len(first.Tasks) != 1 || first.Tasks[0].TaskID != "2" || !first.HasMore || first.NextCursor == "" {
		t.Fatalf("bad first page: %+v", first)
	}
	if first.Capabilities.DependencyReadiness != "provided" || first.Capabilities.DependencyBlockers != "provided" {
		t.Fatalf("DAG query capability was not provided: %+v", first.Capabilities)
	}
	second, err := query.ListNativeTasks(ctx, first.NextCursor, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Tasks) != 1 || second.Tasks[0].TaskID != "10" || second.ProjectID != project {
		t.Fatalf("numeric ordering or project isolation failed: %+v", second)
	}
	show, err := query.ShowNativeTask(ctx, "2")
	if err != nil {
		t.Fatal(err)
	}
	if len(show.Tasks) != 1 || show.Tasks[0].Contract.ContractID != "contract-2" || show.Tasks[0].Status != "open" {
		t.Fatalf("bad task detail: %+v", show)
	}
	duplicate := taskmodel.TaskRecord{ProjectIdentity: project, TaskID: "2", BatchID: "batch-1", Title: "replacement", Type: "task", ContractID: "contract-2", Source: "native", Status: "open", Version: 1, CreatedAt: created, UpdatedAt: created}
	if err = store.transactRecords(ctx, func(tx *fixtureTransaction) error { return tx.PutTask(duplicate) }); err == nil {
		t.Fatal("duplicate numeric task ID was accepted")
	}
	// Any committed center write changes the database revision and invalidates a continuation cursor.
	if err = store.transactRecords(ctx, func(tx *fixtureTransaction) error {
		_, err := tx.AdvanceTaskGraphRevision(project, first.AuthorityVersion, first.GraphRevision)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = query.ListNativeTasks(ctx, first.NextCursor, 1); err == nil || !strings.Contains(err.Error(), "cursor_invalidated") {
		t.Fatalf("stale page cursor was accepted: %v", err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = openSQLiteReaderFixture(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	restarted, err := (productquery.NewService(store, project)).ShowNativeTask(ctx, "10")
	if err != nil {
		t.Fatal(err)
	}
	if len(restarted.Tasks) != 1 || restarted.Tasks[0].Title != "task 10" {
		t.Fatalf("task projection did not survive restart: %+v", restarted)
	}
}

func TestNativeTaskCASGuardsAndDeliveryProjection(t *testing.T) {
	ctx := context.Background()
	c, commit := nativeTaskFixture(t)
	r, err := openHostFixtureNativeProject(ctx, &c)
	if err != nil {
		t.Fatal(err)
	}
	_ = r.Close()
	store, err := openSQLiteFixture(filepath.Join(c.RunDir, "steerline.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	project := c.Identity()
	contract := nativeContract(t, c, commit, "contract-1")
	created := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
	task := taskmodel.TaskRecord{ProjectIdentity: project, TaskID: "7", BatchID: "batch-7", Title: "task", Type: "task", Priority: 2, ContractID: contract.ContractID, Source: "native", Status: "open", Version: 1, CreatedAt: created, UpdatedAt: created}
	err = store.transactRecords(ctx, func(tx *fixtureTransaction) error {
		if err := tx.PutTaskBatch(taskmodel.TaskBatchRecord{BatchID: "batch-7", ProjectIdentity: project, SchemaVersion: "task-batch/v1", PayloadDigest: strings.Repeat("b", 64), CreatedAt: created, CommandID: "cmd-7", TaskIDs: map[string]string{"seven": "7"}}, 1, 0); err != nil {
			return err
		}
		if err := tx.PutTask(task); err != nil {
			return err
		}
		return tx.PutTaskContract(project, "7", contract)
	})
	if err != nil {
		t.Fatal(err)
	}
	claimed := task
	claimed.Status = "in_progress"
	claimed.Claimant = "agent"
	claimed.Version = 2
	claimed.UpdatedAt = created.Add(time.Minute)
	if err = store.transactRecords(ctx, func(tx *fixtureTransaction) error { return tx.UpdateNativeTaskCAS(claimed, 1) }); err != nil {
		t.Fatal(err)
	}
	if err = store.transactRecords(ctx, func(tx *fixtureTransaction) error { return tx.UpdateNativeTaskCAS(claimed, 1) }); err == nil || !strings.Contains(err.Error(), "stale_version") {
		t.Fatalf("stale task update was accepted: %v", err)
	}
	closed := claimed
	closed.Status = "closed"
	closed.Version = 3
	closed.UpdatedAt = created.Add(2 * time.Minute)
	if err = store.transactRecords(ctx, func(tx *fixtureTransaction) error { return tx.UpdateNativeTaskCAS(closed, 2) }); err == nil {
		t.Fatal("unverified delivery closed a task")
	}
	run := executionmodel.WorkflowRunRecord{ProjectIdentity: project, WorkflowRunID: "wf-7", TaskID: "7", WorkflowVersion: executionmodel.Version, Phase: "waiting_pr_merge", SelectedResult: "exec-7", PRURL: "https://forge.invalid/org/repo/pulls/4", CreatedAt: created, UpdatedAt: created}
	execution := executionmodel.ExecutionRecord{ProjectIdentity: project, ExecutionID: "exec-7", CurrentAttemptID: "attempt-7", WorkflowRunID: "wf-7", Backend: string(executionbackend.BackendLocal), Protocol: string(neutralProtocol), Agent: "fixture", Workspace: executionmodel.WorkspaceEvidence{Branch: "fixture/task-7"}, Status: "succeeded", CurrentStep: "waiting_pr_merge", CreatedAt: created, UpdatedAt: created}
	setNeutralBinding(t, &execution, "ws://127.0.0.1:12345", "native-generation", workerFlowMockSession)
	err = store.transactRecords(ctx, func(tx *fixtureTransaction) error {
		if err := tx.PutWorkflowRun(run); err != nil {
			return err
		}
		return tx.PutExecution(execution)
	})
	if err != nil {
		t.Fatal(err)
	}
	delivery := executionmodel.TaskDeliveryRecord{ProjectIdentity: project, TaskID: "7", SelectedWorkflowRunID: "wf-7", PRInstance: "https://forge.invalid", PRRepository: "org/repo", PRNumber: 4, PRURL: "https://forge.invalid/org/repo/pulls/4", MergeCommit: strings.Repeat("c", 40), MergedAt: created.Add(3 * time.Minute), RecordedAt: created.Add(4 * time.Minute), Evidence: json.RawMessage(`{"merged":true}`)}
	delivery.Evidence, err = json.Marshal(executionmodel.TaskDeliveryEvidence{PRInstance: delivery.PRInstance, URL: delivery.PRURL, Repository: delivery.PRRepository, PRNumber: delivery.PRNumber, BaseBranch: "main", State: "merged", Merged: true, MergedAt: delivery.MergedAt, MergeCommit: delivery.MergeCommit})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.transactRecords(ctx, func(tx *fixtureTransaction) error {
		if err := tx.PutTaskDelivery(delivery); err != nil {
			return err
		}
		return tx.UpdateNativeTaskCAS(closed, 2)
	}); err != nil {
		t.Fatal(err)
	}
	page, err := (productquery.NewService(store, project)).ShowNativeTask(ctx, "7")
	if err != nil {
		t.Fatal(err)
	}
	if page.Tasks[0].Status != "closed" || page.Tasks[0].Delivery == nil || page.Tasks[0].Delivery.MergeCommit != delivery.MergeCommit || len(page.Tasks[0].WorkflowRuns) != 1 {
		t.Fatalf("delivery/run association did not project: %+v", page.Tasks[0])
	}
	if err = store.transactRecords(ctx, func(tx *fixtureTransaction) error { return tx.PutTaskDelivery(delivery) }); err != nil {
		t.Fatalf("same verified delivery was not idempotent after restart boundary: %v", err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := openSQLiteReaderFixture(filepath.Join(c.RunDir, "steerline.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	recovered, err := (productquery.NewService(restarted, project)).ShowNativeTask(ctx, "7")
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Tasks[0].Delivery == nil || recovered.Tasks[0].WorkflowRuns[0].SelectedResult != "exec-7" || recovered.Tasks[0].WorkflowRuns[0].Phase != "completed" || recovered.Tasks[0].Status != "closed" || recovered.GraphRevision != 2 {
		t.Fatalf("task/run/delivery projection did not survive restart: %+v", recovered.Tasks[0])
	}
}

func TestNativeDiscardCASKeepsAuditAndNeverReusesID(t *testing.T) {
	ctx := context.Background()
	c, commit := nativeTaskFixture(t)
	reader, err := openHostFixtureNativeProject(ctx, &c)
	if err != nil {
		t.Fatal(err)
	}
	_ = reader.Close()
	store, err := openSQLiteFixture(filepath.Join(c.RunDir, "steerline.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	project := c.Identity()
	contract := nativeContract(t, c, commit, "contract-4")
	created := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
	task := taskmodel.TaskRecord{ProjectIdentity: project, TaskID: "4", BatchID: "batch-4", Title: "discard me", Type: "task", Priority: 2, ContractID: contract.ContractID, Source: "native", Status: "open", Version: 1, CreatedAt: created, UpdatedAt: created}
	if err = store.transactRecords(ctx, func(tx *fixtureTransaction) error {
		if err := tx.PutTaskBatch(taskmodel.TaskBatchRecord{BatchID: "batch-4", ProjectIdentity: project, SchemaVersion: "task-batch/v1", PayloadDigest: strings.Repeat("4", 64), CreatedAt: created, CommandID: "cmd-4", TaskIDs: map[string]string{"four": "4"}}, 1, 0); err != nil {
			return err
		}
		if err := tx.PutTask(task); err != nil {
			return err
		}
		return tx.PutTaskContract(project, "4", contract)
	}); err != nil {
		t.Fatal(err)
	}
	discarded := task
	discarded.Status = "discarded"
	discarded.Version = 2
	discarded.DiscardedAt = &created
	discarded.DiscardReason = "superseded by explicit follow-up"
	discarded.DiscardedBy = "fixture"
	discarded.DiscardRequestID = strings.Repeat("a", 32)
	if err = store.transactRecords(ctx, func(tx *fixtureTransaction) error { return tx.UpdateNativeTaskCAS(discarded, 1) }); err != nil {
		t.Fatal(err)
	}
	reopened := discarded
	reopened.Status = "open"
	reopened.Version = 3
	reopened.DiscardedAt = nil
	reopened.DiscardReason = ""
	reopened.DiscardedBy = ""
	reopened.DiscardRequestID = ""
	reopened.StopPending = nil
	if err = store.transactRecords(ctx, func(tx *fixtureTransaction) error { return tx.UpdateNativeTaskCAS(reopened, 2) }); err == nil || !strings.Contains(err.Error(), "stale_version") {
		t.Fatalf("discarded task was reopened: %v", err)
	}
	replacement := task
	replacement.Title = "new identity"
	if err = store.transactRecords(ctx, func(tx *fixtureTransaction) error { return tx.PutTask(replacement) }); err == nil {
		t.Fatal("discarded task ID was reused")
	}
	page, err := (productquery.NewService(store, project)).ShowNativeTask(ctx, "4")
	if err != nil {
		t.Fatal(err)
	}
	if page.Tasks[0].Status != "discarded" || page.Tasks[0].DiscardedAt == nil || page.Tasks[0].DiscardReason != discarded.DiscardReason || page.Tasks[0].Delivery != nil || page.GraphRevision != 2 {
		t.Fatalf("discard audit was not retained: %+v", page.Tasks[0])
	}
}

func TestTaskContractPinsCommittedBlobsAndRejectsInvalidReferences(t *testing.T) {
	ctx := context.Background()
	c, commit := nativeTaskFixture(t)
	contract := nativeContract(t, c, commit, "contract")
	if err := fullFixtureTaskService(nil, c).VerifyContractFiles(ctx, contract); err != nil {
		t.Fatal(err)
	}
	// Checkout newline conversion does not affect the pinned Git blob bytes.
	specPath := filepath.Join(c.Repository, filepath.FromSlash(contract.SpecRef.Path))
	raw, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(specPath, []byte(strings.ReplaceAll(string(raw), "\n", "\r\n")), 0600); err != nil {
		t.Fatal(err)
	}
	if err = fullFixtureTaskService(nil, c).VerifyContractFiles(ctx, contract); err != nil {
		t.Fatalf("CRLF checkout changed committed-blob verification: %v", err)
	}
	for _, bad := range []string{"../outside.spec.md", "/absolute.spec.md", "C:/outside.spec.md", "docs\\spec\\bad.spec.md"} {
		v := contract
		v.SpecRef.Path = bad
		v.Digest, _ = taskmodel.ContractDigest(v)
		if err = taskmodel.ValidateContract(v); err == nil {
			t.Fatalf("invalid contract path accepted: %q", bad)
		}
	}
	missing := contract
	missing.SpecRef.Path = "docs/spec/missing-task.spec.md"
	missing.PlanRef.Path = "docs/spec/missing-task.plan.md"
	missing.Digest, _ = taskmodel.ContractDigest(missing)
	if err = fullFixtureTaskService(nil, c).VerifyContractFiles(ctx, missing); err == nil || !strings.Contains(err.Error(), "contract_mismatch") {
		t.Fatalf("missing committed contract files were accepted: %v", err)
	}
	v := contract
	v.CommonRefs = []taskmodel.FileReference{contract.SpecRef}
	v.Digest, _ = taskmodel.ContractDigest(v)
	if err = taskmodel.ValidateContract(v); err == nil {
		t.Fatal("task-specific spec was accepted as a common reference")
	}
	v = contract
	v.PlanRef = taskmodel.FileReference{}
	v.Digest, _ = taskmodel.ContractDigest(v)
	if err = taskmodel.ValidateContract(v); err == nil {
		t.Fatal("missing plan was accepted")
	}
	v = contract
	v.AcceptanceRefs = []taskmodel.FileReference{contract.PlanRef}
	v.Digest, _ = taskmodel.ContractDigest(v)
	if err = fullFixtureTaskService(nil, c).VerifyContractFiles(ctx, v); err == nil || !strings.Contains(err.Error(), "acceptance heading") {
		t.Fatalf("acceptance reference without its section was accepted: %v", err)
	}
	if _, err = taskmodel.ValidateContractJSON([]byte(`{"contract_id":"one","contract_id":"two"}`)); err == nil || !strings.Contains(err.Error(), "duplicate JSON object key") {
		t.Fatalf("duplicate JSON key was accepted: %v", err)
	}
	// The digest is over canonical JSON and therefore independent of object key order.
	canonicalA, err := taskmodel.CanonicalJSON([]byte(`{"b":2,"a":{"d":4,"c":3}}`))
	if err != nil {
		t.Fatal(err)
	}
	canonicalB, err := taskmodel.CanonicalJSON([]byte(` { "a" : {"c":3,"d":4},"b":2 } `))
	if err != nil {
		t.Fatal(err)
	}
	if string(canonicalA) != string(canonicalB) {
		t.Fatalf("canonical JSON differs: %s / %s", canonicalA, canonicalB)
	}
}

func TestTaskContractRejectsCommittedSymlinkReferences(t *testing.T) {
	ctx := context.Background()
	c, _ := nativeTaskFixture(t)
	planPath := filepath.Join(c.Repository, "docs", "spec", "linked.plan.md")
	if err := os.WriteFile(planPath, []byte("# Linked Plan\n"), 0600); err != nil {
		t.Fatal(err)
	}
	hash := exec.Command("git", "-C", c.Repository, "hash-object", "-w", "--stdin")
	hash.Stdin = strings.NewReader("task-core.spec.md")
	blob, err := hash.Output()
	if err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", c.Repository, "update-index", "--add", "--cacheinfo", "120000", strings.TrimSpace(string(blob)), "docs/spec/linked.spec.md").CombinedOutput(); err != nil {
		t.Fatalf("stage symlink blob: %v: %s", err, out)
	}
	if out, err := exec.Command("git", "-C", c.Repository, "add", "--", "docs/spec/linked.plan.md").CombinedOutput(); err != nil {
		t.Fatalf("stage linked plan: %v: %s", err, out)
	}
	args := []string{"-C", c.Repository, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "add symlink reference"}
	if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
		t.Fatalf("commit symlink fixture: %v: %s", err, out)
	}
	head, err := exec.Command("git", "-C", c.Repository, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	contract := nativeContract(t, c, strings.TrimSpace(string(head)), "contract-link")
	contract.SpecRef = nativeRef(t, c, contract.SpecRef.Commit, "docs/spec/linked.spec.md")
	contract.PlanRef = nativeRef(t, c, contract.PlanRef.Commit, "docs/spec/linked.plan.md")
	contract.AcceptanceRefs = []taskmodel.FileReference{contract.SpecRef}
	contract.Digest, err = taskmodel.ContractDigest(contract)
	if err != nil {
		t.Fatal(err)
	}
	if err = fullFixtureTaskService(nil, c).VerifyContractFiles(ctx, contract); err == nil || !strings.Contains(err.Error(), "regular committed files") {
		t.Fatalf("committed symlink contract path was accepted: %v", err)
	}
}

func TestNativeInitializationRejectsBeadsStateAndCustomDatabaseFork(t *testing.T) {
	ctx := context.Background()
	c, _ := nativeTaskFixture(t)
	if err := os.Mkdir(filepath.Join(c.Repository, ".beads"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := openHostFixtureNativeProject(ctx, &c); err == nil || !strings.Contains(err.Error(), "authority_mismatch") {
		t.Fatalf("Beads state was adopted: %v", err)
	}
	if err := os.Remove(filepath.Join(c.Repository, ".beads")); err != nil {
		t.Fatal(err)
	}
	custom := c
	custom.RunDir = filepath.Join(t.TempDir(), "custom")
	if err := validateNativeStateLocation(custom); err == nil {
		t.Fatal("custom state directory could fork a second task authority")
	}
}

func TestNativeInitializationRefusesExistingBeadsTaskDatabase(t *testing.T) {
	ctx := context.Background()
	c, _ := nativeTaskFixture(t)
	if err := os.MkdirAll(c.RunDir, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := openSQLiteFixture(filepath.Join(c.RunDir, "steerline.db"))
	if err != nil {
		t.Fatal(err)
	}
	old := taskmodel.TaskRecord{ProjectIdentity: "legacy-project", TaskID: "legacy:one", Source: "beads", Status: "open", Version: 1, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err = store.transactRecords(ctx, func(tx *fixtureTransaction) error { return tx.PutTask(old) }); err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = openHostFixtureNativeProject(ctx, &c); err == nil || !strings.Contains(err.Error(), "authority_mismatch") {
		t.Fatalf("native config adopted a database without matching authority: %v", err)
	}
	reader, err := openSQLiteReaderFixture(filepath.Join(c.RunDir, "steerline.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	var count int
	if err = reader.db.QueryRow(`SELECT COUNT(*) FROM project_task_authorities`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("failed native init wrote project authority: count=%d", count)
	}
}

func TestNativeStoreRejectsRetiredSQLiteRunProjectionBeforeAuthorityOpen(t *testing.T) {
	ctx := context.Background()
	c, _ := nativeTaskFixture(t)
	if err := resolveHostTestIdentity(ctx, &c); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(c.RunDir, "steerline.db")
	store, err := openSQLiteFixture(path)
	if err != nil {
		t.Fatal(err)
	}
	legacyResult := filepath.Join(t.TempDir(), "result.json")
	payload, err := json.Marshal(reconstructiontypes.Runtime{SchemaVersion: 5, Repository: reconstructiontypes.Repository{Identity: c.Identity()}, Runs: []*reconstructiontypes.RunView{{TaskID: "1", RunID: "0123456789abcdef0123456789abcdef", WorkflowVersion: "minimal-v1", ResultPath: &legacyResult}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.db.Exec(`INSERT INTO projection(project_identity,revision,payload,updated_at) VALUES(?,?,?,?)`, c.Identity(), 1, payload, now().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = openBoundNativeStore(ctx, c, path); !isNativeErrorCode(err, "unsupported_schema") {
		t.Fatalf("SQLite projection with a retired result binding was opened: %v", err)
	}
	reader, err := openSQLiteReaderFixture(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	saved, _, err := reader.LoadProjection(ctx, c.Identity())
	if err != nil || !bytes.Equal(saved, payload) {
		t.Fatalf("rejection changed the saved runtime projection: err=%v", err)
	}
	var authorities int
	if err = reader.db.QueryRow(`SELECT COUNT(*) FROM project_task_authorities`).Scan(&authorities); err != nil || authorities != 0 {
		t.Fatalf("projection refusal wrote a replacement task authority: count=%d err=%v", authorities, err)
	}
}

func TestNativeCursorErrorHasStableCode(t *testing.T) {
	var native *HostError
	if !errors.As(nativeErr("cursor_invalidated", "expired"), &native) || native.Code != "cursor_invalidated" {
		t.Fatalf("native error code missing: %+v", native)
	}
}

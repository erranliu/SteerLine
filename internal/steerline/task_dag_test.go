package steerline

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func taskBatchFixture(t *testing.T, c Config, commit string, tasks []TaskBatchTask, edges []TaskBatchEdge) []byte {
	t.Helper()
	if edges == nil {
		edges = []TaskBatchEdge{}
	}
	for i := range tasks {
		if tasks[i].Contract.ContractID == "" {
			tasks[i].Contract = nativeContract(t, c, commit, "batch-contract-"+strconvID(tasks[i].TaskID))
		}
	}
	b, err := json.Marshal(TaskBatchInput{SchemaVersion: taskBatchSchemaVersion, ProjectID: c.Identity(), Tasks: tasks, Dependencies: edges})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func strconvID(v int64) string { return strconv.FormatInt(v, 10) }

func openDAGFixture(t *testing.T) (Config, string, *SQLiteStore) {
	t.Helper()
	c, commit := nativeTaskFixture(t)
	reader, err := OpenNativeTaskProject(context.Background(), &c)
	if err != nil {
		t.Fatal(err)
	}
	if err = reader.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := OpenSQLiteStore(filepath.Join(c.RunDir, "steerline.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return c, commit, store
}

func batchTasks(ids ...int64) []TaskBatchTask {
	out := make([]TaskBatchTask, len(ids))
	for i, id := range ids {
		priority := 2
		out[i] = TaskBatchTask{LocalKey: "k" + strconvID(id), TaskID: id, Title: "Task " + strconvID(id), Type: "task", Priority: &priority}
	}
	return out
}

func TestTaskBatchStrictDecodeAndDeterministicCyclePath(t *testing.T) {
	for _, raw := range []string{
		`{"schema_version":"task-batch/v1","project_id":"p","tasks":[],"unknown":true}`,
		`{"schema_version":"task-batch/v1","project_id":"p","tasks":[{"local_key":"a","title":"x","type":"task","contract":{}}]}`,
		`{"schema_version":"task-batch/v1","project_id":"p","tasks":[{"local_key":"a","task_id":1.5,"title":"x","type":"task","contract":{}}]}`,
		`{"schema_version":"task-batch/v1","project_id":"p","tasks":[],"tasks":[]}`,
	} {
		if _, _, err := decodeTaskBatch([]byte(raw)); err == nil {
			t.Fatalf("invalid batch accepted: %s", raw)
		}
	}
	c, commit, store := openDAGFixture(t)
	tasks := batchTasks(2, 3)
	edges := []TaskBatchEdge{{Predecessor: TaskBatchReference{LocalKey: "k2"}, Successor: TaskBatchReference{LocalKey: "k3"}}, {Predecessor: TaskBatchReference{LocalKey: "k3"}, Successor: TaskBatchReference{LocalKey: "k2"}}}
	beforeGraph, err := store.taskGraphSnapshot(context.Background(), c.Identity())
	require(t, err)
	payload := taskBatchFixture(t, c, commit, tasks, edges)
	validation, err := (CommandService{Store: store, Project: c.Identity()}).ValidateTaskBatch(context.Background(), c, payload)
	if err != nil {
		t.Fatal(err)
	}
	if validation.Valid || len(validation.Cycle) != 3 || validation.Cycle[0] != "2" || validation.Cycle[1] != "3" || validation.Cycle[2] != "2" {
		t.Fatalf("cycle report was not stable: %+v", validation)
	}
	if len(validation.Issues) == 0 || validation.Issues[len(validation.Issues)-1].Code != "dependency_cycle" {
		t.Fatalf("cycle issue missing: %+v", validation.Issues)
	}
	afterGraph, err := store.taskGraphSnapshot(context.Background(), c.Identity())
	require(t, err)
	if beforeGraph.DatabaseRevision != afterGraph.DatabaseRevision || beforeGraph.Authority.GraphRevision != afterGraph.Authority.GraphRevision || len(beforeGraph.Tasks) != len(afterGraph.Tasks) || len(beforeGraph.Edges) != len(afterGraph.Edges) {
		t.Fatal("rejected cycle validation mutated center state")
	}
}

func TestCreateTaskBatchAtomicIdempotentAndQueryable(t *testing.T) {
	ctx := context.Background()
	c, commit, store := openDAGFixture(t)
	tasks := batchTasks(20, 21, 24)
	tasks[2].Priority = func() *int { v := 0; return &v }()
	edges := []TaskBatchEdge{
		{Predecessor: TaskBatchReference{LocalKey: "k20"}, Successor: TaskBatchReference{LocalKey: "k21"}},
	}
	payload := taskBatchFixture(t, c, commit, tasks, edges)
	commands := CommandService{Store: store, Project: c.Identity()}
	validation, err := commands.ValidateTaskBatch(ctx, c, payload)
	if err != nil || !validation.Valid {
		t.Fatalf("batch should validate: %+v / %v", validation, err)
	}
	requestID := strings.Repeat("1", 32)
	receipt, err := commands.CreateTaskBatch(ctx, c, payload, requestID, validation.AuthorityVersion, "planner")
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Status != "accepted" || receipt.GraphRevision != 1 || receipt.TaskIDs["k20"] != "20" || receipt.TaskIDs["k24"] != "24" || receipt.DispatchNote == "" {
		t.Fatalf("unexpected creation receipt: %+v", receipt)
	}
	replayed, err := commands.CreateTaskBatch(ctx, c, payload, requestID, validation.AuthorityVersion+99, "planner")
	if err != nil || replayed.BatchID != receipt.BatchID || replayed.GraphRevision != receipt.GraphRevision {
		t.Fatalf("same request did not return original receipt: %+v / %v", replayed, err)
	}
	changed := append([]byte(nil), payload...)
	changed = []byte(strings.Replace(string(changed), "Task 20", "different title", 1))
	if _, err = commands.CreateTaskBatch(ctx, c, changed, requestID, validation.AuthorityVersion, "planner"); err == nil || !strings.Contains(err.Error(), "idempotency_conflict") {
		t.Fatalf("different content reused request ID: %v", err)
	}
	query := QueryService{Store: store, Project: c.Identity()}
	page, err := query.ListNativeTasks(ctx, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Tasks) != 3 || page.GraphRevision != 1 || page.Capabilities.DependencyReadiness != "provided" {
		t.Fatalf("new graph is absent from task read model: %+v", page)
	}
	ready := map[string]bool{}
	for _, task := range page.Tasks {
		ready[task.TaskID] = task.DependencyReady
		if task.TaskID == "24" && !hasString(task.StartupBlockers, "dispatch_paused") {
			t.Fatalf("startup blockers were not separated from dependency readiness: %+v", task.StartupBlockers)
		}
	}
	if !ready["20"] || !ready["24"] || ready["21"] {
		t.Fatalf("AND readiness is wrong: %v", ready)
	}
	candidates, err := query.ReadyNativeTasks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 2 || candidates[0].TaskID != "24" || candidates[1].TaskID != "20" {
		t.Fatalf("ready candidates were not stably prioritized: %+v", candidates)
	}
	graph, err := query.TaskGraph(ctx, "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Nodes) != 2 || len(graph.Edges) != 1 || graph.NextCursor == "" {
		t.Fatalf("graph relation page is incomplete: %+v", graph)
	}
	if _, err = query.TaskGraph(ctx, graph.NextCursor, 2); err != nil {
		t.Fatal(err)
	}
	// A malformed ID collision is rejected before any batch/task/edge write.
	duplicate := taskBatchFixture(t, c, commit, batchTasks(20), nil)
	bad, err := commands.ValidateTaskBatch(ctx, c, duplicate)
	if err != nil || bad.Valid {
		t.Fatalf("already used task ID passed validation: %+v / %v", bad, err)
	}
	var count int
	if err = store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM task_batches`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("invalid validation wrote partial batch: count=%d err=%v", count, err)
	}
	rootPage, err := query.ShowNativeTask(ctx, "20")
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*TaskRecord){
		func(task *TaskRecord) { task.GoalIDs = []string{"unapproved-goal"} },
		func(task *TaskRecord) { task.IdentityMapDigest = "unapproved-digest" },
	} {
		task := rootPage.Tasks[0].TaskRecord
		task.Status, task.Claimant, task.Version = "in_progress", "worker", task.Version+1
		mutate(&task)
		if err = store.Transact(ctx, func(tx DomainTx) error { return tx.UpdateNativeTaskCAS(task, rootPage.Tasks[0].Version) }); err == nil || !strings.Contains(err.Error(), "contract_mismatch") {
			t.Fatalf("native status update changed fixed task fields: %v", err)
		}
	}
	discard, err := commands.DiscardNativeTask(ctx, c, "20", strings.Repeat("2", 32), rootPage.Tasks[0].Version, "root abandoned", "operator")
	if err != nil || discard.Status != "discarded" {
		t.Fatalf("root discard failed: %+v / %v", discard, err)
	}
	blockedPage, err := query.ShowNativeTask(ctx, "21")
	if err != nil {
		t.Fatal(err)
	}
	if blockedPage.Tasks[0].DependencyReady || len(blockedPage.Tasks[0].UnmetPredecessors) != 1 || blockedPage.Tasks[0].UnmetPredecessors[0].Blocker != "discarded_predecessor" {
		t.Fatalf("discarded predecessor incorrectly released its successor: %+v", blockedPage.Tasks[0])
	}
	if _, err = query.TaskGraph(ctx, graph.NextCursor, 2); err == nil || !strings.Contains(err.Error(), "cursor_invalidated") {
		t.Fatalf("graph cursor survived a task state revision: %v", err)
	}
	if reused, err := commands.ValidateTaskBatch(ctx, c, taskBatchFixture(t, c, commit, batchTasks(20), nil)); err != nil || reused.Valid {
		t.Fatalf("discarded task ID was reused: %+v / %v", reused, err)
	}
}

func TestCreateTaskBatchRollsBackEveryWriteWhenEdgeInsertFails(t *testing.T) {
	ctx := context.Background()
	c, commit, store := openDAGFixture(t)
	commands := CommandService{Store: store, Project: c.Identity()}
	if _, err := store.db.ExecContext(ctx, `CREATE TRIGGER fail_task_edge BEFORE INSERT ON task_dependencies BEGIN SELECT RAISE(ABORT,'injected edge failure'); END`); err != nil {
		t.Fatal(err)
	}
	payload := taskBatchFixture(t, c, commit, batchTasks(40, 41), []TaskBatchEdge{{Predecessor: TaskBatchReference{LocalKey: "k40"}, Successor: TaskBatchReference{LocalKey: "k41"}}})
	if _, err := commands.CreateTaskBatch(ctx, c, payload, strings.Repeat("4", 32), 1, "planner"); err == nil {
		t.Fatal("injected edge failure did not abort creation")
	}
	for _, table := range []string{"tasks", "task_contracts", "task_batches", "task_dependencies", "commands", "events"} {
		var count int
		if err := store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("failed batch left %s rows=%d err=%v", table, count, err)
		}
	}
	if _, err := store.db.ExecContext(ctx, `DROP TRIGGER fail_task_edge`); err != nil {
		t.Fatal(err)
	}
	receipt, err := commands.CreateTaskBatch(ctx, c, payload, strings.Repeat("4", 32), 1, "planner")
	if err != nil || receipt.Status != "accepted" {
		t.Fatalf("request could not be retried after rolled-back transaction: %+v / %v", receipt, err)
	}
}

func TestNativeCommandsRejectConfigurationBindingDriftBeforeSideEffects(t *testing.T) {
	ctx := context.Background()
	c, commit, store := openDAGFixture(t)
	commands := CommandService{Store: store, Project: c.Identity()}
	if _, err := commands.CreateTaskBatch(ctx, c, taskBatchFixture(t, c, commit, batchTasks(50), nil), strings.Repeat("5", 32), 1, "planner"); err != nil {
		t.Fatal(err)
	}
	drifted := c
	drifted.Branch = "other"
	if _, err := commands.ValidateTaskBatch(ctx, drifted, taskBatchFixture(t, c, commit, batchTasks(51), nil)); err == nil || !strings.Contains(err.Error(), "authority_mismatch") {
		t.Fatalf("validation used a changed branch with the registered authority: %v", err)
	}
	if _, err := commands.CreateTaskBatch(ctx, drifted, taskBatchFixture(t, c, commit, batchTasks(51), nil), strings.Repeat("6", 32), 1, "planner"); err == nil || !strings.Contains(err.Error(), "authority_mismatch") {
		t.Fatalf("creation used a changed branch with the registered authority: %v", err)
	}
	if _, err := commands.ClaimNativeTask(ctx, drifted, TaskClaimOptions{TaskID: "50", RequestID: strings.Repeat("7", 32), ExpectedTaskVersion: 1, ExpectedAuthorityVersion: 1, RequestedBy: "operator", Explicit: true}); err == nil || !strings.Contains(err.Error(), "authority_mismatch") {
		t.Fatalf("claim used a changed branch with the registered authority: %v", err)
	}
	if _, err := commands.DiscardNativeTask(ctx, drifted, "50", strings.Repeat("8", 32), 1, "configuration drift", "operator"); err == nil || !strings.Contains(err.Error(), "authority_mismatch") {
		t.Fatalf("discard used a changed branch with the registered authority: %v", err)
	}
	if _, err := commands.CompleteNativeTask(ctx, drifted, "50", 1, "invalid-workflow", &fixedTaskDeliveryVerifier{}); err == nil || !strings.Contains(err.Error(), "authority_mismatch") {
		t.Fatalf("delivery used a changed branch with the registered authority: %v", err)
	}
	for table, want := range map[string]int{"tasks": 1, "commands": 1, "operations": 0} {
		var got int
		if err := store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&got); err != nil || got != want {
			t.Fatalf("configuration drift mutated %s: got %d, want %d, err=%v", table, got, want, err)
		}
	}
}

func TestTaskBatchRejectsDuplicateAndCrossProjectDependencies(t *testing.T) {
	ctx := context.Background()
	c, commit, store := openDAGFixture(t)
	foreignContract := nativeContract(t, c, commit, "foreign-contract-99")
	otherProject := "other-native-project"
	created := time.Now().UTC()
	err := store.Transact(ctx, func(domainTx DomainTx) error {
		tx := domainTx.(*sqliteTx)
		authority := ProjectTaskAuthority{ProjectIdentity: otherProject, Authority: "native", AuthorityVersion: 1, Repository: foreignContract.Repository, Remote: c.Remote, Branch: c.Branch, DispatchMode: "manual", DispatchPaused: true, CreatedAt: created, UpdatedAt: created}
		if e := tx.PutProjectTaskAuthority(authority); e != nil {
			return e
		}
		if e := tx.PutTaskBatch(TaskBatchRecord{BatchID: "foreign-batch", ProjectIdentity: otherProject, SchemaVersion: taskBatchSchemaVersion, PayloadDigest: strings.Repeat("b", 64), CreatedAt: created, CommandID: "foreign-command", TaskIDs: map[string]string{"foreign": "99"}}, 1, 0); e != nil {
			return e
		}
		task := TaskRecord{ProjectIdentity: otherProject, TaskID: "99", BatchID: "foreign-batch", Title: "foreign", Type: "task", Priority: 2, ContractID: foreignContract.ContractID, Source: "native", Status: "open", Version: 1, CreatedAt: created, UpdatedAt: created}
		if e := tx.PutTask(task); e != nil {
			return e
		}
		return tx.PutTaskContract(otherProject, "99", foreignContract)
	})
	if err != nil {
		t.Fatal(err)
	}
	commands := CommandService{Store: store, Project: c.Identity()}
	tasks := batchTasks(30)
	ref := int64(99)
	edges := []TaskBatchEdge{{Predecessor: TaskBatchReference{TaskID: &ref}, Successor: TaskBatchReference{LocalKey: "k30"}}}
	validation, err := commands.ValidateTaskBatch(ctx, c, taskBatchFixture(t, c, commit, tasks, edges))
	if err != nil {
		t.Fatal(err)
	}
	if validation.Valid || !hasTaskIssue(validation.Issues, "cross_project_dependency") {
		t.Fatalf("cross-project dependency was accepted: %+v", validation)
	}
	validateRules := func(payload []byte) (TaskBatchValidation, error) {
		batch, _, err := decodeTaskBatch(payload)
		if err != nil {
			return TaskBatchValidation{}, err
		}
		graph, err := store.taskGraphSnapshot(ctx, c.Identity())
		if err != nil {
			return TaskBatchValidation{}, err
		}
		issues, cycle := validateTaskBatchAgainstGraph(ctx, c, batch, graph, false)
		return TaskBatchValidation{Valid: len(issues) == 0, Issues: issues, Cycle: cycle}, nil
	}
	contractCollision := batchTasks(30)
	contractCollision[0].Contract = nativeContract(t, c, commit, foreignContract.ContractID)
	validation, err = validateRules(taskBatchFixture(t, c, commit, contractCollision, nil))
	if err != nil || validation.Valid || !hasTaskIssue(validation.Issues, "invalid_batch") {
		t.Fatalf("cross-project contract ID mismatch was hidden by validation: %+v / %v", validation, err)
	}
	dupTasks := batchTasks(30, 31)
	dup := []TaskBatchEdge{{Predecessor: TaskBatchReference{LocalKey: "k30"}, Successor: TaskBatchReference{LocalKey: "k31"}}, {Predecessor: TaskBatchReference{LocalKey: "k30"}, Successor: TaskBatchReference{LocalKey: "k31"}}}
	validation, err = validateRules(taskBatchFixture(t, c, commit, dupTasks, dup))
	if err != nil {
		t.Fatal(err)
	}
	if validation.Valid || !hasTaskIssue(validation.Issues, "duplicate_dependency") {
		t.Fatalf("duplicate edge was accepted: %+v", validation)
	}
	self := []TaskBatchEdge{{Predecessor: TaskBatchReference{LocalKey: "k30"}, Successor: TaskBatchReference{LocalKey: "k30"}}}
	validation, err = validateRules(taskBatchFixture(t, c, commit, batchTasks(30), self))
	if err != nil {
		t.Fatal(err)
	}
	if validation.Valid || !hasTaskIssue(validation.Issues, "dependency_cycle") {
		t.Fatalf("self dependency was accepted: %+v", validation)
	}
	missing := int64(404)
	missingEdge := []TaskBatchEdge{{Predecessor: TaskBatchReference{TaskID: &missing}, Successor: TaskBatchReference{LocalKey: "k30"}}}
	validation, err = validateRules(taskBatchFixture(t, c, commit, batchTasks(30), missingEdge))
	if err != nil {
		t.Fatal(err)
	}
	if validation.Valid || !hasTaskIssue(validation.Issues, "missing_predecessor") {
		t.Fatalf("missing predecessor was accepted: %+v", validation)
	}
	oldSuccessor := int64(99)
	badSuccessor := []TaskBatchEdge{{Predecessor: TaskBatchReference{LocalKey: "k30"}, Successor: TaskBatchReference{TaskID: &oldSuccessor}}}
	validation, err = validateRules(taskBatchFixture(t, c, commit, batchTasks(30), badSuccessor))
	if err != nil {
		t.Fatal(err)
	}
	if validation.Valid {
		t.Fatalf("old successor accepted a new dependency: %+v", validation)
	}
}

func TestTaskDependencyStateRequiresEveryPredecessorDelivery(t *testing.T) {
	for _, tc := range []struct {
		name       string
		delivered  int
		wantReady  bool
		wantBlocks int
	}{
		{"none", 0, false, 2},
		{"one", 1, false, 1},
		{"all", 2, true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			task := TaskRecord{TaskID: "3", Status: "open"}
			graph := taskGraphSnapshot{
				ByID:       map[string]TaskRecord{"1": {TaskID: "1", Status: "open"}, "2": {TaskID: "2", Status: "open"}, "3": task},
				Edges:      []TaskDependencyRecord{{PredecessorID: "1", SuccessorID: "3"}, {PredecessorID: "2", SuccessorID: "3"}},
				Deliveries: map[string]TaskDeliveryRecord{},
			}
			for i, id := range []string{"1", "2"} {
				if i < tc.delivered {
					graph.ByID[id] = TaskRecord{TaskID: id, Status: "closed"}
					graph.Deliveries[id] = TaskDeliveryRecord{TaskID: id}
				}
			}
			ready, blockers := taskDependencyState(task, graph)
			if ready != tc.wantReady || len(blockers) != tc.wantBlocks {
				t.Fatalf("ready=%v blockers=%+v, want ready=%v blockers=%d", ready, blockers, tc.wantReady, tc.wantBlocks)
			}
		})
	}
}

func hasTaskIssue(issues []TaskBatchIssue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func hasString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestNativeClaimAllowsManualParallelButKeepsAutoSlot(t *testing.T) {
	ctx := context.Background()
	c, commit := nativeTaskFixture(t)
	if out, err := exec.Command("git", "-C", c.Repository, "push", "--quiet", "origin", "HEAD:refs/heads/main").CombinedOutput(); err != nil {
		t.Fatalf("push fixture branch: %v: %s", err, out)
	}
	reader, err := OpenNativeTaskProject(ctx, &c)
	if err != nil {
		t.Fatal(err)
	}
	if err = reader.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := OpenSQLiteStore(filepath.Join(c.RunDir, "steerline.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	step := func() StepContract {
		return StepContract{Provider: "command", Entry: "builtin:verify", Version: MinimalWorkflowVersion, InputSchema: NativeTaskInputSchema, OutputSchema: MinimalStepOutputSchema, ExecutionSite: "local", Timeout: time.Minute, Command: []string{"git", "--version"}}
	}
	c.WorkflowDefinition = ptr(DefaultMinimalWorkflowDefinition(false))
	c.ProjectSteps = &ProjectSteps{Implement: step(), Verify: step(), PublishPR: step()}
	c.ProjectSteps.Verify.RequiredChecks = []string{"unit"}
	commands := CommandService{Store: store, Project: c.Identity()}
	payload := taskBatchFixture(t, c, commit, batchTasks(80, 81, 82, 83), nil)
	if _, err = commands.CreateTaskBatch(ctx, c, payload, strings.Repeat("b", 32), 1, "planner"); err != nil {
		t.Fatal(err)
	}
	options := TaskClaimOptions{TaskID: "80", RequestID: strings.Repeat("c", 32), ExpectedTaskVersion: 1, ExpectedAuthorityVersion: 1, RequestedBy: "operator", Explicit: true}
	if _, err = commands.ClaimNativeTask(ctx, c, options); err == nil || !strings.Contains(err.Error(), "dispatch_paused") {
		t.Fatalf("paused project admitted a claim: %v", err)
	}
	authority, err := store.nativeAuthority(ctx, c.Identity())
	if err != nil {
		t.Fatal(err)
	}
	authority.AuthorityVersion++
	authority.DispatchPaused = false
	if err = store.Transact(ctx, func(domainTx DomainTx) error {
		return domainTx.(*sqliteTx).UpdateProjectTaskAuthorityCAS(authority, authority.AuthorityVersion-1)
	}); err != nil {
		t.Fatal(err)
	}
	options.ExpectedAuthorityVersion = 2
	receipt, err := commands.ClaimNativeTask(ctx, c, options)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Status != "claim_reserved" || receipt.StartTaskApplied || receipt.WorkflowRunID == "" || receipt.BaseCommit == "" {
		t.Fatalf("claim reservation was reported as a started workflow: %+v", receipt)
	}
	changedVersion := options
	changedVersion.ExpectedAuthorityVersion = 99
	changedVersion.ExpectedTaskVersion = 99
	replayed, err := commands.ClaimNativeTask(ctx, c, changedVersion)
	if err != nil || replayed.WorkflowRunID != receipt.WorkflowRunID {
		t.Fatalf("claim replay did not return original reservation: %+v / %v", replayed, err)
	}
	second, err := commands.ClaimNativeTask(ctx, c, TaskClaimOptions{TaskID: "81", RequestID: strings.Repeat("d", 32), ExpectedTaskVersion: 1, ExpectedAuthorityVersion: 2, RequestedBy: "operator", Explicit: true})
	if err != nil || second.Status != "claim_reserved" || second.WorkflowRunID == receipt.WorkflowRunID {
		t.Fatalf("independent manual task was not reserved: %+v / %v", second, err)
	}
	third, err := commands.ClaimNativeTask(ctx, c, TaskClaimOptions{TaskID: "82", RequestID: strings.Repeat("f", 32), ExpectedTaskVersion: 1, ExpectedAuthorityVersion: 2, RequestedBy: "operator", Explicit: true})
	if err != nil || third.Status != "claim_reserved" || third.WorkflowRunID == receipt.WorkflowRunID || third.WorkflowRunID == second.WorkflowRunID {
		t.Fatalf("third manual task hit a hidden parallel cap: %+v / %v", third, err)
	}
	if _, err = commands.ClaimNativeTask(ctx, c, TaskClaimOptions{TaskID: "80", RequestID: strings.Repeat("e", 32), ExpectedTaskVersion: 1, ExpectedAuthorityVersion: 2, RequestedBy: "operator", Explicit: true}); err == nil || !strings.Contains(err.Error(), "stale_version") {
		t.Fatalf("same task received a second workflow: %v", err)
	}
	page, err := (QueryService{Store: store, Project: c.Identity()}).ShowNativeTask(ctx, "80")
	if err != nil {
		t.Fatal(err)
	}
	if page.Tasks[0].Status != "in_progress" || len(page.Tasks[0].WorkflowRuns) != 1 || page.Tasks[0].WorkflowRuns[0].Phase != "queued" {
		t.Fatalf("claim identities were not persisted together: %+v", page.Tasks[0])
	}
	snapshot, err := store.Query(ctx, c.Identity())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Executions) != 3 || snapshot.Executions[0].Workspace.Repository != c.Identity() || snapshot.Executions[1].Workspace.Repository != c.Identity() || snapshot.Executions[2].Workspace.Repository != c.Identity() {
		t.Fatalf("native claim initialized workspace repository with a non-identity path: %+v", snapshot.Executions)
	}
	alteredWorkflow := page.Tasks[0].WorkflowRuns[0]
	alteredWorkflow.Definition.ReviewRequired = true
	if err = store.Transact(ctx, func(tx DomainTx) error { return tx.PutWorkflowRun(alteredWorkflow) }); err == nil || !strings.Contains(err.Error(), "immutable_task") {
		t.Fatalf("claim workflow gates were edited after reservation: %v", err)
	}
	other, err := (QueryService{Store: store, Project: c.Identity()}).ShowNativeTask(ctx, "81")
	if err != nil {
		t.Fatal(err)
	}
	if hasString(other.Tasks[0].StartupBlockers, "execution_slot_busy") {
		t.Fatalf("manual task inherited another task's slot blocker: %+v", other.Tasks[0].StartupBlockers)
	}
	var operations int
	if err = store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM operations WHERE project_identity=? AND idempotency_key=? AND status='pending'`, c.Identity(), options.RequestID).Scan(&operations); err != nil || operations != 1 {
		t.Fatalf("external preparation intent was not reserved: count=%d err=%v", operations, err)
	}
	auto := "auto"
	if _, err = commands.SetNativeDispatchState(ctx, NativeDispatchUpdate{RequestID: strings.Repeat("1", 32), ExpectedAuthorityVersion: 2, Mode: &auto, RequestedBy: "operator"}); err != nil {
		t.Fatal(err)
	}
	if _, err = commands.ClaimNativeTask(ctx, c, TaskClaimOptions{TaskID: "83", RequestID: strings.Repeat("2", 32), ExpectedTaskVersion: 1, ExpectedAuthorityVersion: 3, RequestedBy: "scheduler", Explicit: false}); err == nil || !strings.Contains(err.Error(), "slot_busy") {
		t.Fatalf("automatic dispatch bypassed its single active execution limit: %v", err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenSQLiteStore(filepath.Join(c.RunDir, "steerline.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	commands.Store = reopened
	replayed, err = commands.ClaimNativeTask(ctx, c, changedVersion)
	if err != nil || replayed.WorkflowRunID != receipt.WorkflowRunID || replayed.StartTaskApplied {
		t.Fatalf("restart changed claim reservation or reported worker start: %+v / %v", replayed, err)
	}
	secondReplay, err := commands.ClaimNativeTask(ctx, c, TaskClaimOptions{TaskID: "81", RequestID: strings.Repeat("d", 32), ExpectedTaskVersion: 1, ExpectedAuthorityVersion: 2, RequestedBy: "operator", Explicit: true})
	if err != nil || secondReplay.WorkflowRunID != second.WorkflowRunID {
		t.Fatalf("restart changed the second claim identity: %+v / %v", secondReplay, err)
	}
}

func TestNativeClaimBlocksWhenFetchedBaseMissesPredecessorMerge(t *testing.T) {
	ctx := context.Background()
	c, commit := nativeTaskFixture(t)
	if out, err := exec.Command("git", "-C", c.Repository, "push", "--quiet", "origin", "HEAD:refs/heads/main").CombinedOutput(); err != nil {
		t.Fatalf("push fixture branch: %v: %s", err, out)
	}
	reader, err := OpenNativeTaskProject(ctx, &c)
	if err != nil {
		t.Fatal(err)
	}
	if err = reader.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := OpenSQLiteStore(filepath.Join(c.RunDir, "steerline.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	authority, err := store.nativeAuthority(ctx, c.Identity())
	if err != nil {
		t.Fatal(err)
	}
	authority.AuthorityVersion++
	authority.DispatchPaused = false
	if err = store.Transact(ctx, func(domainTx DomainTx) error {
		return domainTx.(*sqliteTx).UpdateProjectTaskAuthorityCAS(authority, authority.AuthorityVersion-1)
	}); err != nil {
		t.Fatal(err)
	}
	commands := CommandService{Store: store, Project: c.Identity()}
	payload := taskBatchFixture(t, c, commit, batchTasks(120, 121), []TaskBatchEdge{{Predecessor: TaskBatchReference{LocalKey: "k120"}, Successor: TaskBatchReference{LocalKey: "k121"}}})
	if _, err = commands.CreateTaskBatch(ctx, c, payload, strings.Repeat("5", 32), 2, "planner"); err != nil {
		t.Fatal(err)
	}
	page, err := (QueryService{Store: store, Project: c.Identity()}).ShowNativeTask(ctx, "120")
	if err != nil {
		t.Fatal(err)
	}
	task := page.Tasks[0].TaskRecord
	created := time.Now().UTC()
	run := WorkflowRunRecord{ProjectIdentity: c.Identity(), WorkflowRunID: "wf-120", TaskID: "120", WorkflowVersion: MinimalWorkflowVersion, Phase: "waiting_pr_merge", SelectedResult: "exec-120", CreatedAt: created, UpdatedAt: created}
	execution := ExecutionRecord{ProjectIdentity: c.Identity(), ExecutionID: "exec-120", WorkflowRunID: "wf-120", Status: "succeeded", CurrentStep: "waiting_pr_merge", CreatedAt: created, UpdatedAt: created}
	mergeCommit := strings.Repeat("f", 40)
	mergedAt := created.Add(-time.Minute)
	prURL := "https://forge.invalid/org/repo/pulls/120"
	evidence, _ := json.Marshal(TaskDeliveryEvidence{PRInstance: "https://forge.invalid", URL: prURL, Repository: "org/repo", PRNumber: 120, BaseBranch: "main", State: "merged", Merged: true, MergedAt: mergedAt, MergeCommit: mergeCommit})
	delivery := TaskDeliveryRecord{ProjectIdentity: c.Identity(), TaskID: "120", SelectedWorkflowRunID: run.WorkflowRunID, PRInstance: "https://forge.invalid", PRRepository: "org/repo", PRNumber: 120, PRURL: prURL, MergeCommit: mergeCommit, MergedAt: mergedAt, RecordedAt: created, Evidence: evidence}
	task.Status = "in_progress"
	task.Claimant = "worker"
	task.Version++
	task.UpdatedAt = created
	if err = store.Transact(ctx, func(domainTx DomainTx) error {
		tx := domainTx.(*sqliteTx)
		if e := tx.PutWorkflowRun(run); e != nil {
			return e
		}
		if e := tx.PutExecution(execution); e != nil {
			return e
		}
		if e := tx.UpdateNativeTaskCAS(task, 1); e != nil {
			return e
		}
		if e := tx.PutTaskDelivery(delivery); e != nil {
			return e
		}
		closed := task
		closed.Status = "closed"
		closed.Version++
		closed.UpdatedAt = created
		return tx.UpdateNativeTaskCAS(closed, task.Version)
	}); err != nil {
		t.Fatal(err)
	}
	step := deliveryStepContract()
	c.WorkflowDefinition = ptr(DefaultMinimalWorkflowDefinition(false))
	c.ProjectSteps = &ProjectSteps{Implement: step, Verify: step, PublishPR: step}
	c.ProjectSteps.Verify.RequiredChecks = []string{"unit"}
	_, err = commands.ClaimNativeTask(ctx, c, TaskClaimOptions{TaskID: "121", RequestID: strings.Repeat("6", 32), ExpectedTaskVersion: 1, ExpectedAuthorityVersion: 2, RequestedBy: "operator", Explicit: true})
	if err == nil || !strings.Contains(err.Error(), "base_missing_delivery") {
		t.Fatalf("task started from a base without its predecessor merge: %v", err)
	}
	after, err := (QueryService{Store: store, Project: c.Identity()}).ShowNativeTask(ctx, "121")
	if err != nil {
		t.Fatal(err)
	}
	if after.Tasks[0].Status != "open" || len(after.Tasks[0].WorkflowRuns) != 0 {
		t.Fatalf("base ancestry failure reserved a successor: %+v", after.Tasks[0])
	}
}

func TestNativeClaimCompetitionCreatesOnlyOneWorkflowReservation(t *testing.T) {
	ctx := context.Background()
	c, commit := nativeTaskFixture(t)
	if out, err := exec.Command("git", "-C", c.Repository, "push", "--quiet", "origin", "HEAD:refs/heads/main").CombinedOutput(); err != nil {
		t.Fatalf("push fixture branch: %v: %s", err, out)
	}
	reader, err := OpenNativeTaskProject(ctx, &c)
	if err != nil {
		t.Fatal(err)
	}
	if err = reader.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := OpenSQLiteStore(filepath.Join(c.RunDir, "steerline.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	authority, err := store.nativeAuthority(ctx, c.Identity())
	if err != nil {
		t.Fatal(err)
	}
	authority.AuthorityVersion++
	authority.DispatchPaused = false
	if err = store.Transact(ctx, func(tx DomainTx) error {
		return tx.(*sqliteTx).UpdateProjectTaskAuthorityCAS(authority, authority.AuthorityVersion-1)
	}); err != nil {
		t.Fatal(err)
	}
	step := deliveryStepContract()
	c.WorkflowDefinition = ptr(DefaultMinimalWorkflowDefinition(false))
	c.ProjectSteps = &ProjectSteps{Implement: step, Verify: step, PublishPR: step}
	c.ProjectSteps.Verify.RequiredChecks = []string{"unit"}
	commands := CommandService{Store: store, Project: c.Identity()}
	if _, err = commands.CreateTaskBatch(ctx, c, taskBatchFixture(t, c, commit, batchTasks(82), nil), strings.Repeat("8", 32), 2, "planner"); err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	for _, request := range []string{strings.Repeat("9", 32), strings.Repeat("a", 32)} {
		request := request
		go func() {
			_, claimErr := commands.ClaimNativeTask(ctx, c, TaskClaimOptions{TaskID: "82", RequestID: request, ExpectedTaskVersion: 1, ExpectedAuthorityVersion: 2, RequestedBy: "operator", Explicit: true})
			results <- claimErr
		}()
	}
	errorsFound := 0
	for i := 0; i < 2; i++ {
		if <-results != nil {
			errorsFound++
		}
	}
	var runs, active int
	if err = store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM workflow_runs WHERE project_identity=? AND task_id='82'`, c.Identity()).Scan(&runs); err != nil {
		t.Fatal(err)
	}
	if err = store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks WHERE project_identity=? AND task_id='82' AND status='in_progress'`, c.Identity()).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if errorsFound != 1 || runs != 1 || active != 1 {
		t.Fatalf("concurrent claim did not arbitrate to one reservation: errors=%d runs=%d claimed=%d", errorsFound, runs, active)
	}
}

type fixedTaskDeliveryVerifier struct {
	info    PRInfo
	calls   int
	err     error
	onQuery func()
}

func (v *fixedTaskDeliveryVerifier) Query(ctx context.Context, url string) (PRInfo, error) {
	v.calls++
	if url != v.info.URL {
		return PRInfo{}, ErrPRIdentity
	}
	if v.onQuery != nil {
		v.onQuery()
	}
	return v.info, v.err
}

func deliveryStepContract() StepContract {
	return StepContract{Provider: "command", Entry: "builtin:verify", Version: MinimalWorkflowVersion, InputSchema: NativeTaskInputSchema, OutputSchema: MinimalStepOutputSchema, ExecutionSite: "local", Timeout: time.Minute, Command: []string{"git", "--version"}}
}

func TestTaskDiscardRequestPersistsUnknownAndThenDiscardsOnlyAfterUnmergedProof(t *testing.T) {
	ctx := context.Background()
	c, commit, store := openDAGFixture(t)
	commands := CommandService{Store: store, Project: c.Identity()}
	first := taskBatchFixture(t, c, commit, batchTasks(70), nil)
	created, err := commands.CreateTaskBatch(ctx, c, first, strings.Repeat("7", 32), 1, "planner")
	if err != nil {
		t.Fatal(err)
	}
	page, err := (QueryService{Store: store, Project: c.Identity()}).ShowNativeTask(ctx, "70")
	if err != nil {
		t.Fatal(err)
	}
	discarded, err := commands.DiscardNativeTask(ctx, c, "70", strings.Repeat("8", 32), page.Tasks[0].Version, "no longer needed", "operator")
	if err != nil {
		t.Fatal(err)
	}
	if discarded.Status != "discarded" || discarded.GraphRevision != created.GraphRevision+1 {
		t.Fatalf("task was not discarded atomically: %+v", discarded)
	}
	replayed, err := commands.DiscardNativeTask(ctx, c, "70", strings.Repeat("8", 32), page.Tasks[0].Version, "no longer needed", "operator")
	if err != nil || replayed.Status != "discarded" || replayed.GraphRevision != discarded.GraphRevision {
		t.Fatalf("discard request replay changed its result: %+v / %v", replayed, err)
	}
	drifted := c
	drifted.Branch = "other"
	replayed, err = commands.DiscardNativeTask(ctx, drifted, "70", strings.Repeat("8", 32), page.Tasks[0].Version, "no longer needed", "operator")
	if err != nil || replayed.Status != "discarded" || replayed.GraphRevision != discarded.GraphRevision {
		t.Fatalf("configuration drift prevented reading a committed discard receipt: %+v / %v", replayed, err)
	}
	page, err = (QueryService{Store: store, Project: c.Identity()}).ShowNativeTask(ctx, "70")
	if err != nil {
		t.Fatal(err)
	}
	if page.Tasks[0].Status != "discarded" || page.Tasks[0].DiscardedBy != "operator" || page.Tasks[0].DiscardRequestID != strings.Repeat("8", 32) {
		t.Fatalf("discard audit identity is missing: %+v", page.Tasks[0].TaskRecord)
	}
	second := taskBatchFixture(t, c, commit, batchTasks(71), nil)
	if _, err = commands.CreateTaskBatch(ctx, c, second, strings.Repeat("9", 32), 1, "planner"); err != nil {
		t.Fatal(err)
	}
	page, err = (QueryService{Store: store, Project: c.Identity()}).ShowNativeTask(ctx, "71")
	if err != nil {
		t.Fatal(err)
	}
	task := page.Tasks[0].TaskRecord
	createdAt := time.Now().UTC()
	workflow := WorkflowRunRecord{ProjectIdentity: c.Identity(), WorkflowRunID: "wf-71", TaskID: "71", WorkflowVersion: MinimalWorkflowVersion, Phase: "blocked", CreatedAt: createdAt, UpdatedAt: createdAt}
	execution := ExecutionRecord{ProjectIdentity: c.Identity(), ExecutionID: "exec-71", WorkflowRunID: "wf-71", Status: "blocked", CurrentStep: "publish_pr", CreatedAt: createdAt, UpdatedAt: createdAt}
	attempt := StepAttemptRecord{ProjectIdentity: c.Identity(), AttemptID: "attempt-71", ExecutionID: "exec-71", StepID: "publish_pr", AttemptNumber: 1, OperationID: "publish-op-71", Status: "outcome_unknown", CreatedAt: createdAt, UpdatedAt: createdAt}
	task.Status = "in_progress"
	task.Claimant = "worker"
	task.Version++
	if err = store.Transact(ctx, func(domainTx DomainTx) error {
		tx := domainTx.(*sqliteTx)
		if e := tx.PutWorkflowRun(workflow); e != nil {
			return e
		}
		if e := tx.PutExecution(execution); e != nil {
			return e
		}
		if e := tx.PutStepAttempt(attempt); e != nil {
			return e
		}
		return tx.UpdateNativeTaskCAS(task, page.Tasks[0].Version)
	}); err != nil {
		t.Fatal(err)
	}
	pending, err := commands.DiscardNativeTask(ctx, c, "71", strings.Repeat("a", 32), task.Version, "abandon uncertain publication", "operator")
	if err != nil {
		t.Fatal(err)
	}
	if pending.Status != "pending_verification" || pending.Code != "merge_state_unknown" {
		t.Fatalf("unknown publication was treated as safe to discard: %+v", pending)
	}
	page, err = (QueryService{Store: store, Project: c.Identity()}).ShowNativeTask(ctx, "71")
	if err != nil {
		t.Fatal(err)
	}
	if page.Tasks[0].Status != "in_progress" || page.Tasks[0].Version != task.Version {
		t.Fatalf("unknown external action changed task status/version: %+v", page.Tasks[0].TaskRecord)
	}
	var operationBody []byte
	if err = store.db.QueryRowContext(ctx, `SELECT payload FROM operations WHERE project_identity=? AND idempotency_key=?`, c.Identity(), strings.Repeat("a", 32)).Scan(&operationBody); err != nil {
		t.Fatal(err)
	}
	var operation OperationRecord
	if err = json.Unmarshal(operationBody, &operation); err != nil {
		t.Fatal(err)
	}
	if operation.Status != "outcome_unknown" {
		t.Fatalf("unknown operation identity was not retained: %+v", operation)
	}
}

func TestTaskDiscardRequiresPreciseUnmergedPRAndRetainsUnknownIntent(t *testing.T) {
	states := map[string]string{"90": "open", "91": "merged", "92": "unknown", "94": "open", "95": "open"}
	var statesMu sync.RWMutex
	var lateMergedAt time.Time
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		number := filepath.Base(r.URL.Path)
		numberValue, _ := strconv.Atoi(number)
		statesMu.RLock()
		state := states[number]
		mergedAt := lateMergedAt
		statesMu.RUnlock()
		body := map[string]any{"html_url": server.URL + "/org/repo/pulls/" + number, "number": numberValue, "state": "open", "merged": false, "base": map[string]any{"ref": "main", "repo": map[string]string{"full_name": "org/repo"}}, "head": map[string]string{"sha": strings.Repeat("a", 40)}}
		switch state {
		case "merged":
			body["state"] = "closed"
			body["merged"] = true
			body["merged_at"] = "2026-09-25T01:00:00Z"
			if number == "90" && !mergedAt.IsZero() {
				body["merged_at"] = mergedAt.Format(time.RFC3339Nano)
			}
			body["merge_commit_sha"] = strings.Repeat("b", 40)
		case "unknown":
			delete(body, "merged")
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
	defer server.Close()
	ctx := context.Background()
	c, commit := nativeTaskFixture(t)
	remoteURL := server.URL + "/org/repo.git"
	if out, err := exec.Command("git", "-C", c.Repository, "remote", "set-url", "origin", remoteURL).CombinedOutput(); err != nil {
		t.Fatalf("set fixture remote: %v: %s", err, out)
	}
	reader, err := OpenNativeTaskProject(ctx, &c)
	if err != nil {
		t.Fatal(err)
	}
	if err = reader.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := OpenSQLiteStore(filepath.Join(c.RunDir, "steerline.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	c.PR = PRConfig{Kind: "gitea", BaseURL: server.URL, Repository: "org/repo"}
	commands := CommandService{Store: store, Project: c.Identity()}
	payload := taskBatchFixture(t, c, commit, batchTasks(90, 91, 92, 94, 95), nil)
	if _, err = commands.CreateTaskBatch(ctx, c, payload, strings.Repeat("f", 32), 1, "planner"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"90", "91", "92", "94", "95"} {
		seedPublishedForDiscard(t, ctx, c, store, id, commit, server.URL+"/org/repo/pulls/"+id)
	}
	// The first PR URL cannot settle a later publish attempt whose outcome is unknown.
	unknownAttempt := StepAttemptRecord{ProjectIdentity: c.Identity(), AttemptID: "publish-unknown-94", ExecutionID: "exec-94", StepID: "publish_pr", AttemptNumber: 2, OperationID: "publish-op-unknown-94", Status: "outcome_unknown", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err = store.Transact(ctx, func(tx DomainTx) error { return tx.PutStepAttempt(unknownAttempt) }); err != nil {
		t.Fatal(err)
	}
	var publishBody []byte
	if err = store.db.QueryRowContext(ctx, `SELECT payload FROM step_attempts WHERE project_identity=? AND attempt_id=?`, c.Identity(), "publish-95").Scan(&publishBody); err != nil {
		t.Fatal(err)
	}
	var incomplete StepAttemptRecord
	if err = json.Unmarshal(publishBody, &incomplete); err != nil {
		t.Fatal(err)
	}
	var incompleteResult StepGateResult
	if err = json.Unmarshal(incomplete.Result, &incompleteResult); err != nil {
		t.Fatal(err)
	}
	incompleteResult.ResultID = ""
	incomplete.Result, _ = json.Marshal(incompleteResult)
	if err = store.Transact(ctx, func(tx DomainTx) error { return tx.PutStepAttempt(incomplete) }); err != nil {
		t.Fatal(err)
	}
	open, err := commands.DiscardNativeTask(ctx, c, "90", strings.Repeat("1", 32), 2, "discard open PR", "operator")
	if err != nil {
		t.Fatal(err)
	}
	if open.Status != "discarded" || !hasString(open.CleanupPending, "pull_request_close:"+server.URL+"/org/repo/pulls/90") || !hasString(open.CleanupPending, "workspace_cleanup:tree-90") {
		t.Fatalf("open PR and workspace cleanup were hidden: %+v", open)
	}
	mergedAt := time.Now().UTC()
	lateURL := server.URL + "/org/repo/pulls/90"
	lateMerge := strings.Repeat("a", 40)
	lateEvidence, _ := json.Marshal(TaskDeliveryEvidence{PRInstance: server.URL, URL: lateURL, Repository: "org/repo", PRNumber: 90, BaseBranch: c.Branch, State: "merged", Merged: true, MergedAt: mergedAt, MergeCommit: lateMerge})
	lateDelivery := TaskDeliveryRecord{ProjectIdentity: c.Identity(), TaskID: "90", SelectedWorkflowRunID: "wf-90", PRInstance: server.URL, PRRepository: "org/repo", PRNumber: 90, PRURL: lateURL, MergeCommit: lateMerge, MergedAt: mergedAt, Evidence: lateEvidence}
	if err = store.Transact(ctx, func(tx DomainTx) error { return tx.PutTaskDelivery(lateDelivery) }); err == nil || !strings.Contains(err.Error(), "task_discarded") {
		t.Fatalf("late merge created a normal delivery for discarded task: %v", err)
	}
	verifier := &PRClient{Config: c.PR, Runner: Runner{Dir: c.Repository, Timeout: duration(c.CommandTimeout)}}
	statesMu.Lock()
	states["90"] = "unknown"
	statesMu.Unlock()
	if _, err = commands.CompleteNativeTask(ctx, c, "90", 3, "wf-90", verifier); err == nil || !strings.Contains(err.Error(), "merge_state_unknown") {
		t.Fatalf("unknown late PR state was reported as a confirmed conflict: %v", err)
	}
	statesMu.Lock()
	states["90"] = "merged"
	lateMergedAt = time.Now().UTC()
	statesMu.Unlock()
	for attempt := 0; attempt < 2; attempt++ {
		if _, err = commands.CompleteNativeTask(ctx, c, "90", 3, "wf-90", verifier); err == nil || !strings.Contains(err.Error(), "inconsistent_delivery") {
			t.Fatalf("late PR merge was not reported as a discarded-task conflict: %v", err)
		}
	}
	var conflicts int
	if err = store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE project_identity=? AND task_id=? AND type='task.delivery.conflict'`, c.Identity(), "90").Scan(&conflicts); err != nil || conflicts != 1 {
		t.Fatalf("late merge conflict was not recorded idempotently: count=%d err=%v", conflicts, err)
	}
	var conflictBody []byte
	if err = store.db.QueryRowContext(ctx, `SELECT payload FROM events WHERE project_identity=? AND task_id=? AND type='task.delivery.conflict'`, c.Identity(), "90").Scan(&conflictBody); err != nil {
		t.Fatal(err)
	}
	var conflict struct {
		PRURL       string    `json:"pr_url"`
		DiscardedAt time.Time `json:"discarded_at"`
		MergedAt    time.Time `json:"merged_at"`
	}
	if err = json.Unmarshal(conflictBody, &conflict); err != nil || conflict.PRURL != server.URL+"/org/repo/pulls/90" || !conflict.MergedAt.After(conflict.DiscardedAt) {
		t.Fatalf("late merge conflict lacks fixed PR or chronological proof: %+v / %v", conflict, err)
	}
	conflicted, err := (QueryService{Store: store, Project: c.Identity()}).ShowNativeTask(ctx, "90")
	if err != nil || conflicted.Tasks[0].Status != "discarded" || conflicted.Tasks[0].Delivery != nil {
		t.Fatalf("late merge changed discarded task into normal delivery: %+v / %v", conflicted, err)
	}
	merged, err := commands.DiscardNativeTask(ctx, c, "91", strings.Repeat("2", 32), 2, "discard merged PR", "operator")
	if err != nil {
		t.Fatal(err)
	}
	if merged.Status != "rejected" || merged.Code != "already_merged" {
		t.Fatalf("merged PR was not rejected: %+v", merged)
	}
	unknown, err := commands.DiscardNativeTask(ctx, c, "92", strings.Repeat("3", 32), 2, "discard unknown PR", "operator")
	if err != nil {
		t.Fatal(err)
	}
	if unknown.Status != "pending_verification" || unknown.Code != "merge_state_unknown" {
		t.Fatalf("unknown PR state was called unmerged: %+v", unknown)
	}
	secondUnknown, err := commands.DiscardNativeTask(ctx, c, "94", strings.Repeat("4", 32), 2, "another publication is unresolved", "operator")
	if err != nil || secondUnknown.Status != "pending_verification" || secondUnknown.Code != "merge_state_unknown" {
		t.Fatalf("known PR URL cleared a separate unknown publication: %+v / %v", secondUnknown, err)
	}
	incompleteReceipt, err := commands.DiscardNativeTask(ctx, c, "95", strings.Repeat("5", 32), 2, "publication result identity is missing", "operator")
	if err != nil || incompleteReceipt.Status != "pending_verification" || incompleteReceipt.Code != "merge_state_unknown" {
		t.Fatalf("incomplete published result was treated as a conclusive PR binding: %+v / %v", incompleteReceipt, err)
	}
	operation, err := lookupOperation(ctx, store, c.Identity(), strings.Repeat("4", 32))
	if err != nil {
		t.Fatal(err)
	}
	var checkpoint taskDiscardIntent
	if err = json.Unmarshal(operation.Receipt, &checkpoint); err != nil || checkpoint.PRURL != server.URL+"/org/repo/pulls/94" || !checkpoint.UncertainPR {
		t.Fatalf("discard intent lost its PR binding or uncertain attempt: %+v / %v", checkpoint, err)
	}
	page, err := (QueryService{Store: store, Project: c.Identity()}).ListNativeTasks(ctx, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	statuses := map[string]string{}
	for _, task := range page.Tasks {
		statuses[task.TaskID] = task.Status
	}
	if statuses["90"] != "discarded" || statuses["91"] != "in_progress" || statuses["92"] != "in_progress" || statuses["94"] != "in_progress" || statuses["95"] != "in_progress" {
		t.Fatalf("discarded/merged/unknown PR results changed wrong task states: %v", statuses)
	}
}

func TestTaskDiscardDuringActiveExecutionKeepsSlotAndCleanupIdentity(t *testing.T) {
	ctx := context.Background()
	c, commit, store := openDAGFixture(t)
	commands := CommandService{Store: store, Project: c.Identity()}
	if _, err := commands.CreateTaskBatch(ctx, c, taskBatchFixture(t, c, commit, batchTasks(72), nil), strings.Repeat("7", 32), 1, "planner"); err != nil {
		t.Fatal(err)
	}
	page, err := (QueryService{Store: store, Project: c.Identity()}).ShowNativeTask(ctx, "72")
	if err != nil {
		t.Fatal(err)
	}
	task := page.Tasks[0].TaskRecord
	contract := page.Tasks[0].Contract
	created := time.Now().UTC()
	workspace := WorkspaceEvidence{Repository: c.Repository, Worktree: "tree-72", Branch: "codex/72", Base: commit, Head: commit, VerifiedAt: created}
	run := WorkflowRunRecord{ProjectIdentity: c.Identity(), WorkflowRunID: "wf-72", TaskID: "72", WorkflowVersion: MinimalWorkflowVersion, Phase: "executing", CreatedAt: created, UpdatedAt: created}
	execution := ExecutionRecord{ProjectIdentity: c.Identity(), ExecutionID: "exec-72", WorkflowRunID: run.WorkflowRunID, Status: "active", CurrentStep: "implement", Workspace: workspace, CreatedAt: created, UpdatedAt: created}
	attempt := StepAttemptRecord{ProjectIdentity: c.Identity(), AttemptID: "attempt-72", ExecutionID: execution.ExecutionID, StepID: "implement", AttemptNumber: 1, Status: "running", OperationID: "implement-op-72", ContractDigest: contract.Digest, CreatedAt: created, UpdatedAt: created}
	task.Status = "in_progress"
	task.Claimant = "worker"
	task.Version++
	task.UpdatedAt = created
	if err = store.Transact(ctx, func(domainTx DomainTx) error {
		tx := domainTx.(*sqliteTx)
		if e := tx.PutWorkflowRun(run); e != nil {
			return e
		}
		if e := tx.PutExecution(execution); e != nil {
			return e
		}
		if e := tx.PutStepAttempt(attempt); e != nil {
			return e
		}
		return tx.UpdateNativeTaskCAS(task, 1)
	}); err != nil {
		t.Fatal(err)
	}
	receipt, err := commands.DiscardNativeTask(ctx, c, "72", strings.Repeat("8", 32), task.Version, "stop active task", "operator")
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Status != "discarded" || !hasString(receipt.CleanupPending, "execution_stop_manual:exec-72") || !hasString(receipt.CleanupPending, "workspace_cleanup:tree-72") {
		t.Fatalf("active execution identity or cleanup was lost: %+v", receipt)
	}
	page, err = (QueryService{Store: store, Project: c.Identity()}).ShowNativeTask(ctx, "72")
	if err != nil {
		t.Fatal(err)
	}
	if page.Tasks[0].Status != "discarded" || len(page.Tasks[0].WorkflowRuns) != 1 {
		t.Fatalf("active run was removed by discard: %+v", page.Tasks[0])
	}
	var slots int
	if err = store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM executions WHERE project_identity=? AND status='active'`, c.Identity()).Scan(&slots); err != nil || slots != 1 {
		t.Fatalf("discard released an active execution slot: count=%d err=%v", slots, err)
	}
}

func seedPublishedForDiscard(t *testing.T, ctx context.Context, c Config, store *SQLiteStore, taskID, commit, prURL string) {
	t.Helper()
	page, err := (QueryService{Store: store, Project: c.Identity()}).ShowNativeTask(ctx, taskID)
	if err != nil {
		t.Fatal(err)
	}
	task := page.Tasks[0].TaskRecord
	contract := page.Tasks[0].Contract
	created := time.Now().UTC()
	runID := "wf-" + taskID
	executionID := "exec-" + taskID
	attemptID := "publish-" + taskID
	workspace := WorkspaceEvidence{Repository: c.Repository, Worktree: "tree-" + taskID, Branch: "codex/" + taskID, Base: commit, Head: commit, VerifiedAt: created}
	run := WorkflowRunRecord{ProjectIdentity: c.Identity(), WorkflowRunID: runID, TaskID: taskID, WorkflowVersion: MinimalWorkflowVersion, Phase: "waiting_pr_merge", SelectedResult: executionID, CreatedAt: created, UpdatedAt: created}
	execution := ExecutionRecord{ProjectIdentity: c.Identity(), ExecutionID: executionID, WorkflowRunID: runID, Status: "succeeded", CurrentStep: "waiting_pr_merge", CandidateRevision: commit, Workspace: workspace, CreatedAt: created, UpdatedAt: created}
	result, _ := json.Marshal(StepGateResult{StepID: "publish_pr", Succeeded: true, Revision: commit, PublishedHead: commit, PRURL: prURL, ResultID: "result-" + taskID, Workspace: workspace})
	attempt := StepAttemptRecord{ProjectIdentity: c.Identity(), AttemptID: attemptID, ExecutionID: executionID, StepID: "publish_pr", AttemptNumber: 1, Status: "succeeded", ContractDigest: contract.Digest, ResultID: "result-" + taskID, Result: result, CreatedAt: created, UpdatedAt: created}
	task.Status = "in_progress"
	task.Claimant = "worker"
	task.Version++
	task.UpdatedAt = created
	if err = store.Transact(ctx, func(domainTx DomainTx) error {
		tx := domainTx.(*sqliteTx)
		if e := tx.PutWorkflowRun(run); e != nil {
			return e
		}
		if e := tx.PutExecution(execution); e != nil {
			return e
		}
		if e := tx.PutStepAttempt(attempt); e != nil {
			return e
		}
		return tx.UpdateNativeTaskCAS(task, 1)
	}); err != nil {
		t.Fatal(err)
	}
}

package steerline

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRuntimeProjectionOmitsCenterOwnedControlCommands(t *testing.T) {
	runtime := Runtime{Repository: Repository{Identity: "native-project"}, Control: ControlState{Operation: &ControlOperation{RequestID: strings.Repeat("a", 32), Action: "set_dispatch_mode", Phase: "applying", RequestDigest: strings.Repeat("b", 64)}}}
	if operations := runtimeOperations(runtime); len(operations) != 0 {
		t.Fatalf("native center control was projected as a second operation: %+v", operations)
	}
	runtime.Control.Operation.Action = "stop_service"
	if operations := runtimeOperations(runtime); len(operations) != 1 || operations[0].Kind != "stop_service" {
		t.Fatalf("service stop operation was omitted from the native runtime projection: %+v", operations)
	}
}

func openDomainTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	s, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "steerline.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

const oldActiveWorkflowIndex = `CREATE UNIQUE INDEX one_active_workflow_per_task ON workflow_runs(project_identity,task_id) WHERE phase <> 'completed'`

func TestSQLiteV3IsRejectedWithoutUpgrade(t *testing.T) {
	path := filepath.Join(t.TempDir(), "steerline.db")
	raw, err := sql.Open("sqlite", path)
	require(t, err)
	tx, err := raw.Begin()
	require(t, err)
	_, err = tx.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`)
	require(t, err)
	for _, stmt := range append(append([]string{}, domainExecutionSchema...), nativeTaskSchema...) {
		if stmt == activeWorkflowIndexV5 {
			stmt = oldActiveWorkflowIndex
		}
		_, err = tx.Exec(stmt)
		require(t, err)
	}
	_, err = tx.Exec(`INSERT INTO schema_migrations(version,applied_at) VALUES(3,?)`, time.Now().UTC().Format(time.RFC3339Nano))
	require(t, err)
	require(t, tx.Commit())
	require(t, raw.Close())
	assertOldSQLiteRejectedUnchanged(t, path, 3)
}

func TestSQLiteV4IsRejectedWithoutUpgrade(t *testing.T) {
	path := filepath.Join(t.TempDir(), "steerline.db")
	store, err := OpenSQLiteStore(path)
	require(t, err)
	require(t, store.Close())
	raw, err := sql.Open("sqlite", path)
	require(t, err)
	_, err = raw.Exec(`DROP INDEX one_active_workflow_per_task`)
	require(t, err)
	_, err = raw.Exec(oldActiveWorkflowIndex)
	require(t, err)
	_, err = raw.Exec(`DELETE FROM schema_migrations WHERE version=?`, domainSchemaVersion)
	require(t, err)
	_, err = raw.Exec(`INSERT INTO schema_migrations(version,applied_at) VALUES(4,?)`, now().Format(time.RFC3339Nano))
	require(t, err)
	require(t, raw.Close())
	assertOldSQLiteRejectedUnchanged(t, path, 4)
	reader, err := openSQLiteInspector(path)
	require(t, err)
	defer reader.Close()
	var definition string
	require(t, reader.db.QueryRow(`SELECT sql FROM sqlite_master WHERE name='one_active_workflow_per_task'`).Scan(&definition))
	if strings.Join(strings.Fields(definition), " ") != oldActiveWorkflowIndex {
		t.Fatalf("old workflow index changed during rejection: %s", definition)
	}
}

func assertOldSQLiteRejectedUnchanged(t *testing.T, path string, version int) {
	t.Helper()
	before, err := os.ReadFile(path)
	require(t, err)
	for _, open := range []func(string) (*SQLiteStore, error){OpenSQLiteReader, OpenSQLiteStore} {
		store, openErr := open(path)
		if store != nil {
			_ = store.Close()
		}
		if openErr == nil || !strings.Contains(openErr.Error(), fmt.Sprintf("unsupported SQLite schema %d", version)) {
			t.Fatalf("schema %d was admitted: %v", version, openErr)
		}
	}
	if err = validateExistingDatabase(path); err == nil || !strings.Contains(err.Error(), fmt.Sprintf("unsupported SQLite schema %d", version)) {
		t.Fatalf("schema %d passed preflight: %v", version, err)
	}
	after, err := os.ReadFile(path)
	require(t, err)
	if !bytes.Equal(before, after) {
		t.Fatalf("schema %d database changed during rejection", version)
	}
	reader, err := openSQLiteInspector(path)
	require(t, err)
	defer reader.Close()
	var got int
	require(t, reader.db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&got))
	if got != version {
		t.Fatalf("schema version changed: got %d, want %d", got, version)
	}
}

func TestSQLiteRejectsParallelExecutionsInOneWorkflow(t *testing.T) {
	ctx := context.Background()
	s := openDomainTestStore(t)
	project := "repo:test"
	err := s.Transact(ctx, func(tx DomainTx) error {
		if err := tx.PutTask(TaskRecord{ProjectIdentity: project, TaskID: "17", Status: "in_progress", Version: 1}); err != nil {
			return err
		}
		if err := tx.PutWorkflowRun(WorkflowRunRecord{ProjectIdentity: project, WorkflowRunID: "wf-1", TaskID: "17", WorkflowVersion: "minimal-v1", Phase: "executing"}); err != nil {
			return err
		}
		for _, id := range []string{"exec-a"} {
			if err := tx.PutExecution(ExecutionRecord{ProjectIdentity: project, ExecutionID: id, WorkflowRunID: "wf-1", Agent: id, Status: "active", CurrentStep: "implement", Target: ExecutionTarget{WorktreeID: "worktree-" + id, Branch: "branch-" + id, Dispatch: "dispatch-" + id}}); err != nil {
				return err
			}
			if err := tx.PutStepAttempt(StepAttemptRecord{ProjectIdentity: project, AttemptID: "attempt-" + id, ExecutionID: id, StepID: "implement", AttemptNumber: 1, Status: "running"}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	err = s.Transact(ctx, func(tx DomainTx) error {
		return tx.PutExecution(ExecutionRecord{ProjectIdentity: project, ExecutionID: "exec-b", WorkflowRunID: "wf-1", Status: "active", CurrentStep: "verify"})
	})
	if err == nil {
		t.Fatal("second active execution was accepted")
	}
	snapshot, err := s.Query(ctx, project)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.WorkflowRuns) != 1 || len(snapshot.Executions) != 1 || len(snapshot.Attempts) != 1 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
}

func TestSQLiteTransactionRollsBackAllDomainWrites(t *testing.T) {
	ctx := context.Background()
	s := openDomainTestStore(t)
	project := "repo:rollback"
	err := s.Transact(ctx, func(tx DomainTx) error {
		if err := tx.PutTask(TaskRecord{ProjectIdentity: project, TaskID: "task-a", Status: "open", Version: 1}); err != nil {
			return err
		}
		return tx.PutWorkflowRun(WorkflowRunRecord{ProjectIdentity: project, WorkflowRunID: "wf-bad", TaskID: "missing-task", Phase: "queued"})
	})
	if err == nil {
		t.Fatal("expected FK violation")
	}
	snapshot, queryErr := s.Query(ctx, project)
	if queryErr != nil {
		t.Fatal(queryErr)
	}
	if len(snapshot.Tasks) != 0 || len(snapshot.WorkflowRuns) != 0 {
		t.Fatalf("partial state survived rollback: %+v", snapshot)
	}
}

func TestCommandIdempotencyAndDecisionVersionArbitration(t *testing.T) {
	ctx := context.Background()
	s := openDomainTestStore(t)
	project := "repo:commands"
	request := DecisionRequest{Question: "choose", AllowFreeText: true}
	run := &Run{TaskID: "task", RunID: "wf", DomainTaskID: "task", ExecutionID: "exec", AttemptID: "attempt", ControlRevision: 5, Host: "orca", Backend: string(BackendOrca), Protocol: string(ProtocolOrcaTerminalV2), WorkflowVersion: "legacy-v1", ExecutionStatus: "succeeded", FinishTaskStatus: "waiting_decision", Decision: &DecisionState{ID: "decision", AttemptID: "attempt", Version: 1, Status: "pending", Request: request}}
	projection := Runtime{SchemaVersion: 5, Revision: 1, Repository: Repository{Identity: project}, Service: ServiceState{InstanceID: "service", Status: "running", Ready: true}, Runs: []*Run{run}}
	err := s.Transact(ctx, func(tx DomainTx) error {
		if err := tx.PutTask(TaskRecord{ProjectIdentity: project, TaskID: "task", Status: "in_progress", Version: 1}); err != nil {
			return err
		}
		if err := tx.PutWorkflowRun(WorkflowRunRecord{ProjectIdentity: project, WorkflowRunID: "wf", TaskID: "task", Phase: "executing"}); err != nil {
			return err
		}
		if err := tx.PutExecution(ExecutionRecord{ProjectIdentity: project, ExecutionID: "exec", WorkflowRunID: "wf", Status: "waiting_decision", CurrentStep: "implement"}); err != nil {
			return err
		}
		if err := tx.PutStepAttempt(StepAttemptRecord{ProjectIdentity: project, AttemptID: "attempt", ExecutionID: "exec", StepID: "implement", AttemptNumber: 1, Status: "waiting_decision"}); err != nil {
			return err
		}
		if err := tx.PutDecision(DecisionRecord{ProjectIdentity: project, DecisionID: "decision", Scope: "execution", WorkflowRunID: "wf", ExecutionID: "exec", AttemptID: "attempt", QuestionID: "q1", Status: "pending", Version: 1, Request: mustJSON(request)}); err != nil {
			return err
		}
		return tx.PutProjection(project, projection.Revision, mustJSON(projection))
	})
	if err != nil {
		t.Fatal(err)
	}
	commands := CommandService{Store: s, Project: project}
	answer := DecisionAnswer{DecisionID: "decision", RunID: "wf", Note: "yes", AnsweredBy: "alice", ExpectedDecisionVersion: 1, ExpectedRunRevision: run.ControlRevision, ServiceInstanceID: projection.Service.InstanceID}
	first, created, err := commands.AnswerDecision(ctx, "decision", 1, "answer-key-a", "alice", mustJSON(answer))
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("first answer was not enqueued")
	}
	retry, created, err := commands.AnswerDecision(ctx, "decision", 1, "answer-key-a", "alice", mustJSON(answer))
	if err != nil {
		t.Fatal(err)
	}
	if created || retry.CommandID != first.CommandID {
		t.Fatalf("idempotent retry changed command: %+v %+v", first, retry)
	}
	var savedAnswer DecisionAnswer
	require(t, decode(first.Payload, &savedAnswer))
	if _, _, err = commands.AcceptAnswer(ctx, first, savedAnswer); err != nil {
		t.Fatalf("first answer was not accepted against the current service projection: %v", err)
	}
	if _, _, err = commands.AnswerDecision(ctx, "decision", 1, "answer-key-b", "bob", mustJSON(DecisionAnswer{DecisionID: "decision", RunID: "wf", Note: "no", AnsweredBy: "bob", ExpectedDecisionVersion: 1, ExpectedRunRevision: run.ControlRevision, ServiceInstanceID: projection.Service.InstanceID})); !errors.Is(err, ErrAnswerConflict) {
		t.Fatalf("second accepted answer should lose first-wins arbitration, got %v", err)
	}
	pending, err := commands.Pending(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("unexpected pending commands: %+v", pending)
	}
	snapshot, err := s.Query(ctx, project)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Decisions[0].Status != "answer_saved" || snapshot.Decisions[0].Version != 2 {
		t.Fatalf("first accepted command did not persist its business fact: %+v", snapshot.Decisions[0])
	}
}

func TestPreflightRejectsUnknownSQLiteSchemaWithoutMutation(t *testing.T) {
	runDir := t.TempDir()
	path := filepath.Join(runDir, "steerline.db")
	db, err := sql.Open("sqlite", path)
	require(t, err)
	if _, err = db.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO schema_migrations(version,applied_at) VALUES(99,'unknown')`); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	if err = validateExistingDatabase(path); err == nil || !strings.Contains(err.Error(), "unsupported SQLite schema 99") {
		t.Fatalf("unknown database schema was accepted: %v", err)
	}
	reader, err := openSQLiteInspector(path)
	require(t, err)
	defer reader.Close()
	var version, tableCount int
	require(t, reader.db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&version))
	require(t, reader.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table'`).Scan(&tableCount))
	if version != 99 || tableCount != 1 {
		t.Fatalf("preflight modified unknown database: version=%d tables=%d", version, tableCount)
	}
}

func TestPreflightRejectsRetiredWholeRunResultCommandWithoutMutation(t *testing.T) {
	ctx := context.Background()
	runDir := t.TempDir()
	databasePath := filepath.Join(runDir, "steerline.db")
	store, err := OpenSQLiteStore(databasePath)
	require(t, err)
	commandID, idempotencyKey := newID(), newID()
	command := CommandRecord{ProjectIdentity: "repo:retired-result", CommandID: commandID, Kind: "accept_result", TargetID: "retired-run", ExpectedVersion: 1, IdempotencyKey: idempotencyKey, RequestedBy: "operator", Payload: []byte(`{}`), Status: "pending", CreatedAt: now()}
	err = store.Transact(ctx, func(tx DomainTx) error {
		_, _, enqueueErr := tx.EnqueueCommand(command)
		return enqueueErr
	})
	require(t, err)
	require(t, store.Close())

	reader, err := OpenSQLiteReader(databasePath)
	require(t, err)
	defer reader.Close()
	if err = validateRetiredWholeRunResultCommands(ctx, reader.db); err == nil || !strings.Contains(err.Error(), "retired whole-run result commands") {
		t.Fatalf("retired result command was not rejected during read-only preflight: %v", err)
	}
	saved, err := reader.Command(ctx, command.ProjectIdentity, commandID)
	require(t, err)
	if saved.Kind != command.Kind || saved.Status != command.Status || string(saved.Payload) != string(command.Payload) {
		t.Fatalf("preflight changed retired command state: %+v", saved)
	}
}

func TestQueryEventCursorAndLargeBodyPaging(t *testing.T) {
	ctx := context.Background()
	store := openDomainTestStore(t)
	project := "repo:event-query"
	body := make([]byte, (1<<20)+12345)
	for i := range body {
		body[i] = byte(i % 251)
	}
	var firstID int64
	err := store.Transact(ctx, func(tx DomainTx) error {
		var err error
		firstID, err = tx.AppendEvent(DomainEvent{ProjectIdentity: project, ExecutionID: "exec", AttemptID: "attempt", ExternalSource: "test", ExternalID: "1", Type: "conversation.message", Payload: body})
		if err != nil {
			return err
		}
		_, err = tx.AppendEvent(DomainEvent{ProjectIdentity: project, ExecutionID: "exec", AttemptID: "attempt", ExternalSource: "test", ExternalID: "2", Type: "observation", Payload: []byte("small")})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	query := QueryService{Store: store, Project: project}
	page, err := query.ReadEvents(ctx, EventFilter{ExecutionID: "exec", AttemptID: "attempt"}, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Events) != 1 || page.Events[0].EventID != firstID || !page.Events[0].PayloadOmitted || !page.HasMore || page.NextCursor == "" {
		t.Fatalf("large event page was not bounded: %+v", page)
	}
	firstBody, err := query.ReadEventBody(ctx, firstID, 0, 256<<10)
	if err != nil {
		t.Fatal(err)
	}
	if firstBody.Complete || firstBody.TotalBytes != int64(len(body)) || firstBody.Digest != operationDigest(body) {
		t.Fatalf("bad first body page: %+v", firstBody)
	}
	secondBody, err := query.ReadEventBody(ctx, firstID, firstBody.NextOffset, 256<<10)
	if err != nil {
		t.Fatal(err)
	}
	thirdBody, err := query.ReadEventBody(ctx, firstID, secondBody.NextOffset, 256<<10)
	if err != nil {
		t.Fatal(err)
	}
	fourthBody, err := query.ReadEventBody(ctx, firstID, thirdBody.NextOffset, 256<<10)
	if err != nil {
		t.Fatal(err)
	}
	fifthBody, err := query.ReadEventBody(ctx, firstID, fourthBody.NextOffset, 256<<10)
	if err != nil {
		t.Fatal(err)
	}
	joined := append(append(append(append(append([]byte{}, firstBody.Data...), secondBody.Data...), thirdBody.Data...), fourthBody.Data...), fifthBody.Data...)
	if !fifthBody.Complete || operationDigest(joined) != operationDigest(body) {
		t.Fatal("event body pages did not reconstruct the original payload")
	}
	secondPage, err := query.ReadEvents(ctx, EventFilter{ExecutionID: "exec", AttemptID: "attempt"}, page.NextCursor, 1)
	if err != nil || len(secondPage.Events) != 1 || string(secondPage.Events[0].Payload) != "small" {
		t.Fatalf("second event page: %+v %v", secondPage, err)
	}
	if _, err = query.ReadEvents(ctx, EventFilter{ExecutionID: "other"}, page.NextCursor, 1); err == nil {
		t.Fatal("cursor was reused with a different filter")
	} else if cursorErr, ok := err.(*CursorError); !ok || cursorErr.Code != "cursor_invalidated" {
		t.Fatalf("bad cursor error: %v", err)
	}
}

func TestOperationIdempotencyKeepsOneDurableIntent(t *testing.T) {
	ctx := context.Background()
	store := openDomainTestStore(t)
	project := "repo:ops"
	request := []byte(`{"attempt_id":"attempt-1"}`)
	digest := operationDigest(request)
	base := OperationRecord{ProjectIdentity: project, OperationID: "operation-1", IdempotencyKey: "start:attempt-1", Kind: "start_execution", Status: "pending", PayloadDigest: digest, Request: request}
	err := store.Transact(ctx, func(tx DomainTx) error {
		_, created, err := tx.PutOperation(base)
		if err == nil && !created {
			t.Fatal("initial operation was not created")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	updated := base
	updated.Status = "in_flight"
	err = store.Transact(ctx, func(tx DomainTx) error {
		_, created, err := tx.PutOperation(updated)
		if err == nil && created {
			t.Fatal("retry created a second operation")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	conflict := base
	conflict.Request = []byte(`{"attempt_id":"other"}`)
	conflict.PayloadDigest = operationDigest(conflict.Request)
	err = store.Transact(ctx, func(tx DomainTx) error { _, _, err := tx.PutOperation(conflict); return err })
	if err == nil {
		t.Fatal("same idempotency key with a different payload was accepted")
	}
	var status string
	if err = store.db.QueryRowContext(ctx, `SELECT status FROM operations WHERE project_identity=? AND operation_id=?`, project, base.OperationID).Scan(&status); err != nil || status != "in_flight" {
		t.Fatalf("operation status was not retained: %s %v", status, err)
	}
}

func TestDecisionAnswerFirstWinsIdempotencyAndConflictAreTransactional(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	project := "repo:decision-first-wins"
	db, err := OpenSQLiteStore(filepath.Join(dir, "steerline.db"))
	require(t, err)
	defer db.Close()
	runID := strings.Repeat("a", 32)
	decisionID := strings.Repeat("b", 32)
	seedNativeDecision(t, db, project, runID, decisionID)
	db2, err := OpenSQLiteStore(filepath.Join(dir, "steerline.db"))
	require(t, err)
	defer db2.Close()
	commands1 := CommandService{Store: db, Project: project}
	commands2 := CommandService{Store: db2, Project: project}
	answer := DecisionAnswer{DecisionID: decisionID, RunID: runID, OptionID: "yes", Note: "same", AnsweredBy: "alice", ExpectedDecisionVersion: 1, ExpectedRunRevision: 4, ServiceInstanceID: "service-instance"}
	answerFromSecondClient := answer
	answerFromSecondClient.AnsweredBy = "bob"
	type submitResult struct {
		command CommandRecord
		created bool
		err     error
	}
	start := make(chan struct{})
	results := make(chan submitResult, 2)
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		<-start
		command, created, submitErr := commands1.AnswerDecisionWithID(ctx, strings.Repeat("c", 32), decisionID, 1, decisionAnswerKey(answer), "alice", mustJSON(answer))
		results <- submitResult{command: command, created: created, err: submitErr}
	}()
	go func() {
		defer wait.Done()
		<-start
		command, created, submitErr := commands2.AnswerDecisionWithID(ctx, strings.Repeat("d", 32), decisionID, 1, decisionAnswerKey(answerFromSecondClient), "bob", mustJSON(answerFromSecondClient))
		results <- submitResult{command: command, created: created, err: submitErr}
	}()
	close(start)
	wait.Wait()
	firstResult, secondResult := <-results, <-results
	if firstResult.err != nil || secondResult.err != nil {
		t.Fatalf("concurrent answer submissions failed: first=%v second=%v", firstResult.err, secondResult.err)
	}
	if firstResult.command.CommandID != secondResult.command.CommandID || firstResult.created == secondResult.created {
		t.Fatalf("two clients did not converge on one first-wins command: first=%+v second=%+v", firstResult, secondResult)
	}
	first := firstResult.command
	if !firstResult.created {
		first = secondResult.command
	}
	repeat, created, err := commands1.AnswerDecisionWithID(ctx, strings.Repeat("e", 32), decisionID, 1, decisionAnswerKey(answer), "alice", mustJSON(answer))
	require(t, err)
	if created || repeat.CommandID != first.CommandID || repeat.Status != "pending" {
		t.Fatalf("same accepted answer did not return its original result: %+v created=%v", repeat, created)
	}
	different := answer
	different.OptionID, different.Note = "no", "different"
	if _, created, err = commands1.AnswerDecisionWithID(ctx, strings.Repeat("f", 32), decisionID, 1, decisionAnswerKey(answer), "alice", mustJSON(different)); err == nil || created {
		t.Fatalf("same idempotency key accepted a different answer payload: created=%v err=%v", created, err)
	}
}

type decisionChildResult struct {
	CommandID string `json:"command_id"`
	Created   bool   `json:"created"`
	Error     string `json:"error,omitempty"`
}

func TestDecisionAnswerCommandClientProcess(t *testing.T) {
	if os.Getenv("STEERLINE_PR5_ANSWER_CHILD") != "1" {
		return
	}
	db, err := OpenSQLiteStore(os.Getenv("STEERLINE_PR5_ANSWER_DB"))
	if err != nil {
		t.Fatal(err)
	}
	project, decisionID, commandID, requester := os.Getenv("STEERLINE_PR5_ANSWER_PROJECT"), os.Getenv("STEERLINE_PR5_ANSWER_DECISION"), os.Getenv("STEERLINE_PR5_ANSWER_COMMAND"), os.Getenv("STEERLINE_PR5_ANSWER_BY")
	var answer DecisionAnswer
	if err = json.Unmarshal([]byte(os.Getenv("STEERLINE_PR5_ANSWER_PAYLOAD")), &answer); err != nil {
		t.Fatal(err)
	}
	command, created, submitErr := (CommandService{Store: db, Project: project}).AnswerDecisionWithID(context.Background(), commandID, decisionID, answer.ExpectedDecisionVersion, decisionAnswerKey(answer), requester, mustJSON(answer))
	result := decisionChildResult{CommandID: command.CommandID, Created: created}
	if submitErr != nil {
		result.Error = submitErr.Error()
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(os.Getenv("STEERLINE_PR5_ANSWER_OUTPUT"), data, 0600); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
}

func TestDecisionAnswerSeparateClientProcessesUseOneFirstWinsCommand(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	project := "repo:decision-process-race"
	dbPath := filepath.Join(dir, "steerline.db")
	db, err := OpenSQLiteStore(dbPath)
	require(t, err)
	runID, decisionID := strings.Repeat("1", 32), strings.Repeat("2", 32)
	seedNativeDecision(t, db, project, runID, decisionID)
	_ = db.Close()
	by := []string{"alice", "bob"}
	commands := []string{strings.Repeat("3", 32), strings.Repeat("4", 32)}
	outputPaths := []string{filepath.Join(dir, "client-a.json"), filepath.Join(dir, "client-b.json")}
	children := make([]*exec.Cmd, 2)
	var childOutput [2]bytes.Buffer
	for i := range children {
		answer := DecisionAnswer{DecisionID: decisionID, RunID: runID, OptionID: "yes", Note: "same", AnsweredBy: by[i], ExpectedDecisionVersion: 1, ExpectedRunRevision: 3, ServiceInstanceID: "instance"}
		payload, _ := json.Marshal(answer)
		executable, execErr := os.Executable()
		require(t, execErr)
		children[i] = exec.Command(executable, "-test.run=^TestDecisionAnswerCommandClientProcess$")
		children[i].Stdout, children[i].Stderr = &childOutput[i], &childOutput[i]
		children[i].Env = append(os.Environ(), "STEERLINE_PR5_ANSWER_CHILD=1", "STEERLINE_PR5_ANSWER_DB="+dbPath, "STEERLINE_PR5_ANSWER_PROJECT="+project, "STEERLINE_PR5_ANSWER_DECISION="+decisionID, "STEERLINE_PR5_ANSWER_COMMAND="+commands[i], "STEERLINE_PR5_ANSWER_BY="+by[i], "STEERLINE_PR5_ANSWER_PAYLOAD="+string(payload), "STEERLINE_PR5_ANSWER_OUTPUT="+outputPaths[i])
		require(t, children[i].Start())
	}
	for i, child := range children {
		if waitErr := child.Wait(); waitErr != nil {
			t.Fatalf("answer client process %d failed: %v %s", i, waitErr, childOutput[i].String())
		}
	}
	var results [2]decisionChildResult
	for i, path := range outputPaths {
		data, readErr := os.ReadFile(path)
		require(t, readErr)
		require(t, json.Unmarshal(data, &results[i]))
		if results[i].Error != "" {
			t.Fatalf("answer client process %d returned an error: %s", i, results[i].Error)
		}
	}
	created := 0
	for _, result := range results {
		if result.Created {
			created++
		}
	}
	if created != 1 || results[0].CommandID == "" || results[0].CommandID != results[1].CommandID {
		t.Fatalf("two OS client processes did not converge on one first-wins command: %+v", results)
	}
	store, err := OpenSQLiteStore(dbPath)
	require(t, err)
	defer store.Close()
	commandsPending, err := store.Commands(ctx, project, "pending", 10)
	require(t, err)
	if len(commandsPending) != 1 || commandsPending[0].CommandID != results[0].CommandID {
		t.Fatalf("command history contains duplicate answer writes: %+v", commandsPending)
	}
}

func seedNativeDecision(t *testing.T, db *SQLiteStore, project, runID, decisionID string) {
	t.Helper()
	ctx := context.Background()
	nowTime := now()
	request := DecisionRequest{Question: "Choose", Options: []DecisionOption{{ID: "yes", Label: "Yes"}, {ID: "no", Label: "No"}}}
	err := db.Transact(ctx, func(tx DomainTx) error {
		raw := tx.(*sqliteTx).tx
		if err := tx.PutProjectTaskAuthority(ProjectTaskAuthority{ProjectIdentity: project, Authority: "native", AuthorityVersion: 1, Repository: "repo", Remote: "origin", Branch: "main", DispatchMode: "manual", DispatchPaused: true, CreatedAt: nowTime, UpdatedAt: nowTime}); err != nil {
			return err
		}
		task := TaskRecord{ProjectIdentity: project, TaskID: "1", Title: "Decision task", Type: "task", Priority: 1, BatchID: "batch-1", ContractID: "contract-1", Claimant: "fixture", Source: "native", Status: "in_progress", Version: 1, CreatedAt: nowTime, UpdatedAt: nowTime}
		taskPayload, err := json.Marshal(task)
		if err != nil {
			return err
		}
		if _, err = raw.ExecContext(ctx, `INSERT INTO tasks(project_identity,task_id,status,version,payload) VALUES(?,?,?,?,?)`, project, task.TaskID, task.Status, task.Version, taskPayload); err != nil {
			return fmt.Errorf("insert decision test task: %w", err)
		}
		contractDigest := strings.Repeat("a", 64)
		if _, err = raw.ExecContext(ctx, `INSERT INTO task_contracts(project_identity,contract_id,task_id,digest,payload) VALUES(?,?,?,?,?)`, project, task.ContractID, task.TaskID, contractDigest, []byte(`{"contract_id":"contract-1"}`)); err != nil {
			return err
		}
		batchIDs := []byte(`{"local":"1"}`)
		if _, err = raw.ExecContext(ctx, `INSERT INTO task_batches(project_identity,batch_id,schema_version,payload_digest,created_at,command_id,task_ids) VALUES(?,?,?,?,?,?,?)`, project, task.BatchID, "task-batch/v1", strings.Repeat("b", 64), nowTime.UTC().Format(time.RFC3339Nano), strings.Repeat("c", 32), batchIDs); err != nil {
			return err
		}
		workflow := WorkflowRunRecord{ProjectIdentity: project, WorkflowRunID: runID, TaskID: task.TaskID, WorkflowVersion: WorkerFlowVersion, Phase: "waiting_decision", CreatedAt: nowTime, UpdatedAt: nowTime}
		workflowPayload, err := json.Marshal(workflow)
		if err != nil {
			return err
		}
		if _, err = raw.ExecContext(ctx, `INSERT INTO workflow_runs(project_identity,workflow_run_id,task_id,phase,payload) VALUES(?,?,?,?,?)`, project, runID, "1", workflow.Phase, workflowPayload); err != nil {
			return err
		}
		execution := ExecutionRecord{ProjectIdentity: project, ExecutionID: runID, WorkflowRunID: runID, Status: "waiting_decision", CurrentStep: "decision", CreatedAt: nowTime, UpdatedAt: nowTime}
		executionPayload, err := json.Marshal(execution)
		if err != nil {
			return err
		}
		if _, err = raw.ExecContext(ctx, `INSERT INTO executions(project_identity,execution_id,workflow_run_id,status,current_step,target_json,payload) VALUES(?,?,?,?,?,?,?)`, project, runID, runID, execution.Status, execution.CurrentStep, []byte(`{}`), executionPayload); err != nil {
			return err
		}
		return tx.PutDecision(DecisionRecord{ProjectIdentity: project, DecisionID: decisionID, Scope: "workflow", WorkflowRunID: runID, ExecutionID: runID, QuestionID: "question-1", Request: mustJSON(request), Status: "pending", Version: 1, CreatedAt: nowTime, UpdatedAt: nowTime})
	})
	if err != nil {
		t.Fatalf("seed native decision graph: %v", err)
	}
	store := &Store{Domain: db, Project: project}
	state := Runtime{SchemaVersion: WorkerFlowRuntimeSchema, Repository: Repository{Identity: project, Path: "repo", Remote: "origin", Branch: "main"}, Service: ServiceState{InstanceID: "service-instance", Status: "running", Ready: true, Claimant: "alice"}, Runs: []*Run{}, Tasks: []TaskRecord{}, WorkflowRuns: []WorkflowRunRecord{}, Executions: []ExecutionRecord{}, Attempts: []StepAttemptRecord{}, Decisions: []DecisionRecord{}, Control: ControlState{DispatchMode: "manual", Revision: 1}}
	if err = store.Publish(&state); err != nil {
		t.Fatalf("publish decision test projection: %v", err)
	}
}

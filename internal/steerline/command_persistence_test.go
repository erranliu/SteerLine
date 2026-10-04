package steerline

import (
	"bytes"
	"context"
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

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func TestCommandIdempotencyAndDecisionVersionArbitration(t *testing.T) {
	ctx := context.Background()
	s := openDomainTestStore(t)
	project := "repo:commands"
	request := executionmodel.DecisionRequest{Question: "choose", AllowFreeText: true}
	run := &Run{TaskID: "task", RunID: "wf", DomainTaskID: "task", ExecutionID: "exec", AttemptID: "attempt", ControlRevision: 5, Host: "unregistered", Backend: string(executionbackend.BackendLocal), Protocol: string(neutralProtocol), WorkflowVersion: "legacy-v1", ExecutionStatus: "succeeded", FinishTaskStatus: "waiting_decision", Decision: &executionmodel.DecisionState{ID: "decision", AttemptID: "attempt", Version: 1, Status: "pending", Request: request}}
	projection := ServiceCheckpoint{SchemaVersion: WorkerFlowRuntimeSchema, Revision: 1, Repository: Repository{Identity: project}, Service: ServiceState{InstanceID: "service", Status: "running", Ready: true}}
	err := s.transactRecords(ctx, func(tx *fixtureTransaction) error {
		if err := tx.PutTask(taskmodel.TaskRecord{ProjectIdentity: project, TaskID: "task", Status: "in_progress", Version: 1}); err != nil {
			return err
		}
		if err := tx.PutWorkflowRun(executionmodel.WorkflowRunRecord{ProjectIdentity: project, WorkflowRunID: "wf", TaskID: "task", Phase: "waiting_decision"}); err != nil {
			return err
		}
		if err := tx.PutExecution(executionmodel.ExecutionRecord{ProjectIdentity: project, ExecutionID: "exec", WorkflowRunID: "wf", Status: "idle", CurrentStep: "worker", DecisionID: "decision"}); err != nil {
			return err
		}
		if err := tx.PutStepAttempt(executionmodel.StepAttemptRecord{ProjectIdentity: project, AttemptID: "attempt", ExecutionID: "exec", StepID: "worker", AttemptNumber: 1, Status: "waiting_decision"}); err != nil {
			return err
		}
		if err := tx.PutDecision(executionmodel.DecisionRecord{ProjectIdentity: project, DecisionID: "decision", Scope: "execution", WorkflowRunID: "wf", ExecutionID: "exec", AttemptID: "attempt", QuestionID: "q1", Status: "pending", Version: 1, Request: mustJSON(request)}); err != nil {
			return err
		}
		return tx.PutProjection(project, projection.Revision, mustJSON(projection))
	})
	if err != nil {
		t.Fatal(err)
	}
	commands := newTestCommandService(s, project)
	require(t, s.db.QueryRow(`SELECT control_revision FROM workflow_runs WHERE project_identity=? AND workflow_run_id=?`, project, "wf").Scan(&run.ControlRevision))
	answer := executionmodel.DecisionAnswer{DecisionID: "decision", RunID: "wf", Note: "yes", AnsweredBy: "alice", ExpectedDecisionVersion: 1, ExpectedRunRevision: run.ControlRevision, ServiceInstanceID: projection.Service.InstanceID}
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
	var savedAnswer executionmodel.DecisionAnswer
	require(t, decode(first.Payload, &savedAnswer))
	if _, _, err = commands.AcceptAnswer(ctx, first, savedAnswer); err != nil {
		t.Fatalf("first answer was not accepted against the current service projection: %v", err)
	}
	if _, _, err = commands.AnswerDecision(ctx, "decision", 1, "answer-key-b", "bob", mustJSON(executionmodel.DecisionAnswer{DecisionID: "decision", RunID: "wf", Note: "no", AnsweredBy: "bob", ExpectedDecisionVersion: 1, ExpectedRunRevision: run.ControlRevision, ServiceInstanceID: projection.Service.InstanceID})); !errors.Is(err, storageports.ErrAnswerConflict) {
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

func TestDecisionAnswerFirstWinsIdempotencyAndConflictAreTransactional(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	project := "repo:decision-first-wins"
	db, err := openSQLiteFixture(filepath.Join(dir, "steerline.db"))
	require(t, err)
	defer db.Close()
	runID := strings.Repeat("a", 32)
	decisionID := strings.Repeat("b", 32)
	seedNativeDecision(t, db, project, runID, decisionID)
	db2, err := openSQLiteFixture(filepath.Join(dir, "steerline.db"))
	require(t, err)
	defer db2.Close()
	commands1 := newTestCommandService(db, project)
	commands2 := newTestCommandService(db2, project)
	answer := executionmodel.DecisionAnswer{DecisionID: decisionID, RunID: runID, OptionID: "yes", Note: "same", AnsweredBy: "alice", ExpectedDecisionVersion: 1, ExpectedRunRevision: 4, ServiceInstanceID: "service-instance"}
	answerFromSecondClient := answer
	answerFromSecondClient.AnsweredBy = "bob"
	type submitResult struct {
		command storageports.CommandRecord
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
	db, err := openSQLiteFixture(os.Getenv("STEERLINE_PR5_ANSWER_DB"))
	if err != nil {
		t.Fatal(err)
	}
	project, decisionID, commandID, requester := os.Getenv("STEERLINE_PR5_ANSWER_PROJECT"), os.Getenv("STEERLINE_PR5_ANSWER_DECISION"), os.Getenv("STEERLINE_PR5_ANSWER_COMMAND"), os.Getenv("STEERLINE_PR5_ANSWER_BY")
	var answer executionmodel.DecisionAnswer
	if err = json.Unmarshal([]byte(os.Getenv("STEERLINE_PR5_ANSWER_PAYLOAD")), &answer); err != nil {
		t.Fatal(err)
	}
	command, created, submitErr := (newTestCommandService(db, project)).AnswerDecisionWithID(context.Background(), commandID, decisionID, answer.ExpectedDecisionVersion, decisionAnswerKey(answer), requester, mustJSON(answer))
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
	db, err := openSQLiteFixture(dbPath)
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
		answer := executionmodel.DecisionAnswer{DecisionID: decisionID, RunID: runID, OptionID: "yes", Note: "same", AnsweredBy: by[i], ExpectedDecisionVersion: 1, ExpectedRunRevision: 3, ServiceInstanceID: "instance"}
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
	store, err := openSQLiteFixture(dbPath)
	require(t, err)
	defer store.Close()
	commandsPending, err := store.Commands(ctx, project, "pending", 10)
	require(t, err)
	if len(commandsPending) != 1 || commandsPending[0].CommandID != results[0].CommandID {
		t.Fatalf("command history contains duplicate answer writes: %+v", commandsPending)
	}
}

func seedNativeDecision(t *testing.T, db *sqliteFixture, project, runID, decisionID string) {
	t.Helper()
	ctx := context.Background()
	nowTime := now()
	request := executionmodel.DecisionRequest{Question: "Choose", Options: []executionbackend.DecisionOption{{ID: "yes", Label: "Yes"}, {ID: "no", Label: "No"}}}
	err := db.InitializeProject(ctx, executionmodel.ProjectTaskAuthority{ProjectIdentity: project, Authority: "native", AuthorityVersion: 1, Repository: "repo", Remote: "origin", Branch: "main", DispatchMode: "manual", DispatchPaused: true, CreatedAt: nowTime, UpdatedAt: nowTime})
	require(t, err)
	raw, err := db.db.BeginTx(ctx, nil)
	require(t, err)
	defer raw.Rollback()
	err = func() error {
		task := taskmodel.TaskRecord{ProjectIdentity: project, TaskID: "1", Title: "Decision task", Type: "task", Priority: 1, BatchID: "batch-1", ContractID: "contract-1", Claimant: "fixture", Source: "native", Status: "in_progress", Version: 1, CreatedAt: nowTime, UpdatedAt: nowTime}
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
		workflow := executionmodel.WorkflowRunRecord{ProjectIdentity: project, WorkflowRunID: runID, TaskID: task.TaskID, WorkflowVersion: executionmodel.Version, Phase: "waiting_decision", CreatedAt: nowTime, UpdatedAt: nowTime}
		workflowPayload, err := json.Marshal(workflow)
		if err != nil {
			return err
		}
		if _, err = raw.ExecContext(ctx, `INSERT INTO workflow_runs(project_identity,workflow_run_id,task_id,phase,payload) VALUES(?,?,?,?,?)`, project, runID, "1", workflow.Phase, workflowPayload); err != nil {
			return err
		}
		execution := executionmodel.ExecutionRecord{ProjectIdentity: project, ExecutionID: runID, WorkflowRunID: runID, Status: "waiting_decision", CurrentStep: "decision", CreatedAt: nowTime, UpdatedAt: nowTime}
		executionPayload, err := json.Marshal(execution)
		if err != nil {
			return err
		}
		if _, err = raw.ExecContext(ctx, `INSERT INTO executions(project_identity,execution_id,workflow_run_id,status,current_step,target_json,payload) VALUES(?,?,?,?,?,?,?)`, project, runID, runID, execution.Status, execution.CurrentStep, []byte(`{}`), executionPayload); err != nil {
			return err
		}
		return nil
	}()
	if err == nil {
		err = raw.Commit()
	}
	if err == nil {
		err = transactFixture(db, ctx, func(tx *fixtureTransaction) error {
			return tx.PutDecision(executionmodel.DecisionRecord{ProjectIdentity: project, DecisionID: decisionID, Scope: "workflow", WorkflowRunID: runID, ExecutionID: runID, QuestionID: "question-1", Request: mustJSON(request), Status: "pending", Version: 1, CreatedAt: nowTime, UpdatedAt: nowTime})
		})
	}
	if err != nil {
		t.Fatalf("seed native decision graph: %v", err)
	}
	store := &Store{Domain: db, Project: project}
	state := Runtime{SchemaVersion: WorkerFlowRuntimeSchema, Repository: Repository{Identity: project, Path: "repo", Remote: "origin", Branch: "main"}, Service: ServiceState{InstanceID: "service-instance", Status: "running", Ready: true, Claimant: "alice"}, Runs: []*Run{}, Tasks: []taskmodel.TaskRecord{}, WorkflowRuns: []executionmodel.WorkflowRunRecord{}, Executions: []executionmodel.ExecutionRecord{}, Attempts: []executionmodel.StepAttemptRecord{}, Decisions: []executionmodel.DecisionRecord{}, Control: ControlState{DispatchMode: "manual", Revision: 1}}
	checkpoint := runtimeCheckpoint(state)
	if err = store.Publish(&checkpoint); err != nil {
		t.Fatalf("publish decision test projection: %v", err)
	}
}

func TestCheckpointDoesNotSynthesizeControlOperations(t *testing.T) {
	service, _ := neutralAuditService(t)
	before := recordFingerprint(service.committed.Operations)
	for _, action := range []string{"set_dispatch_mode", "stop_service"} {
		service.State.Control.Operation = &ControlOperation{RequestID: newID(), Action: action, Phase: "applying", RequestDigest: strings.Repeat("b", 64)}
		require(t, service.publishCheckpoint())
		snapshot, err := service.domainStore.Query(context.Background(), service.Config.Identity())
		require(t, err)
		if recordFingerprint(snapshot.Operations) != before {
			t.Fatal("checkpoint synthesized a business operation from its read metadata")
		}
	}
}

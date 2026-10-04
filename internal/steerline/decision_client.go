package steerline

import (
 "github.com/erranliu/SteerLine/internal/execution"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	"os"
	"path/filepath"
	"strings"
)

// AnswerOptions is the CLI entry point for answering a pending decision.
type AnswerOptions struct {
	CommandID       string
	RunID           string
	DecisionID      string
	Option          string
	Note            string
	By              string
	ExpectedVersion uint64
}

// SubmitAnswerDecision publishes one immutable answer command to the live
// service's SQLite authority. It never falls back to a file or offline writer.
func SubmitAnswerDecision(ctx context.Context, c execution.Config, runID, decisionID, optionID, note, requestedBy string, expectedDecisionVersion, expectedRunRevision uint64, expectedInstance, commandID string) (storageports.CommandRecord, bool, error) {
	if strings.TrimSpace(runID) == "" {
		return storageports.CommandRecord{}, false, errors.New("answer requires a run ID")
	}
	if err := requireNativeTaskAuthority(c); err != nil {
		return storageports.CommandRecord{}, false, err
	}
	if err := preflightNativeStatePath(c); err != nil {
		return storageports.CommandRecord{}, false, err
	}
	if err := c.ResolveIdentity(ctx); err != nil {
		return storageports.CommandRecord{}, false, err
	}
	snapshot, err := LoadDatabaseRuntime(c)
	if err != nil {
		return storageports.CommandRecord{}, false, err
	}
	if snapshot == nil {
		return storageports.CommandRecord{}, false, errors.New("no runtime snapshot")
	}
	run := runtimeRunByID(snapshot, runID)
	if run == nil {
		return storageports.CommandRecord{}, false, fmt.Errorf("run %s not found", runID)
	}
	if run.Decision != nil && (run.Decision.RequestPath != "" || run.Decision.AnswerPath != "") {
		return storageports.CommandRecord{}, false, errors.New("decision uses the removed sidecar-answer format; preserve state and do not import answer files")
	}
	if decisionID == "" && run.Decision != nil {
		decisionID = run.Decision.ID
	}
	if decisionID == "" {
		return storageports.CommandRecord{}, false, fmt.Errorf("run %s has no decision identity", runID)
	}
	if requestedBy == "" {
		requestedBy = answererName()
	}
	answer := executionmodel.DecisionAnswer{DecisionID: decisionID, RunID: run.RunID, OptionID: strings.TrimSpace(optionID), Note: note, AnsweredBy: requestedBy, ExpectedDecisionVersion: expectedDecisionVersion, ExpectedRunRevision: expectedRunRevision, ServiceInstanceID: expectedInstance}
	answer.SourceAttemptID, answer.ExpectedRecoveryVersion = run.AttemptID, run.RecoveryVersion
	if run.Decision != nil && run.Decision.ID == decisionID && len(executionmodel.DecisionQuestions(&run.Decision.Request)) > 0 {
		if err := execution.NormalizeNativeDecisionAnswer(&run.Decision.Request, &answer); err != nil {
			return storageports.CommandRecord{}, false, err
		}
	}
	if answer.ExpectedDecisionVersion == 0 && run.Decision != nil {
		answer.ExpectedDecisionVersion = run.Decision.Version
		expectedDecisionVersion = run.Decision.Version
	}
	if answer.ExpectedRunRevision == 0 {
		answer.ExpectedRunRevision = run.ControlRevision
		expectedRunRevision = run.ControlRevision
	}
	key := execution.DecisionAnswerKey(answer)
	reader, err := c.Persistence.OpenReader(filepath.Join(c.RunDir, "steerline.db"))
	if err != nil {
		return storageports.CommandRecord{}, false, err
	}
	existing, lookupErr := reader.CommandByIdempotency(ctx, c.Identity(), key)
	_ = reader.Close()
	if lookupErr == nil {
		var previous executionmodel.DecisionAnswer
		if existing.Kind != "answer_decision" || existing.TargetID != decisionID || decode(existing.Payload, &previous) != nil || !executionmodel.SameDecisionAnswer(previous, answer) {
			return storageports.CommandRecord{}, false, errors.New("answer idempotency identity conflicts with a different command")
		}
		return existing, false, nil
	}
	if !errors.Is(lookupErr, storageports.ErrNotFound) {
		return storageports.CommandRecord{}, false, lookupErr
	}
	connection := ConnectService(ctx, c)
	if !connection.CanControl || connection.Snapshot.Runtime == nil {
		diagnostic := connection.Diagnostic
		if diagnostic == "" {
			diagnostic = string(connection.State)
		}
		return storageports.CommandRecord{}, false, fmt.Errorf("service is offline or not ready for decision commands: %s", diagnostic)
	}
	serviceRuntime := connection.Snapshot.Runtime
	if !serviceHasCapability(serviceRuntime, "answer_decision") {
		return storageports.CommandRecord{}, false, errors.New("service does not advertise answer_decision capability")
	}
	snapshot, err = LoadDatabaseRuntime(c)
	if err != nil {
		return storageports.CommandRecord{}, false, err
	}
	if snapshot == nil || snapshot.Service.InstanceID != serviceRuntime.Service.InstanceID || snapshot.Service.Status != "running" || !snapshot.Service.Ready {
		return storageports.CommandRecord{}, false, errors.New("stale_instance: SQLite projection does not match the ready service instance")
	}
	run = runtimeRunByID(snapshot, runID)
	if run == nil {
		return storageports.CommandRecord{}, false, fmt.Errorf("run %s not found", runID)
	}
	if run.AttemptID != answer.SourceAttemptID || run.RecoveryVersion != answer.ExpectedRecoveryVersion {
		return storageports.CommandRecord{}, false, errors.New("stale_recovery: worker Attempt or recovery version changed; refresh before answering")
	}
	if expectedRunRevision == 0 {
		expectedRunRevision = run.ControlRevision
	}
	answer.ExpectedRunRevision = expectedRunRevision
	answer.ServiceInstanceID = serviceRuntime.Service.InstanceID
	var decisionRecord *executionmodel.DecisionRecord
	for i := range snapshot.Decisions {
		if snapshot.Decisions[i].DecisionID == decisionID {
			decisionRecord = &snapshot.Decisions[i]
			break
		}
	}
	if decisionRecord == nil {
		return storageports.CommandRecord{}, false, errors.New("decision is missing from the authoritative SQLite store")
	}
	if decisionRecord.Status != "pending" {
		var saved executionmodel.DecisionAnswer
		if len(decisionRecord.Answer) == 0 || decode(decisionRecord.Answer, &saved) != nil || !executionmodel.SameDecisionAnswer(saved, answer) {
			return storageports.CommandRecord{}, false, storageports.ErrAnswerConflict
		}
		return storageports.CommandRecord{}, false, fmt.Errorf("decision is already %s but its original idempotency command is missing", decisionRecord.Status)
	}
	if run.FinishTaskStatus != "waiting_decision" && !(run.WorkflowVersion == executionmodel.Version && run.FinishTaskStatus == "waiting_pr_merge") || run.Decision == nil {
		return storageports.CommandRecord{}, false, fmt.Errorf("run %s is not waiting for a decision (status %s)", run.TaskID, run.FinishTaskStatus)
	}
	if expectedInstance != "" && serviceRuntime.Service.InstanceID != expectedInstance {
		return storageports.CommandRecord{}, false, errors.New("stale_instance: service instance changed; refresh before answering")
	}
	if expectedRunRevision == 0 || run.ControlRevision != expectedRunRevision {
		return storageports.CommandRecord{}, false, errors.New("stale_revision: run changed; refresh before answering")
	}
	if run.Decision.ID != decisionID || run.Decision.Status != "pending" {
		return storageports.CommandRecord{}, false, errors.New("stale_revision: the displayed decision is no longer current")
	}
	if expectedDecisionVersion != 0 && run.Decision.Version != expectedDecisionVersion {
		return storageports.CommandRecord{}, false, errors.New("stale_revision: decision version changed; refresh before answering")
	}
	for _, e := range snapshot.Executions {
		if e.ExecutionID == run.ExecutionID && e.Environment != nil && e.Environment.Blocker != nil {
			return storageports.CommandRecord{}, false, nativeErr("environment_blocked", "resolve the environment blocker before sending feedback")
		}
	}
	if err = execution.DecisionReplyEligibility(ctx, c, run); err != nil {
		return storageports.CommandRecord{}, false, err
	}
	if err = executionmodel.ValidateDecisionAnswer(&run.Decision.Request, &answer); err != nil {
		return storageports.CommandRecord{}, false, err
	}
	version := decisionRecord.Version
	if version == 0 {
		return storageports.CommandRecord{}, false, errors.New("decision is missing from the authoritative SQLite store")
	}
	if expectedDecisionVersion == 0 {
		answer.ExpectedDecisionVersion = version
	}
	db, err := c.Persistence.OpenWriter(filepath.Join(c.RunDir, "steerline.db"))
	if err != nil {
		return storageports.CommandRecord{}, false, err
	}
	defer db.Close()
	if commandID == "" {
		commandID = newID()
	}
	payload, err := json.Marshal(answer)
	if err != nil {
		return storageports.CommandRecord{}, false, err
	}
	return (execution.NewCommandService(db, c.Identity(), nil, nil)).AnswerDecisionWithID(ctx, commandID, decisionID, version, key, requestedBy, payload)
}

func runtimeRunByID(runtimeState *Runtime, runID string) *Run {
	if runtimeState == nil {
		return nil
	}
	for _, run := range runtimeState.Runs {
		if run != nil && run.RunID == runID {
			return run
		}
	}
	return nil
}

// AnswerDecision is the CLI wrapper over the same current CommandService path
// used by the console. Offline execution is read-only and returns an error.
func AnswerDecision(c execution.Config, o AnswerOptions) error {
	_, _, err := SubmitDecisionAnswer(context.Background(), c, o)
	return err
}

func SubmitDecisionAnswer(ctx context.Context, c execution.Config, o AnswerOptions) (storageports.CommandRecord, bool, error) {
	if strings.TrimSpace(o.RunID) == "" {
		return storageports.CommandRecord{}, false, errors.New("answer requires a run ID or task ID")
	}
	if err := requireNativeTaskAuthority(c); err != nil {
		return storageports.CommandRecord{}, false, err
	}
	if err := preflightNativeStatePath(c); err != nil {
		return storageports.CommandRecord{}, false, err
	}
	if err := c.ResolveIdentity(ctx); err != nil {
		return storageports.CommandRecord{}, false, err
	}
	snapshot, err := LoadDatabaseRuntime(c)
	if err != nil {
		return storageports.CommandRecord{}, false, err
	}
	if snapshot == nil {
		return storageports.CommandRecord{}, false, errors.New("no runtime snapshot")
	}
	runID := o.RunID
	var selected *Run
	for _, run := range snapshot.Runs {
		if run != nil && run.RunID == o.RunID {
			selected = run
			break
		}
		if run != nil && run.TaskID == o.RunID {
			if selected != nil {
				return storageports.CommandRecord{}, false, nativeErr("ambiguous_execution", "task has multiple candidates; select workflow_run_id")
			}
			selected = run
		}
	}
	if selected != nil {
		runID = selected.RunID
	}
	if o.By == "" {
		o.By = answererName()
	}
	return SubmitAnswerDecision(ctx, c, runID, o.DecisionID, o.Option, o.Note, o.By, o.ExpectedVersion, 0, "", o.CommandID)
}

// AnswererName resolves the default "answered by" identity from the environment.
func AnswererName() string { return answererName() }

func answererName() string {
	for _, key := range []string{"STEERLINE_ANSWERED_BY", "USERNAME", "USER"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return "unknown"
}

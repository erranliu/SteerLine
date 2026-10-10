package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	"os"
	"strings"

	"github.com/erranliu/SteerLine/internal/execution"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
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
func SubmitAnswerDecision(ctx context.Context, c Config, runID, decisionID, optionID, note, requestedBy string, expectedDecisionVersion, expectedRunRevision uint64, expectedInstance, commandID string, answers ...map[string][]string) (storageports.CommandRecord, bool, error) {
	return submitAnswerDecisionWithStoreBound(ctx, c, nil, runID, decisionID, optionID, note, requestedBy, expectedDecisionVersion, expectedRunRevision, expectedInstance, commandID, answerBinding{}, answers...)
}

func submitAnswerDecisionWithStore(ctx context.Context, c Config, store *sqlite.SQLiteStore, runID, decisionID, optionID, note, requestedBy string, expectedDecisionVersion, expectedRunRevision uint64, expectedInstance, commandID string, answers ...map[string][]string) (storageports.CommandRecord, bool, error) {
	return submitAnswerDecisionWithStoreBound(ctx, c, store, runID, decisionID, optionID, note, requestedBy, expectedDecisionVersion, expectedRunRevision, expectedInstance, commandID, answerBinding{}, answers...)
}

type answerBinding struct {
	AttemptID       string
	RecoveryVersion uint64
}

func validateAnswerBinding(run *reconstructiontypes.RunView, binding answerBinding) error {
	if binding.AttemptID != "" && (run == nil || run.AttemptID != binding.AttemptID || run.RecoveryVersion != binding.RecoveryVersion) {
		return errors.New("stale_recovery: source Attempt or recovery version changed; refresh before answering")
	}
	return nil
}

// SubmitAnswerDecisionBound consumes the run/attempt/recovery identity shown
// by the HTTP client. The same SQLite transaction path still owns the actual
// answer command and acceptance rules.
func SubmitAnswerDecisionBound(ctx context.Context, c Config, runID, decisionID, optionID, note, requestedBy string,
	expectedDecisionVersion, expectedRunRevision uint64, expectedInstance, commandID, expectedAttemptID string,
	expectedRecoveryVersion uint64, answers ...map[string][]string) (storageports.CommandRecord, bool, error) {
	return submitAnswerDecisionWithStoreBound(ctx, c, nil, runID, decisionID, optionID, note, requestedBy,
		expectedDecisionVersion, expectedRunRevision, expectedInstance, commandID,
		answerBinding{AttemptID: expectedAttemptID, RecoveryVersion: expectedRecoveryVersion}, answers...)
}

func submitAnswerDecisionWithStoreBound(ctx context.Context, c Config, store *sqlite.SQLiteStore, runID, decisionID, optionID, note, requestedBy string,
	expectedDecisionVersion, expectedRunRevision uint64, expectedInstance, commandID string, binding answerBinding,
	answers ...map[string][]string) (storageports.CommandRecord, bool, error) {
	if strings.TrimSpace(runID) == "" {
		return storageports.CommandRecord{}, false, errors.New("answer requires a run ID")
	}
	if err := c.RequireNativeTaskAuthority(); err != nil {
		return storageports.CommandRecord{}, false, err
	}
	if err := PreflightNativeStatePath(c); err != nil {
		return storageports.CommandRecord{}, false, err
	}
	if err := c.ResolveIdentity(ctx); err != nil {
		return storageports.CommandRecord{}, false, err
	}
	snapshot, err := readConfiguredRuntime(ctx, c)
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
	if err = validateAnswerBinding(run, binding); err != nil {
		return storageports.CommandRecord{}, false, err
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
	var questionAnswers map[string][]string
	if len(answers) > 0 && answers[0] != nil {
		questionAnswers = make(map[string][]string, len(answers[0]))
		for id, values := range answers[0] {
			questionAnswers[id] = append([]string(nil), values...)
		}
	}
	answer := executionmodel.DecisionAnswer{DecisionID: decisionID, RunID: run.RunID, OptionID: strings.TrimSpace(optionID), Note: note, Answers: questionAnswers, AnsweredBy: requestedBy, ExpectedDecisionVersion: expectedDecisionVersion, ExpectedRunRevision: expectedRunRevision, ServiceInstanceID: expectedInstance}
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
	ownedStore := store == nil
	if ownedStore {
		store, err = OpenNativeTaskCommandStore(ctx, &c)
		if err != nil {
			return storageports.CommandRecord{}, false, err
		}
		defer store.Close()
	}
	existing, lookupErr := store.CommandByIdempotency(ctx, c.Identity(), key)
	if lookupErr == nil {
		var previous executionmodel.DecisionAnswer
		if existing.Kind != "answer_decision" || existing.TargetID != decisionID || decodeStrictJSON(existing.Payload, &previous) != nil || !executionmodel.SameDecisionAnswer(previous, answer) {
			return storageports.CommandRecord{}, false, errors.New("answer idempotency identity conflicts with a different command")
		}
		return existing, false, nil
	}
	if !errors.Is(lookupErr, storageports.ErrNotFound) {
		return storageports.CommandRecord{}, false, lookupErr
	}
	connection := connectHostService(ctx, c)
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
	snapshot, err = readConfiguredRuntime(ctx, c)
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
		if len(decisionRecord.Answer) == 0 || decodeStrictJSON(decisionRecord.Answer, &saved) != nil || !executionmodel.SameDecisionAnswer(saved, answer) {
			return storageports.CommandRecord{}, false, storageports.ErrAnswerConflict
		}
		return storageports.CommandRecord{}, false, fmt.Errorf("decision is already %s but its original idempotency command is missing", decisionRecord.Status)
	}
	closed := run.TaskStatus == "closed" || run.TaskStatus == "discarded" || run.Abandonment != nil || run.WorkflowStatus == "completed" || run.WorkflowStatus == "abandoned"
	waitingMerge := run.WorkflowVersion == executionmodel.Version && run.WorkflowStage == executionmodel.StepAwaitMerge && run.WorkflowStatus == "waiting_external"
	async := run.Decision != nil && run.Decision.Request.InteractionKind == "async"
	if closed || run.Decision == nil || run.WorkflowStatus != "waiting_input" && !waitingMerge && !async {
		return storageports.CommandRecord{}, false, fmt.Errorf("run %s is not waiting for a decision (status %s)", run.TaskID, run.WorkflowStatus)
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
			return storageports.CommandRecord{}, false, taskError("environment_blocked", "resolve the environment blocker before sending feedback")
		}
	}
	if err = execution.DecisionReplyEligibility(ctx, c.executionConfig(), run); err != nil {
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
	if commandID == "" {
		commandID = newCommandID()
	}
	payload, err := json.Marshal(answer)
	if err != nil {
		return storageports.CommandRecord{}, false, err
	}
	return (execution.NewCommandService(store, c.Identity(), nil, nil)).AnswerDecisionWithID(ctx, commandID, decisionID, version, key, requestedBy, payload)
}

func serviceHasCapability(runtimeState *reconstructiontypes.Runtime, action string) bool {
	if runtimeState == nil {
		return false
	}
	for _, capability := range runtimeState.Service.Capabilities {
		if capability == action {
			return true
		}
	}
	return false
}

func readConfiguredRuntime(ctx context.Context, config Config) (*reconstructiontypes.Runtime, error) {
	if err := config.RequireNativeTaskAuthority(); err != nil {
		return nil, err
	}
	if config.Identity() == "" {
		if err := config.ResolveIdentity(ctx); err != nil {
			return nil, err
		}
	}
	return LoadDatabaseRuntime(config)
}

func newCommandID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(value[:])
}

func runtimeRunByID(runtimeState *reconstructiontypes.Runtime, runID string) *reconstructiontypes.RunView {
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
func AnswerDecision(c Config, o AnswerOptions) error {
	_, _, err := SubmitDecisionAnswer(context.Background(), c, o)
	return err
}

func SubmitDecisionAnswer(ctx context.Context, c Config, o AnswerOptions) (storageports.CommandRecord, bool, error) {
	return submitDecisionAnswerWithStore(ctx, c, nil, o)
}

func submitDecisionAnswerWithStore(ctx context.Context, c Config, store *sqlite.SQLiteStore, o AnswerOptions) (storageports.CommandRecord, bool, error) {
	if strings.TrimSpace(o.RunID) == "" {
		return storageports.CommandRecord{}, false, errors.New("answer requires a run ID or task ID")
	}
	if err := c.RequireNativeTaskAuthority(); err != nil {
		return storageports.CommandRecord{}, false, err
	}
	if err := PreflightNativeStatePath(c); err != nil {
		return storageports.CommandRecord{}, false, err
	}
	if err := c.ResolveIdentity(ctx); err != nil {
		return storageports.CommandRecord{}, false, err
	}
	snapshot, err := readConfiguredRuntime(ctx, c)
	if err != nil {
		return storageports.CommandRecord{}, false, err
	}
	if snapshot == nil {
		return storageports.CommandRecord{}, false, errors.New("no runtime snapshot")
	}
	runID := o.RunID
	var selected *reconstructiontypes.RunView
	for _, run := range snapshot.Runs {
		if run != nil && run.RunID == o.RunID {
			selected = run
			break
		}
		if run != nil && run.TaskID == o.RunID {
			if selected != nil {
				return storageports.CommandRecord{}, false, taskError("ambiguous_execution", "task has multiple candidates; select workflow_run_id")
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
	return submitAnswerDecisionWithStore(ctx, c, store, runID, o.DecisionID, o.Option, o.Note, o.By, o.ExpectedVersion, 0, "", o.CommandID)
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

package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/persistence"
)

// Decision questions and accepted answers are records in the current SQLite
// authority. Clients append answer_decision commands; the single service writer
// validates and accepts them before applyDecisions considers continuation.

var ErrAnswerConflict = persistence.ErrAnswerConflict

func validateDecisionRequest(req *DecisionRequest) error {
	return contracts.ValidateDecisionRequest(req)
}

// WriteDecisionRequest creates a new decision identity in the service-owned
// runtime projection. The caller persists it to SQLite with the run transition.
func WriteDecisionRequest(r *Run, req DecisionRequest) (*DecisionState, error) {
	return writeDecisionRequestWithID(r, req, newID())
}

func decisionRequestFromEventFact(fact executionapi.EventFact) DecisionRequest {
	request := DecisionRequest{FormatVersion: contracts.DecisionRequestFormatVersion, Question: fact.QuestionText,
		Context: fact.QuestionContext, Options: append([]executionapi.DecisionOption(nil), fact.QuestionOptions...), AllowFreeText: fact.AllowFreeText}
	if len(fact.Questions) == 0 {
		return request
	}
	request.Questions = cloneExecutionQuestions(fact.Questions)
	if fact.QuestionRoute != nil {
		route := fact.QuestionRoute.Clone()
		request.QuestionRoute = &route
	}
	if len(fact.Questions) == 1 {
		question := fact.Questions[0]
		request.Question = question.Question
		request.Options = append([]executionapi.DecisionOption(nil), question.Options...)
		request.AllowFreeText = question.AllowFreeText
		if request.Context == "" && fact.QuestionRoute != nil {
			request.Context = "A human answer is required to continue the original execution."
		}
		return request
	}
	request.Question = fmt.Sprintf("Current execution has %d questions requiring an answer", len(fact.Questions))
	var body strings.Builder
	if request.Context == "" && fact.QuestionRoute != nil {
		request.Context = "A human answer is required to continue the original execution."
	}
	if request.Context != "" {
		body.WriteString(request.Context)
		body.WriteString("\n\n")
	}
	body.WriteString("Answer every question using JSON keyed by question ID.")
	for _, question := range fact.Questions {
		fmt.Fprintf(&body, "\n\n[%s] %s: %s", question.ID, question.Header, question.Question)
		for _, option := range question.Options {
			fmt.Fprintf(&body, "\n- %s: %s", option.ID, option.Label)
			if option.Detail != "" {
				fmt.Fprintf(&body, " (%s)", option.Detail)
			}
		}
		if question.AllowFreeText {
			body.WriteString("\nFree text is allowed.")
		}
	}
	request.Context = body.String()
	request.AllowFreeText = true
	return request
}

func writeDecisionRequestWithID(r *Run, req DecisionRequest, decisionID string) (*DecisionState, error) {
	if r == nil || strings.TrimSpace(decisionID) == "" {
		return nil, errors.New("decision requires a run and stable ID")
	}
	req.FormatVersion = contracts.DecisionRequestFormatVersion
	if e := validateDecisionRequest(&req); e != nil {
		return nil, e
	}
	if req.QuestionRoute != nil && (req.QuestionRoute.WorkflowRunID != r.RunID || req.QuestionRoute.ExecutionID != r.ExecutionID || req.QuestionRoute.AttemptID != r.AttemptID) {
		return nil, errors.New("question route does not belong to the decision's fixed execution and attempt")
	}
	req.Questions = cloneExecutionQuestions(req.Questions)
	if req.QuestionRoute != nil {
		route := req.QuestionRoute.Clone()
		req.QuestionRoute = &route
	}
	st := &DecisionState{
		ID:        decisionID,
		AttemptID: r.AttemptID,
		Version:   1,
		Request:   req,
		Status:    "pending",
	}
	return st, nil
}

func validateDecisionAnswer(req *DecisionRequest, a *DecisionAnswer) error {
	return contracts.ValidateDecisionAnswer(req, a)
}

func normalizeNativeDecisionAnswer(req *DecisionRequest, answer *DecisionAnswer) error {
	if req == nil || len(decisionQuestions(req)) == 0 || answer == nil {
		return errors.New("native answer has no fixed question")
	}
	questions := decisionQuestions(req)
	if len(answer.Answers) != 0 {
		return validateDecisionAnswer(req, answer)
	}
	note := strings.TrimSpace(answer.Note)
	if strings.HasPrefix(note, "{") {
		var values map[string][]string
		if err := decode([]byte(note), &values); err != nil {
			return fmt.Errorf("native multi-question answer must be JSON keyed by question ID: %w", err)
		}
		answer.Answers = values
	} else if len(questions) == 1 {
		q := questions[0]
		values := []string{}
		if answer.OptionID != "" {
			for _, option := range q.Options {
				if option.ID == answer.OptionID {
					values = append(values, option.Label)
					break
				}
			}
			if len(values) == 0 {
				return errors.New("native answer option is not offered")
			}
		}
		if note != "" {
			values = append(values, note)
		}
		answer.Answers = map[string][]string{q.ID: values}
	} else {
		return errors.New("answer all native questions using -note JSON, keyed by question ID")
	}
	return validateDecisionAnswer(req, answer)
}

func decisionQuestions(req *DecisionRequest) []executionapi.Question {
	return contracts.DecisionQuestions(req)
}

func cloneExecutionQuestions(questions []executionapi.Question) []executionapi.Question {
	copy := make([]executionapi.Question, len(questions))
	for i := range questions {
		copy[i] = questions[i]
		copy[i].Options = append([]executionapi.DecisionOption(nil), questions[i].Options...)
	}
	return copy
}

// ReadDecisionAnswer returns the answer already committed to the current
// service projection. A pending decision has no answer; clients cannot create
// one by writing a file.
func ReadDecisionAnswer(r *Run) (DecisionAnswer, error) {
	if r.Decision == nil {
		return DecisionAnswer{}, errors.New("run has no decision")
	}
	if r.Decision.Status == "pending" {
		return DecisionAnswer{}, errors.New("decision has no accepted answer")
	}
	a := DecisionAnswer{DecisionID: r.Decision.ID, RunID: r.RunID, OptionID: r.Decision.OptionID, Note: r.Decision.Note, Answers: r.Decision.Answers, AnsweredBy: r.Decision.AnsweredBy, SourceAttemptID: r.Decision.SourceAttemptID, ExpectedRecoveryVersion: r.Decision.RecoveryVersion}
	if r.Decision.AnsweredAt != nil {
		a.AnsweredAt = *r.Decision.AnsweredAt
	}
	if e := validateDecisionAnswer(&r.Decision.Request, &a); e != nil {
		return DecisionAnswer{}, e
	}
	return a, nil
}

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
func SubmitAnswerDecision(ctx context.Context, c Config, runID, decisionID, optionID, note, requestedBy string, expectedDecisionVersion, expectedRunRevision uint64, expectedInstance, commandID string) (CommandRecord, bool, error) {
	if strings.TrimSpace(runID) == "" {
		return CommandRecord{}, false, errors.New("answer requires a run ID")
	}
	if err := requireNativeTaskAuthority(c); err != nil {
		return CommandRecord{}, false, err
	}
	if err := preflightNativeStatePath(c); err != nil {
		return CommandRecord{}, false, err
	}
	if err := c.ResolveIdentity(ctx); err != nil {
		return CommandRecord{}, false, err
	}
	snapshot, err := LoadDatabaseRuntime(c)
	if err != nil {
		return CommandRecord{}, false, err
	}
	if snapshot == nil {
		return CommandRecord{}, false, errors.New("no runtime snapshot")
	}
	run := runtimeRunByID(snapshot, runID)
	if run == nil {
		return CommandRecord{}, false, fmt.Errorf("run %s not found", runID)
	}
	if run.Decision != nil && (run.Decision.RequestPath != "" || run.Decision.AnswerPath != "") {
		return CommandRecord{}, false, errors.New("decision uses the removed sidecar-answer format; preserve state and do not import answer files")
	}
	if decisionID == "" && run.Decision != nil {
		decisionID = run.Decision.ID
	}
	if decisionID == "" {
		return CommandRecord{}, false, fmt.Errorf("run %s has no decision identity", runID)
	}
	if requestedBy == "" {
		requestedBy = answererName()
	}
	answer := DecisionAnswer{DecisionID: decisionID, RunID: run.RunID, OptionID: strings.TrimSpace(optionID), Note: note, AnsweredBy: requestedBy, ExpectedDecisionVersion: expectedDecisionVersion, ExpectedRunRevision: expectedRunRevision, ServiceInstanceID: expectedInstance}
	answer.SourceAttemptID, answer.ExpectedRecoveryVersion = run.AttemptID, run.RecoveryVersion
	if run.Decision != nil && run.Decision.ID == decisionID && len(decisionQuestions(&run.Decision.Request)) > 0 {
		if err := normalizeNativeDecisionAnswer(&run.Decision.Request, &answer); err != nil {
			return CommandRecord{}, false, err
		}
	}
	key := decisionAnswerKey(answer)
	reader, err := c.Persistence.OpenReader(filepath.Join(c.RunDir, "steerline.db"))
	if err != nil {
		return CommandRecord{}, false, err
	}
	existing, lookupErr := reader.CommandByIdempotency(ctx, c.Identity(), key)
	_ = reader.Close()
	if lookupErr == nil {
		var previous DecisionAnswer
		if existing.Kind != "answer_decision" || existing.TargetID != decisionID || decode(existing.Payload, &previous) != nil || !contracts.SameDecisionAnswer(previous, answer) {
			return CommandRecord{}, false, errors.New("answer idempotency identity conflicts with a different command")
		}
		return existing, false, nil
	}
	if !errors.Is(lookupErr, persistence.ErrNotFound) {
		return CommandRecord{}, false, lookupErr
	}
	connection := ConnectService(ctx, c)
	if !connection.CanControl || connection.Snapshot.Runtime == nil {
		diagnostic := connection.Diagnostic
		if diagnostic == "" {
			diagnostic = string(connection.State)
		}
		return CommandRecord{}, false, fmt.Errorf("service is offline or not ready for decision commands: %s", diagnostic)
	}
	serviceRuntime := connection.Snapshot.Runtime
	if !serviceHasCapability(serviceRuntime, "answer_decision") {
		return CommandRecord{}, false, errors.New("service does not advertise answer_decision capability")
	}
	snapshot, err = LoadDatabaseRuntime(c)
	if err != nil {
		return CommandRecord{}, false, err
	}
	if snapshot == nil || snapshot.Service.InstanceID != serviceRuntime.Service.InstanceID || snapshot.Service.Status != "running" || !snapshot.Service.Ready {
		return CommandRecord{}, false, errors.New("stale_instance: SQLite projection does not match the ready service instance")
	}
	run = runtimeRunByID(snapshot, runID)
	if run == nil {
		return CommandRecord{}, false, fmt.Errorf("run %s not found", runID)
	}
	if run.AttemptID != answer.SourceAttemptID || run.RecoveryVersion != answer.ExpectedRecoveryVersion {
		return CommandRecord{}, false, errors.New("stale_recovery: worker Attempt or recovery version changed; refresh before answering")
	}
	if expectedRunRevision == 0 {
		expectedRunRevision = run.ControlRevision
	}
	answer.ExpectedRunRevision = expectedRunRevision
	answer.ServiceInstanceID = serviceRuntime.Service.InstanceID
	var decisionRecord *DecisionRecord
	for i := range snapshot.Decisions {
		if snapshot.Decisions[i].DecisionID == decisionID {
			decisionRecord = &snapshot.Decisions[i]
			break
		}
	}
	if decisionRecord == nil {
		return CommandRecord{}, false, errors.New("decision is missing from the authoritative SQLite store")
	}
	if decisionRecord.Status != "pending" {
		var saved DecisionAnswer
		if len(decisionRecord.Answer) == 0 || decode(decisionRecord.Answer, &saved) != nil || !contracts.SameDecisionAnswer(saved, answer) {
			return CommandRecord{}, false, ErrAnswerConflict
		}
		return CommandRecord{}, false, fmt.Errorf("decision is already %s but its original idempotency command is missing", decisionRecord.Status)
	}
	if run.FinishTaskStatus != "waiting_decision" && !(run.WorkflowVersion == WorkerFlowVersion && run.FinishTaskStatus == "waiting_pr_merge") || run.Decision == nil {
		return CommandRecord{}, false, fmt.Errorf("run %s is not waiting for a decision (status %s)", run.TaskID, run.FinishTaskStatus)
	}
	if expectedInstance != "" && serviceRuntime.Service.InstanceID != expectedInstance {
		return CommandRecord{}, false, errors.New("stale_instance: service instance changed; refresh before answering")
	}
	if expectedRunRevision == 0 || run.ControlRevision != expectedRunRevision {
		return CommandRecord{}, false, errors.New("stale_revision: run changed; refresh before answering")
	}
	if run.Decision.ID != decisionID || run.Decision.Status != "pending" {
		return CommandRecord{}, false, errors.New("stale_revision: the displayed decision is no longer current")
	}
	if expectedDecisionVersion != 0 && run.Decision.Version != expectedDecisionVersion {
		return CommandRecord{}, false, errors.New("stale_revision: decision version changed; refresh before answering")
	}
	if err = DecisionReplyEligibility(ctx, c, run); err != nil {
		return CommandRecord{}, false, err
	}
	if err = validateDecisionAnswer(&run.Decision.Request, &answer); err != nil {
		return CommandRecord{}, false, err
	}
	version := decisionRecord.Version
	if version == 0 {
		return CommandRecord{}, false, errors.New("decision is missing from the authoritative SQLite store")
	}
	if expectedDecisionVersion == 0 {
		answer.ExpectedDecisionVersion = version
	}
	db, err := c.Persistence.OpenWriter(filepath.Join(c.RunDir, "steerline.db"))
	if err != nil {
		return CommandRecord{}, false, err
	}
	defer db.Close()
	if commandID == "" {
		commandID = newID()
	}
	payload, err := json.Marshal(answer)
	if err != nil {
		return CommandRecord{}, false, err
	}
	return (CommandService{Store: db, Project: c.Identity()}).AnswerDecisionWithID(ctx, commandID, decisionID, version, key, requestedBy, payload)
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
func AnswerDecision(c Config, o AnswerOptions) error {
	_, _, err := SubmitDecisionAnswer(context.Background(), c, o)
	return err
}

func SubmitDecisionAnswer(ctx context.Context, c Config, o AnswerOptions) (CommandRecord, bool, error) {
	if strings.TrimSpace(o.RunID) == "" {
		return CommandRecord{}, false, errors.New("answer requires a run ID or task ID")
	}
	if err := requireNativeTaskAuthority(c); err != nil {
		return CommandRecord{}, false, err
	}
	if err := preflightNativeStatePath(c); err != nil {
		return CommandRecord{}, false, err
	}
	if err := c.ResolveIdentity(ctx); err != nil {
		return CommandRecord{}, false, err
	}
	snapshot, err := LoadDatabaseRuntime(c)
	if err != nil {
		return CommandRecord{}, false, err
	}
	if snapshot == nil {
		return CommandRecord{}, false, errors.New("no runtime snapshot")
	}
	runID := o.RunID
	for _, run := range snapshot.Runs {
		if run != nil && (run.RunID == o.RunID || run.TaskID == o.RunID) {
			runID = run.RunID
			break
		}
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

func decisionAnswerMeta(r *Run) map[string]any {
	if r.Decision == nil {
		return nil
	}
	return map[string]any{
		"decision_id": r.Decision.ID,
		"option_id":   r.Decision.OptionID,
		"answered_by": r.Decision.AnsweredBy,
	}
}

func (s *Service) requireDecisionReplyCapability(ctx context.Context, run *Run) error {
	return DecisionReplyEligibility(ctx, s.Config, run)
}

// DecisionReplyEligibility is the shared read-only qualification used by the
// CLI, console client and service handler. It checks the immutable execution
// binding and verified reply capabilities for this exact target.
func DecisionReplyEligibility(ctx context.Context, c Config, run *Run) error {
	if err := requireNativeTaskAuthority(c); err != nil {
		return err
	}
	if run == nil || run.Decision == nil || run.Decision.Status != "pending" {
		return errors.New("decision is not pending on the current native run")
	}
	if run.WorkflowVersion != WorkerFlowVersion || !executionapi.StableExecutionID(run.ExecutionID) || run.AttemptID == "" || run.Decision.AttemptID != run.AttemptID {
		return &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "feedback does not belong to the fixed worker execution"}
	}
	if target := run.Decision.Request.HostTarget; target != nil {
		if target.ExecutionID != run.ExecutionID || !executionapi.StableExecutionID(target.TurnID) || !executionapi.StableExecutionID(target.BlockerID) {
			return errors.New("host action has an invalid fixed execution target")
		}
		return ctx.Err()
	}
	if run.RecoveryRequired && (run.RecoveryState == "unverified" || run.RecoveryState == "" ||
		run.RecoveryState == "interrupted" && run.Decision.Request.QuestionRoute != nil) {
		return &executionapi.BackendError{Code: executionapi.BackendUnsupported, Diagnostic: "original worker state or native question is not verified; wait for a recovery decision"}
	}
	return ctx.Err()
}
func (s *Service) applyDecisions(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.State.Control.Operation != nil {
		return nil
	}
	for _, run := range s.runs {
		if run == nil || run.FinishTaskStatus != "waiting_decision" && run.FinishTaskStatus != "waiting_pr_merge" || run.Decision == nil || run.Decision.Status != "answer_saved" {
			continue
		}
		blocked := false
		for _, blocker := range s.State.Control.Blockers {
			if blocker.RunID == run.RunID || blocker.TaskID == run.TaskID {
				blocked = true
				break
			}
		}
		if blocked && run.Decision.Request.HostTarget == nil {
			continue
		}
		if run.WorkflowVersion != WorkerFlowVersion {
			return nativeErr("unsupported_schema", "saved decision is not bound to a current worker execution")
		}
		if err := s.applyWorkerFlowDecision(ctx, run); err != nil {
			return err
		}
	}
	return nil
}

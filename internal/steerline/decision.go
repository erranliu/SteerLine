package steerline

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// Decision questions and accepted answers are records in the current SQLite
// authority. Clients append answer_decision commands; the single service writer
// validates and accepts them before applyDecisions considers continuation.

var ErrAnswerConflict = errors.New("answer_conflict: this decision already has a different accepted answer")

func sameDecisionAnswer(a, b DecisionAnswer) bool {
	return a.DecisionID == b.DecisionID && a.RunID == b.RunID && a.SourceAttemptID == b.SourceAttemptID && a.ExpectedRecoveryVersion == b.ExpectedRecoveryVersion && strings.TrimSpace(a.OptionID) == strings.TrimSpace(b.OptionID) && strings.TrimSpace(a.Note) == strings.TrimSpace(b.Note) && digestValue(a.Answers) == digestValue(b.Answers)
}

func validateDecisionRequest(req *DecisionRequest) error {
	if req == nil {
		return errors.New("decision object is missing")
	}
	req.Question = strings.TrimSpace(req.Question)
	req.Context = strings.TrimSpace(req.Context)
	if !utf8.ValidString(req.Question) || !utf8.ValidString(req.Context) {
		return errors.New("decision question and context must be valid UTF-8")
	}
	if req.Question == "" {
		return errors.New("decision question is empty")
	}
	if req.NativeAsk != nil {
		ask := req.NativeAsk
		if !codexThreadID.MatchString(ask.ThreadID) || !codexThreadID.MatchString(ask.TurnID) || ask.CallID == "" || ask.Generation == "" || len(ask.RequestID) == 0 || !json.Valid(ask.RequestID) || len(ask.Questions) == 0 || len(ask.Questions) > 3 {
			return errors.New("native decision lacks its fixed app-server request binding")
		}
	}
	if req.OrcaAsk != nil {
		ask := req.OrcaAsk
		if req.NativeAsk != nil || ask.RunID == "" || ask.TaskID == "" || ask.DispatchID == "" || ask.MessageID == "" || len(ask.Questions) != 1 || ask.Questions[0].ID != ask.MessageID {
			return errors.New("Orca decision lacks its original Run, Task, Dispatch or question binding")
		}
	}
	if len([]rune(req.Question)) > 2000 || len([]rune(req.Context)) > 8000 {
		return errors.New("decision question or context exceeds its protocol length limit")
	}
	if len(req.Options) == 0 && !req.AllowFreeText {
		return errors.New("decision needs options or allow_free_text")
	}
	if len(req.Options) > 12 {
		return errors.New("decision has too many options")
	}
	seen := map[string]bool{}
	for i := range req.Options {
		o := &req.Options[i]
		o.ID = strings.TrimSpace(o.ID)
		o.Label = strings.TrimSpace(o.Label)
		o.Detail = strings.TrimSpace(o.Detail)
		if !utf8.ValidString(o.ID) || !utf8.ValidString(o.Label) || !utf8.ValidString(o.Detail) {
			return fmt.Errorf("decision option %d contains invalid UTF-8", i+1)
		}
		switch {
		case o.ID == "":
			return errors.New("decision option id is empty")
		case len([]rune(o.ID)) > 128:
			return fmt.Errorf("decision option %q id exceeds its protocol length limit", o.ID)
		case o.Label == "":
			return fmt.Errorf("decision option %q has no label", o.ID)
		case len([]rune(o.Label)) > 300:
			return fmt.Errorf("decision option %q label exceeds its protocol length limit", o.ID)
		case len([]rune(o.Detail)) > 4000:
			return fmt.Errorf("decision option %q detail exceeds its protocol length limit", o.ID)
		case seen[o.ID]:
			return fmt.Errorf("duplicate decision option id %q", o.ID)
		}
		seen[o.ID] = true
	}
	if req.Recommended != "" && !seen[req.Recommended] {
		return errors.New("recommended option is not offered")
	}
	return nil
}

// WriteDecisionRequest creates a new decision identity in the service-owned
// runtime projection. The caller persists it to SQLite with the run transition.
func WriteDecisionRequest(r *Run, req DecisionRequest) (*DecisionState, error) {
	return writeDecisionRequestWithID(r, req, newID())
}

func writeDecisionRequestWithID(r *Run, req DecisionRequest, decisionID string) (*DecisionState, error) {
	if r == nil || strings.TrimSpace(decisionID) == "" {
		return nil, errors.New("decision requires a run and stable ID")
	}
	if e := validateDecisionRequest(&req); e != nil {
		return nil, e
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
	a.OptionID = strings.TrimSpace(a.OptionID)
	a.Note = strings.TrimSpace(a.Note)
	a.AnsweredBy = strings.TrimSpace(a.AnsweredBy)
	if a.AnsweredBy == "" {
		return errors.New("answer requires answered_by")
	}
	if !utf8.ValidString(a.OptionID) || !utf8.ValidString(a.Note) || !utf8.ValidString(a.AnsweredBy) {
		return errors.New("answer fields must be valid UTF-8")
	}
	if len([]rune(a.Note)) > 4000 {
		return errors.New("answer note exceeds the 4000 character protocol limit")
	}
	if len([]rune(a.AnsweredBy)) > 200 {
		return errors.New("answered_by exceeds the 200 character protocol limit")
	}
	if questions := nativeDecisionQuestions(req); len(questions) > 0 {
		if len(a.Answers) != len(questions) {
			return errors.New("native answer must include every question ID")
		}
		for _, q := range questions {
			values, ok := a.Answers[q.ID]
			if !ok || len(values) == 0 || len(values) > 2 {
				return fmt.Errorf("native answer for %q is missing or too long", q.ID)
			}
			for _, value := range values {
				if strings.TrimSpace(value) == "" || !utf8.ValidString(value) || len([]rune(value)) > 4000 {
					return fmt.Errorf("native answer for %q is invalid", q.ID)
				}
			}
			if len(q.Options) > 0 && !q.AllowFreeText {
				offered := false
				for _, option := range q.Options {
					if values[0] == option.ID || values[0] == option.Label {
						offered = true
						break
					}
				}
				if !offered {
					return fmt.Errorf("native answer for %q is not offered", q.ID)
				}
			}
		}
		return nil
	}
	if len(req.Options) > 0 {
		if a.OptionID == "" {
			if req.AllowFreeText && a.Note != "" {
				return nil
			}
			return errors.New("answer requires an offered option")
		}
		for _, o := range req.Options {
			if o.ID == a.OptionID {
				return nil
			}
		}
		return fmt.Errorf("option %q is not one of the offered options", a.OptionID)
	}
	if a.Note == "" {
		return errors.New("this decision requires a free-text note")
	}
	return nil
}

func normalizeNativeDecisionAnswer(req *DecisionRequest, answer *DecisionAnswer) error {
	if req == nil || len(nativeDecisionQuestions(req)) == 0 || answer == nil {
		return errors.New("native answer has no fixed question")
	}
	questions := nativeDecisionQuestions(req)
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

func nativeDecisionQuestions(req *DecisionRequest) []nativeAskQuestion {
	if req == nil {
		return nil
	}
	if req.NativeAsk != nil {
		return req.NativeAsk.Questions
	}
	if req.OrcaAsk != nil {
		return req.OrcaAsk.Questions
	}
	return nil
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
	if run.Decision != nil && run.Decision.ID == decisionID && len(nativeDecisionQuestions(&run.Decision.Request)) > 0 {
		if err := normalizeNativeDecisionAnswer(&run.Decision.Request, &answer); err != nil {
			return CommandRecord{}, false, err
		}
	}
	key := decisionAnswerKey(answer)
	reader, err := OpenSQLiteReader(filepath.Join(c.RunDir, "steerline.db"))
	if err != nil {
		return CommandRecord{}, false, err
	}
	existing, lookupErr := reader.CommandByIdempotency(ctx, c.Identity(), key)
	_ = reader.Close()
	if lookupErr == nil {
		var previous DecisionAnswer
		if existing.Kind != "answer_decision" || existing.TargetID != decisionID || decode(existing.Payload, &previous) != nil || !sameDecisionAnswer(previous, answer) {
			return CommandRecord{}, false, errors.New("answer idempotency identity conflicts with a different command")
		}
		return existing, false, nil
	}
	if !errors.Is(lookupErr, sql.ErrNoRows) {
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
		if len(decisionRecord.Answer) == 0 || decode(decisionRecord.Answer, &saved) != nil || !sameDecisionAnswer(saved, answer) {
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
	db, err := OpenSQLiteStore(filepath.Join(c.RunDir, "steerline.db"))
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
	return SubmitAnswerDecision(ctx, c, runID, o.DecisionID, o.Option, o.Note, o.By, o.ExpectedVersion, 0, "", "")
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

func decisionAnswerSummary(r *Run) string {
	if r.Decision == nil {
		return ""
	}
	if r.Decision.OptionID != "" {
		for _, o := range r.Decision.Request.Options {
			if o.ID == r.Decision.OptionID {
				if r.Decision.Note != "" {
					return r.Decision.OptionID + ": " + o.Label + " — " + r.Decision.Note
				}
				return r.Decision.OptionID + ": " + o.Label
			}
		}
		return r.Decision.OptionID
	}
	return r.Decision.Note
}

func DecisionSummary(run *Run) string { return decisionAnswerSummary(run) }

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

func decisionAnswerDigest(answer DecisionAnswer) string {
	return digestValue(struct {
		OptionID, Note string
		Answers        map[string][]string
	}{strings.TrimSpace(answer.OptionID), strings.TrimSpace(answer.Note), answer.Answers})
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
	if run.WorkflowVersion != WorkerFlowVersion || !stableExecutionID(run.ExecutionID) || run.AttemptID == "" || run.Decision.AttemptID != run.AttemptID {
		return &BackendError{Code: BackendIdentityMismatch, Diagnostic: "feedback does not belong to the fixed worker execution"}
	}
	if run.RecoveryRequired && (run.RecoveryState == "unverified" || run.RecoveryState == "" ||
		run.RecoveryState == "interrupted" && run.Decision.Request.NativeAsk != nil) {
		return &BackendError{Code: BackendUnsupported, Diagnostic: "original worker state or native question is not verified; wait for a recovery decision"}
	}
	if run.SessionID != nil && !codexThreadID.MatchString(*run.SessionID) {
		return &BackendError{Code: BackendIdentityMismatch, Diagnostic: "saved worker session identity is invalid"}
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
		if blocked {
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

// Native Orca Dispatches are unsupervised. The center owns the exact agent
// terminal created with the worktree and never uses Codex's exec protocol.
type orcaAskCall struct {
	RunID      string              `json:"run_id"`
	TaskID     string              `json:"task_id"`
	DispatchID string              `json:"dispatch_id"`
	MessageID  string              `json:"message_id"`
	Questions  []nativeAskQuestion `json:"questions"`
}

type nativeAskQuestion struct {
	ID            string           `json:"id"`
	Header        string           `json:"header"`
	Question      string           `json:"question"`
	Options       []DecisionOption `json:"options"`
	AllowFreeText bool             `json:"allow_free_text"`
}

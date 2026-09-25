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
	"time"
	"unicode/utf8"
)

// Decision questions and accepted answers are records in the current SQLite
// authority. Clients append answer_decision commands; the single service writer
// validates and accepts them before applyDecisions considers continuation.

const (
	decisionContinueTxt = "continue.txt"
)

var ErrAnswerConflict = errors.New("answer_conflict: this decision already has a different accepted answer")

func sameDecisionAnswer(a, b DecisionAnswer) bool {
	return a.DecisionID == b.DecisionID && a.RunID == b.RunID && strings.TrimSpace(a.OptionID) == strings.TrimSpace(b.OptionID) && strings.TrimSpace(a.Note) == strings.TrimSpace(b.Note)
}

func decisionContinuePath(r *Run) string {
	return filepath.Join(filepath.Dir(*r.ResultPath), decisionContinueTxt)
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
	if e := validateDecisionRequest(&req); e != nil {
		return nil, e
	}
	st := &DecisionState{
		ID:        newID(),
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
	a := DecisionAnswer{DecisionID: r.Decision.ID, RunID: r.RunID, OptionID: r.Decision.OptionID, Note: r.Decision.Note, AnsweredBy: r.Decision.AnsweredBy}
	if r.Decision.AnsweredAt != nil {
		a.AnsweredAt = *r.Decision.AnsweredAt
	}
	if e := validateDecisionAnswer(&r.Decision.Request, &a); e != nil {
		return DecisionAnswer{}, e
	}
	return a, nil
}

// AnswerOptions is the CLI/viewer entry point for answering a pending decision.
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
	if err := preflightExistingState(c.RunDir, c.RuntimeFile); err != nil {
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
	if run.FinishTaskStatus != "waiting_decision" || run.Decision == nil {
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
	if err := preflightExistingState(c.RunDir, c.RuntimeFile); err != nil {
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

// decisionContinuation is the file the resumed agent reads. It restates the
// question and the human's answer so the same agent can continue in place.
func decisionContinuation(r *Run, ref ExecutionRef) string {
	var b strings.Builder
	b.WriteString("A human answered the blocking decision for this task. Continue the work now.\n\n")
	b.WriteString("Question:\n")
	b.WriteString(r.Decision.Request.Question)
	b.WriteString("\n\n")
	if r.Decision.OptionID != "" {
		b.WriteString("Chosen option: " + r.Decision.OptionID)
		for _, o := range r.Decision.Request.Options {
			if o.ID == r.Decision.OptionID {
				b.WriteString(" — " + o.Label)
			}
		}
		b.WriteString("\n")
	}
	if r.Decision.Note != "" {
		b.WriteString("Human note: " + r.Decision.Note + "\n")
	}
	answer, _ := ReadDecisionAnswer(r)
	b.WriteString("\nResume the original dispatch prompt, apply this decision, and finish the task. When done, overwrite `.steerline-result.json` in the worktree root with the required final JSON (outcome pr_submitted when a PR is published, or blocked with a clear reason).\n")
	b.WriteString("Only after applying the answer, include decision_application with these exact values in that result: decision_id=")
	b.WriteString(r.Decision.ID)
	b.WriteString(", attempt_id=")
	b.WriteString(r.Decision.AttemptID)
	b.WriteString(", execution_id=")
	b.WriteString(r.ExecutionID)
	b.WriteString(", execution_ref_sha256=")
	b.WriteString(digestValue(ref))
	b.WriteString(", answer_sha256=")
	b.WriteString(decisionAnswerDigest(answer))
	b.WriteString(". This is the application receipt; do not report it if the answer was not applied.\n")
	return b.String()
}

func decisionAnswerDigest(answer DecisionAnswer) string {
	return digestValue(struct{ OptionID, Note string }{strings.TrimSpace(answer.OptionID), strings.TrimSpace(answer.Note)})
}

func validateDecisionApplication(run *Run, result Result, ref ExecutionRef) error {
	if run == nil || run.Decision == nil || run.Decision.Status != "reply_accepted" {
		return errors.New("there is no accepted decision reply to verify")
	}
	evidence := result.DecisionApplication
	if evidence == nil {
		return errors.New("result has no decision application evidence")
	}
	answer, err := ReadDecisionAnswer(run)
	if err != nil {
		return fmt.Errorf("accepted answer is unavailable for verification: %w", err)
	}
	if evidence.DecisionID != run.Decision.ID || evidence.AttemptID != run.Decision.AttemptID || evidence.ExecutionID != run.ExecutionID || evidence.ExecutionRefHash != digestValue(ref) || evidence.AnswerHash != decisionAnswerDigest(answer) {
		return errors.New("decision application evidence does not match the original decision, attempt, execution binding and fixed answer")
	}
	return nil
}

func (s *Service) failUnverifiedDecisionApplication(run *Run, reason string) error {
	if run == nil || run.Decision == nil || run.Decision.Status != "reply_accepted" {
		return errors.New("run has no unverified accepted decision reply")
	}
	run.Decision.Status = "failed"
	run.Decision.Version++
	run.ExecutionStatus = "failed"
	run.FinishTaskStatus = "blocked"
	run.EndedAt = ptr(now())
	run.Error = ptr(clip("Decision application was not verified: "+reason, 2000))
	if s.State.CurrentTask != nil && *s.State.CurrentTask == run.TaskID {
		s.State.CurrentTask = nil
		for _, other := range s.State.Runs {
			if other != nil && other.RunID != run.RunID && (other.ExecutionStatus == "unknown" || other.ExecutionStatus == "pending" || other.ExecutionStatus == "running" || other.FinishTaskStatus == "waiting_decision") {
				s.State.CurrentTask = ptr(other.TaskID)
				break
			}
		}
	}
	s.closeActivity(run)
	s.note(run, "steer", "decision", "failed", "decision application was not verified", reason, decisionAnswerMeta(run))
	return s.save()
}

func (s *Service) requireDecisionReplyCapability(ctx context.Context, run *Run) error {
	return DecisionReplyEligibility(ctx, s.Config, run)
}

// DecisionReplyEligibility is the shared read-only qualification used by the
// CLI, console client and service handler. It checks the immutable execution
// binding and verified reply capabilities for this exact target.
func DecisionReplyEligibility(ctx context.Context, c Config, run *Run) error {
	if run == nil || run.Decision == nil || run.Decision.Status != "pending" {
		return errors.New("decision is not pending on the current run")
	}
	if run.AttemptID == "" || run.Decision.AttemptID != run.AttemptID {
		return &BackendError{Code: BackendIdentityMismatch, Diagnostic: "decision belongs to a different execution attempt"}
	}
	if run.Backend != string(BackendOrca) || run.Protocol != string(ProtocolOrcaTerminalV2) {
		return &BackendError{Code: BackendUnsupported, Diagnostic: "local continuation is unsupported; this decision cannot be answered and resumed on the local backend"}
	}
	records := executionRecords{root: c.RunDir}
	ref, err := records.loadBinding(run.RunID, run.AttemptID)
	if err != nil {
		return &BackendError{Code: BackendIdentityMismatch, Diagnostic: "decision execution has no valid fixed backend binding", Cause: err}
	}
	if ref.RunID != run.RunID || ref.AttemptID != run.AttemptID || ref.Backend != BackendOrca || ref.Protocol != ProtocolOrcaTerminalV2 || ref.Target.ID != run.Target.ID || ref.Target.ConnectionID != run.Target.ConnectionID || ref.Agent != (AgentSpec{Name: run.Agent}) || run.Process == nil || *run.Process != ref.External || ref.WorkspacePath != deref(run.WorktreePath) || ref.WorkspaceBranch != deref(run.WorktreeBranch) || ref.BaseCommit != deref(run.BaseCommit) {
		return &BackendError{Code: BackendIdentityMismatch, Diagnostic: "decision execution binding differs from the original run, attempt, target, workspace or process identity"}
	}
	backend := backendFor(c, BeadsContext{})
	kind, protocol := backend.Identity()
	if kind != ref.Backend || protocol != ref.Protocol {
		return &BackendError{Code: BackendIdentityMismatch, Diagnostic: "configured backend differs from the fixed decision execution"}
	}
	probe, err := backend.Probe(ctx, ref.Target, ref.Agent)
	if err != nil {
		return err
	}
	report, err := records.loadCapabilityReport(probe)
	if err != nil {
		status := CapabilityNotTested
		if !os.IsNotExist(err) {
			return err
		}
		return &BackendError{Code: BackendUnsupported, Diagnostic: fmt.Sprintf("reply/observe/output capabilities are %s for %s/%s target %s", status, ref.Backend, ref.Protocol, ref.Target.ID)}
	}
	for _, name := range []string{"reply_continue", "observe", "output_reading"} {
		if capabilityStatus(report, name) != CapabilityVerified {
			return &BackendError{Code: BackendUnsupported, Diagnostic: fmt.Sprintf("%s capability is %s for %s/%s target %s version %s", name, capabilityStatus(report, name), ref.Backend, ref.Protocol, ref.Target.ID, report.PlatformVersion)}
		}
	}
	return nil
}

// applyDecisions resumes runs whose decision a human answered. It runs on a tick
// and never runs alongside a worker: the single execution slot must be free.
func (s *Service) applyDecisions(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.worker != nil {
		return nil
	}
	if reason := s.executionSlotReason("", true); reason != "" {
		return errors.New(reason)
	}
	for _, r := range s.State.Runs {
		if s.worker != nil {
			// Single execution slot: one resumed decision at a time.
			break
		}
		if r.FinishTaskStatus != "waiting_decision" || r.Decision == nil || r.Decision.Status != "answer_saved" {
			continue
		}
		if r.AttemptID == "" || r.Decision.AttemptID != r.AttemptID {
			return &BackendError{Code: BackendIdentityMismatch, Diagnostic: "saved decision belongs to a different execution attempt; continuation refused"}
		}
		a, e := ReadDecisionAnswer(r)
		if e != nil {
			if os.IsNotExist(e) {
				continue
			}
			s.setError(fmt.Errorf("decision answer for %s ignored: %w", r.TaskID, e))
			if se := s.save(); se != nil {
				return se
			}
			continue
		}
		if r.Backend != string(BackendOrca) || r.Protocol != string(ProtocolOrcaTerminalV2) || r.WorkflowVersion == MinimalWorkflowVersion {
			reason := "decision continuation is unsupported for this backend/workflow; the saved answer was retained and no execution was started"
			if r.Error == nil || *r.Error != reason {
				r.Error = ptr(reason)
				s.note(r, "steer", "decision", "unsupported", "decision continuation is unsupported", reason, decisionAnswerMeta(r))
				if saveErr := s.save(); saveErr != nil {
					return saveErr
				}
			}
			continue
		}
		// Reconcile before consuming an answer so a newly discovered orphaned
		// claim cannot remain invisible until after a terminal was resumed.
		if e := s.reconcile(ctx); e != nil {
			return e
		}
		if reason := s.executionSlotReason("", true); reason != "" {
			return errors.New(reason)
		}
		if r.WorkflowVersion == MinimalWorkflowVersion {
			r.Decision.Status = "resuming"
			r.Decision.Version++
			if e := s.syncMinimalDecision(r); e != nil {
				return e
			}
			if e := s.save(); e != nil {
				return e
			}
			if e := s.resumeMinimalAfterDecision(ctx, r, a); e != nil {
				s.blocked(r, "Decision resume failed: "+e.Error())
				s.note(r, "steer", "decision", "failed", "decision resume failed", e.Error(), nil)
				if r.Decision.Status == "resuming" && r.ExecutionStatus != "unknown" {
					r.Decision.Status = "failed"
					r.Decision.Version++
					if syncErr := s.syncMinimalDecision(r); syncErr != nil {
						return errors.Join(e, syncErr)
					}
				}
				if r.ExecutionStatus == "unknown" {
					s.State.SlotOccupied, s.State.CurrentTask = true, ptr(r.TaskID)
				}
				s.closeActivity(r)
				if se := s.save(); se != nil {
					return se
				}
				continue
			}
			continue
		}
		r.Decision.Status = "resuming"
		r.Decision.Version++
		r.Decision.OptionID = a.OptionID
		r.Decision.Note = a.Note
		r.Decision.AnsweredBy = a.AnsweredBy
		answeredAt := a.AnsweredAt
		r.Decision.AnsweredAt = &answeredAt
		s.note(r, "steer", "decision", "answered", "decision answered", decisionAnswerSummary(r), decisionAnswerMeta(r))
		if e := s.save(); e != nil {
			return e
		}
		if e := s.resumeAfterDecision(ctx, r); e != nil {
			s.blocked(r, "Decision resume failed: "+e.Error())
			s.note(r, "steer", "decision", "failed", "decision resume failed", e.Error(), nil)
			if s.decisionResumeDefinitelyRejected(r) {
				r.Decision.Status = "failed"
				r.Decision.Version++
			}
			// The original terminal may still be alive even after a definite
			// reply rejection. Keep its execution slot reserved until recovery.
			r.ExecutionStatus = "unknown"
			s.State.SlotOccupied = true
			s.State.CurrentTask = ptr(r.TaskID)
			s.closeActivity(r)
			if se := s.save(); se != nil {
				return se
			}
			continue
		}
		r.Decision.Status = "reply_accepted"
		r.Decision.Version++
		r.ExecutionStatus = "running"
		r.EndedAt = nil
		r.Error = nil
		if e := s.save(); e != nil {
			return e
		}
		s.note(r, "steer", "decision", "reply_accepted", "decision reply accepted", decisionAnswerSummary(r), nil)
		if e := s.save(); e != nil {
			return e
		}
	}
	return nil
}

func (s *Service) decisionResumeDefinitelyRejected(run *Run) bool {
	if run == nil || run.Decision == nil {
		return false
	}
	op, err := (executionRecords{root: s.Config.RunDir}).loadOperation(run.RunID, run.Decision.AttemptID, "reply-"+run.Decision.ID)
	if os.IsNotExist(err) {
		return true // Failure occurred before a durable external-send intent.
	}
	if err != nil {
		return false
	}
	return op.Status == OperationPending || op.Status == OperationRejected
}

// resumeAfterDecision re-arms supervision of the fixed execution and sends a
// continuation only when the selected backend has verified support.
func (s *Service) resumeAfterDecision(ctx context.Context, r *Run) error {
	if r.Process == nil || r.Decision == nil {
		return &BackendError{Code: BackendIdentityMismatch, Diagnostic: "run has no complete decision/execution identity"}
	}
	if r.AttemptID == "" || r.Decision.AttemptID != r.AttemptID {
		return &BackendError{Code: BackendIdentityMismatch, Diagnostic: "decision belongs to a different execution attempt"}
	}
	if r.ResultPath == nil {
		return errors.New("run has no worktree or result path")
	}
	records := executionRecords{root: s.Config.RunDir}
	ref, e := records.loadBinding(r.RunID, r.AttemptID)
	if e != nil {
		if os.IsNotExist(e) {
			return &BackendError{Code: BackendIdentityMismatch, Diagnostic: "execution has no fixed target binding; manual recovery required"}
		}
		return e
	}
	backend := backendFor(s.Config, BeadsContext{})
	kind, protocol := backend.Identity()
	if ref.Backend != kind || ref.Protocol != protocol || ref.Target != targetFor(s.Config) || ref.External != *r.Process {
		return &BackendError{Code: BackendIdentityMismatch, Diagnostic: "fixed execution backend/protocol/target no longer matches the run"}
	}
	if _, e := s.requireBackendCapabilitiesFor(ctx, backend, ref.Target, ref.Agent, "reply_continue", "observe", "output_reading"); e != nil {
		return e
	}
	contPath := decisionContinuePath(r)
	if ref.WorkspacePath == "" || !filepath.IsAbs(ref.WorkspacePath) {
		return &BackendError{Code: BackendIdentityMismatch, Diagnostic: "fixed workspace identity is incomplete"}
	}
	opID := "reply-" + r.Decision.ID
	answer := "Read " + contPath + " and continue. Follow it exactly."
	continuation := decisionContinuation(r, ref)
	op := Operation{Version: ExecutionRecordVersion, ID: opID, RunID: r.RunID, AttemptID: r.AttemptID, Backend: ref.Backend, Protocol: ref.Protocol, Target: ref.Target, Action: "reply", PayloadDigest: digestValue(struct{ DecisionID, Pointer, Content string }{r.Decision.ID, answer, continuation}), Status: OperationPending, StartedAt: now(), UpdatedAt: now(), Ref: &ref}
	if old, loadErr := records.loadOperation(r.RunID, r.AttemptID, opID); loadErr == nil {
		if old.PayloadDigest != op.PayloadDigest {
			return errors.New("reply operation id conflicts with its fixed answer")
		}
		return fmt.Errorf("reply operation is already %s; refusing duplicate continuation", old.Status)
	} else if !os.IsNotExist(loadErr) {
		return loadErr
	}
	if e := records.saveOperation(op); e != nil {
		return e
	}
	if e := AtomicWrite(contPath, []byte(continuation)); e != nil {
		op.Status, op.Diagnostic, op.UpdatedAt = OperationRejected, e.Error(), now()
		if saveErr := records.saveOperation(op); saveErr != nil {
			return errors.Join(e, saveErr)
		}
		return e
	}
	op.Status, op.UpdatedAt = OperationInFlight, now()
	if e := records.saveOperation(op); e != nil {
		return e
	}
	reply, e := backend.Reply(ctx, ref, answer, op, *r.ResultPath)
	if e != nil {
		var backendErr *BackendError
		if errors.As(e, &backendErr) && backendErr.Code == BackendOutcomeUnknown {
			op.Status = OperationUnknown
		} else {
			op.Status = OperationRejected
		}
		op.Diagnostic, op.UpdatedAt = e.Error(), now()
		if saveErr := records.saveOperation(op); saveErr != nil {
			return errors.Join(e, saveErr)
		}
		return e
	}
	if !reply.InputAccepted || reply.Worker == nil {
		return &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca reply did not return an accepted input and observer"}
	}
	s.worker, s.active = reply.Worker, r
	s.State.SlotOccupied, s.State.CurrentTask = true, ptr(r.TaskID)
	r.StartedAt = ptr(now())
	op.Status, op.UpdatedAt = OperationAccepted, now()
	if e := records.saveOperation(op); e != nil {
		// Preserve the external session, but detach its local observer so an
		// unrecorded reply cannot silently advance business state.
		detachCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = backend.DetachObservation(detachCtx, reply.Worker)
		cancel()
		s.worker, s.active = nil, nil
		return &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "reply was accepted but its durable receipt could not be saved", Cause: e}
	}
	if reply.TurnStarted {
		if e := s.recordOrcaTurnStarted(r, ref, op, reply); e != nil {
			s.setError(fmt.Errorf("conversation turn receipt persistence failed: %w", e))
		}
	}
	return nil
}

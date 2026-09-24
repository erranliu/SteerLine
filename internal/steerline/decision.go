package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Decision protocol: when an agent cannot continue without a human decision it
// returns outcome=needs_decision with a structured question. The service persists
// runs/<run_id>/decision.request.json (authoritative) and pauses the run as
// finish_task_status=waiting_decision. A human answers by atomically writing
// runs/<run_id>/decision.answer.json via `steerline answer` or the viewer; the
// service is the only component that reads it back and resumes the run.
//
// Only the service writes runtime.json. Answerers write one immutable file.

const (
	decisionRequestFile = "decision.request.json"
	decisionAnswerFile  = "decision.answer.json"
	decisionContinueTxt = "continue.txt"
)

func decisionRequestPath(r *Run) string {
	return filepath.Join(filepath.Dir(*r.ResultPath), decisionRequestFile)
}

func decisionAnswerPath(r *Run) string {
	return filepath.Join(filepath.Dir(*r.ResultPath), decisionAnswerFile)
}

func decisionContinuePath(r *Run) string {
	return filepath.Join(filepath.Dir(*r.ResultPath), decisionContinueTxt)
}

func validateDecisionRequest(req *DecisionRequest) error {
	if req == nil {
		return errors.New("decision object is missing")
	}
	req.Question = clip(strings.TrimSpace(req.Question), 2000)
	req.Context = clip(strings.TrimSpace(req.Context), 8000)
	if req.Question == "" {
		return errors.New("decision question is empty")
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
		o.Label = clip(strings.TrimSpace(o.Label), 300)
		o.Detail = clip(strings.TrimSpace(o.Detail), 4000)
		switch {
		case o.ID == "":
			return errors.New("decision option id is empty")
		case o.Label == "":
			return fmt.Errorf("decision option %q has no label", o.ID)
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

// WriteDecisionRequest persists the authoritative request and links it to the run.
func WriteDecisionRequest(r *Run, req DecisionRequest) (*DecisionState, error) {
	if r.ResultPath == nil {
		return nil, errors.New("run has no result path for a decision")
	}
	if e := validateDecisionRequest(&req); e != nil {
		return nil, e
	}
	st := &DecisionState{
		ID:          newID(),
		Request:     req,
		Status:      "pending",
		RequestPath: decisionRequestPath(r),
		AnswerPath:  decisionAnswerPath(r),
	}
	b, e := json.MarshalIndent(st, "", "  ")
	if e != nil {
		return nil, e
	}
	if e := AtomicWrite(st.RequestPath, b); e != nil {
		return nil, e
	}
	return st, nil
}

// WriteAnswer validates a human answer against the run's pending decision and
// writes the answer file atomically. It never mutates runtime.json.
func WriteAnswer(r *Run, a DecisionAnswer) error {
	if r.Decision == nil {
		return errors.New("run has no decision to answer")
	}
	if r.Decision.Status != "pending" {
		return fmt.Errorf("decision is already %s", r.Decision.Status)
	}
	if a.DecisionID == "" {
		a.DecisionID = r.Decision.ID
	}
	if a.DecisionID != r.Decision.ID {
		return errors.New("answer does not match the pending decision id")
	}
	if a.RunID == "" {
		a.RunID = r.RunID
	}
	if a.RunID != r.RunID {
		return errors.New("answer run_id mismatch")
	}
	a.AnsweredBy = clip(strings.TrimSpace(a.AnsweredBy), 200)
	a.Note = clip(strings.TrimSpace(a.Note), 4000)
	if a.AnsweredAt.IsZero() {
		a.AnsweredAt = now()
	}
	if e := validateDecisionAnswer(&r.Decision.Request, &a); e != nil {
		return e
	}
	b, e := json.MarshalIndent(a, "", "  ")
	if e != nil {
		return e
	}
	if e := os.MkdirAll(filepath.Dir(r.Decision.AnswerPath), 0700); e != nil {
		return e
	}
	return AtomicWrite(r.Decision.AnswerPath, b)
}

func validateDecisionAnswer(req *DecisionRequest, a *DecisionAnswer) error {
	if a.AnsweredBy == "" {
		return errors.New("answer requires answered_by")
	}
	if len(req.Options) > 0 {
		if a.OptionID == "" {
			return errors.New("answer requires option_id")
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

// ReadDecisionAnswer reads the answer file if it exists and validates it against
// the persisted request. A missing file returns an error satisfying
// os.IsNotExist so callers can treat "not answered yet" separately.
func ReadDecisionAnswer(r *Run) (DecisionAnswer, error) {
	if r.Decision == nil {
		return DecisionAnswer{}, errors.New("run has no decision")
	}
	b, e := os.ReadFile(r.Decision.AnswerPath)
	if e != nil {
		return DecisionAnswer{}, e
	}
	var a DecisionAnswer
	if e := json.Unmarshal(b, &a); e != nil {
		return DecisionAnswer{}, e
	}
	if a.DecisionID != r.Decision.ID {
		return DecisionAnswer{}, errors.New("answer does not match the pending decision id")
	}
	if a.AnsweredBy == "" {
		a.AnsweredBy = "unknown"
	}
	if e := validateDecisionAnswer(&r.Decision.Request, &a); e != nil {
		return DecisionAnswer{}, e
	}
	return a, nil
}

// AnswerOptions is the CLI/viewer entry point for answering a pending decision.
type AnswerOptions struct {
	RunID      string
	DecisionID string
	Option     string
	Note       string
	By         string
}

// AnswerDecision finds a run waiting for a decision and writes its answer file.
// It is safe to run while the service is live: it never edits runtime.json.
func AnswerDecision(c Config, o AnswerOptions) error {
	if strings.TrimSpace(o.RunID) == "" {
		return errors.New("answer requires a run id or task id")
	}
	snapshot, e := LoadRuntime(filepath.Join(c.RunDir, c.RuntimeFile))
	if e != nil {
		return e
	}
	if snapshot == nil {
		return errors.New("no runtime snapshot")
	}
	var run *Run
	for _, r := range snapshot.Runs {
		if r != nil && (r.RunID == o.RunID || r.TaskID == o.RunID) {
			run = r
			break
		}
	}
	if run == nil {
		return fmt.Errorf("run %s not found", o.RunID)
	}
	if run.FinishTaskStatus != "waiting_decision" || run.Decision == nil {
		return fmt.Errorf("run %s is not waiting for a decision (status %s)", run.TaskID, run.FinishTaskStatus)
	}
	if o.By == "" {
		o.By = answererName()
	}
	return WriteAnswer(run, DecisionAnswer{
		DecisionID: o.DecisionID,
		RunID:      run.RunID,
		OptionID:   strings.TrimSpace(o.Option),
		Note:       o.Note,
		AnsweredBy: o.By,
		AnsweredAt: now(),
	})
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
func decisionContinuation(r *Run) string {
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
	b.WriteString("\nResume the original dispatch prompt, apply this decision, and finish the task. When done, overwrite `.steerline-result.json` in the worktree root with the required final JSON (outcome pr_submitted when a PR is published, or blocked with a clear reason).\n")
	return b.String()
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
		if r.FinishTaskStatus != "waiting_decision" || r.Decision == nil || r.Decision.Status != "pending" {
			continue
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
		// Reconcile before consuming an answer so a newly discovered orphaned
		// claim cannot remain invisible until after a terminal was resumed.
		if e := s.reconcile(ctx); e != nil {
			return e
		}
		if reason := s.executionSlotReason("", true); reason != "" {
			return errors.New(reason)
		}
		r.Decision.Status = "resuming"
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
			var backendErr *BackendError
			if errors.As(e, &backendErr) && (backendErr.Code == BackendOutcomeUnknown || backendErr.Code == BackendIdentityMismatch || backendErr.Code == BackendUnreachable || backendErr.Code == BackendProtocolInvalid) {
				r.ExecutionStatus = "unknown"
				s.State.SlotOccupied = true
				s.State.CurrentTask = ptr(r.TaskID)
			} else {
				s.State.SlotOccupied = false
				s.State.CurrentTask = nil
			}
			s.closeActivity(r)
			if se := s.save(); se != nil {
				return se
			}
			continue
		}
		r.Decision.Status = "applied"
		r.Decision.AppliedAt = ptr(now())
		r.ExecutionStatus = "running"
		r.EndedAt = nil
		r.Error = nil
		if e := s.save(); e != nil {
			return e
		}
		s.note(r, "steer", "decision", "applied", "decision applied", decisionAnswerSummary(r), nil)
		if e := s.save(); e != nil {
			return e
		}
	}
	return nil
}

// resumeAfterDecision re-arms supervision of the fixed execution and sends a
// continuation only when the selected backend has verified support.
func (s *Service) resumeAfterDecision(ctx context.Context, r *Run) error {
	if r.Process == nil || r.Decision == nil {
		return &BackendError{Code: BackendIdentityMismatch, Diagnostic: "run has no complete decision/execution identity"}
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
	op := Operation{Version: ExecutionRecordVersion, ID: opID, RunID: r.RunID, AttemptID: r.AttemptID, Backend: ref.Backend, Protocol: ref.Protocol, Target: ref.Target, Action: "reply", PayloadDigest: digestValue(struct{ DecisionID, Answer string }{r.Decision.ID, answer}), Status: OperationPending, StartedAt: now(), UpdatedAt: now()}
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
	op.Status, op.UpdatedAt = OperationInFlight, now()
	if e := records.saveOperation(op); e != nil {
		return e
	}
	if e := AtomicWrite(contPath, []byte(decisionContinuation(r))); e != nil {
		op.Status, op.Diagnostic, op.UpdatedAt = OperationRejected, e.Error(), now()
		if saveErr := records.saveOperation(op); saveErr != nil {
			return errors.Join(e, saveErr)
		}
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
	return nil
}

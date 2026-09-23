package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
			s.State.SlotOccupied = false
			s.State.CurrentTask = nil
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

// resumeAfterDecision re-arms supervision of the same Orca terminal and sends the
// continuation. It refuses to act unless the terminal is the same live
// incarnation, so a restart or a replaced terminal never silently resumes.
func (s *Service) resumeAfterDecision(ctx context.Context, r *Run) error {
	if s.Config.hostName() != "orca" {
		return errors.New("decision resume is only supported for the orca host")
	}
	if r.Process == nil || r.Process.Kind != "orca" || r.Process.Terminal == "" {
		return errors.New("run has no recorded orca terminal")
	}
	if r.WorktreePath == nil || r.ResultPath == nil {
		return errors.New("run has no worktree or result path")
	}
	client := orcaClient{cfg: s.Config}
	term, e := client.showTerminal(ctx, r.Process.Terminal)
	if e != nil {
		return e
	}
	if term.Orphaned || !term.Connected {
		return errors.New("orca terminal is not live")
	}
	if term.IncarnationID != "" && r.Process.Incarnation != "" && term.IncarnationID != r.Process.Incarnation {
		return errors.New("orca terminal incarnation changed; refusing to resume")
	}
	contPath := decisionContinuePath(r)
	if e := AtomicWrite(contPath, []byte(decisionContinuation(r))); e != nil {
		return e
	}
	resultInWorktree := filepath.Join(*r.WorktreePath, orcaResultFile)
	// Drop the answered result so supervision cannot mistake it for the new one.
	_ = os.Remove(resultInWorktree)
	if e := client.terminalSend(ctx, r.Process.Terminal, "Read "+contPath+" and continue. Follow it exactly."); e != nil {
		return e
	}
	proc := &orcaProcess{cfg: s.Config, term: term}
	proc.term.Handle = r.Process.Terminal
	s.worker = superviseOrcaResult(s.Config, r, client, proc, resultInWorktree, *r.ResultPath)
	s.active = r
	s.State.SlotOccupied = true
	s.State.CurrentTask = ptr(r.TaskID)
	r.StartedAt = ptr(now())
	return nil
}

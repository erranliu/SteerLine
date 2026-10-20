// Package runnerbridge owns the caller's durable candidate-to-Run association
// and CLI outbox. Native execution state remains exclusively in SteerRunner.
package runnerbridge

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnerstate"
)

type Action = runnerstate.Action

type InputError struct{ Code, Message string }

func (e *InputError) Error() string         { return e.Code + ": " + e.Message }
func inputError(code, message string) error { return &InputError{Code: code, Message: message} }

type Candidate = runnerstate.Candidate

type Store interface {
	// Candidate returns storage/ports.ErrNotFound when the binding is absent.
	Candidate(context.Context, string, string) (Candidate, error)
	// Save must atomically CAS the association, projection, event IDs and cursor.
	// Claim/reservation and Task-terminal stop intents are separate task-owned
	// transactions against this same store, before any CLI invocation.
	SaveCandidate(context.Context, Candidate, uint64, []runnerclient.Event) error
}
type CLI interface {
	Submit(context.Context, runnerclient.Request) (runnerclient.Receipt, error)
	Inspect(context.Context, string, string, bool) (runnerclient.Inspection, error)
	Events(context.Context, string, string) (runnerclient.EventPage, error)
	Call(context.Context, string, []string, *runnerclient.Request) (runnerclient.Response, error)
}
type Bridge struct {
	Store  Store
	Client func(string) CLI
	locks  sync.Map
}

// WithCandidate serializes caller withdrawal with the exact candidate's dispatch.
func (b *Bridge) WithCandidate(project, id string, commit func() error) error {
	entry, _ := b.locks.LoadOrStore(project+"\x00"+id, &sync.Mutex{})
	gate := entry.(*sync.Mutex)
	gate.Lock()
	defer gate.Unlock()
	return commit()
}

func newID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(raw[:])
}
func known(err error, code string) bool {
	var f *runnerclient.Failure
	return errors.As(err, &f) && !f.Unknown && f.Code == code
}
func (b *Bridge) save(ctx context.Context, c *Candidate, events ...runnerclient.Event) error {
	if len(events) == 0 {
		current, err := b.Store.Candidate(ctx, c.Project, c.ID)
		if err != nil {
			return err
		}
		if current.Revision != c.Revision {
			return errors.New("candidate changed before CLI checkpoint")
		}
		a, b := current, *c
		a.Revision, b.Revision = 0, 0
		x, _ := json.Marshal(a)
		y, _ := json.Marshal(b)
		if string(x) == string(y) {
			return nil
		}
	}
	previous := c.Revision
	c.Revision++
	return b.Store.SaveCandidate(ctx, *c, previous, events)
}

// Step advances a bounded amount of caller bookkeeping. It never recreates a
// native session or generates a workflow prompt, answer or resume.
func (b *Bridge) Step(ctx context.Context, project, id string) error {
	key := project + "\x00" + id
	entry, _ := b.locks.LoadOrStore(key, &sync.Mutex{})
	gate := entry.(*sync.Mutex)
	gate.Lock()
	defer gate.Unlock()
	c, err := b.Store.Candidate(ctx, project, id)
	if err != nil {
		return err
	}
	// Manual withdrawal freezes caller observation and grants no resource action.
	// Task termination can later add its own native stop obligation.
	if c.Participation != "active" && !c.StopRequested {
		return nil
	}
	cli := b.Client(c.StateDir)
	if c.RunID == "" {
		if c.SubmitState == "rejected" || c.SubmitState == "cancelled" {
			return nil
		}
		if c.SubmitState == "pending" && c.StopRequested {
			c.SubmitState = "cancelled"
			return b.save(ctx, &c)
		}
		if c.SubmitState == "pending" {
			c.SubmitState = "unknown"
			if err := b.save(ctx, &c); err != nil {
				return err
			}
		}
		original, lookup := cli.Inspect(ctx, "request-id", c.Submit.RequestID, false)
		if lookup == nil {
			if original.Command == nil || original.Command.Command != "submit" || original.Run == nil {
				return errors.New("original submission identity is incomplete")
			}
			c.RunID, c.SubmitCommandID, c.SubmitState = original.Run.RunID, original.Command.CommandID, "accepted"
			if err := bind(&c, *original.Run); err != nil {
				return err
			}
			if err := b.save(ctx, &c); err != nil {
				return err
			}
		} else {
			if !known(lookup, "request_not_found") {
				return lookup
			}
			// A terminal task may still acquire a late accepted Run. Continue
			// querying its original request, never create new work after withdrawal.
			if c.StopRequested || c.SubmitObserveOnly {
				return nil
			}
			receipt, submitErr := cli.Submit(ctx, c.Submit)
			if submitErr != nil {
				var failure *runnerclient.Failure
				if errors.As(submitErr, &failure) && !failure.Unknown {
					c.SubmitState = "rejected"
					c.Diagnostic = failure.Code + ": " + failure.Message
					if err := b.save(ctx, &c); err != nil {
						return err
					}
				}
				return submitErr
			}
			c.RunID, c.SubmitCommandID, c.SubmitState = receipt.RunID, receipt.CommandID, "accepted"
			if err := b.save(ctx, &c); err != nil {
				return err
			}
		}
	}
	inspection, err := cli.Inspect(ctx, "run-id", c.RunID, false)
	if err != nil {
		return err
	}
	if inspection.Run == nil {
		return errors.New("bound Runner Run is missing")
	}
	if err := bind(&c, *inspection.Run); err != nil {
		return err
	}
	if c.StopRequested {
		if err := b.prepareStop(ctx, &c, inspection.Run.Revision, newID); err != nil {
			return err
		}
		if c.Stop.State == "unknown" {
			if err := b.deliver(ctx, cli, &c, c.Stop); err != nil {
				return err
			}
		}
		if c.Stop.State == "accepted" {
			c.InputsRevoked = true
		}
		// A later explicit external resume cannot erase the task's terminal
		// stop obligation. A known completed old stop may be superseded.
	}
	for i := range c.Actions {
		action := &c.Actions[i]
		if action.State == "accepted" && action.ActionStatus != "completed" && action.ActionStatus != "failed" && action.ActionStatus != "rejected" && action.ActionStatus != "cancelled" {
			observed, err := cli.Inspect(ctx, "request-id", action.Request.RequestID, false)
			if err != nil {
				return err
			}
			if observed.Command == nil || observed.Command.Command != action.Command || observed.Command.RunID == nil || *observed.Command.RunID != c.RunID || observed.Command.CommandID != action.CommandID {
				return errors.New("accepted input identity changed during observation")
			}
			action.ActionStatus = observed.Command.ActionStatus
		}
		if c.StopRequested && action.State == "pending" {
			action.State = "cancelled"
			continue
		}
		if action.State == "pending" {
			action.State = "unknown"
			if err := b.save(ctx, &c); err != nil {
				return err
			}
		}
		if action.State == "unknown" {
			if c.StopRequested || action.ObserveOnly {
				// An old reply/resume can only be observed after task withdrawal.
				result, err := cli.Inspect(ctx, "request-id", action.Request.RequestID, false)
				if known(err, "request_not_found") {
					continue
				}
				if err != nil {
					return err
				}
				if result.Command == nil || result.Command.RunID == nil || *result.Command.RunID != c.RunID || result.Command.Command != action.Command {
					return errors.New("old action identity changed")
				}
				action.State, action.CommandID, action.ActionStatus = "accepted", result.Command.CommandID, result.Command.ActionStatus
			} else if err := b.deliver(ctx, cli, &c, action); err != nil {
				return err
			}
		}
	}
	page, err := cli.Events(ctx, c.RunID, c.Cursor)
	if err != nil {
		return err
	}
	c.Cursor = page.NextCursor
	latest, err := cli.Inspect(ctx, "run-id", c.RunID, false)
	if err != nil {
		return err
	}
	if latest.Run == nil {
		return errors.New("bound Run disappeared")
	}
	if err := bind(&c, *latest.Run); err != nil {
		return err
	}
	for _, event := range page.Events {
		if event.Revision > latest.Run.Revision {
			return errors.New("Runner events exceed its observed projection")
		}
	}
	if c.InputsRevoked && latest.Run.NewInputsAllowed {
		c.InputsRevoked = false
		c.Stop = nil
	}
	return b.save(ctx, &c, page.Events...)
}

// RecoverInputs withdraws unused caller authorizations at a service boundary.
// Unknown requests remain queryable by their original ID, without resending.
func (b *Bridge) RecoverInputs(ctx context.Context, project, id string) error {
	entry, _ := b.locks.LoadOrStore(project+"\x00"+id, &sync.Mutex{})
	gate := entry.(*sync.Mutex)
	gate.Lock()
	defer gate.Unlock()
	c, err := b.Store.Candidate(ctx, project, id)
	if err != nil {
		return err
	}
	if c.RunID == "" {
		if c.SubmitState == "pending" {
			c.SubmitState = "cancelled"
		}
		if c.SubmitState == "unknown" {
			c.SubmitObserveOnly = true
		}
	}
	for i := range c.Actions {
		if c.Actions[i].State == "pending" {
			c.Actions[i].State = "cancelled"
		}
		if c.Actions[i].State == "unknown" {
			c.Actions[i].ObserveOnly = true
		}
	}
	return b.save(ctx, &c)
}

// VerifyBinding checks original public facts before the reconstructed runtime
// opens admission. It never submits, resumes, stops or refreshes a PR.
func (b *Bridge) VerifyBinding(ctx context.Context, project, id string) error {
	c, err := b.Store.Candidate(ctx, project, id)
	if err != nil {
		return err
	}
	if c.Participation != "active" && !c.StopRequested {
		return nil
	}
	cli := b.Client(c.StateDir)
	if c.RunID != "" {
		observed, err := cli.Inspect(ctx, "run-id", c.RunID, false)
		if err != nil {
			return err
		}
		if observed.Run == nil || observed.Run.InputDigest == "" {
			return errors.New("original Runner Run/input digest is missing during startup")
		}
		return bind(&c, *observed.Run)
	}
	if c.SubmitState != "unknown" {
		return nil
	}
	observed, err := cli.Inspect(ctx, "request-id", c.Submit.RequestID, false)
	if known(err, "request_not_found") {
		return nil
	}
	if err != nil {
		return err
	}
	command := observed.Command
	if command == nil || command.RequestID != c.Submit.RequestID || command.Command != "submit" || command.CommandID == "" || command.RunID == nil || observed.Run == nil || *command.RunID != observed.Run.RunID || observed.Run.InputDigest == "" {
		return errors.New("original submit identity is incomplete during startup")
	}
	c.RunID = observed.Run.RunID
	return bind(&c, *observed.Run)
}

func bind(c *Candidate, r runnerclient.Run) error {
	if project := r.Correlation["steerline_project"]; project != "" && project != c.Project {
		return errors.New("Runner Run belongs to another project")
	}
	if r.RunID != c.RunID || c.InputDigest != "" && c.InputDigest != r.InputDigest || c.Branch != "" && c.Branch != r.Execution.Branch || c.PRURL != "" && (r.PR == nil || c.PRURL != r.PR.URL) {
		return errors.New("Runner projection changed the fixed candidate binding")
	}
	if c.Projection != nil && r.Revision < c.Projection.Revision {
		return errors.New("Runner projection revision moved backwards")
	}
	c.InputDigest, c.Branch = r.InputDigest, r.Execution.Branch
	if r.PR != nil {
		c.PRURL = r.PR.URL
	}
	c.Projection = &r
	return nil
}
func (b *Bridge) deliver(ctx context.Context, cli CLI, c *Candidate, action *Action) error {
	inspection, err := cli.Inspect(ctx, "request-id", action.Request.RequestID, false)
	if err == nil {
		if inspection.Command == nil || inspection.Command.Command != action.Command || inspection.Command.RunID == nil || *inspection.Command.RunID != c.RunID {
			return errors.New("original control belongs to another Run")
		}
		action.State, action.CommandID, action.ActionStatus = "accepted", inspection.Command.CommandID, inspection.Command.ActionStatus
		return b.save(ctx, c)
	}
	if !known(err, "request_not_found") {
		return err
	}
	response, err := cli.Call(ctx, action.Command, []string{"--run-id", c.RunID}, &action.Request)
	if err != nil {
		var failure *runnerclient.Failure
		if errors.As(err, &failure) && !failure.Unknown && failure.Exit != 4 {
			action.State = "rejected"
			action.ErrorCode = failure.Code
			if saveErr := b.save(ctx, c); saveErr != nil {
				return saveErr
			}
		}
		return err
	}
	var receipt runnerclient.Receipt
	if json.Unmarshal(response.Data, &receipt) != nil || receipt.RunID != c.RunID || receipt.CommandID == "" || receipt.ReceiptStatus != "accepted" {
		return &runnerclient.Failure{Code: "request_outcome_unknown", Message: fmt.Sprintf("Inspect original %s request", action.Command), Exit: 6, Unknown: true}
	}
	action.State, action.CommandID, action.ActionStatus = "accepted", receipt.CommandID, receipt.ActionStatus
	return b.save(ctx, c)
}

// QueueControl freezes an explicit current user reply or resume before it can
// cross the process boundary. Restarting Step never manufactures these inputs.
func (b *Bridge) QueueControl(ctx context.Context, project, id, command string, request runnerclient.Request, callerDigest string) error {
	if command != "reply" && command != "resume" {
		return inputError("invalid_request", "unsupported candidate control")
	}
	entry, _ := b.locks.LoadOrStore(project+"\x00"+id, &sync.Mutex{})
	gate := entry.(*sync.Mutex)
	gate.Lock()
	defer gate.Unlock()
	c, err := b.Store.Candidate(ctx, project, id)
	if err != nil {
		return err
	}
	callerID := request.RequestID
	if callerID == "" || callerDigest == "" {
		return inputError("invalid_request", "original product request identity is required")
	}
	for _, action := range c.Actions {
		if action.CallerRequestID == callerID {
			if action.Command == command && action.CallerRequestDigest == callerDigest {
				return nil
			}
			return inputError("idempotency_conflict", "control request identity conflict")
		}
	}
	if c.Participation != "active" || c.StopRequested || c.RunID == "" || c.Projection == nil {
		return inputError("candidate_inactive", "candidate cannot accept new user input")
	}
	if request.Protocol != runnerclient.Protocol || request.RequestID == "" || !json.Valid(request.Input) {
		return inputError("invalid_request", "invalid frozen control request")
	}
	var revision struct {
		Expected uint64 `json:"expected_revision"`
	}
	if json.Unmarshal(request.Input, &revision) != nil || revision.Expected != c.Projection.Revision {
		return inputError("stale_version", "control requires the current observed Run revision")
	}
	for _, action := range c.Actions {
		if action.State == "pending" || action.State == "unknown" {
			return inputError("input_reconciliation_required", "an original control still requires reconciliation")
		}
	}
	if command == "resume" && !c.Projection.Controls.CanResume || command == "reply" && !c.Projection.Controls.CanReply {
		return inputError("candidate_inactive", "current Run does not permit this control")
	}
	request.RequestID = newID()
	c.Actions = append(c.Actions, Action{CallerRequestID: callerID, CallerRequestDigest: callerDigest, Command: command, Request: request, State: "pending"})
	return b.save(ctx, &c)
}

func (b *Bridge) RefreshPR(ctx context.Context, project, id string) (Candidate, time.Time, error) {
	return b.refreshPR(ctx, project, id, true)
}

// ObservePR is an explicit read-only provider refresh, including a withdrawn
// candidate. It grants no delivery participation or native input authority.
func (b *Bridge) ObservePR(ctx context.Context, project, id string) (Candidate, time.Time, error) {
	return b.refreshPR(ctx, project, id, false)
}
func (b *Bridge) refreshPR(ctx context.Context, project, id string, delivery bool) (Candidate, time.Time, error) {
	entry, _ := b.locks.LoadOrStore(project+"\x00"+id, &sync.Mutex{})
	gate := entry.(*sync.Mutex)
	gate.Lock()
	defer gate.Unlock()
	c, err := b.Store.Candidate(ctx, project, id)
	if err != nil {
		return c, time.Time{}, err
	}
	if c.RunID == "" || delivery && (c.StopRequested || c.Participation != "active") {
		return c, time.Time{}, errors.New("candidate cannot verify a new delivery")
	}
	started := time.Now().UTC()
	result, err := b.Client(c.StateDir).Inspect(ctx, "run-id", c.RunID, true)
	if err != nil {
		return c, started, err
	}
	if result.Run == nil {
		return c, started, errors.New("original Run is missing")
	}
	if err := bind(&c, *result.Run); err != nil {
		return c, started, err
	}
	err = b.save(ctx, &c)
	return c, started, err
}

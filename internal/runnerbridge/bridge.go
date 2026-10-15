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
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"sync"
	"time"
)

type Action struct {
	ObserveOnly  bool                 `json:"observe_only,omitempty"`
	Command      string               `json:"command"`
	Request      runnerclient.Request `json:"request"`
	State        string               `json:"state"`
	CommandID    string               `json:"command_id,omitempty"`
	ErrorCode    string               `json:"error_code,omitempty"`
	ActionStatus string               `json:"action_status,omitempty"`
}
type Candidate struct {
	StartDigest     string               `json:"start_digest"`
	ProfileID       string               `json:"profile_id"`
	CreatedAt       time.Time            `json:"created_at"`
	Explicit        bool                 `json:"explicit"`
	Number          int                  `json:"number"`
	Project         string               `json:"project"`
	TaskID          string               `json:"task_id"`
	Claimant        string               `json:"claimant"`
	ContractDigest  string               `json:"contract_digest"`
	BaseRevision    string               `json:"base_revision"`
	Participation   string               `json:"participation"`
	ID              string               `json:"id"`
	Revision        uint64               `json:"revision"`
	StateDir        string               `json:"state_dir"`
	Submit          runnerclient.Request `json:"submit"`
	SubmitState     string               `json:"submit_state"`
	SubmitCommandID string               `json:"submit_command_id,omitempty"`
	RunID           string               `json:"run_id,omitempty"`
	InputDigest     string               `json:"input_digest,omitempty"`
	Branch          string               `json:"branch,omitempty"`
	PRURL           string               `json:"pr_url,omitempty"`
	Cursor          string               `json:"cursor,omitempty"`
	Projection      *runnerclient.Run    `json:"projection,omitempty"`
	StopRequested   bool                 `json:"stop_requested"`
	InputsRevoked   bool                 `json:"inputs_revoked"`
	Stop            *Action              `json:"stop,omitempty"`
	Actions         []Action             `json:"actions,omitempty"`
	Diagnostic      string               `json:"diagnostic,omitempty"`
}
type Store interface {
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
			if c.StopRequested {
				return nil
			}
			receipt, submitErr := cli.Submit(ctx, c.Submit)
			if submitErr != nil {
				var failure *runnerclient.Failure
				if errors.As(submitErr, &failure) && !failure.Unknown {
					c.SubmitState = "rejected"
					c.Diagnostic = failure.Code
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
		if c.Stop == nil || c.Stop.State == "rejected" {
			body, _ := json.Marshal(map[string]any{"expected_revision": inspection.Run.Revision})
			c.Stop = &Action{Command: "stop", Request: runnerclient.Request{Protocol: runnerclient.Protocol, RequestID: newID(), Input: body}, State: "unknown"}
			if err := b.save(ctx, &c); err != nil {
				return err
			}
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
func bind(c *Candidate, r runnerclient.Run) error {
	if r.RunID != c.RunID || c.InputDigest != "" && c.InputDigest != r.InputDigest || c.Branch != "" && c.Branch != r.Execution.Branch || c.PRURL != "" && r.PR != nil && c.PRURL != r.PR.URL {
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
func (b *Bridge) QueueControl(ctx context.Context, project, id, command string, request runnerclient.Request) error {
	if command != "reply" && command != "resume" {
		return errors.New("unsupported candidate control")
	}
	entry, _ := b.locks.LoadOrStore(project+"\x00"+id, &sync.Mutex{})
	gate := entry.(*sync.Mutex)
	gate.Lock()
	defer gate.Unlock()
	c, err := b.Store.Candidate(ctx, project, id)
	if err != nil {
		return err
	}
	if c.StopRequested || c.RunID == "" || c.Projection == nil {
		return errors.New("candidate cannot accept new user input")
	}
	if request.Protocol != runnerclient.Protocol || request.RequestID == "" || !json.Valid(request.Input) {
		return errors.New("invalid frozen control request")
	}
	var revision struct {
		Expected uint64 `json:"expected_revision"`
	}
	if json.Unmarshal(request.Input, &revision) != nil || revision.Expected != c.Projection.Revision {
		return errors.New("control requires the current observed Run revision")
	}
	for _, action := range c.Actions {
		if action.Request.RequestID == request.RequestID {
			if action.Command == command && string(action.Request.Input) == string(request.Input) {
				return nil
			}
			return errors.New("control request identity conflict")
		}
		if action.State == "pending" || action.State == "unknown" {
			return errors.New("an original control still requires reconciliation")
		}
	}
	if command == "resume" && !c.Projection.Controls.CanResume || command == "reply" && !c.Projection.Controls.CanReply {
		return errors.New("current Run does not permit this control")
	}
	c.Actions = append(c.Actions, Action{Command: command, Request: request, State: "pending"})
	return b.save(ctx, &c)
}

func (b *Bridge) RefreshPR(ctx context.Context, project, id string) (Candidate, time.Time, error) {
	entry, _ := b.locks.LoadOrStore(project+"\x00"+id, &sync.Mutex{})
	gate := entry.(*sync.Mutex)
	gate.Lock()
	defer gate.Unlock()
	c, err := b.Store.Candidate(ctx, project, id)
	if err != nil {
		return c, time.Time{}, err
	}
	if c.RunID == "" || c.StopRequested || c.Participation != "active" {
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

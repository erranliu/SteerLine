package runnerbridge

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"testing"
)

type memoryStore struct {
	value  Candidate
	fail   bool
	events map[string]bool
}

func clone(c Candidate) Candidate {
	raw, _ := json.Marshal(c)
	var result Candidate
	_ = json.Unmarshal(raw, &result)
	return result
}
func (s *memoryStore) Candidate(context.Context, string, string) (Candidate, error) {
	return clone(s.value), nil
}
func (s *memoryStore) SaveCandidate(_ context.Context, c Candidate, expected uint64, events []runnerclient.Event) error {
	if s.fail || s.value.Revision != expected {
		return errors.New("commit rejected")
	}
	s.value = clone(c)
	for _, event := range events {
		s.events[event.EventID] = true
	}
	return nil
}

type fixtureCLI struct {
	run                    runnerclient.Run
	submits, stops, inputs int
	lostSubmit             bool
	reads                  int
	commands               map[string]runnerclient.Command
}

func (f *fixtureCLI) Submit(_ context.Context, r runnerclient.Request) (runnerclient.Receipt, error) {
	f.submits++
	copy := f.run.RunID
	f.commands[r.RequestID] = runnerclient.Command{CommandID: "submit-command", RequestID: r.RequestID, RunID: &copy, Command: "submit", ReceiptStatus: "accepted", ActionStatus: "pending"}
	if f.lostSubmit {
		return runnerclient.Receipt{}, &runnerclient.Failure{Code: "request_outcome_unknown", Exit: 6, Unknown: true}
	}
	return runnerclient.Receipt{RunID: f.run.RunID, CommandID: "submit-command", ReceiptStatus: "accepted", ActionStatus: "pending"}, nil
}
func (f *fixtureCLI) Inspect(_ context.Context, selector, id string, _ bool) (runnerclient.Inspection, error) {
	f.reads++
	copy := f.run
	if selector == "run-id" {
		return runnerclient.Inspection{Run: &copy}, nil
	}
	command, ok := f.commands[id]
	if !ok {
		return runnerclient.Inspection{}, &runnerclient.Failure{Code: "request_not_found", Exit: 5}
	}
	return runnerclient.Inspection{Command: &command, Run: &copy}, nil
}
func (f *fixtureCLI) Events(context.Context, string, string) (runnerclient.EventPage, error) {
	return runnerclient.EventPage{NextCursor: "cursor", Events: []runnerclient.Event{{EventID: "stable", RunID: f.run.RunID, Revision: 1, Type: "accepted"}}}, nil
}
func (f *fixtureCLI) Call(_ context.Context, command string, _ []string, r *runnerclient.Request) (runnerclient.Response, error) {
	if command == "stop" {
		f.stops++
		f.run.NewInputsAllowed = false
		f.run.StopStatus = "unsupported"
		f.run.Revision++
	} else {
		f.inputs++
	}
	copy := f.run.RunID
	f.commands[r.RequestID] = runnerclient.Command{CommandID: "control-command", RequestID: r.RequestID, RunID: &copy, Command: command, ReceiptStatus: "accepted", ActionStatus: "failed"}
	body, _ := json.Marshal(runnerclient.Receipt{RunID: copy, CommandID: "control-command", ReceiptStatus: "accepted", ActionStatus: "failed"})
	return runnerclient.Response{Protocol: runnerclient.Protocol, OK: true, Data: body}, nil
}
func setup() (*Bridge, *memoryStore, *fixtureCLI) {
	s := &memoryStore{value: Candidate{Project: "project", TaskID: "1", ID: "candidate", Revision: 1, Participation: "active", StateDir: "state", SubmitState: "pending", Submit: runnerclient.Request{Protocol: runnerclient.Protocol, RequestID: "fixed-submit", Input: json.RawMessage(`{"spec":"fixed"}`)}}, events: map[string]bool{}}
	f := &fixtureCLI{run: runnerclient.Run{RunID: "run", Revision: 1, InputDigest: "input", NewInputsAllowed: true, Execution: runnerclient.Execution{Branch: "branch"}}, commands: map[string]runnerclient.Command{}}
	return &Bridge{Store: s, Client: func(string) CLI { return f }}, s, f
}
func TestSteerRunnerOutboxBoundaries(t *testing.T) {
	t.Run("restart withdraws unused authorizations", func(t *testing.T) {
		b, s, f := setup()
		s.value.RunID, s.value.SubmitState = "run", "accepted"
		s.value.Actions = []Action{
			{Command: "reply", State: "pending", Request: runnerclient.Request{Protocol: runnerclient.Protocol, RequestID: "old-reply", Input: json.RawMessage(`{"expected_revision":1}`)}},
			{Command: "resume", State: "unknown", Request: runnerclient.Request{Protocol: runnerclient.Protocol, RequestID: "old-resume", Input: json.RawMessage(`{"expected_revision":1}`)}},
		}
		if err := b.RecoverInputs(context.Background(), "project", "candidate"); err != nil {
			t.Fatal(err)
		}
		if err := b.Step(context.Background(), "project", "candidate"); err != nil {
			t.Fatal(err)
		}
		if s.value.Actions[0].State != "cancelled" || !s.value.Actions[1].ObserveOnly || f.inputs != 0 {
			t.Fatal("restart replayed a saved user authorization")
		}
	})
	t.Run("commit failure performs zero calls", func(t *testing.T) {
		b, s, f := setup()
		s.fail = true
		if b.Step(context.Background(), "project", "candidate") == nil {
			t.Fatal("fault did not reject")
		}
		if f.submits+f.stops+f.inputs != 0 {
			t.Fatal("external input preceded durable intent")
		}
	})
	t.Run("late accepted run is retained and revoked", func(t *testing.T) {
		b, s, f := setup()
		f.lostSubmit = true
		if b.Step(context.Background(), "project", "candidate") == nil || s.value.SubmitState != "unknown" || s.value.RunID != "" {
			t.Fatal("lost submission guessed")
		}
		s.value.StopRequested = true
		s.value.Revision++
		if err := b.Step(context.Background(), "project", "candidate"); err != nil {
			t.Fatal(err)
		}
		if s.value.RunID != "run" || !s.value.InputsRevoked || f.submits != 1 || f.stops != 1 {
			t.Fatalf("late Run was lost or resubmitted: %+v %d %d", s.value, f.submits, f.stops)
		}
		if s.value.Projection.StopStatus != "unsupported" {
			t.Fatal("native unsupported became stopped")
		}
		if err := b.Step(context.Background(), "project", "candidate"); err != nil {
			t.Fatal(err)
		}
		if f.stops != 1 || len(s.events) != 1 {
			t.Fatal("accepted stop or stable events replayed")
		}
	})
	t.Run("withdrawal before dispatch sends nothing", func(t *testing.T) {
		b, s, f := setup()
		s.value.StopRequested = true
		if err := b.Step(context.Background(), "project", "candidate"); err != nil {
			t.Fatal(err)
		}
		if s.value.SubmitState != "cancelled" || f.submits != 0 {
			t.Fatal("withdrawn task started work")
		}
	})
	t.Run("unknown old control is never resent after terminal", func(t *testing.T) {
		b, s, f := setup()
		s.value.RunID = "run"
		s.value.SubmitState = "accepted"
		s.value.StopRequested = true
		s.value.Actions = []Action{{Command: "resume", State: "unknown", Request: runnerclient.Request{Protocol: runnerclient.Protocol, RequestID: "old-resume", Input: json.RawMessage(`{"expected_revision":1}`)}}}
		if err := b.Step(context.Background(), "project", "candidate"); err != nil {
			t.Fatal(err)
		}
		if f.inputs != 0 || s.value.Actions[0].State != "unknown" {
			t.Fatal("terminal task replayed old resume")
		}
	})
}

func TestSteerRunnerBoundPRCannotDisappear(t *testing.T) {
	_, store, cli := setup()
	c := store.value
	c.RunID = cli.run.RunID
	cli.run.PR = &runnerclient.PRFact{URL: "https://example.invalid/pull/1"}
	if err := bind(&c, cli.run); err != nil {
		t.Fatal(err)
	}
	before := clone(c)
	for _, pr := range []*runnerclient.PRFact{nil, {URL: "https://example.invalid/pull/2"}} {
		invalid := cli.run
		invalid.Revision++
		invalid.PR = pr
		if err := bind(&c, invalid); err == nil {
			t.Fatal("lost fixed PR identity")
		}
		actual, _ := json.Marshal(c)
		expected, _ := json.Marshal(before)
		if string(actual) != string(expected) {
			t.Fatal("rejected projection changed saved binding")
		}
	}
}

func TestSteerRunnerRestartOriginalBinding(t *testing.T) {
	for _, state := range []string{"pending", "unknown"} {
		t.Run(state+"_not_found_zero_submit", func(t *testing.T) {
			b, s, f := setup()
			s.value.SubmitState = state
			if err := b.RecoverInputs(context.Background(), "project", "candidate"); err != nil {
				t.Fatal(err)
			}
			if err := b.VerifyBinding(context.Background(), "project", "candidate"); err != nil {
				t.Fatal(err)
			}
			if err := b.Step(context.Background(), "project", "candidate"); err != nil {
				t.Fatal(err)
			}
			if f.submits != 0 || f.inputs != 0 {
				t.Fatal("restart replayed original submit")
			}
		})
	}
	for _, mutation := range []string{"digest", "run", "missing_request_run"} {
		t.Run(mutation, func(t *testing.T) {
			b, s, f := setup()
			if mutation == "missing_request_run" {
				s.value.SubmitState = "unknown"
				run := "another-run"
				f.commands[s.value.Submit.RequestID] = runnerclient.Command{RequestID: s.value.Submit.RequestID, CommandID: "original-command", Command: "submit", RunID: &run}
			} else {
				s.value.RunID = "run"
				s.value.SubmitState = "accepted"
				s.value.InputDigest = "input"
				if mutation == "digest" {
					f.run.InputDigest = "other"
				} else {
					f.run.RunID = "other"
				}
			}
			if err := b.VerifyBinding(context.Background(), "project", "candidate"); err == nil {
				t.Fatal("conflicting original public fact admitted")
			}
			if f.submits+f.stops+f.inputs != 0 {
				t.Fatal("verification dispatched business")
			}
		})
	}
	t.Run("unknown_original_accepted_query_only", func(t *testing.T) {
		b, s, f := setup()
		s.value.SubmitState = "unknown"
		run := f.run.RunID
		f.commands[s.value.Submit.RequestID] = runnerclient.Command{RequestID: s.value.Submit.RequestID, CommandID: "original-command", Command: "submit", RunID: &run}
		if err := b.RecoverInputs(context.Background(), "project", "candidate"); err != nil {
			t.Fatal(err)
		}
		if err := b.VerifyBinding(context.Background(), "project", "candidate"); err != nil {
			t.Fatal(err)
		}
		if err := b.Step(context.Background(), "project", "candidate"); err != nil {
			t.Fatal(err)
		}
		if s.value.RunID != run || s.value.InputDigest != "input" || f.submits+f.stops+f.inputs != 0 {
			t.Fatal("original request lost/replayed", s.value)
		}
	})
}

func TestCandidateWithdrawal(t *testing.T) {
	for _, state := range []string{"pending", "unknown", "accepted"} {
		t.Run(state, func(t *testing.T) {
			b, s, f := setup()
			s.value.Participation = "abandoned"
			s.value.SubmitState = state
			if state == "accepted" {
				s.value.RunID = "run"
				s.value.Projection = &f.run
			}
			s.value.Actions = []Action{{Command: "resume", State: "unknown", Request: runnerclient.Request{Protocol: runnerclient.Protocol, RequestID: "old-resume", Input: json.RawMessage(`{"expected_revision":1}`)}}}
			before := clone(s.value)
			for range 2 {
				if err := b.Step(context.Background(), "project", "candidate"); err != nil {
					t.Fatal(err)
				}
			}
			if f.submits+f.stops+f.inputs+f.reads != 0 {
				t.Fatal("manual withdrawal caused new CLI action/observation")
			}
			if string(mustJSON(s.value)) != string(mustJSON(before)) {
				t.Fatal("manual withdrawal changed execution facts")
			}
			if err := b.RecoverInputs(context.Background(), "project", "candidate"); err != nil {
				t.Fatal(err)
			}
			if err := b.VerifyBinding(context.Background(), "project", "candidate"); err != nil {
				t.Fatal(err)
			}
			if f.submits+f.stops+f.inputs+f.reads != 0 {
				t.Fatal("restart actively observed a withdrawn candidate")
			}
			err := b.QueueControl(context.Background(), "project", "candidate", "resume", runnerclient.Request{Protocol: runnerclient.Protocol, RequestID: "new-resume", Input: json.RawMessage(`{"expected_revision":1}`)}, "original-product-digest")
			if err == nil {
				t.Fatal("withdrawn candidate accepted new explicit input")
			}
			// Task termination later grants its own stop obligation, including late acceptance.
			s.value.StopRequested = true
			if state == "unknown" {
				runID := "run"
				f.commands[s.value.Submit.RequestID] = runnerclient.Command{CommandID: "late-submit", Command: "submit", RunID: &runID}
			}
			if err := b.Step(context.Background(), "project", "candidate"); err != nil {
				t.Fatal(err)
			}
			if state == "pending" {
				if f.submits+f.stops != 0 || s.value.SubmitState != "cancelled" {
					t.Fatal("Task stop dispatched unsent work")
				}
			} else if f.stops != 1 || !s.value.InputsRevoked || s.value.Participation != "abandoned" {
				t.Fatal("Task stop obligation was lost after manual withdrawal")
			}
		})
	}
}

func mustJSON(value any) []byte { body, _ := json.Marshal(value); return body }

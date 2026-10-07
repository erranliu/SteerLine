package console

import (
	"context"
	"encoding/json"
	"errors"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"os"
	"time"
	"unicode/utf8"
)

// Only exercised pure ports are implemented; unused ports cannot perform I/O.
type testClient struct {
	ConsoleClient
	query   *testQuerySource
	control *testControlClient
	ref     ProjectRef
	closed  int
}

func (c *testClient) Project() api.ProjectRef { return c.ref }

func (c *testClient) Close() error { c.closed++; return nil }

func (c *testClient) ReadProject(_ context.Context, selected string, _ api.ProjectSnapshot) api.ProjectSnapshot {
	c.query.selected = selected
	return c.query.snapshot
}

func (c *testClient) RecentActivity(_ context.Context, run, execution, attempt string) (api.LogBatch, error) {
	batch := c.query.activity
	batch.Cursor = api.ActivityCursor{RunID: run, ExecutionID: execution, AttemptID: attempt, Initialized: true}
	return batch, nil
}

func (c *testClient) Conversation(context.Context, api.ConversationFilter, string) (api.ConversationPage, error) {
	return c.query.page, nil
}

func (c *testClient) Body(_ context.Context, ref api.ConversationBodyRef, offset int64, limit int) (api.ConversationBodyPage, error) {
	if text, ok := c.query.bodies[ref.ItemKey]; ok {
		end := min(int64(len(text)), offset+int64(limit))
		for end < int64(len(text)) && end > offset && !utf8.RuneStart(text[end]) {
			end--
		}
		if c.query.badBody {
			return api.ConversationBodyPage{}, errors.New("body read failed")
		}
		return api.ConversationBodyPage{ItemKey: ref.ItemKey, Revision: ref.Revision, Offset: offset, NextOffset: end, TotalBytes: int64(len(text)), SHA256: ref.SHA256, Completeness: ref.Completeness, Complete: end == int64(len(text)), Data: []byte(text[offset:end])}, nil
	}
	return c.query.body, nil
}

func (c *testClient) SubmitControl(_ context.Context, in api.ControlInput) (api.ControlRequest, error) {
	f := c.control
	f.calls++
	f.instance, f.revision = in.InstanceID, in.ExpectedRevision
	if f.err != nil {
		return api.ControlRequest{}, f.err
	}
	var raw json.RawMessage
	if in.Payload != (api.ControlPayload{}) {
		raw, _ = json.Marshal(in.Payload)
	}
	f.request = api.ControlRequest{RequestID: in.RequestID, Action: string(in.Action), Payload: raw, InstanceID: in.InstanceID}
	return f.request, nil
}

func (c *testClient) Receipt(context.Context, string) (*api.ControlReceipt, error) {
	c.control.receipts++
	return nil, os.ErrNotExist
}

func (c *testClient) Request(context.Context, string) (*api.ControlRequest, error) {
	c.control.requests++
	return nil, os.ErrNotExist
}

func (c *testClient) SubmitDecision(_ context.Context, in api.DecisionInput) (api.CommandReceipt, bool, error) {
	f := c.control
	f.decisionCalls++
	f.decisionAnswer = in.Answer
	if f.decisionErr != nil {
		return api.CommandReceipt{}, false, f.decisionErr
	}
	f.decisionCommand = api.CommandReceipt{ProjectIdentity: c.ref.Identity, CommandID: in.CommandID, Kind: "answer_decision", TargetID: in.Answer.DecisionID, RequestedBy: in.RequestedBy, Status: "pending"}
	return f.decisionCommand, true, nil
}

func (c *testClient) Command(context.Context, string) (api.CommandReceipt, error) {
	c.control.receipts++
	return c.control.decisionCommand, nil
}

func (c *testClient) DecisionEligibility(context.Context, *api.CandidateSummary) error {
	return c.control.eligibilityErr
}

func (c *testClient) InspectRecovery(context.Context, string) (api.RecoveryInspection, error) {
	return api.RecoveryInspection{}, errors.New("fixture inspection unavailable")
}

var _ ConsoleClient = (*testClient)(nil)

func (c *testClient) CandidateAvailability(snapshot api.ProjectSnapshot, now time.Time) (bool, string) {
	return api.CandidateFreshness(c.ref.Identity, snapshot, now)
}

func (c *testClient) StartTaskAvailability(snapshot api.ProjectSnapshot, now time.Time) string {
	return api.StartTaskReason(c.ref, snapshot, now)
}

func (c *testClient) AutoDispatchAvailability(snapshot api.ProjectSnapshot) string {
	return api.AutoDispatchReason(c.ref, snapshot)
}

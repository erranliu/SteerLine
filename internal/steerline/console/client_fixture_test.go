package console

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"time"

	api "github.com/erranliu/SteerLine/internal/steerline/clientcontracts"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
)

// Only exercised pure ports are implemented; unused ports cannot perform I/O.
type testClient struct {
	api.ConsoleClient
	query   *testQuerySource
	control *testControlClient
	ref     ProjectRef
	closed  int
}

func (c *testClient) Project() api.ProjectRef { return c.ref }
func (c *testClient) Close() error            { c.closed++; return nil }
func (c *testClient) ReadProject(_ context.Context, selected string, _ api.ProjectSnapshot) api.ProjectSnapshot {
	c.query.selected = selected
	return c.query.snapshot
}
func (c *testClient) Activity(context.Context, string, string, string, string, api.JournalCursor) (api.LogBatch, error) {
	return c.query.activity, nil
}
func (c *testClient) Conversation(context.Context, contracts.ConversationFilter, string) (contracts.ConversationPage, error) {
	return c.query.page, nil
}
func (c *testClient) Body(context.Context, contracts.ConversationBodyRef, int64, int) (contracts.ConversationBodyPage, error) {
	return c.query.body, nil
}
func (c *testClient) Capabilities(context.Context, api.CapabilityFilter) ([]contracts.ConversationCapabilityReport, error) {
	return nil, nil
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
func (c *testClient) SubmitDecision(_ context.Context, in api.DecisionInput) (contracts.CommandRecord, bool, error) {
	f := c.control
	f.decisionCalls++
	f.decisionAnswer = in.Answer
	if f.decisionErr != nil {
		return contracts.CommandRecord{}, false, f.decisionErr
	}
	f.decisionCommand = contracts.CommandRecord{ProjectIdentity: c.ref.Identity, CommandID: in.CommandID, Kind: "answer_decision", TargetID: in.Answer.DecisionID, RequestedBy: in.RequestedBy, Status: "pending"}
	return f.decisionCommand, true, nil
}
func (c *testClient) Command(context.Context, string) (contracts.CommandRecord, error) {
	c.control.receipts++
	return c.control.decisionCommand, nil
}
func (c *testClient) DecisionEligibility(context.Context, *contracts.Run) error {
	return c.control.eligibilityErr
}
func (c *testClient) CursorAfter(contracts.ConversationFilter, int64) (string, error) {
	return "fixture-cursor", nil
}
func (c *testClient) InspectRecovery(context.Context, string) (api.RecoveryInspection, error) {
	return api.RecoveryInspection{}, errors.New("fixture inspection unavailable")
}

var _ api.ConsoleClient = (*testClient)(nil)

func (c *testClient) CandidateAvailability(snapshot api.ProjectSnapshot, now time.Time) (bool, string) {
	return api.CandidateFreshness(c.ref.Identity, snapshot, now)
}
func (c *testClient) StartTaskAvailability(snapshot api.ProjectSnapshot, now time.Time) string {
	return api.StartTaskReason(c.ref, snapshot, now)
}
func (c *testClient) AutoDispatchAvailability(snapshot api.ProjectSnapshot) string {
	return api.AutoDispatchReason(c.ref, snapshot)
}

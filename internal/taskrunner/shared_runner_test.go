package taskrunner

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/runnerbridge"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	ports "github.com/erranliu/SteerLine/internal/storage/ports"
)

type savedActionStore struct {
	ControlStore
	command   ports.CommandRecord
	candidate runnerbridge.Candidate
}

func (s *savedActionStore) LookupCommand(context.Context, string, string) (ports.CommandRecord, bool, error) {
	return s.command, true, nil
}
func (s *savedActionStore) UpdateCommand(_ context.Context, c ports.CommandRecord) error {
	s.command = c
	return nil
}
func (s *savedActionStore) Candidate(context.Context, string, string) (runnerbridge.Candidate, error) {
	return s.candidate, nil
}

func TestSharedRunnerControlRecoversMissingOuterReceipt(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	req := api.ControlRequest{ProtocolVersion: ControlProtocolVersion, RequestID: strings.Repeat("a", 32), InstanceID: "original", RepositoryIdentity: "project", Action: "runner_reply", ExpectedControlRevision: 1, RequestedBy: "caller", CreatedAt: now, ExpiresAt: now.Add(time.Minute), Payload: json.RawMessage(`{"run_id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","expected_run_revision":1,"runner_input":"{\"expected_revision\":7,\"text\":\"answer\"}"}`)}
	body, _ := json.Marshal(req)
	wire := runnerclient.Request{Protocol: runnerclient.Protocol, RequestID: strings.Repeat("c", 32), Input: json.RawMessage(`{"expected_revision":7,"text":"answer"}`)}
	store := &savedActionStore{command: ports.CommandRecord{Kind: "runner_control", Payload: body}, candidate: runnerbridge.Candidate{ID: strings.Repeat("b", 32), Project: "project", Revision: 99, Actions: []runnerbridge.Action{{CallerRequestID: req.RequestID, CallerRequestDigest: RequestDigest(req), Command: "reply", Request: wire, State: "accepted"}}}}
	controls := Controls{Store: store, Tasks: &Service{Project: "project", Store: store}, ResolveProfile: func(string, string) (string, error) { t.Fatal("replay reconstructed input"); return "", nil }}
	// The original Action survived, but the outer receipt did not. All live
	// revisions and the current host have advanced; no Runner implementation exists.
	for i := 0; i < 2; i++ {
		receipt, err := controls.Handle(ctx, req, "replacement", 100)
		if err != nil || receipt.Status != "applied" || receipt.RunID != store.candidate.ID {
			t.Fatal(receipt, err)
		}
	}
	if store.candidate.Actions[0].Request.RequestID != wire.RequestID || string(store.candidate.Actions[0].Request.Input) != string(wire.Input) {
		t.Fatal("replay changed original envelope")
	}
	changed := req
	changed.RequestedBy = "other"
	if receipt, err := controls.Handle(ctx, changed, "replacement", 100); err != nil || receipt.Code != "idempotency_conflict" {
		t.Fatal(receipt, err)
	}
}

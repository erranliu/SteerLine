package httpapi

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type roadmapPortFixture struct {
	calls   int
	receipt *CommandResponse
}

func (p *roadmapPortFixture) SubmitRoadmap(_ context.Context, e CommandEnvelope) (CommandResponse, error) {
	p.calls++
	out := CommandResponse{RequestID: e.RequestID, ProjectID: e.ProjectID, InstanceID: e.InstanceID, Action: e.Action, TargetID: e.TargetID, ExpectedVersion: e.ExpectedVersion, Original: &e, Status: "applied"}
	p.receipt = &out
	return out, nil
}
func (p *roadmapPortFixture) RoadmapReceipt(context.Context, string) (*CommandResponse, error) {
	return p.receipt, nil
}

func TestRoadmapSharedImmutableCommandNamespace(t *testing.T) {
	ctx := context.Background()
	authority := newRegistryTestAuthority()
	commands := &registryTestCommands{authority: authority}
	records := newRegistryTestRecords()
	c := newRegistryTestOwner(authority, commands, records)
	port := &roadmapPortFixture{}
	e := registryTestEnvelope(strings.Repeat("a", 32))
	e.Action = "roadmap_parse"
	if _, err := c.submitRoadmap(ctx, e, port); err != nil {
		t.Fatal(err)
	}
	if _, err := c.submitRoadmap(ctx, e, port); err != nil || port.calls != 1 {
		t.Fatal("replay resubmitted", err, port.calls)
	}
	other := e
	other.Action = "pause_dispatch"
	if _, err := c.Submit(ctx, other, "fixture"); err == nil {
		t.Fatal("control replaced Roadmap envelope")
	}
	e.Payload = json.RawMessage(`{"changed":true}`)
	if _, err := c.submitRoadmap(ctx, e, port); err == nil {
		t.Fatal("same Roadmap ID changed payload")
	}

	control := registryTestEnvelope(strings.Repeat("b", 32))
	if _, err := c.Submit(ctx, control, "fixture"); err != nil {
		t.Fatal(err)
	}
	control.Action = "roadmap_parse"
	if _, err := c.submitRoadmap(ctx, control, &roadmapPortFixture{}); err == nil {
		t.Fatal("Roadmap replaced control")
	}

	pending := registryTestEnvelope(strings.Repeat("c", 32))
	pending.Action = "roadmap_parse"
	raw, _ := json.Marshal(pending)
	if err := records.Register(requestRecord{Version: 1, Envelope: pending, Digest: digestBytes(raw)}); err != nil {
		t.Fatal(err)
	}
	unsubmitted := &roadmapPortFixture{}
	if _, err := c.submitRoadmap(ctx, pending, unsubmitted); err == nil || unsubmitted.calls != 0 {
		t.Fatal("registered unknown was dispatched again")
	}
}

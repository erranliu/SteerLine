package runnerbridge

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/erranliu/SteerLine/internal/runnerclient"
)

func TestSharedRunnerControlKeepsOriginalProductRequest(t *testing.T) {
	b, s, f := setup()
	s.value.RunID = "run"
	s.value.SubmitState = "accepted"
	f.run.Controls.CanReply = true
	s.value.Projection = &f.run
	q := runnerclient.Request{Protocol: runnerclient.Protocol, RequestID: "same-product-id", Input: json.RawMessage(`{"expected_revision":1}`)}
	ctx := context.Background()
	if err := b.QueueControl(ctx, "project", "candidate", "reply", q, "original-product-digest"); err != nil {
		t.Fatal(err)
	}
	original := s.value.Actions[0].Request.RequestID
	if original == q.RequestID || len(original) != 32 {
		t.Fatal("product ID leaked into shared namespace", original)
	}
	// The action survived a crash before the outer receipt; the public Run advanced.
	s.value.Projection.Revision = 20
	q.Input = json.RawMessage(`{"expected_revision":20}`)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := b.QueueControl(ctx, "project", "candidate", "reply", q, "original-product-digest"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if len(s.value.Actions) != 1 || s.value.Actions[0].Request.RequestID != original || string(s.value.Actions[0].Request.Input) != `{"expected_revision":1}` {
		t.Fatal("original action replaced or duplicated")
	}
	if b.QueueControl(ctx, "project", "candidate", "reply", q, "changed-product-digest") == nil {
		t.Fatal("changed product request accepted")
	}
	other, otherStore, otherCLI := setup()
	otherStore.value.RunID = "run"
	otherCLI.run.Controls.CanReply = true
	otherStore.value.Projection = &otherCLI.run
	q.Input = json.RawMessage(`{"expected_revision":1}`)
	if err := other.QueueControl(ctx, "other-project", "candidate", "reply", q, "original-product-digest"); err != nil {
		t.Fatal(err)
	}
	if otherStore.value.Actions[0].Request.RequestID == original {
		t.Fatal("projects reused Runner request")
	}
	if f.inputs != 0 || otherCLI.inputs != 0 {
		t.Fatal("saving an action dispatched input")
	}
}

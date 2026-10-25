package runnerbridge

import (
	"context"
	"encoding/json"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	protocol "github.com/erranliu/SteerLine/internal/testkit/protocol"
	"testing"
	"time"
)

func TestPreparationWaitObservationPreservesOriginalCandidate(t *testing.T) {
	b, s, plain := setup()
	s.value.RunID, s.value.SubmitState, s.value.InputDigest = "run", "accepted", "input"
	f := &snapshotFixture{fixtureCLI: plain}
	b.Client = func(string) CLI { return f }
	wait := protocol.RunnerExecutionWait(time.Now().UTC())
	for i, stage := range []string{"queue", "calling", "backoff", "exhausted", "cleared"} {
		r := plain.run
		r.Revision = uint64(i + 1)
		r.Status = "queued"
		r.ExecutionWait = wait.Clone()
		raw := json.RawMessage("null")
		switch stage {
		case "calling":
			r.ExecutionWait.Stage, r.ExecutionWait.ReasonCode, r.ExecutionWait.CallsStarted = stage, "orca_query_running", 1
		case "backoff":
			r.ExecutionWait.Stage, r.ExecutionWait.ReasonCode, r.ExecutionWait.CallsStarted = stage, "orca_query_transient", 1
			at := wait.StartedAt.Add(time.Second)
			r.ExecutionWait.NextAttemptAt = &at
		case "exhausted":
			r.ExecutionWait.Stage, r.ExecutionWait.ReasonCode, r.ExecutionWait.CallsStarted = stage, "query_attempts_exhausted", 5
			r.Status = "blocked"
			r.ResumeRequired = true
		case "cleared":
			r.ExecutionWait = nil
			r.Status = "running"
		}
		if r.ExecutionWait != nil {
			raw, _ = json.Marshal(r.ExecutionWait)
		}
		f.result = runnerclient.RunObservation{Contract: runnerclient.RunObservationContract, Run: r, EventPage: runnerclient.EventPage{NextCursor: stage, Events: []runnerclient.Event{{EventID: stage, RunID: "run", Revision: r.Revision, Type: "execution_wait_changed", ExecutionWait: raw}}}}
		if err := b.Step(context.Background(), "project", "candidate"); err != nil {
			t.Fatal(stage, err)
		}
		if s.value.ID != "candidate" || s.value.RunID != "run" || s.value.Projection.Revision != r.Revision || (s.value.Projection.ExecutionWait == nil) != (stage == "cleared") {
			t.Fatal(stage, s.value)
		}
		before := clone(s.value)
		bad := f.result
		bad.Run.Revision++
		bad.Run.ExecutionWait = wait.Clone()
		bad.Run.ExecutionWait.Query = "foreign"
		bad.EventPage.NextCursor = "invalid"
		f.result = bad
		if err := b.Step(context.Background(), "project", "candidate"); err == nil {
			t.Fatal("bad wait accepted")
		}
		if s.value.Cursor != before.Cursor || s.value.Revision != before.Revision {
			t.Fatal("bad page replaced last valid observation")
		}
	}
	if plain.inputs != 0 || plain.submits != 0 || plain.stops != 0 {
		t.Fatal("observation created inputs", plain)
	}
}

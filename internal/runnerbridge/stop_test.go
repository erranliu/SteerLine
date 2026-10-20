package runnerbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func TestPrepareStartupStop(t *testing.T) {
	t.Run("durable_and_unknown_keeps_original_bytes", func(t *testing.T) {
		b, s, f := setup()
		s.value.RunID, s.value.StopRequested = "run", true
		before := clone(s.value)
		stop, err := b.PrepareStartupStop(context.Background(), s.value, "run", 7)
		if err != nil || stop == nil || stop.Request.RequestID != "sl-startup-stop-candidate-1" || string(stop.Request.Input) != `{"expected_revision":7}` {
			t.Fatal(stop, err)
		}
		if s.value.Stop == nil || s.value.Revision != 2 || f.reads+f.submits+f.stops+f.inputs != 0 {
			t.Fatal("not persisted before zero CLI calls", s.value)
		}
		original, _ := json.Marshal(stop)
		again, err := b.PrepareStartupStop(context.Background(), s.value, "run", 99)
		replayed, _ := json.Marshal(again)
		if err != nil || !bytes.Equal(original, replayed) || s.value.Revision != 2 {
			t.Fatal("unknown request replaced", err)
		}
		if before.Submit.RequestID != s.value.Submit.RequestID || before.RunID != s.value.RunID || before.RunnerBuild != s.value.RunnerBuild {
			t.Fatal("original facts changed")
		}
		// The live path consumes the startup envelope unchanged.
		s.value.SubmitState = "accepted"
		if err := b.Step(context.Background(), s.value.Project, s.value.ID); err != nil {
			t.Fatal(err)
		}
		if f.stops != 1 || s.value.Stop.Request.RequestID != stop.Request.RequestID || !bytes.Equal(s.value.Stop.Request.Input, stop.Request.Input) {
			t.Fatal("live path replaced startup stop")
		}
	})
	t.Run("failed_commit_and_stale_snapshot_return_no_dispatchable_stop", func(t *testing.T) {
		b, s, f := setup()
		s.value.RunID, s.value.StopRequested = "run", true
		s.fail = true
		if stop, err := b.PrepareStartupStop(context.Background(), s.value, "run", 7); err == nil || stop != nil || s.value.Stop != nil {
			t.Fatal("failed save exposed stop", stop, err)
		}
		s.fail = false
		stale := clone(s.value)
		s.value.Revision++
		if stop, err := b.PrepareStartupStop(context.Background(), stale, "run", 7); err == nil || stop != nil {
			t.Fatal("stale snapshot exposed stop")
		}
		if f.submits+f.stops+f.inputs+f.reads != 0 {
			t.Fatal("preparation invoked CLI")
		}
	})
	t.Run("rejected_is_reprepared_accepted_is_observed", func(t *testing.T) {
		b, s, _ := setup()
		s.value.StopRequested = true
		s.value.Stop = &Action{State: "rejected", Request: runnercontract.Request{RequestID: "rejected"}}
		stop, err := b.PrepareStartupStop(context.Background(), s.value, "accepted-lost-submit", 8)
		if err != nil || stop == nil || stop.Request.RequestID == "rejected" || s.value.RunID != "" {
			t.Fatal(stop, err)
		}
		s.value.Stop.State = "accepted"
		if stop, err = b.PrepareStartupStop(context.Background(), s.value, "accepted-lost-submit", 9); err != nil || stop != nil {
			t.Fatal("accepted stop dispatched again", stop, err)
		}
	})
}

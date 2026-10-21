package runnerbridge

import (
	"context"
	"testing"

	"github.com/erranliu/SteerLine/internal/runnerclient"
)

type snapshotFixture struct {
	*fixtureCLI
	result runnerclient.RunObservation
	calls  int
}

func (f *snapshotFixture) SupportsObservation() bool { return true }
func (f *snapshotFixture) Observe(context.Context, string, string) (runnerclient.RunObservation, error) {
	f.calls++
	return f.result, nil
}

func TestRunnerObservationAtomicSave(t *testing.T) {
	for _, mode := range []string{"valid", "future-event", "wrong-input", "wrong-run", "missing-contract", "cas-failure"} {
		t.Run(mode, func(t *testing.T) {
			b, s, plain := setup()
			s.value.RunID, s.value.SubmitState = "run", "accepted"
			s.value.InputDigest = "input"
			f := &snapshotFixture{fixtureCLI: plain, result: runnerclient.RunObservation{Contract: runnerclient.RunObservationContract, Run: plain.run, EventPage: runnerclient.EventPage{NextCursor: "next", Events: []runnerclient.Event{{EventID: "one", RunID: "run", Revision: 1}}}}}
			switch mode {
			case "future-event":
				f.result.EventPage.Events[0].Revision = 2
			case "wrong-input":
				f.result.Run.InputDigest = "foreign"
			case "wrong-run":
				f.result.Run.RunID = "foreign"
			case "missing-contract":
				f.result.Contract = ""
			case "cas-failure":
				s.fail = true
			}
			b.Client = func(string) CLI { return f }
			err := b.Step(context.Background(), "project", "candidate")
			if mode == "valid" {
				if err != nil || s.value.Cursor != "next" || !s.events["one"] || s.value.Projection == nil {
					t.Fatal(s.value, err)
				}
			} else if err == nil || s.value.Cursor != "" || len(s.events) != 0 || s.value.Projection != nil {
				t.Fatal("invalid/CAS page partially saved", mode, s.value, err)
			}
			if f.calls != 1 || f.reads != 0 || f.inputs != 0 || f.submits != 0 {
				t.Fatal("observation made extra calls or side effects", f)
			}
		})
	}
}

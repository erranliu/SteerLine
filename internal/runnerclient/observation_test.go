package runnerclient

import (
	"context"
	"testing"
)

func TestRunnerObservationResponse(t *testing.T) {
	c := takeoverCLI(t)
	c.RunObservation = true
	for _, mode := range []string{"valid", "empty", "future-event", "wrong-run", "missing-events", "duplicate-event", "stalled-cursor", "wrong-contract"} {
		t.Run(mode, func(t *testing.T) {
			result := RunObservation{Contract: RunObservationContract, Run: Run{RunID: "original", Revision: 3}, EventPage: EventPage{Events: []Event{{EventID: "one", RunID: "original", Revision: 2}}, NextCursor: "next"}}
			switch mode {
			case "empty":
				result.EventPage.Events = []Event{}
				result.EventPage.NextCursor = "previous"
			case "future-event":
				result.EventPage.Events[0].Revision = 4
			case "wrong-run":
				result.Run.RunID = "foreign"
			case "missing-events":
				result.EventPage.Events = nil
			case "duplicate-event":
				result.EventPage.Events = append(result.EventPage.Events, result.EventPage.Events[0])
			case "stalled-cursor":
				result.EventPage.NextCursor = "previous"
			case "wrong-contract":
				result.Contract = "foreign"
			}
			fixtureResponse(t, result)
			_, err := c.Observe(context.Background(), "original", "previous")
			if (err == nil) != (mode == "valid" || mode == "empty") {
				t.Fatal(mode, err)
			}
		})
	}
}

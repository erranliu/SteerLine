package runnerclient

import (
	"context"
	"encoding/json"
)

const RunObservationContract = "steerrunner-run-observation/v1"

type RunObservation struct {
	Contract  string    `json:"contract"`
	Run       Run       `json:"run"`
	EventPage EventPage `json:"event_page"`
}

func (c Client) SupportsObservation() bool { return c.RunObservation }

func (c Client) Observe(ctx context.Context, id, after string) (RunObservation, error) {
	if !c.SupportsObservation() {
		return RunObservation{}, &Failure{Code: "unsupported_capability", Message: "Runner does not publish same-snapshot observation", Exit: 3}
	}
	args := []string{"--run-id", id, "--limit", "100"}
	if after != "" {
		args = append(args, "--after", after)
	}
	response, err := c.Call(ctx, "observe", args, nil)
	if err != nil {
		return RunObservation{}, err
	}
	var result RunObservation
	if json.Unmarshal(response.Data, &result) != nil || result.Contract != RunObservationContract || result.Run.RunID != id || result.Run.Revision == 0 || result.EventPage.Events == nil || len(result.EventPage.Events) > 100 || result.EventPage.NextCursor == "" || result.EventPage.HasMore && len(result.EventPage.Events) == 0 || len(result.EventPage.Events) > 0 && result.EventPage.NextCursor == after {
		return RunObservation{}, &Failure{Code: "runner_unavailable", Message: "Runner observation identity or event page is invalid", Exit: 4}
	}
	seen := map[string]bool{}
	for _, event := range result.EventPage.Events {
		if event.EventID == "" || seen[event.EventID] || event.RunID != id || event.Revision == 0 || event.Revision > result.Run.Revision {
			return RunObservation{}, &Failure{Code: "runner_unavailable", Message: "Runner event exceeds its bound snapshot", Exit: 4}
		}
		seen[event.EventID] = true
	}
	return result, nil
}

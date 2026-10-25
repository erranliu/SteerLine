package runnerclient

import (
	"context"
	"encoding/json"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

type PRFact = runnercontract.PRFact
type Final = runnercontract.Final
type Execution = runnercontract.Execution
type Question = runnercontract.Question
type Controls = runnercontract.Controls
type Run = runnercontract.Run
type Command = runnercontract.Command
type Inspection = runnercontract.Inspection
type Event = runnercontract.Event
type EventPage = runnercontract.EventPage

func (c Client) Inspect(ctx context.Context, selector, id string, refresh bool) (Inspection, error) {
	switch selector {
	case "run-id", "request-id", "command-id":
	default:
		return Inspection{}, &Failure{Code: "invalid_arguments", Message: "Invalid inspection selector", Exit: 2}
	}
	args := []string{"--" + selector, id}
	if refresh {
		if selector != "run-id" {
			return Inspection{}, &Failure{Code: "invalid_arguments", Message: "PR refresh requires Run identity", Exit: 2}
		}
		args = append(args, "--refresh-pr")
	}
	response, err := c.Call(ctx, "inspect", args, nil)
	if err != nil {
		return Inspection{}, err
	}
	var result Inspection
	var raw struct {
		Run json.RawMessage `json:"run"`
	}
	if json.Unmarshal(response.Data, &raw) == nil && len(raw.Run) != 0 && string(raw.Run) != "null" {
		if err := c.validateRunWait(raw.Run); err != nil {
			return Inspection{}, waitFailure(err)
		}
	}
	if json.Unmarshal(response.Data, &result) != nil || result.Run != nil && (result.Run.RunID == "" || result.Run.Revision == 0) || selector == "run-id" && (result.Run == nil || result.Run.RunID != id) || selector == "request-id" && (result.Command == nil || result.Command.RequestID != id) || selector == "command-id" && (result.Command == nil || result.Command.CommandID != id) || result.Command != nil && result.Command.RunID != nil && (result.Run == nil || result.Run.RunID != *result.Command.RunID) {
		return Inspection{}, &Failure{Code: "runner_unavailable", Message: "Runner inspection identity could not be verified", Exit: 4}
	}
	return result, nil
}
func (c Client) Events(ctx context.Context, id, after string) (EventPage, error) {
	args := []string{"--run-id", id, "--limit", "100"}
	if after != "" {
		args = append(args, "--after", after)
	}
	response, err := c.Call(ctx, "events", args, nil)
	if err != nil {
		return EventPage{}, err
	}
	var result EventPage
	if json.Unmarshal(response.Data, &result) != nil || result.NextCursor == "" {
		return EventPage{}, &Failure{Code: "runner_unavailable", Message: "Runner event page is invalid", Exit: 4}
	}
	for _, event := range result.Events {
		if err := c.validateWaitEvent(event); err != nil {
			return EventPage{}, waitFailure(err)
		}
		if event.EventID == "" || event.RunID != id || event.Revision == 0 {
			return EventPage{}, &Failure{Code: "runner_unavailable", Message: "Runner event identity is invalid", Exit: 4}
		}
	}
	return result, nil
}

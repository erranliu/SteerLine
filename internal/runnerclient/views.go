package runnerclient

import (
	"context"
	"encoding/json"
	"errors"
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

func (c Client) Inspect(ctx context.Context, selector, id string) (Inspection, error) {
	return c.inspect(ctx, selector, id, false)
}

// InspectBeforeStart never starts a service. A cold query is explicitly leased
// by Runner and can prove absence of an original submit before stop injection.
func (c Client) InspectBeforeStart(ctx context.Context, selector, id string) (Inspection, bool, error) {
	info, err := c.ServiceStatus(ctx)
	if err != nil {
		return Inspection{}, false, err
	}
	switch info.Status {
	case "ready":
		if err = c.validateStartedService(info, Version{Version: c.BuildVersion, ResourceDigest: c.ResourceDigest}); err != nil {
			return Inspection{}, false, err
		}
		c.InstanceID = info.InstanceID
		value, err := c.inspect(ctx, selector, id, false)
		return value, false, err
	case "not_running":
		c.InstanceID = ""
		value, err := c.inspect(ctx, selector, id, true)
		return value, true, err
	default:
		return Inspection{}, false, errors.New("original Runner ownership/startup is unverified")
	}
}
func (c Client) inspect(ctx context.Context, selector, id string, offline bool) (Inspection, error) {
	switch selector {
	case "run-id", "request-id", "command-id":
	default:
		return Inspection{}, &Failure{Code: "invalid_arguments", Message: "Invalid inspection selector", Exit: 2}
	}
	args := []string{"--" + selector, id}
	if offline {
		args = append(args, "--offline")
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

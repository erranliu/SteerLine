package app

import (
	"context"
	"errors"

	"github.com/erranliu/SteerLine/internal/runnerbinding"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

// Original requests are inspected before workers start. Only a cold, leased
// request_not_found can prove absence; an online missing reply remains unknown.
func inspectOriginalBeforeStart(ctx context.Context, cli runnerclient.Client, stateDir, runID, digest string, request runnercontract.Request) (runnerclient.Inspection, bool, error) {
	if !runnerbinding.SamePath(stateDir, cli.StateDir) {
		return runnerclient.Inspection{}, false, errors.New("original Runner namespace differs; use the original installation to reconcile its resources")
	}
	selector, id := "run-id", runID
	if id == "" {
		selector, id = "request-id", request.RequestID
	}
	if id == "" {
		return runnerclient.Inspection{}, false, errors.New("original Runner request identity is missing")
	}
	view, cold, err := cli.InspectBeforeStart(ctx, selector, id)
	if err != nil {
		return view, cold, err
	}
	r := view.Run
	if r == nil || r.RunID == "" || r.Revision == 0 || r.InputDigest == "" || runID != "" && r.RunID != runID || digest != "" && digest != r.InputDigest {
		return view, cold, errors.New("original Runner Run/input binding differs")
	}
	if selector == "request-id" && (view.Command == nil || view.Command.Command != "submit" || view.Command.RequestID != request.RequestID || view.Command.RunID == nil || *view.Command.RunID != r.RunID) {
		return view, cold, errors.New("original Runner submit binding differs")
	}
	return view, cold, nil
}

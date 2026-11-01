package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation"
	envmodel "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/runnerbinding"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

// Existing shutdown verification consumes already supplied historical proofs.
// Startup cancellation below never requests this retired cross-build protocol.
func environmentTakeover(proof runnercontract.TakeoverReport, r envmodel.Record, a envmodel.Round) (runnercontract.TakeoverItem, bool) {
	if canonicalPath(r.Build.StateDir) != canonicalPath(proof.StateDir) {
		return runnercontract.TakeoverItem{}, false
	}
	for _, item := range proof.Items {
		if item.Kind != "run" || item.Status != "compatible" {
			continue
		}
		if a.RunID != "" {
			if item.ID == a.RunID && a.InputDigest != "" && item.InputDigest == a.InputDigest {
				return item, true
			}
		} else if item.RequestID == a.Request.RequestID && item.RequestDigest == runnercontract.RequestDigest("submit", "", a.Request) {
			return item, true
		}
	}
	return runnercontract.TakeoverItem{}, false
}

func unsubmittedEnvironment(a envmodel.Round) bool {
	return a.RunID == "" && (a.Submission == "pending" || a.Submission == "rejected" || a.Submission == "cancelled")
}
func verifyEnvironmentBindings(ctx context.Context, c Config, store env.Store, cli runnerclient.Client) error {
	rows, e := store.Environments(ctx, c.Identity())
	if e != nil {
		return e
	}
	for _, r := range rows {
		if canonicalPath(r.Build.StateDir) != canonicalPath(cli.StateDir) {
			return errors.New("original environment Runner directory differs")
		}
		for _, a := range r.Rounds {
			if unsubmittedEnvironment(a) {
				continue
			}
			if !runnerbinding.SameBuild(r.Build, cli.BuildPin()) {
				return errors.New("environment activity belongs to another Runner build; cross-build takeover is unsupported")
			}
			selector, id := "run-id", a.RunID
			if id == "" {
				selector, id = "request-id", a.Request.RequestID
			}
			v, e := cli.Inspect(ctx, selector, id, false)
			if e != nil {
				return e
			}
			if v.Run == nil || v.Run.Correlation["environment_activity"] != r.ID || a.InputDigest != "" && a.InputDigest != v.Run.InputDigest {
				return errors.New("original environment binding is unverified")
			}
		}
	}
	return nil
}

func prepareEnvironmentStops(ctx context.Context, c Config, store env.Store, cli *runnerclient.Client) ([]runnercontract.PendingStop, error) {
	return prepareEnvironmentStopsWith(ctx, c, store, cli.BuildPin(), cli.InspectBeforeStart)
}

// The public pre-start reader returns whether absence was observed under a
// cold writer lease. No caller reads Runner storage or starts its workers.
func prepareEnvironmentStopsWith(ctx context.Context, c Config, store env.Store, build runnercontract.BuildPin, inspect func(context.Context, string, string) (runnerclient.Inspection, bool, error)) ([]runnercontract.PendingStop, error) {
	rows, err := store.Environments(ctx, c.Identity())
	if err != nil {
		return nil, err
	}
	pending := []runnercontract.PendingStop{}
	for _, r := range rows {
		changed := false
		for i := range r.Rounds {
			a := &r.Rounds[i]
			if unsubmittedEnvironment(*a) {
				continue
			}
			if !runnerbinding.SameBuild(r.Build, build) {
				return nil, errors.New("environment activity belongs to another Runner build; cross-build takeover is unsupported")
			}
			if !r.Cancelled || a.Settled {
				continue
			}
			selector, id := "run-id", a.RunID
			if id == "" {
				selector, id = "request-id", a.Request.RequestID
			}
			view, cold, err := inspect(ctx, selector, id)
			var failure *runnerclient.Failure
			if err != nil {
				if cold && selector == "request-id" && errors.As(err, &failure) && failure.Code == "request_not_found" {
					// Absence under the original writer lease proves this submit never ran.
					a.Submission = "cancelled"
					a.Settled = true
					r.State = "cancelled"
					changed = true
					continue
				}
				return nil, err
			}
			run := view.Run
			if run == nil || run.RunID == "" || run.InputDigest == "" || run.Revision == 0 || a.RunID != "" && a.RunID != run.RunID || a.InputDigest != "" && a.InputDigest != run.InputDigest || a.Run != nil && run.Revision < a.Run.Revision || run.Correlation["steerline_project"] != r.Project || run.Correlation["environment_activity"] != r.ID || run.Correlation["environment_round"] != fmt.Sprint(a.Number) {
				return nil, errors.New("original cancelled environment Run/input binding is unverified")
			}
			if selector == "request-id" && (view.Command == nil || view.Command.RequestID != a.Request.RequestID || view.Command.Command != "submit") {
				return nil, errors.New("original environment submit identity differs")
			}
			a.RunID, a.InputDigest, a.Run = run.RunID, run.InputDigest, run
			changed = true
			// Closed permanently revokes further input. Its existing internal stop
			// must be allowed to settle on startup, rather than replaced by another.
			if run.Closed || run.StopStatus == "stopped" && (run.Status == "completed" || run.Status == "stopped") {
				continue
			}
			if a.Stop != nil && a.StopState != "rejected" {
				receipt, _, readErr := inspect(ctx, "request-id", a.Stop.RequestID)
				if readErr != nil {
					var original struct {
						ExpectedRevision uint64 `json:"expected_revision"`
					}
					if !errors.As(readErr, &failure) || failure.Code != "request_not_found" || json.Unmarshal(a.Stop.Input, &original) != nil || original.ExpectedRevision == 0 {
						return nil, readErr
					}
					// Read Run first, then receipt: a missing request at an already newer
					// monotonic revision cannot subsequently accept the old CAS envelope.
					if original.ExpectedRevision > run.Revision {
						return nil, errors.New("original stop revision exceeds persisted Run revision")
					}
					if original.ExpectedRevision < run.Revision {
						a.StopState = "rejected"
					}
				} else {
					if receipt.Command == nil || receipt.Command.Command != "stop" || receipt.Command.RequestID != a.Stop.RequestID || receipt.Command.RunID == nil || *receipt.Command.RunID != run.RunID {
						return nil, errors.New("original environment stop identity differs")
					}
					if receipt.Command.ActionStatus == "rejected" || receipt.Command.ActionStatus == "failed" {
						a.StopState = "rejected"
					}
				}
				if a.StopState == "rejected" {
					// Commit the verified rejection before replacing its envelope.
					old := r.Revision
					r.Revision++
					if err := store.SaveEnvironment(ctx, r, old); err != nil {
						return nil, err
					}
				}
			}
			if a.Stop == nil || a.StopState == "rejected" {
				if a.Stop != nil {
					a.RejectedStops = append(a.RejectedStops, *a.Stop)
				}
				body, _ := json.Marshal(map[string]uint64{"expected_revision": run.Revision})
				a.Stop = &runnercontract.Request{Protocol: runnercontract.Protocol, RequestID: envmodel.ID(), Input: body}
				a.StopState = "unknown"
			}
			pending = append(pending, runnercontract.PendingStop{RunID: run.RunID, Request: *a.Stop})
		}
		if changed {
			old := r.Revision
			r.Revision++
			if err := store.SaveEnvironment(ctx, r, old); err != nil {
				return nil, err
			}
		}
	}
	return pending, nil
}

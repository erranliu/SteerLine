package app

import (
	"context"
	"encoding/json"
	"errors"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation"
	envmodel "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/runnerbinding"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

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
				if cli.Takeover == nil || !runnerbinding.Compatible(*cli.Takeover, cli.BuildPin()) {
					return errors.New("original environment takeover proof unavailable")
				}
				if _, ok := environmentTakeover(*cli.Takeover, r, a); !ok {
					return errors.New("original environment Run/input differs from takeover proof")
				}
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
	rows, e := store.Environments(ctx, c.Identity())
	if e != nil {
		return nil, e
	}
	need := false
	for _, r := range rows {
		if r.Kind == "generation" && !r.Settled() && c.runtimeGenerations != nil {
			active := len(r.Rounds) > 0 && r.Rounds[0].Submission != "pending"
			c.runtimeGenerations.Track(r.Project, "environment:"+r.ID, r.CreatedAt, active)
		}
		for _, a := range r.Rounds {
			if !unsubmittedEnvironment(a) && (!runnerbinding.SameBuild(r.Build, cli.BuildPin()) || r.Cancelled && !a.Settled) {
				need = true
			}
		}
	}
	if !need {
		return nil, nil
	}
	if cli.Takeover == nil {
		proof, e := cli.VerifyTakeover(ctx)
		if e != nil {
			return nil, e
		}
		cli.Takeover = &proof
	}
	if !runnerbinding.Compatible(*cli.Takeover, cli.BuildPin()) {
		return nil, errors.New("environment takeover incompatible")
	}
	pending := []runnercontract.PendingStop{}
	for _, r := range rows {
		changed := false
		for i := range r.Rounds {
			a := &r.Rounds[i]
			if unsubmittedEnvironment(*a) {
				continue
			}
			item, bound := environmentTakeover(*cli.Takeover, r, *a)
			if !bound && a.RunID != "" {
				return nil, errors.New("original environment absent from takeover proof")
			}
			if !r.Cancelled || a.Settled || !bound {
				continue
			}
			if a.Stop == nil || a.StopState == "rejected" {
				if a.Stop != nil {
					a.RejectedStops = append(a.RejectedStops, *a.Stop)
				}
				body, _ := json.Marshal(map[string]uint64{"expected_revision": item.Revision})
				a.Stop = &runnercontract.Request{Protocol: runnercontract.Protocol, RequestID: envmodel.ID(), Input: body}
				a.StopState = "unknown"
				changed = true
			}
			pending = append(pending, runnercontract.PendingStop{RunID: item.ID, Request: *a.Stop})
		}
		if changed {
			old := r.Revision
			r.Revision++
			if e := store.SaveEnvironment(ctx, r, old); e != nil {
				return nil, e
			}
		}
	}
	return pending, nil
}

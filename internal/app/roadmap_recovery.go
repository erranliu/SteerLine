package app

import (
	"context"
	"encoding/json"
	"errors"

	roadmapservice "github.com/erranliu/SteerLine/internal/roadmap"
	roadmap "github.com/erranliu/SteerLine/internal/roadmap/model"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	"github.com/erranliu/SteerLine/internal/testmanagement"
)

// Reuse the existing content-request binding verifier. This is a transport-only
// projection; Roadmap inputs/results are never persisted in test generation tables.
func roadmapBinding(r roadmap.Record) testmanagement.Generation {
	return testmanagement.Generation{ID: r.ID, Project: r.Project, Revision: r.Revision, Runner: r.Runner, Request: r.Request, Submission: r.Submission, State: r.State, RunID: r.RunID, InputDigest: r.InputDigest, Projection: r.Projection, Cancelled: r.Cancelled, StopRequest: r.StopRequest, CreatedAt: r.CreatedAt}
}
func prepareRoadmapStops(ctx context.Context, c *Config, store *sqlite.SQLiteStore, cli *runnerclient.Client) ([]runnercontract.PendingStop, error) {
	rows, e := store.Roadmaps(ctx, c.Identity())
	if e != nil {
		return nil, e
	}
	pending := []runnercontract.PendingStop{}
	for _, r := range rows {
		if c.runtimeGenerations != nil {
			c.runtimeGenerations.UpdateContent(c.Identity(), "roadmap:"+r.ID, r.CreatedAt, r.Submission != "pending", r.Settled())
		}
		if r.Submission == "pending" || r.State == "rejected" && r.RunID == "" || r.ObservationComplete() {
			continue
		}
		if !r.Cancelled || r.Projection != nil && r.Projection.StopStatus == "stopped" {
			continue
		}
		view, cold, err := inspectOriginalBeforeStart(ctx, *cli, r.Runner.StateDir, r.RunID, r.InputDigest, r.Request)
		if err != nil {
			var failure *runnerclient.Failure
			if cold && r.RunID == "" && errors.As(err, &failure) && failure.Code == "request_not_found" {
				r.State, r.Submission = "rejected", "rejected"
				r.Revision++
				if err := store.SaveRoadmap(ctx, r, r.Revision-1); err != nil {
					return nil, err
				}
				continue
			}
			return nil, err
		}
		run := view.Run
		if run.Correlation["steerline_project"] != r.Project || run.Correlation["steerline_roadmap_parse"] != r.ID {
			return nil, errors.New("original Roadmap correlation differs")
		}
		if r.StopRequest == nil || r.StopState == "rejected" {
			body, _ := json.Marshal(map[string]uint64{"expected_revision": run.Revision})
			r.StopRequest = &runnercontract.Request{Protocol: runnercontract.Protocol, RequestID: roadmapservice.Token(), Input: body}
			r.StopState = "unknown"
			r.Revision++
			if e = store.SaveRoadmap(ctx, r, r.Revision-1); e != nil {
				return nil, e
			}
		}
		pending = append(pending, runnercontract.PendingStop{RunID: run.RunID, Request: *r.StopRequest})
	}
	return pending, nil
}

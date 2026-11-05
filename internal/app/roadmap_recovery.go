package app

import (
	"context"
	"errors"

	roadmapservice "github.com/erranliu/SteerLine/internal/roadmap"
	roadmap "github.com/erranliu/SteerLine/internal/roadmap/model"
	"github.com/erranliu/SteerLine/internal/runnerbinding"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
)

func roadmapContent(r roadmap.Record) runnerbinding.Content {
	return runnerbinding.Content{StateDir: r.Runner.StateDir, Request: r.Request, RunID: r.RunID, InputDigest: r.InputDigest, Project: r.Project, ObjectKey: "steerline_roadmap_parse", ObjectID: r.ID, Unsubmitted: r.Submission == "pending", Rejected: r.State == "rejected" && r.RunID == ""}
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
		if err := runnerbinding.ValidateContentCorrelation(roadmapContent(r), run); err != nil {
			return nil, err
		}
		if r.StopRequest == nil || r.StopState == "rejected" {
			r.StopRequest = runnerbinding.ContentStop(roadmapservice.Token(), run.Revision)
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

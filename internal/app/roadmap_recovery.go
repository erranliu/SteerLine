package app

import (
	"context"
	"encoding/json"
	"errors"

	roadmapservice "github.com/erranliu/SteerLine/internal/roadmap"
	roadmap "github.com/erranliu/SteerLine/internal/roadmap/model"
	"github.com/erranliu/SteerLine/internal/runnerbinding"
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
func verifyRoadmapTakeover(r roadmap.Record, proof runnercontract.TakeoverReport) error {
	if r.Submission == "pending" || r.State == "rejected" && r.RunID == "" {
		return nil
	}
	if _, ok := generationTakeover(proof, roadmapBinding(r)); !ok {
		return errors.New("原 Roadmap 解析 Run/输入与接管证明不同")
	}
	return nil
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
		if runnerbinding.SameBuild(r.Runner, cli.BuildPin()) && !r.Cancelled {
			continue
		}
		if cli.Takeover == nil {
			proof, e := cli.VerifyTakeover(ctx)
			if e != nil {
				return nil, e
			}
			cli.Takeover = &proof
		}
		if !runnerbinding.Compatible(*cli.Takeover, cli.BuildPin()) {
			return nil, errors.New("Roadmap 原生成接管不兼容")
		}
		if e = verifyRoadmapTakeover(r, *cli.Takeover); e != nil {
			return nil, e
		}
		if !r.Cancelled || r.Projection != nil && r.Projection.StopStatus == "stopped" {
			continue
		}
		item, _ := generationTakeover(*cli.Takeover, roadmapBinding(r))
		if r.StopRequest == nil || r.StopState == "rejected" {
			body, _ := json.Marshal(map[string]uint64{"expected_revision": item.Revision})
			r.StopRequest = &runnercontract.Request{Protocol: runnercontract.Protocol, RequestID: roadmapservice.Token(), Input: body}
			r.StopState = "unknown"
			r.Revision++
			if e = store.SaveRoadmap(ctx, r, r.Revision-1); e != nil {
				return nil, e
			}
		}
		pending = append(pending, runnercontract.PendingStop{RunID: item.ID, Request: *r.StopRequest})
	}
	return pending, nil
}

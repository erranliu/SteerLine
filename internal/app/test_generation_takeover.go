package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/erranliu/SteerLine/internal/runnerbinding"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/testmanagement"
)

func generationTakeover(proof runnercontract.TakeoverReport, g testmanagement.Generation) (runnercontract.TakeoverItem, bool) {
	if canonicalPath(g.Runner.StateDir) != canonicalPath(proof.StateDir) {
		return runnercontract.TakeoverItem{}, false
	}
	for _, item := range proof.Items {
		if item.Kind != "run" || item.Status != "compatible" {
			continue
		}
		if g.RunID != "" {
			if item.ID == g.RunID && g.InputDigest != "" && item.InputDigest == g.InputDigest {
				return item, true
			}
		} else if item.RequestID == g.Request.RequestID && item.RequestDigest == runnercontract.RequestDigest("submit", "", g.Request) {
			return item, true
		}
	}
	return runnercontract.TakeoverItem{}, false
}
func verifyGenerationTakeover(g testmanagement.Generation, cli runnerclient.Client) error {
	if g.State == "rejected" && g.RunID == "" {
		return nil
	}
	if cli.Takeover == nil || !runnerbinding.Compatible(*cli.Takeover, cli.BuildPin()) {
		return errors.New("original generation takeover proof is unavailable")
	}
	if _, ok := generationTakeover(*cli.Takeover, g); !ok {
		return errors.New("original generation Run/input differs from takeover proof")
	}
	return nil
}

type generationStartupStore interface{ testmanagement.GenerationStore }

func prepareGenerationStops(ctx context.Context, store generationStartupStore, project string, cli *runnerclient.Client) ([]runnercontract.PendingStop, error) {
	rows, err := store.Generations(ctx, project)
	if err != nil {
		return nil, err
	}
	need := false
	for _, g := range rows {
		if g.State == "rejected" && g.RunID == "" {
			continue
		}
		if !runnerbinding.SameBuild(g.Runner, cli.BuildPin()) || g.Cancelled && (g.Projection == nil || g.Projection.StopStatus != "stopped") {
			need = true
		}
	}
	if !need {
		return nil, nil
	}
	if cli.Takeover == nil {
		proof, err := cli.VerifyTakeover(ctx)
		if err != nil {
			return nil, err
		}
		cli.Takeover = &proof
	}
	if !runnerbinding.Compatible(*cli.Takeover, cli.BuildPin()) {
		return nil, errors.New("generation takeover is incompatible")
	}
	var pending []runnercontract.PendingStop
	for _, g := range rows {
		if g.State == "rejected" && g.RunID == "" {
			continue
		}
		item, bound := generationTakeover(*cli.Takeover, g)
		if !bound && g.RunID != "" {
			return nil, errors.New("generation is absent from original takeover proof")
		}
		if !g.Cancelled || !bound || g.Projection != nil && g.Projection.StopStatus == "stopped" {
			continue
		}
		if g.StopRequest == nil {
			body, _ := json.Marshal(map[string]uint64{"expected_revision": item.Revision})
			g.StopRequest = &runnercontract.Request{Protocol: runnercontract.Protocol, RequestID: fmt.Sprintf("sl-generation-stop-%s-%d", g.ID, g.Revision), Input: body}
			g.StopState = "unknown"
			old := g.Revision
			g.Revision++
			if err := store.SaveGeneration(ctx, g, old); err != nil {
				return nil, err
			}
		}
		pending = append(pending, runnercontract.PendingStop{RunID: item.ID, Request: *g.StopRequest})
	}
	return pending, nil
}

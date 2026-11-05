package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/erranliu/SteerLine/internal/runnerbinding"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/testmanagement"
)

type generationStartupStore interface{ testmanagement.GenerationStore }

func prepareGenerationStops(ctx context.Context, store generationStartupStore, project string, cli *runnerclient.Client) ([]runnercontract.PendingStop, error) {
	rows, err := store.Generations(ctx, project)
	if err != nil {
		return nil, err
	}
	var pending []runnercontract.PendingStop
	for _, g := range rows {
		if g.Unsubmitted() || g.State == "rejected" && g.RunID == "" {
			continue
		}
		if !g.Cancelled || g.Projection != nil && g.Projection.StopStatus == "stopped" {
			continue
		}
		view, cold, err := inspectOriginalBeforeStart(ctx, *cli, g.Runner.StateDir, g.RunID, g.InputDigest, g.Request)
		if err != nil {
			var failure *runnerclient.Failure
			if cold && g.RunID == "" && errors.As(err, &failure) && failure.Code == "request_not_found" {
				g.State = "rejected"
				g.Diagnostic = "原测试生成已取消；离线查询确认原提交未被受理，可以重试生成"
				g.Revision++
				if err := store.SaveGeneration(ctx, g, g.Revision-1); err != nil {
					return nil, err
				}
				continue
			}
			return nil, err
		}
		run := view.Run
		if err := runnerbinding.ValidateContentCorrelation(generationContent(g), run); err != nil {
			return nil, err
		}
		if g.StopRequest == nil || g.StopState == "rejected" {
			g.StopRequest = runnerbinding.ContentStop(fmt.Sprintf("sl-generation-stop-%s-%d", g.ID, g.Revision), run.Revision)
			g.StopState = "unknown"
			old := g.Revision
			g.Revision++
			if err := store.SaveGeneration(ctx, g, old); err != nil {
				return nil, err
			}
		}
		pending = append(pending, runnercontract.PendingStop{RunID: run.RunID, Request: *g.StopRequest})
	}
	return pending, nil
}

func generationContent(g testmanagement.Generation) runnerbinding.Content {
	return runnerbinding.Content{StateDir: g.Runner.StateDir, Request: g.Request, RunID: g.RunID, InputDigest: g.InputDigest, Project: g.Project, ObjectKey: "steerline_generation", ObjectID: g.ID, Unsubmitted: g.Unsubmitted(), Rejected: g.State == "rejected" && g.RunID == ""}
}

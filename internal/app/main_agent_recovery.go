package app

import (
	"context"
	"errors"

	"github.com/erranliu/SteerLine/internal/mainagent"
	ma "github.com/erranliu/SteerLine/internal/mainagent/model"
	"github.com/erranliu/SteerLine/internal/runnerbinding"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
)

func prepareMainAgentStops(ctx context.Context, c *Config, store *sqlite.SQLiteStore, cli *runnerclient.Client) ([]runnercontract.PendingStop, error) {
	rows, err := store.Analyses(ctx, c.Identity())
	if err != nil {
		return nil, err
	}
	pending := []runnercontract.PendingStop{}
	for _, r := range rows {
		if r.Submission == "pending" {
			r.Cancelled = true
			r.Submission, r.State = "cancelled", "cancelled"
			r.Revision++
			if err = store.SaveAnalysis(ctx, r, r.Revision-1); err != nil {
				return nil, err
			}
		}
		if c.runtimeGenerations != nil {
			c.runtimeGenerations.UpdateContent(c.Identity(), "main-agent:"+r.ID, r.CreatedAt, r.Submission != "pending", r.Settled())
		}
		if r.Complete() || r.Submission == "rejected" || r.Submission == "cancelled" {
			continue
		}
		view, cold, e := inspectOriginalBeforeStart(ctx, *cli, r.Runner.StateDir, r.RunID, r.InputDigest, r.Request)
		if e != nil {
			var failure *runnerclient.Failure
			if cold && r.RunID == "" && errors.As(e, &failure) && failure.Code == "request_not_found" {
				r.Submission, r.State = "rejected", "rejected"
				r.Diagnostic = "冷态核实原请求未提交，未补发模型输入"
				r.Revision++
				if e = store.SaveAnalysis(ctx, r, r.Revision-1); e != nil {
					return nil, e
				}
				if c.runtimeGenerations != nil {
					c.runtimeGenerations.UpdateContent(c.Identity(), "main-agent:"+r.ID, r.CreatedAt, true, true)
				}
				continue
			}
			return nil, e
		}
		if e = runnerbinding.ValidateContent(mainagent.Binding(r), view); e != nil {
			return nil, e
		}
		if !r.Cancelled || view.Run.StopStatus == "stopped" {
			continue
		}
		if r.StopRequest == nil || r.StopState == "rejected" {
			r.StopRequest = runnerbinding.ContentStop(ma.Token(), view.Run.Revision)
			r.StopState = "unknown"
			r.Revision++
			if e = store.SaveAnalysis(ctx, r, r.Revision-1); e != nil {
				return nil, e
			}
		}
		if r.StopRequest == nil {
			return nil, errors.New("analysis stop outbox missing")
		}
		pending = append(pending, runnercontract.PendingStop{RunID: view.Run.RunID, Request: *r.StopRequest})
	}
	return pending, nil
}

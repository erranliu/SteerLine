package app

import (
	"context"
	"encoding/json"
	"errors"
	ref "github.com/erranliu/SteerLine/internal/refactoring/model"
	"github.com/erranliu/SteerLine/internal/runnerbinding"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	"time"
)

// Restore only original ownership and stops before Runner workers can resume.
func prepareRefactoringStops(ctx context.Context, c *Config, store *sqlite.SQLiteStore, cli *runnerclient.Client) ([]runnercontract.PendingStop, error) {
	rows, e := store.Refactorings(ctx, c.Identity())
	if e != nil {
		return nil, e
	}
	pending := []runnercontract.PendingStop{}
	for _, r := range rows {
		if !runnerbinding.SamePath(r.Frozen.Runner.StateDir, cli.StateDir) {
			return nil, errors.New("maintenance Runner namespace differs")
		}
		if !r.Active() || r.Phase == "waiting_input" {
			continue
		}
		expected := r.Revision
		r.Pause(r.UpdatedAt)
		r.StopRequested = true
		r.Phase = "stopping"
		for i := range r.Operations {
			o := &r.Operations[i]
			if o.State == "pending" {
				o.State = "cancelled"
			}
			if o.Settled() {
				continue
			}
			view, cold, err := inspectOriginalBeforeStart(ctx, *cli, r.Frozen.Runner.StateDir, o.RunID, o.InputDigest, o.Request)
			if err != nil {
				if o.RunID == "" && submitNotAccepted(view, cold, o.Request.RequestID, err) {
					o.State = "rejected"
					o.Feedback = "已核实 Runner 未受理原请求，未重发"
					continue
				}
				return nil, err
			}
			if err = runnerbinding.ValidateContent(o.Content(r), view); err != nil {
				return nil, err
			}
			o.ObserveRun(view.Run)
			if view.Run.StopStatus != "stopped" {
				if o.Stop != nil && o.StopState != "rejected" {
					stopView, stopCold, stopErr := cli.InspectBeforeStart(ctx, "request-id", o.Stop.RequestID)
					var failure *runnerclient.Failure
					state := ""
					if stopErr != nil {
						if stopCold && errors.As(stopErr, &failure) && failure.Code == "request_not_found" {
							var input struct {
								Revision uint64 `json:"expected_revision"`
							}
							if json.Unmarshal(o.Stop.Input, &input) != nil {
								return nil, errors.New("原停止输入无效")
							}
							if input.Revision != view.Run.Revision {
								state = "absent"
							}
						} else {
							return nil, stopErr
						}
					} else {
						command := stopView.Command
						if command == nil || command.RequestID != o.Stop.RequestID || command.Command != "stop" || command.RunID == nil || *command.RunID != o.RunID {
							return nil, errors.New("原维护停止绑定不符")
						}
						if command.ActionStatus == "rejected" || command.ActionStatus == "failed" {
							state = "rejected"
						}
					}
					if state != "" {
						o.PriorStops = append(o.PriorStops, ref.StopAttempt{Request: *o.Stop, State: state})
						o.StopState = "rejected"
					}
				}
				if o.Stop == nil || o.StopState == "rejected" {
					o.Stop = runnerbinding.ContentStop(ref.ID(), view.Run.Revision)
					o.StopState = "unknown"
				}
				pending = append(pending, runnercontract.PendingStop{RunID: o.RunID, Request: *o.Stop})
			}
		}
		r.Revision++
		r.UpdatedAt = time.Now().UTC()
		if e = store.SaveRefactoring(ctx, r, expected, nil); e != nil {
			return nil, e
		}
	}
	return pending, nil
}

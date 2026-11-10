package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/erranliu/SteerLine/internal/runnerbinding"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	suggestionbridge "github.com/erranliu/SteerLine/internal/steeragentbridge/suggestion"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
)

func advisoryRecords(ctx context.Context, store *sqlite.SQLiteStore, project string) ([]suggestionbridge.Record, error) {
	ids, err := store.AdvisoryIDs(ctx, project, "all")
	if err != nil {
		return nil, err
	}
	rows := []suggestionbridge.Record{}
	for _, id := range ids {
		raw, revision, e := store.LoadAdvisory(ctx, project, id)
		if e != nil {
			return nil, e
		}
		var r suggestionbridge.Record
		if e = json.Unmarshal(raw, &r); e != nil {
			return nil, e
		}
		if r.Project != project || r.ID != id || r.Revision != revision {
			return nil, errors.New("advisory identity mismatch")
		}
		rows = append(rows, r)
	}
	return rows, nil
}
func saveAdvisoryRecovery(ctx context.Context, store *sqlite.SQLiteStore, r suggestionbridge.Record) (suggestionbridge.Record, error) {
	expected := r.Revision
	r.Revision++
	raw, e := json.Marshal(r)
	if e != nil {
		return r, e
	}
	return r, store.SaveAdvisory(ctx, r.Project, r.ID, r.Source.CandidateID, raw, expected)
}
func prepareAdvisoryStops(ctx context.Context, c *Config, store *sqlite.SQLiteStore, cli *runnerclient.Client) ([]runnercontract.PendingStop, error) {
	rows, err := advisoryRecords(ctx, store, c.Identity())
	if err != nil {
		return nil, err
	}
	pending := []runnercontract.PendingStop{}
	for _, r := range rows {
		if c.runtimeGenerations != nil {
			c.runtimeGenerations.UpdateContent(c.Identity(), "advisory:"+r.ID, r.CreatedAt, r.Submission == "started", r.Complete())
		}
		if r.Submission != "started" || r.Complete() || r.Projection != nil && r.Projection.StopStatus == "stopped" {
			continue
		}
		content := suggestionbridge.Content(r)
		view, cold, e := inspectOriginalBeforeStart(ctx, *cli, content.StateDir, content.RunID, content.InputDigest, content.Request)
		if e != nil {
			var failure *runnerclient.Failure
			if cold && r.RunID == "" && errors.As(e, &failure) && failure.Code == "request_not_found" {
				r.State, r.Submission, r.Settled = "failed", "rejected", true
				if r.Cancelled {
					r.State = "cancelled"
				}
				now := time.Now().UTC()
				r.FinishedAt = &now
				r.ObservedAt = now
				r.Diagnostic = "Runner 冷态确认原生成请求不存在，未重新提交"
				if r, e = saveAdvisoryRecovery(ctx, store, r); e != nil {
					return nil, e
				}
				if c.runtimeGenerations != nil {
					c.runtimeGenerations.UpdateContent(c.Identity(), "advisory:"+r.ID, r.CreatedAt, false, true)
				}
				continue
			}
			return nil, e
		}
		if e = runnerbinding.ValidateContent(content, view); e != nil {
			return nil, e
		}
		if view.Run.StopStatus == "stopped" {
			continue
		}
		if !r.Cancelled && r.State != "failed" && r.StopRequest == nil {
			continue
		}
		if r.StopRequest != nil && r.StopState == "unknown" {
			state, readErr := suggestionbridge.InspectOriginalStop(ctx, r, view.Run, func(ctx context.Context, selector, id string) (runnercontract.Inspection, error) {
				value, _, err := cli.InspectBeforeStart(ctx, selector, id)
				return value, err
			})
			if readErr != nil {
				return nil, readErr
			}
			if state != r.StopState {
				r.StopState = state
				// Record the original rejection before replacing its envelope.
				if r, e = saveAdvisoryRecovery(ctx, store, r); e != nil {
					return nil, e
				}
			}
		}
		if r.StopRequest == nil || r.StopState == "rejected" {
			r.StopRequest = runnerbinding.ContentStop(advisoryStopToken(), view.Run.Revision)
			r.StopState = "unknown"
			if r, e = saveAdvisoryRecovery(ctx, store, r); e != nil {
				return nil, e
			}
		}
		pending = append(pending, runnercontract.PendingStop{RunID: view.Run.RunID, Request: *r.StopRequest})
	}
	return pending, nil
}

func advisoryStopToken() string {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b[:])
}

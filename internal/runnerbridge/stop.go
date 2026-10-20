package runnerbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/erranliu/SteerLine/internal/runnercontract"
)

// prepareStop is the sole constructor and durable writer of a candidate's stop
// envelope. The caller holds the candidate gate; unknown envelopes are retained.
func (b *Bridge) prepareStop(ctx context.Context, c *Candidate, revision uint64, requestID func() string) error {
	if c.Stop != nil && c.Stop.State != "rejected" {
		return nil
	}
	body, _ := json.Marshal(map[string]uint64{"expected_revision": revision})
	c.Stop = &Action{Command: "stop", Request: runnercontract.Request{Protocol: "steerrunner-cli/v1", RequestID: requestID(), Input: body}, State: "unknown"}
	return b.save(ctx, c)
}

// PrepareStartupStop saves an already-authorized stop before Runner workers can
// start. App owns the project locks/store; no CLI or native operation occurs here.
// runID may be an accepted Run correlated by the original, lost submit receipt.
func (b *Bridge) PrepareStartupStop(ctx context.Context, c Candidate, runID string, revision uint64) (*runnercontract.PendingStop, error) {
	var pending *runnercontract.PendingStop
	err := b.WithCandidate(c.Project, c.ID, func() error {
		current, err := b.Store.Candidate(ctx, c.Project, c.ID)
		if err != nil {
			return err
		}
		if current.Revision != c.Revision {
			return errors.New("candidate changed before CLI checkpoint")
		}
		c = current
		if !c.StopRequested || c.InputsRevoked {
			return nil
		}
		if runID == "" || c.RunID != "" && c.RunID != runID {
			return errors.New("original cancellation target differs")
		}
		if err := b.prepareStop(ctx, &c, revision, func() string {
			return "sl-startup-stop-" + c.ID + fmt.Sprintf("-%d", c.Revision)
		}); err != nil {
			return err
		}
		if c.Stop.State == "unknown" || c.Stop.State == "pending" {
			pending = &runnercontract.PendingStop{RunID: runID, Request: c.Stop.Request}
		}
		return nil
	})
	return pending, err
}

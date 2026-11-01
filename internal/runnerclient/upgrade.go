package runnerclient

import (
	"context"
	"errors"
)

const StateFormat = "steerrunner-state/v7"

// VerifyDrained is a cold, public query. Service exit alone does not prove that
// business input, native resources and outstanding controls have been settled.
func (c Client) VerifyDrained(ctx context.Context) error {
	info, err := c.ServiceStatus(ctx)
	if err != nil {
		return err
	}
	if info.Status != "not_running" {
		return errors.New("Runner upgrade requires the original service to be stopped after draining its Runs and controls")
	}
	proof, err := c.VerifyStopped(ctx)
	if err != nil {
		return err
	}
	for _, item := range proof.Runs {
		view, cold, err := c.InspectBeforeStart(ctx, "run-id", item.RunID)
		if err != nil {
			return err
		}
		r := view.Run
		if !cold || r == nil || r.InputDigest != item.InputDigest || r.StopStatus != "stopped" || !drainedBusiness(r) || r.ResumeRequired || r.NewInputsAllowed || r.Controls.CanResume || r.Controls.CanReply || r.Controls.CanMessage || r.Controls.CanInterrupt {
			return errors.New("Runner upgrade requires all original Runs and unresolved controls to be drained with the original executable")
		}
		seen := map[string]bool{}
		for _, op := range r.Operations {
			if op.ID == "" || op.Action == "" || seen[op.ID] {
				return errors.New("Runner upgrade operation identities are unverified")
			}
			seen[op.ID] = true
			switch op.Status {
			case "accepted", "confirmed", "rejected":
				// Accepted is a known receipt, not independent completion proof.
				// It is only admitted with the terminal business/resource proof above.
			default:
				return errors.New("Runner upgrade requires original pending or unknown operations to be reconciled with the original executable")
			}
		}
	}
	return nil
}

func drainedBusiness(r *Run) bool {
	switch r.Status {
	case "completed", "stopped":
		return true
	case "closed", "failed":
		return r.Closed
	}
	return false
}

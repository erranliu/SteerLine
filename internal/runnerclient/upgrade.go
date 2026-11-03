package runnerclient

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
)

const StateFormat = "steerrunner-state/v8"

// VerifyDrained is a cold, public query. Service exit alone does not prove that
// business input, native resources and outstanding controls have been settled.
func (c Client) VerifyDrained(ctx context.Context) error {
	return c.verifyDrained(ctx, c)
}

// VerifyUpgradeDrained keeps original business admission while allowing the
// validated candidate to interpret native stop receipts in the same namespace.
// Both executables must read the same Run/input set without starting a writer.
func (c Client) VerifyUpgradeDrained(ctx context.Context, candidate Client) error {
	same := filepath.Clean(c.StateDir) == filepath.Clean(candidate.StateDir)
	if runtime.GOOS == "windows" {
		same = strings.EqualFold(filepath.Clean(c.StateDir), filepath.Clean(candidate.StateDir))
	}
	if !filepath.IsAbs(c.StateDir) || !same {
		return errors.New("Runner upgrade resource verifier must use the original state directory")
	}
	return c.verifyDrained(ctx, candidate)
}

func (c Client) verifyDrained(ctx context.Context, resources Client) error {
	info, err := c.ServiceStatus(ctx)
	if err != nil {
		return err
	}
	if info.Status != "not_running" {
		return errors.New("Runner upgrade requires the original service to be stopped after draining its Runs and controls")
	}
	proof, err := resources.VerifyStopped(ctx)
	if err != nil {
		return err
	}
	if err := c.verifyDrainedRuns(ctx, proof); err != nil {
		return err
	}
	if resources.Executable != c.Executable || resources.SHA256 != c.SHA256 {
		return resources.verifyDrainedRuns(ctx, proof)
	}
	return nil
}

func (c Client) verifyDrainedRuns(ctx context.Context, proof StoppedVerification) error {
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

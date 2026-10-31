//go:build e2e

package testenv

import (
	"fmt"

	"github.com/erranliu/SteerLine/internal/runnerstate"
)

// admissionProbe pins the original candidate while its native turn starts.
// Allocating a workspace/session alone does not prove that startup finished.
type admissionProbe struct {
	candidate, run, digest string
}

func (p *admissionProbe) observe(c runnerstate.Candidate) (bool, error) {
	if p.candidate == "" {
		if c.ID == "" || c.RunID == "" || c.InputDigest == "" {
			return false, nil
		}
		p.candidate, p.run, p.digest = c.ID, c.RunID, c.InputDigest
	}
	if c.ID != p.candidate || c.RunID != p.run || c.InputDigest != p.digest {
		return false, fmt.Errorf("original admission candidate/Run binding changed")
	}
	if c.Projection == nil {
		return false, nil
	}
	r := c.Projection
	if r.RunID != p.run || r.InputDigest != p.digest {
		return false, fmt.Errorf("admission projection belongs to another Run/input")
	}
	if r.StopStatus == "unknown" || r.StopStatus == "unsupported" {
		return false, fmt.Errorf("original admission native stop is %s", r.StopStatus)
	}
	return r.Execution.TurnStarted && r.Execution.SessionID != "" && r.Execution.TurnID != "", nil
}

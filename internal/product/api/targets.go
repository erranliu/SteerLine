package api

import "fmt"

// EnvironmentTarget binds a side action to the exact blocker the user saw.
func EnvironmentTarget(run *CandidateSummary) (*HostActionTarget, error) {
	if run == nil || run.EnvironmentBlocker == nil {
		return nil, fmt.Errorf("run has no environment blocker")
	}
	b := run.EnvironmentBlocker
	return &HostActionTarget{RunID: run.RunID, ExecutionID: run.ExecutionID, AttemptID: run.AttemptID, TurnID: b.TurnID, BlockerID: b.ID, Version: b.Version}, nil
}

package steerline

import "strings"

func manuallyStopped(run *Run) bool {
	return run != nil && run.ManualStop != nil && run.ManualStop.AttemptID == run.AttemptID && !run.ManualStop.ConfirmedAt.IsZero() && strings.TrimSpace(run.ManualStop.ConfirmedBy) != "" && strings.TrimSpace(run.ManualStop.Basis) != ""
}

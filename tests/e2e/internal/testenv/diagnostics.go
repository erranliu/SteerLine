package testenv

import (
	"context"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
	"path/filepath"
	"time"
)

// Diagnostics export public execution identities only. Native auth, config,
// transcripts and Runner storage are never enumerated or copied here.
func (p *Project) captureNativeDiagnostics() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	snap, err := p.SnapshotContext(ctx)
	if err != nil {
		return err
	}
	return controlled.Write(filepath.Join(p.Scenario.Output, p.Name+"-native-public.json"), publicNativeDiagnostics(snap))
}

func publicNativeDiagnostics(snap Snapshot) []map[string]any {
	result := []map[string]any{}
	for _, c := range snap.Runner.Candidates {
		row := map[string]any{"candidate_id": c.ID, "run_id": c.RunID, "submit_state": c.SubmitState, "participation": c.Participation}
		if c.Projection != nil {
			r := c.Projection
			row["revision"], row["status"] = r.Revision, r.Status
			row["execution"] = r.Execution
			row["controls"] = r.Controls
		}
		result = append(result, row)
	}
	return result
}

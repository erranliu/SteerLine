package testenv

import (
	"fmt"
	"reflect"
	"strings"
	"time"
)

func (p *Project) AssertStagedFlow(snapshot Snapshot) {
	p.Scenario.t.Helper()
	if len(snapshot.Domain.WorkflowRuns) != 1 || len(snapshot.Domain.Executions) != 1 || len(snapshot.Domain.Attempts) != 1 {
		p.Scenario.t.Fatal("staged chain lacks one original workflow/execution/Attempt")
	}
	w, e := snapshot.Domain.WorkflowRuns[0], snapshot.Domain.Executions[0]
	if w.FlowRef != "development/v1" || w.StepID != "await_merge" || w.Status != "waiting_external" || len(w.StageInputs) != 3 || len(w.StageResults) != 3 || e.TurnNumber != 3 || w.CandidateSHA != snapshot.Helper.PR.HeadRevision {
		p.Scenario.t.Fatal("staged flow/head/count mismatch")
	}
	for i, step := range []string{"implement.execute", "implement.report", "publish"} {
		input, result := w.StageInputs[i], w.StageResults[i]
		if input.StepID != step || result.StepID != step || input.CommandID != result.CommandID || input.Version != result.InputVersion || result.SessionID != e.SessionID || input.AttemptID != snapshot.Domain.Attempts[0].AttemptID || !result.Succeeded || result.FinalSource == "" || result.FinalItemID == "" {
			p.Scenario.t.Fatalf("stage %s lost original input/final binding", step)
		}
		if input.Snapshot != nil && result.Snapshot == nil {
			p.Scenario.t.Fatal("stage snapshot lost")
		}
		copySnap := snapshot
		copySnap.Domain.Executions = append(copySnap.Domain.Executions[:0:0], copySnap.Domain.Executions...)
		copySnap.Domain.Executions[0].CurrentCommandID = result.CommandID
		copySnap.Domain.Executions[0].CurrentTurnID = result.TurnID
		_, err := p.NativeUsage(copySnap, fmt.Sprintf("step-%d-%s", i, strings.ReplaceAll(step, ".", "-")))
		p.Scenario.Must(err)
	}
	a, b := *w.StageResults[0].Snapshot, *w.StageResults[1].Snapshot
	a.Workspace.CheckedAt, b.Workspace.CheckedAt = time.Time{}, time.Time{}
	if !reflect.DeepEqual(a, b) || a.Workspace.ContentDigest == "" || a.Workspace.Dirty {
		p.Scenario.t.Fatal("report wrote business artifacts or lacks comparison evidence")
	}
	if w.StageInputs[1].SourceCommandID != w.StageResults[0].CommandID || w.StageInputs[1].SourceTurnID != w.StageResults[0].TurnID || !strings.Contains(w.StageInputs[1].ReportToolPolicy, "fact_comparison_only") {
		p.Scenario.t.Fatal("report source/guarantee missing")
	}
	p.Record("staged-flow", snapshot)
}

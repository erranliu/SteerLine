package testenv

import (
	"fmt"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
	"path/filepath"
	"strings"
)

func (p *Project) AssertStagedFlow(snapshot Snapshot) {
	p.Scenario.t.Helper()
	if len(snapshot.Domain.WorkflowRuns) != 1 || len(snapshot.Domain.Executions) != 1 || len(snapshot.Domain.Attempts) != 1 {
		p.Scenario.t.Fatal("staged chain lacks one original workflow/execution/Attempt")
	}
	w, e := snapshot.Domain.WorkflowRuns[0], snapshot.Domain.Executions[0]
	if w.FlowRef != "development/v3" || w.StepID != "await_merge" || w.Status != "waiting_external" || len(w.StageInputs) != 3 || len(w.StageResults) != 3 || e.TurnNumber != 3 {
		p.Scenario.t.Fatal("staged flow/count mismatch")
	}
	for i, step := range []string{"implement.execute", "implement.report", "publish"} {
		input, result := w.StageInputs[i], w.StageResults[i]
		if input.StepID != step || result.StepID != step || input.CommandID != result.CommandID || input.Version != result.InputVersion || result.SessionID != e.SessionID || input.AttemptID != snapshot.Domain.Attempts[0].AttemptID || !result.Succeeded || result.FinalSource == "" || result.FinalItemID == "" {
			p.Scenario.t.Fatalf("stage %s lost original input/final binding", step)
		}
		if e.Protocol == string(executionbackend.ProtocolOrcaTerminalV3) {
			p.Scenario.Must(controlled.Write(filepath.Join(p.Scenario.Output, fmt.Sprintf("step-%d-usage-capability.json", i)), map[string]any{"status": "not_exposed_by_orca", "step_id": step, "command_id": result.CommandID, "session_id": result.SessionID, "dispatch_id": result.FinalNativeTurnID, "final_source": result.FinalSource}))
			continue
		}
		copySnap := snapshot
		copySnap.Domain.Executions = append(copySnap.Domain.Executions[:0:0], copySnap.Domain.Executions...)
		copySnap.Domain.Executions[0].CurrentCommandID = result.CommandID
		copySnap.Domain.Executions[0].CurrentTurnID = result.TurnID
		_, err := p.NativeUsage(copySnap, fmt.Sprintf("step-%d-%s", i, strings.ReplaceAll(step, ".", "-")))
		p.Scenario.Must(err)
	}
	if w.StageInputs[1].SourceCommandID != w.StageResults[0].CommandID || w.StageInputs[1].SourceTurnID != w.StageResults[0].TurnID || !strings.Contains(w.StageInputs[1].ReportToolPolicy, "business_read_only_is_prompt_policy") {
		p.Scenario.t.Fatal("report source/guarantee missing")
	}
	p.Record("staged-flow", snapshot)
}

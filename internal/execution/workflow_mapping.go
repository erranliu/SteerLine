package execution

import (
 "github.com/erranliu/SteerLine/internal/execution/reliable"
	"encoding/json"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	workflow "github.com/erranliu/SteerLine/internal/execution/workflow"
)

func workflowSnapshot(w *executionmodel.WorkflowRunRecord) workflow.Snapshot {
	value := workflow.Snapshot{StepID: w.StepID, ReportRetry: uint64(w.ReportRetry), PRURL: w.PRURL}
	for _, input := range w.StageInputs {
		value.StageInputs = append(value.StageInputs, workflowInput(input))
	}
	for _, result := range w.StageResults {
		value.StageResults = append(value.StageResults, workflow.StageResult{CommandID: result.CommandID, AttemptID: result.AttemptID, StepID: result.StepID, InputVersion: result.InputVersion, TurnID: result.TurnID, Succeeded: result.Succeeded, FinalVerified: reliable.VerifiedContent(result.FinalVerified,result.FinalSource,result.FinalItemID), Final: result.Final, Failure: result.Failure, FinalDiagnostic: result.FinalDiagnostic, Outcome: result.Outcome})
	}
	return value
}
func workflowInput(input executionmodel.StageInput) workflow.StageInput {
	value := workflow.StageInput(input)
	value.TaskFacts = append(json.RawMessage(nil), input.TaskFacts...)
	return value
}

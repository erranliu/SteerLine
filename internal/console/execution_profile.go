package console

import (
	api "github.com/erranliu/SteerLine/internal/product/api"
	"strings"
)

const executionConfigChanged = api.ExecutionConfigChanged

func executionConfigMismatch(ref ProjectRef, snapshot ProjectSnapshot) bool {
	return api.ExecutionConfigMismatch(ref, snapshot)
}
func renderFixedRunExecutionProfile(state projectState, run *api.CandidateSummary) []string {
	fixed := fixedRunExecutionProfile(state, run)
	if fixed == nil {
		return nil
	}
	lines := []string{"执行方案：" + safeTerminalInlineText(fixed.Name) + " [" + safeTerminalInlineText(fixed.ID) + "] · " + safeTerminalInlineText(fixed.Host), "Agent：" + safeTerminalInlineText(fixed.Agent)}
	for _, field := range fixed.ImplementationDisplay {
		lines = append(lines, "实现配置："+safeTerminalInlineText(field))
	}
	for _, detail := range fixed.Details {
		lines = append(lines, safeTerminalInlineText(detail))
	}
	if state.snapshot.History != nil {
		for _, execution := range state.snapshot.History.Executions {
			if execution.ExecutionID == run.ExecutionID {
				for _, detail := range execution.Details {
					lines = append(lines, safeTerminalInlineText(detail))
				}
			}
		}
	}
	return lines
}
func renderRunExecutionProfileSummary(state projectState, run *api.CandidateSummary) []string {
	fixed := fixedRunExecutionProfile(state, run)
	if fixed == nil {
		return nil
	}
	return []string{"执行方案：" + safeTerminalInlineText(fixed.Host) + " · " + safeTerminalInlineText(fixed.Agent) + " · " + safeTerminalInlineText(strings.Join(fixed.ImplementationDisplay, " · "))}
}
func fixedRunExecutionProfile(state projectState, run *api.CandidateSummary) *api.ExecutionProfileSummary {
	if run == nil {
		return nil
	}
	if run.ExecutionProfile != nil {
		return run.ExecutionProfile
	}
	if state.snapshot.History != nil {
		for _, workflow := range state.snapshot.History.WorkflowRuns {
			if workflow.WorkflowRunID == run.RunID && workflow.ExecutionProfile != nil {
				return workflow.ExecutionProfile
			}
		}
	}
	return nil
}

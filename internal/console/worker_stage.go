package console

import (
	api "github.com/erranliu/SteerLine/internal/product/api"
	"strings"
)

func workerStagePresentation(run *api.CandidateSummary) []string {
	if run == nil || run.FlowRef == "" {
		return nil
	}
	lines := workerStateLines(run, true)
	var sourceFailed, reportGenerated bool
	var implementationBody string
	for _, result := range run.StageResults {
		if result.StepID == "implement.execute" {
			sourceFailed = !result.Succeeded || result.Failure != ""
			reportGenerated = false
			implementationBody = ""
			if result.FinalVerified && result.FinalSource != "" {
				implementationBody = result.Final
			} else {
				lines = append(lines, "原实现正文来源尚未核验")
			}
		}
		if result.StepID == "implement.report" && result.Succeeded && result.FinalVerified && result.FinalSource != "" {
			reportGenerated = true
		}
	}
	if implementationBody != "" && run.StepID != "implement.execute" {
		lines = append(lines, "原实现正文："+safeTerminalText(implementationBody))
	}
	if sourceFailed && reportGenerated {
		lines = append(lines, "执行失败，报告已生成；原失败仍需处理")
	}
	if run.StageDiagnostic != "" {
		lines = append(lines, "步骤诊断："+safeTerminalText(run.StageDiagnostic))
	}
	if strings.Contains(run.ReportToolPolicy, "fact_comparison_only") {
		lines = append(lines, "报告沿用原生权限；按事实比对核验工作区及绑定 PR，无法保证拦截全部外部写入")
	}
	return lines
}

func workerResultNotice(run *api.CandidateSummary) string {
	if run == nil {
		return ""
	}
	failed, reported := false, false
	for _, result := range run.StageResults {
		if result.StepID == "implement.execute" {
			failed, reported = !result.Succeeded || result.Failure != "", false
		}
		if result.StepID == "implement.report" && result.Succeeded && result.FinalVerified && result.FinalSource != "" {
			reported = true
		}
	}
	if failed && reported {
		return "执行失败，报告已生成；原失败仍需处理"
	}
	return ""
}

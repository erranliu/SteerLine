package console

import (
	api "github.com/erranliu/SteerLine/internal/product/api"
	"strings"
)

func workerStagePresentation(run *api.CandidateSummary) []string {
	if run == nil || run.FlowRef == "" {
		return nil
	}
	lines := []string{"步骤：" + safeTerminalText(run.StepID) + " · 状态：" + safeTerminalText(run.BusinessStatus)}
	var sourceFailed, reportGenerated bool
	for _, result := range run.StageResults {
		if result.StepID == "implement.execute" {
			sourceFailed = !result.Succeeded || result.Failure != ""
			reportGenerated = false
		}
		if result.StepID == "implement.report" && result.Succeeded && result.FinalSource != "" {
			reportGenerated = true
		}
	}
	if sourceFailed && reportGenerated {
		lines = append(lines, "执行失败，报告已生成；原失败仍需处理")
	}
	if run.StageDiagnostic != "" {
		lines = append(lines, "步骤诊断："+safeTerminalText(run.StageDiagnostic))
	}
	if strings.Contains(run.ReportToolPolicy, "fact_comparison_only") {
		lines = append(lines, "报告沿用原生权限；已核对工作区及绑定 PR，无法保证拦截全部外部写入")
	}
	return lines
}

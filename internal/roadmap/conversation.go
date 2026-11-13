package roadmap

import (
	"fmt"

	"github.com/erranliu/SteerLine/internal/activityconversation"
	"github.com/erranliu/SteerLine/internal/roadmap/model"
)

func Conversation(record model.Record) activityconversation.Facts {
	if record.Purpose != "documents" {
		return activityconversation.Facts{Runs: []activityconversation.Run{{Key: record.ID, Label: "草稿解析", RunID: record.RunID, Projection: record.Projection, StateDir: record.Runner.StateDir}}}
	}
	var facts activityconversation.Facts
	for i, generation := range record.Generations {
		label := fmt.Sprintf("文档生成 %d · %s", i+1, generation.Key)
		facts.Runs = append(facts.Runs, activityconversation.Run{Key: generation.ID, Label: label, RunID: generation.RunID, Projection: generation.Projection, StateDir: record.Runner.StateDir})
	}
	if record.AdmissionRequest != "" {
		facts.Operation = &activityconversation.Operation{Label: "正式入图", RequestID: record.AdmissionRequest, Status: record.State, Diagnostic: record.Diagnostic}
	} else if record.Publication != nil {
		facts.Operation = &activityconversation.Operation{Label: "发布文档", RequestID: record.Publication.RequestID, Status: record.Publication.State, Diagnostic: record.Publication.Diagnostic}
	}
	if facts.Operation != nil {
		facts.Diagnostic = "可读会话来自文档生成。当前操作结果单独显示。"
	}
	return facts
}

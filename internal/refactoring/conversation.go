package refactoring

import (
	"fmt"

	"github.com/erranliu/SteerLine/internal/activityconversation"
	"github.com/erranliu/SteerLine/internal/refactoring/model"
)

func Conversation(record model.Record) activityconversation.Facts {
	var facts activityconversation.Facts
	for i, operation := range record.Operations {
		label := fmt.Sprintf("操作 %d · %s", i+1, conversationLabel(operation.Purpose))
		facts.Runs = append(facts.Runs, activityconversation.Run{Key: operation.ID, Label: label, RunID: operation.RunID, Projection: operation.Run, StateDir: record.Frozen.Runner.StateDir})
	}
	return facts
}

func conversationLabel(purpose string) string {
	switch purpose {
	case "select":
		return "选择方向"
	case "execute":
		return "实施"
	case "audit":
		return "审查"
	case "finish":
		return "收尾"
	case "publish":
		return "发布"
	case "conflict":
		return "处理冲突"
	default:
		return purpose
	}
}

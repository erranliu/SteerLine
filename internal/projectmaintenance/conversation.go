package projectmaintenance

import (
	"encoding/json"

	"github.com/erranliu/SteerLine/internal/activityconversation"
)

func BaselineConversation(id, kind, status string, payload, result []byte) (activityconversation.Facts, bool) {
	var input struct {
		Action string `json:"action"`
	}
	if kind != "project_maintenance" || json.Unmarshal(payload, &input) != nil || input.Action != "maintenance_check" {
		return activityconversation.Facts{}, false
	}
	var output struct {
		Diagnostic string `json:"diagnostic"`
	}
	_ = json.Unmarshal(result, &output)
	return activityconversation.Facts{
		Diagnostic: "本次主分支检查未产生模型会话。",
		Operation:  &activityconversation.Operation{Label: "检查主分支", RequestID: id, Status: status, Diagnostic: output.Diagnostic},
	}, true
}

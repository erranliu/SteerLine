package orcaworker

import (
	"encoding/json"
	"strings"
)

func MessageIdentity(message Message) (taskID, dispatchID string) {
	payload := message.Payload
	var text string
	if json.Unmarshal(payload, &text) == nil {
		payload = []byte(text)
	}
	var identity struct {
		TaskID     string `json:"taskId"`
		DispatchID string `json:"dispatchId"`
	}
	if json.Unmarshal(payload, &identity) == nil {
		taskID, dispatchID = identity.TaskID, identity.DispatchID
	}
	if dispatchID == "" && strings.HasPrefix(message.FromHandle, "dispatch:") {
		dispatchID = strings.TrimPrefix(message.FromHandle, "dispatch:")
	}
	return taskID, dispatchID
}

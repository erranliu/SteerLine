package codexapp

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestAskToolResponseUsesNativeAnswerEnvelope(t *testing.T) {
	tool := AskDynamicTool().(map[string]any)
	if tool["name"] != AskToolName {
		t.Fatalf("unexpected native ask tool: %+v", tool)
	}
	response := AskToolResponse(map[string][]string{"choice": {"a"}})
	items := response["contentItems"].([]map[string]string)
	var body struct {
		Answers map[string][]string `json:"answers"`
	}
	if len(items) != 1 || json.Unmarshal([]byte(items[0]["text"]), &body) != nil || !reflect.DeepEqual(body.Answers["choice"], []string{"a"}) {
		t.Fatalf("native ask response lost the answer: %+v", response)
	}
}

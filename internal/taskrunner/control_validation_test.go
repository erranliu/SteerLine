package taskrunner

import (
	"encoding/json"
	"strings"
	"testing"

	api "github.com/erranliu/SteerLine/internal/product/api"
)

func TestRunnerStartRequiresDisplayedTaskAndGraphVersions(t *testing.T) {
	for _, action := range []string{"start_task", "start_candidate"} {
		for _, raw := range []string{`{"task_id":"1"}`, `{"task_id":"1","expected_task_version":1}`, `{"task_id":"1","expected_graph_revision":1}`, `{"task_id":"1","expected_task_version":1,"expected_graph_revision":0}`, `{"task_id":"1","expected_task_version":1,"expected_graph_revision":1,"execution_profile_id":"configured"}`} {
			if _, err := DecodeControlPayload(api.ControlRequest{Action: action, Payload: json.RawMessage(raw)}); err == nil {
				t.Fatal("incomplete start accepted", action, raw)
			}
		}
		for _, raw := range []string{`{"task_id":"1","expected_task_version":1,"expected_graph_revision":1}`, `{"task_id":"1","expected_task_version":1,"expected_graph_revision":1,"execution_profile_id":"configured","execution_profile_digest":"` + strings.Repeat("a", 64) + `"}`} {
			if _, err := DecodeControlPayload(api.ControlRequest{Action: action, Payload: json.RawMessage(raw)}); err != nil {
				t.Fatal("current public selection rejected", action, err)
			}
		}
	}
}

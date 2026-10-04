package task

import (
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"strings"
	"testing"
)

func TestTM03Dependencies(t *testing.T) {
	graph := taskmodel.Snapshot{Authority: taskmodel.GraphAuthority{ProjectIdentity: "fixture"}, ByID: map[string]taskmodel.TaskRecord{"1": {TaskID: "1", Status: "open"}, "2": {TaskID: "2", Status: "closed"}}, Deliveries: map[string]taskmodel.DeliveryFact{"2": {TaskID: "2"}}}
	if err := taskmodel.ValidatePredecessorAddition(graph, "2", []string{"1"}); err == nil || !strings.Contains(err.Error(), "already_merged") {
		t.Fatalf("delivered successor accepted: %v", err)
	}
	graph.ByID["2"] = taskmodel.TaskRecord{TaskID: "2", Status: "discarded"}
	delete(graph.Deliveries, "2")
	if err := taskmodel.ValidatePredecessorAddition(graph, "2", []string{"1"}); err == nil || !strings.Contains(err.Error(), "task_discarded") {
		t.Fatalf("discarded successor accepted: %v", err)
	}
}

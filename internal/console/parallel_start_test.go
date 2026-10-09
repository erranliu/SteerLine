package console

import (
	"testing"
	"time"
)

func TestStartSubmissionScopesPendingRequestToTask(t *testing.T) {
	for _, status := range []string{"submitting", "submitted", "applying", "indeterminate"} {
		t.Run(status, func(t *testing.T) {
			now := time.Now().UTC()
			ref := testProject("parallel-start")
			m := modelForTest([]ProjectRef{ref}, []ProjectSnapshot{readySnapshot(ref, now)}, &testControlClient{}, now)
			id := "0123456789abcdef0123456789abcdef"
			m.projects[0].actions[id] = &actionState{RequestID: id, Action: "start_task", TaskID: "task-A", Status: status, TargetInstance: "instance-parallel-start", UpdatedAt: now}
			updated, cmd := m.createAction("start_task", "task-B")
			m = updated.(Model)
			if cmd == nil || len(m.projects[0].actions) != 2 || m.projects[0].actions[id].TaskID != "task-A" {
				t.Fatalf("task A's request blocked or was replaced by task B: %s", m.status)
			}
			for _, a := range m.projects[0].actions {
				if a.TaskID == "task-B" {
					// Stopping UI tracking never authorizes another start.
					a.Resolved = true
				}
			}
			updated, duplicate := m.createAction("start_task", "task-B")
			m = updated.(Model)
			if duplicate != nil || len(m.projects[0].actions) != 2 {
				t.Fatal("same task received a duplicate unconfirmed start")
			}
		})
	}
}

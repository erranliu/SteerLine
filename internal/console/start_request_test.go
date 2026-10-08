package console

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestStartSelectedRestoredRequestDeduplication(t *testing.T) {
	cases := []struct {
		name     string
		action   string
		status   string
		taskID   string
		resolved bool
		blocked  bool
	}{
		{"accepted decision", "answer_decision", "accepted", "task-B", true, false},
		{"pending decision", "answer_decision", "pending", "task-B", false, false},
		{"pending environment action", "open_terminal", "submitted", "task-B", false, false},
		{"submitted start", "start_task", "submitted", "task-B", false, true},
		{"accepted start", "start_task", "accepted", "task-B", false, true},
		{"unknown start", "start_task", "indeterminate", "task-B", false, true},
		{"unknown start with tracking stopped", "start_task", "indeterminate", "task-B", true, true},
		{"submitting candidate", "start_candidate", "submitting", "task-B", false, true},
		{"completed start", "start_task", "applied", "task-B", true, false},
		{"rejected candidate", "start_candidate", "rejected", "task-B", true, false},
		{"failed start", "start_task", "failed", "task-B", true, false},
		{"another task", "start_task", "submitted", "task-A", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now().UTC()
			ref := testProject("alpha")
			control := &testControlClient{}
			m := modelForTest([]ProjectRef{ref}, []ProjectSnapshot{readySnapshot(ref, now)}, control, now)
			home := t.TempDir()
			profile, prefs, diagnostic, err := acquireViewProfile(home)
			if err != nil || diagnostic != "" {
				t.Fatalf("acquire journal: %v %s", err, diagnostic)
			}
			defer profile.close()
			m.profile, m.prefs = profile, prefs
			requestID := "0123456789abcdef0123456789abcdef"
			m.projects[0].actions[requestID] = &actionState{
				RequestID: requestID, Action: tc.action, TaskID: tc.taskID,
				Status: tc.status, Resolved: tc.resolved, TargetInstance: "old-instance", UpdatedAt: now,
			}
			if err := m.saveProjectView(0); err != nil {
				t.Fatal(err)
			}
			profile.close()
			restored, err := NewModel(context.Background(), []ProjectRef{ref}, Options{
				ApplicationHome: home, Clock: m.clock,
				Clients: map[string]ConsoleClient{ref.Key: m.projects[0].client},
			})
			if err != nil {
				t.Fatal(err)
			}
			defer restored.Close()
			restored.projects[0].snapshot = readySnapshot(ref, now)
			restored.projects[0].snapshotLoaded = true
			restored.projects[0].selectedKey = "task:task-B"
			updated, cmd := restored.startSelected()
			restored = updated.(Model)
			if cmd != nil || control.calls != 0 || len(restored.projects[0].actions) != 1 {
				t.Fatal("opening the profile picker submitted or replaced a request")
			}
			if tc.blocked {
				if restored.profileChoice != nil || !strings.Contains(restored.status, requestID) {
					t.Fatalf("unconfirmed start lost its original request: %q", restored.status)
				}
			} else if restored.profileChoice == nil || restored.profileChoice.taskID != "task-B" {
				t.Fatalf("unrelated or completed request blocked profile selection: %q", restored.status)
			}
			if restored.projects[0].actions[requestID].Action != tc.action {
				t.Fatal("historical action was rewritten")
			}
		})
	}
}

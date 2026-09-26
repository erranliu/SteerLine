package console

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline"
)

func TestLegacyBeadsProjectShowsTasksAndRejectsControls(t *testing.T) {
	now := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)
	ref := ProjectRef{Key: "legacy", Name: "legacy", Legacy: &steerline.LegacyBeadsProject{}}
	snapshot := ProjectSnapshot{Legacy: &steerline.LegacyBeadsSnapshot{
		Tasks:         []steerline.LegacyBeadsTask{{ID: "slt-9", Title: "Finish PR", Status: "in_progress", Predecessors: []string{"slt-6"}}},
		Runs:          []steerline.LegacyBeadsRun{{TaskID: "slt-9", ExecutionStatus: "succeeded", FinishStatus: "pending_merge", PRURL: "https://example.test/pr/9"}},
		ServiceStatus: "running", ServiceLiveness: "live", SampledAt: now,
	}}
	control := &testControlClient{}
	m := modelForTest([]ProjectRef{ref}, []ProjectSnapshot{snapshot}, control, now)
	m.projects[0].selectedKey = "legacy:slt-9"
	rows := m.rows(0)
	if len(rows) != 1 || rows[0].kind != rowLegacy || rows[0].taskID != "slt-9" {
		t.Fatalf("legacy task missing from console: %+v", rows)
	}
	detail := strings.Join(renderLegacyTaskDetail(m.projects[0], snapshot.Legacy.Tasks[0]), "\n")
	for _, want := range []string{"slt-6", "pending_merge", "https://example.test/pr/9", "只读"} {
		if !strings.Contains(detail, want) {
			t.Fatalf("legacy detail missing %q: %s", want, detail)
		}
	}
	updated, cmd := m.startSelected()
	m = updated.(Model)
	if cmd != nil || control.calls != 0 || !strings.Contains(m.status, "只读") {
		t.Fatalf("legacy task reached native start: status=%q calls=%d", m.status, control.calls)
	}
	client := FileControlClient{}
	if _, err := client.Submit(context.Background(), ref, "start_task", nil, "req", "instance", nil); err == nil {
		t.Fatal("legacy project accepted a native control request")
	}
	if _, _, err := client.SubmitServiceAction(context.Background(), ref, "stop_service", "", "test", 1, "cmd", nil); err == nil {
		t.Fatal("legacy project accepted a service command")
	}
}

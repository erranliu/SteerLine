package console

import (
	api "github.com/erranliu/SteerLine/internal/product/api"
	"strings"
	"testing"
	"time"
)

func TestNativeTaskAndItsRunShareOneListRow(t *testing.T) {
	now := time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)
	ref := testProject("native")
	ref.Presentation.TaskAuthority = "native"
	snapshot := readySnapshot(ref, now)
	snapshot.Tasks.Tasks = []api.TaskCandidate{{TaskID: "102", Title: "Native branch A"}}
	run := &api.CandidateSummary{RunID: strings.Repeat("a", 32), TaskID: "102", DomainTaskID: "102", Title: "Native branch A", TaskStatus: "in_progress", ExecutionStatus: "running", FinishTaskStatus: "pending", Process: &api.WorkerLocation{Terminal: "term-native-102"}}
	snapshot.Observation.Page.Runs = []*api.CandidateSummary{run}
	snapshot.NativeTasks = &api.TaskPage{Tasks: []api.TaskDetail{
		{TaskSummary: api.TaskSummary{TaskID: "102", Title: "Native branch A", Status: "in_progress"}, Successors: []string{"104"}},
		{TaskSummary: api.TaskSummary{TaskID: "103", Title: "Native branch B", Status: "open"}},
	}}
	m := modelForTest([]ProjectRef{ref}, []ProjectSnapshot{snapshot}, &testControlClient{}, now)
	m.projects[0].selectedKey = "task:102"
	rows := m.rows(0)
	if len(rows) != 2 || rows[0].kind != rowRun || rows[0].key != "task:"+run.TaskID || rows[1].taskID != "103" {
		t.Fatalf("native task and run were not coalesced in task order: %+v", rows)
	}
	detail := strings.Join(renderDetail(m, m.projects[0], &rows[0], 120, 60), "\n")
	if !strings.Contains(detail, "后继任务：104") || !strings.Contains(detail, "执行：running") || !strings.Contains(detail, "宿主终端标识：term-native-102") {
		t.Fatalf("coalesced row lost task relations or execution detail: %s", detail)
	}
	m.projects[0].refreshID = 1
	updated, _ := m.Update(projectMsg{key: ref.Key, identity: ref.Identity, requestID: 1, snapshot: snapshot})
	m = updated.(Model)
	if m.projects[0].selectedKey != "task:"+run.TaskID || m.selectedRow(0).run == nil {
		t.Fatalf("task selection did not follow its new run: %q", m.projects[0].selectedKey)
	}
	m.projects[0].filter = 1
	if rows := m.rows(0); len(rows) != 0 {
		t.Fatalf("stale Ready candidate remained startable beside its run: %+v", rows)
	}
}

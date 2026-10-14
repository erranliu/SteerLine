package console

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	api "github.com/erranliu/SteerLine/internal/product/api"
)

type progressClient struct {
	*testClient
	receipt *api.ControlReceipt
}

func (c *progressClient) Receipt(_ context.Context, id string) (*api.ControlReceipt, error) {
	c.control.receipts++
	if c.receipt.RequestID != id {
		panic("queried a replacement request")
	}
	return c.receipt, nil
}

func TestOperationProgressFromSubmissionThroughFailure(t *testing.T) {
	now := time.Now().UTC()
	ref := testProject("progress")
	control := &testControlClient{}
	snapshot := readySnapshot(ref, now)
	m := modelForTest([]ProjectRef{ref}, []ProjectSnapshot{snapshot}, control, now)
	client := &progressClient{testClient: m.projects[0].client.(*testClient)}
	m.projects[0].client = client
	updated, submit := m.createAction("start_task", "task-B")
	m = updated.(Model)
	row := m.selectedRow(0)
	if submit == nil || rowExecutionProgress(m.projects[0], *row) != "提交中" || !strings.Contains(m.View(), "提交中") {
		t.Fatal("submission did not immediately appear in the task row")
	}
	updated, poll := m.Update(submit())
	m = updated.(Model)
	request := control.request
	client.receipt = &api.ControlReceipt{RequestID: request.RequestID, InstanceID: request.InstanceID, Action: request.Action, Status: "applying", RunID: request.RequestID, Message: "service accepted the request"}
	if poll == nil || !m.projects[0].actions[request.RequestID].InFlight || control.receipts != 0 {
		t.Fatal("submission did not schedule the original receipt immediately")
	}
	updated, refresh := m.Update(poll())
	m = updated.(Model)
	if refresh == nil || !m.projects[0].refreshing || rowExecutionProgress(m.projects[0], *m.selectedRow(0)) != "启动中" {
		t.Fatal("applying receipt did not refresh the snapshot and show startup progress")
	}
	if !strings.Contains(strings.Join(receiptSummary(m.projects[0], m.selectedRow(0)), "\n"), "service accepted") {
		t.Fatal("early receipt was hidden until its candidate appeared")
	}
	run := &api.CandidateSummary{TaskID: "task-B", RunID: request.RequestID, TaskStatus: "in_progress", ExecutionStatus: "pending"}
	snapshot.Observation.Page.Runs = []*api.CandidateSummary{run}
	client.query.snapshot = snapshot
	updated, _ = m.Update(refresh())
	m = updated.(Model)
	if rowExecutionProgress(m.projects[0], *m.selectedRow(0)) != "启动中" || run.ExecutionStatus != "pending" {
		t.Fatal("progress was lost after refresh or mutated the authoritative worker state")
	}
	updated, repeat := m.Update(actionReceiptMsg{projectKey: ref.Key, identity: ref.Identity, requestID: request.RequestID, receipt: client.receipt})
	m = updated.(Model)
	if repeat != nil {
		t.Fatal("unchanged applying receipt started another full snapshot read")
	}
	client.receipt.Status, client.receipt.Message = "failed", "worker could not start"
	updated, refresh = m.Update(actionReceiptMsg{projectKey: ref.Key, identity: ref.Identity, requestID: request.RequestID, receipt: client.receipt})
	m = updated.(Model)
	if refresh == nil || taskOperation(m.projects[0], "task-B") != nil || !strings.Contains(m.View(), "操作失败") {
		t.Fatal("terminal failure did not clear progress and show its receipt")
	}
	if control.calls != 1 || control.receipts != 1 || len(m.projects[0].actions) != 1 {
		t.Fatal("receipt tracking duplicated the original operation")
	}
}

func TestOperationProgressKeepsSiblingAndServiceIdentity(t *testing.T) {
	now := time.Now().UTC()
	ref := testProject("progress")
	m := modelForTest([]ProjectRef{ref}, []ProjectSnapshot{readySnapshot(ref, now)}, &testControlClient{}, now)
	s := &m.projects[0]
	a := &actionState{RequestID: "new", TaskID: "task-B", Action: "start_candidate", RunID: "new-run", Status: "applying", TargetInstance: "instance-progress", UpdatedAt: now}
	s.actions[a.RequestID] = a
	sibling := consoleRow{taskID: "task-B", run: &api.CandidateSummary{TaskID: "task-B", RunID: "sibling", TaskStatus: "in_progress", ExecutionStatus: "running"}}
	if got := rowExecutionProgress(*s, sibling); got != "执行中" {
		t.Fatalf("new candidate replaced a live sibling: %s", got)
	}
	pending := consoleRow{taskID: "task-B", run: &api.CandidateSummary{TaskID: "task-B", RunID: "new-run", ExecutionStatus: "pending"}}
	if got := rowExecutionProgress(*s, pending); got != "启动中" {
		t.Fatal(got)
	}
	a.TargetInstance = "previous-service"
	if taskOperation(*s, "task-B") != nil || rowExecutionProgress(*s, pending) != "待开始" {
		t.Fatal("previous service's receipt overlaid the current service")
	}
	a.TargetInstance, a.Status, a.Resolved = "instance-progress", "indeterminate", true
	if taskOperation(*s, "task-B") != nil {
		t.Fatal("identity mismatch with stopped tracking became active progress")
	}
}

type progressClock struct {
	testClock
	delay time.Duration
}

func (c *progressClock) After(delay time.Duration) <-chan time.Time {
	c.delay = delay
	ch := make(chan time.Time, 1)
	ch <- c.now
	return ch
}

func TestOperationReceiptPollingIsBriefAndDoesNotStack(t *testing.T) {
	now := time.Now().UTC()
	ref := testProject("progress")
	control := &testControlClient{}
	m := modelForTest([]ProjectRef{ref}, []ProjectSnapshot{readySnapshot(ref, now)}, control, now)
	clock := &progressClock{testClock: testClock{now: now}}
	m.clock = clock
	s := &m.projects[0]
	s.lastRefresh, s.actionPollUntil = now, now.Add(5*time.Second)
	s.actions["request"] = &actionState{RequestID: "request", Status: "submitted", InFlight: true}
	for _, tc := range []struct {
		advance, delay time.Duration
	}{{0, 100 * time.Millisecond}, {6 * time.Second, 500 * time.Millisecond}} {
		clock.now = now.Add(tc.advance)
		_, cmd := m.updateTick()
		if commands, ok := cmd().(tea.BatchMsg); ok {
			commands[len(commands)-1]()
		}
		if clock.delay != tc.delay || control.receipts != 0 {
			t.Fatal("poll interval or in-flight deduplication changed")
		}
	}
}

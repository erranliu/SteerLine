package console

import (
	"strings"
	"testing"
	"time"
)

func TestAsyncQuestionConsoleOffersOptionsWhileRunning(t *testing.T) {
	now := time.Now()
	ref := testProject("async")
	snapshot, run := decisionSnapshot(ref, now)
	run.WorkflowStatus, run.ExecutionStatus = "running", "running"
	run.Decision.Request.InteractionKind = "async"
	run.Decision.Request.BlocksProgress = false
	m := modelForTest([]ProjectRef{ref}, []ProjectSnapshot{snapshot}, &testControlClient{}, now)
	m.profile = &viewProfile{dir: t.TempDir()}
	m.projects[0].selectedKey = "task:" + run.TaskID
	if waitingForDecision(run) || candidateHumanStatus(m.projects[0], run) != "运行中 · 有待回答问题" {
		t.Fatal("async question appeared as a stopped worker")
	}
	updated, _ := m.answerSelectedDecision()
	m = updated.(Model)
	if m.answer == nil {
		t.Fatalf("running question not editable: %s", m.status)
	}
	dialog := renderAnswerDialog(m, m.projects[0], 200)
	if !strings.Contains(dialog, "选项 11") {
		t.Fatal("native options missing from dialog")
	}
}

package model

import (
	"testing"
	"time"
)

func TestWorkerStateActivityUsesOriginalFields(t *testing.T) {
	w := WorkflowRunRecord{WorkflowRunID: "run", TaskID: "task", StepID: StepImplement, Status: "running", ControlRevision: 5, InputVersion: 1}
	e := ExecutionRecord{ExecutionID: "execution", CurrentAttemptID: "attempt", Status: "active", CurrentTurnID: "original-turn"}
	if _, changed := WorkerStateActivity(&w, &e, w, e, time.Now()); changed {
		t.Fatal("unchanged polling created activity")
	}
	next := w
	next.StepID, next.Status, next.InputVersion = StepReport, "queued", 2
	ended := e
	ended.Status, ended.LastTurnOutcome = "idle", "failed"
	event, changed := WorkerStateActivity(&w, &e, next, ended, time.Now())
	if !changed || event.Meta["from"].(WorkerStateFields).WorkflowStatus != "running" || event.Meta["to"].(WorkerStateFields).WorkflowStage != StepReport || event.Meta["to"].(WorkerStateFields).ExecutionStatus != "failed" {
		t.Fatal("state names or original failure were replaced", event)
	}
	replay, _ := WorkerStateActivity(&w, &e, next, ended, time.Now().Add(time.Minute))
	if event.ItemID != replay.ItemID {
		t.Fatal("same committed transition has unstable identity")
	}
	next.ControlRevision++
	back, _ := WorkerStateActivity(&next, &ended, w, e, time.Now())
	if event.ItemID == back.ItemID {
		t.Fatal("different committed transition reused identity")
	}
	stopped := e
	at := time.Now()
	stopped.ResourceStopOperationID, stopped.ResourceStoppedAt = "original-stop", &at
	event, changed = WorkerStateActivity(&w, &e, w, stopped, at)
	if !changed || event.Meta["to"].(WorkerStateFields).ResourceStatus != "stopped" {
		t.Fatal("confirmed resource stop did not create a change")
	}
}

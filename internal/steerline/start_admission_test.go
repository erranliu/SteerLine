package steerline

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestManualStartAdmissionsClaimIndependentTasksBeforeWorkersStart(t *testing.T) {
	f := newFixture(t)
	f.c.TaskPoll = "30s"
	s := f.service(t)
	defer func() {
		if s != nil {
			_ = s.Close()
		}
	}()
	contractA := nativeContract(t, f.c, f.commit, "start-admission-a")
	contractB := nativeContract(t, f.c, f.commit, "start-admission-b")
	contractC := nativeContract(t, f.c, f.commit, "start-admission-c")
	batch, err := json.Marshal(TaskBatchInput{SchemaVersion: taskBatchSchemaVersion, ProjectID: f.c.Identity(), Tasks: []TaskBatchTask{
		{LocalKey: "a", TaskID: 701, Title: "First", Type: "task", Contract: contractA},
		{LocalKey: "b", TaskID: 702, Title: "Second", Type: "task", Contract: contractB},
		{LocalKey: "c", TaskID: 703, Title: "Rejected", Type: "task", Contract: contractC},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = seedExecutionProfileTasks(t, f.c, s.domainStore, batch); err != nil {
		t.Fatal(err)
	}
	authority, err := s.domainStore.nativeAuthority(context.Background(), f.c.Identity())
	if err != nil {
		t.Fatal(err)
	}
	manual, unpaused := "manual", false
	if _, err = (CommandService{Store: s.domainStore, Project: f.c.Identity()}).SetNativeDispatchState(context.Background(), NativeDispatchUpdate{RequestID: newID(), ExpectedAuthorityVersion: authority.AuthorityVersion, Mode: &manual, Paused: &unpaused, RequestedBy: "operator"}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	stopped := false
	go func() { done <- s.RunWithDispatch(ctx, "manual") }()
	defer func() {
		if stopped {
			return
		}
		cancel()
		if err := <-done; err != nil {
			t.Errorf("stop admission service: %v", err)
		}
	}()
	var runtime *Runtime
	deadline := time.Now().Add(10 * time.Second)
	for {
		runtime, err = LoadDatabaseRuntime(f.c)
		if err == nil && runtime != nil && runtime.Service.Ready {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("admission service did not become ready: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	instance := runtime.Service.InstanceID
	submit := func(payload map[string]string) ControlRequest {
		t.Helper()
		created := now()
		req := ControlRequest{ProtocolVersion: ControlProtocolVersion, RequestID: newID(), RepositoryIdentity: f.c.Identity(), InstanceID: instance, Action: "start_task", Payload: mustJSON(payload), RequestedBy: "operator", CreatedAt: created, ExpiresAt: created.Add(2 * time.Minute), ExpectedControlRevision: runtime.Control.Revision}
		if err := publishControlRequest(f.c.RunDir, req); err != nil {
			t.Fatal(err)
		}
		return req
	}
	first := submit(map[string]string{"task_id": "701"})
	second := submit(map[string]string{"task_id": "702"})
	rejected := submit(map[string]string{"task_id": "703", "execution_profile_id": "missing", "execution_profile_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"})
	waitCtx, stop := context.WithTimeout(context.Background(), 15*time.Second)
	defer stop()
	firstReceipt, err := WaitControlReceipt(waitCtx, f.c, first.RequestID, 20*time.Millisecond)
	if err != nil || firstReceipt.Status != "accepted" || firstReceipt.RunID == "" {
		t.Fatalf("first claim was not accepted: %+v / %v", firstReceipt, err)
	}
	secondReceipt, err := WaitControlReceipt(waitCtx, f.c, second.RequestID, 20*time.Millisecond)
	if err != nil || secondReceipt.Status != "accepted" || secondReceipt.RunID == "" || secondReceipt.RunID == firstReceipt.RunID {
		t.Fatalf("second claim did not get its own run: %+v / %v", secondReceipt, err)
	}
	duplicate := submit(map[string]string{"task_id": "701"})
	duplicateReceipt, err := WaitControlReceipt(waitCtx, f.c, duplicate.RequestID, 20*time.Millisecond)
	if err != nil || duplicateReceipt.Status != "rejected" {
		t.Fatalf("another request claimed the same task twice: %+v / %v", duplicateReceipt, err)
	}
	rejectedReceipt, err := WaitControlReceipt(waitCtx, f.c, rejected.RequestID, 20*time.Millisecond)
	if err != nil || rejectedReceipt.Status != "rejected" || rejectedReceipt.Code != "stale_profile" {
		t.Fatalf("pre-claim failure was not reported: %+v / %v", rejectedReceipt, err)
	}
	snapshot, err := s.domainStore.Query(context.Background(), f.c.Identity())
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"701", "702"} {
		var task *TaskRecord
		for i := range snapshot.Tasks {
			if snapshot.Tasks[i].TaskID == id {
				task = &snapshot.Tasks[i]
			}
		}
		if task == nil || task.Status != "in_progress" {
			t.Fatalf("task %s was not claimed before worker startup: %+v", id, task)
		}
	}
	if len(snapshot.WorkflowRuns) != 2 || len(snapshot.Attempts) != 2 {
		t.Fatalf("independent claims did not save planned steps: %d workflows, %d attempts", len(snapshot.WorkflowRuns), len(snapshot.Attempts))
	}
	for _, workflow := range snapshot.WorkflowRuns {
		if workflow.Phase != "queued" {
			t.Fatalf("worker started before the admission-only check: %+v", workflow)
		}
	}
	for _, task := range snapshot.Tasks {
		if task.TaskID == "703" && task.Status != "open" {
			t.Fatalf("rejected pre-claim start changed task state: %+v", task)
		}
	}
	cancel()
	stopErr := <-done
	stopped = true
	if stopErr != nil {
		t.Fatalf("stop admission service: %v", stopErr)
	}
	if err := os.Remove(controlReceiptPath(f.c.RunDir, second.RequestID)); err != nil {
		t.Fatal(err)
	}
	if err := admitStartRequest(context.Background(), f.c, s.domainStore, instance, second); err != nil {
		t.Fatalf("lost receipt could not replay the original claim: %v", err)
	}
	replayed, err := ReadVerifiedControlReceipt(f.c, second.RequestID)
	if err != nil || replayed.Status != "accepted" || replayed.RunID != secondReceipt.RunID {
		t.Fatalf("replay created a second run: %+v / %v", replayed, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = nil
	f.c.TaskPoll = "100ms"
	seedFakeCapabilities(t, f.c)
	restarted := f.service(t)
	defer restarted.Close()
	resumeCtx, stopResume := context.WithCancel(context.Background())
	resumed := make(chan error, 1)
	go func() { resumed <- restarted.RunWithDispatch(resumeCtx, "manual") }()
	defer func() {
		stopResume()
		if err := <-resumed; err != nil {
			t.Errorf("stop resumed service: %v", err)
		}
	}()
	started := false
	deadline = time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		state, readErr := LoadDatabaseRuntime(f.c)
		if readErr == nil && state != nil {
			for _, workflow := range state.WorkflowRuns {
				if (workflow.WorkflowRunID == firstReceipt.RunID || workflow.WorkflowRunID == secondReceipt.RunID) && workflow.Phase == "executing" && state.Service.Ready {
					started = true
				}
			}
		}
		if started {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !started {
		t.Fatal("restarted service did not advance a queued prepare")
	}
	current, err := LoadDatabaseRuntime(f.c)
	if err != nil {
		t.Fatal(err)
	}
	created := now()
	third := ControlRequest{ProtocolVersion: ControlProtocolVersion, RequestID: newID(), RepositoryIdentity: f.c.Identity(), InstanceID: current.Service.InstanceID, Action: "start_task", Payload: mustJSON(map[string]string{"task_id": "703"}), RequestedBy: "operator", CreatedAt: created, ExpiresAt: created.Add(2 * time.Minute), ExpectedControlRevision: current.Control.Revision}
	if err := publishControlRequest(f.c.RunDir, third); err != nil {
		t.Fatal(err)
	}
	thirdWaitCtx, stopThird := context.WithTimeout(context.Background(), 10*time.Second)
	defer stopThird()
	thirdReceipt, err := WaitControlReceipt(thirdWaitCtx, f.c, third.RequestID, 20*time.Millisecond)
	if err != nil || thirdReceipt.Status != "accepted" || thirdReceipt.RunID == "" {
		t.Fatalf("earlier step processing blocked a later independent claim: %+v / %v", thirdReceipt, err)
	}
}

func TestStartAdmissionUsesTaskCASInsteadOfGlobalControlRevision(t *testing.T) {
	created := now()
	req := ControlRequest{ProtocolVersion: ControlProtocolVersion, RequestID: newID(), RepositoryIdentity: "repo", InstanceID: "instance", Action: "start_task", RequestedBy: "operator", CreatedAt: created, ExpiresAt: created.Add(time.Minute), ExpectedControlRevision: 3}
	if code, err := validateControlRequest(req, "instance", "repo", 4, created); err != nil {
		t.Fatalf("another task's control revision rejected this start: %s / %v", code, err)
	}
	req.Action = "set_dispatch_mode"
	if code, err := validateControlRequest(req, "instance", "repo", 4, created); err == nil || code != "stale_revision" {
		t.Fatalf("mode change lost its global revision guard: %s / %v", code, err)
	}
}

package reconstruction

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/hostcontract"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func TestHostCheckpointProjectionPreservesFactsAndOwnership(t *testing.T) {
	at := time.Date(2026, 10, 17, 1, 2, 3, 0, time.UTC)
	task, diagnostic, startup := "task", "original diagnostic", "startup"
	cp := hostcontract.ServiceCheckpoint{SchemaVersion: hostcontract.ServiceCheckpointSchema, Revision: 7, UpdatedAt: at, HeartbeatAt: at,
		Repository: hostcontract.Repository{Identity: "project", Path: "repo", Remote: "origin", Branch: "main"}, ReservedTask: &task,
		Service:   hostcontract.ServiceState{RunnerBuild: &runnercontract.BuildPin{SHA256: "original-build"}, InstanceID: "instance", PID: 42, Status: "running", Ready: true, Error: &diagnostic, StartupID: &startup, ProcessIdentity: &hostcontract.ProcessIdentity{Kind: "os", PID: 42, Created: "original-process"}, Capabilities: []string{"task_dag"}},
		Operation: &hostcontract.ControlOperation{RequestID: "request", RequestDigest: "digest", Phase: "completed", Payload: json.RawMessage(`{"original":true}`), Result: &hostcontract.ControlReceipt{RequestID: "request", Status: "applied", AcceptedAt: &at, CompletedAt: &at}},
		Blockers:  []hostcontract.ControlBlocker{{RequestID: "blocked", Reason: "original"}}}
	before, _ := json.Marshal(cp)
	rt := RuntimeFromCheckpoint(cp, "auto", false, 19)
	if rt.Control.DispatchMode != "auto" || rt.Control.DispatchPaused || rt.Control.Revision != 19 || rt.Service.InstanceID != "instance" || rt.Service.ProcessIdentity.Created != "original-process" || rt.Revision != 7 || !rt.HeartbeatAt.Equal(at) || !rt.SlotOccupied || *rt.CurrentTask != "task" {
		t.Fatal("checkpoint facts or separate task authority changed", rt)
	}
	if got := Checkpoint(rt); !reflect.DeepEqual(got, cp) {
		t.Fatalf("host checkpoint roundtrip changed: %+v", got)
	}
	mutate := func(v *Runtime) {
		v.Service.RunnerBuild.SHA256 = "changed"
		v.Service.ProcessIdentity.Created = "changed"
		v.Service.Capabilities[0] = "changed"
		*v.Service.Error = "changed"
		*v.Service.StartupID = "changed"
		*v.CurrentTask = "changed"
		v.Control.Operation.Payload[0] = 'x'
		*v.Control.Operation.Result.AcceptedAt = at.Add(time.Hour)
		*v.Control.Operation.Result.CompletedAt = at.Add(time.Hour)
		v.Control.Blockers[0].Reason = "changed"
	}
	mutate(&rt)
	after, _ := json.Marshal(cp)
	if string(before) != string(after) {
		t.Fatal("runtime projection mutated sampled host facts")
	}
	rt = RuntimeFromCheckpoint(cp, "manual", true, 21)
	projected := Checkpoint(rt)
	// A saved checkpoint is also an independent value, including nested receipt
	// timestamps and the reserved task pointer.
	copyRuntime := RuntimeFromCheckpoint(projected, "manual", true, 21)
	copyRuntime.CurrentTask = projected.ReservedTask
	copyRuntime.Control.Operation = projected.Operation
	mutate(&copyRuntime)
	if *rt.CurrentTask != "task" || !rt.Control.Operation.Result.AcceptedAt.Equal(at) || !rt.Control.Operation.Result.CompletedAt.Equal(at) {
		t.Fatal("checkpoint output retained mutable runtime ownership")
	}
	rt.SchemaVersion = 0
	if metadata := MetadataOf(rt); metadata.Checkpoint.SchemaVersion != 0 || metadata.AuthorityRevision != 21 {
		t.Fatal("metadata hid the sampled schema or task authority")
	}
}

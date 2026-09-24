package steerline

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecutionRecordsPinBindingsAndOperations(t *testing.T) {
	r := executionRecords{root: t.TempDir()}
	ref := ExecutionRef{Version: 1, Backend: BackendLocal, Protocol: ProtocolLocalProcess, Target: ExecutionTarget{ID: "local:test"}, Agent: AgentSpec{Name: "codex"}, RunID: newID(), AttemptID: newID(), WorkspacePath: t.TempDir(), WorkspaceBranch: "codex/test", BaseCommit: "0123456789abcdef0123456789abcdef01234567", External: ProcessIdentity{Kind: "os", PID: 42, Created: "creation-1", Executable: "codex"}, Generation: "creation-1"}
	if err := r.saveBinding(ref); err != nil {
		t.Fatal(err)
	}
	loaded, err := r.loadBinding(ref.RunID, ref.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	if digestValue(loaded) != digestValue(ref) {
		t.Fatalf("binding changed across persistence: %#v", loaded)
	}
	changed := ref
	changed.Target = ExecutionTarget{ID: "local:other"}
	if err := r.saveBinding(changed); err == nil {
		t.Fatal("expected immutable binding conflict")
	}

	op := Operation{Version: 1, ID: "start-" + ref.RunID, RunID: ref.RunID, AttemptID: ref.AttemptID, Backend: ref.Backend, Protocol: ref.Protocol, Target: ref.Target, Action: "start", PayloadDigest: digestValue("fixed"), Status: OperationPending}
	if err := r.saveOperation(op); err != nil {
		t.Fatal(err)
	}
	op.Status = OperationInFlight
	if err := r.saveOperation(op); err != nil {
		t.Fatal(err)
	}
	loadedOp, err := r.loadOperation(op.RunID, op.AttemptID, op.ID)
	if err != nil || loadedOp.Status != OperationInFlight {
		t.Fatalf("operation update not persisted: %#v, %v", loadedOp, err)
	}
	op.PayloadDigest = digestValue("different")
	if err := r.saveOperation(op); err == nil {
		t.Fatal("expected operation payload conflict")
	}
	if _, err := r.loadBinding(ref.RunID, "../escape"); err == nil {
		t.Fatalf("unexpected invalid binding lookup result: %v", err)
	}
}

func TestExecutionBackendCommonCapabilityContract(t *testing.T) {
	c := Config{Host: "local", Codex: Command{Executable: "codex"}, Orca: OrcaConfig{Executable: "orca"}}
	backends := []ExecutionBackend{localExecutionBackend{config: c}, orcaTerminalBackend{config: c}}
	for _, backend := range backends {
		kind, protocol := backend.Identity()
		report, err := backend.Probe(context.Background(), ExecutionTarget{ID: "fixture"}, AgentSpec{Name: "codex"})
		if err != nil {
			t.Fatal(err)
		}
		if report.Backend != kind || report.Protocol != protocol || report.CheckedAt.IsZero() {
			t.Fatalf("probe lost fixed backend identity: %#v", report)
		}
		for _, capability := range report.Capabilities {
			if capability.Status != CapabilityNotTested {
				t.Fatalf("static declaration claimed verification: %#v", capability)
			}
		}
	}
}

func TestLocalReadOutputUsesBoundedExecutionCursor(t *testing.T) {
	runID := newID()
	attemptID := runID
	runDir := t.TempDir()
	logDir := filepath.Join(runDir, "runs", runID)
	if err := os.MkdirAll(logDir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(logDir, "events.jsonl")
	if err := os.WriteFile(path, []byte("abcdefghij"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Host: "local", RunDir: runDir}
	backend := localExecutionBackend{config: cfg}
	ref := ExecutionRef{Backend: BackendLocal, Protocol: ProtocolLocalProcess, Target: targetFor(cfg), RunID: runID, AttemptID: attemptID}
	first, err := backend.ReadOutput(context.Background(), ref, "", 4)
	if err != nil || string(first.Data) != "abcd" || first.NextCursor == "" {
		t.Fatalf("first bounded read: %#v, %v", first, err)
	}
	if err = os.WriteFile(path, []byte("abcdefghijkl"), 0600); err != nil {
		t.Fatal(err)
	}
	second, err := backend.ReadOutput(context.Background(), ref, first.NextCursor, 4)
	if err != nil || string(second.Data) != "efgh" || second.SourceChanged || second.Gap {
		t.Fatalf("append read: %#v, %v", second, err)
	}
	if err = os.WriteFile(path, []byte("zzzzzzzz"), 0600); err != nil {
		t.Fatal(err)
	}
	changed, err := backend.ReadOutput(context.Background(), ref, first.NextCursor, 4)
	if err != nil || string(changed.Data) != "zzzz" || !changed.SourceChanged {
		t.Fatalf("replacement read: %#v, %v", changed, err)
	}
	other := ref
	other.AttemptID = newID()
	if _, err = backend.ReadOutput(context.Background(), other, first.NextCursor, 4); err == nil || !strings.Contains(err.Error(), "cursor") {
		t.Fatalf("cross-attempt cursor was accepted: %v", err)
	}
}

func TestOperationRecordRejectsCorruptionBeforeUse(t *testing.T) {
	r := executionRecords{root: t.TempDir()}
	run, attempt := newID(), newID()
	if err := os.MkdirAll(r.operationPath(run, attempt, "op"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := r.loadOperation(run, attempt, "op"); err == nil {
		t.Fatal("expected operation path conflict to be reported")
	}
}

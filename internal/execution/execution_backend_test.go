package execution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
)

func TestExecutionRecordsPinBindingsAndOperations(t *testing.T) {
	root := t.TempDir()
	reader, err := sqlite.OpenSQLiteStore(filepath.Join(root, "steerline.db"))
	require(t, err)
	defer reader.Close()
	r := executionRecords{root: root, reader: reader}
	ref := executionbackend.ExecutionRef{Version: 1, Backend: executionbackend.BackendLocal, Protocol: executionbackend.ProtocolLocalProcess, Target: executionbackend.ExecutionTarget{ID: "local:test"}, Agent: executionbackend.AgentSpec{Name: "fixture"}, RunID: newID(), AttemptID: newID(), WorkspacePath: t.TempDir(), WorkspaceBranch: "fixture/test", BaseCommit: "0123456789abcdef0123456789abcdef01234567", External: executionbackend.ProcessIdentity{Kind: "os", PID: 42, Created: "creation-1", Executable: "fixture"}, Generation: "creation-1"}
	if err := r.saveBinding(ref); err != nil {
		t.Fatal(err)
	}
	loaded, err := r.loadBinding(ref.RunID, ref.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	normalized := ref
	normalized.WorkspacePath = canonical(ref.WorkspacePath)
	if digestValue(loaded) != digestValue(normalized) {
		t.Fatalf("binding changed across persistence: %#v", loaded)
	}
	if runtime.GOOS == "windows" {
		caseChanged := ref
		caseChanged.WorkspacePath = strings.ToUpper(ref.WorkspacePath)
		if err := r.saveBinding(caseChanged); err != nil {
			t.Fatalf("same Windows workspace path conflicted after restart: %v", err)
		}
	}
	changed := ref
	changed.Target = executionbackend.ExecutionTarget{ID: "local:other"}
	if err := r.saveBinding(changed); err == nil {
		t.Fatal("expected immutable binding conflict")
	}

	op := executionbackend.Operation{Version: 1, ID: "start-" + ref.RunID, RunID: ref.RunID, AttemptID: ref.AttemptID, Backend: ref.Backend, Protocol: ref.Protocol, Target: ref.Target, Action: "start", PayloadDigest: digestValue("fixed"), Status: executionbackend.OperationPending}
	if err := r.saveOperation(op); err != nil {
		t.Fatal(err)
	}
	op.Status = executionbackend.OperationInFlight
	if err := r.saveOperation(op); err != nil {
		t.Fatal(err)
	}
	loadedOp, err := r.loadOperation(op.RunID, op.AttemptID, op.ID)
	if err != nil || loadedOp.Status != executionbackend.OperationInFlight {
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

func TestCapabilityReportRejectsRetiredWholeRunActions(t *testing.T) {
	for _, name := range []string{"reply_continue", "retry"} {
		report := executionbackend.CapabilityReport{Version: 1, Backend: executionbackend.BackendLocal, Protocol: executionbackend.ProtocolLocalProcess, Target: executionbackend.ExecutionTarget{ID: "local"}, Agent: executionbackend.AgentSpec{Name: "fixture"}, PlatformVersion: "fixture", CheckedAt: now(), Capabilities: []executionbackend.Capability{{Name: name, Status: executionbackend.CapabilityUnsupported}}}
		if err := (executionRecords{root: t.TempDir(), resources: fixtureResources{}}).saveCapabilityReport(report); err == nil {
			t.Fatalf("capability report retained retired whole-run action %q", name)
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
	ref := executionbackend.ExecutionRef{Backend: executionbackend.BackendLocal, Protocol: executionbackend.ProtocolLocalProcess, Target: TargetFor(cfg), RunID: runID, AttemptID: attemptID}
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
	root := t.TempDir()
	reader, err := sqlite.OpenSQLiteStore(filepath.Join(root, "steerline.db"))
	require(t, err)
	defer reader.Close()
	r := executionRecords{root: root, reader: reader}
	run, attempt := newID(), newID()
	if err := r.writeRecord("execution_operations", run, attempt, "op", []byte("{broken")); err != nil {
		t.Fatal(err)
	}
	if _, err := r.loadOperation(run, attempt, "op"); err == nil {
		t.Fatal("expected operation path conflict to be reported")
	}
}

func TestNativeCapabilityCheckKeepsFixedAgent(t *testing.T) {
	c := workerContextConfigFixture()
	c.RunDir = t.TempDir()
	c.Resources = fixtureResources{}
	backend := BackendFor(c)
	fixedTarget, fixedAgent := TargetFor(c), AgentFor(c)
	report, err := backend.Probe(context.Background(), fixedTarget, fixedAgent)
	require(t, err)
	data := []byte("isolated neutral workspace inspection fact")
	sum := sha256.Sum256(data)
	evidencePath := filepath.Join(c.RunDir, "fixture-evidence.json")
	require(t, fixtureAtomicWrite(evidencePath, data))
	for i := range report.Capabilities {
		if report.Capabilities[i].Name == "workspace_inspection" {
			report.Capabilities[i].Status = executionbackend.CapabilityVerified
			report.Capabilities[i].Evidence = evidencePath + "#sha256=" + hex.EncodeToString(sum[:])
		}
	}
	require(t, (executionRecords{root: c.RunDir, resources: c.Resources}).saveCapabilityReport(report))
	c.Agent = "changed-default"
	s := &Service{Config: c}
	if _, err := s.requireBackendCapabilitiesFor(context.Background(), backend, fixedTarget, fixedAgent, "workspace_inspection"); err != nil {
		t.Fatalf("fixed capability was lost after changing the default agent: %v", err)
	}
	if _, err := s.requireBackendCapabilities(context.Background(), backend, "workspace_inspection"); err == nil {
		t.Fatal("new default agent reused evidence for another fixed agent")
	}
}

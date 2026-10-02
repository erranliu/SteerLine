package steerline

import (
	"context"

	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
)

func TestExecutionRecordsPinBindingsAndOperations(t *testing.T) {
	r := executionRecords{root: t.TempDir()}
	ref := executionapi.ExecutionRef{Version: 1, Backend: executionapi.BackendLocal, Protocol: executionapi.ProtocolLocalProcess, Target: executionapi.ExecutionTarget{ID: "local:test"}, Agent: executionapi.AgentSpec{Name: "codex"}, RunID: newID(), AttemptID: newID(), WorkspacePath: t.TempDir(), WorkspaceBranch: "codex/test", BaseCommit: "0123456789abcdef0123456789abcdef01234567", External: executionapi.ProcessIdentity{Kind: "os", PID: 42, Created: "creation-1", Executable: "codex"}, Generation: "creation-1"}
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
	changed.Target = executionapi.ExecutionTarget{ID: "local:other"}
	if err := r.saveBinding(changed); err == nil {
		t.Fatal("expected immutable binding conflict")
	}

	op := executionapi.Operation{Version: 1, ID: "start-" + ref.RunID, RunID: ref.RunID, AttemptID: ref.AttemptID, Backend: ref.Backend, Protocol: ref.Protocol, Target: ref.Target, Action: "start", PayloadDigest: digestValue("fixed"), Status: executionapi.OperationPending}
	if err := r.saveOperation(op); err != nil {
		t.Fatal(err)
	}
	op.Status = executionapi.OperationInFlight
	if err := r.saveOperation(op); err != nil {
		t.Fatal(err)
	}
	loadedOp, err := r.loadOperation(op.RunID, op.AttemptID, op.ID)
	if err != nil || loadedOp.Status != executionapi.OperationInFlight {
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
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	c := Config{Host: "local", Agent: "codex", Repository: t.TempDir(), CommandTimeout: "5s"}
	setTestImplementationProfiles(t, &c, codexworker.ProfileConfig{Executable: exe}, nil)
	orcaConfig := c
	orcaConfig.Host = "orca"
	setTestImplementationProfiles(t, &orcaConfig, codexworker.ProfileConfig{Executable: exe}, &orcaworker.ProfileConfig{Executable: exe, Args: []string{"--fake", "orca", t.TempDir()}, Shell: "cmd.exe", WaitTimeout: "5s"})
	backends := []executionapi.ExecutionBackend{localExecutionBackend{config: c}, orcaworker.NewBackend(nativeOrcaConfig(orcaConfig))}
	for index, backend := range backends {
		kind, protocol := backend.Identity()
		targets := []executionapi.ExecutionTarget{testLocalTarget(), testTerminalTarget(orcaConfig)}
		report, err := backend.Probe(context.Background(), targets[index], executionapi.AgentSpec{Name: "codex"})
		if err != nil {
			t.Fatal(err)
		}
		if report.Backend != kind || report.Protocol != protocol || report.CheckedAt.IsZero() {
			t.Fatalf("probe lost fixed backend identity: %#v", report)
		}
		for _, capability := range report.Capabilities {
			if capability.Status == executionapi.CapabilityVerified {
				t.Fatalf("static declaration claimed verification: %#v", capability)
			}
		}
	}
}

func TestCapabilityReportRejectsRetiredWholeRunActions(t *testing.T) {
	for _, name := range []string{"reply_continue", "retry"} {
		report := executionapi.CapabilityReport{Version: 1, Backend: executionapi.BackendLocal, Protocol: executionapi.ProtocolLocalProcess, Target: executionapi.ExecutionTarget{ID: "local"}, Agent: executionapi.AgentSpec{Name: "codex"}, PlatformVersion: "fixture", CheckedAt: now(), Capabilities: []executionapi.Capability{{Name: name, Status: executionapi.CapabilityUnsupported}}}
		if err := (executionRecords{root: t.TempDir()}).saveCapabilityReport(report); err == nil {
			t.Fatalf("capability report retained retired whole-run action %q", name)
		}
	}
}

func TestNativeCapabilityCheckKeepsFixedAgent(t *testing.T) {
	f := newFixture(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	f.c.Host = "orca"
	f.c.Agent = "codex"
	f.c.Host, f.c.Agent = "orca", "codex"
	setTestOrcaProfile(t, &f.c, orcaworker.ProfileConfig{Executable: exe, Args: []string{"--fake", "orca", f.c.RunDir}, Shell: "cmd.exe", WaitTimeout: "5s"})
	seedFakeCapabilities(t, f.c)
	fixedTarget, fixedAgent := targetFor(f.c), agentFor(f.c)
	backend := backendFor(f.c)
	f.c.Agent = "pi"
	s := &Service{Config: f.c}
	if _, err := s.requireBackendCapabilitiesFor(context.Background(), backend, fixedTarget, fixedAgent, "workspace_inspection"); err != nil {
		t.Fatalf("native capability was lost after changing the default agent: %v", err)
	}
	if _, err := s.requireBackendCapabilities(context.Background(), backendFor(f.c), "workspace_inspection"); err == nil {
		t.Fatal("new default agent reused evidence for another fixed agent")
	}
}

func TestLocalObservationRejectsReusedPIDIdentity(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows process creation identity")
	}
	identity, err := currentProcessIdentity()
	if err != nil {
		t.Fatal(err)
	}
	identity.Created = "different-process-generation"
	cfg := Config{Host: "local"}
	ref := executionapi.ExecutionRef{Backend: executionapi.BackendLocal, Protocol: executionapi.ProtocolLocalProcess, Target: targetFor(cfg), External: *identity}
	_, err = (localExecutionBackend{config: cfg}).Observe(context.Background(), ref)
	var backendErr *executionapi.BackendError
	if !errors.As(err, &backendErr) || backendErr.Code != executionapi.BackendIdentityMismatch {
		t.Fatalf("reused PID identity was accepted: %v", err)
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
	ref := executionapi.ExecutionRef{Backend: executionapi.BackendLocal, Protocol: executionapi.ProtocolLocalProcess, Target: targetFor(cfg), RunID: runID, AttemptID: attemptID}
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
	if err := r.writeRecord("execution_operations", run, attempt, "op", []byte("{broken")); err != nil {
		t.Fatal(err)
	}
	if _, err := r.loadOperation(run, attempt, "op"); err == nil {
		t.Fatal("expected operation path conflict to be reported")
	}
}

func TestOrcaObservationRejectsReplacedIncarnationAndConnection(t *testing.T) {
	dir := t.TempDir()
	cfg := orcaTestConfig(t, dir, "fail")
	if err := writeFakeOrcaState(dir, fakeOrcaState{Handle: "term-fixed", Incarnation: "inc-original"}); err != nil {
		t.Fatal(err)
	}
	ref := executionapi.ExecutionRef{Backend: executionapi.BackendOrca, Protocol: executionapi.ProtocolOrcaTerminalV2, Target: targetFor(cfg), Agent: executionapi.AgentSpec{Name: "codex"}, External: executionapi.ProcessIdentity{Kind: "orca", Terminal: "term-fixed", RuntimeID: "fake-runtime", Incarnation: "inc-original"}}
	backend := orcaworker.NewBackend(nativeOrcaConfig(cfg))
	if obs, err := backend.Observe(context.Background(), ref); err != nil || obs.Liveness != "live" {
		t.Fatalf("fixed terminal was not live: %#v, %v", obs, err)
	}
	t.Setenv("STEERLINE_FAKE_ORCA_INCARNATION", "inc-replacement")
	var mismatch *executionapi.BackendError
	if _, err := backend.Observe(context.Background(), ref); !errors.As(err, &mismatch) || mismatch.Code != executionapi.BackendIdentityMismatch {
		t.Fatalf("replacement incarnation was accepted: %v", err)
	}
	t.Setenv("STEERLINE_FAKE_ORCA_INCARNATION", "")
	orcaSettings := orcaProfileSettings(cfg)
	orcaSettings.Args = append(orcaSettings.Args, "different-connection")
	setTestOrcaProfile(t, &cfg, orcaSettings)
	if _, err := (orcaworker.NewBackend(nativeOrcaConfig(cfg))).Observe(context.Background(), ref); !errors.As(err, &mismatch) || mismatch.Code != executionapi.BackendIdentityMismatch {
		t.Fatalf("changed connection was accepted: %v", err)
	}
}

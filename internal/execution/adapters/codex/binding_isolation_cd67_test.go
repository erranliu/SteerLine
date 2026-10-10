package codexworker_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"testing"

	codexworker "github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func TestCD63BindingCodecIsolation(t *testing.T) {
	record := bindingRecord{Protocol: string(executionbackend.ProtocolLocalCodexAppServer)}
	setBinding(t, &record, "ws://127.0.0.1:12345", "native-generation", "00000000-0000-0000-0000-000000000008")
	snapshotFor := func(value *bindingRecord) executionbackend.LifecycleSnapshot {
		return executionbackend.LifecycleSnapshot{
			Session:             executionbackend.SessionContext{ProjectID: value.ProjectIdentity, WorkflowRunID: value.WorkflowRunID, ExecutionID: value.ExecutionID, AttemptID: value.CurrentAttemptID, SessionID: value.SessionID, Generation: value.SessionGeneration},
			ConfigurationDigest: value.ConfigurationDigest, ResourceGeneration: value.Generation,
			Implementation: cloneData(value.ImplementationData),
			PendingTurn:    &executionbackend.TurnInput{CommandID: newID(), Prompt: "fixed instruction"},
		}
	}
	t.Run("duplicate binding is an error", func(t *testing.T) {
		copy := record
		copy.ImplementationData = append(cloneData(record.ImplementationData), record.ImplementationData[0].Clone())
		if _, err := (codexworker.ExecutionLifecycle{}).Next(context.Background(), snapshotFor(&copy)); err == nil {
			t.Fatal("duplicate binding was treated as an unbound execution")
		}
	})
	t.Run("codec must belong to protocol", func(t *testing.T) {
		copy := record
		foreign, err := executionbackend.NewImplementationDataEnvelope(executionbackend.ImplementationDataEnvelope{CodecVersion: 1, Purpose: executionbackend.ImplementationResourcePurpose, ImplementationKey: "orca.terminal-state", ProjectID: copy.ProjectIdentity, ExecutionID: copy.ExecutionID, AttemptID: copy.CurrentAttemptID, Generation: copy.Generation, ConfigDigest: copy.ConfigurationDigest}, []byte(`{"process":{}}`))
		require(t, err)
		copy.ImplementationData = []executionbackend.ImplementationDataEnvelope{foreign}
		if _, err := (codexworker.ExecutionLifecycle{}).Next(context.Background(), snapshotFor(&copy)); err == nil {
			t.Fatal("Codex implementation accepted an Orca binding")
		}
	})
	t.Run("seed cannot stand in for a bound process", func(t *testing.T) {
		copy := record
		removeEnvelope(&copy, codexworker.ExecutionBindingCodecKey)
		setBinding(t, &copy, "", "", "00000000-0000-0000-0000-000000000008")
		copy.Process = &executionbackend.ProcessIdentity{Kind: "os", PID: 1, Created: "process-generation"}
		workspace, serverDir := t.TempDir(), t.TempDir()
		ref := executionbackend.ExecutionRef{Version: executionbackend.ExecutionRecordVersion, Backend: executionbackend.BackendLocal, Protocol: executionbackend.ProtocolLocalCodexAppServer, Target: copy.Target, Agent: executionbackend.AgentSpec{Name: "codex"}, RunID: copy.WorkflowRunID, AttemptID: copy.CurrentAttemptID, WorkspacePath: workspace, External: *copy.Process, Generation: copy.SessionGeneration}
		session := executionbackend.SessionContext{ProjectID: copy.ProjectIdentity, TaskID: "task", WorkflowRunID: copy.WorkflowRunID, ExecutionID: copy.ExecutionID, AttemptID: copy.CurrentAttemptID, Agent: executionbackend.AgentSpec{Name: "codex"}, Target: copy.Target, SessionID: copy.SessionID}
		snapshot := executionbackend.LifecycleSnapshot{Ref: ref, Session: session, ConfigurationDigest: copy.ConfigurationDigest, ResourceGeneration: copy.Generation, Implementation: cloneData(copy.ImplementationData), Workspace: executionbackend.WorkspaceIntent{Path: workspace}}
		input := executionbackend.TurnInput{CommandID: newID(), Prompt: "original saved input"}
		operation := executionbackend.Operation{Version: 1, ID: newID(), RunID: copy.WorkflowRunID, AttemptID: copy.CurrentAttemptID, Backend: ref.Backend, Protocol: ref.Protocol, Target: ref.Target, Action: string(executionbackend.LifecyclePrepareSession), PayloadDigest: operationDigest(mustJSON(input)), Status: executionbackend.OperationInFlight, Ref: &ref}
		operation.LifecycleIdentity = &executionbackend.LifecycleOperationIdentity{ProjectID: snapshot.Session.ProjectID, ExecutionID: snapshot.Session.ExecutionID, ConfigurationDigest: snapshot.ConfigurationDigest, ResourceGeneration: snapshot.ResourceGeneration, InputBindingDigest: executionbackend.LifecycleBindingDigest(snapshot), Ref: snapshot.Ref}
		startCalled := false
		lifecycle := codexworker.ExecutionLifecycle{Backend: codexworker.Backend{Config: codexworker.Config{Executable: os.Args[0], Workspace: workspace, RunDir: t.TempDir(), RunID: copy.WorkflowRunID, ServerDir: serverDir, Sandbox: "workspace-write", WindowsSandbox: "elevated"}, Process: codexworker.ProcessPort{Start: func(*exec.Cmd) (executionbackend.StopHandle, error) {
			startCalled = true
			return nil, errors.New("unexpected process start")
		}, Inspect: func(*executionbackend.ProcessIdentity) string { return "matching process still exists" }, NewGeneration: newID, WriteAtomic: AtomicWrite}}}
		receipt, _, err := lifecycle.Execute(context.Background(), executionbackend.LifecycleRequest{Snapshot: snapshot, Operation: operation, Input: executionbackend.LifecycleInput{Turn: &input}})
		if err == nil || startCalled || receipt.Disposition != executionbackend.OperationUnknown {
			t.Fatalf("recovery seed authorized replacing its missing original process receipt: receipt=%+v err=%v start_called=%v", receipt, err, startCalled)
		}
	})

}

type bindingRecord struct {
	ProjectIdentity, WorkflowRunID, ExecutionID, CurrentAttemptID, Generation, ConfigurationDigest, Protocol, SessionID, SessionGeneration string
	Target                                                                                                                                 executionbackend.ExecutionTarget
	Process                                                                                                                                *executionbackend.ProcessIdentity
	ImplementationData                                                                                                                     []executionbackend.ImplementationDataEnvelope
}

func setBinding(t *testing.T, e *bindingRecord, endpoint, generation, session string) {
	t.Helper()
	if e.ProjectIdentity == "" {
		e.ProjectIdentity = "project"
	}
	if e.WorkflowRunID == "" {
		e.WorkflowRunID = newID()
	}
	if e.ExecutionID == "" {
		e.ExecutionID = newID()
	}
	if e.CurrentAttemptID == "" {
		e.CurrentAttemptID = newID()
	}
	if e.Generation == "" {
		e.Generation = "resource-generation"
	}
	if e.ConfigurationDigest == "" {
		e.ConfigurationDigest = "fixed-config"
	}
	env, err := codexworker.NewAppServerBindingEnvelope(codexworker.AppServerBindingIdentity{ProjectID: e.ProjectIdentity, ExecutionID: e.ExecutionID, AttemptID: e.CurrentAttemptID, Generation: e.Generation, ConfigDigest: e.ConfigurationDigest}, codexworker.AppServerBinding{Endpoint: endpoint, Generation: generation, SessionID: session})
	require(t, err)
	e.ImplementationData = []executionbackend.ImplementationDataEnvelope{env}
	e.SessionID, e.SessionGeneration = session, generation
}

func removeEnvelope(e *bindingRecord, key string) {
	var values []executionbackend.ImplementationDataEnvelope
	for _, v := range e.ImplementationData {
		if v.ImplementationKey != key {
			values = append(values, v.Clone())
		}
	}
	e.ImplementationData = values
}

func cloneData(values []executionbackend.ImplementationDataEnvelope) []executionbackend.ImplementationDataEnvelope {
	result := make([]executionbackend.ImplementationDataEnvelope, len(values))
	for i, e := range values {
		result[i] = e.Clone()
	}
	return result
}

func operationDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

func AtomicWrite(path string, data []byte) error { return os.WriteFile(path, data, 0600) }

func require(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

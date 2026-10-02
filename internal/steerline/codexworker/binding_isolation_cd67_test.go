package codexworker_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
	"os"
	"os/exec"
	"testing"
)

func TestCD63BindingCodecIsolation(t *testing.T) {
	record := bindingRecord{Protocol: string(executionapi.ProtocolLocalCodexAppServer)}
	setBinding(t, &record, "ws://127.0.0.1:12345", "native-generation", "00000000-0000-0000-0000-000000000008")
	snapshotFor := func(value *bindingRecord) executionapi.LifecycleSnapshot {
		return executionapi.LifecycleSnapshot{
			Session:             executionapi.SessionContext{ProjectID: value.ProjectIdentity, WorkflowRunID: value.WorkflowRunID, ExecutionID: value.ExecutionID, AttemptID: value.CurrentAttemptID, SessionID: value.SessionID, Generation: value.SessionGeneration},
			ConfigurationDigest: value.ConfigurationDigest, ResourceGeneration: value.Generation,
			Implementation: cloneData(value.ImplementationData),
			PendingTurn:    &executionapi.TurnInput{CommandID: newID(), Prompt: "fixed instruction"},
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
		foreign, err := orcaworker.NewExecutionStateEnvelope(stateIdentity(&copy), orcaworker.NativeExecutionState{RepositoryID: "repo"})
		require(t, err)
		copy.ImplementationData = []executionapi.ImplementationDataEnvelope{foreign}
		if _, err := (codexworker.ExecutionLifecycle{}).Next(context.Background(), snapshotFor(&copy)); err == nil {
			t.Fatal("Codex implementation accepted an Orca binding")
		}
	})
	t.Run("seed cannot stand in for a bound process", func(t *testing.T) {
		copy := record
		removeEnvelope(&copy, codexworker.ExecutionBindingCodecKey)
		setBinding(t, &copy, "", "", "00000000-0000-0000-0000-000000000008")
		copy.Process = &executionapi.ProcessIdentity{Kind: "os", PID: 1, Created: "process-generation"}
		workspace, serverDir := t.TempDir(), t.TempDir()
		ref := executionapi.ExecutionRef{Version: executionapi.ExecutionRecordVersion, Backend: executionapi.BackendLocal, Protocol: executionapi.ProtocolLocalCodexAppServer, Target: copy.Target, Agent: executionapi.AgentSpec{Name: "codex"}, RunID: copy.WorkflowRunID, AttemptID: copy.CurrentAttemptID, WorkspacePath: workspace, External: *copy.Process, Generation: copy.SessionGeneration}
		session := executionapi.SessionContext{ProjectID: copy.ProjectIdentity, TaskID: "task", WorkflowRunID: copy.WorkflowRunID, ExecutionID: copy.ExecutionID, AttemptID: copy.CurrentAttemptID, Agent: executionapi.AgentSpec{Name: "codex"}, Target: copy.Target, SessionID: copy.SessionID}
		snapshot := executionapi.LifecycleSnapshot{Ref: ref, Session: session, ConfigurationDigest: copy.ConfigurationDigest, ResourceGeneration: copy.Generation, Implementation: cloneData(copy.ImplementationData), Workspace: executionapi.WorkspaceIntent{Path: workspace}}
		input := executionapi.TurnInput{CommandID: newID(), Prompt: "original saved input"}
		operation := executionapi.Operation{Version: 1, ID: newID(), RunID: copy.WorkflowRunID, AttemptID: copy.CurrentAttemptID, Backend: ref.Backend, Protocol: ref.Protocol, Target: ref.Target, Action: string(executionapi.LifecyclePrepareSession), PayloadDigest: operationDigest(mustJSON(input)), Status: executionapi.OperationInFlight, Ref: &ref}
		operation.LifecycleIdentity = &executionapi.LifecycleOperationIdentity{ProjectID: snapshot.Session.ProjectID, ExecutionID: snapshot.Session.ExecutionID, ConfigurationDigest: snapshot.ConfigurationDigest, ResourceGeneration: snapshot.ResourceGeneration, InputBindingDigest: executionapi.LifecycleBindingDigest(snapshot), Ref: snapshot.Ref}
		startCalled := false
		lifecycle := codexworker.ExecutionLifecycle{Backend: codexworker.Backend{Config: codexworker.Config{Policy: workspacePolicyFixture(), Executable: os.Args[0], Workspace: workspace, RunDir: t.TempDir(), RunID: copy.WorkflowRunID, ServerDir: serverDir, Sandbox: "workspace-write", WindowsSandbox: "elevated"}, Process: codexworker.ProcessPort{Start: func(*exec.Cmd) (executionapi.StopHandle, error) {
			startCalled = true
			return nil, errors.New("unexpected process start")
		}, Inspect: func(*executionapi.ProcessIdentity) string { return "matching process still exists" }, NewGeneration: newID, WriteAtomic: AtomicWrite}}}
		receipt, _, err := lifecycle.Execute(context.Background(), executionapi.LifecycleRequest{Snapshot: snapshot, Operation: operation, Input: executionapi.LifecycleInput{Turn: &input}})
		if err == nil || startCalled || receipt.Disposition != executionapi.OperationUnknown {
			t.Fatalf("recovery seed authorized replacing its missing original process receipt: receipt=%+v err=%v start_called=%v", receipt, err, startCalled)
		}
	})
	t.Run("native coordinator must match the target", func(t *testing.T) {
		copy := record
		copy.Protocol = string(executionapi.ProtocolOrcaTerminalV2)
		copy.Target.CoordinatorTerminal = "original-coordinator"
		foreign, err := orcaworker.NewExecutionStateEnvelope(stateIdentity(&copy), orcaworker.NativeExecutionState{Coordinator: executionapi.ProcessIdentity{Terminal: "replacement-coordinator", RuntimeID: "runtime"}})
		require(t, err)
		copy.ImplementationData = []executionapi.ImplementationDataEnvelope{foreign}
		if _, err := orcaworker.DecodeExecutionStateEnvelope(foreign, stateIdentity(&copy)); err == nil {
			t.Fatal("Orca implementation accepted a different native coordinator target")
		}
	})
}

type bindingRecord struct {
	ProjectIdentity, WorkflowRunID, ExecutionID, CurrentAttemptID, Generation, ConfigurationDigest, Protocol, SessionID, SessionGeneration string
	Target                                                                                                                                 executionapi.ExecutionTarget
	Process                                                                                                                                *executionapi.ProcessIdentity
	ImplementationData                                                                                                                     []executionapi.ImplementationDataEnvelope
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
	e.ImplementationData = []executionapi.ImplementationDataEnvelope{env}
	e.SessionID, e.SessionGeneration = session, generation
}
func stateIdentity(e *bindingRecord) orcaworker.ExecutionStateIdentity {
	process := executionapi.ProcessIdentity{}
	if e.Process != nil {
		process = *e.Process
	}
	return orcaworker.ExecutionStateIdentity{ProjectID: e.ProjectIdentity, ExecutionID: e.ExecutionID, AttemptID: e.CurrentAttemptID, Generation: e.Generation, ConfigDigest: e.ConfigurationDigest, Protocol: executionapi.ExecutionProtocol(e.Protocol), Process: process, CoordinatorTerminal: e.Target.CoordinatorTerminal}
}
func removeEnvelope(e *bindingRecord, key string) {
	var values []executionapi.ImplementationDataEnvelope
	for _, v := range e.ImplementationData {
		if v.ImplementationKey != key {
			values = append(values, v.Clone())
		}
	}
	e.ImplementationData = values
}
func cloneData(values []executionapi.ImplementationDataEnvelope) []executionapi.ImplementationDataEnvelope {
	result := make([]executionapi.ImplementationDataEnvelope, len(values))
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
func workspacePolicyFixture() executionapi.ExecutionPolicy {
	return executionapi.ExecutionPolicy{Filesystem: executionapi.FilesystemPolicy{ReadScope: "host", WriteScope: "workspace", GitMetadataWrite: true}, Network: "allow", Escalation: "deny"}
}
func require(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

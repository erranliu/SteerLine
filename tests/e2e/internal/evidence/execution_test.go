package evidence

import (
	"encoding/json"
	"testing"

	codexworker "github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	orcaworker "github.com/erranliu/SteerLine/internal/execution/adapters/orca"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/reconstruction"
)

const thread = "00000000-0000-0000-0000-000000000008"

func must[T any](t *testing.T, value T, err error) T {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func fixture(t *testing.T) reconstruction.Snapshot {
	t.Helper()
	e := executionmodel.ExecutionRecord{ProjectIdentity: "project", ExecutionID: "execution", CurrentAttemptID: "attempt", Generation: "generation", ConfigurationDigest: "config", Protocol: string(executionbackend.ProtocolLocalCodexAppServer)}
	profile := &executionmodel.FixedExecutionProfile{ID: "profile", AcceptedConfigVersion: "accepted", Host: "local", Agent: "codex"}
	payload, err := json.Marshal(codexworker.ProfileConfig{Executable: "codex", Model: "gpt-6-luna", ReasoningEffort: "medium"})
	if err != nil {
		t.Fatal(err)
	}
	data, err := executionbackend.NewImplementationDataEnvelope(executionbackend.ImplementationDataEnvelope{CodecVersion: 2, Purpose: executionbackend.ImplementationConfigPurpose, ImplementationKey: codexworker.ProfileCodecKey, ProjectID: e.ProjectIdentity, ProfileID: profile.ID, AcceptedConfigVersion: profile.AcceptedConfigVersion}, payload)
	profile.ImplementationData = []executionbackend.ImplementationDataEnvelope{must(t, data, err)}
	permissions, err := codexworker.NativePermissions("windows", "local", "codex")
	if err != nil {
		t.Fatal(err)
	}
	resource, err := codexworker.NewPolicyResourceEnvelope(codexworker.PolicyResourceIdentity{ProjectID: e.ProjectIdentity, ExecutionID: e.ExecutionID, AttemptID: e.CurrentAttemptID, Generation: e.Generation, ConfigDigest: e.ConfigurationDigest}, codexworker.PolicyResourcePayload{NativeVersion: codexworker.PolicyCodexVersion, Contract: codexworker.PolicyContract, Settings: executionbackend.LaunchCommand{Executable: "codex", Args: permissions.Args()}})
	e.PolicyResources = &executionmodel.PolicyResources{WriteRoots: []string{"workspace"}, ImplementationData: must(t, resource, err)}
	binding, err := codexworker.NewAppServerBindingEnvelope(codexworker.AppServerBindingIdentity{ProjectID: e.ProjectIdentity, ExecutionID: e.ExecutionID, AttemptID: e.CurrentAttemptID, Generation: e.Generation, ConfigDigest: e.ConfigurationDigest}, codexworker.AppServerBinding{Endpoint: "ws://127.0.0.1:12345", SessionID: thread, Generation: "native-generation"})
	e.ImplementationData = []executionbackend.ImplementationDataEnvelope{must(t, binding, err)}
	return reconstruction.Snapshot{Executions: []executionmodel.ExecutionRecord{e}, WorkflowRuns: []executionmodel.WorkflowRunRecord{{ExecutionProfile: profile}}}
}

func TestExecutionEvidencePreservesBoundIdentityAndAuthority(t *testing.T) {
	domain := fixture(t)
	if err := FixedNativeLaunch(domain); err != nil {
		t.Fatal(err)
	}
	model, err := ModelSelection(domain)
	if err != nil || model != (ModelSettings{Model: "gpt-6-luna", Effort: "medium"}) {
		t.Fatalf("model evidence: %+v / %v", model, err)
	}
	if session, err := Session(domain.Executions[0]); err != nil || session != thread {
		t.Fatalf("session evidence: %s / %v", session, err)
	}
	for _, field := range []string{"project", "execution", "attempt", "generation", "config"} {
		t.Run(field, func(t *testing.T) {
			d := fixture(t)
			e := &d.Executions[0]
			switch field {
			case "project":
				e.ProjectIdentity = "other"
			case "execution":
				e.ExecutionID = "other"
			case "attempt":
				e.CurrentAttemptID = "other"
			case "generation":
				e.Generation = "other"
			case "config":
				e.ConfigurationDigest = "other"
			}
			if err := FixedNativeLaunch(d); err == nil {
				t.Fatal("foreign permission receipt accepted")
			}
			if _, err := Session(*e); err == nil {
				t.Fatal("foreign session receipt accepted")
			}
		})
	}
	domain.WorkflowRuns[0].ExecutionProfile.AcceptedConfigVersion = "other"
	if _, err := ModelSelection(domain); err == nil {
		t.Fatal("foreign profile accepted")
	}
	domain = fixture(t)
	domain.Executions[0].PolicyResources = nil
	if err := FixedNativeLaunch(domain); err == nil {
		t.Fatal("configured authority accepted without launch evidence")
	}
}

func TestExplicitOrcaArgsEvidenceRejectsUncoveredVariants(t *testing.T) {
	for _, args := range []string{"null", "[]", `["--native"]`} {
		d := fixture(t)
		profile := d.WorkflowRuns[0].ExecutionProfile
		payload := []byte(`{"executable":"orca","args":` + args + `,"shell":"powershell.exe","wait_timeout":"4m"}`)
		data, err := executionbackend.NewImplementationDataEnvelope(executionbackend.ImplementationDataEnvelope{CodecVersion: 1, Purpose: executionbackend.ImplementationConfigPurpose, ImplementationKey: orcaworker.ProfileCodecKey, ProjectID: d.Executions[0].ProjectIdentity, ProfileID: profile.ID, AcceptedConfigVersion: profile.AcceptedConfigVersion}, payload)
		profile.ImplementationData = append(profile.ImplementationData, must(t, data, err))
		if err := OrcaExplicitEmptyArgs(d); (err == nil) != (args == "[]") {
			t.Fatalf("args=%s evidence result=%v", args, err)
		}
	}
}

func orcaFixture(t *testing.T, succeeded bool) executionmodel.ExecutionRecord {
	e := fixture(t).Executions[0]
	e.Protocol = string(executionbackend.ProtocolOrcaTerminalV2)
	e.Process = &executionbackend.ProcessIdentity{Kind: "orca", RuntimeID: "runtime", Terminal: "agent", Incarnation: "incarnation", WorktreeID: "worktree", Task: "task", Dispatch: "dispatch"}
	e.CurrentTurnID, e.LastStoppedTurnID, e.TurnNumber = "business", "business", 1
	state := orcaworker.NativeExecutionState{Coordinator: executionbackend.ProcessIdentity{RuntimeID: "runtime", Terminal: "coordinator"}, Turns: []orcaworker.NativeTurnState{{TurnID: "business", TaskID: "task", DispatchID: "dispatch", Purpose: "business", Settled: true, Outcome: "succeeded"}}}
	if !succeeded {
		state.Turns[0].Outcome = "failed"
	}
	data, err := orcaworker.NewExecutionStateEnvelope(orcaworker.ExecutionStateIdentity{ProjectID: e.ProjectIdentity, ExecutionID: e.ExecutionID, AttemptID: e.CurrentAttemptID, Generation: e.Generation, ConfigDigest: e.ConfigurationDigest}, state)
	e.ImplementationData = []executionbackend.ImplementationDataEnvelope{must(t, data, err)}
	return e
}

func TestOrcaEvidenceRequiresOriginalNativeBusinessOutcome(t *testing.T) {
	e := orcaFixture(t, true)
	if err := OrcaNativeBusiness(e); err != nil {
		t.Fatal(err)
	}
	for _, reason := range []string{"runtime", "generation", "dispatch", "unsettled", "failed-turn"} {
		t.Run(reason, func(t *testing.T) {
			e := orcaFixture(t, reason != "failed-turn")
			switch reason {
			case "runtime":
				e.Process.RuntimeID = "other"
			case "generation":
				e.Generation = "other"
			case "dispatch":
				e.Process.Dispatch = "other"
			case "unsettled":
				e.LastStoppedTurnID = ""
			}
			if err := OrcaNativeBusiness(e); err == nil {
				t.Fatalf("invalid native business outcome accepted: %s", reason)
			}
		})
	}
}

func TestNativeOrcaLaunchRequiresOriginalAcceptedReceipt(t *testing.T) {
	d := fixture(t)
	e := &d.Executions[0]
	e.Protocol, e.Agent = string(executionbackend.ProtocolOrcaTerminalV2), "codex"
	e.PolicyResources = nil
	e.Process = &executionbackend.ProcessIdentity{Kind: "orca", RuntimeID: "runtime", Terminal: "terminal", Incarnation: "inc", WorktreeID: "worktree", OrchestrationRun: "run", Task: "task", Dispatch: "dispatch"}
	e.CurrentTurnID = "original-command"
	profile := d.WorkflowRuns[0].ExecutionProfile
	profile.Host = "orca"
	payload, _ := json.Marshal(orcaworker.ProfileConfig{Executable: "orca", Model: "gpt-6-luna", ReasoningEffort: "medium"})
	envelope, err := executionbackend.NewImplementationDataEnvelope(executionbackend.ImplementationDataEnvelope{CodecVersion: 1, Purpose: executionbackend.ImplementationConfigPurpose, ImplementationKey: orcaworker.ProfileCodecKey, ProjectID: e.ProjectIdentity, ProfileID: profile.ID, AcceptedConfigVersion: profile.AcceptedConfigVersion}, payload)
	profile.ImplementationData = []executionbackend.ImplementationDataEnvelope{must(t, envelope, err)}
	raw := []byte(`{"runId":"run","taskId":"task","dispatchId":"dispatch","mode":{"mode":"terminal"},"launch":{"effective":{"agent":"codex","model":"gpt-6-luna","effort":"medium"}}}`)
	receipt := executionbackend.LifecycleReceipt{Process: e.Process, State: executionbackend.LifecycleStateFacts{InputAccepted: true, TurnID: e.CurrentTurnID}, Operation: executionbackend.Operation{Receipt: raw}}
	encoded, _ := json.Marshal(receipt)
	d.Operations = []executionmodel.OperationRecord{{Kind: "start_worker_turn", Status: "confirmed", LifecycleIdentity: &executionbackend.LifecycleOperationIdentity{ProjectID: e.ProjectIdentity, ExecutionID: e.ExecutionID, ConfigurationDigest: e.ConfigurationDigest, ResourceGeneration: e.Generation, Ref: executionbackend.ExecutionRef{AttemptID: e.CurrentAttemptID}}, Receipt: encoded}}
	if err := FixedNativeLaunch(d); err != nil {
		t.Fatal(err)
	}
	settings, err := ModelSelection(d)
	if err != nil || settings.Model != "gpt-6-luna" || settings.Effort != "medium" {
		t.Fatalf("native model receipt: %+v %v", settings, err)
	}
	d.Operations[0].LifecycleIdentity.ExecutionID = "another-execution"
	if err := FixedNativeLaunch(d); err == nil {
		t.Fatal("another execution's receipt became native launch proof")
	}
	d.Operations[0].LifecycleIdentity.ExecutionID = e.ExecutionID
	e.Process.Incarnation = "replacement"
	if err := FixedNativeLaunch(d); err == nil {
		t.Fatal("replaced terminal became native launch proof")
	}
}

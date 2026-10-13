package evidence

import (
	"encoding/json"
	"testing"

	codexworker "github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	transport "github.com/erranliu/SteerLine/internal/execution/adapters/orcatransport"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/reconstruction"
)

const thread = "00000000-0000-0000-0000-000000000008"

func TestWorkingEvidenceRejectsOtherCommandOrResource(t *testing.T) {
	for _, fault := range []string{"none", "command", "process", "generation", "working", "acceptance"} {
		t.Run(fault, func(t *testing.T) {
			e := fixture(t).Executions[0]
			e.Protocol, e.CurrentCommandID = string(executionbackend.ProtocolOrcaTerminalV3), "original-command"
			e.Process = &executionbackend.ProcessIdentity{Kind: "terminal", Terminal: "original"}
			state := transport.State{Process: *e.Process, Turns: []transport.Turn{{Command: e.CurrentCommandID, Accepted: true, Started: true, Native: transport.NativeProof{WorkingAt: 1, RootWorkingAt: 1, Provider: "original-provider", Authority: "original-authority"}}}}
			switch fault {
			case "command":
				state.Turns[0].Command = "other"
			case "process":
				state.Process.Terminal = "other"
			case "working":
				state.Turns[0].Native.WorkingAt = 0
			case "acceptance":
				state.Turns[0].Accepted = false
			}
			raw, err := json.Marshal(state)
			if err != nil {
				t.Fatal(err)
			}
			data, err := executionbackend.NewImplementationDataEnvelope(executionbackend.ImplementationDataEnvelope{CodecVersion: 3, ImplementationKey: transport.StateKey, Purpose: executionbackend.ImplementationResourcePurpose, ProjectID: e.ProjectIdentity, ExecutionID: e.ExecutionID, AttemptID: e.CurrentAttemptID, Generation: e.Generation, ConfigDigest: e.ConfigurationDigest}, raw)
			if err != nil {
				t.Fatal(err)
			}
			e.ImplementationData = []executionbackend.ImplementationDataEnvelope{data}
			if fault == "generation" {
				e.Generation = "other"
			}
			if OrcaOriginalWorkingObserved(e) != (fault == "none") {
				t.Fatal("working evidence crossed original binding")
			}
		})
	}
}

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

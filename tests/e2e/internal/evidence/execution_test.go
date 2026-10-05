package evidence

import (
	"encoding/json"
	"testing"

	"github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	"github.com/erranliu/SteerLine/internal/execution/adapters/orca"
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
	policy := executionbackend.ExecutionPolicy{Filesystem: executionbackend.FilesystemPolicy{ReadScope: "host", WriteScope: "host"}, Network: "allow", Escalation: "deny"}
	profile := &executionmodel.FixedExecutionProfile{ID: "profile", AcceptedConfigVersion: "accepted", Host: "local", Agent: "codex", ExecutionPolicy: policy}
	payload, err := json.Marshal(codexworker.ProfileConfig{Executable: "codex", Model: "gpt-6-luna", ReasoningEffort: "medium"})
	if err != nil {
		t.Fatal(err)
	}
	data, err := executionbackend.NewImplementationDataEnvelope(executionbackend.ImplementationDataEnvelope{CodecVersion: 1, Purpose: executionbackend.ImplementationConfigPurpose, ImplementationKey: codexworker.ProfileCodecKey, ProjectID: e.ProjectIdentity, ProfileID: profile.ID, AcceptedConfigVersion: profile.AcceptedConfigVersion}, payload)
	profile.ImplementationData = []executionbackend.ImplementationDataEnvelope{must(t, data, err)}
	resource, err := codexworker.NewPolicyResourceEnvelope(codexworker.PolicyResourceIdentity{ProjectID: e.ProjectIdentity, ExecutionID: e.ExecutionID, AttemptID: e.CurrentAttemptID, Generation: e.Generation, ConfigDigest: e.ConfigurationDigest}, codexworker.PolicyResourcePayload{NativeVersion: codexworker.PolicyCodexVersion, Contract: codexworker.FullAccessPolicyContract, Settings: executionbackend.LaunchCommand{Executable: "codex"}})
	e.PolicyResources = &executionmodel.PolicyResources{ImplementationData: must(t, resource, err)}
	binding, err := codexworker.NewAppServerBindingEnvelope(codexworker.AppServerBindingIdentity{ProjectID: e.ProjectIdentity, ExecutionID: e.ExecutionID, AttemptID: e.CurrentAttemptID, Generation: e.Generation, ConfigDigest: e.ConfigurationDigest}, codexworker.AppServerBinding{Endpoint: "ws://127.0.0.1:12345", SessionID: thread, Generation: "native-generation"})
	e.ImplementationData = []executionbackend.ImplementationDataEnvelope{must(t, binding, err)}
	return reconstruction.Snapshot{Executions: []executionmodel.ExecutionRecord{e}, WorkflowRuns: []executionmodel.WorkflowRunRecord{{ExecutionProfile: profile}}}
}

func TestExecutionEvidencePreservesBoundIdentityAndAuthority(t *testing.T) {
	domain := fixture(t)
	if err := FullAccess(domain); err != nil {
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
			if err := FullAccess(d); err == nil {
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
	if err := FullAccess(domain); err == nil {
		t.Fatal("configured authority accepted without launch evidence")
	}
}

func orcaFixture(t *testing.T, succeeded bool) executionmodel.ExecutionRecord {
	e := fixture(t).Executions[0]
	e.Protocol = string(executionbackend.ProtocolOrcaTerminalV2)
	e.Process = &executionbackend.ProcessIdentity{Kind: "orca", RuntimeID: "runtime", Terminal: "agent", Incarnation: "incarnation", WorktreeID: "worktree"}
	state := orcaworker.NativeExecutionState{Coordinator: executionbackend.ProcessIdentity{RuntimeID: "runtime", Terminal: "coordinator"}, Turns: []orcaworker.NativeTurnState{{TurnID: "readiness", Purpose: "readiness", Settled: true, Outcome: "succeeded"}, {TurnID: "business", Purpose: "business", Settled: true, Outcome: "succeeded"}}}
	if !succeeded {
		state.Turns[1].Outcome = "failed"
	}
	data, err := orcaworker.NewExecutionStateEnvelope(orcaworker.ExecutionStateIdentity{ProjectID: e.ProjectIdentity, ExecutionID: e.ExecutionID, AttemptID: e.CurrentAttemptID, Generation: e.Generation, ConfigDigest: e.ConfigurationDigest}, state)
	e.ImplementationData = []executionbackend.ImplementationDataEnvelope{must(t, data, err)}
	e.Environment = &executionmodel.WorkerEnvironment{Ready: true, Preparation: &executionmodel.WorkerPreparation{Verified: true}}
	return e
}

func TestOrcaEvidenceRequiresOriginalRuntimeAndSucceededPreparation(t *testing.T) {
	e := orcaFixture(t, true)
	if err := OrcaReadiness(e); err != nil {
		t.Fatal(err)
	}
	if session, err := Session(e); err != nil || session != "orca-terminal:runtime:agent:incarnation" {
		t.Fatalf("original Orca session: %s / %v", session, err)
	}
	for _, reason := range []string{"runtime", "generation", "missing-preparation", "unverified", "failed-turn"} {
		t.Run(reason, func(t *testing.T) {
			e := orcaFixture(t, reason != "failed-turn")
			switch reason {
			case "runtime":
				e.Process.RuntimeID = "other"
			case "generation":
				e.Generation = "other"
			case "missing-preparation":
				e.Environment.Preparation = nil
			case "unverified":
				e.Environment.Preparation.Verified = false
			}
			if err := OrcaReadiness(e); err == nil {
				t.Fatalf("invalid Orca continuity accepted: %s", reason)
			}
		})
	}
}

package app

import (
	"os"
	"testing"

	codexworker "github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	b "github.com/erranliu/SteerLine/internal/execution/backend"
	m "github.com/erranliu/SteerLine/internal/execution/model"
	api "github.com/erranliu/SteerLine/internal/product/api"
	p "github.com/erranliu/SteerLine/internal/storage/ports"
)

func TestNativeSessionUsesFrozenProfileAndExactCandidate(t *testing.T) {
	target := api.NativeSessionTarget{RunID: "run", ExecutionID: "execution", AttemptID: "attempt", SessionID: "00000000-0000-0000-0000-000000000001", SessionGeneration: "native"}
	profileData, err := codexworker.EncodeProfileConfig(codexworker.ProfileConfig{Executable: os.Args[0]})
	if err != nil {
		t.Fatal(err)
	}
	profile, err := b.NewImplementationDataEnvelope(b.ImplementationDataEnvelope{CodecVersion: codexworker.ProfileCodecVersion, Purpose: b.ImplementationConfigPurpose, ImplementationKey: codexworker.ProfileCodecKey, ProjectID: "project", ProfileID: "original-profile", AcceptedConfigVersion: "config-version"}, profileData)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := codexworker.NewAppServerBindingEnvelope(codexworker.AppServerBindingIdentity{ProjectID: "project", ExecutionID: target.ExecutionID, AttemptID: target.AttemptID, Generation: "resources", ConfigDigest: "config"}, codexworker.AppServerBinding{Endpoint: "ws://127.0.0.1:12345", SessionID: target.SessionID, Generation: target.SessionGeneration})
	if err != nil {
		t.Fatal(err)
	}
	records := p.ExecutionSnapshot{ProjectIdentity: "project", WorkflowRuns: []m.WorkflowRunRecord{{ProjectIdentity: "project", WorkflowRunID: target.RunID, ExecutionProfile: &m.FixedExecutionProfile{ID: "original-profile", Host: "local", Agent: "codex", AcceptedConfigVersion: "config-version", ImplementationData: []b.ImplementationDataEnvelope{profile}}}}, Executions: []m.ExecutionRecord{{ProjectIdentity: "project", WorkflowRunID: target.RunID, ExecutionID: target.ExecutionID, CurrentAttemptID: target.AttemptID, SessionID: target.SessionID, SessionGeneration: target.SessionGeneration, CurrentTurnID: "original-turn", Generation: "resources", ConfigurationDigest: "config", Backend: string(b.BackendLocal), Protocol: string(b.ProtocolLocalCodexAppServer), Agent: "codex", Process: &b.ProcessIdentity{Kind: "os", PID: 123, Created: "original"}, Workspace: m.WorkspaceEvidence{Worktree: t.TempDir()}, ImplementationData: []b.ImplementationDataEnvelope{envelope}}}}
	command, binding, process, err := nativeSessionCommand(records, "project", target)
	if err != nil {
		t.Fatal(err)
	}
	if command.Executable != os.Args[0] || binding.Endpoint != "ws://127.0.0.1:12345" || process.PID != 123 || len(command.Args) != 6 || command.Args[0] != "resume" || command.Args[5] != target.SessionID {
		t.Fatalf("viewer lost original native identity: %+v / %+v / %+v", command, binding, process)
	}
	for _, field := range []string{"project", "run", "execution", "attempt", "session", "generation", "envelope", "backend", "empty history"} {
		t.Run(field, func(t *testing.T) {
			copy := records
			copy.Executions = append([]m.ExecutionRecord(nil), records.Executions...)
			stale := target
			switch field {
			case "project":
				copy.ProjectIdentity = "other"
			case "run":
				stale.RunID = "other"
			case "execution":
				stale.ExecutionID = "other"
			case "attempt":
				stale.AttemptID = "other"
			case "session":
				stale.SessionID = "00000000-0000-0000-0000-000000000002"
			case "generation":
				stale.SessionGeneration = "other"
			case "envelope":
				copy.Executions[0].ImplementationData = append(copy.Executions[0].ImplementationData[:0:0], envelope, envelope)
			case "empty history":
				copy.Executions[0].CurrentTurnID = ""
			case "backend":
				copy.Executions[0].Backend = "orca"
			}
			if _, _, _, err := nativeSessionCommand(copy, "project", stale); err == nil {
				t.Fatal("stale/foreign target opened")
			}
		})
	}
}

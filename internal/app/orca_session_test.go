package app

import (
	"encoding/json"

	"testing"

	remote "github.com/erranliu/SteerLine/internal/execution/adapters/orcatransport"
	b "github.com/erranliu/SteerLine/internal/execution/backend"
	m "github.com/erranliu/SteerLine/internal/execution/model"
	api "github.com/erranliu/SteerLine/internal/product/api"
	p "github.com/erranliu/SteerLine/internal/storage/ports"
)

func orcaSessionRecords(t *testing.T) (p.ExecutionSnapshot, api.NativeSessionTarget) {
	t.Helper()
	i := b.ProcessIdentity{Kind: "orca_terminal", RuntimeID: "peer", Terminal: "original", Incarnation: "inc", ExecutionHost: "local", WorktreeID: "repo::/worker"}
	target := api.NativeSessionTarget{RunID: "run", ExecutionID: "execution", AttemptID: "attempt", SessionID: remote.SessionID(i), SessionGeneration: i.Incarnation}
	raw, err := json.Marshal(remote.Profile{Executable: "original-orca", WaitTimeout: "30s"})
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := b.NewImplementationDataEnvelope(b.ImplementationDataEnvelope{CodecVersion: remote.ProfileVersion, Purpose: b.ImplementationConfigPurpose, ImplementationKey: remote.ProfileKey, ProjectID: "project", ProfileID: "frozen", AcceptedConfigVersion: "version"}, raw)
	if err != nil {
		t.Fatal(err)
	}
	records := p.ExecutionSnapshot{ProjectIdentity: "project", WorkflowRuns: []m.WorkflowRunRecord{{ProjectIdentity: "project", WorkflowRunID: "run", ExecutionProfile: &m.FixedExecutionProfile{ID: "frozen", AcceptedConfigVersion: "version", Host: "orca", Agent: "claude", ImplementationData: []b.ImplementationDataEnvelope{envelope}}}}, Executions: []m.ExecutionRecord{{ProjectIdentity: "project", WorkflowRunID: "run", ExecutionID: "execution", CurrentAttemptID: "attempt", SessionID: target.SessionID, SessionGeneration: target.SessionGeneration, CurrentTurnID: "turn", Backend: string(b.BackendOrca), Protocol: string(b.ProtocolOrcaTerminalV3), Agent: "claude", Process: &i, Workspace: m.WorkspaceEvidence{Worktree: "/worker", Branch: "branch"}}}}
	return records, target
}

func TestOrcaSessionResolvesFrozenProfileAndRejectsStaleCandidate(t *testing.T) {
	records, target := orcaSessionRecords(t)
	spec, err := resolveOrcaSession(records, "project", target)
	if err != nil || spec.profile.Executable != "original-orca" || spec.ref.External.Terminal != "original" || spec.ref.Agent.Name != "claude" {
		t.Fatalf("lost frozen candidate: %+v / %v", spec, err)
	}
	for _, scenario := range []string{"project", "attempt", "session", "generation", "process", "profile"} {
		t.Run(scenario, func(t *testing.T) {
			copy, stale := records, target
			copy.Executions = append([]m.ExecutionRecord(nil), records.Executions...)
			project := "project"
			switch scenario {
			case "project":
				project = "other"
			case "attempt":
				stale.AttemptID = "other"
			case "session":
				stale.SessionID = "other"
			case "generation":
				stale.SessionGeneration = "other"
			case "process":
				i := *copy.Executions[0].Process
				i.Incarnation = "new"
				copy.Executions[0].Process = &i
			case "profile":
				copy.WorkflowRuns = nil
			case "empty-turn":
				copy.Executions[0].CurrentTurnID = ""
			}
			if _, err := resolveOrcaSession(copy, project, stale); err == nil {
				t.Fatal("stale candidate opened")
			}
		})
	}
}

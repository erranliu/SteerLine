package orcaworker

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	resourceprocess "github.com/erranliu/SteerLine/internal/execution/process"
)

// This opt-in fixture exercises real native launch/continuation and the same
// production material/settlement boundary, without a GitHub publication.
func TestOrcaNativeWorkerSmoke(t *testing.T) {
	repository := os.Getenv("STEERLINE_ORCA_NATIVE_REPOSITORY")
	if repository == "" {
		t.Skip("isolated registered native Orca fixture is not selected")
	}
	executable := os.Getenv("STEERLINE_ORCA_NATIVE_EXECUTABLE")
	evidence := os.Getenv("STEERLINE_ORCA_NATIVE_EVIDENCE")
	if executable == "" || evidence == "" {
		t.Fatal("native fixture executable/evidence paths must be explicit")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Minute)
	defer cancel()
	head, err := exec.Command("git", "-C", repository, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	config := ProfileConfig{Executable: executable, Shell: "cmd.exe", WaitTimeout: "4m"}
	agent := os.Getenv("STEERLINE_ORCA_NATIVE_AGENT")
	if agent == "" {
		agent = "codex"
	}
	workflow, execution, attempt := liveIdentity(t), liveIdentity(t), liveIdentity(t)
	target := executionbackend.ExecutionTarget{ID: NativeTargetID(config)}
	snapshot := executionbackend.LifecycleSnapshot{Ref: executionbackend.ExecutionRef{Version: executionbackend.ExecutionRecordVersion, Backend: executionbackend.BackendOrca, Protocol: executionbackend.ProtocolOrcaTerminalV2, Target: target, Agent: executionbackend.AgentSpec{Name: agent}, RunID: workflow, AttemptID: attempt}, Session: executionbackend.SessionContext{ProjectID: filepath.Join(repository, ".git"), TaskID: "9001086", WorkflowRunID: workflow, ExecutionID: execution, AttemptID: attempt, Agent: executionbackend.AgentSpec{Name: agent}, Target: target}, TaskTitle: "isolated Orca native probe", ConfigurationDigest: "native-profile", ResourceGeneration: liveIdentity(t), WorkspacePlan: executionbackend.WorkspaceIntent{Path: filepath.Join(evidence, "worktree-intent"), Branch: "orca/native-probe", BaseCommit: strings.TrimSpace(string(head))}}
	raw, _ := EncodeProfileConfig(config)
	envelope, err := executionbackend.NewImplementationDataEnvelope(executionbackend.ImplementationDataEnvelope{CodecVersion: ProfileCodecVersion, Purpose: executionbackend.ImplementationConfigPurpose, ImplementationKey: ProfileCodecKey, ProjectID: snapshot.Session.ProjectID, ProfileID: "native-probe", AcceptedConfigVersion: "native-probe"}, raw)
	if err != nil {
		t.Fatal(err)
	}
	setup := executionbackend.LifecycleSetup{Snapshot: snapshot, RepositoryPath: repository, RunDirectory: evidence, ProfileID: "native-probe", AcceptedConfigVersion: "native-probe", ProfileData: []executionbackend.ImplementationDataEnvelope{envelope}, ProcessHost: resourceprocess.Host{}, CommandTimeout: 5 * time.Minute, GitExecutable: "git"}
	port, err := NewExecutionLifecycle(setup)
	if err != nil {
		t.Fatal(err)
	}
	native := port.(*ExecutionLifecycle)
	save := func(name string, value any) {
		t.Helper()
		data, _ := json.MarshalIndent(value, "", "  ")
		if err := os.MkdirAll(evidence, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(evidence, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	save("initial-snapshot.json", snapshot)
	apply := func(receipt executionbackend.LifecycleReceipt) {
		if receipt.Target != nil {
			snapshot.Ref.Target = *receipt.Target
			snapshot.Session.Target = *receipt.Target
		}
		if receipt.Workspace != nil {
			snapshot.Workspace = executionbackend.WorkspaceIntent{Path: receipt.Workspace.Worktree, Branch: receipt.Workspace.Branch, BaseCommit: receipt.Workspace.Base}
			snapshot.Ref.WorkspacePath, snapshot.Ref.WorkspaceBranch, snapshot.Ref.BaseCommit = receipt.Workspace.Worktree, receipt.Workspace.Branch, receipt.Workspace.Base
		}
		if receipt.Process != nil {
			snapshot.Ref.External = *receipt.Process
		}
		if len(receipt.Implementation) > 0 {
			snapshot.Implementation = receipt.Implementation
		}
	}
	// Cleanup is bounded to resources proven to belong to this probe.
	defer func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), time.Minute)
		defer stop()
		if snapshot.Ref.External.Dispatch != "" {
			result, proof, e := workerResourceAction(cleanupCtx, native.client.MetaCall, snapshot.Ref.External, "worker-stop", "probe-final-stop:"+execution)
			save("cleanup-stop.json", result)
			if e == nil && (proof.State == "stopped" || proof.AlreadySettled) {
				released, p, e := workerResourceAction(cleanupCtx, native.client.MetaCall, snapshot.Ref.External, "worker-release", "probe-final-release:"+execution)
				save("cleanup-release.json", released)
				if e != nil || !workerReleased(p) {
					t.Errorf("probe release unverified: %v %+v", e, p)
				}
			} else {
				t.Errorf("probe stop unverified: %v %+v", e, proof)
			}
		}
		state, _, e := native.readState(snapshot)
		if e == nil && state.Coordinator.Terminal != "" {
			_, e = CloseTerminal(cleanupCtx, native.client.MetaCall, state.Coordinator.Terminal, state.Coordinator.RuntimeID)
			if e != nil {
				t.Errorf("probe coordinator close: %v", e)
			}
		}
	}()
	for round := 1; round <= 2; round++ {
		turn := liveIdentity(t)
		snapshot.PendingTurn = &executionbackend.TurnInput{CommandID: turn, Purpose: "business", Prompt: "Target: only this isolated fixture worktree. Write native-probe-" + string(rune('0'+round)) + ".txt containing exactly round-" + string(rune('0'+round)) + ". Do not modify any other source file, commit, push, create a PR, or ask other agents. Your complete final response must be: round-" + string(rune('0'+round)) + " complete. Then comply with the generic output-material and native worker_done instructions."}
		snapshot.Session.PendingCommandID = turn
		for steps := 0; steps < 8; steps++ {
			candidate, e := port.Next(ctx, snapshot)
			if e != nil {
				t.Fatal(e)
			}
			if candidate.Done {
				break
			}
			ref := snapshot.Ref
			request := executionbackend.LifecycleRequest{Snapshot: snapshot, Input: candidate.Input, Operation: executionbackend.Operation{ID: liveIdentity(t), Action: string(candidate.Action), PayloadDigest: "fixture-input", Ref: &ref, LifecycleIdentity: &executionbackend.LifecycleOperationIdentity{ProjectID: snapshot.Session.ProjectID, ExecutionID: execution, Ref: ref, ConfigurationDigest: snapshot.ConfigurationDigest, ResourceGeneration: snapshot.ResourceGeneration, InputBindingDigest: "fixture-input"}}}
			receipt, observer, e := port.Execute(ctx, request)
			save("receipt-"+string(rune('0'+round))+"-"+string(rune('0'+steps))+".json", receipt)
			apply(receipt)
			if e != nil {
				t.Fatal(e)
			}
			if observer != nil {
				done := false
				for !done {
					select {
					case event, ok := <-observer.Events():
						if !ok {
							done = true
							break
						}
						if len(event.Implementation) > 0 {
							snapshot.Implementation = event.Implementation
						}
						event.Persisted <- nil
						event.Processed <- nil
						if event.DeliveryFinal && event.DeliveryID != "" {
							state, _, e := native.readState(snapshot)
							if e != nil {
								t.Fatal(e)
							}
							if _, e = AckInbox(ctx, native.client.MetaCall, state.Coordinator.Terminal, snapshot.Ref.Target.OrchestrationRun, event.DeliveryID, state.Coordinator.RuntimeID); e != nil {
								t.Fatal(e)
							}
						}
					case e := <-observer.Done():
						if e != nil {
							t.Fatal(e)
						}
						done = true
					case <-ctx.Done():
						t.Fatal(ctx.Err())
					}
				}
				if e = observer.Close(ctx); e != nil {
					t.Fatal(e)
				}
			}
		}
		expected := "round-" + string(rune('0'+round))
		data, e := os.ReadFile(filepath.Join(snapshot.Workspace.Path, "native-probe-"+string(rune('0'+round))+".txt"))
		if e != nil || strings.TrimSpace(string(data)) != expected {
			t.Fatalf("actual worker operation: %q %v", data, e)
		}
		save("round-"+string(rune('0'+round))+"-snapshot.json", snapshot)
		snapshot.PendingTurn = nil
		snapshot.Session.PendingCommandID = ""
	}
}

func liveIdentity(t *testing.T) string {
	t.Helper()
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(value[:])
}

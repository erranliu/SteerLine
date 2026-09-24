package steerline

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func auditOrcaRef(t *testing.T) (Config, ExecutionRef) {
	t.Helper()
	dir := t.TempDir()
	c := orcaTestConfig(t, dir, "")
	runID := newID()
	identity := ProcessIdentity{Kind: "orca", RuntimeID: "fake-runtime", Terminal: "audit-terminal", Incarnation: "audit-generation", Executable: c.Orca.Executable}
	require(t, writeFakeOrcaState(dir, fakeOrcaState{Handle: identity.Terminal, Incarnation: identity.Incarnation}))
	ref := ExecutionRef{Version: 1, Backend: BackendOrca, Protocol: ProtocolOrcaTerminalV2, Target: targetFor(c), Agent: AgentSpec{Name: "codex"}, RunID: runID, AttemptID: runID, WorkspacePath: dir, WorkspaceBranch: "audit/branch", BaseCommit: strings.Repeat("a", 40), External: identity}
	require(t, os.MkdirAll(filepath.Join(dir, "runs", runID), 0700))
	return c, ref
}

func TestAuditOrcaReplyRequiresLiveOriginalTerminal(t *testing.T) {
	for _, mode := range []string{"exited", "disconnected"} {
		t.Run(mode, func(t *testing.T) {
			c, ref := auditOrcaRef(t)
			if mode == "exited" {
				state, err := readFakeOrcaState(c.RunDir, ref.External.Terminal)
				require(t, err)
				state.Closed = true
				require(t, writeFakeOrcaState(c.RunDir, state))
			} else {
				t.Setenv("STEERLINE_FAKE_ORCA_DISCONNECTED", "1")
			}
			result := filepath.Join(ref.WorkspacePath, orcaResultFile)
			require(t, os.WriteFile(result, []byte("preserve original result"), 0600))
			_, err := (orcaTerminalBackend{config: c}).Reply(context.Background(), ref, "continue", Operation{}, filepath.Join(c.RunDir, "runs", ref.RunID, "result.json"))
			var typed *BackendError
			if !errors.As(err, &typed) || typed.Code != BackendIdentityMismatch {
				t.Fatalf("non-live continuation accepted: %v", err)
			}
			data, err := os.ReadFile(result)
			require(t, err)
			if string(data) != "preserve original result" {
				t.Fatal("rejected continuation removed prior output")
			}
			state, err := readFakeOrcaState(c.RunDir, ref.External.Terminal)
			require(t, err)
			if state.Sends != 0 {
				t.Fatal("input was sent to a non-live terminal")
			}
		})
	}
}

func TestAuditOrcaSendValidatesReceipt(t *testing.T) {
	for _, mode := range []string{"reject", "missing", "wrong_handle", "wrong_incarnation", "exit_error", "accepted"} {
		t.Run(mode, func(t *testing.T) {
			c, ref := auditOrcaRef(t)
			prompt := filepath.Join(c.RunDir, "prompt.txt")
			require(t, os.WriteFile(prompt, []byte("probe"), 0600))
			t.Setenv("STEERLINE_FAKE_ORCA_SEND_RECEIPT", mode)
			receipt, err := (orcaClient{cfg: c}).terminalSend(context.Background(), ref.External, "Read and execute "+prompt+" now. Follow it exactly.")
			if mode == "accepted" {
				require(t, err)
				if !receipt.InputAccepted || !receipt.TurnStarted || receipt.Applied {
					t.Fatalf("receipt conflates input, turn and application: %+v", receipt)
				}
				return
			}
			want := BackendOutcomeUnknown
			if mode == "reject" {
				want = BackendRejected
			}
			var typed *BackendError
			if !errors.As(err, &typed) || typed.Code != want || receipt.InputAccepted {
				t.Fatalf("bad send receipt was accepted: %+v %v", receipt, err)
			}
		})
	}
}

func TestAuditOrcaObserverCannotSettleReplacedOrLiveUnknownExecution(t *testing.T) {
	for _, mode := range []string{"replaced", "disconnected", "oversized", "stalled"} {
		t.Run(mode, func(t *testing.T) {
			c, ref := auditOrcaRef(t)
			c.StallTimeout = "1ms"
			input := filepath.Join(c.RunDir, orcaResultFile)
			output := filepath.Join(c.RunDir, "runs", ref.RunID, "result.json")
			switch mode {
			case "replaced":
				t.Setenv("STEERLINE_FAKE_ORCA_INCARNATION", "new-generation")
				require(t, os.WriteFile(input, []byte("foreign result"), 0600))
			case "disconnected":
				t.Setenv("STEERLINE_FAKE_ORCA_DISCONNECTED", "1")
				require(t, os.WriteFile(input, []byte("unverified result"), 0600))
			case "oversized":
				require(t, os.WriteFile(input, make([]byte, (2<<20)+1), 0600))
			}
			proc := &orcaProcess{cfg: c, term: orcaTerminal{Handle: ref.External.Terminal, RuntimeID: ref.External.RuntimeID, IncarnationID: ref.External.Incarnation}}
			w := superviseOrcaResult(c, proc, input, output)
			defer proc.Close()
			var resultErr error
			select {
			case resultErr = <-w.Done:
			case <-time.After(6 * time.Second):
				t.Fatal("observer failed to terminate")
			}
			var typed *BackendError
			if !errors.As(resultErr, &typed) {
				t.Fatalf("uncertain observation was reported as an ordinary exit: %v", resultErr)
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Fatalf("unverified output was published: %v", err)
			}
			run := &Run{TaskID: "audit", RunID: ref.RunID, AttemptID: ref.AttemptID, Host: "orca", Process: &ref.External, ExecutionStatus: "running", FinishTaskStatus: "pending"}
			s := &Service{Config: c, State: Runtime{SchemaVersion: CurrentRuntimeSchema, Runs: []*Run{run}, SlotOccupied: true}, worker: w, active: run, store: &Store{Path: filepath.Join(c.RunDir, "runtime.json")}, activities: map[string]*ActivityWriter{}}
			require(t, s.finished(resultErr))
			if run.ExecutionStatus != "unknown" || run.EndedAt != nil || !s.State.SlotOccupied {
				t.Fatalf("observer failure released a potentially active execution: %+v", run)
			}
		})
	}
}

func TestAuditRecoveryIncludesStopAndReplyOperations(t *testing.T) {
	for _, action := range []string{"stop", "reply"} {
		t.Run(action, func(t *testing.T) {
			f := newFixture(t, task("audit"))
			s := f.service(t)
			require(t, s.dispatch(context.Background()))
			finishWorker(t, s)
			run := s.State.Runs[0]
			records := executionRecords{root: f.c.RunDir}
			ref, err := records.loadBinding(run.RunID, run.AttemptID)
			require(t, err)
			op := Operation{Version: 1, ID: action + "-audit", RunID: run.RunID, AttemptID: run.AttemptID, Backend: ref.Backend, Protocol: ref.Protocol, Target: ref.Target, Action: action, PayloadDigest: digestValue("fixed payload"), Status: OperationUnknown, Ref: &ref}
			require(t, records.saveOperation(op))
			restarted, err := NewService(f.c)
			require(t, err)
			if restarted.State.Runs[0].ExecutionStatus != "unknown" || !restarted.State.SlotOccupied {
				t.Fatal("runtime projection hid an unresolved external operation")
			}
			require(t, os.WriteFile(records.operationPath(run.RunID, run.AttemptID, op.ID), []byte("{broken"), 0600))
			if _, err := NewService(f.c); err == nil {
				t.Fatal("corrupt non-start operation was ignored during recovery")
			}
		})
	}
}

func TestAuditLocalCancellationAfterPrepareDoesNotLaunch(t *testing.T) {
	f := newFixture(t, task("audit"))
	require(t, f.c.prepareWorktreeRoot())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runID := newID()
	base, err := fetchBase(ctx, f.c, runID)
	require(t, err)
	intent, err := planWorkspace(f.c, "audit", runID, base)
	require(t, err)
	s := f.service(t)
	claimed, err := s.beads.Claim(ctx, "audit")
	require(t, err)
	if !claimed {
		t.Fatal("fixture claim failed")
	}
	source, err := s.beads.Context(ctx)
	require(t, err)
	dir := filepath.Join(f.c.RunDir, "runs", runID)
	run := Run{TaskID: "audit", RunID: runID, AttemptID: runID, Claimant: f.c.Claimant, WorkspaceIntent: intent, ResultPath: ptr(filepath.Join(dir, "result.json")), LogPath: ptr(filepath.Join(dir, "events.jsonl"))}
	backend := localExecutionBackend{config: f.c, source: source, afterPrepare: cancel}
	_, err = backend.Start(ctx, AttemptSpec{Run: run, Task: task("audit"), RunID: runID, AttemptID: runID, Workspace: *intent, Beads: source, Target: targetFor(f.c), Agent: agentFor(f.c)})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation after prepare was ignored: %v", err)
	}
	if len(readFake(f.db).Starts) != 0 {
		t.Fatal("a worker launched after cancellation")
	}
}

func TestAuditOrcaWorkspaceMustBeVerifiedBeforeDispatch(t *testing.T) {
	f := newFixture(t, task("audit"))
	cfg := orcaTestConfig(t, f.c.RunDir, "")
	f.c.Host, f.c.Orca = "orca", cfg.Orca
	t.Setenv("STEERLINE_FAKE_ORCA_WRONG_WORKSPACE_BRANCH", "1")
	s := f.service(t)
	if err := s.dispatch(context.Background()); err == nil {
		t.Fatal("Orca workspace claims were trusted without Git verification")
	}
	run := s.State.Runs[0]
	state, err := readFakeOrcaState(f.c.RunDir, "term_fake_"+orcaSafeName(run.TaskID, run.RunID))
	require(t, err)
	if state.Sends != 0 || run.ExecutionStatus != "unknown" || !s.State.SlotOccupied {
		t.Fatalf("unverified workspace received task instructions: sends=%d run=%+v", state.Sends, run)
	}
}

func TestAuditLocalRejectedLaunchCanRetryPreparedWorkspace(t *testing.T) {
	f := newFixture(t, task("t-1"))
	s := f.service(t)
	runID := newID()
	s.reservedRunID = runID
	// Fail before spawning: stderr cannot be created over this directory.
	fault := filepath.Join(f.c.RunDir, "runs", runID, "stderr.log")
	require(t, os.MkdirAll(fault, 0700))
	require(t, s.dispatch(context.Background()))
	run := s.State.Runs[0]
	if run.Process != nil || run.ExecutionStatus != "failed" || run.FinishTaskStatus != "blocked" {
		t.Fatalf("fixture did not reject launch before creating a process: %+v", run)
	}
	op, err := (executionRecords{root: f.c.RunDir}).loadOperation(runID, runID, attemptOperationID(runID, runID))
	require(t, err)
	if len(op.ResidualResources) != 1 || op.ResidualResources[0] != run.WorkspaceIntent.Path {
		t.Fatal("rejected launch lost its prepared workspace residual")
	}
	require(t, os.Remove(fault))
	restarted := f.service(t)
	require(t, restarted.startRetry(context.Background(), runID))
	finishWorker(t, restarted)
	if restarted.State.Runs[0].FinishTaskStatus != "waiting_pr_merge" || readFake(f.db).Claims["t-1"] != 1 {
		t.Fatalf("prepared launch retry lost workspace or re-claimed task: %+v", restarted.State.Runs[0])
	}
}

func TestAuditManualStopSurvivesRecoveryWithoutRewritingUnknownOutcome(t *testing.T) {
	f, s, run := failedStartup(t)
	records := executionRecords{root: f.c.RunDir}
	op, err := records.loadOperation(run.RunID, run.AttemptID, attemptOperationID(run.RunID, run.AttemptID))
	require(t, err)
	op.Status = OperationUnknown
	require(t, records.saveOperation(op))
	run.ExecutionStatus = "unknown"
	require(t, s.save())
	require(t, s.MarkStopped(run.RunID))
	restarted, err := NewService(f.c)
	require(t, err)
	if restarted.State.Runs[0].ExecutionStatus != "failed" || restarted.State.SlotOccupied {
		t.Fatal("recovery erased the operator's explicit stop confirmation")
	}
	stored, err := records.loadOperation(run.RunID, run.AttemptID, op.ID)
	require(t, err)
	if stored.Status != OperationUnknown {
		t.Fatal("operator stop invented a successful platform operation outcome")
	}
	require(t, restarted.startRetry(context.Background(), run.RunID))
	finishWorker(t, restarted)
	if restarted.State.Runs[0].ManualStop != nil {
		t.Fatal("manual stop confirmation leaked to the new attempt")
	}
}

func TestAuditOrcaNativeEvidenceCannotEnableCodexOverride(t *testing.T) {
	f := newFixture(t)
	cfg := orcaTestConfig(t, f.c.RunDir, "")
	f.c.Host, f.c.Orca = "orca", cfg.Orca
	s := f.service(t)
	_, err := s.requireBackendCapabilities(context.Background(), backendFor(s.Config, BeadsContext{}), "start")
	require(t, err)
	s.Config.Model = "different-launch-path"
	report, err := s.requireBackendCapabilities(context.Background(), backendFor(s.Config, BeadsContext{}), "start")
	if err == nil || report.AgentVersion != "fake-codex/1" || capabilityStatus(report, "start") != CapabilityNotTested {
		t.Fatalf("native agent verification enabled an unverified CLI override: %+v %v", report, err)
	}
}

package integration

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/execution/adapters/orca"
	remote "github.com/erranliu/SteerLine/internal/execution/adapters/orcaremote"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

// Protocol fixtures invoke production adapters without claiming a Service run
// or fabricating permission enforcement evidence. Admission is tested separately.
type remoteProtocolFixture struct {
	config                 remote.Config
	client                 remote.Client
	dir, runID, worktreeID string
}

func newRemoteProtocolFixture(t *testing.T) *remoteProtocolFixture {
	t.Helper()
	dir := t.TempDir()
	state := remoteFixtureState{Origin: "https://git.example/org/repo.git", Base: strings.Repeat("a", 40), Name: "worker", Path: "/srv/steerline/worker", Branch: "remote/worker", Coordinator: "term-remote-coordinator"}
	require(t, os.WriteFile(filepath.Join(dir, "remote.json"), mustJSON(state), 0600))
	cfg := remote.Config{Repository: dir, ProjectID: "project", Agent: "codex", Model: "gpt-6-luna", ReasoningEffort: "high", CommandTimeout: "5s",
		Settings: remote.Settings{ConnectionID: "home-connection", EnvironmentID: "env-home", RepositorySelector: "id:repo-test", CoordinatorTerminal: state.Coordinator, PollInterval: "1ms"},
		Call: func(ctx context.Context, timeout time.Duration, args ...string) ([]byte, error) {
			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			return exec.CommandContext(ctx, os.Args[0], append([]string{"--fake-remote", dir}, args...)...).Output()
		}}
	cfg.ExpectedRemoteRuntime = "peer"
	cfg.Origin = func(context.Context) ([]byte, error) { return []byte(state.Origin), nil }
	f := &remoteProtocolFixture{config: cfg, client: remote.New(cfg), dir: dir, runID: newID(), worktreeID: "repo-test::" + state.Path}
	_, err := orcaworker.CreateRun(context.Background(), f.client.CenterCall, "remote protocol fixture", state.Coordinator, "center")
	require(t, err)
	return f
}

func readRemoteFixture(t *testing.T, dir string) remoteFixtureState {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "remote.json"))
	require(t, err)
	var state remoteFixtureState
	require(t, json.Unmarshal(raw, &state))
	return state
}

func changeRemoteFixture(t *testing.T, dir string, change func(*remoteFixtureState)) {
	t.Helper()
	state := readRemoteFixture(t, dir)
	change(&state)
	require(t, AtomicWrite(filepath.Join(dir, "remote.json"), mustJSON(state)))
}

func (f *remoteProtocolFixture) task(t *testing.T, prompt string) string {
	t.Helper()
	receipt, err := orcaworker.CreateTask(context.Background(), f.client.CenterCall, prompt, "fixture task", "remote-run", f.config.Settings.CoordinatorTerminal, "center")
	require(t, err)
	return receipt.ID
}

func (f *remoteProtocolFixture) bind(t *testing.T, start remote.StartReceipt, previous *executionbackend.ProcessIdentity) remote.Ref {
	t.Helper()
	state := readRemoteFixture(t, f.dir)
	identity, err := f.client.BindStart(context.Background(), start, previous, state.Path, state.Branch)
	require(t, err)
	target := executionbackend.ExecutionTarget{ID: "orca-supervised:home-connection", ConnectionID: "home-connection", ExternalRuntime: "env-home"}
	target.OrchestrationRun, target.ExternalTask, target.Dispatch, target.ExecutionHost, target.WorktreeID, target.CoordinatorTerminal = identity.OrchestrationRun, identity.Task, identity.Dispatch, identity.ExecutionHost, identity.WorktreeID, state.Coordinator
	return remote.Ref{Backend: executionbackend.BackendOrca, Protocol: executionbackend.ProtocolOrcaSupervised, RunID: f.runID, Target: target, External: identity, WorkspacePath: state.Path, WorkspaceBranch: state.Branch, BaseCommit: state.Base}
}

func (f *remoteProtocolFixture) start(t *testing.T, previous *executionbackend.ProcessIdentity) remote.Ref {
	t.Helper()
	task := f.task(t, "Continue original remote work")
	receipt, err := f.client.StartTask(context.Background(), "remote-run", task, f.worktreeID, previous)
	require(t, err)
	return f.bind(t, receipt, previous)
}

func (f *remoteProtocolFixture) reconnect() {
	f.client = remote.New(f.config)
}

func (f *remoteProtocolFixture) finish(t *testing.T, ref remote.Ref) {
	t.Helper()
	ctx := context.Background()
	changeRemoteFixture(t, f.dir, func(s *remoteFixtureState) { s.Finish = true })
	delivery, err := f.client.Check(ctx, ref)
	require(t, err)
	if len(delivery.Messages) != 1 || delivery.Messages[0].Type != "worker_done" {
		t.Fatalf("missing original done receipt: %+v", delivery)
	}
	_, err = f.client.Ack(ctx, ref, delivery.ID)
	require(t, err)
	outcome, err := f.client.Settlement(ctx, ref, "succeeded")
	require(t, err)
	if outcome != "succeeded" {
		t.Fatalf("settlement=%s", outcome)
	}
}

func testRemoteOrcaProtocolTwoTurnsKeepSessionAndSettleResources(t *testing.T) {
	ctx := context.Background()
	f := newRemoteProtocolFixture(t)
	first := f.start(t, nil)
	if _, err := f.client.Release(ctx, first); err == nil || readRemoteFixture(t, f.dir).Releases != 0 {
		t.Fatal("active worker was released")
	}
	f.finish(t, first)
	f.reconnect()
	changeRemoteFixture(t, f.dir, func(s *remoteFixtureState) { s.Finish = false })
	second := f.start(t, &first.External)
	if remote.SessionID(first.External) != remote.SessionID(second.External) || first.WorkspacePath != second.WorkspacePath || first.External.Dispatch == second.External.Dispatch || readRemoteFixture(t, f.dir).Starts != 2 {
		t.Fatal("continuation replaced session/workspace or duplicated start")
	}
	f.finish(t, second)
	_, err := f.client.Release(ctx, second)
	require(t, err)
	coordinator, err := f.client.ShowCoordinator(ctx, second.Target.CoordinatorTerminal, "center")
	require(t, err)
	_, err = f.client.CloseCoordinator(ctx, second, coordinator, false)
	require(t, err)
	state := readRemoteFixture(t, f.dir)
	if state.Releases != 1 || state.CoordinatorCloses != 1 {
		t.Fatalf("settled resources not cleaned: %+v", state)
	}
}

func testRemoteOrcaProtocolLostStartReconcilesWithoutResend(t *testing.T) {
	ctx := context.Background()
	f := newRemoteProtocolFixture(t)
	task := f.task(t, "original saved input")
	changeRemoteFixture(t, f.dir, func(s *remoteFixtureState) { s.LoseStart = true })
	_, err := f.client.StartTask(ctx, "remote-run", task, f.worktreeID, nil)
	if !isBackendError(err, executionbackend.BackendOutcomeUnknown) {
		t.Fatalf("lost receipt guessed successful start: %v", err)
	}
	f.reconnect()
	for i := 0; i < 2; i++ {
		receipt, err := f.client.RecoverStart(ctx, "remote-run", task, f.worktreeID, "center")
		require(t, err)
		ref := f.bind(t, receipt, nil)
		if ref.External.Dispatch != "dispatch-1" {
			t.Fatal("recovery replaced original Dispatch")
		}
	}
	changeRemoteFixture(t, f.dir, func(s *remoteFixtureState) { s.Offline = true })
	if _, err = f.client.RecoverStart(ctx, "remote-run", task, f.worktreeID, "center"); err == nil {
		t.Fatal("offline observation was guessed")
	}
	if readRemoteFixture(t, f.dir).Starts != 1 {
		t.Fatal("lost receipt recovery sent another start")
	}
}

func testRemoteOrcaProtocolQuestionLostReplyAndAckAreReconciled(t *testing.T) {
	ctx := context.Background()
	f := newRemoteProtocolFixture(t)
	ref := f.start(t, nil)
	changeRemoteFixture(t, f.dir, func(s *remoteFixtureState) { s.Question, s.LoseAck = true, true })
	delivery, err := f.client.Check(ctx, ref)
	require(t, err)
	if len(delivery.Messages) != 1 || delivery.Messages[0].Type != "question" {
		t.Fatalf("question missing: %+v", delivery)
	}
	_, err = f.client.Ack(ctx, ref, delivery.ID)
	if !isBackendError(err, executionbackend.BackendOutcomeUnknown) {
		t.Fatalf("lost ack receipt not uncertain: %v", err)
	}
	acknowledged, err := f.client.DeliveryAcknowledged(ctx, ref, delivery.ID)
	require(t, err)
	if !acknowledged {
		t.Fatal("original acknowledgement not reconciled")
	}
	changeRemoteFixture(t, f.dir, func(s *remoteFixtureState) { s.LoseReply = true })
	question := delivery.Messages[0].ID
	_, err = f.client.Reply(ctx, ref, question, "left")
	if !isBackendError(err, executionbackend.BackendOutcomeUnknown) {
		t.Fatalf("lost reply not uncertain: %v", err)
	}
	id, err := f.client.FindAnswer(ctx, ref, question, "left")
	require(t, err)
	if id == "" || readRemoteFixture(t, f.dir).Replies != 1 {
		t.Fatal("answer reconciliation repeated or lost original reply")
	}
}

func testRemoteOrcaProtocolRejectsReplacementSession(t *testing.T) {
	ctx := context.Background()
	f := newRemoteProtocolFixture(t)
	ref := f.start(t, nil)
	f.finish(t, ref)
	changeRemoteFixture(t, f.dir, func(s *remoteFixtureState) { s.ChangedIdentity = true })
	if err := f.client.VerifyWorker(ctx, ref); !isBackendError(err, executionbackend.BackendIdentityMismatch) {
		t.Fatalf("replacement identity was accepted: %v", err)
	}
	task := f.task(t, "continue original session")
	if _, err := f.client.StartTask(ctx, "remote-run", task, f.worktreeID, &ref.External); err == nil {
		t.Fatal("replacement agent received continuation")
	}
	if readRemoteFixture(t, f.dir).Starts != 1 {
		t.Fatal("replacement agent was started")
	}
}

func testCD64RemoteProtocolAssertions(t *testing.T) {
	t.Run("two_turns_and_settlement", testRemoteOrcaProtocolTwoTurnsKeepSessionAndSettleResources)
	t.Run("lost_start_no_resend", testRemoteOrcaProtocolLostStartReconcilesWithoutResend)
	t.Run("lost_reply_and_ack", testRemoteOrcaProtocolQuestionLostReplyAndAckAreReconciled)
	t.Run("replacement_session_rejected", testRemoteOrcaProtocolRejectsReplacementSession)
}

func require(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func newID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(value[:])
}

func mustJSON(value any) []byte {
	wire, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return wire
}

func AtomicWrite(path string, value []byte) error {
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, value, 0600); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

func isBackendError(err error, code executionbackend.BackendErrorCode) bool {
	var remoteErr *remote.Error
	return errors.As(err, &remoteErr) && remoteErr.Code == code
}

package steerline

import (
	"context"
	"encoding/json"
	"github.com/erranliu/SteerLine/internal/steerline/publication"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func remoteServiceFixture(t *testing.T) (*Service, *Run, string) {
	t.Helper()
	ctx := context.Background()
	c, commit := nativeTaskFixture(t)
	if out, err := exec.Command("git", "-C", c.Repository, "push", "--quiet", "origin", "HEAD:refs/heads/main").CombinedOutput(); err != nil {
		t.Fatalf("fixture push: %v %s", err, out)
	}
	origin, err := exec.Command("git", "-C", c.Repository, "remote", "get-url", "origin").Output()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("STEERLINE_FAKE_GIT_FETCH_REMOTE", strings.TrimSpace(string(origin)))
	require(t, exec.Command("git", "-C", c.Repository, "remote", "set-url", "origin", "https://git.example/org/repo.git").Run())
	c.Git = Command{Executable: os.Args[0], Args: []string{"--fake", "git-proxy", "git"}}
	dir := t.TempDir()
	state := remoteFixtureState{Origin: "https://git.example/org/repo.git"}
	data, _ := json.Marshal(state)
	require(t, os.WriteFile(filepath.Join(dir, "remote.json"), data, 0600))
	c.PR = publication.Config{Kind: "command", BaseURL: "https://git.example", Repository: "org/repo", Query: Command{Executable: os.Args[0]}}
	c.PublicationPolicy.Publisher = "worker"
	c.Host, c.Agent, c.Model, c.ReasoningEffort, c.Sandbox, c.Claimant = "orca_supervised", "codex", "gpt-6-luna", "high", "workspace-write", "operator"
	c.Orca = OrcaConfig{Executable: os.Args[0], Args: []string{"--fake", "orca-remote-worker", dir}, Shell: "powershell", WaitTimeout: "2s", Supervised: &SupervisedOrcaConfig{ConnectionID: "home-connection", EnvironmentID: "env-home", RepositorySelector: "id:repo-test", CoordinatorTerminal: "bootstrap", PollInterval: "1ms", CapabilityReportVersion: 1}}
	project, err := OpenNativeTaskProject(ctx, &c)
	require(t, err)
	require(t, project.Close())
	store, err := OpenSQLiteStore(filepath.Join(c.RunDir, "steerline.db"))
	require(t, err)
	t.Cleanup(func() { _ = store.Close() })
	commands := CommandService{Store: store, Project: c.Identity()}
	input := taskBatchFixture(t, c, commit, batchTasks(901), nil)
	validation, err := commands.ValidateTaskBatch(ctx, c, input)
	if err != nil || !validation.Valid {
		t.Fatalf("validation: %+v %v", validation, err)
	}
	_, err = commands.CreateTaskBatch(ctx, c, input, newID(), validation.AuthorityVersion, "operator")
	require(t, err)
	authority, err := store.nativeAuthority(ctx, c.Identity())
	require(t, err)
	authority.AuthorityVersion++
	authority.DispatchPaused = false
	require(t, store.Transact(ctx, func(tx DomainTx) error {
		return tx.(*sqliteTx).UpdateProjectTaskAuthorityCAS(authority, authority.AuthorityVersion-1)
	}))
	_, err = commands.ClaimNativeTask(ctx, c, TaskClaimOptions{TaskID: "901", RequestID: newID(), ExpectedTaskVersion: 1, ExpectedAuthorityVersion: authority.AuthorityVersion, RequestedBy: "operator", Explicit: true})
	require(t, err)
	s, err := NewService(c)
	require(t, err)
	t.Cleanup(func() { _ = s.Close() })
	run := s.find("901")
	if run == nil {
		t.Fatal("remote claim was not reconstructed")
	}
	return s, run, dir
}

func readRemoteFixture(t *testing.T, dir string) remoteFixtureState {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "remote.json"))
	require(t, err)
	var state remoteFixtureState
	require(t, json.Unmarshal(raw, &state))
	return state
}
func changeRemoteFixture(t *testing.T, dir string, fn func(*remoteFixtureState)) {
	t.Helper()
	state := readRemoteFixture(t, dir)
	fn(&state)
	raw, _ := json.Marshal(state)
	require(t, AtomicWrite(filepath.Join(dir, "remote.json"), raw))
}

func saveRemoteAnswer(t *testing.T, s *Service, run *Run, note string, native bool) {
	t.Helper()
	s.State.Service.Ready, s.State.Service.Status = true, "running"
	require(t, s.publishCheckpoint())
	answer := DecisionAnswer{DecisionID: run.Decision.ID, RunID: run.RunID, Note: note, AnsweredBy: "operator", ExpectedDecisionVersion: run.Decision.Version, ExpectedRunRevision: run.ControlRevision, SourceAttemptID: run.AttemptID, ExpectedRecoveryVersion: run.RecoveryVersion, ServiceInstanceID: s.State.Service.InstanceID}
	if native {
		answer.Note = ""
		answer.Answers = map[string][]string{orcaQuestionID(run.Decision): {note}}
	}
	commands := CommandService{Store: s.domainStore, Project: s.Config.Identity()}
	command, _, err := commands.AnswerDecisionWithID(context.Background(), newID(), answer.DecisionID, answer.ExpectedDecisionVersion, decisionAnswerKey(answer), answer.AnsweredBy, mustJSON(answer))
	require(t, err)
	_, _, err = commands.AcceptAnswer(context.Background(), command, answer)
	require(t, err)
	require(t, s.refreshNativeSnapshot(context.Background()))
}

func TestRemoteOrcaTwoTurnsKeepSessionAndRequireMergedDelivery(t *testing.T) {
	ctx := context.Background()
	s, run, dir := remoteServiceFixture(t)
	require(t, s.prepareWorkerFlow(ctx, *nativeTaskRecord(s.domain.Tasks, "901"), run))
	require(t, s.resumeWorkerFlow(ctx))
	firstSession, firstPath := deref(run.SessionID), deref(run.WorktreePath)
	if firstSession == "" || !strings.HasPrefix(firstPath, "/srv/") || run.Process.Kind != "orca_supervised" {
		t.Fatalf("remote identity not bound: %+v", run.Process)
	}
	changeRemoteFixture(t, dir, func(state *remoteFixtureState) { state.Finish = true })
	require(t, s.resumeWorkerFlow(ctx))
	if run.Decision == nil {
		t.Fatal("settled remote turn has no feedback decision")
	}
	c := s.Config
	require(t, s.Close())
	var err error
	s, err = NewService(c)
	require(t, err)
	defer s.Close()
	run = s.find("901")
	s.markServiceRecoveryGate()
	require(t, s.commitWorker(run))
	require(t, s.resumeConfiguredWork(ctx))
	changeRemoteFixture(t, dir, func(state *remoteFixtureState) { state.Finish = false })
	saveRemoteAnswer(t, s, run, "Continue original remote work", false)
	require(t, s.applyWorkerFlowDecision(ctx, run))
	require(t, s.resumeWorkerFlow(ctx))
	if deref(run.SessionID) != firstSession || deref(run.WorktreePath) != firstPath || readRemoteFixture(t, dir).Starts != 2 {
		t.Fatalf("remote continuation changed session/workspace or duplicated start: session=%q first=%q path=%q firstPath=%q starts=%d recovery=%s required=%t error=%q decision=%+v", deref(run.SessionID), firstSession, deref(run.WorktreePath), firstPath, readRemoteFixture(t, dir).Starts, run.RecoveryState, run.RecoveryRequired, deref(run.Error), run.Decision)
	}
	if observation := s.inspectRunProcess(run); observation != "matching process still exists" {
		t.Fatalf("current remote Dispatch was not inspected through original session: %s", observation)
	}
	changeRemoteFixture(t, dir, func(state *remoteFixtureState) { state.Finish = true })
	require(t, s.resumeWorkerFlow(ctx))
	workflow, execution := s.workflowRecords(run)
	url := "https://git.example/org/repo/pulls/901"
	info := PRInfo{URL: url, Repository: s.Config.PR.Repository, BaseBranch: "main", HeadRepository: s.Config.PR.Repository, HeadBranch: execution.Workspace.Branch, HeadRevision: execution.Workspace.Head, State: "open"}
	require(t, BindWorkerPR(workflow, execution, info, url, s.Config.PR.Repository, false))
	s.refreshRun(run)
	require(t, s.commitWorker(run))
	verifier := &fixedTaskDeliveryVerifier{info: info}
	task := nativeTaskRecord(s.domain.Tasks, "901")
	_, err = (CommandService{Store: s.domainStore, Project: s.Config.Identity()}).CompleteNativeTask(ctx, s.Config, "901", task.Version, run.RunID, verifier)
	if err == nil || readRemoteFixture(t, dir).Releases != 0 {
		t.Fatal("open PR completed or released remote task")
	}
	merged, at := strings.Repeat("c", 40), now()
	verifier.info.State, verifier.info.MergeCommit, verifier.info.MergedAt, verifier.info.Merged = "merged", &merged, &at, true
	_, err = (CommandService{Store: s.domainStore, Project: s.Config.Identity()}).CompleteNativeTask(ctx, s.Config, "901", task.Version, run.RunID, verifier)
	require(t, err)
	require(t, s.refreshNativeSnapshot(ctx))
	run = s.find("901")
	require(t, s.resumeConfiguredWork(ctx))
	if run.TaskStatus != "closed" || run.FinishTaskStatus != "completed" || (readRemoteFixture(t, dir).Releases != 1 || readRemoteFixture(t, dir).CoordinatorCloses != 1) {
		t.Fatalf("merged remote task not delivered/cleaned: %+v", run)
	}
	require(t, s.resumeConfiguredWork(ctx))
	if readRemoteFixture(t, dir).Releases != 1 || readRemoteFixture(t, dir).CoordinatorCloses != 1 {
		t.Fatal("remote release repeated")
	}
}

func TestRemoteOrcaLostStartReconcilesAfterRestartWithoutResend(t *testing.T) {
	ctx := context.Background()
	s, run, dir := remoteServiceFixture(t)
	changeRemoteFixture(t, dir, func(state *remoteFixtureState) { state.LoseStart = true })
	require(t, s.prepareWorkerFlow(ctx, *nativeTaskRecord(s.domain.Tasks, "901"), run))
	if run.Process != nil {
		t.Fatal("lost receipt guessed worker identity")
	}
	c := s.Config
	require(t, s.Close())
	s, err := NewService(c)
	require(t, err)
	defer s.Close()
	run = s.find("901")
	s.markServiceRecoveryGate()
	require(t, s.commitWorker(run))
	require(t, s.resumeConfiguredWork(ctx))
	if readRemoteFixture(t, dir).Starts != 1 || run.Process == nil || deref(run.SessionID) == "" {
		t.Fatalf("original start not reconciled: %+v", run)
	}
	require(t, s.resumeConfiguredWork(ctx))
	if readRemoteFixture(t, dir).Starts != 1 {
		t.Fatal("restart repeated remote start")
	}
	changeRemoteFixture(t, dir, func(state *remoteFixtureState) { state.Offline = true })
	require(t, s.resumeConfiguredWork(ctx))
	if run.RecoveryState != "unverified" || readRemoteFixture(t, dir).Starts != 1 {
		t.Fatal("network loss retried/replaced remote worker")
	}
}

func TestRemoteOrcaQuestionLostReplyAndAckAreReconciled(t *testing.T) {
	ctx := context.Background()
	s, run, dir := remoteServiceFixture(t)
	changeRemoteFixture(t, dir, func(state *remoteFixtureState) { state.Question, state.LoseAck = true, true })
	require(t, s.prepareWorkerFlow(ctx, *nativeTaskRecord(s.domain.Tasks, "901"), run))
	require(t, s.resumeWorkerFlow(ctx))
	if run.Decision == nil || run.Decision.Request.OrcaAsk == nil {
		t.Fatal("remote question did not enter shared decision")
	}
	require(t, s.resumeWorkerFlow(ctx))
	changeRemoteFixture(t, dir, func(state *remoteFixtureState) { state.LoseReply = true })
	saveRemoteAnswer(t, s, run, "left", true)
	require(t, s.applyWorkerFlowDecision(ctx, run))
	require(t, s.applyWorkerFlowDecision(ctx, run))
	if readRemoteFixture(t, dir).Replies != 1 || run.Decision.Status != "reply_accepted" {
		t.Fatalf("remote reply repeated or not recovered: %+v", run.Decision)
	}
}

func TestRemoteOrcaRejectsReplacementSession(t *testing.T) {
	ctx := context.Background()
	s, run, dir := remoteServiceFixture(t)
	require(t, s.prepareWorkerFlow(ctx, *nativeTaskRecord(s.domain.Tasks, "901"), run))
	require(t, s.resumeWorkerFlow(ctx))
	changeRemoteFixture(t, dir, func(state *remoteFixtureState) { state.Finish = true })
	require(t, s.resumeWorkerFlow(ctx))
	changeRemoteFixture(t, dir, func(state *remoteFixtureState) { state.ChangedIdentity = true; state.Finish = false })
	saveRemoteAnswer(t, s, run, "Continue", false)
	require(t, s.applyWorkerFlowDecision(ctx, run))
	if readRemoteFixture(t, dir).Starts != 1 || run.Decision.Status != "answer_saved" {
		t.Fatal("replacement agent received feedback")
	}
}

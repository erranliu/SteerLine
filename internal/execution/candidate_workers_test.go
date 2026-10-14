package execution

import (
	"context"
	"errors"
	"fmt"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/product/query"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type mcFixture struct {
	s        *Service
	c        Config
	store    *sqliteFixture
	provider string
	first    *reconstructiontypes.RunView
	commands CommandService
}

func newMCFixture(t *testing.T) mcFixture {
	t.Helper()
	c, commit, store := openDAGFixture(t)
	provider := t.TempDir()
	c.PR.Query = executionbackend.Command{Executable: os.Args[0], Args: []string{"mc-provider", provider, "{number}"}}
	setNeutralProfile(t, &c, neutralSettings{Executable: os.Args[0], Model: "candidate-a", ReasoningEffort: "medium"})
	commands := newTestCommandService(store, c)
	input := taskBatchFixture(t, c, commit, batchTasks(8, 9), []taskmodel.TaskBatchEdge{{Predecessor: taskmodel.TaskBatchReference{LocalKey: "k8"}, Successor: taskmodel.TaskBatchReference{LocalKey: "k9"}}})
	_, err := commands.CreateTaskBatch(context.Background(), c, input, newID(), 1, "operator")
	require(t, err)
	authority, err := store.NativeAuthority(context.Background(), c.Identity())
	require(t, err)
	authority.DispatchPaused = false
	authority.AuthorityVersion++
	require(t, store.transactRecords(context.Background(), func(tx *fixtureTransaction) error {
		return tx.UpdateProjectTaskAuthorityCAS(authority, authority.AuthorityVersion-1)
	}))
	out, err := exec.Command("git", "-C", c.Repository, "push", "--quiet", "origin", "HEAD:refs/heads/main").CombinedOutput()
	if err != nil {
		t.Fatalf("isolated push: %v: %s", err, out)
	}
	receipt, err := commands.ClaimNativeTask(context.Background(), c, TaskClaimOptions{TaskID: "8", RequestID: newID(), ExpectedTaskVersion: 1, ExpectedAuthorityVersion: authority.AuthorityVersion, RequestedBy: "operator", Explicit: true})
	require(t, err)
	s, err := newNeutralService(c)
	require(t, err)
	t.Cleanup(func() { _ = s.Close() })
	s.lifecycleResolver = mcResolver{}
	s.State.Service.Ready = true
	require(t, s.publishCheckpoint())
	return mcFixture{s: s, c: c, store: store, provider: provider, first: s.find(receipt.WorkflowRunID), commands: commands}
}

type mcResolver struct{}

func (mcResolver) ResolveLifecycle(setup executionbackend.LifecycleSetup) (executionbackend.LifecyclePort, error) {
	return mcPort{&neutralLifecycle{Setup: setup}}, nil
}

type mcPort struct{ *neutralLifecycle }

func (p mcPort) Execute(ctx context.Context, r executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	receipt, observer, err := p.neutralLifecycle.Execute(ctx, r)
	if r.Operation.Action == string(executionbackend.LifecyclePrepareSession) && err == nil {
		receipt.State.SessionID = "session-" + r.Snapshot.Session.ExecutionID
		envelope, envelopeErr := executionbackend.NewImplementationDataEnvelope(executionbackend.ImplementationDataEnvelope{ImplementationKey: neutralBindingKey, CodecVersion: 1, Purpose: executionbackend.ImplementationResourcePurpose, ProjectID: r.Snapshot.Session.ProjectID, ExecutionID: r.Snapshot.Session.ExecutionID, AttemptID: r.Snapshot.Session.AttemptID, Generation: r.Snapshot.ResourceGeneration, ConfigDigest: r.Snapshot.ConfigurationDigest}, mustJSON(neutralBinding{SessionID: receipt.State.SessionID, Generation: receipt.State.SessionGeneration}))
		if envelopeErr != nil {
			return receipt, observer, envelopeErr
		}
		receipt.Implementation = append(receipt.Implementation, envelope)
	}
	return receipt, observer, err
}

func (f mcFixture) start(t *testing.T, model string) (executionmodel.TaskClaimReceipt, TaskClaimOptions) {
	t.Helper()
	graph, err := f.store.TaskGraphSnapshot(context.Background(), f.c.Identity())
	require(t, err)
	c := f.c
	setNeutralProfile(t, &c, neutralSettings{Executable: os.Args[0], Model: model, ReasoningEffort: "high"})
	p := testProjectExecutionProfile(c)
	dependencies, _ := neutralFixtureRuntime(c)
	fixed := p.Freeze(dependencies.Directory, c.Identity())
	options := TaskClaimOptions{TaskID: "8", RequestID: newID(), ExpectedTaskVersion: graph.ByID["8"].Version, ExpectedAuthorityVersion: graph.Authority.AuthorityVersion, ExpectedGraphRevision: graph.Authority.GraphRevision, ExecutionProfile: &fixed, RequestedBy: "operator", ServiceInstanceID: f.s.State.Service.InstanceID}
	receipt, err := f.commands.StartNativeCandidate(context.Background(), c, options)
	require(t, err)
	t.Logf("candidate start receipt: %s", mustJSON(receipt))
	require(t, f.s.refreshNativeSnapshot(context.Background()))
	return receipt, options
}

func mcStartTurn(t *testing.T, s *Service, run *reconstructiontypes.RunView) {
	t.Helper()
	task := nativeTaskRecord(s.domain.Tasks, run.TaskID)
	require(t, s.prepareWorkerFlow(context.Background(), *task, run))
	if s.workers[run.RunID] == nil {
		t.Fatalf("candidate did not launch: %s", deref(run.Error))
	}
}

func mcFinishTurn(t *testing.T, s *Service, run *reconstructiontypes.RunView) {
	t.Helper()
	for stage := 0; stage < 3; stage++ {
		w := s.workers[run.RunID]
		if w == nil {
			workflow, execution := s.workflowRecords(run)
			t.Fatalf("original candidate observer missing: step=%s status=%s pending=%s reason=%s", workflow.StepID, workflow.Status, execution.PendingCommandID, deref(execution.Error))
		}
		for ev := range w.Events {
			require(t, s.eventForRun(run, ev))
		}
		err := <-w.Done
		delete(s.workers, run.RunID)
		require(t, s.finishedForRun(run, w, err))
		workflow, execution := s.workflowRecords(run)
		if workflow.Status != "queued" || execution.PendingCommandID == "" {
			return
		}
		dispatched, err := s.dispatchQueuedStage(context.Background(), run)
		require(t, err)
		if !dispatched {
			t.Fatal("queued candidate stage was not dispatched")
		}
	}
	t.Fatal("candidate fixture exceeded its staged turn limit")
}

func (f mcFixture) proof(t *testing.T, run *reconstructiontypes.RunView, number int, state string) executionmodel.PRInfo {
	t.Helper()
	e := f.s.executionRecord(run)
	info := executionmodel.PRInfo{URL: fmt.Sprintf("https://github.com/owner/repo/pull/%d", number), Repository: "owner/repo", BaseBranch: "main", HeadRepository: "owner/repo", HeadBranch: e.Workspace.Branch, HeadRevision: e.Workspace.Base, State: state}
	if state == "merged" {
		info.Merged, info.MergedAt, info.MergeCommit = true, ptr(now()), ptr(e.Workspace.Base)
	}
	require(t, os.WriteFile(filepath.Join(f.provider, strconv.Itoa(number)+".json"), mustJSON(info), 0600))
	return info
}

func (f mcFixture) bind(t *testing.T, run *reconstructiontypes.RunView, number int, state string) executionmodel.PRInfo {
	t.Helper()
	info := f.proof(t, run, number, state)
	w, e := f.s.workflowRecords(run)
	require(t, BindWorkerPR(w, e, info, info.URL, info.Repository, false))
	w.StepID, w.Status = executionmodel.StepAwaitMerge, "waiting_external"
	f.s.refreshRun(run)
	require(t, f.s.commitWorker(run))
	t.Logf("candidate PR proof: %s", mustJSON(info))
	return info
}

// Provider and publication helper are separate processes. The worker helper
// really commits and pushes in its own worktree and creates its provider PR.
func mcSubprocess(args []string) int {
	if args[0] == "mc-provider" {
		body, err := os.ReadFile(filepath.Join(args[1], args[2]+".json"))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Println(string(body))
		return 0
	}
	worktree, provider, number := args[1], args[2], args[3]
	if err := os.WriteFile(filepath.Join(worktree, "candidate-result.txt"), []byte(number), 0600); err != nil {
		return 1
	}
	for _, gitArgs := range [][]string{{"add", "candidate-result.txt"}, {"-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "candidate result"}, {"push", "--quiet", "origin", "HEAD"}} {
		command := exec.Command("git", append([]string{"-C", worktree}, gitArgs...)...)
		if out, err := command.CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "%v: %s", err, out)
			return 1
		}
	}
	branch, err := exec.Command("git", "-C", worktree, "branch", "--show-current").Output()
	if err != nil {
		return 1
	}
	head, err := exec.Command("git", "-C", worktree, "rev-parse", "HEAD").Output()
	if err != nil {
		return 1
	}
	info := executionmodel.PRInfo{URL: "https://github.com/owner/repo/pull/" + number, Repository: "owner/repo", BaseBranch: "main", HeadRepository: "owner/repo", HeadBranch: strings.TrimSpace(string(branch)), HeadRevision: strings.TrimSpace(string(head)), State: "open"}
	if err := os.WriteFile(filepath.Join(provider, number+".json"), mustJSON(info), 0600); err != nil {
		return 1
	}
	fmt.Println(info.URL)
	return 0
}

func TestMC1(t *testing.T) {
	f := newMCFixture(t)
	mcStartTurn(t, f.s, f.first)
	mcFinishTurn(t, f.s, f.first)
	base := f.s.executionRecord(f.first).Workspace.Base
	profiles, sessions, branches, worktrees := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	for i := 0; i < 5; i++ {
		run := f.first
		if i > 0 {
			receipt, _ := f.start(t, fmt.Sprintf("candidate-%d", i))
			run = f.s.find(receipt.WorkflowRunID)
			mcStartTurn(t, f.s, run)
			mcFinishTurn(t, f.s, run)
		}
		w, e := f.s.workflowRecords(run)
		for label, value := range map[string]string{"profile": w.ExecutionProfile.Digest, "session": e.SessionID, "branch": e.Workspace.Branch, "worktree": e.Workspace.Worktree} {
			set := map[string]map[string]bool{"profile": profiles, "session": sessions, "branch": branches, "worktree": worktrees}[label]
			if value == "" || set[value] {
				t.Fatalf("candidate %d reused/missed %s: %q", i, label, value)
			}
			set[value] = true
		}
		if e.Workspace.Base != base {
			t.Fatal("candidate changed common base")
		}
	}
	if len(f.s.State.Runs) != 5 || f.s.find("8") != nil || participatingCandidates(f.s.domain.WorkflowRuns, "8") != 5 {
		t.Fatal("projection lost candidates or guessed a task target")
	}
	graph, err := f.store.TaskGraphSnapshot(context.Background(), f.c.Identity())
	require(t, err)
	if graph.ByID["8"].Version != 2 || graph.ByID["8"].Claimant != "operator" {
		t.Fatal("additional candidates claimed task again")
	}
	var claims int
	require(t, f.store.db.QueryRow(`SELECT COUNT(*) FROM commands WHERE kind='claim_task'`).Scan(&claims))
	if claims != 1 {
		t.Fatal("claim repeated")
	}
	page, err := query.NewService(f.store, f.c.Identity()).ReadProjectView(context.Background(), "8", 20)
	require(t, err)
	if len(page.SelectedTask.WorkflowRuns) != 5 {
		t.Fatalf("public query lost current/history candidates: %+v", page)
	}
}

func TestMC2(t *testing.T) {
	f := newMCFixture(t)
	r, options := f.start(t, "candidate-b")
	c := f.c
	setNeutralProfile(t, &c, neutralSettings{Executable: os.Args[0], Model: "candidate-b", ReasoningEffort: "high"})
	first, err := f.commands.StartNativeCandidate(context.Background(), c, options)
	require(t, err)
	if first.WorkflowRunID != r.WorkflowRunID || first.ExecutionID != r.ExecutionID {
		t.Fatal("request replay allocated a replacement")
	}
	for name, alter := range map[string]func(*TaskClaimOptions){"content": func(o *TaskClaimOptions) { o.RequestedBy = "other" }, "task-version": func(o *TaskClaimOptions) { o.RequestID = newID(); o.ExpectedTaskVersion++ }, "graph-version": func(o *TaskClaimOptions) { o.RequestID = newID(); o.ExpectedGraphRevision++ }} {
		t.Run(name, func(t *testing.T) {
			bad := options
			alter(&bad)
			if _, err := f.commands.StartNativeCandidate(context.Background(), c, bad); err == nil {
				t.Fatal("invalid candidate accepted")
			}
		})
	}
	run := f.s.find(r.WorkflowRunID)
	f.s.executionRecord(run).Workspace.Base = strings.Repeat("f", 40)
	require(t, f.s.commitWorker(run))
	bad := options
	bad.RequestID = newID()
	if _, err := f.commands.StartNativeCandidate(context.Background(), c, bad); !isNativeErrorCode(err, "contract_mismatch") {
		t.Fatalf("bad common base: %v", err)
	}
	if len(f.s.domain.WorkflowRuns) != 2 {
		t.Fatal("rejected start created a worker")
	}
	t.Run("inbox_reservation_receipt_loss", func(t *testing.T) {
		f := newMCFixture(t)
		graph, err := f.store.TaskGraphSnapshot(context.Background(), f.c.Identity())
		require(t, err)
		profile := testProjectExecutionProfile(f.c)
		p := productapi.ControlPayload{TaskID: "8", ExpectedTaskVersion: 2, ExpectedGraphRevision: graph.Authority.GraphRevision, ExecutionProfileID: profile.ID, ExecutionProfileDigest: profile.Digest()}
		req := productapi.ControlRequest{ProtocolVersion: ControlProtocolVersion, RequestID: newID(), RepositoryIdentity: f.c.Identity(), InstanceID: f.s.State.Service.InstanceID, ExpectedControlRevision: graph.Authority.AuthorityVersion, Action: "start_candidate", Payload: mustJSON(p), RequestedBy: "operator", CreatedAt: now().Add(-3 * time.Minute), ExpiresAt: now().Add(-time.Minute)}
		require(t, publishControlRequest(f.c.RunDir, req))
		digest := rawRequestDigest(controlRequestPath(f.c.RunDir, req.RequestID))
		// Persist the exact original reservation and simulate losing the inbox
		// receipt. Recovery must precede expiry/version/count rejection.
		dependencies, _ := neutralFixtureRuntime(f.c)
		fixed := profile.Freeze(dependencies.Directory, f.c.Identity())
		saved, err := f.commands.StartNativeCandidate(context.Background(), f.c, TaskClaimOptions{TaskID: "8", RequestID: req.RequestID, WorkflowRunID: req.RequestID, ExpectedTaskVersion: 2, ExpectedAuthorityVersion: graph.Authority.AuthorityVersion, ExpectedGraphRevision: graph.Authority.GraphRevision, RequestedBy: req.RequestedBy, ExecutionProfile: &fixed, ServiceInstanceID: req.InstanceID, controlRequestDigest: digest})
		require(t, err)
		f.s.State.Service.InstanceID = newID()
		require(t, f.s.processNextControl(context.Background(), func() {}))
		receipt, err := ReadControlReceipt(f.c, req.RequestID)
		require(t, err)
		if receipt.RunID != saved.WorkflowRunID || receipt.Code != "candidate_reserved" || len(f.s.domain.WorkflowRuns) != 2 {
			t.Fatal("original inbox reservation was rejected or replaced")
		}
	})
}

func mcAnswer(t *testing.T, s *Service, run *reconstructiontypes.RunView, note string) {
	t.Helper()
	answer := executionmodel.DecisionAnswer{DecisionID: run.Decision.ID, RunID: run.RunID, Note: note, AnsweredBy: "operator", ExpectedDecisionVersion: run.Decision.Version, ExpectedRunRevision: run.ControlRevision, ServiceInstanceID: s.State.Service.InstanceID}
	_, _, err := (newTestCommandService(s.commandStore, s.Config)).AnswerDecisionWithID(context.Background(), newID(), answer.DecisionID, answer.ExpectedDecisionVersion, DecisionAnswerKey(answer), answer.AnsweredBy, mustJSON(answer))
	require(t, err)
	require(t, s.processCommandsAndDecisions(context.Background()))
}

func TestMC3(t *testing.T) {
	f := newMCFixture(t)
	r, _ := f.start(t, "candidate-b")
	second := f.s.find(r.WorkflowRunID)
	for _, run := range []*reconstructiontypes.RunView{f.first, second} {
		mcStartTurn(t, f.s, run)
		mcFinishTurn(t, f.s, run)
	}
	sessionA, sessionB := f.s.executionRecord(f.first).SessionID, f.s.executionRecord(second).SessionID
	f.s.runs[0], f.s.runs[1] = f.s.runs[1], f.s.runs[0]
	blocked := f.s.executionRecord(second)
	questionID := second.Decision.ID
	blocked.Environment = &executionmodel.WorkerEnvironment{ProfileDigest: blocked.ConfigurationDigest, Runtime: blocked.Process.RuntimeID, LaunchCommand: "fixture", WriteRoots: blocked.PolicyResources.WriteRoots, Preparation: &executionmodel.WorkerPreparation{TurnID: newID(), Nonce: newID(), StartedAt: now(), Deadline: now().Add(time.Minute)}, Blocker: &executionmodel.WorkerHostBlocker{ID: newID(), Version: 1, TurnID: blocked.CurrentTurnID, Fact: executionbackend.HostFact{Reason: "host_wait", Source: "mc", Confidence: "confirmed"}}}
	require(t, f.s.commitWorker(second))
	mcAnswer(t, f.s, second, "refused candidate B")
	if f.s.workers[second.RunID] != nil || second.Decision.ID != questionID || second.Decision.Status != "pending" {
		t.Fatal("blocker replaced the question or queued business input")
	}
	mcAnswer(t, f.s, f.first, "only candidate A")
	blocked = f.s.executionRecord(second)
	blocked.Environment = nil
	require(t, f.s.commitWorker(second))
	require(t, f.s.applyDecisions(context.Background()))
	if f.s.workers[second.RunID] != nil {
		t.Fatal("clearing blocker supplemented refused input")
	}
	mcAnswer(t, f.s, second, "only candidate B")
	for _, run := range []*reconstructiontypes.RunView{f.first, second} {
		mcFinishTurn(t, f.s, run)
	}
	require(t, f.s.Close())
	restored, err := newNeutralService(f.c)
	require(t, err)
	defer restored.Close()
	if len(restored.runs) != 2 || restored.find(f.first.RunID).SessionID != sessionA || restored.find(second.RunID).SessionID != sessionB {
		t.Fatal("restart changed the precisely selected candidate session")
	}
	if f.s.executionRecord(f.first).SessionID != sessionA || f.s.executionRecord(second).SessionID != sessionB {
		t.Fatal("interleaved feedback crossed sessions")
	}
	for _, run := range []*reconstructiontypes.RunView{f.first, second} {
		e := f.s.executionRecord(run)
		body, err := os.ReadFile(filepath.Join(f.c.RunDir, "runs", run.RunID, "turns", e.CurrentCommandID, "prompt.txt"))
		require(t, err)
		wanted := "only candidate A"
		unwanted := "only candidate B"
		if run == second {
			wanted, unwanted = unwanted, wanted
		}
		if !strings.Contains(string(body), wanted) || strings.Contains(string(body), unwanted) {
			t.Fatal("feedback crossed fixed execution input")
		}
	}
}

func TestMC4(t *testing.T) {
	f := newMCFixture(t)
	r, _ := f.start(t, "candidate-b")
	second := f.s.find(r.WorkflowRunID)
	for i, run := range []*reconstructiontypes.RunView{f.first, second} {
		mcStartTurn(t, f.s, run)
		mcFinishTurn(t, f.s, run)
		cmd := exec.Command(os.Args[0], "mc-worker", deref(run.WorktreePath), f.provider, strconv.Itoa(81+i))
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("worker commit/push/create: %v: %s", err, output)
		}
		w := f.s.workflowRecord(run)
		w.PRCandidateURL = strings.TrimSpace(string(output))
		head, err := exec.Command("git", "-C", deref(run.WorktreePath), "rev-parse", "HEAD").Output()
		require(t, err)
		w.StepID, w.Status = executionmodel.StepAwaitMerge, "waiting_external"
		require(t, f.s.commitWorker(run))
		task := nativeTaskRecord(f.s.domain.Tasks, "8")
		require(t, f.s.pollWorkerFlowPR(context.Background(), run, task, w))
		if run.PR.URL == nil || run.PR.State != "open" {
			t.Fatal("worker PR was not bound by real provider query")
		}
		remote, err := exec.Command("git", "-C", f.c.Repository, "ls-remote", "origin", deref(run.WorktreeBranch)).Output()
		require(t, err)
		if strings.TrimSpace(string(head)) == deref(run.BaseCommit) || !strings.Contains(string(remote), strings.TrimSpace(string(head))) {
			t.Fatal("worker did not really commit and push its branch")
		}
	}
	w, e := f.s.workflowRecords(second)
	wrong := f.proof(t, f.first, 81, "open")
	if err := BindWorkerPR(w, e, wrong, wrong.URL, wrong.Repository, false); err == nil {
		t.Fatal("sibling PR accepted")
	}
	if len(f.s.domain.Deliveries) != 0 {
		t.Fatal("unmerged candidates delivered")
	}
}

func TestMC5(t *testing.T) {
	f := newMCFixture(t)
	r, _ := f.start(t, "candidate-b")
	second := f.s.find(r.WorkflowRunID)
	for i, run := range []*reconstructiontypes.RunView{f.first, second} {
		mcStartTurn(t, f.s, run)
		mcFinishTurn(t, f.s, run)
		f.bind(t, run, 81+i, "open")
	}
	f.proof(t, second, 82, "merged")
	task := nativeTaskRecord(f.s.domain.Tasks, "8")
	_, err := f.store.db.Exec(`CREATE TRIGGER mc_delivery_rollback BEFORE UPDATE ON tasks WHEN NEW.status='closed' BEGIN SELECT RAISE(ABORT,'mc rollback'); END`)
	require(t, err)
	_, err = f.commands.CompleteNativeTask(context.Background(), f.c, "8", task.Version, second.RunID, testPublicationClient(f.c))
	if err == nil {
		t.Fatal("required transaction failure was ignored")
	}
	snapshot, err := readCommandSnapshot(context.Background(), f.store, f.c.Identity(), "8")
	require(t, err)
	if len(snapshot.Graph.Deliveries) != 0 || snapshot.Graph.ByID["8"].Status != "in_progress" {
		t.Fatal("delivery/closed partially committed")
	}
	var normalizedResults int
	require(t, f.store.db.QueryRow(`SELECT COUNT(*) FROM task_results`).Scan(&normalizedResults))
	if normalizedResults != 0 {
		t.Fatal("failed close committed a normalized task result")
	}
	_, err = f.store.db.Exec(`DROP TRIGGER mc_delivery_rollback`)
	require(t, err)
	first, err := f.commands.CompleteNativeTask(context.Background(), f.c, "8", task.Version, second.RunID, testPublicationClient(f.c))
	require(t, err)
	replay, err := f.commands.CompleteNativeTask(context.Background(), f.c, "8", task.Version, second.RunID, testPublicationClient(f.c))
	require(t, err)
	t.Logf("unique delivery receipt: %s", mustJSON(first))
	if first.Delivery.SelectedWorkflowRunID != second.RunID || digestValue(first.Delivery) != digestValue(replay.Delivery) {
		t.Fatal("winner/replay changed")
	}
	require(t, f.s.refreshNativeSnapshot(context.Background()))
	ready, err := query.NewService(f.store, f.c.Identity()).ReadyNativeTasks(context.Background())
	require(t, err)
	if len(ready) != 1 || ready[0].TaskID != "9" || len(f.s.domain.Deliveries) != 1 {
		t.Fatal("unique delivery did not unlock successor once")
	}
	if f.first.ExecutionStatus == "stopped" || f.first.TaskStatus != "closed" || f.first.WorkflowStatus == "completed" || f.s.domain.Deliveries[0].SelectedWorkflowRunID == f.first.RunID {
		t.Fatal("loser falsely settled its resources")
	}
	// The winner's immutable delivery and subsequent physical stop are separate
	// facts, committed together with the exact original stop operation receipt.
	records := f.s.records
	ref, err := records.loadBinding(second.RunID, second.AttemptID)
	require(t, err)
	op := executionbackend.Operation{Version: executionbackend.ExecutionRecordVersion, ID: "stop-" + second.AttemptID, RunID: second.RunID, AttemptID: second.AttemptID, Backend: ref.Backend, Protocol: ref.Protocol, Target: ref.Target, Action: "stop", PayloadDigest: digestValue("mc-original-stop"), Status: executionbackend.OperationPending, StartedAt: now(), UpdatedAt: now(), Ref: &ref}
	require(t, f.s.commitExecutionOperation(second, op, ""))
	expected := recordFingerprint(op)
	op.Status = executionbackend.OperationInFlight
	require(t, f.s.commitExecutionOperation(second, op, expected))
	expected = recordFingerprint(op)
	op.Status = executionbackend.OperationConfirmed
	e := f.s.executionRecord(second)
	e.ResourceStopOperationID, e.ResourceStoppedAt = op.ID, ptr(now())
	require(t, f.s.commitExecutionOperation(second, op, expected))
	if second.ResourceStatus != "stopped" || digestValue(f.s.domain.Deliveries[0]) != digestValue(first.Delivery) {
		t.Fatal("resource stop changed delivery or disappeared from the candidate DTO")
	}
	e = f.s.executionRecord(second)
	e.ResourceStopOperationID = "unproved-stop"
	if err := f.s.commitWorker(second); err == nil {
		t.Fatal("resource stop without its original receipt was admitted")
	}
}

type mcCountPRQueries struct {
	executionbackend.ManagedProcessHost
	calls int
}

func (p *mcCountPRQueries) StartManaged(cmd *exec.Cmd) (executionbackend.StopHandle, error) {
	if len(cmd.Args) > 1 && cmd.Args[1] == "mc-provider" {
		p.calls++
	}
	return p.ManagedProcessHost.StartManaged(cmd)
}

type mcCountVerifier struct {
	TaskDeliveryVerifier
	urls []string
}

func (v *mcCountVerifier) Query(ctx context.Context, url string) (executionmodel.PRInfo, error) {
	v.urls = append(v.urls, url)
	return v.TaskDeliveryVerifier.Query(ctx, url)
}

func TestMC6(t *testing.T) {
	for _, mode := range []string{"multiple_merged", "unknown_sibling", "late_after_delivery"} {
		t.Run(mode, func(t *testing.T) {
			f := newMCFixture(t)
			r, _ := f.start(t, "candidate-b")
			second := f.s.find(r.WorkflowRunID)
			for i, run := range []*reconstructiontypes.RunView{f.first, second} {
				mcStartTurn(t, f.s, run)
				mcFinishTurn(t, f.s, run)
				f.bind(t, run, 81+i, "open")
			}
			f.proof(t, second, 82, "merged")
			if mode == "multiple_merged" {
				f.proof(t, f.first, 81, "merged")
			}
			if mode == "unknown_sibling" {
				require(t, os.Remove(filepath.Join(f.provider, "81.json")))
			}
			task := nativeTaskRecord(f.s.domain.Tasks, "8")
			verifier := &mcCountVerifier{TaskDeliveryVerifier: testPublicationClient(f.c)}
			if mode == "multiple_merged" {
				// Hold the real verified declaration while a sibling changes stage.
				captured := &resultCaptureStore{CommandStore: f.store}
				_, err := newTestCommandService(captured, f.c).CompleteNativeTask(context.Background(), f.c, "8", task.Version, second.RunID, verifier)
				if !errors.Is(err, errCapturedResult) {
					t.Fatal("selected merge did not reach atomic commit", err)
				}
				f.s.workflowRecord(f.first).Status = "blocked"
				f.s.refreshRun(f.first)
				require(t, f.s.commitWorker(f.first))
				_, err = f.store.CommitTaskResult(context.Background(), captured.commit)
				require(t, err)
			} else {
				_, err := f.commands.CompleteNativeTask(context.Background(), f.c, "8", task.Version, second.RunID, verifier)
				require(t, err)
			}
			if len(verifier.urls) != 1 || verifier.urls[0] != *second.PR.URL {
				t.Fatal("delivery queried a sibling PR", verifier.urls)
			}
			require(t, f.s.refreshNativeSnapshot(context.Background()))
			if nativeTaskRecord(f.s.domain.Tasks, "8").Status != "closed" || len(f.s.domain.Deliveries) != 1 || f.s.domain.Deliveries[0].SelectedWorkflowRunID != second.RunID {
				t.Fatal("verified candidate did not close task with one authoritative delivery")
			}
			ready, err := query.NewService(f.store, f.c.Identity()).ReadyNativeTasks(context.Background())
			require(t, err)
			if len(ready) != 1 || ready[0].TaskID != "9" {
				t.Fatal("close did not release the successor")
			}
			original := digestValue(f.s.domain.Deliveries)
			if mode == "late_after_delivery" {
				f.proof(t, f.first, 81, "merged")
			}
			before := digestValue([]executionmodel.PRState{f.first.PR, second.PR})
			counter := &mcCountPRQueries{ManagedProcessHost: f.s.runtime.ProcessHost}
			f.s.runtime.ProcessHost = counter
			require(t, f.s.pollNativePRs(context.Background(), ""))
			require(t, f.s.pollWorkerFlowPR(context.Background(), f.first, nativeTaskRecord(f.s.domain.Tasks, "8"), f.s.workflowRecord(f.first)))
			if counter.calls != 0 || digestValue([]executionmodel.PRState{f.first.PR, second.PR}) != before || digestValue(f.s.domain.Deliveries) != original {
				t.Fatal("closed task still observed PR state or changed its delivery", counter.calls)
			}
			require(t, f.s.Close())
			restored, err := newNeutralService(f.c)
			require(t, err)
			defer restored.Close()
			restoredCounter := &mcCountPRQueries{ManagedProcessHost: restored.runtime.ProcessHost}
			restored.runtime.ProcessHost = restoredCounter
			require(t, restored.pollNativePRs(context.Background(), ""))
			if restoredCounter.calls != 0 || digestValue(restored.domain.Deliveries) != original {
				t.Fatal("restart resumed PR monitoring for a closed task")
			}
		})
	}
}

func TestMC7(t *testing.T) {
	t.Run("discard_full_history_and_candidate_set", func(t *testing.T) {
		f := newMCFixture(t)
		r, _ := f.start(t, "candidate-b")
		second := f.s.find(r.WorkflowRunID)
		for i, run := range []*reconstructiontypes.RunView{f.first, second} {
			mcStartTurn(t, f.s, run)
			mcFinishTurn(t, f.s, run)
			if i == 0 {
				// Seed retained audit facts; current execution never replaces a PR.
				history := f.proof(t, run, 80, "closed_unmerged")
				f.s.workflowRecord(run).PRHistory = []string{history.URL}
			}
			f.bind(t, run, 81+i, "open")
		}
		f.bind(t, f.first, 81, "closed_unmerged")
		other := f.proof(t, f.first, 83, "open")
		w, e := f.s.workflowRecords(f.first)
		if err := BindWorkerPR(w, e, other, other.URL, other.Repository, false); err == nil {
			t.Fatal("closed PR allowed an implicit replacement")
		}
		f.bind(t, f.first, 81, "open")
		f.proof(t, second, 82, "merged")
		rejected, err := f.commands.DiscardNativeTask(context.Background(), f.c, "8", newID(), 2, "all candidates", "operator")
		require(t, err)
		if rejected.Status != "rejected" || rejected.Code != "already_merged" {
			t.Fatal("whole-task discard missed a sibling merged PR")
		}
		f.proof(t, second, 82, "open")
		accepted, err := f.commands.DiscardNativeTask(context.Background(), f.c, "8", newID(), 2, "all candidates", "operator")
		require(t, err)
		if accepted.Status != "discarded" || len(accepted.StopPending) != 2 || !contains(accepted.StopPending, "execution_stop_manual:"+f.first.ExecutionID) {
			t.Fatal("discard lost candidate/history PRs or live idle worker resources")
		}
		t.Logf("complete discard receipt: %s", mustJSON(accepted))
	})
}

func TestMC9(t *testing.T) {
	f := newMCFixture(t)
	var receipt executionmodel.TaskClaimReceipt
	var options TaskClaimOptions
	for i := 1; i < 5; i++ {
		receipt, options = f.start(t, "candidate-b")
	}
	c := f.c
	setNeutralProfile(t, &c, neutralSettings{Executable: os.Args[0], Model: "candidate-b", ReasoningEffort: "high"})
	replay, err := f.commands.StartNativeCandidate(context.Background(), c, options)
	require(t, err)
	if replay.WorkflowRunID != receipt.WorkflowRunID {
		t.Fatal("limit replaced original request")
	}
	bad := options
	bad.RequestID = newID()
	if _, err := f.commands.StartNativeCandidate(context.Background(), c, bad); !isNativeErrorCode(err, "candidate_limit_reached") {
		t.Fatalf("sixth serial start: %v", err)
	}
	var reserved int
	require(t, f.store.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name='one_active_workflow_per_task' OR name LIKE '%candidate%slot%'`).Scan(&reserved))
	if reserved != 0 {
		t.Fatal("quantity became a hard resource slot")
	}
	// No parallel overshoot is required. Acceptance is not reversed or repaired
	// merely because an externally sampled participation count exceeds five.
	var exported reconstructiontypes.Runtime = f.s.State
	if len(exported.Runs) != 5 {
		t.Fatal("history was pruned to enforce quantity")
	}
}

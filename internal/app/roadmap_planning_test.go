package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	process "github.com/erranliu/SteerLine/internal/execution/process"
	"github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/product/httpapi"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	roadmap "github.com/erranliu/SteerLine/internal/roadmap/model"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

func planningGitFixture(t *testing.T) (*planningPort, roadmap.PlanningTask, string) {
	t.Helper()
	ctx := context.Background()
	config, _ := nativeTaskFixture(t)
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	pmGit(t, root, "init", "--bare", "--initial-branch=main", remote)
	pmGit(t, config.Repository, "branch", "-M", "main")
	pmGit(t, config.Repository, "config", "url."+filepath.ToSlash(remote)+".insteadOf", "https://github.com/owner/repo.git")
	pmGit(t, config.Repository, "config", "core.autocrlf", "false")
	writePlanningFixture(t, config.Repository, "shared.md", "base\n")
	pmGit(t, config.Repository, "add", "shared.md")
	pmGit(t, config.Repository, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "base")
	pmGit(t, config.Repository, "push", "origin", "main")
	base := pmGit(t, config.Repository, "rev-parse", "HEAD")
	worker := filepath.Join(root, "worker")
	pmGit(t, root, "-c", "core.autocrlf=false", "clone", remote, worker)
	pmGit(t, worker, "config", "core.autocrlf", "false")
	pmGit(t, worker, "checkout", "-b", "codex/planning")
	project, err := OpenNativeTaskProject(ctx, &config)
	require(t, err)
	require(t, project.Close())
	store, err := OpenNativeTaskCommandStore(ctx, &config)
	require(t, err)
	t.Cleanup(func() { _ = store.Close() })
	maintenance := newProjectMaintenance(config, store)
	port := &planningPort{
		roadmapPort:                &roadmapPort{config: config, store: store, materials: maintenance, mergeHistory: maintenance},
		store:                      store,
		readAdmissionMapping:       store.RoadmapMapping,
		refreshBaselineWithRewrite: maintenance.refreshBaselineWithRewrite,
	}
	task := roadmap.PlanningTask{ID: strings.Repeat("a", 32), Project: config.Identity(), Revision: 1,
		Title: "Planning", Path: roadmap.Root + "new.md", Base: base, Material: filepath.Join(root, "rules.md"),
		Run: &runnercontract.Run{RunID: "run", WorkspaceOriginRunID: "run", WorkspaceIdentity: "workspace",
			WorkspaceWrite: &runnercontract.WorkspaceWrite{Owner: "steerline", LeaseID: "lease"},
			Execution:      runnercontract.Execution{Workspace: worker, Branch: "codex/planning", SessionID: "session"}}}
	return port, task, remote
}

func writePlanningFixture(t *testing.T, root, path, body string) {
	t.Helper()
	file := filepath.Join(root, filepath.FromSlash(path))
	require(t, os.MkdirAll(filepath.Dir(file), 0700))
	require(t, os.WriteFile(file, []byte(body), 0600))
}

func TestRoadmapPlanningRebaseAndConflictContinuation(t *testing.T) {
	ctx := context.Background()
	port, task, _ := planningGitFixture(t)
	worker := task.Run.Execution.Workspace
	writePlanningFixture(t, worker, task.Path, "# Roadmap draft\n")
	writePlanningFixture(t, worker, "shared.md", "staged change\n")
	pmGit(t, worker, "add", "shared.md")
	writePlanningFixture(t, worker, "shared.md", "worktree change\n")
	writePlanningFixture(t, port.config.Repository, "shared.md", "main change\n")
	pmGit(t, port.config.Repository, "add", "shared.md")
	pmGit(t, port.config.Repository, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "target")
	target := pmGit(t, port.config.Repository, "rev-parse", "HEAD")
	task.Operation = &roadmap.PlanningOperation{Kind: "rebase", Stage: "leased", Target: target, Lease: "lease"}
	require(t, port.store.SavePlanning(ctx, task.Project, "", nil, task))
	require(t, port.applyPlanningLease(ctx, &task))
	if task.Conflict == nil || len(task.Conflict.Files) != 1 || task.Conflict.Files[0] != "shared.md" || task.Operation.Command != "workspace release" {
		t.Fatalf("rebase did not stop and prepare original lease release: %+v", task)
	}
	writePlanningFixture(t, worker, "shared.md", "resolved change\n")
	pmGit(t, worker, "add", "shared.md")
	require(t, port.checkPlanningRebase(ctx, &task, target))
	if task.Conflict == nil || len(task.Conflict.Files) != 0 || task.Base == target {
		t.Fatal("staged conflict was mistaken for completed rebase")
	}
	_, err := port.planningGit(ctx, worker, nil, nil, "rebase", "--continue")
	require(t, err)
	admin := pmGit(t, worker, "rev-parse", "--absolute-git-dir")
	partial := filepath.Join(admin, "rebase-merge")
	require(t, os.Mkdir(partial, 0700))
	if err := port.checkPlanningRebase(ctx, &task, target); err == nil || task.Conflict == nil || task.Base == target {
		t.Fatal("incomplete Rebase metadata was mistaken for a completed Rebase")
	}
	require(t, os.Remove(partial))
	require(t, port.checkPlanningRebase(ctx, &task, target))
	if task.Conflict != nil || task.Base != target || task.Body != "# Roadmap draft\n" {
		t.Fatalf("original worktree did not retain the draft: %+v", task)
	}
	writePlanningFixture(t, worker, "shared.md", "staged after first rebase\n")
	pmGit(t, worker, "add", "shared.md")
	writePlanningFixture(t, worker, "shared.md", "working after first rebase\n")
	writePlanningFixture(t, worker, "new-local.md", "untracked after first rebase\n")
	writePlanningFixture(t, port.config.Repository, "shared.md", "second main change\n")
	pmGit(t, port.config.Repository, "add", "shared.md")
	pmGit(t, port.config.Repository, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "second target")
	secondTarget := pmGit(t, port.config.Repository, "rev-parse", "HEAD")
	task.Operation = &roadmap.PlanningOperation{Kind: "rebase", Stage: "leased", Target: secondTarget, Lease: "lease"}
	require(t, port.applyPlanningLease(ctx, &task))
	if task.Conflict == nil {
		t.Fatal("expected original conflict before abort")
	}
	task.Operation = &roadmap.PlanningOperation{Kind: "abort", Stage: "leased", Target: secondTarget, Lease: "lease"}
	require(t, port.applyPlanningLease(ctx, &task))
	working, err := os.ReadFile(filepath.Join(worker, "shared.md"))
	require(t, err)
	untracked, err := os.ReadFile(filepath.Join(worker, "new-local.md"))
	require(t, err)
	if task.Conflict != nil || task.Base != target || pmGit(t, worker, "show", ":shared.md") != "staged after first rebase" || string(working) != "working after first rebase\n" || string(untracked) != "untracked after first rebase\n" {
		t.Fatalf("abort did not restore the staged, working and untracked versions: %s", task.Diagnostic)
	}
	writePlanningFixture(t, admin, "refs/steerline/planning/"+task.ID+"/checkpoint.lock", "locked\n")
	task.Operation = &roadmap.PlanningOperation{Kind: "rebase", Stage: "leased", Target: secondTarget, Lease: "lease"}
	if err := port.applyPlanningLease(ctx, &task); err == nil || task.Operation.Stage != "git_running" || task.Operation.Command != "" || task.Operation.Lease != "lease" {
		t.Fatal("an unverified Git write prepared a lease release")
	}
	if err := port.applyPlanningLease(ctx, &task); err == nil || task.Operation.Command != "" {
		t.Fatal("recovery returned a lease without proving the Rebase result")
	}
}

func TestRoadmapPlanningSelectedPublicationAndHandoff(t *testing.T) {
	t.Run("lost create receipt and original PR update", testPlanningPublicationReplay)
	t.Run("unknown push after target rewrite back to reviewed base", testPlanningPushAfterTargetRewrite)
	ctx := context.Background()
	port, task, remote := planningGitFixture(t)
	worker := task.Run.Execution.Workspace
	writePlanningFixture(t, worker, task.Path, "# Roadmap draft\n")
	writePlanningFixture(t, worker, "selected.md", "approved\n")
	writePlanningFixture(t, worker, "excluded.md", "keep local\n")
	writePlanningFixture(t, worker, "unavailable.md", strings.Repeat("x", roadmap.MaxBytes+1))
	task.Operation = &roadmap.PlanningOperation{Kind: "review"}
	require(t, port.capturePlanningReview(ctx, &task))
	if changes := task.Review.Changes; len(changes) != 4 || changes[3].Path != "unavailable.md" || changes[3].Unavailable == "" {
		t.Fatal("unpublishable extra file was not identified in the review")
	}
	task.Operation = &roadmap.PlanningOperation{Kind: "publish", Digest: task.Review.Digest, Selected: []string{task.Path, "unavailable.md"}}
	if err := port.capturePlanningReview(ctx, &task); err == nil || task.Publication != nil {
		t.Fatal("unavailable file reached publication")
	}
	task.Operation = &roadmap.PlanningOperation{Kind: "publish", Digest: task.Review.Digest, Selected: []string{task.Path, "selected.md"}}
	require(t, port.capturePlanningReview(ctx, &task))
	version := &task.Publication.Versions[0]
	head, err := port.preparePlanningCommit(ctx, &task, version)
	require(t, err)
	version.Head = head
	commitFiles := pmGit(t, port.config.Repository, "diff-tree", "--no-commit-id", "--name-only", "-r", head)
	if commitFiles != task.Path+"\nselected.md" {
		t.Fatalf("unconfirmed files reached publication: %s", commitFiles)
	}
	task.Publication.Versions = append(task.Publication.Versions, roadmap.PlanningVersion{Base: task.Base, Head: strings.Repeat("9", 40)})
	require(t, port.store.SavePlanning(ctx, task.Project, "", nil, task))
	pmGit(t, port.config.Repository, "push", remote, head+":refs/heads/main")
	fact := roadmapPR{Merged: true, MergeCommit: head}
	fact.Base.Ref, fact.Head.Ref, fact.Head.SHA = "main", task.Publication.Branch, head
	changes := task.Publication.Versions[0].Changes
	approved := changes[1].After
	changes[1].After = []byte("different approved content\n")
	task.Body = "# Previous verified preview\n"
	if err := port.adoptPlanningMerge(ctx, &task, fact); err == nil || task.Body != "# Previous verified preview\n" || task.Merged != "" {
		t.Fatal("a partial match updated the verified preview or accepted the merge")
	}
	changes[1].After = approved
	stale := task
	stale.Revision++
	if err := port.adoptPlanningMerge(ctx, &stale, fact); err == nil || stale.Merged != "" || stale.Body != task.Body {
		t.Fatal("a stale database revision adopted an uncommitted merge observation")
	}
	require(t, port.adoptPlanningMerge(ctx, &task, fact))
	task.Operation = nil
	if err := port.handoffPlanning(ctx, &task); err == nil || task.SourceID != "" {
		t.Fatal("handed off while local target was behind")
	}
	pmGit(t, port.config.Repository, "merge", "--ff-only", head)
	writePlanningFixture(t, port.config.Repository, task.Path, "local-only\n")
	if err := port.handoffPlanning(ctx, &task); err == nil || task.SourceID != "" {
		t.Fatal("handed off a locally modified draft")
	}
	pmGit(t, port.config.Repository, "restore", "--", task.Path)
	pmGit(t, remote, "update-ref", "refs/heads/main", task.Base, head)
	if err := port.handoffPlanning(ctx, &task); err == nil || task.SourceID != "" {
		t.Fatal("a cached observation allowed handoff after the remote dropped the merge")
	}
	pmGit(t, remote, "update-ref", "refs/heads/main", head, task.Base)
	offline := errors.New("original Runner offline")
	port.runner = planningLeaseFixture{inspectErr: offline}
	if err := port.advance(ctx, &task); !errors.Is(err, offline) {
		t.Fatalf("offline Runner proof was lost: %v", err)
	}
	if task.SourceID != roadmap.SourceID(task.Project, task.Path) || task.Status != "handed_off" || task.Operation != nil || task.Run.StopStatus == "stopped" {
		t.Fatalf("missing stable source handoff: %+v", task)
	}
}

func testPlanningPublicationReplay(t *testing.T) {
	ctx := context.Background()
	port, task, remote := planningGitFixture(t)
	worker := task.Run.Execution.Workspace
	writePlanningFixture(t, worker, task.Path, "# First approved draft\n")
	writePlanningFixture(t, worker, "keep-local.txt", "never selected\n")
	var fact *roadmapPR
	var server *httptest.Server
	creates := 0
	var providerMu sync.Mutex
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerMu.Lock()
		defer providerMu.Unlock()
		switch {
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/pulls"):
			items := []*roadmapPR{}
			if fact != nil {
				fact.Head.SHA = pmGit(t, remote, "rev-parse", "refs/heads/"+fact.Head.Ref)
				items = append(items, fact)
			}
			_ = json.NewEncoder(w).Encode(items)
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/pulls"):
			creates++
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			fact = &roadmapPR{Number: 1, URL: server.URL + "/owner/repo/pulls/1", State: "open"}
			fact.Head.Ref, fact.Head.Repo.FullName, fact.Base.Ref = body["head"], "owner/repo", body["base"]
			http.Error(w, "original creation receipt lost", 500)
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/pulls/1") && fact != nil:
			fact.Head.SHA = pmGit(t, remote, "rev-parse", "refs/heads/"+fact.Head.Ref)
			_ = json.NewEncoder(w).Encode(fact)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	port.config.PR = executionmodel.PRProviderConfig{Kind: "gitea", BaseURL: server.URL, Repository: "owner/repo"}
	confirm := func() {
		task.Operation = &roadmap.PlanningOperation{Kind: "review"}
		require(t, port.capturePlanningReview(ctx, &task))
		task.Operation = &roadmap.PlanningOperation{Kind: "publish", Stage: "publish_ready", Digest: task.Review.Digest, Selected: []string{task.Path}}
		require(t, port.capturePlanningReview(ctx, &task))
	}
	confirm()
	// A restart between saving the candidate head and sending the push must
	// continue the same protected publication when the old ref is still current.
	version := &task.Publication.Versions[0]
	head, err := port.preparePlanningCommit(ctx, &task, version)
	require(t, err)
	version.Head = head
	task.Operation.Stage = "push_unknown"
	require(t, port.store.SavePlanning(ctx, task.Project, "", nil, task))
	require(t, port.publishPlanning(ctx, &task))
	if err := port.publishPlanning(ctx, &task); err == nil || task.Operation.Stage != "pr_unknown" {
		t.Fatal("lost provider reply must preserve the original PR intent")
	}
	require(t, port.publishPlanning(ctx, &task))
	url := task.Publication.URL
	writePlanningFixture(t, worker, task.Path, "# Second approved draft\n")
	confirm()
	require(t, port.publishPlanning(ctx, &task))
	require(t, port.publishPlanning(ctx, &task))
	providerMu.Lock()
	count := creates
	providerMu.Unlock()
	if count != 1 || task.Publication.URL != url || task.Publication.Head == head || len(task.Publication.Versions) != 2 {
		t.Fatal("reviewed update did not reuse the original PR")
	}
}

func testPlanningPushAfterTargetRewrite(t *testing.T) {
	ctx := context.Background()
	port, task, remote := planningGitFixture(t)
	writePlanningFixture(t, task.Run.Execution.Workspace, task.Path, "# Approved draft\n")
	task.Operation = &roadmap.PlanningOperation{Kind: "review"}
	require(t, port.capturePlanningReview(ctx, &task))
	task.Operation = &roadmap.PlanningOperation{Kind: "publish", Digest: task.Review.Digest, Selected: []string{task.Path}}
	require(t, port.capturePlanningReview(ctx, &task))
	version := &task.Publication.Versions[0]
	head, err := port.preparePlanningCommit(ctx, &task, version)
	require(t, err)
	version.Head = head
	task.Operation.Stage = "push_unknown"
	require(t, port.store.SavePlanning(ctx, task.Project, "", nil, task))

	pmGit(t, port.config.Repository, "push", remote, head+":refs/heads/main")
	target, err := port.mergeHistory.refreshBaseline(ctx)
	require(t, err)
	if target != head {
		t.Fatalf("expected to observe the advanced target, got %s", target)
	}
	pmGit(t, remote, "update-ref", "refs/heads/main", version.Base, head)
	if err := port.publishPlanning(ctx, &task); err == nil || task.Operation != nil || task.Review != nil {
		t.Fatalf("rewrite back to the reviewed base must invalidate the push and review: %v", err)
	}
	ref := "refs/heads/" + task.Publication.Branch
	if observed := pmGit(t, port.config.Repository, "ls-remote", "--refs", "origin", ref); observed != "" {
		t.Fatalf("recovery pushed to %s after the rewrite: %s", ref, observed)
	}
}

type planningLeaseFixture struct {
	run        *runnercontract.Run
	command    *runnercontract.Command
	stale      bool
	inspectErr error
	requestID  string
}

func (f planningLeaseFixture) BuildPin() runnercontract.BuildPin { return runnercontract.BuildPin{} }
func (f planningLeaseFixture) Inspect(_ context.Context, selector, id string) (runnercontract.Inspection, error) {
	if selector == "request-id" && f.requestID != "" && id != f.requestID {
		return runnercontract.Inspection{}, errors.New("unexpected recovery request ID")
	}
	if f.inspectErr != nil {
		return runnercontract.Inspection{}, f.inspectErr
	}
	if f.stale && selector == "request-id" {
		return runnercontract.Inspection{}, &runnerclient.Failure{Code: "request_not_found"}
	}
	return runnercontract.Inspection{Run: f.run, Command: f.command}, nil
}
func (f planningLeaseFixture) Call(context.Context, string, []string, *runnercontract.Request) (runnerclient.Response, error) {
	if f.stale {
		return runnerclient.Response{}, &runnerclient.Failure{Code: "stale_revision", Exit: 5}
	}
	return runnerclient.Response{}, errors.New("recovery must use the original receipt")
}

type planningStoreFixture struct {
	planningStore
	task    []byte
	receipt []byte
	saves   int
}

func (s *planningStoreFixture) PlanningTask(context.Context, string, string) (roadmap.PlanningTask, error) {
	var task roadmap.PlanningTask
	err := json.Unmarshal(s.task, &task)
	return task, err
}

func (s *planningStoreFixture) PlanningReceipt(context.Context, string, string) ([]byte, error) {
	return s.receipt, nil
}

func (s *planningStoreFixture) SavePlanning(_ context.Context, _ string, request string, receipt []byte, tasks ...roadmap.PlanningTask) error {
	if len(tasks) != 1 || request != "" && s.receipt != nil {
		return errors.New("unexpected planning write")
	}
	s.task = mustJSON(tasks[0])
	if request != "" {
		s.receipt = receipt
	}
	s.saves++
	return nil
}

type planningReplayAuthority struct{ heldRuntimePort }

type planningReplayTaskStore struct{ taskStorage }

func (planningReplayTaskStore) LookupCommand(context.Context, string, string) (storageports.CommandRecord, bool, error) {
	return storageports.CommandRecord{}, false, nil
}

func (*planningReplayAuthority) Command(context.Context, string) (api.CommandReceipt, error) {
	return api.CommandReceipt{}, &api.Error{Kind: api.UnknownRecord, Message: "command not found"}
}

func (*planningReplayAuthority) Request(context.Context, string) (*api.ControlRequest, error) {
	return nil, os.ErrNotExist
}

func testPlanningStoreReplayAndRecovery(t *testing.T) {
	ctx := context.Background()
	authority := &planningReplayAuthority{}
	config := Config{identity: authority.Project().Identity}
	id := strings.Repeat("a", 32)
	run := &runnercontract.Run{
		RunID: "run", Revision: 5, Workflow: runnercontract.Workflow{ID: "conversation", Version: 2},
		Correlation: map[string]string{"steerline_planning": id}, WorkspaceWrite: &runnercontract.WorkspaceWrite{Owner: "runner"},
	}
	run.Controls.CanMessage = true
	store := &planningStoreFixture{task: mustJSON(roadmap.PlanningTask{ID: id, Project: config.Identity(), Revision: 1, Run: run})}
	newRuntime := func(runner planningRunner) *runtimeProjectPort {
		server, err := httpapi.NewProject(httpapi.Ports{Query: authority, Commands: authority, Authority: authority, NativeViewer: authority},
			httpapi.Options{RunDir: t.TempDir(), ProjectIdentity: config.Identity(), InstanceID: "instance"})
		require(t, err)
		require(t, server.SetReady("instance", true, ""))
		roadmaps := &roadmapPort{config: config}
		roadmaps.planning = &planningPort{roadmapPort: roadmaps, store: store, runner: runner}
		return &runtimeProjectPort{Server: server, config: Config{identity: config.Identity(), runtimeRoadmaps: roadmaps}, store: planningReplayTaskStore{}}
	}
	in := httpcontract.CommandEnvelope{RequestID: strings.Repeat("b", 32), ProjectID: httpcontract.ProjectID(config.Identity()),
		InstanceID: "instance", Action: planningAction + "message", TargetID: id, ExpectedVersion: 1, Payload: []byte(`{"text":"original"}`)}
	first := newRuntime(nil)
	receipt, err := first.Server.CommandRoadmap(ctx, in, first)
	require(t, err)
	task, err := store.PlanningTask(ctx, config.Identity(), id)
	require(t, err)
	if receipt.Status != "applied" || store.saves != 1 || task.Operation == nil || task.Operation.Request == nil {
		t.Fatalf("command did not save the original operation and receipt: %+v", receipt)
	}

	restarted := newRuntime(planningLeaseFixture{run: run, requestID: task.Operation.Request.RequestID,
		command: &runnercontract.Command{Command: "message", ActionStatus: "completed"}})
	restarted.config.runtimeRoadmaps.planning.tick(ctx, task)
	recovered, err := store.PlanningTask(ctx, config.Identity(), id)
	require(t, err)
	if recovered.Operation != nil || recovered.Revision != task.Revision+1 || store.saves != 2 {
		t.Fatalf("restart did not settle the original operation: %+v", recovered)
	}
	replayed, err := restarted.Server.CommandRoadmap(ctx, in, restarted)
	require(t, err)
	if string(mustJSON(replayed)) != string(mustJSON(receipt)) || store.saves != 2 {
		t.Fatal("replay changed the original receipt or resubmitted the command")
	}
}

func TestRoadmapPlanningRecoveryRejectsHistoricalLease(t *testing.T) {
	t.Run("store port replays receipt and recovers original operation", testPlanningStoreReplayAndRecovery)
	task := roadmap.PlanningTask{ID: "plan", Operation: &roadmap.PlanningOperation{Command: "workspace acquire", Kind: "rebase", Request: &runnercontract.Request{RequestID: "original"}}}
	run := &runnercontract.Run{RunID: "run", Correlation: map[string]string{"steerline_planning": task.ID}, Workflow: runnercontract.Workflow{ID: "conversation", Version: 2},
		WorkspaceWrite: &runnercontract.WorkspaceWrite{Owner: "steerline", LeaseID: "another-lease"}}
	receipt := &runnercontract.Command{Command: "workspace acquire", CommandID: "original-lease", ActionStatus: "completed"}
	port := planningPort{runner: planningLeaseFixture{run: run, command: receipt}}
	if _, err := port.runPlanningControl(context.Background(), &task); err == nil || task.Operation.Lease != "" {
		t.Fatal("historical acquire incorrectly became the current lease")
	}
	run.WorkspaceWrite.LeaseID = receipt.CommandID
	if _, err := port.runPlanningControl(context.Background(), &task); err != nil || task.Operation.Stage != "leased" || task.Operation.Lease != receipt.CommandID {
		t.Fatalf("could not resume the actual current lease: %v", err)
	}
	writer, err := process.CurrentIdentity()
	require(t, err)
	task.Operation.Stage = "git_running"
	task.Operation.Writer, err = json.Marshal(writer)
	require(t, err)
	if err = port.applyPlanningLease(context.Background(), &task); err == nil || task.Operation.Stage != "git_running" {
		t.Fatal("recovery released a lease while the original writer remained alive")
	}
	t.Run("retry confirmed stale lease control", func(t *testing.T) {
		ctx := context.Background()
		latest := *run
		latest.Revision = 12
		latest.WorkspaceWrite = &runnercontract.WorkspaceWrite{Owner: "runner", CanAcquire: true}
		owner := &planningPort{runner: planningLeaseFixture{run: &latest, stale: true}}
		state := roadmap.PlanningTask{ID: task.ID, Run: run, Operation: &roadmap.PlanningOperation{Kind: "review"}}
		planControl(state.Operation, "workspace acquire", 10, nil)
		original := state.Operation.Request.RequestID
		_, err := owner.runPlanningControl(ctx, &state)
		require(t, err)
		if state.Operation.Request.RequestID == original || string(state.Operation.Request.Input) != `{"expected_revision":12}` {
			t.Fatal("retry did not bind the newly observed eligible revision")
		}
		state.Operation.Lease = "lease"
		planControl(state.Operation, "workspace release", 10, map[string]any{"lease_id": "lease"})
		original = state.Operation.Request.RequestID
		latest.WorkspaceWrite = &runnercontract.WorkspaceWrite{Owner: "steerline", LeaseID: "another-lease"}
		if _, err = owner.runPlanningControl(ctx, &state); err == nil || state.Operation.Request.RequestID != original {
			t.Fatal("retry tried to return a different lease")
		}
		latest.WorkspaceWrite.LeaseID = "lease"
		_, err = owner.runPlanningControl(ctx, &state)
		require(t, err)
		if state.Operation.Request.RequestID == original || string(state.Operation.Request.Input) != `{"expected_revision":12,"lease_id":"lease"}` {
			t.Fatal("retry did not preserve the original held lease")
		}
	})
	t.Run("persisted acquire and release receipts", func(t *testing.T) {
		ctx := context.Background()
		owner, task, _ := planningGitFixture(t)
		cli := startupFixtureCLI(t)
		task.Build = cli.BuildPin()
		writePlanningFixture(t, task.Run.Execution.Workspace, task.Path, "# Persisted draft\n")
		task.Run.Workflow = runnercontract.Workflow{ID: "conversation", Version: 2}
		task.Run.Correlation = map[string]string{"steerline_planning": task.ID}
		task.Run.InputDigest = strings.Repeat("a", 64)
		task.Run.Revision = 10
		task.Operation = &roadmap.PlanningOperation{Kind: "review", Stage: "runner", Command: "workspace acquire", Request: &runnercontract.Request{RequestID: "original-acquire"}}
		require(t, owner.store.SavePlanning(ctx, task.Project, "", nil, task))

		// A new port has no in-memory acquire response. Recover the original
		// receipt and verify the current owner before capturing the review.
		acquire := planningLeaseFixture{run: task.Run, command: &runnercontract.Command{Command: "workspace acquire", CommandID: "lease", ActionStatus: "completed"}}
		reopened := &planningPort{roadmapPort: owner.roadmapPort, store: owner.store, runner: acquire}
		state, err := owner.store.PlanningTask(ctx, task.Project, task.ID)
		require(t, err)
		_, err = reopened.runPlanningControl(ctx, &state)
		require(t, err)
		require(t, reopened.save(ctx, &state))
		require(t, reopened.applyPlanningLease(ctx, &state))
		require(t, reopened.save(ctx, &state))
		if state.Operation.Command != "workspace release" || !strings.Contains(string(state.Operation.Request.Input), `"lease_id":"lease"`) {
			t.Fatal("release did not bind the original acquired lease")
		}

		// The release response can also be lost after its effect. A second
		// port observes the completed original request without sending again.
		released := *task.Run
		released.WorkspaceWrite = &runnercontract.WorkspaceWrite{Owner: "runner", CanAcquire: true}
		released.Controls.CanStop = true
		state.StopRequested = true
		require(t, reopened.save(ctx, &state))
		startup := func(run *runnercontract.Run) []runnercontract.PendingStop {
			t.Helper()
			require(t, os.WriteFile(filepath.Join(cli.StateDir, "idle-inspections-fixture.json"), mustJSON(map[string]runnercontract.Inspection{run.RunID: {Run: run}}), 0600))
			stops, err := preparePlanningStops(ctx, &owner.config, owner.store, &cli)
			require(t, err)
			return stops
		}
		if len(startup(task.Run)) != 0 {
			t.Fatal("startup stop was queued while the original lease remained held")
		}
		proof := runnerclient.StoppedVerification{StateDir: cli.StateDir, ResourcesSettled: true,
			Runs: []runnerclient.StoppedRun{{RunID: task.Run.RunID, InputDigest: task.Run.InputDigest, ResourcesSettled: true}}}
		if err := verifyPlanningLifecycle(ctx, []roadmap.PlanningTask{state}, proof, nil); err == nil {
			t.Fatal("offline resource proof alone discarded the held lease")
		}
		require(t, verifyPlanningLifecycle(ctx, []roadmap.PlanningTask{state}, proof, &task.Build))
		wrongBuild := task.Build
		wrongBuild.SHA256 = strings.Repeat("f", 64)
		if err := verifyPlanningLifecycle(ctx, []roadmap.PlanningTask{state}, proof, &wrongBuild); err == nil {
			t.Fatal("recovery accepted a different Runner build while the lease remained held")
		}
		if len(startup(&released)) != 0 {
			t.Fatal("startup stop was queued before reconciling the original lease release")
		}
		finish := &planningPort{roadmapPort: owner.roadmapPort, store: owner.store, runner: planningLeaseFixture{run: &released, command: &runnercontract.Command{Command: "workspace release", ActionStatus: "completed"}}}
		state, err = owner.store.PlanningTask(ctx, task.Project, task.ID)
		require(t, err)
		_, err = finish.runPlanningControl(ctx, &state)
		require(t, err)
		if state.Operation != nil || state.Review == nil || len(state.Review.Changes) != 1 || state.Review.Changes[0].Path != task.Path {
			t.Fatalf("recovered release did not settle the original captured review: %+v, diagnostic %q", state.Review, state.Diagnostic)
		}
		require(t, finish.save(ctx, &state))
		stops := startup(state.Run)
		if len(stops) != 1 || stops[0].RunID != task.Run.RunID || string(stops[0].Request.Input) != `{"expected_revision":10}` {
			t.Fatal("verified release did not allow the durable startup stop")
		}
		if again := startup(state.Run); len(again) != 1 || again[0].Request.RequestID != stops[0].Request.RequestID {
			t.Fatal("startup stop did not reuse its original request")
		}
		stopped := released
		stopped.StopStatus = "stopped"
		state.Run = &stopped
		require(t, verifyPlanningLifecycle(ctx, []roadmap.PlanningTask{state}, proof, nil))
		proof.Runs[0].InputDigest = "another-input"
		if err := verifyPlanningLifecycle(ctx, []roadmap.PlanningTask{state}, proof, nil); err == nil {
			t.Fatal("offline verification accepted another input's stop proof")
		}
	})
}

func TestRoadmapPlanningBatchFreezesOneTarget(t *testing.T) {
	if !validPlanningFileName("发布规划.md") || validPlanningFileName("../outside.md") || validPlanningFileName("bad:name.md") {
		t.Fatal("planning draft filename must accept safe UTF-8 and reject escaping paths")
	}
	ctx := context.Background()
	port, first, _ := planningGitFixture(t)
	first.Run.WorkspaceWrite = &runnercontract.WorkspaceWrite{Owner: "runner", CanAcquire: true}
	second, busy := first, first
	second.ID, second.Path = strings.Repeat("b", 32), roadmap.Root+"second.md"
	busy.ID, busy.Path = strings.Repeat("c", 32), roadmap.Root+"busy.md"
	busy.Operation = &roadmap.PlanningOperation{Kind: "message"}
	require(t, port.store.SavePlanning(ctx, first.Project, "", nil, first, second, busy))
	duplicate := first
	duplicate.ID, duplicate.Path = strings.Repeat("d", 32), roadmap.Root+"NEW.md"
	if err := port.store.SavePlanning(ctx, first.Project, "", nil, duplicate); err == nil {
		t.Fatal("case-insensitive planning path was occupied twice")
	}
	project := httpcontract.ProjectID(first.Project)
	batch := httpcontract.CommandEnvelope{Action: planningAction + "rebase_all", ProjectID: project, TargetID: project, ExpectedVersion: 1, Payload: []byte(`{}`)}
	single := httpcontract.CommandEnvelope{Action: planningAction + "rebase", ProjectID: project, TargetID: first.ID, ExpectedVersion: first.Revision, Payload: []byte(`{}`)}
	refresh := port.refreshBaselineWithRewrite
	refreshes := 0
	port.refreshBaselineWithRewrite = func(ctx context.Context) (string, bool, error) {
		refreshes++
		return refresh(ctx)
	}
	queued, summary, err := port.prepare(ctx, batch)
	require(t, err)
	batchSummary := summary.(map[string]any)
	if refreshes != 1 || len(queued) != 2 || queued[0].Operation.Target != first.Base || queued[1].Operation.Target != first.Base ||
		batchSummary["target"] != first.Base || batchSummary["skipped"].(map[string]string)[busy.ID] == "" {
		t.Fatalf("batch did not preserve independent eligibility and one fixed target: %+v %v", queued, summary)
	}
	queued, _, err = port.prepare(ctx, single)
	require(t, err)
	if refreshes != 2 || len(queued) != 1 || queued[0].Operation.Target != first.Base {
		t.Fatalf("single Rebase did not freeze its refreshed target: %+v", queued)
	}
	port.refreshBaselineWithRewrite = func(context.Context) (string, bool, error) { return first.Base, true, nil }
	for _, in := range []httpcontract.CommandEnvelope{batch, single} {
		if queued, _, err := port.prepare(ctx, in); len(queued) != 0 || err == nil || err.Error() != "目标分支历史已改写，请先核对项目基线" {
			t.Fatalf("%s accepted a rewritten target: %+v %v", in.Action, queued, err)
		}
	}
}

type planningSubmitFixture struct {
	run  *runnercontract.Run
	sent *runnercontract.Request
}

func (f *planningSubmitFixture) BuildPin() runnercontract.BuildPin { return runnercontract.BuildPin{} }
func (f *planningSubmitFixture) Inspect(_ context.Context, _, _ string) (runnercontract.Inspection, error) {
	if f.run == nil {
		return runnercontract.Inspection{}, &runnerclient.Failure{Code: "request_not_found"}
	}
	return runnercontract.Inspection{Run: f.run, Command: &runnercontract.Command{Command: "submit", ActionStatus: "completed"}}, nil
}
func (f *planningSubmitFixture) Call(_ context.Context, _ string, _ []string, req *runnercontract.Request) (runnerclient.Response, error) {
	f.sent = req
	return runnerclient.Response{}, nil
}

func TestRoadmapPlanningMaterialSendAndOriginalRecovery(t *testing.T) {
	t.Run("create at refreshed target", testPlanningCreateAtRefreshedTarget)
	t.Run("task card uses the original session and settles only the planning turn", testTaskCardPlanningTurn)
	ctx := context.Background()
	file := filepath.Join(t.TempDir(), "rules.md")
	rules := []byte("fixed rules")
	task := roadmap.PlanningTask{ID: "plan", Material: file, MaterialSHA: roadmap.Digest(rules)}
	pending := &runnercontract.Request{Protocol: runnercontract.Protocol, RequestID: "original", Input: []byte(`{"conversation":{"instruction":"read rules.md","materials":[]}}`)}
	task.Operation = &roadmap.PlanningOperation{Command: "submit", Request: pending}
	fixture := &planningSubmitFixture{}
	port := &planningPort{roadmapPort: &roadmapPort{}, runner: fixture}
	if _, err := port.runPlanningControl(ctx, &task); err == nil || fixture.sent != nil {
		t.Fatal("missing rules reached Runner")
	}
	require(t, storePlanningMaterial(file, rules))
	require(t, storePlanningMaterial(file, rules))
	_, err := port.runPlanningControl(ctx, &task)
	require(t, err)
	if fixture.sent != pending {
		t.Fatal("submitted different envelope")
	}
	require(t, os.WriteFile(file, []byte("changed rules"), 0600))
	if err := storePlanningMaterial(file, rules); err == nil {
		t.Fatal("creation retry overwrote drifted material")
	}
	fixture.sent = nil
	if _, err = port.runPlanningControl(ctx, &task); err == nil || fixture.sent != nil {
		t.Fatal("drifted rules reached Runner")
	}
	fixture.run = &runnercontract.Run{RunID: "run", Correlation: map[string]string{"steerline_planning": task.ID}, Workflow: runnercontract.Workflow{ID: "conversation", Version: 2}}
	_, err = port.runPlanningControl(ctx, &task)
	require(t, err)
	if task.Run == nil || task.Run.RunID != "run" || task.Operation != nil || fixture.sent != nil {
		t.Fatal("accepted original was not reconciled")
	}
}

func testTaskCardPlanningTurn(t *testing.T) {
	ctx := context.Background()
	port, task, _ := planningGitFixture(t)
	task.Kind, task.Path = roadmap.SingleTaskPlanning, roadmap.TaskCardPath(task.ID)
	task.Material = ""
	task.Goal = "Discuss a release"
	task.Run.Controls.CanMessage = true
	task.Run.WorkspaceWrite = &runnercontract.WorkspaceWrite{Owner: "runner"}
	task.Run.Execution.TurnID = "discussion"
	port.store = &planningStoreFixture{task: mustJSON(task)}
	port.readAdmissionMapping = func(context.Context, string, string) (map[string]string, error) { return nil, nil }
	oldCard := `{"goal":"Old","scope":"Scope","constraints":"","acceptance":"Check"}`
	newCard := `{"goal":"New","scope":"Scope","constraints":"","acceptance":"Check"}`
	writePlanningFixture(t, task.Run.Execution.Workspace, task.Path, oldCard)
	in := httpcontract.CommandEnvelope{Action: planningAction + taskCardOperation, TargetID: task.ID, ExpectedVersion: task.Revision, Payload: []byte(`{}`)}
	rows, _, err := port.prepare(ctx, in)
	require(t, err)
	task = rows[0]
	if task.Operation.Previous != "discussion" || task.Operation.Command != "message" || !viewPlanning(task).PlanningCard {
		t.Fatalf("task card operation did not bind the original turn: %+v", task.Operation)
	}
	var control struct{ Text string }
	require(t, json.Unmarshal(task.Operation.Request.Input, &control))
	if !strings.Contains(control.Text, task.Material) || !strings.Contains(control.Text, task.Path) || strings.Contains(control.Text, task.Goal) {
		t.Fatalf("unexpected follow-up instruction: %q", control.Text)
	}
	receipt := &runnercontract.Command{Command: "message", ActionStatus: "completed"}
	finished, err := finishTaskCardPlanning(&task, receipt)
	require(t, err)
	if finished || task.Operation == nil {
		t.Fatal("accepted message was treated as completed before the new turn")
	}
	task.Run.Execution.TurnID = "planning"
	task.Run.Execution.NativeOutcome = "succeeded"
	if _, err = finishTaskCardPlanning(&task, receipt); err == nil || task.Body != "" {
		t.Fatal("file from discussion was incorrectly adopted as a new task card")
	}
	task.Operation = rows[0].Operation
	writePlanningFixture(t, task.Run.Execution.Workspace, task.Path, newCard)
	finished, err = finishTaskCardPlanning(&task, receipt)
	require(t, err)
	if !finished || task.Body != newCard || task.CardVersion != 1 {
		t.Fatal("completed planning turn did not adopt the new card")
	}
	task.Operation = &roadmap.PlanningOperation{Kind: taskCardOperation, Previous: "planning"}
	task.Run.Execution.TurnID = "replanning"
	task.Run.Execution.NativeOutcome = "failed"
	writePlanningFixture(t, task.Run.Execution.Workspace, task.Path, oldCard)
	if _, err = finishTaskCardPlanning(&task, receipt); err == nil || task.Body != newCard || task.CardVersion != 1 {
		t.Fatal("a failed turn replaced the verified card")
	}
	task.Operation = rows[0].Operation
	fixture := &planningSubmitFixture{}
	port.runner = fixture
	require(t, os.Remove(task.Material))
	if _, err = port.runPlanningControl(ctx, &task); err == nil || fixture.sent != nil || task.Operation != nil || task.Body != newCard {
		t.Fatal("a missing rules file sent an instruction, held the original session busy or replaced the card")
	}
}

func testPlanningCreateAtRefreshedTarget(t *testing.T) {
	ctx := context.Background()
	port, task, remote := planningGitFixture(t)
	var caps runnerclient.Capabilities
	require(t, json.Unmarshal([]byte(`{
		"workflows":[{"id":"conversation","version":2,"supported_adapters":["codex-app-server"]}],
		"workspace_write":{"workflow":"conversation/2","acquire":"workspace acquire","release":"workspace release","run_property":"workspace_write","survives_service_restart":true},
		"configuration_schemas":{"workflow":{},"run_configuration":{},"managed_conversation_input":{}}
	}`), &caps))
	catalog, err := runnerclient.CompileCatalog(caps)
	require(t, err)
	port.config.runtimeCatalog = catalog
	port.admit = func(context.Context) error { return nil }
	port.runner = &planningSubmitFixture{}

	worker := task.Run.Execution.Workspace
	writePlanningFixture(t, worker, task.Path, "# Existing draft\n")
	card := roadmap.TaskCardPath(task.ID)
	writePlanningFixture(t, worker, card, "{}\n")
	pmGit(t, worker, "add", "--", task.Path, card)
	pmGit(t, worker, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "existing draft")
	pmGit(t, worker, "push", remote, "HEAD:refs/heads/main")
	target := pmGit(t, worker, "rev-parse", "HEAD")
	project := httpcontract.ProjectID(task.Project)
	in := httpcontract.CommandEnvelope{Action: planningAction + "create", ProjectID: project, TargetID: project, ExpectedVersion: 1, RequestID: task.ID}
	for _, payload := range []string{
		`{"title":"Planning","goal":"Plan a release","file":"NEW.md"}`,
		`{"title":"Planning","goal":"Plan a release","kind":"task"}`,
	} {
		in.Payload = []byte(payload)
		if _, _, err := port.prepare(ctx, in); err == nil || err.Error() != "目标分支已有同名草稿" {
			t.Fatalf("creation did not check %s at the refreshed remote target: %v", payload, err)
		}
	}
	cardID := strings.Repeat("c", 32)
	discussID := strings.Repeat("d", 32)
	for _, tc := range []struct{ id, payload, kind, path string }{
		{strings.Repeat("b", 32), `{"title":"Planning","goal":"Plan a release","file":"fresh.md"}`, "", roadmap.Root + "fresh.md"},
		{cardID, `{"title":"Planning","goal":"Plan a release","kind":"task"}`, roadmap.SingleTaskPlanning, roadmap.TaskCardPath(cardID)},
		{discussID, `{"title":"Planning","goal":"  Discuss a release\r\n","kind":"task","discuss":true}`, roadmap.SingleTaskPlanning, roadmap.TaskCardPath(discussID)},
	} {
		in.RequestID = tc.id
		in.Payload = []byte(tc.payload)
		created, _, err := port.prepare(ctx, in)
		require(t, err)
		if len(created) != 1 || created[0].Base != target || created[0].Kind != tc.kind || created[0].Path != tc.path {
			t.Fatalf("creation did not preserve the kind, path and refreshed target: %+v", created)
		}
		var submission runnercontract.ConversationRunInput
		require(t, json.Unmarshal(created[0].Submission.Input, &submission))
		if submission.Repository.BaseRevision != target {
			t.Fatalf("Runner submission did not use the frozen target: %+v", submission.Repository)
		}
		if tc.id == discussID && (submission.Conversation.Instruction != "  Discuss a release\r\n" || created[0].Material != "" || viewPlanning(created[0]).PlanningCard) {
			t.Fatal("discussion creation changed the first message or injected planning rules")
		}
		if tc.id == cardID && (!strings.Contains(submission.Conversation.Instruction, "Plan a release") || !strings.Contains(submission.Conversation.Instruction, tc.path) || !viewPlanning(created[0]).PlanningCard) {
			t.Fatal("direct creation did not start a planning turn")
		}
	}
}

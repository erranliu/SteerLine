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
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	roadmap "github.com/erranliu/SteerLine/internal/roadmap/model"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
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
	config.runtimeMaintenance = newProjectMaintenance(config, store)
	port := &planningPort{roadmapPort: &roadmapPort{config: config, store: store, materials: config.runtimeMaintenance}}
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

type planningLeaseFixture struct {
	run        *runnercontract.Run
	command    *runnercontract.Command
	stale      bool
	inspectErr error
}

func (f planningLeaseFixture) BuildPin() runnercontract.BuildPin { return runnercontract.BuildPin{} }
func (f planningLeaseFixture) Inspect(_ context.Context, selector, _ string) (runnercontract.Inspection, error) {
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

func TestRoadmapPlanningRecoveryRejectsHistoricalLease(t *testing.T) {
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
		reopened := &planningPort{roadmapPort: owner.roadmapPort, runner: acquire}
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
		finish := &planningPort{roadmapPort: owner.roadmapPort, runner: planningLeaseFixture{run: &released, command: &runnercontract.Command{Command: "workspace release", ActionStatus: "completed"}}}
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
	queued, summary, err := port.prepare(ctx, httpcontract.CommandEnvelope{Action: planningAction + "rebase_all", ProjectID: project, TargetID: project, ExpectedVersion: 1, Payload: []byte(`{}`)})
	require(t, err)
	if len(queued) != 2 || queued[0].Operation.Target != first.Base || queued[1].Operation.Target != first.Base || summary.(map[string]any)["skipped"].(map[string]string)[busy.ID] == "" {
		t.Fatalf("batch did not preserve independent eligibility and one fixed target: %+v %v", queued, summary)
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

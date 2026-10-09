package testenv

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
	"github.com/erranliu/SteerLine/tests/e2e/internal/driver"
	"github.com/erranliu/SteerLine/tests/e2e/internal/evidence"
	"github.com/erranliu/SteerLine/tests/e2e/internal/native"
)

func (s *Scenario) newRemoteProject() *Project {
	m, err := native.LoadRemoteManifest(s.Run.RemoteConfig, s.Run.SourceCommit)
	s.Must(err)
	command := os.Getenv("ORCA_CLI_COMMAND")
	if command == "" {
		command = "orca"
	}
	executable, err := exec.LookPath(command)
	s.Must(err)
	peer := &native.RemoteOrca{Manifest: m, Executable: executable, Record: func(label string, value any) error {
		return controlled.Write(filepath.Join(s.Output, fmt.Sprintf("remote-%s-%d.json", label, time.Now().UnixNano())), value)
	}}
	s.Must(peer.Preflight(s.Context))
	s.Must(controlled.Write(filepath.Join(s.Output, "remote-target.json"), map[string]any{"manifest": m, "pair": peer.Pair, "repository_path": peer.RepoPath}))
	p := s.NewProject("remote")
	p.RemoteOrca = peer
	p.Chain = "orca-remote-codex"
	p.OrcaExecutable = executable
	p.enableGitHub()
	// Contracts travel through the isolated Git base. Worker credentials remain
	// on the paired host; no center auth, absolute temp path or private agent home is sent.
	publish := `$ErrorActionPreference='Stop'
git add -- result.txt
if ($LASTEXITCODE -ne 0) { throw 'stage failed' }
git -c user.name=E2E -c user.email=e2e@example.invalid -c commit.gpgsign=false commit -m 'Remote Orca E2E delivery'
if ($LASTEXITCODE -ne 0) { throw 'commit failed' }
$branch=(git symbolic-ref --short HEAD).Trim()
if ($branch -eq 'main' -or $branch -eq ` + native.PSQuote(p.BaseBranch) + `) { throw 'refuse base push' }
git -c credential.helper= -c 'credential.helper=!gh auth git-credential' push origin ('HEAD:refs/heads/'+$branch)
if ($LASTEXITCODE -ne 0) { throw 'push failed' }
$existing=gh pr list --repo erranliu/SteerlineTest --head $branch --base ` + native.PSQuote(p.BaseBranch) + ` --state open --json url
if ($LASTEXITCODE -ne 0) { throw 'PR lookup failed' }
$prs=@($existing|ConvertFrom-Json)
if ($prs.Count -gt 1) { throw 'ambiguous PR' }
if ($prs.Count -eq 1) { Write-Output $prs[0].url } else {
 gh pr create --repo erranliu/SteerlineTest --base ` + native.PSQuote(p.BaseBranch) + ` --head $branch --draft --title 'SteerLine remote E2E' --body 'Isolated real remote Orca double turn delivery.'
 if ($LASTEXITCODE -ne 0) { throw 'PR create failed' }
}
`
	s.Must(os.WriteFile(filepath.Join(p.Repo, "publish.ps1"), []byte(publish), 0600))
	agents := "# Isolated remote Orca fixture\n\nOnly perform the bound business task in this worktree. Preserve original Spec/Plan files. Use the remote host's native tools and authentication. Follow the original Orca preamble to report worker_done exactly once; writing a file or saying done is insufficient. Never request other agents, reset the worktree, change global host or agent settings or push to main.\n"
	spec := "# Remote live delivery\n\nCreate result.txt containing exactly `remote first turn` followed by one newline, UTF-8 without BOM. Verify its exact content, then run `powershell.exe -NoProfile -ExecutionPolicy Bypass -File ./publish.ps1` once. This commits, pushes the assigned head and creates the real isolated GitHub PR. Report its complete URL in final and in Orca worker_done. Subsequent feedback uses the same script to update the same PR.\n\n## Acceptance\n\nThe first result matches the contract and the original agent completes feedback on the same branch before merge.\n"
	plan := "# Remote fixture plan\n\nRead the original Spec. Write and verify result.txt; execute publish.ps1 once; report the PR URL through Orca.\n\n## Test plan\n\nCompare the exact file content. The harness verifies both commits, original Orca public output source, launch selection, PR head/base and unique delivery after merge.\n"
	for name, content := range map[string]string{"AGENTS.md": agents, "docs/spec/task.spec.md": spec, "docs/spec/task.plan.md": plan} {
		s.Must(os.WriteFile(filepath.Join(p.Repo, filepath.FromSlash(name)), []byte(content), 0600))
	}
	p.git(p.Repo, "add", ".")
	p.git(p.Repo, "commit", "--quiet", "-m", "Pin remote E2E contracts")
	p.git(p.Repo, "push", "--quiet", "origin", p.BaseBranch)
	base := p.git(p.Repo, "rev-parse", "HEAD")
	bootstrap := peer.Pair.RepositoryID + "::" + peer.RepoPath
	_, err = peer.Shell(s.Context, bootstrap, requestID(), `gh api user --jq .login | Out-Null; if ($LASTEXITCODE -ne 0) { throw 'remote GitHub authentication unavailable' }; git -c credential.helper= -c 'credential.helper=!gh auth git-credential' fetch --no-tags origin `+native.PSQuote("refs/heads/"+p.BaseBranch)+`; if ($LASTEXITCODE -ne 0) { throw 'remote base fetch failed' }; $head=(git rev-parse FETCH_HEAD).Trim(); if ($head -cne `+native.PSQuote(base)+`) {throw 'remote fetched another base'}; @{base=$head;ready=$true}`)
	s.Must(err)
	// Register the independent center checkout solely for its coordinator shell.
	_, err = p.orca(s.Context, "repo", "add", "--path", p.Repo, "--json")
	s.Must(err)
	p.OrcaRepoID, err = native.FindOrcaRepo(s.Context, p.orca, p.Repo)
	s.Must(err)
	p.OrcaSetupID, err = p.findSetup(s.Context)
	s.Must(err)
	s.register("orca-repo", p.OrcaRepoID, 0)
	traceExecutable := filepath.Join(p.Home, "bin", "orca-remote-trace.exe")
	traceBinary, err := os.ReadFile(s.Run.Helper)
	s.Must(err)
	s.Must(os.WriteFile(traceExecutable, traceBinary, 0700))
	s.Must(controlled.Write(filepath.Join(p.Home, "remote-trace.json"), map[string]string{"Executable": executable, "Output": s.Output}))
	configs, err := native.RemoteProfileConfigs(traceExecutable, m)
	s.Must(err)
	profile := map[string]any{"id": p.Chain, "name": "Remote Orca E2E", "host": "orca_supervised", "agent": "codex", "implementation_configs": configs}
	s.Must(controlled.Write(filepath.Join(p.Home, "execution.json"), map[string]any{"version": 2, "execution_profiles": []any{profile}, "default_execution_profile": p.Chain}))
	p.Driver.Profile = p.Chain
	return p
}

func (p *Project) remoteOutput(ctx context.Context, snap Snapshot, label string) (native.RemoteOutputEvidence, error) {
	var output native.RemoteOutputEvidence
	if p.RemoteOrca == nil || len(snap.Domain.Executions) != 1 {
		return output, errors.New("public remote output needs one exact execution")
	}
	e := snap.Domain.Executions[0]
	if e.Process == nil || e.Process.ExecutionHost != p.RemoteOrca.Manifest.EnvironmentID || e.Process.RuntimeID != p.RemoteOrca.Pair.RemoteRuntime {
		return output, errors.New("remote public output target mismatch")
	}
	if err := p.RemoteOrca.CheckWorkerTerminalInventory(ctx, *e.Process, e.Agent); err != nil {
		return output, err
	}
	settings, err := evidence.RemoteModelSelection(snap.Domain)
	if err != nil || settings.Model != native.Model || settings.Effort != native.Effort {
		return output, errors.New("frozen Orca launch selection differs")
	}
	output, err = p.RemoteOrca.Output(ctx, e.Process.Dispatch, snap.Helper.PR.URL)
	if output.Provider != e.Agent {
		return output, errors.New("Orca provider differs from opaque selected agent")
	}
	if writeErr := controlled.Write(filepath.Join(p.Scenario.Output, "remote-"+label+"-public-output.json"), output); writeErr != nil {
		return output, writeErr
	}
	return output, err
}

func RemoteDelivery(s *Scenario) {
	p := s.newRemoteProject()
	p.CreateTasks()
	p.StartService()
	p.Claim("a")
	first := p.WaitPhase("waiting_pr_merge")
	p.Record("first", first)
	if len(first.Domain.Executions) != 1 || first.Helper.PR.State != "open" {
		s.t.Fatal("remote first PR missing")
	}
	original := first.Domain.Executions[0]
	if original.Process == nil || original.Protocol != "orca_supervised" || original.Process.ExecutionHost != p.RemoteOrca.Manifest.EnvironmentID || original.Process.RuntimeID != p.RemoteOrca.Pair.RemoteRuntime || original.TurnNumber != 1 {
		s.t.Fatal("remote first execution binding differs")
	}
	s.register("orca-remote-worktree", original.Process.WorktreeID, 0)
	p.verifyRemoteFile(first, "remote first turn")
	s.Must(p.RemoteOrca.RevealWorker(s.Context, *original.Process))
	firstOutput, err := p.remoteOutput(s.Context, first, "first")
	s.Must(err)
	for _, task := range first.Page.Tasks {
		if task.TaskID == p.Tasks["b"].ID && (task.DependencyReady || len(task.WorkflowRuns) != 0) {
			s.t.Fatal("successor released before merge")
		}
	}
	var decisionID string
	var decisionVersion, revision uint64
	if first.Runtime != nil {
		for _, run := range first.Runtime.Runs {
			if run.RunID == original.WorkflowRunID && run.Decision != nil {
				decisionID = run.Decision.ID
				decisionVersion = run.Decision.Version
				revision = run.ControlRevision
			}
		}
	}
	if decisionID == "" {
		s.t.Fatal("remote first turn lacks feedback decision")
	}
	request := driver.CandidateCommand{Action: "answer_decision", RequestID: requestID(), RunID: original.WorkflowRunID, ExecutionID: original.ExecutionID, DecisionID: decisionID, DecisionVersion: decisionVersion, RunRevision: revision, InstanceID: first.Runtime.Service.InstanceID, Note: "Continue this original agent session. Replace result.txt with exactly `remote second turn` followed by one newline, UTF-8 without BOM. Verify it, run publish.ps1 once to commit/push and update the SAME PR, then report its complete URL through the original Orca worker_done protocol. Preserve the original worktree, branch and task references."}
	receipt, err := p.Driver.Candidate(s.Context, s.Run.Helper, request)
	s.Must(err)
	s.Must(controlled.Write(filepath.Join(s.Output, "feedback-receipt.json"), receipt))
	var second Snapshot
	s.Wait("remote second turn settled", func() (bool, error) {
		snap, err := p.Snapshot()
		if err != nil {
			return false, err
		}
		if len(snap.Domain.Executions) != 1 {
			return false, errors.New("remote feedback created another execution")
		}
		e := snap.Domain.Executions[0]
		if e.PendingCommandID != "" || e.CurrentCommandID != request.RequestID || e.LastStoppedTurnID != e.CurrentTurnID {
			return false, nil
		}
		if len(snap.Domain.WorkflowRuns) != 1 || snap.Domain.WorkflowRuns[0].Phase != "waiting_pr_merge" {
			return false, nil
		}
		second = snap
		return true, nil
	})
	next := second.Domain.Executions[0]
	p.Record("second", second)
	if next.Process == nil || next.ExecutionID != original.ExecutionID || next.CurrentAttemptID != original.CurrentAttemptID || next.Workspace.Worktree != original.Workspace.Worktree || next.Workspace.Branch != original.Workspace.Branch || next.Process.Terminal != original.Process.Terminal || next.Process.Incarnation != original.Process.Incarnation || next.Process.OrchestrationRun != original.Process.OrchestrationRun || next.Process.Dispatch == original.Process.Dispatch || next.Process.Task == original.Process.Task || next.TurnNumber != 2 || second.Helper.PR.URL != first.Helper.PR.URL || second.Helper.PR.HeadRevision == first.Helper.PR.HeadRevision {
		s.t.Fatal("remote feedback replaced session/workspace or failed to update original PR")
	}
	p.verifyRemoteFile(second, "remote second turn")
	secondOutput, err := p.remoteOutput(s.Context, second, "second")
	s.Must(err)
	if secondOutput.SourceIdentity != firstOutput.SourceIdentity || secondOutput.DispatchID == firstOutput.DispatchID {
		s.t.Fatal("remote feedback public output continuity missing")
	}
	p.Merge()
	done := p.WaitPhase("completed")
	p.Record("completed", done)
	if len(done.Domain.Deliveries) != 1 || !done.Helper.PR.Merged || done.Helper.PR.State != "merged" {
		s.t.Fatal("remote real unique delivery missing")
	}
	for _, task := range done.Page.Tasks {
		if task.TaskID == p.Tasks["a"].ID && (task.Status != "closed" || task.Delivery == nil || task.Delivery.SelectedWorkflowRunID != original.WorkflowRunID) {
			s.t.Fatal("wrong remote task delivery")
		}
		if task.TaskID == p.Tasks["b"].ID && (!task.DependencyReady || len(task.WorkflowRuns) != 0) {
			s.t.Fatal("remote successor not ready and unstarted")
		}
	}
	finalOutput, err := p.remoteOutput(s.Context, done, "final")
	s.Must(err)
	if finalOutput != secondOutput {
		s.t.Fatal("merge/polling changed original public output")
	}
	proof := done.Helper.PR
	p.RealPRProof = &proof
	s.remoteOutputVerified = true
	s.Must(s.finishBody())
	p.StopService()
}

func (p *Project) verifyRemoteFile(snap Snapshot, want string) {
	// Fetch the exact PR head to the center fixture. Never use a remote path with
	// center-local Git or claim local filesystem inspection of the worker.
	p.git(p.Repo, "fetch", "--quiet", "origin", snap.Helper.PR.HeadRevision)
	content := p.git(p.Repo, "show", snap.Helper.PR.HeadRevision+":result.txt")
	if content != want {
		p.Scenario.t.Fatalf("remote committed result differs: %q", content)
	}
}

func (p *Project) cleanupRemoteOrca() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if p.ProjectID == "" {
		return nil
	}
	snap, err := p.SnapshotContext(ctx)
	if err != nil {
		return err
	}
	for _, e := range snap.Domain.Executions {
		worktreeID := e.Target.WorktreeID
		if worktreeID == "" {
			continue
		}
		if e.Target.ExternalRuntime != p.RemoteOrca.Manifest.EnvironmentID || !strings.HasPrefix(worktreeID, p.RemoteOrca.Pair.RepositoryID+"::") || e.Workspace.Worktree == "" {
			return errors.New("remote workspace ownership unproven")
		}
		if e.Workspace.Branch != "" && e.Workspace.Branch != p.BaseBranch && e.Workspace.Branch != p.DefaultBranch && strings.Contains(e.Workspace.Branch, e.WorkflowRunID) {
			p.RemoteBranches = append(p.RemoteBranches, e.Workspace.Branch)
		}
		if e.Process != nil {
			identity := e.Process
			if identity.ExecutionHost != p.RemoteOrca.Manifest.EnvironmentID || identity.RuntimeID != p.RemoteOrca.Pair.RemoteRuntime || identity.WorktreeID != worktreeID {
				return errors.New("remote worker cleanup target changed")
			}
			env, callErr := p.RemoteOrca.Raw(ctx, "orchestration", "worker-show", "--dispatch", identity.Dispatch, "--json")
			if callErr != nil {
				return callErr
			}
			var shown struct {
				Dispatch           struct{ ID, RunID, TaskID, Status string }
				Worker             struct{ WorktreeID, AgentTerminalHandle string }
				RemoteRuntimeEpoch string
				Observation        struct{ ExactWorker bool }
				Terminal           struct{ Handle, IncarnationID string }
			}
			if json.Unmarshal(env.Result, &shown) != nil || env.Meta.RuntimeID != identity.CoordinatorRuntimeID || shown.Dispatch.ID != identity.Dispatch || shown.Dispatch.RunID != identity.OrchestrationRun || shown.Dispatch.TaskID != identity.Task || shown.RemoteRuntimeEpoch != identity.RuntimeID || !shown.Observation.ExactWorker || shown.Terminal.Handle != identity.Terminal || shown.Terminal.IncarnationID != identity.Incarnation || shown.Worker.WorktreeID != worktreeID {
				return errors.New("remote cleanup worker binding changed")
			}
			verb := "worker-stop"
			if shown.Dispatch.Status == "completed" || shown.Dispatch.Status == "failed" {
				verb = "worker-release"
			}
			result, callErr := p.RemoteOrca.Raw(ctx, "orchestration", verb, "--dispatch", identity.Dispatch, "--json")
			if callErr != nil {
				return callErr
			}
			if result.Meta.RuntimeID != identity.CoordinatorRuntimeID {
				return errors.New("remote cleanup coordinator runtime changed")
			}
			if verb == "worker-release" {
				var released struct{ DispatchID, State string }
				if json.Unmarshal(result.Result, &released) != nil || released.DispatchID != identity.Dispatch || (released.State != "released" && released.State != "already_released") {
					return errors.New("remote release unconfirmed; preserve fixture")
				}
			} else {
				exited := false
				for !exited && ctx.Err() == nil {
					observed, readErr := p.RemoteOrca.Raw(ctx, "orchestration", "worker-show", "--dispatch", identity.Dispatch, "--json")
					if readErr != nil {
						return readErr
					}
					var state struct {
						Dispatch           struct{ ID string }
						RemoteRuntimeEpoch string
						Projection         struct{ Liveness struct{ Verdict string } }
					}
					if json.Unmarshal(observed.Result, &state) != nil || observed.Meta.RuntimeID != identity.CoordinatorRuntimeID || state.Dispatch.ID != identity.Dispatch || state.RemoteRuntimeEpoch != identity.RuntimeID {
						return errors.New("remote stop observation identity changed")
					}
					exited = state.Projection.Liveness.Verdict == "exited"
					if !exited {
						select {
						case <-ctx.Done():
						case <-time.After(250 * time.Millisecond):
						}
					}
				}
				if !exited {
					return errors.New("remote stop could not prove exit; preserve fixture")
				}
			}
		}
		// The isolated workspace and all of its fresh fallback/setup shells are
		// test-owned. Resource cleanup completes before deleting GitHub fixtures.
		closed, closeErr := p.RemoteOrca.Raw(ctx, "terminal", "close", "--worktree", "id:"+worktreeID, "--all", "--environment", p.RemoteOrca.Manifest.EnvironmentID, "--json")
		if closeErr != nil || closed.Meta.RuntimeID != p.RemoteOrca.Pair.RemoteRuntime {
			return errors.Join(closeErr, errors.New("remote workspace terminal close unconfirmed"))
		}
		removed, removeErr := p.RemoteOrca.Raw(ctx, "worktree", "rm", "--worktree", "id:"+worktreeID, "--force", "--environment", p.RemoteOrca.Manifest.EnvironmentID, "--json")
		if removeErr != nil {
			return removeErr
		}
		var removal struct{ Removed bool }
		if json.Unmarshal(removed.Result, &removal) != nil || !removal.Removed || removed.Meta.RuntimeID != p.RemoteOrca.Pair.RemoteRuntime {
			return errors.New("remote workspace removal unconfirmed")
		}
	}
	return nil
}

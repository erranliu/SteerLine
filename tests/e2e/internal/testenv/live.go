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

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	testprocess "github.com/erranliu/SteerLine/internal/testkit/process"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
	"github.com/erranliu/SteerLine/tests/e2e/internal/evidence"
	"github.com/erranliu/SteerLine/tests/e2e/internal/native"
)

func (s *Scenario) NewLiveProject(name, chain string) *Project {
	s.t.Helper()
	if chain != "local-codex" && chain != "orca-codex" {
		s.t.Fatal("unknown native chain")
	}
	p := s.NewProject(name)
	p.Chain = chain
	if chain == "local-codex" {
		s.Must(native.SeedAuth(s.Run.AuthHome, filepath.Join(p.Home, "codex"), "unelevated"))
		s.Must(native.TrustProject(filepath.Join(p.Home, "codex"), p.Repo))
	}
	if strings.HasPrefix(s.ID, "smoke.") || strings.HasPrefix(s.ID, "candidates.") {
		p.enableGitHub()
	}
	codex, err := native.CodexExecutable("codex")
	s.Must(err)
	publisher, host := "worker", "local"
	var orcaConfig map[string]any
	if chain == "orca-codex" {
		s.orcaUsageUnavailable = true
		host, publisher = "orca", "worker"
		command := os.Getenv("ORCA_CLI_COMMAND")
		if command == "" {
			command = "orca"
		}
		p.OrcaExecutable, err = exec.LookPath(command)
		s.Must(err)
		// The fixture preserves native argv and receipts without injecting agent settings.
		shim := filepath.Join(p.Home, "bin", "orca-env.exe")
		binary, readErr := os.ReadFile(s.Run.Helper)
		s.Must(readErr)
		s.Must(os.WriteFile(shim, binary, 0700))
		logDir := filepath.Join(s.Output, name+"-orca")
		s.Must(os.MkdirAll(logDir, 0700))
		s.Must(controlled.Write(filepath.Join(p.Home, "orca-env.json"), native.OrcaEnvironment{Executable: p.OrcaExecutable, LogDir: logDir}))
		orcaConfig = native.OrcaWorkerProfile(shim, p.Repo, "30s")
	}
	var config map[string]any
	raw, err := os.ReadFile(p.Config)
	s.Must(err)
	s.Must(json.Unmarshal(raw, &config))
	config["publication_policy"] = map[string]string{"publisher": publisher}
	// Exercise the production default; startup owns its separate readiness budget.
	delete(config, "command_timeout")
	s.Must(controlled.Write(p.Config, config))
	implementations := native.WorkerImplementationConfigs(codex, orcaConfig)
	profile := map[string]any{"id": chain, "name": "Real E2E " + chain, "host": host, "agent": "codex", "implementation_configs": implementations}
	s.Must(controlled.Write(filepath.Join(p.Home, "execution.json"), map[string]any{"version": 2, "execution_profiles": []any{profile}, "default_execution_profile": chain}))
	p.Driver.Profile = chain

	s.Must(os.WriteFile(filepath.Join(p.Repo, "AGENTS.md"), []byte("# Isolated SteerLine E2E fixture\n\nFor a readiness-only Dispatch, run only its exact probe command and report the result; do not inspect business task references. Preserve the COMPLETE native command tool result, including its exit_code and output. If using the exec JavaScript wrapper, use exactly `const r = await tools.exec_command({cmd: <exact probe command>, login: false}); text(r);`. Do not project r.output, stringify individual fields, add exit= text, or transform the result: the readiness verifier consumes the full native JSON result. For business work, only implement the bound task. Invoke shell tools with login=false to disable unrelated user shell profiles. Initially HEAD equals the reference commit: confirm with git rev-parse HEAD, then use Get-FileHash directly on docs/spec/task.spec.md and task.plan.md. Read those original files. Do not reconstruct files from git show text or PowerShell pipelines: newline conversion changes their hashes. Any scratch work must stay inside this assigned worktree, and must not be staged. No questions or other tasks are needed: the fixture contract specifies the entire output and publication action.\n"), 0600))
	spec := "# Live delivery fixture\n\nCreate result.txt containing exactly `real model delivery` followed by a newline. Verify the file. Stage only result.txt. Do not modify any other tracked file.\n\n"
	if p.RealPR {
		script := fmt.Sprintf(`$ErrorActionPreference = 'Stop'
if ((Get-Content -LiteralPath 'result.txt' -Raw).Trim() -cne 'real model delivery') { throw 'result differs' }
$env:GH_TOKEN = (Get-Content -LiteralPath '%s' -Raw).Trim()
$branch = git symbolic-ref --short HEAD
# The shell credential helper cannot run in the unelevated Windows sandbox.
# Keep the test token out of argv/config/logs and scope its header to this remote.
$env:GIT_CONFIG_COUNT = '1'
$env:GIT_CONFIG_KEY_0 = 'http.https://github.com/erranliu/SteerlineTest.git.extraheader'
$env:GIT_CONFIG_VALUE_0 = 'AUTHORIZATION: basic ' + [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes('x-access-token:' + $env:GH_TOKEN))
try {
  git -c credential.helper= push origin ('HEAD:refs/heads/' + $branch)
  if ($LASTEXITCODE -ne 0) { throw 'push failed' }
} finally {
  Remove-Item Env:GIT_CONFIG_COUNT,Env:GIT_CONFIG_KEY_0,Env:GIT_CONFIG_VALUE_0 -ErrorAction SilentlyContinue
}
gh pr create --repo '%s' --base '%s' --head $branch --draft --title 'SteerLine E2E %s' --body 'Isolated native gpt-6-luna medium delivery test. Disposable test branches.'
if ($LASTEXITCODE -ne 0) { throw 'PR create failed' }
`, filepath.Join(p.Home, "github-token"), githubRepository, p.BaseBranch, s.ID)
		s.Must(os.WriteFile(filepath.Join(p.Repo, "publish.ps1"), []byte(script), 0600))
		spec += "You own publication. Only in the publish step, after the local candidate is committed, execute `powershell.exe -NoProfile -ExecutionPolicy Bypass -File ./publish.ps1` once. It pushes to the real test repository and creates a real GitHub draft PR. Copy its returned URL verbatim into your final reply and completion relay body, including the repository owner. The URL must have the form https://github.com/erranliu/SteerlineTest/pull/<number>; do not shorten or reconstruct it.\n"
	} else {
		state, err := controlled.Read(p.Provider)
		s.Must(err)
		base := strings.TrimSuffix(state.GitURL, "/e2e/project.git")
		script := fmt.Sprintf(`$ErrorActionPreference = 'Stop'
if ((Get-Content -LiteralPath 'result.txt' -Raw).Trim() -cne 'real model delivery') { throw 'result differs' }
$branch = git symbolic-ref --short HEAD
if ($branch -ceq 'main') { throw 'publication must push the assigned PR head, not the base branch' }
$head = git rev-parse HEAD
git -c 'url.%s.insteadOf=https://github.com/e2e/project.git' push origin ('HEAD:refs/heads/' + $branch)
if ($LASTEXITCODE -ne 0) { throw 'push failed' }
$body = @{branch=$branch; head=$head} | ConvertTo-Json -Compress
$receipt = Invoke-RestMethod -Method Post -Uri '%s/__e2e/pull' -ContentType 'application/json' -Body $body
Write-Output $receipt.url
`, state.GitURL, base)
		s.Must(os.WriteFile(filepath.Join(p.Repo, "publish.ps1"), []byte(script), 0600))
		spec += "You own publication. Only in the publish step, after the local candidate is committed, execute `powershell.exe -NoProfile -ExecutionPolicy Bypass -File ./publish.ps1` once. The assigned working branch is the PR HEAD; main in the dispatch is the PR BASE, never a request to push to main. The origin URL remains the dispatch's https://github.com/e2e/project.git identity; only the script's single push command uses url.insteadOf to route its transport to the loopback server. This does not change repository config or the remote identity seen by task validation. The explicitly authorized loopback PR endpoint represents that same fixture provider. The script pushes origin's assigned head and creates the fixture PR. Report its returned URL in your final reply. Do not call gh or real GitHub.\n"
	}
	implement := `$ErrorActionPreference = 'Stop'
[IO.File]::WriteAllText((Join-Path (Get-Location) 'result.txt'), "real model delivery" + [char]10, [Text.UTF8Encoding]::new($false))
if ((Get-Content -LiteralPath 'result.txt' -Raw).Trim() -cne 'real model delivery') { throw 'result differs' }
git add -- result.txt
if ($LASTEXITCODE -ne 0) { throw 'stage failed' }
git -c user.name=E2E -c user.email=e2e@example.invalid -c commit.gpgsign=false commit -m 'Real model candidate'
if ($LASTEXITCODE -ne 0) { throw 'commit failed' }
git rev-parse HEAD
`
	s.Must(os.WriteFile(filepath.Join(p.Repo, "implement.ps1"), []byte(implement), 0600))
	spec += "In implement.execute, run powershell.exe -NoProfile -ExecutionPolicy Bypass -File ./implement.ps1 exactly once to write, verify and commit the candidate locally. Do not push or invoke publish.ps1 in implement.execute or implement.report. implement.report only summarizes its supplied source facts. publish.ps1 is reserved for the single publish step, which must return the JSON result contract in its native final reply.\n"

	if s.ID == "recovery.orca-codex" {
		spec += "\n## Original-execution recovery window\n\nBefore writing result.txt or publishing, run one shell command that writes `.e2e-recovery-started.txt` in this assigned worktree, waits until `.e2e-recovery-release.txt` exists, then removes both files. Use `Set-Content -LiteralPath .e2e-recovery-started.txt -Value waiting; while (-not (Test-Path -LiteralPath .e2e-recovery-release.txt)) { Start-Sleep -Milliseconds 250 }; Remove-Item -LiteralPath .e2e-recovery-started.txt,.e2e-recovery-release.txt`. The test driver releases the existing command; it does not send another prompt. These two files are temporary fixture controls: do not stage or commit them. After release, finish the original task and its publication in this same original round.\n"
	}
	spec += "\n## Acceptance\n\nThe committed result.txt equals the specified content and is delivered after the fixture PR merges.\n"
	s.Must(os.WriteFile(filepath.Join(p.Repo, "docs", "spec", "task.spec.md"), []byte(spec), 0600))
	s.Must(os.WriteFile(filepath.Join(p.Repo, "docs", "spec", "task.plan.md"), []byte("# Live fixture plan\n\nRead the bound Spec. In implement.execute run implement.ps1 to write, verify and commit the local candidate. In implement.report summarize only existing supplied evidence. In publish run publish.ps1 to push the working branch and create or update its PR; return strict result JSON in the same native final. Do not combine the steps.\n\n## Test plan\n\nVerify result.txt locally; the E2E harness checks native usage, real Git delivery and dependency state.\n"), 0600))
	p.git(p.Repo, "add", ".")
	p.git(p.Repo, "commit", "--quiet", "-m", "Pin live task contract")
	state, err := controlled.Read(p.Provider)
	s.Must(err)
	if p.RealPR {
		p.git(p.Repo, "push", "--quiet", "origin", p.BaseBranch)
	} else {
		p.git(p.Repo, "push", "--quiet", state.GitURL, "main")
	}
	if chain == "orca-codex" {
		_, err := p.orca(s.Context, "repo", "add", "--path", p.Repo, "--json")
		s.Must(err)
		// Read registration by the exact owned repository path, never display name.
		p.OrcaRepoID, err = native.FindOrcaRepo(s.Context, p.orca, p.Repo)
		s.Must(err)
		p.OrcaSetupID, err = p.findSetup(s.Context)
		s.Must(err)
		s.register("orca-repo", p.OrcaRepoID, 0)
	}
	return p
}

func (p *Project) orca(ctx context.Context, args ...string) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	cmd := exec.Command(p.OrcaExecutable, args...)
	var out, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &stderr
	err := testprocess.Run(ctx, cmd)
	payload, parseErr := native.OrcaPayload([]byte(out.String()))
	record := map[string]any{"command": append([]string{p.OrcaExecutable}, args...), "stdout": out.String(), "stderr": stderr.String(), "error": fmt.Sprint(err)}
	logErr := controlled.Write(filepath.Join(p.Scenario.Output, fmt.Sprintf("orca-call-%d.json", time.Now().UnixNano())), record)
	return payload, errors.Join(err, parseErr, logErr)
}
func (p *Project) findSetup(ctx context.Context) (string, error) {
	raw, err := p.orca(ctx, "project", "setups", "--json")
	if err != nil {
		return "", err
	}
	var listing struct {
		Setups []struct{ ID, RepoID, Path string } `json:"setups"`
	}
	if err := json.Unmarshal(raw, &listing); err != nil {
		return "", err
	}
	for _, setup := range listing.Setups {
		if setup.RepoID == p.OrcaRepoID && sameLocalPath(setup.Path, p.Repo) {
			return setup.ID, nil
		}
	}
	return "", errors.New("owned Orca registration setup missing")
}
func sameLocalPath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(filepath.FromSlash(a)), filepath.Clean(filepath.FromSlash(b)))
}

func (p *Project) cleanupOrca() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if p.OrcaRepoID == "" {
		id, err := native.FindOrcaRepo(ctx, p.orca, p.Repo)
		if errors.Is(err, native.ErrOrcaRepoUnavailable) {
			return nil
		}
		if err != nil {
			return err
		}
		p.OrcaRepoID = id
	}
	raw, err := p.orca(ctx, "worktree", "list", "--repo", "id:"+p.OrcaRepoID, "--limit", "1000", "--json")
	if err != nil {
		return err
	}
	worktrees, err := native.OrcaWorktrees(raw)
	if err != nil {
		return err
	}
	var failures []error
	for _, w := range worktrees {
		if w.RepoID != p.OrcaRepoID || w.ID == "" {
			failures = append(failures, errors.New("Orca cleanup inventory ownership mismatch"))
			continue
		}
		p.Scenario.register("orca-worktree", w.ID, 0)
		_, err := p.orca(ctx, "terminal", "close", "--worktree", "id:"+w.ID, "--all", "--json")
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if !sameLocalPath(w.Path, p.Repo) {
			_, err = p.orca(ctx, "worktree", "rm", "--worktree", "id:"+w.ID, "--force", "--json")
			if err != nil {
				failures = append(failures, err)
			}
		}
	}
	// Root coordinator terminals may be outside worktree-list's linked checkouts.
	_, err = p.orca(ctx, "terminal", "close", "--worktree", "path:"+filepath.ToSlash(p.Repo), "--all", "--json")
	if err != nil {
		failures = append(failures, err)
	}
	if len(failures) > 0 {
		return errors.Join(failures...)
	}
	setup, err := p.findSetup(ctx)
	if err != nil {
		return err
	}
	_, err = p.orca(ctx, "project", "setup-delete", "--setup", setup, "--json")
	return err
}

func (p *Project) NativeUsage(snap Snapshot, label string) (native.Usage, error) {

	var u native.Usage
	if len(snap.Domain.Executions) != 1 || len(snap.Domain.WorkflowRuns) != 1 {
		return u, errors.New("usage needs one bound execution")
	}
	e := snap.Domain.Executions[0]
	if e.Protocol == string(executionbackend.ProtocolOrcaTerminalV3) {
		return u, errors.New("token usage is not exposed by the Orca public interface")
	}
	profile, profileErr := evidence.ModelSelection(snap.Domain)
	if profileErr != nil || profile.Model != native.Model || profile.Effort != native.Effort {
		return u, errors.New("frozen native model/effort mismatch")
	}
	var usageEvidence []byte
	var err error
	if e.Backend == "local" {
		path := filepath.Join(filepath.Dir(p.Database), "runs", e.WorkflowRunID, "turns", e.CurrentCommandID, "appserver.jsonl")
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return u, readErr
		}
		session, bindErr := ExecutionSession(e)
		if bindErr != nil {
			return u, bindErr
		}
		u, usageEvidence, err = native.AppServerTurnUsage(raw, session, e.CurrentTurnID)
	} else {
		return u, errors.New("selected backend has no bound public usage source")
	}
	if err != nil {
		return u, err
	}
	if err := os.WriteFile(filepath.Join(p.Scenario.Output, p.Name+"-"+label+"-usage.jsonl"), usageEvidence, 0600); err != nil {
		return u, err
	}
	if err := controlled.Write(filepath.Join(p.Scenario.Output, p.Name+"-"+label+"-usage.json"), u); err != nil {
		return u, err
	}
	return u, nil
}

// LiveDelivery implements common assertions, not another task state machine.
// Each top-level test supplies its own ID, chain and isolated environment.
func LiveDelivery(s *Scenario, chain string, restart bool) {
	p := s.NewLiveProject("live", chain)
	p.CreateTasks()
	p.StartService()
	p.Claim("a")
	if restart && chain == "orca-codex" {
		verifyContinuingWorkerServiceRecovery(s, p)
	}
	before := p.WaitPhase("waiting_pr_merge")
	p.Record("open", before)
	if before.Helper.PR.State != "open" {
		s.t.Fatal("native worker did not publish fixture PR")
	}
	e := before.Domain.Executions[0]
	p.AssertStagedFlow(before)
	p.AssertFixedNativeLaunch(before)
	if e.Process == nil || e.Workspace.Worktree == "" || e.TurnNumber != 3 {
		s.t.Fatal("native execution identity/turn missing")
	}
	if chain == "orca-codex" {
		s.Must(evidence.OrcaExplicitEmptyArgs(before.Domain))
		s.Must(evidence.OrcaStagedContinuity(before.Domain))
	}
	for _, task := range before.Page.Tasks {
		if task.TaskID == p.Tasks["b"].ID && (task.DependencyReady || len(task.WorkflowRuns) > 0) {
			s.t.Fatal("successor released before merge")
		}
	}
	content := ""
	if p.RealPR {
		content = p.git(e.Workspace.Worktree, "show", before.Helper.PR.HeadRevision+":result.txt")
	} else {
		content = p.git(p.Repo, "--git-dir", p.Remote, "show", before.Helper.PR.HeadRevision+":result.txt")
	}
	if content != "real model delivery" {
		s.t.Fatalf("real native result differs: %q", content)
	}
	var usage native.Usage
	baselineTurn := ""
	if chain == "local-codex" {
		s.Wait("completed native usage baseline", func() (bool, error) {
			current, err := p.NativeUsage(before, "before")
			if baselineTurn != "" && current.Turn != baselineTurn {
				return false, errors.New("native turn changed while awaiting usage completion")
			}
			if current.Turn != "" {
				baselineTurn = current.Turn
			}
			if errors.Is(err, native.ErrNativeTurnPending) {
				return false, nil
			}
			usage = current
			return err == nil, err
		})
	}
	if restart {
		p.RestartService()
		after := p.WaitPhase("waiting_pr_merge")
		p.AssertFixedNativeLaunch(after)
		p.Record("after-restart", after)
		a := after.Domain.Executions[0]
		originalSession, bindErr := ExecutionSession(e)
		s.Must(bindErr)
		afterSession, bindErr := ExecutionSession(a)
		s.Must(bindErr)
		if before.Page.ProjectID != after.Page.ProjectID || before.Page.DatabaseID != after.Page.DatabaseID || e.ExecutionID != a.ExecutionID || e.WorkflowRunID != a.WorkflowRunID || originalSession != afterSession || e.Workspace.Worktree != a.Workspace.Worktree || e.CurrentTurnID != a.CurrentTurnID || e.TurnNumber != a.TurnNumber || len(before.Domain.Attempts) != len(after.Domain.Attempts) {
			s.t.Fatal("restart replaced execution identity or repeated turn")
		}
		if chain == "local-codex" {
			afterUsage, err := p.NativeUsage(after, "after-restart")
			s.Must(err)
			if usage != afterUsage {
				s.t.Fatal("restart consumed additional model tokens or replaced session")
			}
		}
	}
	p.Merge()
	done := p.WaitPhase("completed")
	p.AssertFixedNativeLaunch(done)
	p.Record("completed", done)
	for _, task := range done.Page.Tasks {
		if task.TaskID == p.Tasks["a"].ID && (task.Status != "closed" || task.Delivery == nil || task.Delivery.SelectedWorkflowRunID != e.WorkflowRunID) {
			s.t.Fatal("original native delivery missing")
		}
		if task.TaskID == p.Tasks["b"].ID && (!task.DependencyReady || task.Status != "open" || len(task.WorkflowRuns) != 0) {
			s.t.Fatal("successor not ready and unstarted")
		}
	}
	if chain == "local-codex" {
		finalUsage, err := p.NativeUsage(done, "final")
		s.Must(err)
		if usage != finalUsage {
			s.t.Fatal("delivery polling consumed additional tokens")
		}
	}
	if p.RealPR {
		if !done.Helper.PR.Merged || done.Helper.PR.State != "merged" {
			s.t.Fatal("real GitHub PR was not merged")
		}
		proof := done.Helper.PR
		p.RealPRProof = &proof
	}
	s.usageVerified = chain == "local-codex"
	s.orcaUsageUnavailable = chain == "orca-codex"
	s.Must(s.finishBody())
	p.StopService()
}

// The original worker remains inside its original native command while the
// center disconnects. Releasing an owned fixture file is not a new business turn.
func verifyContinuingWorkerServiceRecovery(s *Scenario, p *Project) {
	s.t.Helper()
	var original Snapshot
	s.Wait("original Orca business command reached its recovery window", func() (bool, error) {
		snap, err := p.Snapshot()
		if err != nil {
			return false, err
		}
		if len(snap.Domain.Executions) != 1 {
			return false, nil
		}
		e := snap.Domain.Executions[0]
		if e.Workspace.Worktree == "" || e.CurrentTurnID == "" || e.CurrentTurnID == e.LastStoppedTurnID {
			return false, nil
		}
		if _, err := os.Stat(filepath.Join(e.Workspace.Worktree, ".e2e-recovery-started.txt")); os.IsNotExist(err) {
			return false, nil
		} else if err != nil {
			return false, err
		}
		original = snap
		return true, nil
	})
	e := original.Domain.Executions[0]
	if original.Domain.WorkflowRuns[0].ExecutionProfile.ServiceStopBehavior != "continue_execution" {
		s.t.Fatal("Orca worker did not freeze its continuing service-stop behavior")
	}
	originalSession, err := ExecutionSession(e)
	s.Must(err)
	p.Record("original-active-before-stop", original)
	p.RestartService()
	var reopened Snapshot
	s.Wait("original active Orca execution reattached", func() (bool, error) {
		snap, err := p.Snapshot()
		if err != nil {
			return false, err
		}
		if len(snap.Domain.Executions) != 1 {
			return false, nil
		}
		current := snap.Domain.Executions[0]
		if current.RecoveryState != "running" && current.RecoveryState != "live" && current.RecoveryState != "question" {
			return false, nil
		}
		session, err := ExecutionSession(current)
		if err != nil {
			return false, err
		}
		if current.ExecutionID != e.ExecutionID || current.CurrentAttemptID != e.CurrentAttemptID || current.CurrentTurnID != e.CurrentTurnID || current.TurnNumber != e.TurnNumber || session != originalSession || current.Process == nil || e.Process == nil || *current.Process != *e.Process {
			return false, errors.New("service restart replaced the active original Orca worker")
		}
		reopened = snap
		return true, nil
	})
	p.Record("original-active-after-restart", reopened)
	p.StopService()
	s.Must(os.WriteFile(filepath.Join(e.Workspace.Worktree, ".e2e-recovery-release.txt"), []byte("release original execution\n"), 0600))
	// The real worker must produce its original output and PR while the service is
	// offline. The loopback provider receipt is an independent publication fact.
	s.Wait("original worker published while center offline", func() (bool, error) {
		provider, err := controlled.Read(p.Provider)
		if err != nil {
			return false, err
		}
		return provider.PR.URL != "" && provider.PR.State == "open", nil
	})
	p.StartService()
	after := p.WaitPhase("waiting_pr_merge")
	current := after.Domain.Executions[0]
	session, err := ExecutionSession(current)
	s.Must(err)
	if current.CurrentAttemptID != e.CurrentAttemptID || current.CurrentTurnID != e.CurrentTurnID || current.TurnNumber != e.TurnNumber || session != originalSession || current.LastStoppedTurnID != e.CurrentTurnID || len(after.Domain.Attempts) != len(original.Domain.Attempts) || current.RecoveryState != "completed" {
		s.t.Fatal("offline original completion was lost, duplicated, or left unverified")
	}
	p.Record("original-completed-after-offline-window", after)
}

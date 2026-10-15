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

	testprocess "github.com/erranliu/SteerLine/internal/testkit/process"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
	"github.com/erranliu/SteerLine/tests/e2e/internal/native"
)

func (s *Scenario) NewLiveProject(name, chain string) *Project {
	s.t.Helper()
	if chain != "local-codex" {
		s.t.Fatal("unregistered native chain; use Runner's explicit Orca target validation")
	}
	p := s.NewProject(name)
	p.Chain = chain
	p.Driver.NativeHome = s.Run.AuthHome
	if strings.HasPrefix(s.ID, "smoke.") || s.ID == "recovery.codex-resume-live" {
		p.enableGitHub()
	}
	codex, err := native.CodexExecutable("codex")
	s.Must(err)
	var config map[string]any
	raw, err := os.ReadFile(p.Config)
	s.Must(err)
	s.Must(json.Unmarshal(raw, &config))
	delete(config, "command_timeout")
	s.Must(controlled.Write(p.Config, config))
	var catalog map[string]any
	raw, err = os.ReadFile(filepath.Join(p.Home, "execution.json"))
	s.Must(err)
	s.Must(json.Unmarshal(raw, &catalog))
	catalog["default_execution_profile"] = chain
	catalog["execution_profiles"] = []any{map[string]any{"id": chain, "name": "Real E2E " + chain, "execution": map[string]any{"adapter": "codex-app-server", "target": map[string]string{"kind": "local"}, "settings": map[string]string{"executable": codex, "model": native.Model, "reasoning_effort": native.Effort}}, "workflow": map[string]any{"id": "development", "version": 1}}}
	s.Must(controlled.Write(filepath.Join(p.Home, "execution.json"), catalog))
	p.Driver.Profile = chain

	s.Must(os.WriteFile(filepath.Join(p.Repo, "AGENTS.md"), []byte("# Isolated SteerLine E2E fixture\n\nFor a readiness-only Dispatch, run only its exact probe command and report the result; do not inspect business task references. Preserve the COMPLETE native command tool result, including its exit_code and output. If using the exec JavaScript wrapper, use exactly `const r = await tools.exec_command({cmd: <exact probe command>, login: false}); text(r);`. Do not project r.output, stringify individual fields, add exit= text, or transform the result: the readiness verifier consumes the full native JSON result. For business work, only implement the bound task. Invoke shell tools with login=false to disable unrelated user shell profiles. Initially HEAD equals the reference commit: confirm with git rev-parse HEAD, then use Get-FileHash directly on docs/spec/task.spec.md and task.plan.md. Read those original files. Do not reconstruct files from git show text or PowerShell pipelines: newline conversion changes their hashes. Any scratch work must stay inside this assigned worktree, and must not be staged. No questions or other tasks are needed: the fixture contract specifies the entire output and publication action.\n"), 0600))
	spec := "# Live delivery fixture\n\nCreate result.txt containing exactly `real model delivery` followed by a newline. Verify the file. Stage only result.txt. Do not modify any other tracked file.\n\n"
	if p.RealPR {
		script := fmt.Sprintf(`$ErrorActionPreference = 'Stop'
if ((Get-Content -LiteralPath 'result.txt' -Raw).Trim() -cne 'real model delivery') { throw 'result differs' }
$branch = git symbolic-ref --short HEAD
git push origin ('HEAD:refs/heads/' + $branch)
if ($LASTEXITCODE -ne 0) { throw 'push failed' }
gh pr create --repo '%s' --base '%s' --head $branch --draft --title 'SteerLine E2E %s' --body 'Isolated native gpt-6-luna medium delivery test. Disposable test branches.'
if ($LASTEXITCODE -ne 0) { throw 'PR create failed' }
`, githubRepository, p.BaseBranch, s.ID)
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

	if s.ID == "recovery.codex-resume-live" || s.ID == "recovery.orca-local-reconnect-live" {
		spec += "\n## Original-execution recovery window\n\nBefore writing result.txt or publishing, run one shell command that writes `.e2e-recovery-started.txt` in this assigned worktree, waits until `.e2e-recovery-release.txt` exists, then removes both files. Use `Set-Content -LiteralPath .e2e-recovery-started.txt -Value waiting; while (-not (Test-Path -LiteralPath .e2e-recovery-release.txt)) { Start-Sleep -Milliseconds 250 }; Remove-Item -LiteralPath .e2e-recovery-started.txt,.e2e-recovery-release.txt`. The test driver releases the existing command; it does not send another prompt. These two files are temporary fixture controls: do not stage or commit them. After release, finish only implement.execute in this original round. Later report and publish are separate authorized stages.\n"
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
	var empty native.Usage
	if len(snap.Runner.Candidates) != 1 || snap.Runner.Candidates[0].Projection == nil {
		return empty, errors.New("usage needs one public original Run")
	}
	candidate := snap.Runner.Candidates[0]
	run := candidate.Projection
	e := run.Execution
	if e.Backend != "local" || e.SessionID == "" || e.TurnID == "" {
		return empty, errors.New("selected native usage identity is unavailable")
	}
	var input struct {
		Execution struct {
			Settings struct {
				Model  string `json:"model"`
				Effort string `json:"reasoning_effort"`
			} `json:"settings"`
		} `json:"execution"`
	}
	if json.Unmarshal(candidate.Submit.Input, &input) != nil || input.Execution.Settings.Model != native.Model || input.Execution.Settings.Effort != native.Effort {
		return empty, errors.New("frozen public model settings differ")
	}
	var result native.Usage
	var proof []byte
	found := false
	err := filepath.WalkDir(filepath.Join(p.Scenario.Run.AuthHome, "sessions"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.Contains(entry.Name(), e.SessionID) || filepath.Ext(path) != ".jsonl" {
			return nil
		}
		if found {
			return errors.New("ambiguous original native rollout")
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Size() > 32<<20 {
			return errors.New("native rollout exceeds bounded read")
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		matched, err := native.BusinessRolloutMatches(raw, e.Workspace)
		if err != nil {
			return err
		}
		if !matched {
			return errors.New("native rollout belongs to another root worktree")
		}
		result, proof, err = native.RolloutTurnUsage(raw, e.Workspace, e.TurnID)
		if err != nil {
			return err
		}
		if result.Session != e.SessionID {
			return errors.New("usage belongs to another original session")
		}
		found = true
		return nil
	})
	if err != nil {
		return result, err
	}
	if !found {
		return result, errors.New("original native rollout not available")
	}
	if err = os.WriteFile(filepath.Join(p.Scenario.Output, p.Name+"-"+label+"-usage.jsonl"), proof, 0600); err != nil {
		return result, err
	}
	return result, controlled.Write(filepath.Join(p.Scenario.Output, p.Name+"-"+label+"-usage.json"), result)
}

// LiveDelivery consumes only public Run/PR facts and the native tool's own rollout.
func LiveDelivery(s *Scenario, chain string, restart bool) {
	p := s.NewLiveProject("live", chain)
	p.CreateTasks()
	p.StartService()
	p.Claim("a")
	before := p.WaitPhase("waiting_pr_merge")
	p.AssertRunnerBinding(before)
	p.AssertStagedFlow(before)
	p.Record("open", before)
	first := before.Runner.Candidates[0]
	e := first.Projection.Execution
	if before.Helper.PR.State != "open" {
		s.t.Fatal("original native Run did not publish a verified PR")
	}
	if got := p.git(e.Workspace, "show", before.Helper.PR.HeadRevision+":result.txt"); got != "real model delivery" {
		s.t.Fatal("native committed result differs", got)
	}
	usage, err := p.NativeUsage(before, "before")
	s.Must(err)
	if restart {
		p.RestartService()
		after := p.WaitPhase("waiting_pr_merge")
		p.AssertRunnerBinding(after)
		last := after.Runner.Candidates[0]
		x := last.Projection.Execution
		if first.ID != last.ID || first.RunID != last.RunID || e.SessionID != x.SessionID || e.AttemptID != x.AttemptID || e.TurnID != x.TurnID || e.Workspace != x.Workspace {
			s.t.Fatal("restart replaced original Run or issued another turn")
		}
		afterUsage, err := p.NativeUsage(after, "after-restart")
		s.Must(err)
		if usage != afterUsage {
			s.t.Fatal("read-only restart consumed model tokens")
		}
	}
	p.Merge()
	done := p.WaitPhase("completed")
	p.AssertOneDelivery(done)
	p.Record("completed", done)
	if done.Runner.Graph.Deliveries[p.Tasks["a"].ID].SelectedWorkflowRunID != first.ID {
		s.t.Fatal("delivery lost the original public candidate")
	}
	finalUsage, err := p.NativeUsage(done, "final")
	s.Must(err)
	if usage != finalUsage {
		s.t.Fatal("delivery polling changed original usage")
	}
	if p.RealPR {
		proof := done.Helper.PR
		if !proof.Merged {
			s.t.Fatal("actual GitHub merge is missing")
		}
		p.RealPRProof = &proof
	}
	s.usageVerified = true
	s.Must(s.FinishBody())
	p.StopService()
}

package testenv

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/erranliu/SteerLine/internal/runnerclient"
	testfeedback "github.com/erranliu/SteerLine/internal/testkit/feedback"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
	"github.com/erranliu/SteerLine/tests/e2e/internal/driver"
)

func (s *Scenario) NewLiveProject(name, chain string) *Project {
	s.t.Helper()
	if chain != "local-codex" {
		s.t.Fatal("unregistered native chain; use Runner's explicit Orca target validation")
	}
	if s.ID == "workflow.local-codex-audited" && (s.Run.ExecutionModel == "" || s.Run.AuditModel == "" || s.Run.ExecutionModel == s.Run.AuditModel) {
		s.t.Fatal("audited models must be explicitly distinct before creating fixtures")
	}
	p := s.NewProject(name)
	p.Chain = chain
	p.Driver.NativeHome = s.Run.AuthHome
	if s.ID == "smoke.local-codex" || s.ID == "workflow.local-codex-audited" {
		p.enableGitHub()
	}
	codex, err := driver.CodexExecutable("codex")
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
	catalog["execution_profiles"] = []any{map[string]any{"id": chain, "name": "Real E2E " + chain, "execution": map[string]any{"adapter": "codex-app-server", "target": map[string]string{"kind": "local"}, "settings": map[string]string{"executable": codex, "model": "gpt-6-luna", "reasoning_effort": "medium"}}, "workflow": map[string]any{"id": "development", "version": 2}}}
	if s.ID == "workflow.local-codex-audited" {
		profile := catalog["execution_profiles"].([]any)[0].(map[string]any)
		profile["name"] = "Real E2E audited preset"
		profile["execution"].(map[string]any)["settings"].(map[string]string)["model"] = s.Run.ExecutionModel
		profile["workflow"] = map[string]any{"id": "development-audited", "version": 2, "audit_model": map[string]string{"model": s.Run.AuditModel, "reasoning_effort": "high"}}
		p.AuditObservations = map[string]string{}
	}
	s.Must(controlled.Write(filepath.Join(p.Home, "execution.json"), catalog))
	selection, err := driver.RunnerSelection(s.Run.Runner)
	s.Must(err)
	catalog["runner"] = selection
	s.Must(controlled.Write(filepath.Join(p.Home, "execution.json"), catalog))
	p.Driver.Profile = chain

	s.Must(os.WriteFile(filepath.Join(p.Repo, "AGENTS.md"), []byte("# Isolated SteerLine E2E fixture\n\nFor a readiness-only Dispatch, run only its exact probe command and report the result; do not inspect business task references. Preserve the COMPLETE native command tool result, including its exit_code and output. If using the exec JavaScript wrapper, use exactly `const r = await tools.exec_command({cmd: <exact probe command>, login: false}); text(r);`. Do not project r.output, stringify individual fields, add exit= text, or transform the result: the readiness verifier consumes the full native JSON result. For business work, only implement the bound task. Invoke shell tools with login=false to disable unrelated user shell profiles. Initially HEAD equals the reference commit: confirm with git rev-parse HEAD, then use Get-FileHash directly on SteerlineFlow/spec/task.spec.md and SteerlineFlow/plans/task.plan.md. Read those original files. Do not reconstruct files from git show text or PowerShell pipelines: newline conversion changes their hashes. Any scratch work must stay inside this assigned worktree, and must not be staged. No questions or other tasks are needed: the fixture contract specifies the entire output and publication action.\n"), 0600))
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
	if s.ID == "workflow.local-codex-audited" {
		// Keep the business acceptance independent of workflow history. Stage
		// instructions belong to Runner's published prompt-override contract.
		spec = "# Audited delivery fixture\n\nCreate result.txt containing exactly `real model delivery` followed by a newline. Verify and commit only result.txt. Do not change other tracked files.\n"
		profile := catalog["execution_profiles"].([]any)[0].(map[string]any)
		profile["workflow"].(map[string]any)["prompt_overrides"] = map[string]string{
			"implement.execute": "Implement the bound Spec by running powershell.exe -NoProfile -ExecutionPolicy Bypass -File ./implement.ps1 exactly once. It writes, verifies and commits only result.txt locally. This step ends after that command. Do not run publish.ps1, push, create a PR or use gh in this step. Give a short factual implementation summary; the Runner will request reporting, audits and publication in separate later turns.",
			"publish":           "Publish the already committed and audited candidate by running powershell.exe -NoProfile -ExecutionPolicy Bypass -File ./publish.ps1 exactly once. Do not implement again. Copy the exact returned https://github.com/erranliu/SteerlineTest/pull/<number> URL into the Runner publication result contract, preserving the repository owner. Return strict publication result JSON in the same final reply.",
		}
		s.Must(controlled.Write(filepath.Join(p.Home, "execution.json"), catalog))
	}

	spec += "\n## Acceptance\n\nThe committed result.txt equals the specified content and is delivered after the fixture PR merges.\n"
	s.Must(os.WriteFile(filepath.Join(p.Repo, "SteerlineFlow", "spec", "task.spec.md"), []byte(spec), 0600))
	s.Must(os.WriteFile(filepath.Join(p.Repo, "SteerlineFlow", "plans", "task.plan.md"), []byte("# Live fixture plan\n\nRead the bound Spec. In implement.execute run implement.ps1 to write, verify and commit the local candidate. In implement.report summarize only existing supplied evidence. In publish run publish.ps1 to push the working branch and create or update its PR; return strict result JSON in the same native final. Do not combine the steps.\n\n## Test plan\n\nVerify result.txt locally; the E2E harness checks public stage results, real Git delivery and dependency state.\n"), 0600))
	if s.ID == "workflow.local-codex-audited" {
		s.Must(os.WriteFile(filepath.Join(p.Repo, "SteerlineFlow", "plans", "task.plan.md"), []byte("# Audited fixture plan\n\nRead the bound Spec. Use implement.ps1 to write, verify and commit result.txt. The Runner owns workflow sequencing, report generation, audit and publication. Do not edit fixture scripts.\n\n## Test plan\n\nVerify the committed result.txt matches the exact requested content and newline; no additional business implementation is required.\n"), 0600))
	}
	planPath := filepath.Join(p.Repo, "SteerlineFlow", "plans", "task.plan.md")
	planBody, readErr := os.ReadFile(planPath)
	s.Must(readErr)
	s.Must(os.WriteFile(planPath, append(planBody, []byte("\nNo automated tests are required for this fixture. The committed test catalogue is {\"tests\":[]}; this fixture's automatic arrangement must also be {\"tests\":[]}. Verifying the repository-root result.txt is a manual implementation check, not a registered test ID. Local checks and external harness acceptance remain required.\n")...), 0600))
	arrangement, readErr := os.ReadFile(filepath.Join(s.Run.Root, "tests", "e2e", "testdata", "tasks", "task.tests.json"))
	s.Must(readErr)
	s.Must(os.WriteFile(filepath.Join(p.Repo, "SteerlineFlow", "plans", "task.tests.json"), arrangement, 0600))
	p.prepareTestProtocol()
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

// LiveDelivery exercises the real public product-to-Runner chain and GitHub.
func LiveDelivery(s *Scenario, chain string) {
	p := s.NewLiveProject("live", chain)
	p.CreateTasks()
	p.StartService()
	p.Claim("a")
	open := p.WaitPhase("waiting_pr_merge")
	p.AssertRunnerBinding(open)
	if s.ID == "workflow.local-codex-audited" {
		p.observeAuditedProduct(open.Runner.Candidates[0], true)
	} else {
		p.AssertStagedFlow(open)
	}
	p.Record("open", open)
	c := open.Runner.Candidates[0]
	pr := open.Helper.PR
	if pr.State != "open" || c.Projection.PR == nil || c.Projection.PR.HeadRevision != pr.HeadRevision {
		s.t.Fatal("original live PR binding missing")
	}
	// Fetch public Git objects; no Runner-owned workspace or rollout is read.
	p.git(p.Repo, "fetch", "origin", pr.HeadBranch)
	artifact := p.git(p.Repo, "show", pr.HeadRevision+":result.txt")
	if artifact != "real model delivery" {
		s.t.Fatal("committed artifact differs", artifact)
	}
	before := string(mustJSON(s, p.RunnerSubmit()))
	p.Merge()
	done := p.WaitPhase("completed")
	p.AssertOneDelivery(done)
	p.Record("completed", done)
	if before != string(mustJSON(s, p.RunnerSubmit())) {
		s.t.Fatal("delivery replaced original submission")
	}
	facts := done.Helper.PR
	var selection struct {
		Runner runnerclient.Selection `json:"runner"`
	}
	raw, err := os.ReadFile(filepath.Join(p.Home, "execution.json"))
	s.Must(err)
	s.Must(json.Unmarshal(raw, &selection))
	proof := &testfeedback.LiveDeliveryProof{RunnerVersion: selection.Runner.Version, RunnerSHA256: selection.Runner.SHA256, RunID: c.RunID, CandidateID: c.ID, FinalSources: sources(s, p, c.ID, c.RunID), ArtifactCommit: pr.HeadRevision, Artifact: artifact, PRURL: facts.URL, Repository: facts.Repository, Base: facts.BaseBranch, Head: facts.HeadBranch, HeadSHA: facts.HeadRevision, Merged: facts.Merged, DeliveryCount: len(done.Runner.Graph.Deliveries)}
	if s.ID == "workflow.local-codex-audited" {
		proof.Audited = &testfeedback.AuditedWorkflowProof{ExecutionModel: s.Run.ExecutionModel, AuditModel: s.Run.AuditModel, Stages: p.AuditObservations}
	}
	if facts.MergeCommit != nil {
		proof.MergeSHA = *facts.MergeCommit
	}
	d := done.Runner.Graph.Deliveries[p.Tasks["a"].ID]
	proof.DeliveryCandidate = d.SelectedWorkflowRunID
	for _, task := range done.Page.Tasks {
		if task.TaskID == p.Tasks["a"].ID && task.Delivery != nil {
			proof.DeliveryPR = task.Delivery.PRURL
		}
		if task.TaskID == p.Tasks["b"].ID {
			proof.SuccessorReady = task.DependencyReady
			proof.SuccessorUnstarted = len(task.Candidates) == 0 && task.Status == "open"
		}
	}
	s.Must(proof.Validate())
	s.LiveProof = proof
	p.RealPRProof = &facts
	s.Must(s.FinishBody())
	p.WaitStoppedCandidates()
	p.StopService()
}
func mustJSON(s *Scenario, v any) []byte { b, err := json.Marshal(v); s.Must(err); return b }
func (p *Project) RunnerSubmit() any {
	snap, err := p.Snapshot()
	p.Scenario.Must(err)
	return snap.Runner.Candidates[0].Submit
}
func sources(s *Scenario, p *Project, candidate, run string) []testfeedback.FinalSource {
	conversation, err := driver.ReadCandidateConversation(s.Context, p.Database, p.ProjectID, candidate, run)
	s.Must(err)
	var result []testfeedback.FinalSource
	for _, e := range conversation.Events {
		if e.Body != nil && e.SourceRole == "canonical" && e.Phase == "final_answer" {
			result = append(result, testfeedback.FinalSource{RunID: e.ExecutionID, CandidateID: e.WorkflowID, Key: e.SessionID + ":" + e.TurnID + ":" + e.ItemKey})
		}
	}
	return result
}

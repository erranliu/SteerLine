package testenv

import (
	"encoding/json"
	"fmt"
	"github.com/erranliu/SteerLine/internal/product/applicationclient"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	"github.com/google/uuid"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Extend the existing live delivery fixture; the host prepares only its owned base.
func (p *Project) ResolveOriginalPRConflict(original Snapshot) Snapshot {
	s := p.Scenario
	first := original.Runner.Candidates[0]
	p.git(p.Repo, "fetch", "origin", original.Helper.PR.HeadBranch)
	if flags := p.git(p.Repo, "show", original.Helper.PR.HeadRevision+":conflict.txt"); flags != "base=0 candidate=1" {
		s.t.Fatal("original candidate did not create the conflicting change", flags)
	}
	s.Must(os.WriteFile(filepath.Join(p.Repo, "conflict.txt"), []byte("base=1 candidate=0\n"), 0600))
	p.git(p.Repo, "add", "conflict.txt")
	p.git(p.Repo, "commit", "--quiet", "-m", "Advance owned base flag")
	base := p.git(p.Repo, "rev-parse", "HEAD")
	p.git(p.Repo, "push", "--quiet", "origin", p.BaseBranch)
	merge := exec.CommandContext(s.Context, "git", "merge-tree", "--write-tree", base, original.Helper.PR.HeadRevision)
	merge.Dir, merge.Env = p.Repo, p.Driver.Environment(os.Environ())
	proof, mergeErr := merge.CombinedOutput()
	failure, ok := mergeErr.(*exec.ExitError)
	if !ok || failure.ExitCode() != 1 || !strings.Contains(string(proof), "CONFLICT") || !strings.Contains(string(proof), "conflict.txt") {
		s.t.Fatal("fixture did not produce a real Git conflict", mergeErr, string(proof))
	}
	s.Must(os.WriteFile(filepath.Join(s.Output, "conflict-proof.txt"), proof, 0600))
	project, err := applicationclient.OpenProject(s.Context, p.Home, filepath.Base(p.Config))
	s.Must(err)
	defer project.Close()
	client, err := applicationclient.Open(s.Context, p.Home)
	s.Must(err)
	defer client.Close()
	identity := project.Identity()
	result := original
	for round := 0; round < 2; round++ {
		current, err := p.Snapshot()
		s.Must(err)
		c := current.Runner.Candidates[0]
		if c.Projection == nil || !c.Projection.Controls.CanMessage {
			s.t.Fatal("original Run does not advertise message")
		}
		requestID := strings.ReplaceAll(uuid.NewString(), "-", "")
		payload, _ := json.Marshal(map[string]string{"task_id": c.TaskID})
		request := httpcontract.CommandEnvelope{RequestID: requestID, ProjectID: identity.ProjectID, InstanceID: identity.InstanceID, Action: "resolve_pr_conflict", TargetID: c.ID, ExpectedVersion: c.Revision, ExpectedControlRevision: current.Runtime.Control.Revision, Payload: payload}
		var response httpcontract.CommandResponse
		if err := client.Post(s.Context, "/projects/"+string(identity.ProjectID)+"/commands", request, &response); err != nil {
			s.Must(client.Get(s.Context, "/projects/"+string(identity.ProjectID)+"/commands/"+requestID, &response))
		}
		if response.Status == "rejected" {
			s.t.Fatal("conflict command rejected", response.Diagnostic)
		}
		previousTurn := c.Projection.Execution.TurnID
		s.Wait(fmt.Sprintf("original conflict message %d", round+1), func() (bool, error) {
			next, err := p.Snapshot()
			if err != nil {
				return false, err
			}
			candidate := next.Runner.Candidates[0]
			r := candidate.Projection
			if r == nil {
				return false, nil
			}
			if r.Status == "blocked" || r.Status == "failed" || r.Question != nil {
				s.t.Fatalf("conflict Agent requires attention: %+v", r)
			}
			if candidate.RunID != first.RunID || r.Execution.SessionID != first.Projection.Execution.SessionID || r.Execution.Workspace != first.Projection.Execution.Workspace || r.Execution.Branch != first.Projection.Execution.Branch {
				s.t.Fatal("conflict replaced original execution")
			}
			for _, action := range candidate.Actions {
				if action.CallerRequestID == requestID && action.State == "rejected" {
					s.t.Fatal("message rejected", action.ErrorCode)
				}
				if action.CallerRequestID == requestID && action.State == "accepted" && r.Controls.CanMessage && r.Final != nil && r.Final.TurnID != previousTurn && r.Final.Verified && r.Final.Text != "" {
					result = next
					return true, nil
				}
			}
			return false, nil
		})
		if result.Helper.PR.URL != original.Helper.PR.URL || result.Helper.PR.Merged {
			s.t.Fatal("conflict replaced or merged PR")
		}
		p.Record(fmt.Sprintf("conflict-round-%d", round+1), result)
	}
	pr := result.Helper.PR
	p.git(p.Repo, "fetch", "origin", pr.HeadBranch)
	p.git(p.Repo, "merge-base", "--is-ancestor", base, pr.HeadRevision)
	if flags := p.git(p.Repo, "show", pr.HeadRevision+":conflict.txt"); flags != "base=1 candidate=1" {
		s.t.Fatal("conflict lost branch intent", flags)
	}
	return result
}

// The fixture declares its actual Git/gh prerequisites and uses the existing public adaptation path.
func (p *Project) writeLiveFixtureEnvironment() {
	s := p.Scenario
	files := map[string]string{
		"SteerlineFlow/environment.md": "# Live fixture tools\n\nGit and gh must be available on PATH.\n\n```steerline-environment\n{\"version\":5,\"script\":\"SteerlineFlow/scripts/verify-environment.ps1\",\"interpreter\":\"powershell.exe\",\"dependencies\":[]}\n```\n",
		"SteerlineFlow/scripts/verify-environment.ps1": `$ErrorActionPreference = 'Stop'
$utf8 = New-Object System.Text.UTF8Encoding($false)
$path = $env:STEERLINE_RESULT_FILE
$report = [IO.File]::ReadAllText($path, $utf8) | ConvertFrom-Json
if ($report.version -ne 3) { throw 'Unsupported result contract' }
$missing = @('git.exe', 'gh.exe' | Where-Object { -not (Get-Command $_ -CommandType Application -ErrorAction SilentlyContinue) })
$report.completed = $true
$report.ready = ($missing.Count -eq 0)
$report.message = if ($report.ready) { 'Git and gh are available.' } else { 'Missing tools: ' + ($missing -join ', ') }
$temporary = $path + '.' + [guid]::NewGuid().ToString('N') + '.tmp'
[IO.File]::WriteAllText($temporary, ($report | ConvertTo-Json -Depth 32), $utf8)
[IO.File]::Replace($temporary, $path, [System.Management.Automation.Language.NullString]::Value)
`,
	}
	for path, body := range files {
		target := filepath.Join(p.Repo, filepath.FromSlash(path))
		s.Must(os.MkdirAll(filepath.Dir(target), 0700))
		s.Must(os.WriteFile(target, []byte(body), 0600))
	}
}

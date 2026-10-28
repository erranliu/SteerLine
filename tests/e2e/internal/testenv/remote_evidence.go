//go:build e2e

package testenv

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	app "github.com/erranliu/SteerLine/internal/app"
	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
	"github.com/erranliu/SteerLine/tests/e2e/internal/driver"
)

// Rules live beside the project script, not in a second product PR config.
type remotePRRules struct {
	RequiredChecks   []string `json:"required_checks"`
	MinimumApprovals int      `json:"minimum_approvals"`
	L2Summary        string   `json:"l2_summary"`
}

func (p *Project) environmentRules() remotePRRules {
	s := p.Scenario
	doc, err := os.ReadFile(filepath.Join(p.Repo, env.DocumentPath))
	s.Must(err)
	parts := regexp.MustCompile("(?s)```steerline-pr-acceptance[\\t ]*\\r?\\n(.*?)\\r?\\n```").FindAllSubmatch(doc, -1)
	if len(parts) != 1 {
		s.t.Fatal("Git environment document must declare one steerline-pr-acceptance block (required_checks/minimum_approvals/l2_summary)")
	}
	var rules remotePRRules
	s.Must(env.Decode(parts[0][1], &rules))
	clean := filepath.Clean(rules.L2Summary)
	if rules.RequiredChecks == nil || rules.MinimumApprovals < 0 || clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		s.t.Fatal("invalid project PR/L2 acceptance rules")
	}
	return rules
}

// This is read-only HTTP evidence. It cannot create, approve or merge a PR.
func (p *Project) providerRead(config app.ProjectConfigInput, path string, target any) error {
	if config.PR.BaseURL == "https://github.com" {
		raw, err := p.gh(p.Scenario.Context, "api", path)
		if err != nil {
			return err
		}
		return json.Unmarshal(raw, target)
	}
	ctx, cancel := context.WithTimeout(p.Scenario.Context, 30*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(config.PR.BaseURL, "/")+"/api/v1/"+path, nil)
	if err != nil {
		return err
	}
	if token := os.Getenv(config.PR.TokenEnv); token != "" {
		request.Header.Set("Authorization", "token "+token)
	}
	client := &http.Client{Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("provider redirect refused") }}
	defer client.CloseIdleConnections()
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return fmt.Errorf("provider evidence GET returned %d", response.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, target)
}

func (p *Project) verifyEnvironmentEvidence(record env.Record) {
	s := p.Scenario
	rules := p.environmentRules()
	last := record.Rounds[len(record.Rounds)-1]
	var config app.ProjectConfigInput
	raw, err := os.ReadFile(p.Config)
	s.Must(err)
	s.Must(json.Unmarshal(raw, &config))
	number, err := strconv.Atoi(filepath.Base(last.Report.PR.URL))
	s.Must(err)
	root := "repos/" + config.PR.Repository
	var pr struct {
		Merged bool   `json:"merged"`
		Merge  string `json:"merge_commit_sha"`
		Base   struct {
			Ref string `json:"ref"`
		} `json:"base"`
		Head struct {
			Ref string `json:"ref"`
			SHA string `json:"sha"`
		} `json:"head"`
		User struct {
			Login string `json:"login"`
		} `json:"user"`
	}
	s.Must(p.providerRead(config, root+"/pulls/"+strconv.Itoa(number), &pr))
	if !pr.Merged || pr.Merge != last.Report.PR.MergeCommit || pr.Base.Ref != record.Prepared.Manifest.ValidationBase || pr.Head.Ref != last.Report.PR.Head || pr.Head.SHA == "" {
		s.t.Fatal("real provider PR differs from sealed script report")
	}
	// Explicitly verify the original branch's rules; a new base inherits nothing.
	sourceBranch := "main"
	if s.Run.Remote.ProjectConfig != nil {
		sourceBranch = "AIRefactor"
	}
	if config.PR.BaseURL == "https://github.com" {
		var branch struct {
			Protected bool `json:"protected"`
		}
		s.Must(p.providerRead(config, root+"/branches/"+url.PathEscape(sourceBranch), &branch))
		if branch.Protected {
			var policy struct {
				RequiredStatusChecks struct {
					Contexts []string `json:"contexts"`
				} `json:"required_status_checks"`
				RequiredPullRequestReviews struct {
					Count int `json:"required_approving_review_count"`
				} `json:"required_pull_request_reviews"`
			}
			s.Must(p.providerRead(config, root+"/branches/"+url.PathEscape(sourceBranch)+"/protection", &policy))
			if policy.RequiredPullRequestReviews.Count > rules.MinimumApprovals {
				s.t.Fatal("project rules omit required approvals")
			}
			for _, name := range policy.RequiredStatusChecks.Contexts {
				found := false
				for _, declared := range rules.RequiredChecks {
					found = found || name == declared
				}
				if !found {
					s.t.Fatal("project rules omit protected check", name)
				}
			}
		}
	} else {
		var policies []struct {
			Rule      string   `json:"rule_name"`
			Branch    string   `json:"branch_name"`
			Checks    []string `json:"status_check_contexts"`
			Approvals int      `json:"required_approvals"`
		}
		s.Must(p.providerRead(config, root+"/branch_protections?limit=100", &policies))
		if len(policies) >= 100 {
			s.t.Fatal("branch policy evidence may be truncated")
		}
		for _, policy := range policies {
			pattern := policy.Rule
			if pattern == "" {
				pattern = policy.Branch
			}
			match, e := filepath.Match(pattern, sourceBranch)
			s.Must(e)
			if !match {
				continue
			}
			if policy.Approvals > rules.MinimumApprovals {
				s.t.Fatal("project rules omit required approvals")
			}
			for _, name := range policy.Checks {
				found := false
				for _, declared := range rules.RequiredChecks {
					found = found || name == declared
				}
				if !found {
					s.t.Fatal("project rules omit protected check", name)
				}
			}
		}
	}
	if len(rules.RequiredChecks) > 0 {
		var status struct {
			Statuses []struct {
				Context string `json:"context"`
				State   string `json:"state"`
				Status  string `json:"status"`
			} `json:"statuses"`
		}
		s.Must(p.providerRead(config, root+"/commits/"+pr.Head.SHA+"/status", &status))
		for _, required := range rules.RequiredChecks {
			passed := false
			for _, st := range status.Statuses {
				if st.Context == required {
					passed = st.State == "success" || st.Status == "success"
					break
				}
			}
			if !passed {
				s.t.Fatal("required PR check not passed", required)
			}
		}
	}
	if rules.MinimumApprovals > 0 {
		var reviews []struct {
			State     string `json:"state"`
			Dismissed bool   `json:"dismissed"`
			User      struct {
				Login string `json:"login"`
			} `json:"user"`
		}
		s.Must(p.providerRead(config, root+"/pulls/"+strconv.Itoa(number)+"/reviews?limit=100&per_page=100", &reviews))
		if len(reviews) >= 100 {
			s.t.Fatal("PR review evidence may be truncated")
		}
		latest := map[string]bool{}
		for _, review := range reviews {
			if review.User.Login != "" && review.User.Login != pr.User.Login {
				switch strings.ToUpper(review.State) {
				case "APPROVED":
					latest[review.User.Login] = !review.Dismissed
				case "CHANGES_REQUESTED", "REQUEST_CHANGES", "DISMISSED":
					latest[review.User.Login] = false
				}
			}
		}
		count := 0
		for _, approved := range latest {
			if approved {
				count++
			}
		}
		if count < rules.MinimumApprovals {
			s.t.Fatal("required independent PR approvals missing")
		}
	}
	r := loadProgressRemote(s, p)
	script := `$path=Join-Path (Get-Location) '` + strings.ReplaceAll(rules.L2Summary, "'", "''") + `';$text=[IO.File]::ReadAllText($path);if($text.Length -gt 65536){throw 'L2 summary exceeds evidence bound'};$summary=$text|ConvertFrom-Json;`
	validators := []string{"SteerlineFlow/testing/test-list.json", "tools/test-project.py", "tools/project_checks.py", "tools/steerline_reports.py"}
	if s.ID == "testing.project-environment-orca" {
		validators = []string{"SteerlineFlow/testing/test-list.json", "tools/run-tests.ps1", "tools/testing/ExecutionPlan.ps1", "tools/testing/ManagedReports.ps1"}
	}
	for _, path := range validators {
		blob := p.git(p.Repo, "rev-parse", record.Prepared.Commit+":"+path)
		script += "$blob=(& git hash-object --path " + psProgressLiteral(path) + " " + psProgressLiteral(path) + "|Out-String).Trim();if($LASTEXITCODE -or $blob -cne " + psProgressLiteral(blob) + "){throw 'original L2 validator changed'};"
	}
	if s.ID == "testing.project-environment-orca" {
		script += `. ./tools/testing/ExecutionPlan.ps1;. ./tools/testing/ManagedReports.ps1;$output=Split-Path $path -Parent;$plan=Get-Content -Raw -LiteralPath (Join-Path $output 'plan.json')|ConvertFrom-Json;if($plan.level -cne 'L2' -or $summary.level -cne 'L2' -or $summary.result -cne 'Passed' -or $plan.shardCount -ne 1 -or $plan.selection.caseId -or $plan.selection.group -or $plan.selection.tag){throw 'not complete project L2'};$definitions=@((Get-Content -Raw ./SteerlineFlow/testing/test-list.json|ConvertFrom-Json).tests|Where-Object level -CEQ 'L2');$selected=@{};foreach($d in $definitions){$selected[$d.id]=$d};$verified=@(Get-ManagedArchitectureResults @{selected=$selected;output=$output} $summary $plan L2);if(!$verified.Count -or $verified.Count -ne $definitions.Count -or @($verified|Where-Object status -CNE 'passed').Count){throw 'original XML/input/cleanup evidence failed'};@{passed=$true;cleanup_complete=$true;level='L2';original_summary=$summary;original_summary_sha256=(Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash;selected_ids=@($definitions.id);project_verification=@($verified|ForEach-Object {@{test_id=$_.id;status=$_.status}})}`
	} else {
		script += `$definitions=@((Get-Content -Raw ./SteerlineFlow/testing/test-list.json|ConvertFrom-Json).tests|Where-Object level -CEQ 'L2');$verified=@($summary.results);if(!$summary.passed -or !$summary.complete -or $summary.exit_code -ne 0 -or !$summary.before_clean -or !$summary.after_clean -or $verified.Count -ne $definitions.Count -or @($summary.details.skipped).Count -or $summary.details.tests_run -le 0){throw 'not complete original project L2'};foreach($d in $definitions){$matches=@($verified|Where-Object test_id -CEQ $d.id);if($matches.Count -ne 1 -or $matches[0].status -cne 'passed' -or $d.id -cnotin @($summary.selection)){throw 'original L2 item not passed'}};@{passed=$true;cleanup_complete=$true;level='L2';original_summary=$summary;original_summary_sha256=(Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash;selected_ids=@($definitions.id);project_verification=$verified}`
	}
	summary, err := r.Shell(s.Context, s, p, last.Run.Execution.Workspace, script)
	s.Must(err)
	// The untouched original summary is retained for project-specific checks.
	var l2 struct {
		Passed  bool   `json:"passed"`
		Cleanup bool   `json:"cleanup_complete"`
		Level   string `json:"level"`
		Cases   []struct {
			Status string `json:"status"`
		} `json:"cases"`
	}
	s.Must(json.Unmarshal(summary, &l2))
	if !l2.Passed || !l2.Cleanup || l2.Level != "L2" {
		s.t.Fatal("original complete L2 summary did not pass/clean up")
	}
	s.Must(controlled.Write(filepath.Join(s.Output, "original-l2-summary.json"), json.RawMessage(summary)))
	s.Must(controlled.Write(filepath.Join(s.Output, "provider-pr-proof.json"), map[string]any{"pr": pr, "rules": rules, "required_checks_verified": true, "required_reviews_verified": true, "execution_digest": s.Run.Remote.ExecutionDigest}))
}

func (p *Project) verifyUnreadyEnvironmentEvidence(record env.Record) {
	s := p.Scenario
	if record.State != "failed" || len(record.Rounds) != 3 {
		s.t.Fatal("unready verification must settle its original three-round budget")
	}
	for _, round := range record.Rounds {
		if !round.Settled || round.Run == nil || round.Run.StopStatus != "stopped" || round.Result == nil || round.Result.Version != 1 || round.Result.CollectionStatus != "closed" || round.Report == nil {
			s.t.Fatal("unready verification lacks original stopped/sealed facts")
		}
		if !round.Report.Completed {
			s.t.Fatalf("round %d did not complete its requirement checks: %s", round.Number, round.Run.Diagnostic)
		}
		if round.Report.PR.URL != "" || round.Report.PR.Merged || round.Report.PR.OutcomeUnknown || !round.Report.PR.CleanupCompleted {
			s.t.Fatal("unready requirements produced unexpected PR effects")
		}
	}
	var config app.ProjectConfigInput
	raw, err := os.ReadFile(p.Config)
	s.Must(err)
	s.Must(json.Unmarshal(raw, &config))
	var prs []json.RawMessage
	s.Must(p.providerRead(config, "repos/"+config.PR.Repository+"/pulls?state=all&limit=50&base_branch="+url.QueryEscape(record.Prepared.Manifest.ValidationBase), &prs))
	if len(prs) != 0 {
		s.t.Fatal("requirements failed but a validation PR exists")
	}
	s.Must(controlled.Write(filepath.Join(s.Output, "provider-pr-proof.json"), map[string]any{"no_pr": true, "validation_base": record.Prepared.Manifest.ValidationBase, "execution_digest": s.Run.Remote.ExecutionDigest}))
}

func (p *Project) cleanupRemoteValidation(base string) {
	s := p.Scenario
	p.RemoteCleanupFailed = true
	p.StopService()
	// Failure/unverified records retain the exact refs instead of hiding an
	// unknown PR or deleting branches still held by Runner.
	_, err := os.Stat(filepath.Join(s.Output, "provider-pr-proof.json"))
	if err != nil {
		p.RemoteCleanupFailed = true
		s.t.Error("retain original remote refs: environment result unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := p.Driver.CleanupOwnedRunner(ctx, s.Run.Helper); err != nil {
		p.RemoteCleanupFailed = true
		p.RunnerCleanupFailed = true
		s.t.Error("retain refs: original Runner cleanup unverified:", err)
		return
	}
	if p.RealPR {
		copy := *p
		copy.BaseBranch = base
		if err := copy.cleanupGitHub(); err != nil {
			p.RemoteCleanupFailed = true
			s.t.Error(err)
			return
		}
	}
	if !strings.HasPrefix(base, "codex/e2e/") || !strings.HasPrefix(p.BaseBranch, "codex/e2e/") {
		p.RemoteCleanupFailed = true
		s.t.Error("unowned validation refs")
		return
	}
	refs := map[string]bool{base: true, p.BaseBranch: true}
	reader, err := sqlite.OpenSQLiteReader(p.Database)
	if err != nil {
		p.RemoteCleanupFailed = true
		s.t.Error(err)
		return
	}
	defer reader.Close()
	activities, err := reader.Environments(ctx, p.ProjectID)
	if err != nil {
		p.RemoteCleanupFailed = true
		s.t.Error(err)
		return
	}
	for _, activity := range activities {
		for _, round := range activity.Rounds {
			if round.Run != nil && round.Run.Execution.Branch != "" {
				refs[round.Run.Execution.Branch] = true
			}
		}
	}
	candidates, err := driver.ReadOriginalOperations(ctx, p.Database, p.ProjectID)
	if err != nil {
		p.RemoteCleanupFailed = true
		s.t.Error(err)
		return
	}
	for _, candidate := range candidates {
		if candidate.Project != p.ProjectID {
			p.RemoteCleanupFailed = true
			s.t.Error("candidate ref ownership differs")
			return
		}
		if candidate.Branch != "" {
			refs[candidate.Branch] = true
		}
	}
	for ref := range refs {
		if ref == "AIRefactor" || ref == "main" || ref == "master" || strings.HasPrefix(ref, "-") {
			p.RemoteCleanupFailed = true
			s.t.Error("unsafe owned ref", ref)
			return
		}
		probe := exec.CommandContext(ctx, "git", "-C", p.Repo, "ls-remote", "--exit-code", "origin", "refs/heads/"+ref)
		probe.Env = p.Driver.Environment(os.Environ())
		found, e := probe.CombinedOutput()
		if e != nil {
			if exit, ok := e.(*exec.ExitError); ok && exit.ExitCode() == 2 {
				continue
			}
			p.RemoteCleanupFailed = true
			s.t.Errorf("owned ref query: %v: %s", e, found)
			return
		}
		cmd := exec.CommandContext(ctx, "git", "-C", p.Repo, "push", "origin", "--delete", ref)
		cmd.Env = p.Driver.Environment(os.Environ())
		if output, err := cmd.CombinedOutput(); err != nil {
			p.RemoteCleanupFailed = true
			s.t.Errorf("owned remote ref cleanup: %v: %s", err, output)
			return
		}
	}
	s.Must(controlled.Write(filepath.Join(s.Output, "remote-ref-cleanup.json"), map[string]any{"confirmed": true, "refs": refs}))
	p.RemoteCleanupFailed = false
}

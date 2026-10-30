//go:build e2e

package testenv

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
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

type environmentPR struct {
	Merged bool   `json:"merged"`
	Merge  string `json:"merge_commit_sha"`
	State  string `json:"state"`
	URL    string `json:"html_url"`
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

// Provider evidence uses existing authenticated CLIs for read-only API calls.
// It cannot create, approve or merge a PR.
func (p *Project) providerRead(config executionmodel.PRProviderConfig, path string, target any) error {
	if config.BaseURL == "https://github.com" {
		raw, err := p.gh(p.Scenario.Context, "api", path)
		if err != nil {
			return err
		}
		return json.Unmarshal(raw, target)
	}
	remote, err := url.Parse(p.RemoteURL)
	if err != nil {
		return err
	}
	provider, err := url.Parse(config.BaseURL)
	if err != nil {
		return err
	}
	if remote.Host == "" || !strings.EqualFold(remote.Host, provider.Host) {
		return errors.New("configured Gitea evidence provider differs from the original Git host")
	}
	// Use the project's existing authenticated provider CLI, just as GitHub
	// evidence uses gh. No credential extraction or weaker permission fallback.
	raw, err := p.Driver.RunHelper(p.Scenario.Context, "tea", "api", "--method", "GET", "--repo", p.Repo, path)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, target)
}

func (p *Project) verifyEnvironmentEvidence(record env.Record) {
	s := p.Scenario
	rules := p.environmentRules()
	last := record.Rounds[len(record.Rounds)-1]
	raw, err := os.ReadFile(p.Config)
	s.Must(err)
	config, err := driver.DecodeProjectConfig(raw)
	s.Must(err)
	number, err := strconv.Atoi(filepath.Base(last.Report.PR.URL))
	s.Must(err)
	root := "repos/" + config.PR.Repository
	var pr environmentPR
	s.Must(p.providerRead(config.PR, root+"/pulls/"+strconv.Itoa(number), &pr))
	if pr.URL != last.Report.PR.URL || pr.Base.Ref != record.Prepared.Manifest.ValidationBase || pr.Head.Ref != last.Report.PR.Head || pr.Head.SHA != last.Report.PR.HeadRevision {
		s.t.Fatal("real provider PR differs from sealed script report")
	}
	// Explicitly verify the original branch's rules; a new base inherits nothing.
	sourceBranch := "main"
	if s.Run.Remote.ProjectConfig != nil {
		original, err := driver.DecodeProjectConfig(s.Run.Remote.ProjectConfig)
		s.Must(err)
		sourceBranch = original.Branch
	}
	if config.PR.BaseURL == "https://github.com" {
		var branch struct {
			Protected bool `json:"protected"`
		}
		s.Must(p.providerRead(config.PR, root+"/branches/"+url.PathEscape(sourceBranch), &branch))
		if branch.Protected {
			var policy struct {
				RequiredStatusChecks struct {
					Contexts []string `json:"contexts"`
				} `json:"required_status_checks"`
				RequiredPullRequestReviews struct {
					Count int `json:"required_approving_review_count"`
				} `json:"required_pull_request_reviews"`
			}
			s.Must(p.providerRead(config.PR, root+"/branches/"+url.PathEscape(sourceBranch)+"/protection", &policy))
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
		s.Must(p.providerRead(config.PR, root+"/branch_protections?limit=100", &policies))
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
			Statuses []environmentStatus `json:"statuses"`
		}
		s.Must(p.providerRead(config.PR, root+"/commits/"+pr.Head.SHA+"/status", &status))
		s.Must(requireEnvironmentChecks(rules.RequiredChecks, status.Statuses))
	}
	if rules.MinimumApprovals > 0 {
		var reviews []environmentReview
		s.Must(p.providerRead(config.PR, root+"/pulls/"+strconv.Itoa(number)+"/reviews?limit=100&per_page=100", &reviews))
		s.Must(requireEnvironmentReviews(rules.MinimumApprovals, pr.User.Login, reviews))
	}
	managed := s.ID == "testing.project-environment-orca" || s.Run.Remote.L2Verification == "managed-powershell"
	script := `$path=Join-Path (Get-Location) '` + strings.ReplaceAll(rules.L2Summary, "'", "''") + `';$text=[IO.File]::ReadAllText($path);if($text.Length -gt 65536){throw 'L2 summary exceeds evidence bound'};$summary=$text|ConvertFrom-Json;`
	validators := []string{"SteerlineFlow/testing/test-list.json", "tools/test-project.py", "tools/project_checks.py", "tools/steerline_reports.py"}
	if managed {
		validators = []string{"SteerlineFlow/testing/test-list.json", "tools/run-tests.ps1", "tools/testing/ExecutionPlan.ps1", "tools/testing/ManagedReports.ps1"}
	}
	for _, path := range validators {
		blob := p.git(p.Repo, "rev-parse", record.Prepared.Commit+":"+path)
		script += "$blob=(& git hash-object --path " + psProgressLiteral(path) + " " + psProgressLiteral(path) + "|Out-String).Trim();if($LASTEXITCODE -or $blob -cne " + psProgressLiteral(blob) + "){throw 'original L2 validator changed'};"
	}
	if managed {
		script += `. ./tools/testing/ExecutionPlan.ps1;. ./tools/testing/ManagedReports.ps1;$output=Split-Path $path -Parent;$plan=Get-Content -Raw -LiteralPath (Join-Path $output 'plan.json')|ConvertFrom-Json;if($plan.level -cne 'L2' -or $summary.level -cne 'L2' -or $summary.result -cne 'Passed' -or $plan.shardCount -ne 1 -or $plan.selection.caseId -or $plan.selection.group -or $plan.selection.tag){throw 'not complete project L2'};$definitions=@((Get-Content -Raw ./SteerlineFlow/testing/test-list.json|ConvertFrom-Json).tests|Where-Object level -CEQ 'L2');$selected=@{};foreach($d in $definitions){$selected[$d.id]=$d};$verified=@(Get-ManagedArchitectureResults @{selected=$selected;output=$output} $summary $plan L2);if(!$verified.Count -or $verified.Count -ne $definitions.Count -or @($verified|Where-Object status -CNE 'passed').Count){throw 'original XML/input/cleanup evidence failed'};@{passed=$true;cleanup_complete=$true;level='L2';original_summary=$summary;original_summary_sha256=(Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash;selected_ids=@($definitions.id);project_verification=@($verified|ForEach-Object {@{test_id=$_.id;status=$_.status}})}`
	} else {
		script += `$definitions=@((Get-Content -Raw ./SteerlineFlow/testing/test-list.json|ConvertFrom-Json).tests|Where-Object level -CEQ 'L2');$verified=@($summary.results);if(!$summary.passed -or !$summary.complete -or $summary.exit_code -ne 0 -or !$summary.before_clean -or !$summary.after_clean -or $verified.Count -ne $definitions.Count -or @($summary.details.skipped).Count -or $summary.details.tests_run -le 0){throw 'not complete original project L2'};foreach($d in $definitions){$matches=@($verified|Where-Object test_id -CEQ $d.id);if($matches.Count -ne 1 -or $matches[0].status -cne 'passed' -or $d.id -cnotin @($summary.selection)){throw 'original L2 item not passed'}};@{passed=$true;cleanup_complete=$true;level='L2';original_summary=$summary;original_summary_sha256=(Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash;selected_ids=@($definitions.id);project_verification=$verified}`
	}
	var summary []byte
	if s.Run.Remote.Profile.Execution.Adapter == "codex-app-server" {
		cmd, buildErr := localCodexEnvironmentEvidenceCommand(s.Run.Remote.Profile.Execution, last.Run.Execution.Workspace, script)
		s.Must(buildErr)
		cmd.Env = p.Driver.Environment(os.Environ())
		summary, err = s.RunCommandFor(cmd, "local-l2-evidence", 2*time.Minute)
	} else {
		r := loadOrcaTarget(s, p)
		summary, err = r.Shell(s.Context, s, p, last.Run.Execution.Workspace, script)
	}
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
	pr, err = settleEnvironmentPublication(filepath.Join(s.Output, "environment-pr-merge-intent.json"), last.Report.PR,
		func() (environmentPR, error) {
			var current environmentPR
			err := p.providerRead(config.PR, root+"/pulls/"+strconv.Itoa(number), &current)
			return current, err
		}, func() error {
			if config.PR.BaseURL == "https://github.com" {
				_, err := p.gh(s.Context, "pr", "merge", last.Report.PR.URL, "--repo", config.PR.Repository, "--merge", "--match-head-commit", last.Report.PR.HeadRevision)
				return err
			}
			body, _ := json.Marshal(map[string]any{"do": "merge", "head_commit_id": last.Report.PR.HeadRevision, "delete_branch_after_merge": false})
			path := filepath.Join(s.Output, "environment-pr-merge-body.json")
			if err := os.WriteFile(path, body, 0600); err != nil {
				return err
			}
			_, err := p.Driver.RunHelper(s.Context, "tea", "api", "--method", "POST", "--repo", p.Repo, "--data", "@"+path, root+"/pulls/"+strconv.Itoa(number)+"/merge")
			return err
		})
	s.Must(err)
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
		if round.Report.PR.URL != "" || round.Report.PR.OutcomeUnknown || !round.Report.PR.CleanupCompleted {
			s.t.Fatal("unready requirements produced unexpected PR effects")
		}
	}
	raw, err := os.ReadFile(p.Config)
	s.Must(err)
	config, err := driver.DecodeProjectConfig(raw)
	s.Must(err)
	var prs []json.RawMessage
	s.Must(p.providerRead(config.PR, validationPullsPath(config.PR.BaseURL, config.PR.Repository, record.Prepared.Manifest.ValidationBase), &prs))
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

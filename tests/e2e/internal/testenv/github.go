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

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/internal/testprocess"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
	"github.com/erranliu/SteerLine/tests/e2e/internal/driver"
	"github.com/google/uuid"
)

const githubRepository = "erranliu/SteerlineTest"
const githubURL = "https://github.com/" + githubRepository + ".git"
const githubFields = "url,state,mergedAt,mergeCommit,headRefName,headRefOid,headRepository,headRepositoryOwner,baseRefName"
const githubInfoQuery = `{url:.url,repository:"erranliu/SteerlineTest",base_branch:.baseRefName,head_repository:(.headRepositoryOwner.login + "/" + .headRepository.name),head_branch:.headRefName,head_revision:.headRefOid,state:(if .mergedAt != null then "merged" elif .state == "OPEN" then "open" else "closed_unmerged" end),merged:(.mergedAt != null),merged_at:.mergedAt,merge_commit:.mergeCommit.oid}`

func (p *Project) enableGitHub() {
	s := p.Scenario
	gh, err := exec.LookPath("gh")
	s.Must(err)
	// Authentication stdout is never logged, returned in an error or in evidence.
	cmd := exec.CommandContext(s.Context, gh, "auth", "token", "--hostname", "github.com")
	token, err := cmd.Output()
	if err != nil {
		s.t.Fatal("live GitHub tests require gh authentication")
	}
	if len(strings.TrimSpace(string(token))) == 0 {
		s.t.Fatal("GitHub authentication token missing")
	}
	s.Must(os.WriteFile(filepath.Join(p.Home, "github-token"), []byte(strings.TrimSpace(string(token))), 0600))
	p.RealPR = true
	p.BaseBranch = "codex/e2e/" + strings.ReplaceAll(uuid.NewString(), "-", "") + "/" + s.ID + "/base"
	p.DefaultBranch = "main"
	s.Must(p.GitServer.Stop(s.Context))
	// This exact owned bootstrap repository has not initialized task authority.
	s.Must(RemoveOwned(p.Root, p.Repo))
	p.git(p.Root, "-c", "credential.helper=", "-c", "credential.helper=!gh auth git-credential", "clone", "--quiet", "--no-tags", "--single-branch", "--branch", p.DefaultBranch, githubURL, p.Repo)
	p.RemoteURL = githubURL
	p.git(p.Repo, "config", "credential.helper", "!gh auth git-credential")
	p.git(p.Repo, "config", "http.sslBackend", "openssl")
	p.git(p.Repo, "config", "gc.auto", "0")
	p.git(p.Repo, "config", "maintenance.auto", "false")
	for _, kv := range [][2]string{{"user.name", "E2E"}, {"user.email", "e2e@example.invalid"}, {"commit.gpgsign", "false"}, {"core.autocrlf", "false"}} {
		p.git(p.Repo, "config", kv[0], kv[1])
	}
	p.git(p.Repo, "checkout", "--quiet", "-b", p.BaseBranch)
	// Seed a minimal fixture tree on the disposable base branch. The test
	// repository's future instructions/content must not change this contract.
	p.git(p.Repo, "rm", "-r", "--quiet", "--ignore-unmatch", ".")
	s.Must(os.CopyFS(p.Repo, os.DirFS(filepath.Join(s.Run.Root, "tests", "e2e", "testdata", "projects", "minimal"))))
	s.Must(os.MkdirAll(filepath.Join(p.Repo, "docs", "spec"), 0700))
	s.register("github-base-branch", p.BaseBranch, 0)
	query := map[string]any{"executable": gh, "args": []string{"pr", "view", "{pr_url}", "--repo", "{repository}", "--json", githubFields, "--jq", githubInfoQuery}}
	closeCommand := map[string]any{"executable": gh, "args": []string{"pr", "close", "{pr_url}", "--repo", "{repository}"}}
	var config map[string]any
	raw, err := os.ReadFile(p.Config)
	s.Must(err)
	s.Must(json.Unmarshal(raw, &config))
	config["branch"] = p.BaseBranch
	config["git"] = map[string]any{"executable": "git"}
	config["pr"] = map[string]any{"kind": "command", "base_url": "https://github.com", "repository": githubRepository, "query": query, "close": closeCommand}
	config["pr_poll"] = "1s"
	s.Must(controlled.Write(p.Config, config))
	s.Must(controlled.Write(p.Provider, controlled.State{GitURL: githubURL}))
}

func (p *Project) gh(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	cmd := exec.Command("gh", args...)
	cmd.Env = driver.Environment(os.Environ(), p.Home, p.Provider)
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := testprocess.Run(ctx, cmd)
	logErr := controlled.Write(filepath.Join(p.Scenario.Output, fmt.Sprintf("github-%d.json", time.Now().UnixNano())), map[string]any{"command": append([]string{"gh"}, args...), "stdout": stdout.String(), "stderr": stderr.String(), "error": fmt.Sprint(err)})
	return []byte(stdout.String()), errors.Join(err, logErr)
}
func (p *Project) githubPR(snap Snapshot) (contracts.PRInfo, error) {
	var info contracts.PRInfo
	if len(snap.Domain.WorkflowRuns) != 1 || snap.Domain.WorkflowRuns[0].PRURL == "" {
		return info, nil
	}
	url := snap.Domain.WorkflowRuns[0].PRURL
	if !strings.HasPrefix(url, "https://github.com/"+githubRepository+"/pull/") {
		return info, errors.New("PR escaped the authorized test repository")
	}
	raw, err := p.gh(p.Scenario.Context, "pr", "view", url, "--repo", githubRepository, "--json", githubFields, "--jq", githubInfoQuery)
	if err != nil {
		return info, err
	}
	err = json.Unmarshal(raw, &info)
	if err == nil && (info.BaseBranch != p.BaseBranch || info.HeadBranch == "" || info.HeadBranch == p.DefaultBranch || info.HeadBranch == p.BaseBranch) {
		err = errors.New("real PR branch ownership mismatch")
	}
	return info, err
}
func (p *Project) mergeGitHub() error {
	snap, err := p.Snapshot()
	if err != nil {
		return err
	}
	info := snap.Helper.PR
	if info.State != "open" || info.BaseBranch != p.BaseBranch {
		return errors.New("owned open GitHub PR missing")
	}
	if _, err := p.gh(p.Scenario.Context, "pr", "ready", info.URL, "--repo", githubRepository); err != nil {
		return err
	}
	_, err = p.gh(p.Scenario.Context, "pr", "merge", info.URL, "--repo", githubRepository, "--merge", "--match-head-commit", info.HeadRevision)
	return err
}

func (p *Project) cleanupGitHub() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	// The random base branch was created by this run. Never select PRs by title
	// or delete the test repository's default branch.
	if !strings.HasPrefix(p.BaseBranch, "codex/e2e/") || !strings.HasSuffix(p.BaseBranch, "/"+p.Scenario.ID+"/base") || p.BaseBranch == p.DefaultBranch {
		return errors.New("refuse unowned GitHub cleanup")
	}
	raw, err := p.gh(ctx, "pr", "list", "--repo", githubRepository, "--base", p.BaseBranch, "--state", "all", "--limit", "100", "--json", "url,state,headRefName,baseRefName")
	if err != nil {
		return err
	}
	var prs []struct{ URL, State, HeadRefName, BaseRefName string }
	if err := json.Unmarshal(raw, &prs); err != nil {
		return err
	}
	if len(prs) >= 100 {
		return errors.New("GitHub cleanup inventory may be truncated")
	}
	branches := map[string]bool{p.BaseBranch: true}
	// A push can succeed before PR creation fails. This single-branch clone
	// contained only main initially; other local heads belong to this fixture.
	heads, err := controlled.Git(ctx, p.Repo, "for-each-ref", "--format=%(refname:short)", "refs/heads")
	if err != nil {
		return err
	}
	for _, branch := range strings.Split(heads, "\n") {
		if branch != "" && branch != p.DefaultBranch {
			branches[branch] = true
		}
	}
	for _, pr := range prs {
		if pr.BaseRefName != p.BaseBranch || !strings.HasPrefix(pr.URL, "https://github.com/"+githubRepository+"/pull/") || pr.HeadRefName == p.DefaultBranch || pr.HeadRefName == p.BaseBranch {
			return errors.New("GitHub PR cleanup ownership mismatch")
		}
		if pr.State == "OPEN" {
			if _, err := p.gh(ctx, "pr", "close", pr.URL, "--repo", githubRepository); err != nil {
				return err
			}
		}
		branches[pr.HeadRefName] = true
	}
	if err := controlled.Write(filepath.Join(p.Scenario.Output, "github-prs.json"), prs); err != nil {
		return err
	}
	for branch := range branches {
		// Checking the remote first makes pre-push setup failure cleanup exact.
		raw, err := p.gh(ctx, "api", "repos/"+githubRepository+"/git/matching-refs/heads/"+branch)
		if err != nil {
			return err
		}
		var refs []struct{ Ref string }
		if err := json.Unmarshal(raw, &refs); err != nil {
			return err
		}
		exists := false
		for _, ref := range refs {
			if ref.Ref == "refs/heads/"+branch {
				exists = true
			}
		}
		if exists {
			if _, err := p.gh(ctx, "api", "--method", "DELETE", "repos/"+githubRepository+"/git/refs/heads/"+branch); err != nil {
				return err
			}
		}
	}
	return nil
}

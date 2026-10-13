package controlled

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

func githubCommand(path string, args []string) error {
	return githubCommandOutput(path, args, os.Stdout)
}

func githubCommandOutput(path string, args []string, output io.Writer) error {
	s, err := Read(path)
	if err != nil {
		return err
	}
	if len(args) > 1 && args[0] == "api" && strings.Contains(args[1], "?state=all&head=") {
		result := []any{}
		if s.PR.State == "open" {
			result = append(result, map[string]any{"html_url": s.PR.URL, "state": "open", "head": map[string]any{"ref": s.PR.HeadBranch, "sha": s.PR.HeadRevision, "repo": map[string]string{"full_name": s.PR.Repository}}, "base": map[string]string{"ref": s.BranchName()}})
		}
		return json.NewEncoder(output).Encode(result)
	}
	if len(args) > 1 && args[0] == "pr" && args[1] == "create" {
		branchRequest := argument(args, "--head")
		if s.PR.State != "" && s.PR.HeadBranch == branchRequest {
			return fmt.Errorf("duplicate PR creation")
		}
		for _, pr := range s.PRs {
			if pr.HeadBranch == branchRequest {
				return fmt.Errorf("duplicate PR creation")
			}
		}
		if argument(args, "--repo") != s.RepositoryName() || argument(args, "--base") != s.BranchName() {
			return fmt.Errorf("unexpected PR target")
		}
		cwd := s.Cwd
		if cwd == "" {
			var err error
			cwd, err = os.Getwd()
			if err != nil {
				return err
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		head, err := Git(ctx, cwd, "rev-parse", "HEAD")
		if err != nil {
			return err
		}
		branch, err := Git(ctx, cwd, "symbolic-ref", "--short", "HEAD")
		if err != nil {
			return err
		}
		if branch != argument(args, "--head") {
			return fmt.Errorf("PR branch differs from assigned worktree")
		}
		remoteHead, err := Git(ctx, cwd, "ls-remote", "origin", "refs/heads/"+branch)
		if err != nil || !strings.HasPrefix(remoteHead, head) {
			return fmt.Errorf("PR branch was not pushed: %v", err)
		}
		if s.PRs == nil {
			s.PRs = map[string]executionmodel.PRInfo{}
		}
		if s.PR.URL != "" {
			s.PRs[s.PR.URL] = s.PR
		}
		s.PR = executionmodel.PRInfo{URL: "https://github.com/" + s.RepositoryName() + "/pull/" + strconv.Itoa(len(s.PRs)+1), Repository: s.RepositoryName(), BaseBranch: s.BranchName(), HeadRepository: s.RepositoryName(), HeadBranch: branch, HeadRevision: head, State: "open"}
		s.PRs[s.PR.URL] = s.PR
		if err := Write(path, s); err != nil {
			return err
		}
		fmt.Fprintln(output, s.PR.URL)
		return nil
	}
	return fmt.Errorf("unsupported controlled GitHub command")
}

func ProviderPR(s State, url string) (executionmodel.PRInfo, error) {
	if url == "" || url == s.PR.URL {
		if s.PR.URL == "" {
			return executionmodel.PRInfo{}, fmt.Errorf("controlled PR not published")
		}
		return s.PR, nil
	}
	if pr, ok := s.PRs[url]; ok {
		return pr, nil
	}
	return executionmodel.PRInfo{}, fmt.Errorf("unknown controlled PR URL")
}

package controlled

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

func githubCommand(path string, args []string) error {
	s, err := Read(path)
	if err != nil {
		return err
	}
	if len(args) > 1 && args[0] == "api" && strings.Contains(args[1], "?state=all&head=") {
		result := []any{}
		if s.PR.State == "open" {
			result = append(result, map[string]any{"html_url": s.PR.URL, "state": "open", "head": map[string]any{"ref": s.PR.HeadBranch, "sha": s.PR.HeadRevision, "repo": map[string]string{"full_name": s.PR.Repository}}, "base": map[string]string{"ref": "main"}})
		}
		return json.NewEncoder(os.Stdout).Encode(result)
	}
	if len(args) > 1 && args[0] == "pr" && args[1] == "create" {
		if s.PR.State != "" {
			return fmt.Errorf("duplicate PR creation")
		}
		if argument(args, "--repo") != "e2e/project" || argument(args, "--base") != "main" {
			return fmt.Errorf("unexpected PR target")
		}
		cwd, err := os.Getwd()
		if err != nil {
			return err
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
		s.PR = executionmodel.PRInfo{URL: "https://github.com/e2e/project/pull/1", Repository: "e2e/project", BaseBranch: "main", HeadRepository: "e2e/project", HeadBranch: branch, HeadRevision: head, State: "open"}
		if err := Write(path, s); err != nil {
			return err
		}
		fmt.Println(s.PR.URL)
		return nil
	}
	return fmt.Errorf("unsupported controlled GitHub command")
}

// Package controlled implements only the external protocols used by E2E scenarios.
package controlled

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	testprocess "github.com/erranliu/SteerLine/internal/testkit/process"
)

type State struct {
	WebFeedback bool                             `json:"web_feedback,omitempty"`
	Recovery    *RecoveryFixture                 `json:"recovery,omitempty"`
	Repository  string                           `json:"repository,omitempty"`
	BaseBranch  string                           `json:"base_branch,omitempty"`
	ResultText  string                           `json:"result_text,omitempty"`
	Threads     map[string]ThreadState           `json:"threads,omitempty"`
	GitURL      string                           `json:"git_url"`
	Cwd         string                           `json:"cwd"`
	Session     string                           `json:"session"`
	Sessions    int                              `json:"sessions"`
	Dispatches  int                              `json:"dispatches"`
	Turns       []map[string]any                 `json:"turns"`
	PR          executionmodel.PRInfo            `json:"pr"`
	PRs         map[string]executionmodel.PRInfo `json:"prs,omitempty"`
}

func (s State) RepositoryName() string {
	if s.Repository != "" {
		return s.Repository
	}
	return "e2e/project"
}
func (s State) BranchName() string {
	if s.BaseBranch != "" {
		return s.BaseBranch
	}
	return "main"
}
func (s State) RemoteIdentity() string { return "https://github.com/" + s.RepositoryName() + ".git" }

type ThreadState struct {
	Cwd   string           `json:"cwd"`
	Turns []map[string]any `json:"turns"`
}

func Read(path string) (State, error) {
	var s State
	raw, err := os.ReadFile(path)
	if err != nil {
		return s, err
	}
	err = json.Unmarshal(raw, &s)
	return s, err
}

func Write(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".state-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(append(raw, '\n')); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return replaceStateFile(name, path)
}

func Git(ctx context.Context, dir string, args ...string) (string, error) {
	out, err := gitBytes(ctx, dir, args...)
	return strings.TrimSpace(string(out)), err
}

func gitBytes(ctx context.Context, dir string, args ...string) ([]byte, error) {
	argv := append([]string{"-C", dir}, args...)
	projectDir := dir
	for i, arg := range args {
		if arg == "-C" && i+1 < len(args) {
			projectDir = args[i+1]
		}
	}
	if path := statePathForDirectory(projectDir); path != "" {
		state, err := Read(path)
		if err != nil {
			return nil, err
		}
		if state.GitURL != "" && !isRemoteIdentityQuery(args) {
			argv = append([]string{"-c", "url." + state.GitURL + ".insteadOf=" + state.RemoteIdentity()}, argv...)
		}
	}
	command := exec.Command("git", argv...)
	// Initial fixture setup also ignores host Git identity, hooks and proxy settings.
	var env []string
	for _, v := range os.Environ() {
		key := strings.ToUpper(strings.SplitN(v, "=", 2)[0])
		if strings.HasPrefix(key, "GIT_") || key == "HTTP_PROXY" || key == "HTTPS_PROXY" || key == "ALL_PROXY" || key == "NO_PROXY" {
			continue
		}
		env = append(env, v)
	}
	command.Env = append(env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_TERMINAL_PROMPT=0", "NO_PROXY=127.0.0.1,localhost")
	var logs, stderr strings.Builder
	command.Stdout, command.Stderr = &logs, &stderr
	err := testprocess.Run(ctx, command)
	out := []byte(logs.String())
	if err != nil {
		return nil, fmt.Errorf("git %v: %w: %s%s", args, err, out, stderr.String())
	}
	return out, nil
}

func isRemoteIdentityQuery(args []string) bool {
	for i, a := range args {
		if a == "remote" && i+1 < len(args) && args[i+1] == "get-url" {
			return true
		}
	}
	return false
}

// Merge updates the isolated Git remote before publishing external merge facts.
func Merge(ctx context.Context, path, repo string) error {
	s, err := Read(path)
	if err != nil {
		return err
	}
	if s.PR.State != "open" || s.PR.HeadBranch == "" {
		return fmt.Errorf("no open controlled PR")
	}
	for _, args := range [][]string{{"fetch", "origin"}, {"merge", "--no-ff", "--no-edit", "origin/" + s.PR.HeadBranch}, {"push", "origin", s.BranchName()}} {
		if _, err = Git(ctx, repo, append([]string{"-c", "url." + s.GitURL + ".insteadOf=" + s.RemoteIdentity()}, args...)...); err != nil {
			return err
		}
	}
	head, err := Git(ctx, repo, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	s.PR.State, s.PR.Merged, s.PR.MergedAt, s.PR.MergeCommit = "merged", true, &now, &head
	return Write(path, s)
}

func statePathForDirectory(dir string) string {
	path := os.Getenv("STEERLINE_E2E_CONTROLLED_STATE")
	if path == "" {
		return ""
	}
	root := filepath.Dir(filepath.Dir(path))
	for _, relative := range []string{filepath.Join("projects", "SteerlineTestB"), filepath.Join("home", "worktrees-b")} {
		prefix := strings.ToLower(filepath.Join(root, relative))
		candidate := strings.ToLower(filepath.Clean(dir))
		if candidate == prefix || strings.HasPrefix(candidate, prefix+string(filepath.Separator)) {
			secondary := filepath.Join(root, "provider-b", "state.json")
			if _, err := os.Stat(filepath.Join(root, "manifest.json")); err == nil {
				return secondary
			}
		}
	}
	return path
}

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

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/internal/testprocess"
)

type State struct {
	GitURL     string           `json:"git_url"`
	Cwd        string           `json:"cwd"`
	Session    string           `json:"session"`
	Sessions   int              `json:"sessions"`
	Dispatches int              `json:"dispatches"`
	Turns      []map[string]any `json:"turns"`
	PR         contracts.PRInfo `json:"pr"`
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
	if path := os.Getenv("STEERLINE_E2E_CONTROLLED_STATE"); path != "" {
		state, err := Read(path)
		if err != nil {
			return nil, err
		}
		if state.GitURL != "" && !isRemoteIdentityQuery(args) {
			argv = append([]string{"-c", "url." + state.GitURL + ".insteadOf=https://github.com/e2e/project.git"}, argv...)
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
	for _, args := range [][]string{{"fetch", "origin"}, {"merge", "--no-ff", "--no-edit", "origin/" + s.PR.HeadBranch}, {"push", "origin", "main"}} {
		if _, err = Git(ctx, repo, append([]string{"-c", "url." + s.GitURL + ".insteadOf=https://github.com/e2e/project.git"}, args...)...); err != nil {
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

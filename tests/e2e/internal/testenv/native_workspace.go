package testenv

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
	"github.com/erranliu/SteerLine/tests/e2e/internal/native"
)

// Native workspace tools need no Codex authentication, agent launch or model.
func (p *Project) EnableNativeWorkspaceTools() {
	s := p.Scenario
	if os.Getenv("ORCA_ENVIRONMENT") != "" || os.Getenv("ORCA_PAIRING_CODE") != "" {
		s.t.Fatal("native workspace scene requires local Orca without inherited remote targeting")
	}
	command := os.Getenv("ORCA_CLI_COMMAND")
	if command == "" {
		command = "orca"
	}
	var err error
	p.OrcaExecutable, err = exec.LookPath(command)
	s.Must(err)
	_, err = p.orca(s.Context, "status", "--json")
	s.Must(err)
	_, err = p.orca(s.Context, "repo", "add", "--path", p.Repo, "--json")
	s.Must(err)
	p.OrcaRepoID, err = native.FindOrcaRepo(s.Context, p.orca, p.Repo)
	s.Must(err)
	s.register("orca-repo", p.OrcaRepoID, 0)
}

func (p *Project) UseNativeWorkspaceLayout() {
	// Orca appends the repository directory name to its configured base path.
	// Prepare the fixture before authority creation so both native and product
	// workspaces use the installed layout without moving any live execution.
	if p.ProjectID != "" || filepath.Dir(p.Repo) != p.Root {
		p.Scenario.t.Fatal("native layout must be prepared before project initialization")
	}
	target := filepath.Join(p.Root, "worktrees")
	p.Scenario.Must(os.Rename(p.Repo, target))
	p.Repo = target
	raw, err := os.ReadFile(p.Config)
	p.Scenario.Must(err)
	var config map[string]any
	p.Scenario.Must(json.Unmarshal(raw, &config))
	delete(config, "worktree_root")
	config["repository"] = p.Repo
	query := config["pr"].(map[string]any)["query"].(map[string]any)
	query["args"] = append(query["args"].([]any), "--pr-url", "{pr_url}")
	p.Scenario.Must(controlled.Write(p.Config, config))
	home, err := p.Driver.Run(p.Scenario.Context, "home", "-config", p.Config)
	p.Scenario.Must(err)
	for _, line := range strings.Split(string(home), "\n") {
		if strings.HasPrefix(line, "run_dir ") {
			p.Database = filepath.Join(strings.TrimSpace(strings.TrimPrefix(line, "run_dir")), "steerline.db")
		}
	}
}

func (p *Project) UseNativeDefaultWorktreeRoot() {
	setup, err := p.findSetup(p.Scenario.Context)
	p.Scenario.Must(err)
	root := filepath.Join(filepath.Dir(p.Database), "worktrees")
	_, err = p.orca(p.Scenario.Context, "project", "setup-update", "--setup", setup, "--worktree-base-path", filepath.Dir(root), "--json")
	p.Scenario.Must(err)
}

func (p *Project) NativeCreateWorktree(name string) native.OrcaWorktree {
	s := p.Scenario
	base := p.git(p.Repo, "rev-parse", "HEAD")
	raw, err := p.orca(s.Context, "worktree", "create", "--repo", "id:"+p.OrcaRepoID, "--name", name, "--base-branch", base, "--no-parent", "--setup", "skip", "--json")
	s.Must(err)
	w, err := native.OrcaCreatedWorktree(raw, name)
	s.Must(err)
	p.assertNativeWorkspaceOwned(w)
	s.register("orca-worktree", w.ID, 0)
	if p.git(w.Path, "rev-parse", "HEAD") != base {
		s.t.Fatal("native worktree changed its fixed base")
	}
	// A bare worktree may own a fallback shell. An agent is never requested.
	raw, err = p.orca(s.Context, "terminal", "list", "--worktree", "id:"+w.ID, "--json")
	s.Must(err)
	var terminals struct {
		Terminals []struct {
			AgentIdentity string `json:"agentIdentity"`
		} `json:"terminals"`
	}
	s.Must(json.Unmarshal(raw, &terminals))
	for _, terminal := range terminals.Terminals {
		if terminal.AgentIdentity != "" {
			s.t.Fatal("no-model workspace operation launched an agent")
		}
	}
	return w
}

func (p *Project) assertNativeWorkspaceOwned(w native.OrcaWorktree) {
	if w.ID == "" || w.RepoID != p.OrcaRepoID || !filepath.IsAbs(filepath.FromSlash(w.Path)) || sameLocalPath(w.Path, p.Repo) {
		p.Scenario.t.Fatal("refuse unowned/root native worktree operation")
	}
	common := p.git(w.Path, "rev-parse", "--path-format=absolute", "--git-common-dir")
	root := p.git(p.Repo, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if !sameLocalPath(common, root) {
		p.Scenario.t.Fatal("native worktree belongs to a different repository")
	}
}

func (p *Project) NativeRemoveWorktree(w native.OrcaWorktree) {
	p.assertNativeWorkspaceOwned(w)
	s := p.Scenario
	_, err := p.orca(s.Context, "terminal", "close", "--worktree", "id:"+w.ID, "--all", "--json")
	s.Must(err)
	_, err = p.orca(s.Context, "worktree", "rm", "--worktree", "id:"+w.ID, "--force", "--json")
	s.Must(err)
	s.Wait("native worktree removal", func() (bool, error) {
		_, err := os.Stat(filepath.FromSlash(w.Path))
		return os.IsNotExist(err), nil
	})
	for _, line := range strings.Split(p.git(p.Repo, "worktree", "list", "--porcelain"), "\n") {
		if strings.HasPrefix(line, "worktree ") && sameLocalPath(strings.TrimPrefix(line, "worktree "), w.Path) {
			s.t.Fatal("native removal retained its Git worktree registration")
		}
	}
}

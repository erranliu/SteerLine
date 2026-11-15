package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/erranliu/SteerLine/internal/app/host"
	"github.com/erranliu/SteerLine/internal/pathguard"
)

type worktreeSettings struct {
	Directory string `json:"directory"`
	Effective string `json:"effective"`
	Default   string `json:"default_directory"`
}

func (c Config) worktreeSettings() (worktreeSettings, error) {
	result := worktreeSettings{Effective: c.RunDir, Default: c.RunDir}
	name := filepath.Join(c.RunDir, "worktree-settings", "directory.json")
	if err := ordinaryVerificationPath(c.RunDir, name); err != nil {
		return result, err
	}
	raw, err := os.ReadFile(name)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	if err := json.Unmarshal(raw, &result.Directory); err != nil {
		return result, err
	}
	if result.Directory != "" {
		result.Effective = filepath.Join(result.Directory, RepositoryStateKey(c.Identity()))
	}
	return result, nil
}

func (c Config) setWorktreeDirectory(directory string) (worktreeSettings, error) {
	if directory != "" {
		directory = filepath.Clean(directory)
		if err := c.validateWorktreeRoot(directory); err != nil {
			return worktreeSettings{}, err
		}
		if err := os.MkdirAll(directory, 0700); err != nil {
			return worktreeSettings{}, err
		}
		effective := filepath.Join(directory, RepositoryStateKey(c.Identity()))
		if err := ordinaryVerificationPath(directory, effective); err != nil {
			return worktreeSettings{}, err
		}
		if err := os.MkdirAll(effective, 0700); err != nil {
			return worktreeSettings{}, err
		}
		probe, err := os.CreateTemp(effective, ".steerline-write-*")
		if err != nil {
			return worktreeSettings{}, err
		}
		if err := errors.Join(probe.Close(), os.Remove(probe.Name())); err != nil {
			return worktreeSettings{}, err
		}
	}
	dir := filepath.Join(c.RunDir, "worktree-settings")
	if err := ordinaryVerificationPath(c.RunDir, dir); err != nil {
		return worktreeSettings{}, err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return worktreeSettings{}, err
	}
	raw, _ := json.Marshal(directory)
	if err := host.AtomicWrite(filepath.Join(dir, "directory.json"), raw); err != nil {
		return worktreeSettings{}, err
	}
	return c.worktreeSettings()
}

func (c Config) validateWorktreeRoot(root string) error {
	if !filepath.IsAbs(root) {
		return errors.New("请输入 SteerLine 所在机器上的绝对目录")
	}
	if err := ordinaryVerificationPath(root, root); err != nil {
		return err
	}
	for _, protected := range []string{c.Repository, c.Identity(), c.RunDir} {
		if protected != "" && (pathguard.Within(root, protected) || pathguard.Within(protected, root)) {
			return errors.New("Worktree 根目录须与纳管仓库及项目状态目录分开")
		}
	}
	return nil
}

func (c Config) localWorktreePath(category, id string) (string, error) {
	settings, err := c.worktreeSettings()
	if err != nil {
		return "", err
	}
	path := filepath.Join(settings.Effective, category, id)
	return path, c.verifyLocalWorktreePath(path, category, id)
}

// Existing worktrees keep their original directory across settings changes.
func (c Config) verifyLocalWorktreePath(path, category, id string) error {
	categoryDir := filepath.Dir(path)
	root := filepath.Dir(categoryDir)
	if filepath.Base(path) != id || filepath.Base(categoryDir) != category || !filepath.IsLocal(id) || filepath.Base(id) != id {
		return errors.New("Worktree 路径与原记录不匹配")
	}
	if canonicalPath(root) != canonicalPath(c.RunDir) {
		if filepath.Base(root) != RepositoryStateKey(c.Identity()) {
			return errors.New("Worktree 路径与项目不匹配")
		}
		if err := c.validateWorktreeRoot(filepath.Dir(root)); err != nil {
			return err
		}
	}
	return ordinaryVerificationPath(root, path)
}

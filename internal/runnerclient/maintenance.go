package runnerclient

import (
	"errors"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

// ValidateMaintenance checks only the capabilities this caller actually uses.
func (c *Catalog) ValidateMaintenance(write, read runnercontract.ExecutionInput) error {
	if c == nil {
		return errors.New("Runner能力目录不可用")
	}
	r := c.Capabilities.WorktreeReuse
	if r == nil || r.InputProperty != "reuse_worktree_from_run_id" || r.Workflow != "execute/1" || !r.NewSession || r.ArbitraryDirectory {
		return errors.New("Runner缺少受管工作区复用能力")
	}
	if write.Adapter != "codex-app-server" || write.Target.Kind != "local" || read.Adapter != write.Adapter || read.Target != write.Target {
		return errors.New("维护首版仅支持同一目标的本地Codex组合")
	}
	if err := c.validateExecuteFinal(write); err != nil {
		return err
	}
	if err := c.fileMaterialGenerationUnavailable(); err != "" {
		return errors.New(err)
	}
	return c.ValidateConfiguration(read, runnercontract.Workflow{ID: "generate", Version: 1})
}

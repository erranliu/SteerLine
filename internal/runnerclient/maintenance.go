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
	if r == nil || r.InputProperty != "reuse_worktree_from_run_id" || r.Workflow != "execute/1" || !r.NewSession || r.ArbitraryDirectory || r.OptionalFileWrites == "" {
		return errors.New("Runner缺少受管工作区复用及文件写入结算能力")
	}
	if write.Adapter != "codex-app-server" || write.Target.Kind != "local" || read.Adapter != write.Adapter || read.Target != write.Target {
		return errors.New("维护首版仅支持同一目标的本地Codex组合")
	}
	if err := c.ValidateEnvironmentExecution(write); err != nil {
		return err
	}
	if c.Capabilities.ResultFile.RequiredInitialization != "require_initialization" {
		return errors.New("Runner缺少派发前核验维护结果文件初始化的能力")
	}
	if err := c.fileMaterialGenerationUnavailable(); err != "" {
		return errors.New(err)
	}
	return c.ValidateConfiguration(read, runnercontract.Workflow{ID: "generate", Version: 1})
}

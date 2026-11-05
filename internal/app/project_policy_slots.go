package app

import (
	"context"
	"fmt"
	"time"
)

// The slots are part of the Runner development workflow contract. An explicit
// empty selection records that SteerLine checked all of them.
func (c Config) projectPolicySlots(ctx context.Context, commit string) ([]string, error) {
	reader := taskDocuments{repository: c.Repository, git: c.Git, timeout: positiveDuration(c.CommandTimeout, 30*time.Second), processHost: c.ProcessHost}
	slots, err := c.runtimeCatalog.ProjectPolicySlots()
	if err != nil {
		return nil, err
	}
	paths := make([]string, len(slots))
	for i, slot := range slots {
		paths[i] = slot.Path
	}
	files, err := reader.readPinnedFiles(ctx, commit, paths, true)
	if err != nil {
		return nil, fmt.Errorf("材料准备不足：无法读取固定提交的项目策略：%w", err)
	}
	selected := make([]string, 0, len(slots))
	for i, path := range paths {
		if _, ok := files[path]; ok {
			selected = append(selected, slots[i].Slot)
		}
	}
	return selected, nil
}

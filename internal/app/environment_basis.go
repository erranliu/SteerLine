package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
)

type invalidEnvironmentBasis struct{ error }

func (e *invalidEnvironmentBasis) Unwrap() error { return e.error }

func confirmedEnvironmentBasisError(err error) bool {
	var invalid *invalidEnvironmentBasis
	var nonregular *nonRegularCommittedFileError
	var missing *missingCommittedFileError
	return errors.As(err, &missing) || errors.As(err, &invalid) || errors.As(err, &nonregular)
}

func (p *environmentPort) observeBasis(ctx context.Context, commit string) error {
	ctx = materialReads(ctx)
	if p.store == nil {
		return nil
	}
	seen := map[string]bool{}
	for _, profile := range p.config.RunnerExecution.Profiles {
		if profile.Workflow.ID != "development" && profile.Workflow.ID != "development-audited" {
			continue
		}
		key, err := p.config.profileEnvironmentKey(ctx, profile)
		if err != nil || seen[key] {
			continue
		}
		seen[key] = true
		current, record, err := p.current(ctx, profile.ID)
		if err != nil {
			return err
		}
		if current.ActivityID == "" || current.Invalidated {
			continue
		}
		material, err := p.basisAt(ctx, profile.ID, commit)
		if err != nil && !confirmedEnvironmentBasisError(err) {
			return err
		}
		if err == nil && material.Inputs == record.Prepared.Inputs {
			continue
		}
		reason := environmentBasisChange(record.Prepared, material) + "；须重新检测"
		if err != nil {
			reason = err.Error()
		}
		if err := p.store.RevokeEnvironmentBasis(ctx, current.Project, key, current.ActivityID, reason, current.Revision); err != nil {
			return err
		}
	}
	return nil
}

// Compare the frozen inputs, without rereading an old commit or interpreting
// project-specific requirement semantics. The same reason serves queries and CAS revocation.
func environmentBasisChange(before, after env.Prepared) string {
	var changes []string
	if before.Manifest.Version != after.Manifest.Version {
		changes = append(changes, "环境接口版本变化")
	}
	if before.Manifest.Script != after.Manifest.Script {
		changes = append(changes, fmt.Sprintf("检测入口：%s → %s", before.Manifest.Script, after.Manifest.Script))
	}
	if before.Manifest.Interpreter != after.Manifest.Interpreter {
		changes = append(changes, fmt.Sprintf("解释器：%s → %s", before.Manifest.Interpreter, after.Manifest.Interpreter))
	}
	names := map[string]bool{}
	for name := range before.Manifest.Requirements {
		names[name] = true
	}
	for name := range after.Manifest.Requirements {
		names[name] = true
	}
	keys := make([]string, 0, len(names))
	for name := range names {
		keys = append(keys, name)
	}
	slices.Sort(keys)
	for _, name := range keys {
		old, oldOK := before.Manifest.Requirements[name]
		next, nextOK := after.Manifest.Requirements[name]
		switch {
		case !oldOK:
			changes = append(changes, fmt.Sprintf("新增环境要求 %s：%s", name, next))
		case !nextOK:
			changes = append(changes, fmt.Sprintf("移除环境要求 %s（原值 %s）", name, old))
		case old != next:
			changes = append(changes, fmt.Sprintf("环境要求 %s：%s → %s", name, old, next))
		}
	}
	for _, path := range before.Manifest.Dependencies {
		if _, exists := slices.BinarySearch(after.Manifest.Dependencies, path); !exists {
			changes = append(changes, "移除检测依赖："+path)
		}
	}
	for _, path := range after.Manifest.Dependencies {
		if _, exists := slices.BinarySearch(before.Manifest.Dependencies, path); !exists {
			changes = append(changes, "新增检测依赖："+path)
		} else if before.Files[path] != after.Files[path] {
			changes = append(changes, "检测依赖内容变化："+path)
		}
	}
	if before.Manifest.Script == after.Manifest.Script && before.Files[before.Manifest.Script] != after.Files[after.Manifest.Script] {
		changes = append(changes, "检测脚本内容变化："+after.Manifest.Script)
	}
	if len(changes) == 0 {
		changes = append(changes, "项目身份、执行上下文或接口语义变化")
	}
	if len(changes) > 8 {
		changes = append(changes[:8], fmt.Sprintf("另有 %d 项变化", len(changes)-8))
	}
	return strings.Join(changes, "；")
}

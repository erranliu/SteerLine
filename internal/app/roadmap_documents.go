package app

import (
	"context"
	"errors"
	"os"
	"sort"
	"strings"
	"unicode/utf8"

	roadmap "github.com/erranliu/SteerLine/internal/roadmap/model"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func (p *roadmapPort) documentContext(ctx context.Context, r roadmap.Record) (roadmap.DocumentContext, error) {
	var result roadmap.DocumentContext
	m := p.config.runtimeMaintenance
	if m == nil {
		return result, errors.New("项目代码基线不可用")
	}
	baseline, err := m.check(ctx)
	if err != nil {
		return result, err
	}
	result.Commit = baseline.Commit
	tree, err := m.git(ctx, "rev-parse", "--verify", result.Commit+"^{tree}")
	if err != nil {
		return result, err
	}
	result.Tree = strings.TrimSpace(string(tree))
	for _, name := range []string{"SteerlineFlow/spec/SpecGuide.md", "SteerlineFlow/testing.md", "SteerlineProjectGuideDoc/spec-plan-guidelines.md"} {
		body, err := m.read(ctx, result.Commit, name)
		if err != nil {
			return result, errors.New("固定版本缺少必需生成规范：" + name)
		}
		if len(body) == 0 || len(body) > roadmap.MaxBytes || !utf8.Valid(body) {
			return result, errors.New("生成规范为空或超过大小上限：" + name)
		}
		text := string(body)
		result.Materials = append(result.Materials, runnercontract.Document{Kind: "inline", Path: name, Commit: result.Commit, Content: &text})
	}
	// Inline content is frozen by Runner; its schema requires no independent commit field.
	for i := range result.Materials {
		result.Materials[i].Commit = ""
	}
	return result, nil
}

func (p *roadmapPort) publicationPreview(ctx context.Context, r roadmap.Record) (any, error) {
	files, err := p.publicationFiles(ctx, r)
	if err != nil {
		return nil, err
	}
	type change struct {
		Path   string `json:"path"`
		Before string `json:"before"`
		After  string `json:"after"`
	}
	out := struct {
		Repository string   `json:"repository"`
		Branch     string   `json:"branch"`
		Files      []change `json:"files"`
	}{Repository: p.config.PR.BaseURL + "/" + p.config.PR.Repository, Branch: p.config.Branch}
	keys := []string{}
	for key := range files {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		old, err := p.config.runtimeMaintenance.read(ctx, r.CodeCommit, key)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		out.Files = append(out.Files, change{key, string(old), files[key]})
	}
	return out, nil
}

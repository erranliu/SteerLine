package app

import (
	"context"
	"encoding/json"
	"errors"
	roadmapservice "github.com/erranliu/SteerLine/internal/roadmap"
	testmodel "github.com/erranliu/SteerLine/internal/testmanagement/model"
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
	entry, err := testmodel.ReadProjectEntry(func(path string) ([]byte, error) { return m.read(ctx, result.Commit, path) })
	if err != nil {
		return result, err
	}
	names := append([]string{"SteerlineFlow/spec/SpecGuide.md", "SteerlineProjectGuideDoc/spec-plan-guidelines.md"}, entry.Paths()...)
	for _, name := range names {
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
	// Retain exact guideline bytes in the record; the model reads the pinned
	// repository copy, whose manifest must later match these bytes.
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
		var missing *missingCommittedFileError
		if err != nil && !errors.As(err, &missing) {
			return nil, err
		}
		out.Files = append(out.Files, change{key, string(old), files[key]})
	}
	current := p.config.runtimeMaintenance.current()
	if current.Commit != "" && current.Commit != r.CodeCommit && current.Diagnostic == "" {
		before, _, err := p.testCatalogueAt(ctx, r.CodeCommit)
		if err != nil {
			return nil, err
		}
		after, name, err := p.testCatalogueAt(ctx, current.Commit)
		if err != nil {
			return nil, err
		}
		for _, d := range r.Documents {
			old, err := roadmapTestPreview(r, d, before)
			if err != nil {
				return nil, err
			}
			latest, err := roadmapTestPreview(r, d, after)
			if err != nil {
				return nil, err
			}
			oldJSON, _ := json.MarshalIndent(old, "", "  ")
			newJSON, _ := json.MarshalIndent(latest, "", "  ")
			if string(oldJSON) != string(newJSON) {
				out.Files = append(out.Files, change{Path: name + " · " + d.Key + " 所选定义", Before: string(oldJSON), After: string(newJSON)})
			}
		}
	}
	return out, nil
}

// Pending IDs remain visible as promises, never fabricated definitions.
func roadmapTestPreview(r roadmap.Record, d roadmap.Documents, catalogue testmodel.Catalogue) (any, error) {
	if err := roadmapservice.ValidateTestIDs(r, catalogue); err != nil {
		return nil, err
	}
	known := map[string]testmodel.Definition{}
	for _, definition := range catalogue.Tests {
		known[definition.ID] = definition
	}
	view := struct {
		Definitions []testmodel.Definition `json:"definitions"`
		Pending     []string               `json:"pending_tests"`
	}{Definitions: []testmodel.Definition{}, Pending: []string{}}
	for _, id := range d.Tests {
		if definition, ok := known[id]; ok {
			view.Definitions = append(view.Definitions, definition)
		} else {
			view.Pending = append(view.Pending, id)
		}
	}
	return view, nil
}

func (p *roadmapPort) testCatalogueAt(ctx context.Context, commit string) (testmodel.Catalogue, string, error) {
	read := func(path string) ([]byte, error) { return p.config.runtimeMaintenance.read(ctx, commit, path) }
	entry, err := testmodel.ReadProjectEntry(read)
	if err != nil {
		return testmodel.Catalogue{}, "", err
	}
	raw, err := read(entry.TestingCatalogue)
	if err != nil {
		return testmodel.Catalogue{}, entry.TestingCatalogue, err
	}
	catalogue, err := testmodel.ParseCatalogue(raw)
	return catalogue, entry.TestingCatalogue, err
}

package runnerclient

import (
	"errors"
	"fmt"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"path/filepath"
)

// GenerateCapability mirrors the published generate capability, not a new workflow.
type GenerateCapability struct {
	Workflow   string `json:"workflow"`
	Repository struct {
		InputProperty    string `json:"input_property"`
		Source           string `json:"source"`
		Commit           string `json:"commit"`
		Snapshot         string `json:"snapshot"`
		Selection        string `json:"selection"`
		MaxManifestBytes int    `json:"max_manifest_bytes"`
		MaxFiles         int    `json:"max_files"`
		MaxBytes         int    `json:"max_bytes"`
	} `json:"repository"`
	Result             string `json:"result"`
	Source             string `json:"source"`
	MaxResultUTF8Bytes int    `json:"max_result_utf8_bytes"`
	OutputSchema       string `json:"output_schema"`
}

// VerifyRepositoryFiles consumes the existing public snapshot manifest. It is
// availability/provenance evidence, never proof that a model read the files.
func VerifyRepositoryFiles(source *runnercontract.RepositorySource, repository runnercontract.GenerateRepository, expected map[string]string) error {
	if source == nil || source.Commit != repository.Commit || filepath.Clean(source.LocalPath) != filepath.Clean(repository.LocalPath) || source.Tree == "" {
		return errors.New("fixed repository source identity is unverified")
	}
	selected := map[string]bool{}
	for _, path := range repository.Paths {
		if selected[path] {
			return errors.New("duplicate selected file")
		}
		selected[path] = true
	}
	if repository.Paths != nil && (len(selected) == 0 || len(source.Files) != len(selected)) {
		return errors.New("repository selection differs from fixed source files")
	}
	seen := map[string]bool{}
	matched := map[string]bool{}
	for _, file := range source.Files {
		if seen[file.Path] || repository.Paths != nil && !selected[file.Path] {
			return errors.New("duplicate repository source file")
		}
		seen[file.Path] = true
		if digest, ok := expected[file.Path]; ok {
			if file.SHA256 != digest || file.Blob == "" || file.SizeBytes < 1 {
				return fmt.Errorf("fixed source file differs: %s", file.Path)
			}
			matched[file.Path] = true
		}
	}
	for path := range expected {
		if !matched[path] {
			return fmt.Errorf("fixed source file is absent: %s", path)
		}
	}
	return nil
}

func (c *Catalog) RepositoryGenerationUnavailable() string {
	if c == nil || c.generate == nil || c.Capabilities.Generate == nil {
		return "所选 Runner 未公布固定仓库 generate 能力"
	}
	g := c.Capabilities.Generate
	if g.Workflow != "generate/1" || g.Repository.InputProperty != "generate.repository" ||
		g.Repository.Source != "existing_local_git_objects" || g.Repository.Commit != "full_commit_object_id" ||
		g.Repository.Selection != "explicit_file_paths_or_whole_tree" || g.Repository.Snapshot != "private_regular_files_without_git" || g.Repository.MaxFiles < 1 ||
		g.Repository.MaxBytes < 1 || g.Repository.MaxManifestBytes < 1 || g.MaxResultUTF8Bytes < 1 ||
		g.Result != "inspect/history: content_result.json" || g.Source != "repository_source: commit/tree/files/blob/sha256/size_bytes" ||
		g.OutputSchema != "caller_defined_json_schema_2020_12" {
		return "所选 Runner 固定仓库 generate 契约不完整"
	}
	return ""
}
func (c *Catalog) ValidateRepositoryGenerate(in runnercontract.GenerateRunInput) error {
	if reason := c.RepositoryGenerationUnavailable(); reason != "" {
		return errors.New(reason)
	}
	if in.Generate.Repository == nil {
		return errors.New("文档生成必须固定仓库与完整提交")
	}
	return c.ValidateGenerate(in)
}

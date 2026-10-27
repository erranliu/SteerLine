package runnerclient

import (
	"errors"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

// GenerateCapability mirrors the published generate capability, not a new workflow.
type GenerateCapability struct {
	Workflow   string `json:"workflow"`
	Repository struct {
		InputProperty    string `json:"input_property"`
		Source           string `json:"source"`
		Commit           string `json:"commit"`
		Snapshot         string `json:"snapshot"`
		MaxManifestBytes int    `json:"max_manifest_bytes"`
		MaxFiles         int    `json:"max_files"`
		MaxBytes         int    `json:"max_bytes"`
	} `json:"repository"`
	Result             string `json:"result"`
	Source             string `json:"source"`
	MaxResultUTF8Bytes int    `json:"max_result_utf8_bytes"`
	OutputSchema       string `json:"output_schema"`
}

func (c *Catalog) RepositoryGenerationUnavailable() string {
	if c == nil || c.generate == nil || c.Capabilities.Generate == nil {
		return "所选 Runner 未公布固定仓库 generate 能力"
	}
	g := c.Capabilities.Generate
	if g.Workflow != "generate/1" || g.Repository.InputProperty != "generate.repository" ||
		g.Repository.Source != "existing_local_git_objects" || g.Repository.Commit != "full_commit_object_id" ||
		g.Repository.Snapshot != "private_regular_files_without_git" || g.Repository.MaxFiles < 1 ||
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

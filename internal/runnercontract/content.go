package runnercontract

import "encoding/json"

type ConversationRunInput struct {
	Title        string            `json:"title"`
	Correlation  map[string]string `json:"correlation"`
	Workflow     Workflow          `json:"workflow"`
	Execution    ExecutionInput    `json:"execution"`
	Repository   Repository        `json:"repository"`
	Conversation ConversationInput `json:"conversation"`
}

type ConversationInput struct {
	Instruction string     `json:"instruction"`
	Materials   []Document `json:"materials"`
}

type WorkspaceWrite struct {
	Owner      string `json:"owner"`
	LeaseID    string `json:"lease_id,omitempty"`
	CanAcquire bool   `json:"can_acquire"`
}

type WorkspaceReleaseInput struct {
	ExpectedRevision uint64 `json:"expected_revision"`
	LeaseID          string `json:"lease_id"`
}

// GenerateInput is the published generate/1 payload; it carries no development fields.
type GenerateInput struct {
	Repository   *GenerateRepository `json:"repository,omitempty"`
	Instruction  string              `json:"instruction"`
	Materials    []Document          `json:"materials"`
	OutputSchema json.RawMessage     `json:"output_schema,omitempty"`
}
type GenerateRunInput struct {
	Title       string            `json:"title"`
	Correlation map[string]string `json:"correlation,omitempty"`
	Workflow    Workflow          `json:"workflow"`
	Execution   ExecutionInput    `json:"execution"`
	Generate    GenerateInput     `json:"generate"`
}
type ContentResult struct {
	FileMaterials    []Material        `json:"file_materials,omitempty"`
	RepositorySource *RepositorySource `json:"repository_source,omitempty"`
	Status           string            `json:"status"`
	Text             string            `json:"text,omitempty"`
	JSON             json.RawMessage   `json:"json,omitempty"`
	Diagnostic       string            `json:"diagnostic,omitempty"`
	Source           Final             `json:"source"`
}

// RepositorySource is the Runner's fixed Git-object snapshot manifest. It
// describes available files, not proof that the model read every file.
type GenerateRepository struct {
	LocalPath string   `json:"local_path"`
	Commit    string   `json:"commit"`
	Paths     []string `json:"paths,omitempty"`
}
type RepositoryFile struct {
	Path      string `json:"path"`
	Blob      string `json:"blob"`
	SHA256    string `json:"sha256"`
	SizeBytes int    `json:"size_bytes"`
}
type RepositorySource struct {
	LocalPath string           `json:"local_path"`
	Commit    string           `json:"commit"`
	Tree      string           `json:"tree"`
	Files     []RepositoryFile `json:"files"`
}

const MaxGenerateResultBytes = 1 << 20

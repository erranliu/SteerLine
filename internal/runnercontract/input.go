package runnercontract

import "encoding/json"

// BuildPin never pins a daemon incarnation; that belongs to ProjectRuntime.
type BuildPin struct {
	Executable     string `json:"executable"`
	Version        string `json:"version"`
	SHA256         string `json:"sha256"`
	ResourceDigest string `json:"resource_digest"`
	StateDir       string `json:"state_dir"`
}

type Document struct {
	Kind    string  `json:"kind,omitempty"`
	Path    string  `json:"path"`
	Commit  string  `json:"commit,omitempty"`
	Content *string `json:"content,omitempty"`
}
type Repository struct {
	URL          string `json:"url"`
	LocalPath    string `json:"local_path,omitempty"`
	Remote       string `json:"remote"`
	BaseBranch   string `json:"base_branch"`
	BaseRevision string `json:"base_revision"`
}
type Documents struct {
	Spec       Document   `json:"spec"`
	Plan       Document   `json:"plan"`
	References []Document `json:"references,omitempty"`
}
type Workflow struct {
	promptOverridesPresent bool
	auditModelPresent      bool
	ID                     string            `json:"id"`
	Version                int               `json:"version"`
	PromptOverrides        map[string]string `json:"prompt_overrides,omitempty"`
	AuditModel             *ModelSettings    `json:"audit_model,omitempty"`
}

type ModelSettings struct {
	effortPresent   bool
	effortNull      bool
	Model           string `json:"model"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
}

// WorkflowPrompt exposes only published prompt metadata, not execution transitions.
type WorkflowPrompt struct {
	Kind      string `json:"kind"`
	Contract  string `json:"contract"`
	PromptKey string `json:"prompt_key"`
	Prompt    string `json:"prompt"`
}

type WorkflowChoice struct {
	States              map[string]WorkflowPrompt `json:"states,omitempty"`
	ID                  string                    `json:"id"`
	Version             int                       `json:"version"`
	Description         string                    `json:"description"`
	SupportedAdapters   []string                  `json:"supported_adapters"`
	AuditModelRequired  bool                      `json:"audit_model_required"`
	PromptSteps         []string                  `json:"prompt_steps"`
	MaximumRepairCycles uint64                    `json:"maximum_repair_cycles"`
}
type ExecutionTarget struct {
	Kind        string `json:"kind"`
	Environment string `json:"environment,omitempty"`
}

type ExecutionInput struct {
	Adapter  string          `json:"adapter"`
	Target   ExecutionTarget `json:"target"`
	Settings json.RawMessage `json:"settings"`
}
type Publication struct {
	Provider   string          `json:"provider"`
	Repository string          `json:"repository"`
	BaseBranch string          `json:"base_branch"`
	Settings   json.RawMessage `json:"settings"`
}
type RunInput struct {
	ResultFile   *ProgressFileInput `json:"result_file,omitempty"`
	ProgressFile *ProgressFileInput `json:"progress_file,omitempty"`
	Title        string             `json:"title"`
	NamePrefix   string             `json:"name_prefix,omitempty"`
	Correlation  map[string]string  `json:"correlation,omitempty"`
	Repository   Repository         `json:"repository"`
	Documents    Documents          `json:"documents"`
	Workflow     Workflow           `json:"workflow"`
	Execution    ExecutionInput     `json:"execution"`
	Publication  Publication        `json:"publication"`
}

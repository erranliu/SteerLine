package runnerclient

import "encoding/json"

type Document struct {
	Kind    string  `json:"kind"`
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
	ID              string            `json:"id"`
	Version         int               `json:"version"`
	PromptOverrides map[string]string `json:"prompt_overrides,omitempty"`
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
	Title       string            `json:"title"`
	Correlation map[string]string `json:"correlation,omitempty"`
	Repository  Repository        `json:"repository"`
	Documents   Documents         `json:"documents"`
	Workflow    Workflow          `json:"workflow"`
	Execution   ExecutionInput    `json:"execution"`
	Publication Publication       `json:"publication"`
}

package publication

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// ProviderRef fixes provider identity while keeping command arguments and credentials in runtime configuration.
type ProviderRef struct {
	Kind            string `json:"kind"`
	BaseURL         string `json:"base_url"`
	Repository      string `json:"repository"`
	QueryExecutable string `json:"query_executable"`
}

func ProviderIdentity(config Config) ProviderRef {
	return ProviderRef{Kind: config.Kind, BaseURL: config.BaseURL, Repository: config.Repository, QueryExecutable: config.Query.Executable}
}
func (identity ProviderRef) Matches(config Config) bool { return identity == ProviderIdentity(config) }

// Request contains only the fixed external action and its candidate evidence.
// The center persists this value before calling Publish.
type Request struct {
	Version      int                          `json:"version"`
	RunID        string                       `json:"run_id"`
	ExecutionID  string                       `json:"execution_id"`
	TurnID       string                       `json:"turn_id"`
	TaskID       string                       `json:"task_id"`
	Workspace    executionapi.WorkspaceIntent `json:"workspace"`
	GitCommonDir string                       `json:"git_common_dir"`
	Remote       string                       `json:"remote"`
	BaseBranch   string                       `json:"base_branch"`
	Provider     ProviderRef                  `json:"provider"`
	StartingHead string                       `json:"starting_head"`
	IndexTree    string                       `json:"index_tree"`
	Digest       string                       `json:"digest"`
}

var runIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
var turnIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func WorkerInstruction(runID string) string {
	return "\nLocal publication: stage only the files you intend to deliver. Do not commit, push, or create a PR from this Codex session. When the task is ready for a draft PR, end your final reply with this exact line (outside a code block):\nSTEERLINE_PUBLISH_READY " + runID + "\nIf you still need input or the work is incomplete, omit that line.\n"
}
func RequestedByWorker(message, runID string) bool {
	if !runIDPattern.MatchString(runID) {
		return false
	}
	lines := strings.Split(strings.TrimSpace(message), "\n")
	return len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "STEERLINE_PUBLISH_READY "+runID
}
func canonical(path string) string {
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		return strings.ToLower(path)
	}
	return path
}
func requestDigest(request Request) string {
	request.Digest = ""
	data, _ := json.Marshal(request)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func (request Request) Validate() error {
	if request.Version != 1 || !runIDPattern.MatchString(request.RunID) || request.ExecutionID == "" || !turnIDPattern.MatchString(request.TurnID) || request.TaskID == "" ||
		!filepath.IsAbs(request.Workspace.Path) || !filepath.IsAbs(request.GitCommonDir) || request.Workspace.Branch == "" || !validCommit(request.Workspace.BaseCommit) || request.Remote == "" || request.BaseBranch == "" {
		return errors.New("local publication requires a fixed execution, turn and workspace")
	}
	if !regexp.MustCompile(`^codex/[A-Za-z0-9][A-Za-z0-9._-]*-[0-9a-f]{12}$`).MatchString(request.Workspace.Branch) || strings.Contains(request.Workspace.Branch, "..") {
		return errors.New("local publication execution branch is invalid")
	}
	if request.Provider.Kind != "command" || strings.TrimSuffix(request.Provider.BaseURL, "/") != "https://github.com" || request.Provider.QueryExecutable == "" || request.Provider.Repository == "" {
		return errors.New("local host publication requires a fixed GitHub command provider")
	}
	if (Config{Kind: request.Provider.Kind, BaseURL: request.Provider.BaseURL, Repository: request.Provider.Repository}).ValidateRemote("https://github.com/"+request.Provider.Repository+".git") != nil {
		return errors.New("local GitHub publication repository is invalid")
	}
	return nil
}
func (request Request) ValidateCandidate() error {
	if err := request.Validate(); err != nil {
		return err
	}
	if !validCommit(request.StartingHead) || !validCommit(request.IndexTree) || request.Digest == "" || request.Digest != requestDigest(request) {
		return errors.New("saved local publication candidate or digest is invalid")
	}
	return nil
}

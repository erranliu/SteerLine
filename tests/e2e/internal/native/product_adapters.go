package native

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	"github.com/erranliu/SteerLine/internal/execution/adapters/orca"
)

const SupportedCodexVersion = codexworker.PolicyCodexVersion
const OrcaWorktreeTrashDirectory = orcaworker.WorktreeTrashDirectory

var ErrOrcaRepoUnavailable = orcaworker.ErrRepoUnavailable

func CodexExecutable(command string) (string, error) { return codexworker.NativeExecutable(command) }

func OrcaWorkerProfile(executable, waitTimeout string) map[string]any {
	return map[string]any{"executable": executable, "args": []string{}, "shell": "powershell.exe", "wait_timeout": waitTimeout, "model": Model, "reasoning_effort": Effort}
}

func WorkerImplementationConfigs(executable string, orca map[string]any) []map[string]any {
	if orca != nil {
		return []map[string]any{{"implementation_key": orcaworker.ProfileCodecKey, "codec_version": orcaworker.ProfileCodecVersion, "payload": orca}}
	}
	codex := map[string]any{"executable": executable, "model": Model, "reasoning_effort": Effort}
	configs := []map[string]any{{"implementation_key": codexworker.ProfileCodecKey, "codec_version": codexworker.ProfileCodecVersion, "payload": codex}}
	return configs
}

func FindOrcaRepo(ctx context.Context, call func(context.Context, ...string) (json.RawMessage, error), repository string) (string, error) {
	return orcaworker.FindRepo(ctx, call, repository)
}

func OrcaPayload(raw []byte) (json.RawMessage, error) {
	envelope, err := orcaworker.ParseEnvelope(raw)
	return envelope.Result, err
}

type OrcaWorktree struct{ ID, RepoID, Path string }

func OrcaCreatedWorktree(raw []byte, name string) (OrcaWorktree, error) {
	var receipt struct {
		Worktree orcaworker.Worktree `json:"worktree"`
	}
	if err := json.Unmarshal(raw, &receipt); err != nil {
		return OrcaWorktree{}, err
	}
	w := receipt.Worktree
	if w.ID == "" || w.RepoID == "" || w.Path == "" || w.DisplayName != name {
		return OrcaWorktree{}, errors.New("native worktree create did not return its fixed identity/name")
	}
	return OrcaWorktree{w.ID, w.RepoID, w.Path}, nil
}

func OrcaWorktrees(raw []byte) ([]OrcaWorktree, error) {
	var listing struct {
		Worktrees []orcaworker.Worktree `json:"worktrees"`
		Truncated bool                  `json:"truncated"`
	}
	if err := json.Unmarshal(raw, &listing); err != nil {
		return nil, err
	}
	if listing.Truncated {
		return nil, errors.New("owned worktree inventory truncated")
	}
	result := make([]OrcaWorktree, 0, len(listing.Worktrees))
	for _, w := range listing.Worktrees {
		result = append(result, OrcaWorktree{ID: w.ID, RepoID: w.RepoID, Path: w.Path})
	}
	return result, nil
}

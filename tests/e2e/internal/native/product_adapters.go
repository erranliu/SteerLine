package native

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	"github.com/erranliu/SteerLine/internal/execution/adapters/orca"
)

const SupportedCodexVersion = codexworker.PolicyCodexVersion

var ErrOrcaRepoUnavailable = orcaworker.ErrRepoUnavailable

func CodexExecutable(command string) (string, error) { return codexworker.NativeExecutable(command) }

func WorkerImplementationConfigs(executable string, orca map[string]any) []map[string]any {
	codex := map[string]any{"executable": executable, "model": Model, "reasoning_effort": Effort}
	configs := []map[string]any{{"implementation_key": codexworker.ProfileCodecKey, "codec_version": 1, "payload": codex}}
	if orca != nil {
		codex["args"] = []string{"--no-daemon"}
		configs = append(configs, map[string]any{"implementation_key": orcaworker.ProfileCodecKey, "codec_version": 1, "payload": orca})
	}
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

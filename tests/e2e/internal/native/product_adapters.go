package native

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	transport "github.com/erranliu/SteerLine/internal/execution/adapters/orcatransport"
	orcaworker "github.com/erranliu/SteerLine/internal/execution/adapters/protocol/orca"
	backend "github.com/erranliu/SteerLine/internal/execution/backend"
)

const SupportedCodexVersion = codexworker.PolicyCodexVersion

var ErrOrcaRepoUnavailable = orcaworker.ErrRepoUnavailable

func OrcaOriginalTurnEnded(ctx context.Context, environment string, process backend.ProcessIdentity, proof transport.NativeProof) (bool, error) {
	return orcaOriginalTurnEnded(ctx, environment, process, proof, orcaworker.OpenStateFeed)
}

func orcaOriginalTurnEnded(ctx context.Context, environment string, process backend.ProcessIdentity, proof transport.NativeProof, factory orcaworker.StateFeedFactory) (bool, error) {
	if proof.WorkingAt <= 0 || proof.Provider == "" || proof.Pane == "" {
		return false, errors.New("missing original native turn start")
	}
	feed, err := factory(ctx, environment, process)
	if err != nil {
		return false, err
	}
	defer feed.Close()
	current, err := feed.Query(ctx)
	if err != nil {
		return false, err
	}
	if current.PaneKey != proof.Pane || current.ProviderSession.ID != proof.Provider || current.AgentType != "codex" {
		return false, errors.New("original native main-agent identity changed")
	}
	return current.MainAgent != nil && current.MainAgent.State == "done" && current.MainAgent.StateStartedAt > proof.WorkingAt && current.UpdatedAt >= current.MainAgent.StateStartedAt && !current.Interrupted && (current.MainAgent.Outcome == "" || current.MainAgent.Outcome == "success"), nil
}

func CodexExecutable(command string) (string, error) { return codexworker.NativeExecutable(command) }

func OrcaWorkerProfile(executable, waitTimeout string) map[string]any {
	return map[string]any{"executable": executable, "args": []string{}, "wait_timeout": waitTimeout}
}

func WorkerImplementationConfigs(executable string, orca map[string]any) []map[string]any {
	if orca != nil {
		return []map[string]any{{"implementation_key": transport.ProfileKey, "codec_version": transport.ProfileVersion, "payload": orca}}
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

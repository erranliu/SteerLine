package native

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	transport "github.com/erranliu/SteerLine/internal/execution/adapters/orcatransport"
	orcaworker "github.com/erranliu/SteerLine/internal/execution/adapters/protocol/orca"
	backend "github.com/erranliu/SteerLine/internal/execution/backend"
)

type endStateFeed struct {
	current orcaworker.AgentState
	err     error
	closed  bool
}

func (f *endStateFeed) Initial() orcaworker.AgentState                       { return f.current }
func (f *endStateFeed) Drain() ([]orcaworker.AgentState, error)              { return nil, nil }
func (f *endStateFeed) Query(context.Context) (orcaworker.AgentState, error) { return f.current, f.err }
func (f *endStateFeed) Close()                                               { f.closed = true }

func TestOrcaOriginalEndRequiresBoundNativeFacts(t *testing.T) {
	for _, fault := range []string{"none", "working", "previous-end", "provider", "pane", "interrupted", "failure", "query-error"} {
		t.Run(fault, func(t *testing.T) {
			proof := transport.NativeProof{Pane: "tab:pane", Provider: "provider", WorkingAt: 10}
			state := orcaworker.AgentState{PaneKey: proof.Pane, AgentType: "codex", UpdatedAt: 20, MainAgent: &orcaworker.MainAgentState{State: "done", StateStartedAt: 20}}
			state.ProviderSession.ID = proof.Provider
			feed := &endStateFeed{current: state}
			switch fault {
			case "working":
				feed.current.MainAgent.State = "working"
			case "previous-end":
				feed.current.MainAgent.StateStartedAt = 9
			case "provider":
				feed.current.ProviderSession.ID = "another"
			case "pane":
				feed.current.PaneKey = "another:pane"
			case "interrupted":
				feed.current.Interrupted = true
			case "failure":
				feed.current.MainAgent.Outcome = "failure"
			case "query-error":
				feed.err = errors.New("unavailable")
			}
			ended, err := orcaOriginalTurnEnded(context.Background(), "original-host", backend.ProcessIdentity{Terminal: "original"}, proof, func(_ context.Context, host string, process backend.ProcessIdentity) (orcaworker.StateFeed, error) {
				if host != "original-host" || process.Terminal != "original" {
					t.Fatal("query changed its source")
				}
				return feed, nil
			})
			if ended != (fault == "none") || !feed.closed || fault == "none" && err != nil {
				t.Fatalf("original end attribution failed: %s %t %v", fault, ended, err)
			}
		})
	}
}

func TestWorkerConfigsPreserveNativeModelAndChainSettings(t *testing.T) {
	local := WorkerImplementationConfigs("isolated-codex", nil)
	codex := local[0]["payload"].(map[string]any)
	if len(local) != 1 || codex["executable"] != "isolated-codex" || codex["model"] != Model || codex["reasoning_effort"] != Effort || codex["windows_sandbox"] != nil || codex["args"] != nil {
		t.Fatal("local model/permission profile changed")
	}
	orca := OrcaWorkerProfile("isolated-orca", "30s")
	configs := WorkerImplementationConfigs("isolated-codex", orca)
	if len(configs) != 1 || configs[0]["payload"].(map[string]any)["executable"] != "isolated-orca" || orca["model"] != nil || orca["reasoning_effort"] != nil || orca["repository_selector"] != nil || orca["windows_sandbox"] != nil {
		t.Fatal("Orca native launch inherited the client Codex configuration")
	}
}

func TestOrcaSmokeProfileUsesExplicitEmptyArgs(t *testing.T) {
	profile := OrcaWorkerProfile("isolated-orca", "30s")
	raw, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if string(fields["args"]) != "[]" {
		t.Fatalf("Orca smoke omitted its regression-triggering args: %s", raw)
	}
}

func TestOrcaInventoryPreservesOwnershipAndRejectsIncompleteEvidence(t *testing.T) {
	raw, err := OrcaPayload([]byte(`{"ok":true,"result":{"worktrees":[{"id":"worktree","repoId":"repo","path":"C:/owned"}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	worktrees, err := OrcaWorktrees(raw)
	if err != nil || len(worktrees) != 1 || worktrees[0] != (OrcaWorktree{ID: "worktree", RepoID: "repo", Path: "C:/owned"}) {
		t.Fatalf("ownership changed: %+v / %v", worktrees, err)
	}
	if _, err := OrcaWorktrees(json.RawMessage(`{"worktrees":[],"truncated":true}`)); err == nil {
		t.Fatal("truncated cleanup inventory accepted")
	}
	if _, err := OrcaPayload([]byte(`{"ok":false,"error":{"message":"rejected"}}`)); err == nil {
		t.Fatal("native rejection accepted")
	}
}

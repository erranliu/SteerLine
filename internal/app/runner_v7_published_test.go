//go:build runneracceptance

package app

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnerstatepath"
)

// Opt-in integration with an independently built, pinned public Runner. No
// business Run, model, Git publication or installed application is involved.
func TestPublishedRunnerApplicationLifecycle(t *testing.T) {
	exe, version, sha := os.Getenv("STEERRUNNER_EXE"), os.Getenv("STEERRUNNER_VERSION"), os.Getenv("STEERRUNNER_SHA256")
	if exe == "" || version == "" || sha == "" {
		t.Fatal("requires an independent pinned current-format Runner")
	}
	home := t.TempDir()
	g := RunnerExecutionConfig{Version: 3, Runner: runnerclient.Selection{Executable: exe, Version: version, SHA256: sha}, DefaultProfile: "idle", Profiles: []RunnerProfile{{ID: "idle", Name: "Idle", Execution: runnerclient.ExecutionInput{Adapter: "codex-app-server", Target: runnerclient.ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{"access_mode":"full_access"}`)}, Workflow: runnerclient.Workflow{ID: "development", Version: 2}}}}
	t.Logf("isolated home=%s state_dir=%s Runner=%s SHA256=%s", home, runnerstatepath.Directory(home), exe, sha)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	var first, second applicationRunner
	t.Cleanup(func() {
		if err := first.shutdown(context.Background()); err != nil {
			t.Error(err)
			return
		}
		if err := second.shutdown(context.Background()); err != nil {
			t.Error(err)
			return
		}
		last := &first
		if second.attempted {
			last = &second
		}
		if last.attempted {
			if !last.result().Settled() {
				t.Error("original idle service shutdown unverified; preserve namespace")
				return
			}
			proof, err := last.client.VerifyStopped(context.Background())
			if err != nil || !proof.ResourcesSettled || len(proof.Runs) != 0 {
				t.Error("public idle settlement unverified", proof, err)
				return
			}
			if err := runnerstatepath.RemoveStopped(home); err != nil {
				t.Error(err)
			}
		}
	})
	require(t, first.start(ctx, g, home, 15*time.Second))
	if first.info.Status != "ready" || first.catalog.Capabilities.Recovery.StateFormat != runnerclient.StateFormat {
		t.Fatal("application did not bind the public current-format service")
	}
	require(t, first.shutdown(ctx))
	if !first.result().Settled() {
		t.Fatal("first idle service exit unverified", first.result())
	}
	require(t, second.start(ctx, g, home, 15*time.Second))
	if second.info.InstanceID == first.info.InstanceID || second.client.SHA256 != sha {
		t.Fatal("same-build restart lost its instance/release binding")
	}
	require(t, second.shutdown(ctx))
	if !second.result().Settled() {
		t.Fatal("restarted idle service exit unverified", second.result())
	}
}

package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	"github.com/erranliu/SteerLine/internal/execution/adapters/orca"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func TestOrcaSelectedTargetMatchesRestoredLifecycle(t *testing.T) {
	var emptyTarget string
	var emptyDigest string
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"null args", nil},
		{"empty args", []string{}},
		{"nonempty args", []string{"--native"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, commit := nativeTaskFixture(t)
			terminal := &orcaworker.ProfileConfig{Executable: "orca.exe", Args: tc.args, Shell: "powershell.exe", WaitTimeout: "15m"}
			profile := boundProfile(c, testExecutionProfile(t, "orca", "Orca", "orca_supervised", "codex", codexworker.ProfileConfig{Executable: "codex.exe"}, terminal))
			selected := c.ApplyExecutionProfile(profile)
			config := selected.executionConfig()
			entry, err := selected.ExecutionDirectory.Resolve(config)
			require(t, err)
			target := entry.Target(config)
			if tc.args == nil {
				emptyTarget = target.ID
				emptyDigest = profile.Digest()
			} else if len(tc.args) == 0 && target.ID != emptyTarget {
				t.Fatalf("empty args changed target: %q != %q", target.ID, emptyTarget)
			} else if len(tc.args) > 0 && profile.Digest() == emptyDigest {
				t.Fatal("native arguments lost frozen configuration digest")
			}
			// Reopen the persisted profile carrier before lifecycle construction.
			raw, err := json.Marshal(config.ImplementationData)
			require(t, err)
			var restored []executionbackend.ImplementationDataEnvelope
			require(t, json.Unmarshal(raw, &restored))
			snapshot := executionbackend.LifecycleSnapshot{
				Ref:                 executionbackend.ExecutionRef{Backend: entry.Key.Backend, Protocol: entry.Key.Protocol, Target: target, Agent: executionbackend.AgentSpec{Name: "codex"}},
				Session:             executionbackend.SessionContext{ProjectID: selected.Identity(), TaskID: "101", WorkflowRunID: strings.Repeat("1", 32), ExecutionID: strings.Repeat("2", 32), AttemptID: strings.Repeat("3", 32)},
				ConfigurationDigest: profile.Digest(), ResourceGeneration: strings.Repeat("4", 32),
				WorkspacePlan: executionbackend.WorkspaceIntent{Path: filepath.Join(t.TempDir(), "worker"), Branch: "codex/101", BaseCommit: commit},
			}
			setup := executionbackend.LifecycleSetup{Snapshot: snapshot, ProfileData: restored, ProfileID: profile.ID,
				AcceptedConfigVersion: profile.AcceptedConfigVersion, ProcessHost: selected.ProcessHost, RepositoryPath: selected.Repository}
			port, err := entry.Lifecycle(setup.Clone())
			if err != nil {
				t.Fatalf("selected Orca profile rejected before workspace creation: %v", err)
			}
			step, err := port.Next(context.Background(), snapshot.Clone())
			if err != nil || step.Action != executionbackend.LifecyclePrepareWorkspace {
				t.Fatalf("selected target could not reach workspace creation: %+v, %v", step, err)
			}
			setup.Snapshot.Ref.Target.ID = target.ID + "-different"
			if _, err := entry.Lifecycle(setup); err == nil {
				t.Fatal("mismatched frozen target was accepted")
			}
		})
	}
}

func TestOrcaWorktreeMetadataKeepsNativeQueriesReadOnly(t *testing.T) {
	c, _ := nativeTaskFixture(t)
	store, err := OpenNativeTaskProject(context.Background(), &c)
	require(t, err)
	require(t, store.Close())
	client, err := OpenClient(context.Background(), c.ConfigPath)
	require(t, err)
	before, err := client.ListTasks(context.Background(), "", 100)
	require(t, err)
	require(t, client.Close())
	metadata := filepath.Join(c.RunDir, "worktrees", orcaworker.WorktreeTrashDirectory)
	require(t, os.MkdirAll(metadata, 0700))
	evidence := filepath.Join(metadata, "native-retained.txt")
	require(t, os.WriteFile(evidence, []byte("keep native contents"), 0600))
	client, err = OpenClient(context.Background(), c.ConfigPath)
	require(t, err)
	defer client.Close()
	after, err := client.ListTasks(context.Background(), "", 100)
	require(t, err)
	if after.DatabaseID != before.DatabaseID || after.DatabaseRevision != before.DatabaseRevision || after.AuthorityVersion != before.AuthorityVersion || after.GraphRevision != before.GraphRevision || len(after.Tasks) != len(before.Tasks) {
		t.Fatal("metadata was adopted as authoritative state or changed the database")
	}
	content, err := os.ReadFile(evidence)
	require(t, err)
	if string(content) != "keep native contents" {
		t.Fatal("native trash contents changed")
	}
}

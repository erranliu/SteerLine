package steerlineapp

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline"
	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/publication"
)

type catalogCountingHost struct {
	executionapi.ManagedProcessHost
	calls int
}

func (h *catalogCountingHost) StartManaged(c *exec.Cmd) (executionapi.StopHandle, error) {
	h.calls++
	return h.ManagedProcessHost.StartManaged(c)
}
func (h *catalogCountingHost) OutputCommand(ctx context.Context, c executionapi.LaunchCommand) ([]byte, error) {
	h.calls++
	return h.ManagedProcessHost.OutputCommand(ctx, c)
}
func (h *catalogCountingHost) WriteExecutionFile(path string, data []byte) error {
	h.calls++
	return h.ManagedProcessHost.WriteExecutionFile(path, data)
}

func TestCD66ExplicitCatalogAndEntrypoints(t *testing.T) {
	host := &catalogCountingHost{ManagedProcessHost: NewProcessHost(time.Second)}
	directory, err := NewBuiltinDirectory(host)
	if err != nil {
		t.Fatal(err)
	}
	policy := executionapi.ExecutionPolicy{Filesystem: executionapi.FilesystemPolicy{ReadScope: "host", WriteScope: "workspace", GitMetadataWrite: true}, Network: "allow", Escalation: "deny"}
	payload, err := codexworker.EncodeProfileConfig(codexworker.ProfileConfig{Executable: "must-not-be-called", WindowsSandbox: "elevated"})
	if err != nil {
		t.Fatal(err)
	}
	input := executionapi.ImplementationConfigInput{ImplementationKey: codexworker.ProfileCodecKey, CodecVersion: 1, Payload: payload}
	codec := steerline.ImplementationProfileCodec{Key: codexworker.ProfileCodecKey, Version: 1, Normalize: func(data []byte, p executionapi.ExecutionPolicy, h, a string) ([]byte, []string, error) {
		return codexworker.NormalizeProfilePayload(data, p, "windows", h, a)
	}}
	entry, err := directory.Resolve(steerline.Config{Host: "local", Agent: "codex"})
	if err != nil {
		t.Fatal(err)
	}
	t.Run("missing_directory_and_ports_reject_before_state", func(t *testing.T) {
		options := ConsoleOptions("focused-run")
		if options.Query == nil || options.Control == nil || options.RunID != "focused-run" || options.ApplicationHome != HomeDir() {
			t.Fatal("console entry retained hidden default sources or paths")
		}
		if _, err := NewCommandService(nil, steerline.Config{}); err == nil {
			t.Fatal("command entry accepted missing store/project/directory")
		}
		if _, err := NewBuiltinDirectory(nil); err == nil {
			t.Fatal("missing process host was accepted")
		}
		state := filepath.Join(t.TempDir(), "absent-state")
		c := steerline.Config{TaskAuthority: "native", RunDir: state, Repository: "must-not-be-read"}
		if _, err := NewService(c); err == nil {
			t.Fatal("app constructed an implicit catalog")
		}
		if _, err := steerline.NewService(c, nil, nil, nil, nil); err == nil {
			t.Fatal("core accepted missing ports")
		}
		if _, err := steerline.OpenNativeTaskProject(context.Background(), &c); err == nil || !strings.Contains(err.Error(), "directory") {
			t.Fatalf("missing directory reached project initialization: %v", err)
		}
		if _, err := os.Stat(state); !os.IsNotExist(err) {
			t.Fatal("rejected construction created state")
		}
		if _, err := CertifyExecutionBackend(context.Background(), steerline.Config{RunDir: state}, []string{"missing.json"}); err == nil || !strings.Contains(err.Error(), "task_authority=native") {
			t.Fatalf("capability entry lost its authority admission: %v", err)
		}
	})
	t.Run("duplicate_keys_versions_and_missing_capability", func(t *testing.T) {
		if _, err := steerline.NewExecutionDirectory("v", []steerline.ImplementationProfileCodec{codec}, []steerline.ExecutionRegistration{entry, entry}); err == nil {
			t.Fatal("duplicate implementation key accepted")
		}
		if _, err := steerline.NewExecutionDirectory("v", []steerline.ImplementationProfileCodec{codec, codec}, []steerline.ExecutionRegistration{entry}); err == nil {
			t.Fatal("duplicate codec key accepted")
		}
		incomplete := entry
		incomplete.PrepareClaim = nil
		if _, err := steerline.NewExecutionDirectory("v", []steerline.ImplementationProfileCodec{codec}, []steerline.ExecutionRegistration{incomplete}); err == nil {
			t.Fatal("missing claim capability accepted")
		}
		if _, err := steerline.NewExecutionDirectory("", []steerline.ImplementationProfileCodec{codec}, []steerline.ExecutionRegistration{entry}); err == nil {
			t.Fatal("unversioned directory accepted")
		}
	})
	t.Run("unknown_key_version_and_duplicate_input_reject_before_target", func(t *testing.T) {
		for _, inputs := range [][]executionapi.ImplementationConfigInput{{{ImplementationKey: "unknown", CodecVersion: 1, Payload: payload}}, {{ImplementationKey: input.ImplementationKey, CodecVersion: 2, Payload: payload}}, {input, input}} {
			if _, _, err := directory.NormalizeProfile(inputs, policy, "local", "codex", "project", "profile", "accepted"); err == nil {
				t.Fatal("invalid codec input accepted")
			}
		}
		if _, err := directory.Resolve(steerline.Config{Host: "missing", Agent: "codex"}); err == nil {
			t.Fatal("unknown route accepted")
		}
		if _, err := directory.ResolveIdentity(steerline.ExecutionRegistrationKey{Backend: "missing", Protocol: "missing", Agent: "codex"}); err == nil {
			t.Fatal("unknown fixed identity accepted")
		}
		if host.calls != 0 {
			t.Fatalf("catalog rejection performed %d external actions", host.calls)
		}
	})
	t.Run("two_instances_keep_factories_config_and_returned_slices_independent", func(t *testing.T) {
		firstEntry, secondEntry := entry, entry
		firstEntry.Target = func(steerline.Config) executionapi.ExecutionTarget { return executionapi.ExecutionTarget{ID: "first"} }
		secondEntry.Target = func(steerline.Config) executionapi.ExecutionTarget { return executionapi.ExecutionTarget{ID: "second"} }
		first, err := steerline.NewExecutionDirectory("first-v1", []steerline.ImplementationProfileCodec{codec}, []steerline.ExecutionRegistration{firstEntry})
		if err != nil {
			t.Fatal(err)
		}
		second, err := steerline.NewExecutionDirectory("second-v1", []steerline.ImplementationProfileCodec{codec}, []steerline.ExecutionRegistration{secondEntry})
		if err != nil {
			t.Fatal(err)
		}
		a, err := first.Resolve(steerline.Config{Host: "local", Agent: "codex"})
		if err != nil {
			t.Fatal(err)
		}
		b, err := second.Resolve(steerline.Config{Host: "local", Agent: "codex"})
		if err != nil {
			t.Fatal(err)
		}
		if first == second || a.Target(steerline.Config{}).ID != "first" || b.Target(steerline.Config{}).ID != "second" {
			t.Fatal("catalog instances exchanged factories")
		}
		a.ProfileCodecKeys[0] = "changed"
		a.CleanupCapabilities[0] = "changed"
		fixed, err := first.ResolveIdentity(entry.Key)
		if err != nil {
			t.Fatal(err)
		}
		if fixed.ProfileCodecKeys[0] != codec.Key || fixed.CleanupCapabilities[0] == "changed" {
			t.Fatal("returned metadata mutated the immutable directory")
		}
		data, display, err := first.NormalizeProfile([]executionapi.ImplementationConfigInput{input}, policy, "local", "codex", "project", "profile", "accepted")
		if err != nil {
			t.Fatal(err)
		}
		data[0].Payload[0] = '['
		display[0] = "changed"
		fresh, public, err := second.NormalizeProfile([]executionapi.ImplementationConfigInput{input}, policy, "local", "codex", "project", "profile", "accepted")
		if err != nil {
			t.Fatal(err)
		}
		if fresh[0].Payload[0] == '[' || public[0] == "changed" || fresh[0].ValidateProfileIdentity("project", "profile", "accepted") != nil {
			t.Fatal("profile data or display leaked between instances")
		}
		c := steerline.Config{Repository: "fixture", Branch: "main", Remote: "origin", ExecutionPolicy: policy, PublicationPolicy: executionapi.PublicationPolicy{Publisher: executionapi.PublisherHostService}, PR: publication.Config{Kind: "command", BaseURL: "https://github.com", Repository: "org/repo", Query: contracts.Command{Executable: "gh"}}}
		c.BindApplicationConfig(first, "explicit", "", "accepted", false)
		c = c.ApplyExecutionProfile(steerline.ExecutionProfile{ID: "fixture", Name: "Fixture", Host: "local", Agent: "codex", ImplementationConfigs: []executionapi.ImplementationConfigInput{input}})
		frozen := c.AvailableExecutionProfiles()[0].Freeze()
		if err := c.CheckFixedProfile(frozen); err != nil {
			t.Fatal(err)
		}
		changed := c
		changed.ExecutionDirectory = second
		if changed.CheckFixedProfile(frozen) == nil {
			t.Fatal("a different catalog version reinterpreted the original snapshot")
		}
		if err := c.CheckFixedProfile(frozen); err != nil {
			t.Fatal("second catalog polluted the original fixed profile", err)
		}
	})
	t.Run("config_loader_rejects_missing_key_before_repository_probe", func(t *testing.T) {
		t.Setenv("STEERLINE_HOME", t.TempDir())
		project := map[string]any{"repository": "must-not-be-read", "branch": "main", "task_authority": "native", "workflow_protocol": steerline.WorkerFlowVersion, "execution_policy": policy, "publication_policy": executionapi.PublicationPolicy{Publisher: executionapi.PublisherHostService}, "pr": publication.Config{Kind: "command", BaseURL: "https://github.com", Repository: "org/repo", Query: contracts.Command{Executable: "gh"}}}
		raw, _ := json.Marshal(project)
		path := filepath.Join(t.TempDir(), "project.json")
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		bad := input
		bad.ImplementationKey = "missing"
		raw, _ = json.Marshal(map[string]any{"version": 2, "default_execution_profile": "local", "execution_profiles": []steerline.ExecutionProfile{{ID: "local", Name: "Local", Host: "local", Agent: "codex", ImplementationConfigs: []executionapi.ImplementationConfigInput{bad}}}})
		if err := os.WriteFile(GlobalExecutionConfigPath(), raw, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadConfig(path); err == nil || !strings.Contains(err.Error(), "codec") || strings.Contains(err.Error(), "must-not-be-read") {
			t.Fatalf("invalid directory reached repository I/O: %v", err)
		}
	})
	t.Run("publication_support_uses_the_selected_catalog", func(t *testing.T) {
		github := publication.Config{Kind: "command", BaseURL: "https://github.com", Repository: "owner/repo", Query: contracts.Command{Executable: "gh"}}
		gitea := publication.Config{Kind: "gitea", BaseURL: "https://forge.invalid", Repository: "owner/repo"}
		for _, row := range []struct {
			host, publisher string
			provider        publication.Config
			supported       bool
		}{{"local", "host_service", github, true}, {"local", "worker", github, false}, {"local", "host_service", gitea, false}, {"orca", "worker", github, true}, {"orca", "worker", gitea, true}, {"orca", "host_service", github, false}, {"orca_supervised", "worker", gitea, true}, {"orca_supervised", "host_service", github, false}, {"other", "worker", github, false}, {"orca", "", github, false}, {"orca", "automatic", github, false}, {"orca", "worker", publication.Config{Kind: "command", BaseURL: github.BaseURL, Repository: github.Repository}, false}} {
			selected, err := directory.Resolve(steerline.Config{Host: row.host, Agent: "codex"})
			if err == nil {
				err = selected.ValidatePublicationPolicy(executionapi.PublicationPolicy{Publisher: row.publisher}, row.provider)
			}
			if (err == nil) != row.supported {
				t.Fatalf("%s/%s/%s support=%t error=%v", row.host, row.publisher, row.provider.Kind, row.supported, err)
			}
		}
	})
}

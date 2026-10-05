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

	"github.com/erranliu/SteerLine/internal/execution"
	"github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/publication"
)

type catalogCountingHost struct {
	executionbackend.ManagedProcessHost
	calls int
}

func (h *catalogCountingHost) StartManaged(c *exec.Cmd) (executionbackend.StopHandle, error) {
	h.calls++
	return h.ManagedProcessHost.StartManaged(c)
}

func (h *catalogCountingHost) OutputCommand(ctx context.Context, c executionbackend.LaunchCommand) ([]byte, error) {
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
	policy := executionbackend.ExecutionPolicy{Filesystem: executionbackend.FilesystemPolicy{ReadScope: "host", WriteScope: "workspace", GitMetadataWrite: true}, Network: "allow", Escalation: "deny"}
	payload, err := codexworker.EncodeProfileConfig(codexworker.ProfileConfig{Executable: "must-not-be-called", WindowsSandbox: "elevated"})
	if err != nil {
		t.Fatal(err)
	}
	input := executionbackend.ImplementationConfigInput{ImplementationKey: codexworker.ProfileCodecKey, CodecVersion: 1, Payload: payload}
	codec := execution.ImplementationProfileCodec{Key: codexworker.ProfileCodecKey, Version: 1, Normalize: func(data []byte, p executionbackend.ExecutionPolicy, h, a string) ([]byte, []string, error) {
		return codexworker.NormalizeProfilePayload(data, p, "windows", h, a)
	}}
	entry, err := directory.Resolve(execution.Config{Host: "local", Agent: "codex"})
	if err != nil {
		t.Fatal(err)
	}
	t.Run("missing_directory_and_ports_reject_before_state", func(t *testing.T) {
		t.Setenv("STEERLINE_HOME", t.TempDir())
		_, options, err := PrepareConsole(context.Background(), "", "focused-run")
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			for _, c := range options.Clients {
				_ = c.Close()
			}
		}()
		if options.Clients == nil || options.RunID != "focused-run" || options.ApplicationHome != HomeDir() {
			t.Fatal("console entry retained hidden default sources or paths")
		}
		if _, err := NewCommandService(nil, execution.Config{}); err == nil {
			t.Fatal("command entry accepted missing store/project/directory")
		}
		if _, err := NewBuiltinDirectory(nil); err == nil {
			t.Fatal("missing process host was accepted")
		}
		state := filepath.Join(t.TempDir(), "absent-state")
		c := execution.Config{TaskAuthority: "native", RunDir: state, Repository: "must-not-be-read"}
		if _, err := NewService(c); err == nil {
			t.Fatal("app constructed an implicit catalog")
		}
		if _, err := execution.NewService(c, nil, nil, nil, nil, execution.ServiceStorage{}, nil, nil, nil); err == nil {
			t.Fatal("core accepted missing ports")
		}
		if _, err := OpenNativeTaskProject(context.Background(), &c); err == nil || !strings.Contains(err.Error(), "directory") {
			t.Fatalf("missing directory reached project initialization: %v", err)
		}
		if _, err := os.Stat(state); !os.IsNotExist(err) {
			t.Fatal("rejected construction created state")
		}
		if _, err := CertifyExecutionBackend(context.Background(), execution.Config{RunDir: state}, []string{"missing.json"}); err == nil || !strings.Contains(err.Error(), "task_authority=native") {
			t.Fatalf("capability entry lost its authority admission: %v", err)
		}
		c.TaskAuthority, c.WorkflowProtocol, c.Git.Executable = "native", executionmodel.Version, "must-not-be-called"
		if _, err := CertifyExecutionBackend(context.Background(), c, []string{"missing.json"}); err == nil || !strings.Contains(err.Error(), "directory") {
			t.Fatalf("capability entry reached target without directory: %v", err)
		}
	})
	t.Run("duplicate_keys_versions_and_missing_capability", func(t *testing.T) {
		if _, err := execution.NewExecutionDirectory("v", []execution.ImplementationProfileCodec{codec}, []execution.ExecutionRegistration{entry, entry}); err == nil {
			t.Fatal("duplicate implementation key accepted")
		}
		if _, err := execution.NewExecutionDirectory("v", []execution.ImplementationProfileCodec{codec, codec}, []execution.ExecutionRegistration{entry}); err == nil {
			t.Fatal("duplicate codec key accepted")
		}
		incomplete := entry
		incomplete.PrepareClaim = nil
		if _, err := execution.NewExecutionDirectory("v", []execution.ImplementationProfileCodec{codec}, []execution.ExecutionRegistration{incomplete}); err == nil {
			t.Fatal("missing claim capability accepted")
		}
		if _, err := execution.NewExecutionDirectory("", []execution.ImplementationProfileCodec{codec}, []execution.ExecutionRegistration{entry}); err == nil {
			t.Fatal("unversioned directory accepted")
		}
	})
	t.Run("unknown_key_version_and_duplicate_input_reject_before_target", func(t *testing.T) {
		for _, inputs := range [][]executionbackend.ImplementationConfigInput{{{ImplementationKey: "unknown", CodecVersion: 1, Payload: payload}}, {{ImplementationKey: input.ImplementationKey, CodecVersion: 2, Payload: payload}}, {input, input}} {
			if _, _, err := directory.NormalizeProfile(inputs, policy, "local", "codex", "project", "profile", "accepted"); err == nil {
				t.Fatal("invalid codec input accepted")
			}
		}
		if _, err := directory.Resolve(execution.Config{Host: "missing", Agent: "codex"}); err == nil {
			t.Fatal("unknown route accepted")
		}
		if _, err := directory.ResolveIdentity(execution.ExecutionRegistrationKey{Backend: "missing", Protocol: "missing", Agent: "codex"}); err == nil {
			t.Fatal("unknown fixed identity accepted")
		}
		if host.calls != 0 {
			t.Fatalf("catalog rejection performed %d external actions", host.calls)
		}
	})
	t.Run("two_instances_keep_factories_config_and_returned_slices_independent", func(t *testing.T) {
		firstEntry, secondEntry := entry, entry
		firstEntry.Target = func(execution.Config) executionbackend.ExecutionTarget {
			return executionbackend.ExecutionTarget{ID: "first"}
		}
		secondEntry.Target = func(execution.Config) executionbackend.ExecutionTarget {
			return executionbackend.ExecutionTarget{ID: "second"}
		}
		first, err := execution.NewExecutionDirectory("first-v1", []execution.ImplementationProfileCodec{codec}, []execution.ExecutionRegistration{firstEntry})
		if err != nil {
			t.Fatal(err)
		}
		second, err := execution.NewExecutionDirectory("second-v1", []execution.ImplementationProfileCodec{codec}, []execution.ExecutionRegistration{secondEntry})
		if err != nil {
			t.Fatal(err)
		}
		a, err := first.Resolve(execution.Config{Host: "local", Agent: "codex"})
		if err != nil {
			t.Fatal(err)
		}
		b, err := second.Resolve(execution.Config{Host: "local", Agent: "codex"})
		if err != nil {
			t.Fatal(err)
		}
		if first == second || a.Target(execution.Config{}).ID != "first" || b.Target(execution.Config{}).ID != "second" {
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
		data, display, err := first.NormalizeProfile([]executionbackend.ImplementationConfigInput{input}, policy, "local", "codex", "project", "profile", "accepted")
		if err != nil {
			t.Fatal(err)
		}
		data[0].Payload[0] = '['
		display[0] = "changed"
		fresh, public, err := second.NormalizeProfile([]executionbackend.ImplementationConfigInput{input}, policy, "local", "codex", "project", "profile", "accepted")
		if err != nil {
			t.Fatal(err)
		}
		if fresh[0].Payload[0] == '[' || public[0] == "changed" || fresh[0].ValidateProfileIdentity("project", "profile", "accepted") != nil {
			t.Fatal("profile data or display leaked between instances")
		}
		c := execution.Config{Repository: "fixture", Branch: "main", Remote: "origin", ExecutionPolicy: policy, PublicationPolicy: executionbackend.PublicationPolicy{Publisher: executionbackend.PublisherWorker}, PR: publication.Config{Kind: "command", BaseURL: "https://github.com", Repository: "org/repo", Query: executionbackend.Command{Executable: "gh"}}}
		c.BindApplicationConfig(first, "explicit", "", "accepted", false)
		c = c.ApplyExecutionProfile(execution.ExecutionProfile{ID: "fixture", Name: "Fixture", Host: "local", Agent: "codex", ImplementationConfigs: []executionbackend.ImplementationConfigInput{input}})
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
		project := map[string]any{"repository": "must-not-be-read", "branch": "main", "task_authority": "native", "workflow_protocol": executionmodel.Version, "execution_policy": policy, "publication_policy": executionbackend.PublicationPolicy{Publisher: executionbackend.PublisherWorker}, "pr": publication.Config{Kind: "command", BaseURL: "https://github.com", Repository: "org/repo", Query: executionbackend.Command{Executable: "gh"}}}
		raw, _ := json.Marshal(project)
		path := filepath.Join(t.TempDir(), "project.json")
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		bad := input
		bad.ImplementationKey = "missing"
		raw, _ = json.Marshal(map[string]any{"version": 2, "default_execution_profile": "local", "execution_profiles": []execution.ExecutionProfile{{ID: "local", Name: "Local", Host: "local", Agent: "codex", ImplementationConfigs: []executionbackend.ImplementationConfigInput{bad}}}})
		if err := os.WriteFile(GlobalExecutionConfigPath(), raw, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadConfig(path); err == nil || !strings.Contains(err.Error(), "codec") || strings.Contains(err.Error(), "must-not-be-read") {
			t.Fatalf("invalid directory reached repository I/O: %v", err)
		}
	})
	t.Run("publication_support_uses_the_selected_catalog", func(t *testing.T) {
		github := publication.Config{Kind: "command", BaseURL: "https://github.com", Repository: "owner/repo", Query: executionbackend.Command{Executable: "gh"}}
		gitea := publication.Config{Kind: "gitea", BaseURL: "https://forge.invalid", Repository: "owner/repo"}
		for _, row := range []struct {
			host, publisher string
			provider        publication.Config
			supported       bool
		}{{"local", "host_service", github, false}, {"local", "worker", github, true}, {"local", "worker", gitea, true}, {"local", "host_service", gitea, false}, {"orca", "worker", github, true}, {"orca", "worker", gitea, true}, {"orca", "host_service", github, false}, {"orca_supervised", "worker", gitea, true}, {"orca_supervised", "host_service", github, false}, {"other", "worker", github, false}, {"orca", "", github, false}, {"orca", "automatic", github, false}, {"orca", "worker", publication.Config{Kind: "command", BaseURL: github.BaseURL, Repository: github.Repository}, false}} {
			selected, err := directory.Resolve(execution.Config{Host: row.host, Agent: "codex"})
			if err == nil {
				err = selected.ValidatePublicationPolicy(executionbackend.PublicationPolicy{Publisher: row.publisher}, row.provider)
			}
			if (err == nil) != row.supported {
				t.Fatalf("%s/%s/%s support=%t error=%v", row.host, row.publisher, row.provider.Kind, row.supported, err)
			}
		}
	})
}

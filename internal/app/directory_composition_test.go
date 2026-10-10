package app

import (
	"context"
	"errors"
	"os/exec"
	"reflect"
	"testing"

	"github.com/erranliu/SteerLine/internal/execution"
	codexworker "github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	orcaworker "github.com/erranliu/SteerLine/internal/execution/adapters/orca"
	remote "github.com/erranliu/SteerLine/internal/execution/adapters/orcaremote"
	b "github.com/erranliu/SteerLine/internal/execution/backend"
)

var compositionProbeStopped = errors.New("composition probe stopped before process start")

type compositionHost struct {
	b.ManagedProcessHost
	starts, inspections int
	command             []string
	liveness            string
}

func (h *compositionHost) StartManaged(cmd *exec.Cmd) (b.StopHandle, error) {
	h.starts++
	h.command = append([]string(nil), cmd.Args...)
	return nil, compositionProbeStopped
}

func (h *compositionHost) InspectProcess(*b.ProcessIdentity) string {
	h.inspections++
	return h.liveness
}

func TestBuiltinCompositionIndependentCatalogsAndHosts(t *testing.T) {
	first := &compositionHost{liveness: "matching process still exists"}
	second := &compositionHost{liveness: "process absent"}
	d1, err := NewBuiltinDirectory(first)
	require(t, err)
	d2, err := NewBuiltinDirectory(second)
	require(t, err)
	if d1 == d2 || d1.Version() != "builtin-execution-v4" || d2.Version() != d1.Version() || first.starts+second.starts+first.inspections+second.inspections != 0 {
		t.Fatal("construction probed a backend or reused its catalog")
	}
	if _, err := NewBuiltinDirectory(nil); err == nil {
		t.Fatal("missing process host accepted")
	}
	for i, d := range []*execution.ExecutionDirectory{d1, d2} {
		entry, err := d.Resolve(execution.Config{Host: "local", Agent: "codex"})
		require(t, err)
		if entry.Key != (execution.ExecutionRegistrationKey{Backend: b.BackendLocal, Agent: "codex", Protocol: b.ProtocolLocalCodexAppServer}) || entry.ProfileKey != "local/local_codex_app_server_v1/codex" || !entry.LocalWorkspace || entry.HostManagedPermissions || entry.ServiceStopBehavior != codexworker.ServiceStopBehavior || !reflect.DeepEqual(entry.ProfileCodecKeys, []string{codexworker.ProfileCodecKey}) {
			t.Fatal("local registration changed", entry.Key)
		}
		backend := entry.Backend(execution.Config{Host: "local", Agent: "codex"})
		local := backend.(localExecutionBackend)
		if local.directory != d {
			t.Fatal("factory captured another catalog")
		}
		ref := b.ExecutionRef{Backend: b.BackendLocal, Protocol: b.ProtocolLocalCodexAppServer, Target: entry.Target(execution.Config{}), External: b.ProcessIdentity{Kind: "os", PID: 123, Created: "original", Executable: "codex"}}
		observed, err := backend.Observe(context.Background(), ref)
		require(t, err)
		want := []string{"live", "exited"}[i]
		if observed.Liveness != want {
			t.Fatal("factory consulted another process host", observed)
		}
	}
	if first.inspections != 1 || second.inspections != 1 || first.starts+second.starts != 0 {
		t.Fatal("observation ownership crossed instances")
	}
}

func TestBuiltinCompositionSupervisedTargetsKeepFactoryAndFrozenConfig(t *testing.T) {
	for _, placement := range []string{"local", "environment", ""} {
		t.Run("target="+placement, func(t *testing.T) {
			first, second := &compositionHost{}, &compositionHost{}
			directory, err := NewBuiltinDirectory(first)
			require(t, err)
			_, err = NewBuiltinDirectory(second)
			require(t, err)
			c := bindUnitDirectory(t, Config{Host: "orca_supervised", Agent: "claude", Repository: t.TempDir()})
			c.ProcessHost, c.ExecutionDirectory = first, directory
			settings := remote.ProfileConfig{Target: placement, ConnectionID: "pair", EnvironmentID: "environment", RepositorySelector: "id:repo", PollInterval: "1s", CapabilityReportVersion: 1}
			if placement == "local" {
				settings.ConnectionID, settings.EnvironmentID, settings.RepositorySelector = "", "", ""
			}
			profile := boundProfile(c, testExecutionProfile(t, "supervised", "Supervised", c.Host, c.Agent, codexworker.ProfileConfig{Model: "original"}, &orcaworker.ProfileConfig{Executable: "original-orca.exe", Args: []string{"--original"}, Shell: "pwsh", WaitTimeout: "2m"}, &settings))
			selected := c.ApplyExecutionProfile(profile)
			cfg := selected.executionConfig()
			entry, err := directory.Resolve(cfg)
			require(t, err)
			if entry.Key != (execution.ExecutionRegistrationKey{Backend: b.BackendOrca, Agent: c.Agent, Protocol: b.ProtocolOrcaSupervised}) || entry.ProfileKey != "orca/orca_supervised" || entry.LocalWorkspace || entry.HostManagedPermissions || entry.ServiceStopBehavior != remote.ServiceStopBehavior || entry.ReadinessRequired(cfg) || !reflect.DeepEqual(entry.ProfileCodecKeys, []string{remote.ProfileCodecKey}) || entry.BuildPolicyResource == nil || entry.VerifyPolicyResource == nil {
				t.Fatal("supervised inherited another implementation's policy")
			}
			fixed := selected.ApplyFixedExecutionProfile(profile.Freeze()).executionConfig()
			target := entry.Target(fixed)
			want := (remote.Config{Settings: remote.Settings{Target: placement, ConnectionID: settings.ConnectionID, EnvironmentID: settings.EnvironmentID}}).Target()
			if target != want {
				t.Fatal("frozen profile routed to another target", target, want)
			}
			backend := entry.Backend(fixed)
			if _, ok := backend.(remote.Backend); !ok {
				t.Fatal("target installed a second implementation")
			}
			wrong := target
			wrong.ID += "-different"
			if _, err := backend.Probe(context.Background(), wrong, b.AgentSpec{Name: c.Agent}); err == nil || first.starts+second.starts != 0 {
				t.Fatal("wrong target reached a process")
			}
			// Only the matching frozen request reaches the originally supplied host.
			_, err = backend.Probe(context.Background(), target, b.AgentSpec{Name: c.Agent})
			if !errors.Is(err, compositionProbeStopped) || first.starts != 1 || second.starts != 0 || !reflect.DeepEqual(first.command, []string{"original-orca.exe", "--original", "--version"}) {
				t.Fatal("factory lost its frozen command or host", first.command, err)
			}
			claim, err := entry.PrepareClaim(context.Background(), fixed)
			require(t, err)
			if claim.RequiresReadiness || claim.WaitTimeout != 0 || first.starts != 1 {
				t.Fatal("claim introduced readiness or repeated preflight")
			}
		})
	}
}

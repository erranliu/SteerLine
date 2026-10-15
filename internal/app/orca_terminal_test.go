package app

import (
	"context"
	"encoding/json"
	"os"

	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/execution"
	terminal "github.com/erranliu/SteerLine/internal/execution/adapters/orcatransport"
	b "github.com/erranliu/SteerLine/internal/execution/backend"
)

func TestTerminalRegistrationRejectsRetiredRoutesAndAgentPrivateFields(t *testing.T) {
	d, err := NewBuiltinDirectory(NewProcessHost(time.Second))
	require(t, err)
	for _, agent := range []string{"codex", "claude", "opaque-agent"} {
		entry, err := d.Resolve(execution.Config{Host: "orca", Agent: agent})
		require(t, err)
		if entry.Key.Protocol != b.ProtocolOrcaTerminalV3 || entry.LocalWorkspace || !entry.HostManagedPermissions || len(entry.ProfileCodecKeys) != 1 || entry.ProfileCodecKeys[0] != terminal.ProfileKey {
			t.Fatal("Orca inherited a provider or orchestration route", entry)
		}
		payload := mustJSON(terminal.Profile{Executable: "orca.exe", WaitTimeout: "30s"})
		_, _, err = d.NormalizeProfile([]b.ImplementationConfigInput{{ImplementationKey: terminal.ProfileKey, CodecVersion: terminal.ProfileVersion, Payload: payload}}, "orca", agent, "project", "profile", "accepted")
		require(t, err)
	}
	if _, err = d.Resolve(execution.Config{Host: "orca_supervised", Agent: "codex"}); err == nil {
		t.Fatal("retired route remained registered")
	}
	if _, err = d.ResolveIdentity(execution.ExecutionRegistrationKey{Backend: b.BackendOrca, Agent: "codex", Protocol: b.ExecutionProtocol("orca_supervised")}); err == nil {
		t.Fatal("retired frozen execution remained routable")
	}
	for _, extra := range []string{`"repository_selector":"id:repo"`, `"target":"environment"`, `"model":"model"`, `"reasoning_effort":"high"`, `"return_origin":"http://localhost:1234"`, `"agent_command":"opaque"`} {
		data := []byte(`{"executable":"orca","wait_timeout":"30s",` + extra + `}`)
		if _, err = terminal.DecodeProfile(data); err == nil {
			t.Fatal("unsupported override was silently ignored", extra)
		}
	}
}

func TestCurrentExampleProfilesFreezeOpaqueOrcaCommands(t *testing.T) {
	t.Setenv("STEERLINE_HOME", t.TempDir())
	raw, err := os.ReadFile("../../examples/execution.json")
	require(t, err)
	var file struct {
		Version  int                `json:"version"`
		Default  string             `json:"default_execution_profile"`
		Profiles []ExecutionProfile `json:"execution_profiles"`
	}
	require(t, json.Unmarshal(raw, &file))
	d, err := NewBuiltinDirectory(NewProcessHost(time.Second))
	require(t, err)
	if len(file.Profiles) != 4 {
		t.Fatal("example profiles lost")
	}
	for _, p := range file.Profiles {
		c := bindUnitDirectory(t, Config{Repository: t.TempDir(), ExecutionDirectory: d})
		p = boundProfile(c, p)
		fixed := p.Freeze(d, c.Identity())
		require(t, c.CheckFixedProfile(fixed))
		if p.Host == "orca" {
			payload, err := c.ApplyExecutionProfile(p).executionConfig().DecodeImplementationConfig(terminal.ProfileKey, d)
			require(t, err)
			_, err = terminal.DecodeProfile(payload)
			require(t, err)
			p.Agent = "changed"
			selected := c.ApplyFixedExecutionProfile(fixed)
			if selected.Agent != fixed.Agent || selected.Host != "orca" {
				t.Fatal("mutable selection redirected frozen profile")
			}
		}
	}
}

func TestTerminalFactoryKeepsFrozenCommandAndCatalog(t *testing.T) {
	first, second := &compositionHost{}, &compositionHost{}
	d, err := NewBuiltinDirectory(first)
	require(t, err)
	_, err = NewBuiltinDirectory(second)
	require(t, err)
	c := bindUnitDirectory(t, Config{Host: "orca", Agent: "opaque-agent", Repository: t.TempDir()})
	c.ExecutionDirectory = d
	c.ProcessHost = first
	p := ExecutionProfile{ID: "profile", Host: "orca", Agent: c.Agent, ImplementationConfigs: []b.ImplementationConfigInput{{ImplementationKey: terminal.ProfileKey, CodecVersion: terminal.ProfileVersion, Payload: mustJSON(terminal.Profile{Executable: "frozen-orca.exe", Args: []string{"--fixed"}, WaitTimeout: "30s"})}}}
	p = boundProfile(c, p)
	selected := c.ApplyExecutionProfile(p).executionConfig()
	entry, err := d.Resolve(selected)
	require(t, err)
	wrong := entry.Target(selected)
	wrong.ID += "changed"
	if _, err = entry.Backend(selected).Probe(context.Background(), wrong, b.AgentSpec{Name: c.Agent}); err == nil {
		t.Fatal("wrong target reached the native host")
	}
	if first.starts+second.starts != 0 {
		t.Fatal("catalog construction or rejected probe launched anything")
	}
}

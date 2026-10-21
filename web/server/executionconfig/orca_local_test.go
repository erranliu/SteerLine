package executionconfig

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	cfg "github.com/erranliu/SteerLine/internal/executionconfig"
	"github.com/erranliu/SteerLine/internal/localservice"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func TestLocalOrcaConfiguration(t *testing.T) {
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		t.Skip("local mapping targets Windows amd64")
	}
	s, c := fixture(t)
	s.ReadCatalog = func(context.Context, cfg.Pin) (*catalog, error) {
		raw, err := os.ReadFile("testdata/local-orca-capabilities.json")
		if err != nil {
			return nil, err
		}
		var v struct {
			Schemas struct {
				Run      json.RawMessage            `json:"run_configuration"`
				Adapters map[string]json.RawMessage `json:"adapters"`
			} `json:"configuration_schemas"`
			Workflows []runnercontract.WorkflowChoice `json:"workflows"`
		}
		if err = json.Unmarshal(raw, &v); err != nil {
			return nil, err
		}
		return compileCatalog(v.Schemas.Run, v.Workflows, v.Schemas.Adapters["codex-app-server"])
	}
	ctx := context.Background()
	helper := filepath.Join(s.Home, "deployment-helper.exe")
	body := []byte("trusted deployment fixture; never executed")
	if err := os.WriteFile(helper, body, 0600); err != nil {
		t.Fatal(err)
	}
	original := c.Profiles[0]
	template := original
	template.ID = "orca-template"
	template.Name = "本机 Orca 模板"
	template.Execution.Adapter = "orca-terminal"
	settings := func(agent, path, sha string) json.RawMessage {
		raw, _ := json.Marshal(map[string]any{"agent": agent, "transport": map[string]any{"executable": "orca.exe"}, "agent_launch": map[string]any{"executable": map[string]string{"codex": "codex", "pi": "pi.cmd", "dsh": "dsh.cmd"}[agent], "model": "manual-model", "reasoning_effort": "high", "runtime_executable": path, "runtime_sha256": sha}})
		return raw
	}
	template.Execution.Settings = settings("codex", helper, cfg.Digest(body))
	c.Profiles = append(c.Profiles, template)
	if err := localservice.WritePrivateFile(s.Home, "execution.json", c); err != nil {
		t.Fatal(err)
	}
	snap, err := s.Snapshot(ctx)
	if err != nil || !snap.LocalHelper.Available {
		t.Fatal("helper unavailable", err)
	}
	for _, agent := range []string{"codex", "pi", "dsh"} {
		t.Run(agent, func(t *testing.T) {
			p := template
			p.ID = "new-" + agent
			p.Execution.Settings = settings(agent, helper, cfg.Digest(body))
			next := c
			next.Profiles = append(append([]cfg.Profile{}, c.Profiles...), p)
			if _, err := s.Validate(ctx, encode(t, next)); err != nil {
				t.Fatal("native mapping rejected", err)
			}
			audited := p
			audited.Workflow = runnercontract.Workflow{ID: "development-audited", Version: 2, AuditModel: &runnercontract.ModelSettings{Model: "audit-model"}}
			next.Profiles[len(next.Profiles)-1] = audited
			_, auditErr := s.Validate(ctx, encode(t, next))
			if (auditErr == nil) != (agent == "codex") {
				t.Fatal("wrong audit mapping", agent, auditErr)
			}
			next.Profiles[len(next.Profiles)-1] = p
			found := false
			for _, choice := range snap.Combinations {
				if choice.Host == "local" && choice.Adapter == "orca-terminal" && choice.Agent == agent {
					found = choice.Supported
					if agent != "codex" {
						for _, w := range choice.Workflows {
							if w.AuditModelRequired {
								t.Fatal("audit expanded to unmanaged Agent")
							}
						}
					}
					for _, effort := range choice.Efforts {
						if agent == "pi" && (effort == "max" || effort == "ultra") || agent == "dsh" && (effort == "minimal" || effort == "xhigh" || effort == "ultra") {
							t.Fatal("foreign effort", agent, effort)
						}
					}
				}
			}
			if !found {
				t.Fatal("schema projection absent")
			}
			p.Execution.Settings = settings(agent, helper, strings.Repeat("b", 64))
			next.Profiles[len(next.Profiles)-1] = p
			if _, err := s.Validate(ctx, encode(t, next)); err == nil {
				t.Fatal("wrong helper identity accepted")
			}
			p.Execution.Settings = settings(agent, filepath.Join(s.Home, "arbitrary.exe"), cfg.Digest(body))
			next.Profiles[len(next.Profiles)-1] = p
			if _, err := s.Validate(ctx, encode(t, next)); err == nil {
				t.Fatal("browser supplied helper accepted")
			}
			p.Execution.Target.Kind = "orca_environment"
			p.Execution.Target.Environment = "remote"
			next.Profiles[len(next.Profiles)-1] = p
			if _, err := s.Validate(ctx, encode(t, next)); err == nil {
				t.Fatal("remote helper accepted")
			}
		})
	}
	for _, raw := range []string{`{"agent":"codex","agent_launch":{"runtime_executable":""}}`, `{"agent":"codex","agent_launch":{"runtime_sha256":""}}`} {
		remote := template
		remote.ID = "remote-empty-runtime"
		remote.Execution.Target.Kind = "orca_environment"
		remote.Execution.Target.Environment = "paired"
		remote.Execution.Settings = json.RawMessage(raw)
		next := c
		next.Profiles = append(append([]cfg.Profile{}, c.Profiles...), remote)
		if _, err := s.Validate(ctx, encode(t, next)); err == nil {
			t.Fatal("empty remote helper key accepted")
		}
	}
	// Discovery can expose remote agents without granting execution mappings.
	for _, agent := range []string{"pi", "dsh", "custom-agent"} {
		remote := template
		remote.ID = "remote-" + agent
		remote.Execution.Target.Kind = "orca_environment"
		remote.Execution.Target.Environment = "paired"
		remote.Execution.Settings = json.RawMessage(`{"agent":"` + agent + `","agent_launch":{"model":"openrouter/vendor/model"}}`)
		next := c
		next.Profiles = append(append([]cfg.Profile{}, c.Profiles...), remote)
		if _, err := s.Validate(ctx, encode(t, next)); err == nil {
			t.Fatal("native discovery expanded unsupported remote execution", agent)
		}
	}
	for _, field := range []string{"api_key", "provider_credentials", "base_url"} {
		next := c
		next.Profiles = append([]cfg.Profile{}, c.Profiles...)
		raw := map[string]any{"model": "manual-model", field: "fixture-deployment-secret"}
		next.Profiles[0].Execution.Settings, _ = json.Marshal(raw)
		if _, err := s.Validate(ctx, encode(t, next)); err == nil {
			t.Fatal("Web accepted provider credential management", field)
		}
	}
	// Missing deployment material blocks changed local execution, while names,
	// defaults, unrelated routes and exact saved execution values remain usable.
	if err := os.Remove(helper); err != nil {
		t.Fatal(err)
	}
	c.Profiles[1].Name = "改名但不改变执行值"
	if _, err := s.Validate(ctx, encode(t, c)); err != nil {
		t.Fatal("name-only edit blocked", err)
	}
	next := c
	next.Profiles = append([]cfg.Profile{}, c.Profiles...)
	next.Profiles[1].Execution.Settings = settings("codex", helper, cfg.Digest(body))
	var m map[string]any
	json.Unmarshal(next.Profiles[1].Execution.Settings, &m)
	m["agent_launch"].(map[string]any)["model"] = "changed"
	next.Profiles[1].Execution.Settings, _ = json.Marshal(m)
	if _, err := s.Validate(ctx, encode(t, next)); err == nil {
		t.Fatal("changed execution accepted missing helper")
	}
	snap, err = s.Snapshot(ctx)
	if err != nil || snap.LocalHelper.Available {
		t.Fatal("material state untruthful", err)
	}
	c.Profiles[0].Name = "其他链路仍可改名"
	c.DefaultProfile = original.ID
	if _, err := s.Save(ctx, snap.SavedDigest, encode(t, c)); err != nil {
		t.Fatal("unrelated save blocked", err)
	}
}

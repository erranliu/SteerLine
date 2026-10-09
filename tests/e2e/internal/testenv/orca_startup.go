package testenv

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
	"github.com/erranliu/SteerLine/tests/e2e/internal/native"
)

func (p *Project) UseControlledOrcaStartup(mode string) string {
	s := p.Scenario
	state := filepath.Join(p.Root, "orca-startup")
	s.Must(os.MkdirAll(state, 0700))
	s.Must(controlled.ControlledOrcaStartup(state, mode))
	raw, err := os.ReadFile(p.Config)
	s.Must(err)
	var config map[string]any
	s.Must(json.Unmarshal(raw, &config))
	// Scale the ordinary/startup mismatch to 2s/10s; the slow CLI takes 3s.
	config["command_timeout"] = "2s"
	s.Must(controlled.Write(p.Config, config))
	profile := map[string]any{"id": "orca-startup", "name": "Controlled native Orca startup", "host": "orca", "agent": "codex", "implementation_configs": []any{
		map[string]any{"implementation_key": "orca.worker-profile", "codec_version": 1, "payload": map[string]any{
			"executable": s.Run.Helper, "args": []string{"native-orca", state}, "shell": "powershell.exe", "wait_timeout": "10s", "model": native.Model, "reasoning_effort": native.Effort,
		}},
	}}
	s.Must(controlled.Write(filepath.Join(p.Home, "execution.json"), map[string]any{"version": 2, "execution_profiles": []any{profile}, "default_execution_profile": "orca-startup"}))
	p.Driver.Profile = "orca-startup"
	return state
}

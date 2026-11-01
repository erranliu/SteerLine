package model_test

import (
	"encoding/json"
	"strings"
	"testing"

	model "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
)

func TestEnvironmentRequirementsCarryBlockingGapsWithoutGenerationHistory(t *testing.T) {
	key := strings.Repeat("a", 64)
	m := model.Manifest{Version: 3, EnvironmentKey: key, Script: model.ScriptPath(key, "sh"), Interpreter: "bash", ValidationBase: "validation", RequiredChecks: []string{"tool.runtime"}, Gaps: []string{"runtime version is not declared"}, Dependencies: []string{}}
	parse := func(m model.Manifest) (model.Manifest, error) {
		raw, _ := json.Marshal(m)
		return model.ParseManifest([]byte("```steerline-environment\n" + string(raw) + "\n```"))
	}
	got, err := parse(m)
	if err != nil || got.Ready() == nil {
		t.Fatal("standalone document did not block", got, err)
	}
	m.Gaps = []string{}
	got, err = parse(m)
	if err != nil || got.Ready() != nil {
		t.Fatal("resolved document rejected", got, err)
	}
	for _, change := range []func(*model.Manifest){
		func(m *model.Manifest) { m.Version = 2 }, func(m *model.Manifest) { m.Gaps = nil }, func(m *model.Manifest) { m.RequiredChecks = nil }, func(m *model.Manifest) { m.RequiredChecks = []string{"terminal.tty"} }, func(m *model.Manifest) { m.RequiredChecks = []string{"tool.runtime", "tool.runtime"} }, func(m *model.Manifest) { m.Gaps = []string{""} },
	} {
		bad := m
		change(&bad)
		if _, err := parse(bad); err == nil {
			t.Fatal("invalid requirement contract accepted", bad)
		}
	}
}

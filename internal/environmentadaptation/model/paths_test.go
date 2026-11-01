package model_test

import (
	"encoding/json"
	"strings"
	"testing"

	model "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
)

func TestEnvironmentMaterialManifestRejectsCrossTargetAndGenerationDependencies(t *testing.T) {
	key := strings.Repeat("a", 64)
	base := model.Manifest{Version: 2, EnvironmentKey: key, Script: model.ScriptPath(key, "ps1"), Interpreter: "powershell.exe", ValidationBase: "validation", Dependencies: []string{}}
	parse := func(m model.Manifest) error {
		body, _ := json.Marshal(m)
		_, err := model.ParseManifest([]byte("```steerline-environment\n" + string(body) + "\n```"))
		return err
	}
	if err := parse(base); err != nil {
		t.Fatal(err)
	}
	for _, dependency := range []string{model.GenerationContextPath(key), model.GenerationRulesPath, model.DocumentPath(strings.Repeat("b", 64)), "../outside", "C:/outside"} {
		m := base
		m.Dependencies = []string{dependency}
		if parse(m) == nil {
			t.Fatal("unsafe dependency accepted", dependency)
		}
	}
	m := base
	m.Dependencies = nil
	if parse(m) == nil {
		t.Fatal("null dependencies accepted")
	}
	m = base
	m.Script = model.ScriptPath(strings.Repeat("b", 64), "ps1")
	if parse(m) == nil {
		t.Fatal("cross target script accepted")
	}
	if model.DocumentPath("../outside") != "" || model.ValidKey(strings.Repeat("a", 10)) {
		t.Fatal("unsafe path/key accepted")
	}
}

package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEnvironmentMaterialManifestRejectsCrossTargetAndGenerationDependencies(t *testing.T) {
	key := strings.Repeat("a", 64)
	base := Manifest{Version: 4, EnvironmentKey: key, Script: ScriptPath(key, "ps1"), Interpreter: "powershell.exe", Dependencies: []string{}}
	parse := func(m Manifest) error {
		body, _ := json.Marshal(m)
		_, err := ParseManifest([]byte("```steerline-environment\n" + string(body) + "\n```"))
		return err
	}
	if err := parse(base); err != nil {
		t.Fatal(err)
	}
	for _, dependency := range []string{"SteerlineFlow/environment-generation.md", DocumentPath(strings.Repeat("b", 64)), "../outside", "C:/outside"} {
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
	m.Script = ScriptPath(strings.Repeat("b", 64), "ps1")
	if parse(m) == nil {
		t.Fatal("cross target script accepted")
	}
	if DocumentPath("../outside") != "" || ValidKey(strings.Repeat("a", 10)) {
		t.Fatal("unsafe path/key accepted")
	}
}

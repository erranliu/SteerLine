package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSharedEnvironmentManifestRejectsLegacyAndUnsafeMaterials(t *testing.T) {
	base := Manifest{Version: 5, Script: ScriptPath("ps1"), Interpreter: "powershell.exe", Dependencies: []string{}}
	parse := func(m Manifest) error {
		body, _ := json.Marshal(m)
		_, err := ParseManifest([]byte("```steerline-environment\n" + string(body) + "\n```"))
		return err
	}
	if err := parse(base); err != nil {
		t.Fatal(err)
	}
	for _, dependency := range []string{"SteerlineFlow/environment-generation.md", DocumentPath(), base.Script, "SteerlineFlow/environments/" + strings.Repeat("a", 64) + "/helper.ps1", "../outside", "C:/outside", "tools/*.ps1"} {
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
	m.Script = "SteerlineFlow/environments/" + strings.Repeat("b", 64) + "/verify-environment.ps1"
	if parse(m) == nil {
		t.Fatal("cross target script accepted")
	}
	if ScriptPath("../outside") != "" || ValidKey(strings.Repeat("a", 10)) {
		t.Fatal("unsafe path/key accepted")
	}
	m = base
	m.Version = 4
	if parse(m) == nil {
		t.Fatal("legacy version accepted")
	}
	m = base
	m.Dependencies = []string{"tools/check.ps1", "tools/check.ps1"}
	if parse(m) == nil {
		t.Fatal("duplicate dependencies accepted")
	}
	legacyKey := "```steerline-environment\n{\"version\":5,\"environment_key\":\"" + strings.Repeat("a", 64) + "\",\"script\":\"" + base.Script + "\",\"interpreter\":\"powershell.exe\",\"dependencies\":[]}\n```"
	if _, err := ParseManifest([]byte(legacyKey)); err == nil {
		t.Fatal("environment key accepted in shared manifest")
	}
}

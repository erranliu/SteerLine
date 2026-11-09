package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSharedEnvironmentManifestRejectsLegacyAndUnsafeMaterials(t *testing.T) {
	base := Manifest{Version: 6, Requirements: map[string]string{}, Script: ScriptPath("ps1"), Interpreter: "powershell.exe", Dependencies: []string{}}
	parse := func(m Manifest) error {
		body, _ := json.Marshal(m)
		_, err := ParseManifest([]byte("```steerline-environment\n" + string(body) + "\n```"))
		return err
	}
	if err := parse(base); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(base)
	document := "```steerline-environment\n" + string(body) + "\n```"
	for _, doc := range []string{document, "# Help\n" + document + "\n", strings.ReplaceAll(document, "\n", "\r\n")} {
		if _, err := ParseManifest([]byte(doc)); err != nil {
			t.Fatal("valid fenced block rejected", err)
		}
	}
	for _, doc := range []string{"inline " + document, document + "suffix", document + "`", document + "\n" + document} {
		if _, err := ParseManifest([]byte(doc)); err == nil {
			t.Fatal("invalid fenced block accepted", doc)
		}
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
	m.Version = 5
	if parse(m) == nil {
		t.Fatal("previous manifest without effective requirements accepted")
	}
	for _, requirements := range []map[string]string{nil, {"go": ""}, {"../go": "1.26"}, {"go": "1.26\nsecret"}} {
		m = base
		m.Requirements = requirements
		if parse(m) == nil {
			t.Fatal("invalid requirements accepted", requirements)
		}
	}
	m = base
	m.Dependencies = []string{"tools/check.ps1", "tools/check.ps1"}
	if parse(m) == nil {
		t.Fatal("duplicate dependencies accepted")
	}
	legacyKey := "```steerline-environment\n{\"version\":6,\"requirements\":{},\"environment_key\":\"" + strings.Repeat("a", 64) + "\",\"script\":\"" + base.Script + "\",\"interpreter\":\"powershell.exe\",\"dependencies\":[]}\n```"
	if _, err := ParseManifest([]byte(legacyKey)); err == nil {
		t.Fatal("environment key accepted in shared manifest")
	}
}

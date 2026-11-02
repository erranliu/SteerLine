package prompts

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"testing"

	testmodel "github.com/erranliu/SteerLine/internal/testmanagement/model"
)

var testProject = testmodel.ProjectEntry{Version: 1, Requirements: []string{"quality/policy.md"}, TestingCatalogue: "quality/catalogue.json"}

const testSnapshot = `{"project":{"version":1,"requirements":["quality/policy.md"],"testing_catalogue":"quality/catalogue.json"},"status":"ready","plan_path":"task.plan.md","commit":"fixed","digest":"digest","generation_id":"generation","inputs":"inputs","definitions":[{"id":"z.last"},{"id":"a.first"}]}`

func TestPromptGovernanceGolden(t *testing.T) {
	cases := []struct {
		name   string
		render func() (string, error)
	}{
		{"testing-generate", func() (string, error) { return TestingGenerate("task.plan.md", "fixed", testProject) }},
		{"testing-reference", func() (string, error) { return TestingContext([]byte(testSnapshot), true) }},
		{"testing-readonly", TestingReadOnly},
		{"testing-step", func() (string, error) {
			return TestingStep("用户原文 {{.Ignore}}\r\n", []byte(testSnapshot), true, true)
		}},
		{"environment-invoke", func() (string, error) {
			return EnvironmentInvoke("powershell.exe", "SteerlineFlow/environments/"+strings.Repeat("a", 64)+"/verify-environment.ps1")
		}},
		{"roadmap-parse", func() (string, error) { return RoadmapParse("roadmap.md", "fixed") }},
		{"roadmap-documents", func() (string, error) { return RoadmapDocuments("fixed", "roadmap.md") }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			want, err := os.ReadFile("testdata/" + c.name + ".golden.md")
			if err != nil {
				t.Fatal(err)
			}
			got, err := c.render()
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("UTF-8 caller body: expected=%d rendered=%d", len(want), len(got))
			if got != string(want) {
				t.Fatalf("short caller body changed; review the actual diff\nexpected: %q\nactual: %q", want, got)
			}
			again, err := c.render()
			if err != nil || again != got {
				t.Fatal("nondeterministic rendering", err)
			}
			for _, long := range []string{"Set-RuntimeProgress", "PowerShell 7 正式共用写入器", `"tests":["z.last"`, "完整原因"} {
				if strings.Contains(got, long) {
					t.Fatal("material was expanded into the instruction", long)
				}
			}
		})
	}
}
func TestInlineProgressWriterForProjectWithoutHelper(t *testing.T) {
	if body, err := TestingContext([]byte(testSnapshot), false); err == nil || body != "" {
		t.Fatal("missing writer fell back to inline source")
	}
	body, err := TestingContext([]byte(testSnapshot), true)
	if err != nil || !strings.Contains(body, "task.tests.json") || strings.Contains(body, "Set-RuntimeProgress") {
		t.Fatal("missing real references or source expanded", err)
	}
	external := "用户原文 {{.Ignore}}\r\n"
	readOnly, err := TestingReadOnly()
	if err != nil {
		t.Fatal(err)
	}
	report, err := TestingStep(external, []byte(testSnapshot), true, false)
	if err != nil || report != external+"\n\n"+readOnly {
		t.Fatal("external body changed", err)
	}
}
func TestPromptGovernanceRejectsInvalidInputs(t *testing.T) {
	cases := []struct {
		name string
		run  func() (string, error)
	}{
		{"missing-file", func() (string, error) { return render("absent", nil) }},
		{"bad-template", func() (string, error) { return renderText("bad", "{{", nil) }},
		{"missing-field", func() (string, error) { return renderText("bad", "{{.Missing}}", struct{}{}) }},
		{"bad-snapshot", func() (string, error) { return TestingContext([]byte(`{"status":"missing"}`), true) }},
		{"missing-body", func() (string, error) { return TestingStep("", []byte(testSnapshot), true, true) }},
		{"missing-entry", func() (string, error) { return EnvironmentInvoke("powershell.exe", "") }},
		{"unsupported-interpreter", func() (string, error) { return EnvironmentInvoke("other", "entry") }},
		{"missing-plan", func() (string, error) { return TestingGenerate("", "fixed", testProject) }},
		{"missing-roadmap-commit", func() (string, error) { return RoadmapDocuments("", "roadmap.md") }},
		{"missing-roadmap-source", func() (string, error) { return RoadmapParse("", "fixed") }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if body, err := c.run(); err == nil || body != "" {
				t.Fatal("invalid input accepted", err)
			}
		})
	}
}
func TestDocumentDrivenRuleCopies(t *testing.T) {
	// Byte snapshots of long rule assets without a second copy of their prose.
	// Updates require the readable asset diff and independent review, just as
	// short golden bodies do. No automatic acceptance or runtime signature.
	cases := []struct {
		name, sha256 string
		read         func() ([]byte, error)
	}{
		{"testing", "75671b11e26c540791678c1593c3b14ed4161ba53b721d8758ae8c9c1eba937a", TestingRules},
		{"roadmap-parse", "c1452c0d591caf5cf6f3db0f4d156c2e1a4451c797e96837c0afb1d6699463e6", RoadmapParseRules},
		{"roadmap-documents", "4f04f38c14ba54d90d6ebb0000f6a21652a5dd224afd074efd9a10a909ff827e", RoadmapDocumentRules},
		{"roadmap-spec-guide", "140acc1062a7838474a11947b55087c8c5194a18ec0be9cd5c8c313db92f230b", RoadmapSpecGuide},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			raw, err := c.read()
			if err != nil || len(raw) == 0 {
				t.Fatal("missing canonical rules", err)
			}
			normalized := strings.ReplaceAll(string(raw), "\r\n", "\n")
			t.Logf("UTF-8 canonical rule bytes: %d", len(normalized))
			if fmt.Sprintf("%x", sha256.Sum256([]byte(normalized))) != c.sha256 {
				t.Fatal("fixed rule text changed; review its full diff and explicitly update the byte snapshot")
			}
			crlf := []byte(strings.ReplaceAll(normalized, "\n", "\r\n"))
			if !SameRules(crlf, raw) || SameRules(append(append([]byte{}, raw...), ' '), raw) {
				t.Fatal("rule provenance comparison changed")
			}
		})
	}
}

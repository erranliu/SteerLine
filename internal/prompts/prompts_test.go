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
		{"testing-reference", func() (string, error) { return TestingContext([]byte(testSnapshot)) }},
		{"testing-readonly", TestingReadOnly},
		{"main-agent", func() (string, error) { return MainAgentAdvisory(`{"goal":"clarify","materials":[]}`) }},
		{"pr-conflict-resolution", func() (string, error) { return PRConflictResolution("https://example.test/repo/pull/1", "main") }},
		{"testing-step", func() (string, error) {
			return TestingStep("用户原文 {{.Ignore}}\r\n", []byte(testSnapshot), true)
		}},
		{"environment-invoke", func() (string, error) {
			return EnvironmentInvoke("powershell.exe", "SteerlineFlow/scripts/verify-environment.ps1")
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
func TestTestingContextOnlyBindsRuntimeFiles(t *testing.T) {
	body, err := TestingContext([]byte(testSnapshot))
	if err != nil || !strings.HasPrefix(body, "STEERLINE_RESULT_FILE SHA-256: ") || strings.Contains(body, "\n") {
		t.Fatal("missing real references or source expanded", err)
	}
	external := "用户原文 {{.Ignore}}\r\n"
	readOnly, err := TestingReadOnly()
	if err != nil {
		t.Fatal(err)
	}
	report, err := TestingStep(external, []byte(testSnapshot), false)
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
		{"bad-snapshot", func() (string, error) { return TestingContext([]byte(`{"status":"missing"}`)) }},
		{"missing-body", func() (string, error) { return TestingStep("", []byte(testSnapshot), true) }},
		{"missing-entry", func() (string, error) { return EnvironmentInvoke("powershell.exe", "") }},
		{"unsupported-interpreter", func() (string, error) { return EnvironmentInvoke("other", "entry") }},
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
		{"main-agent", "3a7a447bbe7527bf4c1c46867f9180cab32bc3b61359a92d9db9784172c38189", MainAgentRules},
		{"roadmap-parse", "426c10f34a883e9066d3060bc06723bcf2823989e5cc74d78c50a3a3882738bb", RoadmapParseRules},
		{"roadmap-documents", "f8f61a7b1f58e48ac2d1365f098cf26821800d32e4db1dd6f7d6bb52ae89a67c", RoadmapDocumentRules},
		{"roadmap-spec-guide", "a39d6ae2bc64bb682ba0cbe39efc688ac7dfee03a247a61f3f1340c902f8f50c", RoadmapSpecGuide},
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

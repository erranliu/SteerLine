package prompts

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"testing"
)

const testSnapshot = `{"status":"ready","plan_path":"task.plan.md","commit":"fixed","digest":"digest","generation_id":"generation","inputs":"inputs","definitions":[{"id":"z.last"},{"id":"a.first"}]}`

func TestPromptGovernanceGolden(t *testing.T) {
	cases := []struct {
		name   string
		render func() (string, error)
	}{
		{"testing-generate", func() (string, error) { return TestingGenerate("task.plan.md", "fixed") }},
		{"testing-reference", func() (string, error) { return TestingContext([]byte(testSnapshot), true) }},
		{"testing-readonly", TestingReadOnly},
		{"testing-step", func() (string, error) {
			return TestingStep("用户原文 {{.Ignore}}\r\n", []byte(testSnapshot), true, true)
		}},
		{"environment-generate", func() (string, error) { return EnvironmentGenerate("fixed") }},
		{"environment-prepare", EnvironmentPrepare}, {"environment-publish", EnvironmentPublish},
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
		{"missing-commit", func() (string, error) { return EnvironmentGenerate("") }},
		{"missing-plan", func() (string, error) { return TestingGenerate("", "fixed") }},
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
		{"testing", "e499f40f543ae1bfddb98ee5024f74eb0d46354392cf6794d34b8b204b6d25f3", TestingRules},
		{"roadmap-parse", "c1452c0d591caf5cf6f3db0f4d156c2e1a4451c797e96837c0afb1d6699463e6", RoadmapParseRules},
		{"roadmap-documents", "0c84c0ce636e4c7ca8c9f0b80cf8bdf41bbd2ecb588ea225fb10fa4bb88f2f71", RoadmapDocumentRules},
		{"environment-generation", "2f4ab8b58363f0982cd0518c1e52902c2db6a0fcc2fb50e6260036d580f7247e", EnvironmentGenerationRules},
		{"environment-execution", "a3f872c1164543002318d90badf46db5094277de6b3b19eb367d89a5e909bd9a", EnvironmentExecutionRules},
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

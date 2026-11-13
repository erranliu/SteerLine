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
		{"maintenance-decision", MaintenanceDecision},
		{"maintenance-execute", func() (string, error) {
			return MaintenanceExecute([]byte(`{"action_id":"a","operation_id":"o","instruction":"用户原文 {{.Ignore}}\r\n下一行  "}`))
		}},
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
		{"environment-repair", func() (string, error) { return EnvironmentRepair("SteerlineFlow/environment.md") }},
		{"project-initialization", ProjectInitialization},
		{"project-registration", func() (string, error) {
			return ProjectRegistration(`D:\Tools\SteerLine 安装`)
		}},
		{"roadmap-parse", func() (string, error) { return RoadmapParse("roadmap.md", "fixed") }},
		{"roadmap-documents", func() (string, error) { return RoadmapDocuments("fixed", "roadmap.md", "T1") }},
		{"roadmap-planning", func() (string, error) {
			return RoadmapPlanning("rules.md", "drafts/topic.md", "用户目标原文\r\n下一行")
		}},
		{"task-planning", func() (string, error) {
			return TaskPlanning("rules.md", "cards/topic.json", "用户目标原文\r\n下一行")
		}},
		{"task-planning-documents", func() (string, error) { return TaskPlanningDocuments("fixed", "T1") }},
		{"roadmap-planning-conflict", func() (string, error) { return RoadmapPlanningConflict("保留原定范围。") }},
		{"roadmap-planning-conflict-default", func() (string, error) { return RoadmapPlanningConflict("") }},
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
		{"missing-environment-document", func() (string, error) { return EnvironmentRepair("") }},
		{"unsupported-interpreter", func() (string, error) { return EnvironmentInvoke("other", "entry") }},
		{"missing-roadmap-commit", func() (string, error) { return RoadmapDocuments("", "roadmap.md", "T1") }},
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
		{"testing", "8ec76ded720a6d01623da9e70c48f6b24795344a82018fd3f59895f6b5154421", TestingRules},

		{"maintenance-first", "ac9c1d8c7abb827251e0d6ed68a79cc033a0920014e2e59b458fea053acd312d", func() ([]byte, error) { return MaintenanceRules("first") }},
		{"maintenance-followup", "92018bfa6670c1dbecc33fd20c62afc0bc85846c69e78493b24e2ad401ecd43d", func() ([]byte, error) { return MaintenanceRules("followup") }},
		{"maintenance-report", "77a94a8fa2e9eac5e5e11f55ac0f0bc3437718a17b39533885d57a309b34a098", func() ([]byte, error) { return MaintenanceRules("report") }},
		{"maintenance-cautious", "36fb56b30ba9e12306c3066b0f9315752bb5e880dbaaa7261cf543612657c7f4", func() ([]byte, error) { return MaintenanceStrategy("cautious") }},
		{"maintenance-autonomous", "f0c2470a07e019911ae36c30046e1ad320a5ea60b9a5db46a7d67b404ef57776", func() ([]byte, error) { return MaintenanceStrategy("autonomous") }},
		{"maintenance-aggressive", "e9ed219ceaf5abdab0b467664db2d247ddf3f57a8da853d918246c993a08130b", func() ([]byte, error) { return MaintenanceStrategy("aggressive") }},
		{"main-agent", "1fa5687dcca43dc78f5a91b0114dd3892c5918d626d0c86105e540d5ae5f2128", MainAgentRules},
		{"roadmap-parse", "aa7efda5cdc15f1801427b2a9a845c29bb2e7846aeee9057d4619eb7b2b902d7", RoadmapParseRules},
		{"roadmap-documents", "62f392fc7d180d272949910dc7a7b32514bd43040c8f5655d354db16bdaff7ea", RoadmapDocumentRules},
		{"roadmap-spec-guide", "a39d6ae2bc64bb682ba0cbe39efc688ac7dfee03a247a61f3f1340c902f8f50c", RoadmapSpecGuide},
		{"roadmap-planning", "d405343c050b42dad6cd74d63d2d53e52ee34551965bd21ecb79fa49bb362261", RoadmapPlanningRules},
		{"task-planning", "62818dbf7dc3186cdbaee93ee5feb5caa13c056a960e0a3e24126f1f39880e42", TaskPlanningRules},
	}
	for _, direction := range []struct{ id, sha256 string }{
		{"simplify", "0eae9dbb8636492f585982d01964e43b8a3116d6957ed5fb92f39437477b1abd"},
		{"cleanup", "feac63c598b83c95ffa1ec682f41a7a1d5b86ef277b1022f42fa61694a2a8e7d"},
		{"boundaries", "6d2e4581612d586a8d41d9a7bdce889f3228f0654a5433747df64f201a428a5b"},
		{"readability", "773a8da0d06062805ec61c91f0177bd316d5307806c331131e1c575d980f0f11"},
		{"docs-tests", "c38c3472c7e3cfead403511934779c3dc677251a6d518d660dd0b36a08ad0eaf"},
	} {
		cases = append(cases, struct {
			name, sha256 string
			read         func() ([]byte, error)
		}{"maintenance-direction/" + direction.id, direction.sha256, func() ([]byte, error) {
			return MaintenanceDirection(direction.id)
		}})
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

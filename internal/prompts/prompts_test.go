package prompts

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	progresstools "github.com/erranliu/SteerLine/tools"
)

const testSnapshot = `{"status":"ready","plan_path":"task.plan.md","commit":"fixed","digest":"digest","generation_id":"generation","inputs":"inputs","definitions":[{"id":"z.last"},{"id":"a.first"}]}`

// Expected bodies were captured with the pre-migration callers. Fixed template
// CRLF is now LF; the inline writer is expanded from its one official asset.
// No update flag: a changed body requires an explicit, reviewed fixture change.
func TestPromptGovernanceGolden(t *testing.T) {
	snapshot := []byte(testSnapshot)
	public := json.RawMessage(`{"provider":"github"}`)
	previous := json.RawMessage(`{"activity_id":"prior","message":"失败 {{.Ignored}}"}`)
	guides := json.RawMessage(`[{"path":"SteerlineFlow/testing.md","sha256":"fixed"}]`)
	cases := []struct {
		name   string
		render func() (string, error)
	}{
		{"testing-generate", TestingGenerate},
		{"testing-inline", func() (string, error) { return TestingContext(snapshot, false) }},
		{"testing-reference", func() (string, error) { return TestingContext(snapshot, true) }},
		{"testing-readonly", TestingReadOnly},
		{"testing-step", func() (string, error) { return TestingStep("用户原文 {{.Ignore}}\r\n", snapshot, true, true) }},
		{"environment-generate", func() (string, error) { return EnvironmentGenerate("fixed", "main", public, nil) }},
		{"environment-previous", func() (string, error) { return EnvironmentGenerate("fixed", "main", public, previous) }},
		{"environment-prepare", func() (string, error) { return EnvironmentPrepare("") }},
		{"environment-retry", func() (string, error) { return EnvironmentPrepare("失败 {{.Ignored}}\r\n完整原因") }},
		{"environment-publish", EnvironmentPublish},
		{"roadmap-parse", RoadmapParse},
		{"roadmap-documents", func() (string, error) { return RoadmapDocuments("fixed", guides) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			want, err := os.ReadFile("testdata/" + c.name + ".golden.md")
			if err != nil {
				t.Fatal(err)
			}
			if c.name == "testing-inline" {
				want = []byte(strings.Replace(string(want), "{{WRITER_ASSET}}", progresstools.PowerShellWriter, 1))
			}
			got, err := c.render()
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("UTF-8 caller body: frozen=%d rendered=%d", len(want), len(got))
			if got != string(want) {
				i := 0
				for i < len(want) && i < len(got) && want[i] == got[i] {
					i++
				}
				t.Fatalf("caller body changed at byte %d; review template and final-body diff", i)
			}
			again, err := c.render()
			if err != nil || again != got {
				t.Fatal("render is not deterministic", err)
			}
		})
	}
}

func TestInlineProgressWriterForProjectWithoutHelper(t *testing.T) {
	inline, err := TestingContext([]byte(testSnapshot), false)
	if err != nil {
		t.Fatal(err)
	}
	referenced, err := TestingContext([]byte(testSnapshot), true)
	if err != nil {
		t.Fatal(err)
	}
	readOnly, err := TestingReadOnly()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(inline, progresstools.PowerShellWriter) || !strings.Contains(inline, "入口开始调用 Set-RuntimeProgress") || !strings.Contains(inline, "逐项核对") {
		t.Fatal("missing full writer/wiring contract")
	}
	if strings.Contains(referenced, progresstools.PowerShellWriter) || !strings.Contains(referenced, "逐 ID 核对") {
		t.Fatal("reference mode lost wiring boundary")
	}
	if strings.Contains(readOnly, "Set-RuntimeProgress") || !strings.Contains(readOnly, "不得写入") {
		t.Fatal("read-only boundary lost")
	}
	body := "用户原文 {{.Ignore}}\r\n"
	step, err := TestingStep(body, []byte(testSnapshot), true, false)
	if err != nil || step != body+"\n\n"+readOnly {
		t.Fatal("external body changed or wrong context", err)
	}
	if strings.Count(inline, "SteerLine 执行绑定测试安排") != 1 {
		t.Fatal("duplicate context")
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
		{"bad-snapshot", func() (string, error) { return TestingContext([]byte(`{"status":"missing"}`), false) }},
		{"missing-body", func() (string, error) { return TestingStep("", []byte(testSnapshot), false, true) }},
		{"missing-commit", func() (string, error) { return EnvironmentGenerate("", "main", json.RawMessage(`{}`), nil) }},
		{"missing-branch", func() (string, error) { return EnvironmentGenerate("fixed", "", json.RawMessage(`{}`), nil) }},
		{"bad-publication", func() (string, error) { return EnvironmentGenerate("fixed", "main", nil, nil) }},
		{"bad-previous", func() (string, error) {
			return EnvironmentGenerate("fixed", "main", json.RawMessage(`{}`), json.RawMessage(`{`))
		}},
		{"missing-roadmap-commit", func() (string, error) { return RoadmapDocuments("", json.RawMessage(`[]`)) }},
		{"bad-guides", func() (string, error) { return RoadmapDocuments("fixed", nil) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if body, err := c.run(); err == nil || body != "" {
				t.Fatal("invalid input accepted", err)
			}
		})
	}
}

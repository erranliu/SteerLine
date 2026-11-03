package driver

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/erranliu/SteerLine/internal/prompts"
	"github.com/erranliu/SteerLine/internal/roadmap"
)

func TestDebugRoadmapMaterialsInInitialCommit(t *testing.T) {
	root := t.TempDir()
	d := &debugRelease{root: root, home: filepath.Join(root, "home"), repo: filepath.Join(root, "project"), config: filepath.Join(root, "home", "etc", "SteerlineTest.json"), provider: filepath.Join(root, "provider", "state.json"), base: "main"}
	for name, body := range map[string]string{
		"scheme.json":                              `{"scheme":"simulate"}`,
		"home/execution.json":                      `{"version":3}`,
		"seed/tasks.json":                          `{"tasks":[],"dependencies":[]}`,
		"project/SteerlineFlow/spec/task.spec.md":  "# Spec\nFixture\n",
		"project/SteerlineFlow/plans/task.plan.md": "# Plan\nFixture\n",
	} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := d.initialize(); err != nil {
		t.Fatal(err)
	}
	for name, load := range map[string]func() ([]byte, error){roadmap.ParseRulesPath: prompts.RoadmapParseRules, roadmap.DocumentRulesPath: prompts.RoadmapDocumentRules, "SteerlineFlow/spec/SpecGuide.md": prompts.RoadmapSpecGuide} {
		body, err := d.command("git", "-C", d.repo, "show", "HEAD:"+name).Output()
		if err != nil {
			t.Fatal(err)
		}
		expected, err := load()
		if err != nil || string(body) != string(expected) {
			t.Fatal("initial commit lacks canonical static file", name, err)
		}
	}
	if status, err := d.git("status", "--porcelain"); err != nil || status != "" {
		t.Fatal("initialization left unpublished files", status, err)
	}
	if _, err := os.Stat(filepath.Join(d.repo, "roadmap-inputs")); !os.IsNotExist(err) {
		t.Fatal("initialization wrote dynamic materials")
	}
}

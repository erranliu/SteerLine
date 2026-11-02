package roadmap

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRoadmapPrepareStatic(t *testing.T) {
	root := t.TempDir()
	if _, err := PrepareStatic(root, true); err == nil {
		t.Fatal("missing static materials passed a read-only check")
	}
	if entries, _ := os.ReadDir(root); len(entries) != 0 {
		t.Fatal("read-only preparation wrote files")
	}
	files, err := PrepareStatic(root, false)
	if err != nil || len(files) != 3 {
		t.Fatal(files, err)
	}
	for _, file := range files {
		if !file.Created {
			t.Fatal("fresh static material was not created", file)
		}
	}
	files, err = PrepareStatic(root, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if file.Created {
			t.Fatal("repeat preparation rewrote a file")
		}
	}
	guide := filepath.Join(root, "SteerlineFlow/spec/SpecGuide.md")
	if err := os.WriteFile(guide, []byte("# Project guide\nProject-specific rules\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareStatic(root, true); err != nil {
		t.Fatal("valid project-specific guide rejected", err)
	}
	if raw, _ := os.ReadFile(guide); string(raw) != "# Project guide\nProject-specific rules\n" {
		t.Fatal("project guide was replaced")
	}
	if err := os.WriteFile(filepath.Join(root, ParseRulesPath), []byte("different rules"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareStatic(root, false); err == nil {
		t.Fatal("different platform rules silently replaced")
	}
	for _, name := range []string{GraphPath, MappingPath, PlanningPath, SourceInputPath} {
		if _, err := os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Fatal("project initialization wrote a dynamic material", name)
		}
	}
}

func TestRoadmapPrepareStaticRejectsRedirectedDirectories(t *testing.T) {
	root, target := t.TempDir(), t.TempDir()
	alias := filepath.Join(root, "SteerlineFlow")
	roadmapDirectoryAlias(t, alias, target)
	if _, err := PrepareStatic(root, false); err == nil {
		t.Fatal("redirected material directory accepted")
	}
	if _, err := PrepareStatic(alias, false); err == nil {
		t.Fatal("redirected project root accepted")
	}
	if entries, err := os.ReadDir(target); err != nil || len(entries) != 0 {
		t.Fatal("preparation wrote outside the project", entries, err)
	}
}

func roadmapDirectoryAlias(t *testing.T, alias, target string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		if output, err := exec.Command("cmd", "/c", "mklink", "/J", alias, target).CombinedOutput(); err != nil {
			t.Fatal("could not create the local junction fixture", err, string(output))
		}
	} else if err := os.Symlink(target, alias); err != nil {
		t.Fatal(err)
	}
}

func TestRoadmapSourcesDoNotEnumerateRedirectedChildren(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	drafts := filepath.Join(root, "SteerlineFlow", "Roadmap", "drafts")
	if err := os.MkdirAll(drafts, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.md"), []byte("# Outside private material"), 0600); err != nil {
		t.Fatal(err)
	}
	roadmapDirectoryAlias(t, filepath.Join(drafts, "alias"), outside)
	sources, err := ListSources(root, "fixture")
	if err != nil || len(sources) != 0 {
		t.Fatal("external draft names enumerated", sources, err)
	}
	if _, err := ReadSource(root, "fixture", "SteerlineFlow/Roadmap/drafts/alias/secret.md", ""); err == nil {
		t.Fatal("redirected draft body read")
	}
}

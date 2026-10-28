package controlled

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func TestFixtureRepositoryPreservesCommittedBytes(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(root, "empty.gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", root, "-c", "core.autocrlf=false", "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false"}, args...)...)
		raw, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, raw)
		}
		return strings.TrimSpace(string(raw))
	}
	git("init", "-b", "main")
	files := map[string][]byte{"plain.txt": []byte("original\n"), "材料.md": []byte("原始材料\r\n"), "binary.bin": {0, 10, 255, 13, 10, 0}, "empty": {}}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(root, name), body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	git("add", ".")
	git("commit", "-qm", "original bytes")
	commit := git("rev-parse", "HEAD")
	for name := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte("uncommitted replacement"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	materials, source, err := fixtureRepository(runnercontract.GenerateRepository{LocalPath: root, Commit: commit})
	if err != nil {
		t.Fatal(err)
	}
	if source.LocalPath != root || source.Commit != commit || source.Tree != git("rev-parse", commit+"^{tree}") || len(source.Files) != len(files) || len(materials) != len(files) {
		t.Fatal("snapshot identity differs", source)
	}
	observed := map[string]bool{}
	for _, file := range source.Files {
		body, ok := files[file.Path]
		sum := sha256.Sum256(body)
		if !ok || observed[file.Path] || file.Blob != git("rev-parse", commit+":"+file.Path) || file.SizeBytes != len(body) || file.SHA256 != hex.EncodeToString(sum[:]) {
			t.Fatal("original blob differs", file)
		}
		observed[file.Path] = true
	}
	for _, material := range materials {
		if material.Content == nil || !bytes.Equal([]byte(*material.Content), files[material.Path]) {
			t.Fatal("original bytes differ", material.Path)
		}
	}
	if _, _, err := fixtureRepository(runnercontract.GenerateRepository{LocalPath: root, Commit: strings.Repeat("0", 40)}); err == nil {
		t.Fatal("missing commit read from worktree")
	}
}

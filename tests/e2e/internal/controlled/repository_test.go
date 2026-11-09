package controlled

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitForwardingPreservesBatchInputAndCommittedBytes(t *testing.T) {
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
	oid := git("rev-parse", commit+":binary.bin")
	batch, err := gitBytes(context.Background(), root, strings.NewReader(oid+"\n"), "cat-file", "--batch")
	if err != nil || !bytes.Equal(batch, append([]byte(oid+" blob 6\n"), append(files["binary.bin"], '\n')...)) {
		t.Fatal("Git forwarding lost batch input or original bytes", batch, err)
	}
}

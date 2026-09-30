package codexworker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareTurnNeverOverwritesOriginalJournal(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "runs", "run", "turns", "command")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	journal := filepath.Join(dir, "appserver.jsonl")
	if err := os.WriteFile(journal, []byte("original\n"), 0600); err != nil {
		t.Fatal(err)
	}
	called := false
	_, err := PrepareTurn(root, "run", "command", filepath.Join(root, "runs", "run", "app-server"), "prompt", func(string, []byte) error {
		called = true
		return nil
	})
	if err == nil || called {
		t.Fatalf("existing native journal did not block another send: %v, write=%v", err, called)
	}
}

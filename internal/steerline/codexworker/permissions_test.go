package codexworker

import (
	"reflect"
	"testing"
)

func TestGitPermissionLinkedWorktreeWriteArgsCompilesOnlyResolvedDirectories(t *testing.T) {
	want := []string{"--add-dir", "repo/objects", "--add-dir", "repo/refs", "--add-dir", "repo/logs", "--add-dir", "repo/worktrees/run-one"}
	got, err := LinkedWorktreeWriteArgs("repo/objects", "repo/refs", "repo/logs", "repo/worktrees/run-one")
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("args=%q err=%v", got, err)
	}
	if _, err := LinkedWorktreeWriteArgs("repo/objects", "repo/refs", "repo/logs", ""); err == nil {
		t.Fatal("incomplete write resource list was accepted")
	}
}

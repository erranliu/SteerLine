package app

import (
	"context"

	"os"
	"path/filepath"

	"testing"
)

func TestOrcaWorktreeMetadataKeepsNativeQueriesReadOnly(t *testing.T) {
	c, _ := nativeTaskFixture(t)
	store, err := OpenNativeTaskProject(context.Background(), &c)
	require(t, err)
	require(t, store.Close())
	client, err := OpenClient(context.Background(), c.ConfigPath)
	require(t, err)
	before, err := client.ListTasks(context.Background(), "", 100)
	require(t, err)
	require(t, client.Close())
	metadata := filepath.Join(c.RunDir, "worktrees", ".orca-worktree-trash")
	require(t, os.MkdirAll(metadata, 0700))
	evidence := filepath.Join(metadata, "native-retained.txt")
	require(t, os.WriteFile(evidence, []byte("keep native contents"), 0600))
	client, err = OpenClient(context.Background(), c.ConfigPath)
	require(t, err)
	defer client.Close()
	after, err := client.ListTasks(context.Background(), "", 100)
	require(t, err)
	if after.DatabaseID != before.DatabaseID || after.DatabaseRevision != before.DatabaseRevision || after.AuthorityVersion != before.AuthorityVersion || after.GraphRevision != before.GraphRevision || len(after.Tasks) != len(before.Tasks) {
		t.Fatal("metadata was adopted as authoritative state or changed the database")
	}
	content, err := os.ReadFile(evidence)
	require(t, err)
	if string(content) != "keep native contents" {
		t.Fatal("native trash contents changed")
	}
}

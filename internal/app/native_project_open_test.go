package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/storage/ports"
)

func TestNativeProjectInitializesOnceAndRejectsMismatchedBinding(t *testing.T) {
	root := t.TempDir()
	t.Setenv("STEERLINE_HOME", filepath.Join(root, "home"))
	localGlobalExecutionFixture(t)
	repository := projectRepo(t)
	if output, err := exec.Command("git", "-C", repository, "remote", "add", "origin", "https://github.com/Org/Repo.git").CombinedOutput(); err != nil {
		t.Fatalf("configure fixture remote: %v: %s", err, output)
	}
	input := minimalConfig(repository)
	input.PR.Repository = "Org/Repo"
	configPath := filepath.Join(root, "project.json")
	if err := os.WriteFile(configPath, projectConfigJSON(t, input), 0600); err != nil {
		t.Fatal(err)
	}
	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	db, err := OpenNativeTaskProject(context.Background(), &config)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reader, err := OpenNativeTaskReader(context.Background(), &config)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := reader.NativeAuthority(context.Background(), config.Identity())
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if authority.Authority != "native" || authority.AuthorityVersion != 1 || authority.GraphRevision != 0 || authority.DispatchMode != "manual" || !authority.DispatchPaused {
		t.Fatalf("bad initial native authority: %+v", authority)
	}
	repeated, err := OpenNativeTaskProject(context.Background(), &config)
	if err != nil {
		t.Fatalf("repeat open: %v", err)
	}
	if err := repeated.Close(); err != nil {
		t.Fatal(err)
	}
	writer, err := OpenNativeTaskCommandStore(context.Background(), &config)
	if err != nil {
		t.Fatal(err)
	}
	updatedAuthority := authority
	updatedAuthority.AuthorityVersion = 2
	updatedAuthority.UpdatedAt = time.Now().UTC()
	if err := writer.TransactDispatch(context.Background(), func(tx ports.DispatchTx) error {
		return tx.UpdateProjectTaskAuthorityCAS(updatedAuthority, 1)
	}); err != nil {
		t.Fatalf("authority CAS 1->2 failed: %v", err)
	}
	staleAuthority := updatedAuthority
	staleAuthority.UpdatedAt = time.Now().UTC()
	err = writer.TransactDispatch(context.Background(), func(tx ports.DispatchTx) error {
		return tx.UpdateProjectTaskAuthorityCAS(staleAuthority, 1)
	})
	if err == nil || !strings.Contains(err.Error(), "stale_version") {
		t.Fatalf("replayed authority version was not rejected as stale: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	changedBranch := config
	changedBranch.Branch = "release"
	if _, err := OpenNativeTaskProject(context.Background(), &changedBranch); err == nil || !strings.Contains(err.Error(), "authority_mismatch") {
		t.Fatalf("branch drift did not fail closed: %v", err)
	}
	retiredAuthority := config
	retiredAuthority.TaskAuthority = "beads"
	if _, err := OpenNativeTaskProject(context.Background(), &retiredAuthority); err == nil || !strings.Contains(err.Error(), "authority_mismatch") {
		t.Fatalf("retired authority opened native state: %v", err)
	}
}

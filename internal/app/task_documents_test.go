package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func TestCommittedFilesBatchPreservesExactBytesAndBoundsProcesses(t *testing.T) {
	c, _ := nativeTaskFixture(t)
	names := []string{}
	want := map[string]string{}
	for i := 0; i < 52; i++ {
		name := fmt.Sprintf("runtime/input-%02d", i)
		body := fmt.Sprintf("setting-%d\r\n\x00\n", i)
		if i == 0 {
			name, body = "runtime/空 文件", ""
		}
		names = append(names, name)
		want[name] = body
		require(t, os.MkdirAll(filepath.Dir(filepath.Join(c.Repository, name)), 0700))
		require(t, os.WriteFile(filepath.Join(c.Repository, name), []byte(body), 0600))
	}
	fixtureGit(t, c.Repository, "-c", "core.autocrlf=false", "add", "runtime")
	fixtureGit(t, c.Repository, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "runtime inputs")
	commit := fixtureGit(t, c.Repository, "rev-parse", "HEAD")
	probe := &startupCountingHost{ManagedProcessHost: c.ProcessHost}
	reader := taskDocuments{repository: c.Repository, git: c.Git, timeout: 30 * time.Second, processHost: probe}
	ctx := materialReads(context.Background())
	files, err := reader.readCommittedFiles(ctx, commit, names)
	require(t, err)
	if len(files) != len(want) || probe.calls != 3 {
		t.Fatalf("52 files require a bounded batch, got files=%d processes=%d", len(files), probe.calls)
	}
	for name, expected := range want {
		if string(files[name]) != expected {
			t.Fatalf("blob bytes changed for %s: %q", name, files[name])
		}
	}
	_, err = reader.readCommittedFiles(ctx, commit, append(names, names[0]))
	require(t, err)
	if probe.calls != 3 {
		t.Fatal("same assessment reread immutable blobs", probe.calls)
	}
	_, err = reader.readCommittedFiles(materialReads(context.Background()), commit, names)
	require(t, err)
	if probe.calls != 6 {
		t.Fatal("blob evidence escaped its assessment", probe.calls)
	}
	if _, err := reader.readCommittedFiles(ctx, commit, []string{"runtime/not-present"}); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("missing path lost its type", err)
	}
	if _, err := reader.readCommittedFiles(ctx, commit, []string{"runtime"}); err == nil {
		t.Fatal("directory accepted as a regular blob")
	}
	if _, err := reader.readCommittedFiles(ctx, commit, []string{"runtime/input-*"}); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("path was treated as a wildcard", err)
	}
	oid := fixtureGit(t, c.Repository, "rev-parse", commit+":base.txt")
	fixtureGit(t, c.Repository, "update-index", "--add", "--cacheinfo", "120000,"+oid+",runtime/link")
	fixtureGit(t, c.Repository, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "non-regular input")
	linkedCommit := fixtureGit(t, c.Repository, "rev-parse", "HEAD")
	var nonregular *nonRegularCommittedFileError
	if _, err := reader.readCommittedFiles(ctx, linkedCommit, []string{"runtime/link"}); !errors.As(err, &nonregular) {
		t.Fatal("symlink was accepted or lost its invalidation classification", err)
	}
}

type failingDocumentHost struct {
	executionbackend.ManagedProcessHost
	stage string
	cause error
}

func (h failingDocumentHost) StartManaged(cmd *exec.Cmd) (executionbackend.StopHandle, error) {
	for _, arg := range cmd.Args[1:] {
		if arg == h.stage {
			return nil, h.cause
		}
	}
	return h.ManagedProcessHost.StartManaged(cmd)
}

func TestCommittedFileFailuresKeepOriginalCause(t *testing.T) {
	c, _ := nativeTaskFixture(t)
	commit := fixtureGit(t, c.Repository, "rev-parse", "HEAD")
	for _, stage := range []string{"rev-parse", "ls-tree", "cat-file"} {
		t.Run(stage, func(t *testing.T) {
			cause := fmt.Errorf("controlled Git %s timeout: %w", stage, context.DeadlineExceeded)
			reader := taskDocuments{repository: c.Repository, git: c.Git, timeout: time.Second, processHost: failingDocumentHost{ManagedProcessHost: c.ProcessHost, stage: stage, cause: cause}}
			_, err := reader.readCommittedFile(context.Background(), commit, "base.txt")
			if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), cause.Error()) || strings.Contains(err.Error(), "commit is missing") {
				t.Fatal("Git failure was replaced by a contract diagnosis", err)
			}
		})
	}
}

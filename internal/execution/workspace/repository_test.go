package workspace

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	b "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/hostcontract"
)

type repositoryTestProcess struct{ p *os.Process }

func (p repositoryTestProcess) Identity() hostcontract.ProcessIdentity {
	return hostcontract.ProcessIdentity{}
}
func (p repositoryTestProcess) Kill()  { _ = p.p.Kill() }
func (p repositoryTestProcess) Close() {}
func TestResolveRepositoryUsesOriginalCommonDir(t *testing.T) {
	ctx := context.Background()
	git := Git{Command: b.LaunchCommand{Executable: "git"}, Timeout: 15 * time.Second, Start: func(cmd *exec.Cmd) (b.StopHandle, error) {
		if err := cmd.Start(); err != nil {
			return nil, err
		}
		return repositoryTestProcess{cmd.Process}, nil
	}}
	root := t.TempDir()
	if _, err := git.Output(ctx, root, "init"); err != nil {
		t.Fatal(err)
	}
	repo, err := ResolveRepository(ctx, git, root)
	if err != nil {
		t.Fatal(err)
	}
	if canonical(repo.CommonDir) != canonical(filepath.Join(repo.Path, ".git")) {
		t.Fatal("repository common directory differs")
	}
	if err := repo.Verify(ctx, git); err != nil {
		t.Fatal(err)
	}
	repo.CommonDir = filepath.Join(root, "foreign")
	if err := repo.Verify(ctx, git); err == nil {
		t.Fatal("changed fixed identity accepted")
	}
}

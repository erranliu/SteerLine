//go:build e2e

package testenv

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/runnerstatepath"
	testprocess "github.com/erranliu/SteerLine/internal/testkit/process"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
	"github.com/erranliu/SteerLine/tests/e2e/internal/driver"
)

func TestTwoProjectsIsolateTasksGitAndCleanup(t *testing.T) {
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	runDir := t.TempDir()
	run := Run{Root: root, Binary: filepath.Join(runDir, "steerline.exe"), Helper: filepath.Join(runDir, "helper.exe"), Output: filepath.Join(runDir, "output"), Scratch: filepath.Join(runDir, "scratch")}
	buildCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	for _, b := range []struct{ path, pkg string }{{run.Binary, "./cmd/steerline"}, {run.Helper, "./tests/e2e/testdata/controlled-helper"}} {
		cmd := exec.Command("go", "build", "-o", b.path, b.pkg)
		cmd.Dir = root
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := testprocess.Run(buildCtx, cmd); err != nil {
			t.Fatal(err)
		}
	}
	cancel()
	s := newScenario(t, "isolation", run)
	// The test process's global catalog must not participate in either project's
	// readiness read. Each CLI and snapshot belongs to its isolated home/database.
	outsideHome := filepath.Join(runDir, "outside-home")
	s.Must(os.MkdirAll(outsideHome, 0700))
	s.Must(os.WriteFile(filepath.Join(outsideHome, "execution.json"), []byte("invalid outside catalog"), 0600))
	t.Setenv("STEERLINE_HOME", outsideHome)
	a, b := s.NewProject("a"), s.NewProject("b")
	a.CreateTasks()
	b.CreateTasks()
	a.StartService()
	b.StartService()
	ap, err := a.Driver.Tasks(s.Context)
	s.Must(err)
	bp, err := b.Driver.Tasks(s.Context)
	s.Must(err)
	if ap.ProjectID == bp.ProjectID || ap.DatabaseID == bp.DatabaseID || a.Home == b.Home || a.Provider == b.Provider || len(ap.Tasks) != 2 || len(bp.Tasks) != 2 {
		t.Fatal("projects share identity or task state")
	}
	a.StopService()
	b.StopService()
	// Freeze B's writer before comparing its database revision across A's cleanup.
	// Task CLI reads above require the live application; these snapshots use the
	// same explicit project/database through the existing product read contract.
	beforeView, err := driver.ReadProjectView(s.Context, b.Database, b.ProjectID)
	s.Must(err)
	s.Must(beforeView.TaskError)
	if beforeView.Tasks == nil || beforeView.Tasks.DatabaseID != bp.DatabaseID {
		t.Fatal("stopped B snapshot lost the original database identity")
	}
	before := b.git(b.Repo, "rev-parse", "HEAD")
	s.Must(os.WriteFile(filepath.Join(a.Repo, "only-a.txt"), []byte("isolated"), 0600))
	a.git(a.Repo, "add", "only-a.txt")
	a.git(a.Repo, "commit", "--quiet", "-m", "only A")
	state, err := controlled.Read(a.Provider)
	s.Must(err)
	state.Marker = 7
	s.Must(controlled.Write(a.Provider, state))
	other, err := controlled.Read(b.Provider)
	s.Must(err)
	if other.Marker != 0 || b.git(b.Repo, "rev-parse", "HEAD") != before {
		t.Fatal("A changed B's state")
	}
	s.Must(a.Driver.CleanupOwnedRunner(s.Context, s.Run.Helper))
	s.Must(runnerstatepath.RemoveStopped(a.Home))
	stopCtx, stop := context.WithTimeout(context.Background(), 30*time.Second)
	defer stop()
	s.Must(a.GitServer.Stop(stopCtx))
	s.Must(RemoveOwned(s.Root, a.Root))
	if b.git(b.Repo, "rev-parse", "HEAD") != before {
		t.Fatal("A cleanup damaged B")
	}
	afterView, err := driver.ReadProjectView(s.Context, b.Database, b.ProjectID)
	s.Must(err)
	s.Must(afterView.TaskError)
	if afterView.Tasks == nil || afterView.Tasks.DatabaseID != beforeView.Tasks.DatabaseID || afterView.Tasks.DatabaseRevision != beforeView.Tasks.DatabaseRevision {
		t.Fatal("A cleanup changed B's database")
	}
	// A path escape is rejected and leaves the other project intact.
	if RemoveOwned(s.Root, runDir) == nil {
		t.Fatal("deletion escaped scenario root")
	}
}

func TestPartialPreparationCleanupKeepsEvidence(t *testing.T) {
	dir := t.TempDir()
	run := Run{Root: dir, Binary: filepath.Join(dir, "unused.exe"), Helper: filepath.Join(dir, "unused-helper.exe"), Output: filepath.Join(dir, "output"), Scratch: filepath.Join(dir, "scratch")}
	s := newScenario(t, "partial", run)
	s.register("partial-project", filepath.Join(s.Root, "project"), 0)
	s.Must(os.WriteFile(filepath.Join(s.Output, "partial.log"), []byte("preparation failed before launch"), 0600))
	s.cancel()
	s.cleanup()
	if _, err := os.Stat(s.Root); !os.IsNotExist(err) {
		t.Fatalf("scratch remains: %v", err)
	}
	if raw, err := os.ReadFile(filepath.Join(s.Output, "partial.log")); err != nil || len(raw) == 0 {
		t.Fatal("failure diagnostics were removed")
	}
	raw, err := os.ReadFile(filepath.Join(s.Output, "scenario.json"))
	s.Must(err)
	var evidence struct {
		Passed  bool `json:"passed"`
		Cleanup bool `json:"cleanup_complete"`
	}
	s.Must(json.Unmarshal(raw, &evidence))
	if evidence.Passed || !evidence.Cleanup {
		t.Fatal("cancelled preparation was reported passed or left resources")
	}
}

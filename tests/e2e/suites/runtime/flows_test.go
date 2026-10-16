//go:build e2e

package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/tests/e2e/internal/testenv"
)

// L3 exercises ordinary caller-host operations. L4 owns malformed paths,
// stale writes, repository conflicts and deliberate request-ID collisions.
func TestServiceLifecycle(t *testing.T) {
	t.Parallel()
	s := testenv.NewScenario(t, "runtime.service-lifecycle")
	p := s.NewProjectAt("service", filepath.Join("中文 路径", "有空格的现场"), "")
	cwd := filepath.Join(s.Root, "无关 cwd")
	s.Must(os.MkdirAll(cwd, 0700))
	p.Driver.WorkingDirectory = cwd
	for i := 0; i < 2; i++ {
		_, err := p.Driver.Run(s.Context, "launchers")
		s.Must(err)
	}
	for _, name := range []string{"start.bat", "stop.bat"} {
		_, err := os.Stat(filepath.Join(p.Home, name))
		s.Must(err)
	}
	for _, name := range []string{"service-window.bat", "console.bat"} {
		if _, err := os.Stat(filepath.Join(p.Home, name)); !os.IsNotExist(err) {
			t.Fatalf("retired launcher present: %s: %v", name, err)
		}
	}
	p.CreateTasks()
	before := mustSnapshot(t, p)
	hostBefore, err := p.Driver.HostStatus(s.Context)
	s.Must(err)
	if before.Runtime == nil || before.Runtime.Service.ProcessIdentity == nil || before.Runtime.Service.PID != p.Service.Cmd.Process.Pid || hostBefore.Backend.Identity.Process.PID != p.Service.Cmd.Process.Pid || !strings.EqualFold(before.Runtime.Service.ProcessIdentity.Executable, p.Executable) {
		t.Fatal("runtime is not owned by the exact home host")
	}
	catalogID := ""
	for _, row := range hostBefore.Projects {
		if row.ConfigID == filepath.Base(p.Config) {
			catalogID = row.ProjectID
		}
	}
	if catalogID == "" {
		t.Fatal("host catalog lacks project identity")
	}
	p.Record("first-start", before)
	count := controlRequestCount(t, p)
	s.Must(p.Driver.Control(s.Context, s.Run.Helper, "resume_dispatch", ""))
	if controlRequestCount(t, p) != count+1 {
		t.Fatal("normal command did not reach the home control inbox")
	}
	p.StopService()
	assertHostStopped(t, p, hostBefore.Backend.Identity.InstanceID, catalogID, serviceInstance(before))
	p.StartService()
	after := mustSnapshot(t, p)
	hostAfter, err := p.Driver.HostStatus(s.Context)
	s.Must(err)
	assertProjectFactsEqual(t, before, after, "host restart changed task facts")
	if serviceInstance(before) == "" || serviceInstance(after) == "" || serviceInstance(before) == serviceInstance(after) || hostBefore.Backend.Identity.InstanceID == hostAfter.Backend.Identity.InstanceID {
		t.Fatal("host restart retained old identities")
	}
	p.Record("after-restart", after)
	p.StopService()
	assertHostStopped(t, p, hostAfter.Backend.Identity.InstanceID, catalogID, serviceInstance(after))
	s.Must(s.FinishBody())
}

func TestDualHomeOperation(t *testing.T) {
	t.Parallel()
	s := testenv.NewScenario(t, "runtime.dual-home-operation")
	a, b := s.NewProject("a"), s.NewProject("b")
	a.CreateTasks()
	b.CreateTasks()
	aBefore, bBefore := mustSnapshot(t, a), mustSnapshot(t, b)
	if aBefore.Page.ProjectID == bBefore.Page.ProjectID || aBefore.Page.DatabaseID == bBefore.Page.DatabaseID || serviceInstance(aBefore) == serviceInstance(bBefore) || aBefore.Runtime.Service.PID == bBefore.Runtime.Service.PID {
		t.Fatal("homes shared project/database/host identity")
	}
	// Different operations and unique request IDs exercise legitimate coexistence;
	// the same-ID/replay matrix remains exclusively in L4.
	bProvider := fileSHA256(t, b.Provider)
	discardOwnedTask(t, a, aBefore, "a", strings.Repeat("a", 32))
	aDone := mustSnapshot(t, a)
	persistedDiscard(t, aDone, a.Tasks["a"].ID)
	assertProjectFactsEqual(t, bBefore, mustSnapshot(t, b), "A command changed B")
	if fileSHA256(t, b.Provider) != bProvider {
		t.Fatal("A command changed B provider")
	}
	aProvider := fileSHA256(t, a.Provider)
	discardOwnedTask(t, b, bBefore, "b", strings.Repeat("b", 32))
	bDone := mustSnapshot(t, b)
	persistedDiscard(t, bDone, b.Tasks["b"].ID)
	assertProjectFactsEqual(t, aDone, mustSnapshot(t, a), "B command changed A")
	if fileSHA256(t, a.Provider) != aProvider || len(aDone.Runner.Candidates) != 0 || len(bDone.Runner.Candidates) != 0 {
		t.Fatal("home operation changed peer provider or dispatched an execution")
	}
	assertSameServiceInstance(t, aBefore, aDone)
	assertSameServiceInstance(t, bBefore, bDone)
	a.Record("operated", aDone)
	b.Record("operated", bDone)
	a.StopService()
	b.StopService()
	s.Must(s.FinishBody())
}

func persistedDiscard(t *testing.T, snapshot testenv.Snapshot, taskID string) {
	t.Helper()
	for _, task := range snapshot.Page.Tasks {
		if task.TaskID == taskID && task.Status == "discarded" {
			return
		}
	}
	t.Fatal("accepted discard was not persisted")
}

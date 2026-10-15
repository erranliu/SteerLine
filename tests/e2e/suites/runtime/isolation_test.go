//go:build e2e

package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/tests/e2e/internal/testenv"
)

// Each parallel scene owns its homes, repos, provider, ports and output. The
// runner binaries are read-only; only the build scene writes its own source clone.
func TestSourceBuildIsolation(t *testing.T) {
	t.Parallel()
	s := testenv.NewScenario(t, "runtime.source-build-isolation")
	p := s.NewProject("other-home")
	installFixture(t, p)
	p.CreateTasks()
	assertDirectBuildOutputIsolation(t, s, p, p, powershellExecutable(t), map[string]any{})
	s.Must(s.FinishBody())
}

func TestPathIsolation(t *testing.T) {
	t.Parallel()
	s := testenv.NewScenario(t, "runtime.path-isolation")
	a, b := s.NewProject("protected"), s.NewProject("target")
	a.CreateTasks()
	before := mustSnapshot(t, a)
	evidence := map[string]any{}
	powershell := powershellExecutable(t)
	config := writeConfigVariant(t, b, "DefaultState.json", func(value map[string]any) {
		delete(value, "run_dir")
		delete(value, "worktree_root")
	})
	sentinel := filepath.Join(a.Home, "state", "isolation-sentinel")
	s.Must(os.WriteFile(sentinel, []byte("protected state"), 0600))
	hash := fileSHA256(t, sentinel)
	invalid := filepath.Join(s.Root, "invalid-batch.json")
	s.Must(os.WriteFile(invalid, []byte("{}\n"), 0600))
	withJunction(t, s, b, powershell, filepath.Join(b.Home, "state"), filepath.Join(a.Home, "state"), "state-alias", func() {
		output, err := s.RunCommand(b.Command(b.Executable, "task", "validate", "-config", config, "-file", invalid), "state-alias-refusal")
		if err == nil || !strings.Contains(strings.ToLower(string(output)), "state root must resolve inside steerline_home") {
			t.Fatalf("state alias was not rejected before initialization: %v: %s", err, output)
		}
		if fileSHA256(t, sentinel) != hash {
			t.Fatal("state alias refusal changed protected sentinel")
		}
		evidence["state_alias_refusal"] = string(output)
	})
	b.CreateTasks()
	for _, field := range []string{"run_dir", "worktree_root"} {
		outside := filepath.Join(a.Home, "escaped-"+field)
		variant := writeConfigVariant(t, b, field+".json", func(value map[string]any) { value[field] = outside })
		output, err := s.RunCommand(b.Command(b.Executable, "task", "validate", "-config", variant, "-file", invalid), field+"-refusal")
		if err == nil || !strings.Contains(strings.ToLower(string(output)), "must resolve inside steerline_home") {
			t.Fatalf("%s escape accepted: %v: %s", field, err, output)
		}
		if _, err := os.Stat(outside); !os.IsNotExist(err) {
			t.Fatalf("refusal created outside path %s: %v", outside, err)
		}
		evidence[field+"_refusal"] = string(output)
	}
	assertSharedSQLiteRejected(t, s, b, a, evidence)
	assertProjectFactsEqual(t, before, mustSnapshot(t, a), "path refusals changed protected project")
	writeRuntimeEvidence(t, s.Output, evidence)
	s.Must(s.FinishBody())
}

func TestServiceWriteIsolation(t *testing.T) {
	t.Parallel()
	s := testenv.NewScenario(t, "runtime.service-write-isolation")
	p := s.NewProjectAt("service", filepath.Join("中文 路径", "有空格的现场"), "")
	installFixture(t, p)
	// Launcher regeneration must be idempotent, without copying another home.
	for i := 0; i < 2; i++ {
		_, err := p.Driver.Run(s.Context, "launchers")
		s.Must(err)
	}
	p.CreateTasks()
	output, err := p.Driver.Run(s.Context, "start-service", "-config", p.Config)
	s.Must(err)
	var requested struct {
		Status       string `json:"status"`
		LauncherPath string `json:"launcher_path"`
		ConfigPath   string `json:"config_path"`
	}
	s.Must(json.Unmarshal(output, &requested))
	if requested.Status != "window-required" || requested.LauncherPath != filepath.Join(p.Home, "service-window.bat") || requested.ConfigPath != p.Config {
		t.Fatalf("wrong service entry: %+v", requested)
	}
	start := func(name string) testenv.Snapshot {
		p.StartServiceViaVisibleConsole(p.Command("cmd.exe", "/d", "/c", "call", requested.LauncherPath, p.Config), name)
		snap := mustSnapshot(t, p)
		if snap.Runtime == nil || snap.Runtime.Service.ProcessIdentity == nil || snap.Runtime.Service.PID <= 0 || snap.Runtime.Service.ProcessIdentity.PID != snap.Runtime.Service.PID || !strings.EqualFold(snap.Runtime.Service.ProcessIdentity.Executable, p.Executable) || snap.Runtime.Service.StartupID == nil {
			t.Fatal("service window lost its native child identity")
		}
		assertServiceIdentity(t, p, p.Executable, s.Run.SourceCommit, s.Run.SourceDirty)
		assertRuntimePathsInsideHome(t, p)
		p.Record(name, snap)
		return snap
	}
	before := start("first-start")
	count := controlRequestCount(t, p)
	err = p.Driver.ControlTargeted(s.Context, s.Run.Helper, "start_task", p.Tasks["a"].ID, serviceInstance(before)+"-stale")
	if err == nil || !strings.Contains(err.Error(), "stale_instance") {
		t.Fatalf("stale instance accepted: %v", err)
	}
	stale := p.Driver
	stale.Config = writeConfigVariant(t, p, "Changed.json", func(value map[string]any) { value["heartbeat"] = "250ms" })
	if err := stale.Control(s.Context, s.Run.Helper, "start_task", p.Tasks["a"].ID); err == nil {
		t.Fatal("changed config write accepted")
	}
	if controlRequestCount(t, p) != count || len(mustSnapshot(t, p).Domain.WorkflowRuns) != 0 {
		t.Fatal("rejected write published a request or workflow")
	}
	mirror := s.NewHomeForProject(p, "same-repo-second-home", p.Executable)
	output, err = s.RunCommandFor(mirror.Command(p.Executable, "run", "-config", mirror.Config, "-dispatch", "manual", "-console", "off"), "second-writer-refusal", 30*time.Second)
	if err == nil || !strings.Contains(string(output), "repository already locked") {
		t.Fatalf("same repo second writer accepted: %v: %s", err, output)
	}
	p.StopService()
	assertServiceStopReceipt(t, p.ServiceStopOutput)
	after := start("same-build-restart")
	assertProjectFactsEqual(t, before, after, "restart changed task facts")
	if serviceInstance(before) == "" || serviceInstance(after) == "" || serviceInstance(before) == serviceInstance(after) {
		t.Fatal("restart did not publish a new service instance")
	}
	count = controlRequestCount(t, p)
	err = p.Driver.ControlTargeted(s.Context, s.Run.Helper, "start_task", p.Tasks["a"].ID, serviceInstance(before))
	if err == nil || !strings.Contains(err.Error(), "stale_instance") || controlRequestCount(t, p) != count {
		t.Fatalf("old instance accepted after restart: %v", err)
	}
	p.StopService()
	assertServiceStopReceipt(t, p.ServiceStopOutput)
	s.Must(s.FinishBody())
}

func TestDualHomeIsolation(t *testing.T) {
	t.Parallel()
	s := testenv.NewScenario(t, "runtime.dual-home-isolation")
	a, b := s.NewProject("a"), s.NewProject("b")
	a.CreateTasks()
	b.CreateTasks()
	a.StartService()
	b.StartService()
	aBefore, bBefore := mustSnapshot(t, a), mustSnapshot(t, b)
	if aBefore.Page.ProjectID == bBefore.Page.ProjectID || aBefore.Page.DatabaseID == bBefore.Page.DatabaseID || serviceInstance(aBefore) == serviceInstance(bBefore) {
		t.Fatal("homes shared project/database/service identity")
	}
	a.Claim("a")
	requests := controlRequestFiles(t, b)
	b.Claim("a")
	writeRuntimeEvidence(t, s.Output, map[string]any{"normal_control_publication": assertControlPublication(t, b, requests, bBefore)})
	aOpen, bOpen := a.WaitPhase("waiting_pr_merge"), b.WaitPhase("waiting_pr_merge")
	for _, snap := range []testenv.Snapshot{aOpen, bOpen} {
		if snap.Helper.Dispatches != 3 || snap.Helper.Sessions != 1 || len(snap.Domain.WorkflowRuns) != 1 || len(snap.Domain.Executions) != 1 {
			t.Fatal("dual homes did not retain one independent original execution each")
		}
	}
	// Only A merges first: B's authority and provider must remain unchanged.
	a.Merge()
	aDone := a.WaitPhase("completed")
	bStillOpen := mustSnapshot(t, b)
	assertProjectFactsEqual(t, bOpen, bStillOpen, "A delivery changed B task facts")
	if bStillOpen.Helper.PR.Merged {
		t.Fatal("A merge changed B provider")
	}
	b.Merge()
	bDone := b.WaitPhase("completed")
	assertProjectFactsEqual(t, aDone, mustSnapshot(t, a), "B delivery changed A task facts")
	assertSameServiceInstance(t, aBefore, aDone)
	assertSameServiceInstance(t, bBefore, bDone)
	for _, item := range []struct {
		project    *testenv.Project
		open, done testenv.Snapshot
	}{{a, aOpen, aDone}, {b, bOpen, bDone}} {
		for _, task := range item.done.Page.Tasks {
			if task.TaskID == item.project.Tasks["a"].ID && (task.Status != "closed" || task.Delivery == nil || task.Delivery.SelectedWorkflowRunID != item.open.Domain.WorkflowRuns[0].WorkflowRunID) {
				t.Fatal("delivery does not belong to the home's original workflow")
			}
			if task.TaskID == item.project.Tasks["b"].ID && (!task.DependencyReady || len(task.WorkflowRuns) != 0) {
				t.Fatal("delivery did not release only its own unstarted successor")
			}
		}
	}
	a.Record("completed", aDone)
	b.Record("completed", bDone)
	a.StopService()
	b.StopService()
	s.Must(s.FinishBody())
}

func installFixture(t *testing.T, p *testenv.Project) {
	t.Helper()
	_, err := p.Driver.Run(p.Scenario.Context, "install")
	p.Scenario.Must(err)
	p.Executable = filepath.Join(p.Home, "bin", "steerline.exe")
	p.Driver.Binary = p.Executable
}

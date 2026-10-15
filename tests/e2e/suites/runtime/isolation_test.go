//go:build e2e

package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	p.StopService()
	assertDirectBuildOutputIsolation(t, s, p, p, powershellExecutable(t), map[string]any{})
	s.Must(s.FinishBody())
}

func TestPathIsolation(t *testing.T) {
	t.Parallel()
	s := testenv.NewScenario(t, "runtime.path-isolation")
	a, b := s.NewProject("protected"), s.NewProject("target")
	a.CreateTasks()
	a.StopService()
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
	withJunction(t, s, b, powershell, filepath.Join(b.Home, "state"), filepath.Join(a.Home, "state"), "state-alias", func() {
		output, err := s.RunCommand(b.Command(b.Executable, "check", "-config", config), "state-alias-refusal")
		if err == nil || !strings.Contains(strings.ToLower(string(output)), "state root must resolve inside steerline_home") {
			t.Fatalf("state alias was not rejected before initialization: %v: %s", err, output)
		}
		if fileSHA256(t, sentinel) != hash {
			t.Fatal("state alias refusal changed protected sentinel")
		}
		evidence["state_alias_refusal"] = string(output)
	})
	b.CreateTasks()
	b.StopService()
	for _, field := range []string{"run_dir", "worktree_root"} {
		outside := filepath.Join(a.Home, "escaped-"+field)
		variant := writeConfigVariant(t, b, field+".json", func(value map[string]any) { value[field] = outside })
		output, err := s.RunCommand(b.Command(b.Executable, "check", "-config", variant), field+"-refusal")
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
	if before.Runtime == nil || before.Runtime.Service.PID != p.Service.Cmd.Process.Pid || hostBefore.Backend.Identity.Process.PID != p.Service.Cmd.Process.Pid || before.Runtime.Service.ProcessIdentity == nil || !strings.EqualFold(before.Runtime.Service.ProcessIdentity.Executable, p.Executable) {
		t.Fatal("runtime is not owned by the exact home host")
	}
	p.Record("first-start", before)
	count := controlRequestCount(t, p)
	err = p.Driver.ControlTargeted(s.Context, s.Run.Helper, "start_task", p.Tasks["a"].ID, serviceInstance(before)+"-stale")
	if err == nil {
		t.Fatal("stale instance accepted")
	}
	stale := p.Driver
	stale.Config = writeConfigVariant(t, p, "Changed.json", func(value map[string]any) { value["heartbeat"] = "250ms" })
	if err := stale.Control(s.Context, s.Run.Helper, "start_task", p.Tasks["a"].ID); err == nil {
		t.Fatal("configuration absent from frozen host catalog accepted")
	}
	s.Must(os.Remove(stale.Config))
	if controlRequestCount(t, p) != count || len(mustSnapshot(t, p).Domain.WorkflowRuns) != 0 {
		t.Fatal("rejected write published request or workflow")
	}
	// A successful command proves that the rejection counter observes this transport.
	s.Must(p.Driver.Control(s.Context, s.Run.Helper, "resume_dispatch", ""))
	if controlRequestCount(t, p) != count+1 {
		t.Fatal("normal command did not reach the original control inbox")
	}
	mirror := s.NewHomeForProject(p, "same-repo-second-home", p.Executable)
	mirror.StartHost()
	s.Wait("cross-home repository refusal", func() (bool, error) {
		status, err := mirror.Driver.HostStatus(s.Context)
		if err != nil {
			return false, err
		}
		for _, project := range status.Projects {
			if project.RuntimeState == "failed" {
				if !strings.Contains(project.Diagnostic, "repository already locked") {
					t.Fatal(project.Diagnostic)
				}
				return true, nil
			}
		}
		return false, nil
	})
	if _, err := os.Stat(mirror.Database); !os.IsNotExist(err) {
		t.Fatal("conflicting home initialized a database")
	}
	mirror.StopService()
	p.StopService()
	assertHostStopped(t, p, hostBefore.Backend.Identity.InstanceID, serviceInstance(before))
	p.StartService()
	after := mustSnapshot(t, p)
	hostAfter, err := p.Driver.HostStatus(s.Context)
	s.Must(err)
	assertProjectFactsEqual(t, before, after, "host restart changed task facts")
	if serviceInstance(before) == "" || serviceInstance(after) == "" || serviceInstance(before) == serviceInstance(after) || hostBefore.Backend.Identity.InstanceID == hostAfter.Backend.Identity.InstanceID {
		t.Fatal("host restart retained old identities")
	}
	count = controlRequestCount(t, p)
	err = p.Driver.ControlTargeted(s.Context, s.Run.Helper, "start_task", p.Tasks["a"].ID, serviceInstance(before))
	if err == nil || controlRequestCount(t, p) != count {
		t.Fatalf("old instance accepted after restart: %v", err)
	}
	p.Record("after-restart", after)
	p.StopService()
	assertHostStopped(t, p, hostAfter.Backend.Identity.InstanceID, serviceInstance(after))
	s.Must(s.FinishBody())
}

func assertHostStopped(t *testing.T, p *testenv.Project, hostID, projectInstance string) {
	t.Helper()
	raw, err := p.Driver.Run(p.Scenario.Context, "status", "--json")
	p.Scenario.Must(err)
	var status struct {
		Backend struct {
			State string `json:"state"`
		} `json:"backend"`
		LastShutdown *struct {
			Clean    bool `json:"clean"`
			Identity struct {
				InstanceID string `json:"instance_id"`
			} `json:"identity"`
			Projects []struct {
				ProjectID        string `json:"project_id"`
				InstanceID       string `json:"instance_id"`
				LocalClosed      bool   `json:"local_closed"`
				LocksReleased    bool   `json:"locks_released"`
				ResourcesSettled bool   `json:"resources_settled"`
			} `json:"projects"`
		} `json:"last_shutdown"`
	}
	p.Scenario.Must(json.Unmarshal(raw, &status))
	if status.Backend.State != "exited" || status.LastShutdown == nil || !status.LastShutdown.Clean || status.LastShutdown.Identity.InstanceID != hostID || len(status.LastShutdown.Projects) != 1 {
		t.Fatalf("host stop is unverified: %s", raw)
	}
	result := status.LastShutdown.Projects[0]
	if result.ProjectID != p.ProjectID || result.InstanceID != projectInstance || !result.LocalClosed || !result.LocksReleased || !result.ResourcesSettled {
		t.Fatalf("project stop is unverified: %s", raw)
	}
	p.Scenario.Must(os.WriteFile(filepath.Join(p.Scenario.Output, "shutdown-"+hostID+".json"), raw, 0600))
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

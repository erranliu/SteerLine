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
		validRefusal := strings.Contains(strings.ToLower(string(output)), "must resolve inside steerline_home")
		if field == "worktree_root" {
			validRefusal = strings.Contains(string(output), "worktree_root") && strings.Contains(strings.ToLower(string(output)), "unknown")
		}
		if err == nil || !validRefusal {
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
	p := s.NewProject("service")
	p.CreateTasks()
	before := mustSnapshot(t, p)
	hostBefore, err := p.Driver.HostStatus(s.Context)
	s.Must(err)
	if before.Runtime == nil || before.Runtime.Service.PID != p.Service.Cmd.Process.Pid || hostBefore.Backend.Identity.Process.PID != p.Service.Cmd.Process.Pid || before.Runtime.Service.ProcessIdentity == nil || !strings.EqualFold(before.Runtime.Service.ProcessIdentity.Executable, p.Executable) {
		t.Fatal("runtime is not owned by the exact home host")
	}
	catalogID := ""
	for _, row := range hostBefore.Projects {
		if row.ConfigID == filepath.Base(p.Config) {
			catalogID = row.ProjectID
		}
	}
	if catalogID == "" {
		t.Fatal("host catalog lacks the original project identity")
	}
	p.Record("first-start", before)
	count := controlRequestCount(t, p)
	err = p.Driver.ControlTargeted(s.Context, s.Run.Helper, "start_task", p.Tasks["a"].ID, serviceInstance(before)+"-stale")
	if err == nil {
		t.Fatal("stale instance accepted")
	}
	stale := p.Driver
	stale.Config = writeConfigVariantAt(t, p.Config, filepath.Join(p.Home, "etc", "Changed.json"), func(value map[string]any) { value["heartbeat"] = "250ms" })
	if err := stale.Control(s.Context, s.Run.Helper, "start_task", p.Tasks["a"].ID); err == nil {
		t.Fatal("configuration absent from frozen host catalog accepted")
	}
	s.Must(os.Remove(stale.Config))
	if controlRequestCount(t, p) != count || len(mustSnapshot(t, p).Runner.Candidates) != 0 {
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
	assertHostStopped(t, p, hostBefore.Backend.Identity.InstanceID, catalogID, serviceInstance(before))
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
	assertHostStopped(t, p, hostAfter.Backend.Identity.InstanceID, catalogID, serviceInstance(after))
	s.Must(s.FinishBody())
}

func assertHostStopped(t *testing.T, p *testenv.Project, hostID, projectID, projectInstance string) {
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
	if result.ProjectID != projectID || result.InstanceID != projectInstance || !result.LocalClosed || !result.LocksReleased || !result.ResourcesSettled {
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
	aBefore, bBefore := mustSnapshot(t, a), mustSnapshot(t, b)
	if aBefore.Page.ProjectID == bBefore.Page.ProjectID || aBefore.Page.DatabaseID == bBefore.Page.DatabaseID || serviceInstance(aBefore) == serviceInstance(bBefore) || aBefore.Runtime.Service.PID == bBefore.Runtime.Service.PID {
		t.Fatal("homes shared project/database/host/runtime identity")
	}
	// Equal task and request IDs deliberately exercise the home/project namespace.
	if a.Tasks["a"].ID != b.Tasks["a"].ID {
		t.Fatal("fixture needs colliding task IDs")
	}
	requestID := strings.Repeat("d", 32)
	discard := func(p *testenv.Project, before testenv.Snapshot) {
		raw, args := discardOwnedTask(t, p, before, "a", requestID)
		after := mustSnapshot(t, p)
		replay, err := p.Driver.Run(s.Context, args...)
		s.Must(err)
		if strings.TrimSpace(string(replay)) != strings.TrimSpace(string(raw)) {
			t.Fatal("original request replay changed its receipt")
		}
		unchanged := mustSnapshot(t, p)
		assertProjectFactsEqual(t, after, unchanged, "request replay changed task facts")
		if after.Page.GraphRevision != unchanged.Page.GraphRevision {
			t.Fatal("request replay changed graph revision")
		}
		p.Record("discarded", after)
	}
	bProvider := fileSHA256(t, b.Provider)
	discard(a, aBefore)
	aDone := mustSnapshot(t, a)
	assertProjectFactsEqual(t, bBefore, mustSnapshot(t, b), "A command changed B")
	if fileSHA256(t, b.Provider) != bProvider {
		t.Fatal("A command changed B provider")
	}
	aProvider := fileSHA256(t, a.Provider)
	discard(b, bBefore)
	assertProjectFactsEqual(t, aDone, mustSnapshot(t, a), "B command changed A")
	if fileSHA256(t, a.Provider) != aProvider {
		t.Fatal("B command changed A provider")
	}
	for _, p := range []*testenv.Project{a, b} {
		snap := mustSnapshot(t, p)
		if len(snap.Runner.Candidates) != 0 {
			t.Fatal("isolation check dispatched an execution")
		}
	}
	assertSameServiceInstance(t, aBefore, mustSnapshot(t, a))
	assertSameServiceInstance(t, bBefore, mustSnapshot(t, b))
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

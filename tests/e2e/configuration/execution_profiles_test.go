//go:build e2e

package configuration

import (
	"context"
	"encoding/json"
	cfg "github.com/erranliu/SteerLine/internal/executionconfig"
	"github.com/erranliu/SteerLine/internal/localservice"
	"github.com/erranliu/SteerLine/internal/product/applicationclient"
	testprocess "github.com/erranliu/SteerLine/internal/testkit/process"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
	"github.com/erranliu/SteerLine/tests/e2e/internal/driver"
	"github.com/erranliu/SteerLine/tests/e2e/internal/testenv"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExecutionProfiles(t *testing.T) {
	t.Parallel()
	s := testenv.NewScenario(t, "configuration.execution-profiles")
	p := s.NewProject("SteerlineTest")
	path := filepath.Join(p.Home, "execution.json")
	raw, err := os.ReadFile(path)
	s.Must(err)
	var c cfg.File[cfg.Pin]
	s.Must(cfg.Decode(raw, &c))
	c.Profiles[0].Execution.Settings = json.RawMessage(`{"access_mode":"full_access","executable":` + strconvQuote(s.Run.Helper) + `,"model":"fixture-original","reasoning_effort":"high"}`)
	c.Profiles[0].Workflow.Version = 2
	for _, agent := range []string{"codex", "pi"} {
		template := c.Profiles[0]
		template.ID = "template-" + agent
		template.Name = "部署模板 " + agent
		template.Execution.Adapter = "orca-terminal"
		template.Execution.Settings, _ = json.Marshal(map[string]any{"access_mode": "full_access", "agent": agent})
		c.Profiles = append(c.Profiles, template)
	}
	s.Must(controlled.Write(path, c))
	p.PrepareControlledEnvironment("controlled")
	p.CreateTasks()
	p.Claim("a")
	before := p.WaitPhase("waiting_pr_merge")
	p.Record("before-config-save", before)
	oldID := before.Runner.Candidates[0].ID
	oldDigest := before.Runner.Candidates[0].InputDigest
	web := filepath.Join(p.Root, "steerline-web.exe")
	build := exec.Command("go", "build", "-tags=webui", "-o", web, "./cmd/steerline-web")
	build.Dir = s.Run.Root
	_, err = s.RunCommand(build, "build-web")
	s.Must(err)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	s.Must(err)
	address := listener.Addr().String()
	listener.Close()
	s.Must(localservice.WritePrivateFile(p.Home, "web.json", map[string]any{"version": 1, "address": address}))
	// The caller fixture normally publishes an unconstrained object schema.
	// Give this Web test an explicit offline contract and an absent, owned Orca
	// userData so a controlled save cannot query the user's live Orca runtime.
	offlineCaps := filepath.Join(p.Root, "web-offline-capabilities.json")
	metadata, err := os.ReadFile(filepath.Join(s.Run.Root, "internal/testkit/protocol/configuration_catalog.json"))
	s.Must(err)
	s.Must(os.WriteFile(offlineCaps, metadata, 0600))
	orcaData := filepath.Join(p.Root, "owned-orca-unavailable")
	s.Must(os.MkdirAll(orcaData, 0700))
	startWeb := func() *testprocess.Process {
		log, err := os.Create(filepath.Join(s.Output, "web-"+time.Now().Format("150405.000")+".log"))
		s.Must(err)
		cmd := p.Command(web, "serve")
		cmd.Env = append(cmd.Env, "STEERLINE_TEST_WEB_OFFLINE_CAPABILITIES="+offlineCaps, "ORCA_USER_DATA_PATH="+orcaData)
		cmd.Stdout = log
		cmd.Stderr = log
		process, err := testprocess.Start(cmd)
		s.Must(err)
		s.RecordResource("web-process", web, process.Cmd.Process.Pid)
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			process.Stop(ctx)
			log.Close()
		})
		s.Wait("Web ready", func() (bool, error) {
			_, err := p.Driver.RunHelper(s.Context, web, "open", "-print-url")
			return err == nil, err
		})
		return process
	}
	browser := func(phase string) {
		cmd := p.Command("npm.cmd", "--prefix", filepath.Join(s.Run.Root, "web"), "run", "test:e2e", "--", "execution-configs.spec.ts")
		cmd.Env = append(cmd.Env, "STEERLINE_CONFIG_WEB="+web, "STEERLINE_CONFIG_PHASE="+phase, "STEERLINE_WEB_EVIDENCE="+filepath.Join(s.Output, "browser-"+phase))
		cmd.Dir = s.Run.Root
		_, err := s.RunCommandFor(cmd, "browser-"+phase, 90*time.Second)
		s.Must(err)
	}
	webProcess := startWeb()
	browser("edit")
	startCandidate := func(profile, requestID string) (driver.CandidateReceipt, error) {
		snap, e := p.Snapshot()
		if e != nil {
			return driver.CandidateReceipt{}, e
		}
		var version uint64
		for _, task := range snap.Page.Tasks {
			if task.TaskID == p.Tasks["a"].ID {
				version = task.Version
			}
		}
		return p.Driver.Candidate(s.Context, s.Run.Helper, driver.CandidateCommand{Action: "start_candidate", RequestID: requestID, TaskID: p.Tasks["a"].ID, TaskVersion: version, GraphRevision: snap.Page.GraphRevision, ProfileID: profile})
	}
	afterSave, err := p.Snapshot()
	s.Must(err)
	for _, candidate := range afterSave.Runner.Candidates {
		if candidate.ID == oldID && candidate.InputDigest != oldDigest {
			t.Fatal("saved profile rewrote old input")
		}
	}
	browser("loaded")
	// Stop only the Web surface. The execution consumer must continue without
	// Web caches or a capability query receipt.
	_, err = p.Driver.RunHelper(s.Context, web, "stop")
	s.Must(err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	s.Must(webProcess.Wait(ctx))
	cancel()
	raw, err = os.ReadFile(path)
	s.Must(err)
	s.Must(cfg.Decode(raw, &c))
	p.Driver.Profile = c.DefaultProfile
	expected := map[string]cfg.Profile{}
	for _, profile := range c.Profiles {
		if strings.HasPrefix(profile.Name, "浏览器保存的新配置") {
			expected[profile.ID] = profile
		}
	}
	if len(expected) != 2 {
		t.Fatalf("missing browser Agent combinations: %d", len(expected))
	}
	if _, ok := expected[c.DefaultProfile]; !ok {
		t.Fatalf("new default was not saved: %s", c.DefaultProfile)
	}
	// The saved agents have distinct environment keys. Commit their fixture
	// entrypoints, refresh the public baseline/materials, then prepare each key.
	// The PR fixture leaves its owned repository on the candidate branch.
	// Prepare on main, and finish all environment work before another candidate
	// is allowed to switch this fixture repository again.
	_, err = controlled.Git(s.Context, p.Repo, "checkout", "main")
	s.Must(err)
	p.WriteControlledEnvironment()
	project, err := applicationclient.OpenProject(s.Context, p.Home, filepath.Base(p.Config))
	s.Must(err)
	_, err = driver.WaitFixtureMaterials(s.Context, p.Home, project.Identity(), "SteerlineFlow/plans/task.plan.md", filepath.Join(s.Output, "saved-profile-materials"))
	project.Close()
	s.Must(err)
	for _, profile := range c.Profiles {
		if _, ok := expected[profile.ID]; ok {
			p.ValidateDeclaredControlledEnvironment(profile.ID)
		}
	}
	requestIndex := 0
	for _, profile := range c.Profiles {
		if _, ok := expected[profile.ID]; !ok {
			continue
		}
		requestIndex++
		receipt, err := startCandidate(profile.ID, strings.Repeat(string(rune('b'+requestIndex-1)), 32))
		s.Must(err)
		if receipt.Control == nil || receipt.Control.Status != "applied" {
			t.Fatalf("new configuration rejected: %+v", receipt)
		}
	}
	s.Wait("new profile consumption", func() (bool, error) {
		snap, e := p.Snapshot()
		if e != nil {
			return false, e
		}
		foundOld := false
		found := map[string]bool{}
		for _, candidate := range snap.Runner.Candidates {
			if candidate.ID == oldID {
				foundOld = candidate.InputDigest == oldDigest
				continue
			}
			var input struct {
				Execution struct {
					Adapter string `json:"adapter"`
					Target  struct {
						Kind        string `json:"kind"`
						Environment string `json:"environment"`
					} `json:"target"`
					Settings struct {
						Agent  string `json:"agent"`
						Launch struct {
							Executable string `json:"executable"`
							Model      string `json:"model"`
							Effort     string `json:"reasoning_effort"`
							Runtime    string `json:"runtime_executable"`
							SHA        string `json:"runtime_sha256"`
						} `json:"agent_launch"`
					} `json:"settings"`
				} `json:"execution"`
			}
			if json.Unmarshal(candidate.Submit.Input, &input) != nil {
				continue
			}
			execution := input.Execution
			launch := execution.Settings.Launch
			agent := execution.Settings.Agent
			if execution.Adapter == "orca-terminal" && execution.Target.Kind == "local" && execution.Target.Environment == "" && launch.Model == "fixture-"+agent && launch.Effort == "high" && launch.Runtime == "" && launch.SHA == "" && launch.Executable == "" {
				found[agent] = true
			}
		}
		return foundOld && len(found) == 2, nil
	})
	snap, err := p.Snapshot()
	s.Must(err)
	if snap.Runner.Checkpoint.Service.InstanceID != before.Runner.Checkpoint.Service.InstanceID {
		t.Fatal("project service restarted during profile reload")
	}
	p.Record("after-profile-reload-and-web-off", snap)
	p.StopService()
	s.Must(s.FinishBody())
}
func strconvQuote(s string) string { b, _ := json.Marshal(s); return string(b) }

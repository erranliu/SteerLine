//go:build e2e

package configuration

import (
	"context"
	"encoding/json"
	cfg "github.com/erranliu/SteerLine/internal/executionconfig"
	"github.com/erranliu/SteerLine/internal/localservice"
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
	c.Profiles[0].Execution.Settings = json.RawMessage(`{"executable":` + strconvQuote(s.Run.Helper) + `,"model":"fixture-original","reasoning_effort":"high"}`)
	helperBytes, err := os.ReadFile(s.Run.Helper)
	s.Must(err)
	helperSHA := cfg.Digest(helperBytes)
	for _, agent := range []string{"codex", "pi", "dsh"} {
		template := c.Profiles[0]
		template.ID = "template-" + agent
		template.Name = "部署模板 " + agent
		template.Execution.Adapter = "orca-terminal"
		template.Execution.Settings, _ = json.Marshal(map[string]any{"agent": agent, "transport": map[string]any{"executable": "orca.exe"}, "agent_launch": map[string]any{"executable": map[string]string{"codex": "codex", "pi": "pi.cmd", "dsh": "dsh.cmd"}[agent], "runtime_executable": s.Run.Helper, "runtime_sha256": helperSHA}})
		c.Profiles = append(c.Profiles, template)
	}
	s.Must(controlled.Write(path, c))
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
	efforts := map[string]any{"enum": []string{"high"}}
	s.Must(controlled.Write(offlineCaps, map[string]any{
		"workflows":             []any{map[string]any{"id": "development", "version": 1, "supported_adapters": []string{"codex-app-server", "orca-terminal"}, "audit_model_required": false}},
		"configuration_schemas": map[string]any{"version": 1, "run_configuration": map[string]any{"type": "object", "properties": map[string]any{"execution": map[string]any{"properties": map[string]any{"settings": map[string]any{"properties": map[string]any{"reasoning_effort": efforts, "agent_launch": map[string]any{"properties": map[string]any{"reasoning_effort": efforts}}}}}}}}, "adapters": map[string]any{"codex-app-server": map[string]any{"properties": map[string]any{"reasoning_effort": efforts}}}},
	}))
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
	firstWeb := startWeb()
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
	rejected, rejectErr := startCandidate("controlled", strings.Repeat("a", 32))
	if rejectErr == nil && (rejected.Control == nil || rejected.Control.Status != "rejected" && rejected.Control.Status != "failed") {
		t.Fatalf("changed file allowed new candidate: %+v", rejected)
	}
	if rejectErr != nil && !(strings.Contains(rejectErr.Error(), "identity-error") && strings.Contains(rejectErr.Error(), "cannot verify that this process instance owns the configured run_dir")) {
		t.Fatalf("unexpected failure instead of configuration gate: %v", rejectErr)
	}
	afterSave, err := p.Snapshot()
	s.Must(err)
	for _, candidate := range afterSave.Runner.Candidates {
		if candidate.ID == oldID && candidate.InputDigest != oldDigest {
			t.Fatal("saved profile rewrote old input")
		}
	}
	p.StopService()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	s.Must(firstWeb.Wait(ctx))
	cancel()
	p.StartService()
	secondWeb := startWeb()
	browser("loaded")
	// Stop only the Web surface. The execution consumer must continue without
	// Web caches or a capability query receipt.
	_, err = p.Driver.RunHelper(s.Context, web, "stop")
	s.Must(err)
	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	s.Must(secondWeb.Wait(ctx))
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
	if len(expected) != 3 {
		t.Fatalf("missing browser Agent combinations: %d", len(expected))
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
			if execution.Adapter == "orca-terminal" && execution.Target.Kind == "local" && execution.Target.Environment == "" && launch.Model == "fixture-"+agent && launch.Effort == "high" && launch.Runtime == s.Run.Helper && launch.SHA == helperSHA && launch.Executable == map[string]string{"codex": "codex", "pi": "pi.cmd", "dsh": "dsh.cmd"}[agent] {
				found[agent] = true
			}
		}
		return foundOld && len(found) == 3, nil
	})
	snap, err := p.Snapshot()
	s.Must(err)
	p.Record("after-restart-and-web-off", snap)
	p.StopService()
	s.Must(s.FinishBody())
}
func strconvQuote(s string) string { b, _ := json.Marshal(s); return string(b) }

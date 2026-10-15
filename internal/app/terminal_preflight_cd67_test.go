package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/erranliu/SteerLine/internal/testkit/protocol"

	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/execution/adapters/codex"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func TestMain(m *testing.M) {
	if record := os.Getenv("STEERLINE_TEST_OPENER_RECORD"); record != "" && len(os.Args) == 2 && os.Args[1] == "open" {
		cwd, _ := os.Getwd()
		raw, _ := json.Marshal(map[string]any{"executable": os.Args[0], "args": os.Args[1:], "home": os.Getenv("STEERLINE_HOME"), "cwd": cwd})
		if err := os.WriteFile(record, raw, 0600); err != nil {
			os.Exit(3)
		}
		if os.Getenv("STEERLINE_TEST_OPENER_FAIL") == "1" {
			os.Exit(2)
		}
		os.Exit(0)
	}
	for i, arg := range os.Args {
		if arg == "--terminal-fixture" && i+1 < len(os.Args) {
			os.Exit(testprotocol.FakeTerminal(os.Args[i+1], os.Args[i+2:]))
		}
	}
	if os.Getenv("STEERLINE_APP_SERVER_FIXTURE") == "1" {
		switch strings.Join(os.Args[1:], " ") {
		case "--version":
			fmt.Println("codex-cli 0.159.2")
		case "app-server --help":
			fmt.Println("app-server --listen")
		case "app-server --stdio":
			testprotocol.MockAppServerStdio()
		default:
			os.Exit(2)
		}
		os.Exit(0)
	}
	for i, arg := range os.Args {
		if arg == "--terminal-fixture" && i+1 < len(os.Args) {
			os.Exit(testprotocol.FakeTerminal(os.Args[i+1], os.Args[i+2:]))
		}
		if arg == "--version" && os.Getenv("STEERLINE_APP_VERSION_FIXTURE") == "1" {
			fmt.Println("codex-cli 0.159.2")
			os.Exit(0)
		}
		if arg == "--identity-fixture" {
			time.Sleep(time.Minute)
			os.Exit(0)
		}
	}
	home, err := os.MkdirTemp("", "steerlineapp-fixture-")
	if err != nil {
		panic(err)
	}
	if err := os.Setenv("STEERLINE_HOME", home); err != nil {
		panic(err)
	}
	code := m.Run()
	// Only the exact directory created above is removed. All default app
	// config/state paths in this test process are inside this isolated home.
	if relative, err := filepath.Rel(os.TempDir(), home); err == nil && !filepath.IsAbs(relative) && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		_ = os.RemoveAll(home)
	}
	os.Exit(code)
}

func TestExecutionBackendCommonCapabilityContract(t *testing.T) {
	t.Setenv("STEERLINE_APP_VERSION_FIXTURE", "1")
	for _, name := range []string{"local", "orca"} {
		c := bindUnitDirectory(t, Config{Host: name, Agent: "codex", Repository: t.TempDir(), CommandTimeout: "2s"})
		var terminal *terminalFixtureProfile
		if name == "orca" {
			c.Repository = configTestRepository(t)
			fixtureGit(t, c.Repository, "remote", "add", "origin", "https://github.com/owner/repo.git")
			c.Git = executionbackend.Command{Executable: "git"}
			c.Remote = "origin"
			c.PublicationPolicy = executionbackend.PublicationPolicy{Publisher: executionbackend.PublisherWorker}
			terminal = &terminalFixtureProfile{Executable: os.Args[0], Args: []string{"--terminal-fixture", t.TempDir()}, Shell: "powershell.exe", WaitTimeout: "2s"}
		}
		profile := boundProfile(c, testExecutionProfile(t, "probe", "Probe", name, "codex", codexworker.ProfileConfig{Executable: os.Args[0]}, terminal))
		if reason := profile.StaticUnavailableReason(); reason != "" {
			t.Fatalf("probe fixture profile is invalid: %s", reason)
		}
		c.ExecutionProfiles, c.DefaultExecutionProfile = []ExecutionProfile{profile}, profile.ID
		c = c.ApplyExecutionProfile(profile)
		executionConfig := c.executionConfig()
		entry, err := c.ExecutionDirectory.Resolve(executionConfig)
		require(t, err)
		backend := entry.Backend(executionConfig)
		kind, protocol := backend.Identity()
		report, err := backend.Probe(context.Background(), entry.Target(executionConfig), executionbackend.AgentSpec{Name: c.Agent})
		require(t, err)
		if report.Backend != kind || report.Protocol != protocol || report.CheckedAt.IsZero() {
			t.Fatalf("production probe lost fixed backend identity: %+v", report)
		}
		for _, capability := range report.Capabilities {
			if capability.Status == executionbackend.CapabilityVerified {
				t.Fatalf("static declaration claimed verification: %+v", capability)
			}
		}
	}
}

func TestLocalObservationRejectsReusedPIDIdentity(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows process creation identity")
	}
	host := NewProcessHost(5 * time.Second)
	command := exec.Command(os.Args[0], "--identity-fixture")
	handle, err := host.StartManaged(command)
	require(t, err)
	defer func() { handle.Kill(); _ = command.Wait(); handle.Close() }()
	identity := handle.Identity()
	identity.Created = "different-process-generation"
	directory, err := NewBuiltinDirectory(host)
	require(t, err)
	config := Config{Host: "local", Agent: "codex"}.executionConfig()
	entry, err := directory.Resolve(config)
	require(t, err)
	ref := executionbackend.ExecutionRef{Backend: executionbackend.BackendLocal, Protocol: executionbackend.ProtocolLocalCodexAppServer, Target: entry.Target(config), External: identity}
	_, err = entry.Backend(config).Observe(context.Background(), ref)
	var backendErr *executionbackend.BackendError
	if !errors.As(err, &backendErr) || backendErr.Code != executionbackend.BackendIdentityMismatch {
		t.Fatalf("reused PID identity was accepted by the production backend: %v", err)
	}
}

func TestOrcaPreflightDoesNotProbeAgentExecutable(t *testing.T) {
	c, _ := nativeTaskFixture(t)
	stateDir := t.TempDir()
	missing := filepath.Join(stateDir, "missing-worker.exe")
	c.Host, c.Agent = "orca", "codex"
	c.PublicationPolicy = executionbackend.PublicationPolicy{Publisher: executionbackend.PublisherWorker}
	c.PR.Query = executionbackend.Command{Executable: os.Args[0]}
	terminal := &terminalFixtureProfile{Executable: os.Args[0], Args: []string{"--terminal-fixture", stateDir}, Shell: "cmd.exe", WaitTimeout: "5s"}
	profile := testExecutionProfile(t, "terminal", "Terminal", "orca", "codex", codexworker.ProfileConfig{Executable: missing, Model: "fixture-model", ReasoningEffort: "high"}, terminal)
	c.ExecutionProfiles, c.DefaultExecutionProfile = []ExecutionProfile{profile}, profile.ID
	c = c.ApplyExecutionProfile(profile)
	require(t, Preflight(context.Background(), &c, false))
	if len(profile.ImplementationConfigs) != 1 || profile.ImplementationConfigs[0].ImplementationKey != "orca.terminal-profile" {
		t.Fatal("Orca preflight retained agent-owned executable configuration")
	}
}

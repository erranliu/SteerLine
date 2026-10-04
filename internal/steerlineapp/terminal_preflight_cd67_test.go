package steerlineapp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
	"github.com/erranliu/SteerLine/internal/steerline/testprotocol"
)

func TestMain(m *testing.M) {
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
		var terminal *orcaworker.ProfileConfig
		if name == "orca" {
			c.PublicationPolicy = executionapi.PublicationPolicy{Publisher: executionapi.PublisherWorker}
			terminal = &orcaworker.ProfileConfig{Executable: os.Args[0], Args: []string{"--terminal-fixture", t.TempDir()}, Shell: "powershell.exe", WaitTimeout: "2s"}
		}
		profile := boundProfile(c, testExecutionProfile(t, "probe", "Probe", name, "codex", codexworker.ProfileConfig{Executable: os.Args[0]}, terminal))
		if reason := profile.StaticUnavailableReason(); reason != "" {
			t.Fatalf("probe fixture profile is invalid: %s", reason)
		}
		c.ExecutionProfiles, c.DefaultExecutionProfile = []ExecutionProfile{profile}, profile.ID
		c = c.ApplyExecutionProfile(profile)
		entry, err := c.ExecutionDirectory.Resolve(c)
		require(t, err)
		backend := entry.Backend(c)
		kind, protocol := backend.Identity()
		report, err := backend.Probe(context.Background(), entry.Target(c), executionapi.AgentSpec{Name: c.Agent})
		require(t, err)
		if report.Backend != kind || report.Protocol != protocol || report.CheckedAt.IsZero() {
			t.Fatalf("production probe lost fixed backend identity: %+v", report)
		}
		for _, capability := range report.Capabilities {
			if capability.Status == executionapi.CapabilityVerified {
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
	config := Config{Host: "local", Agent: "codex"}
	entry, err := directory.Resolve(config)
	require(t, err)
	ref := executionapi.ExecutionRef{Backend: executionapi.BackendLocal, Protocol: executionapi.ProtocolLocalCodexAppServer, Target: entry.Target(config), External: identity}
	_, err = entry.Backend(config).Observe(context.Background(), ref)
	var backendErr *executionapi.BackendError
	if !errors.As(err, &backendErr) || backendErr.Code != executionapi.BackendIdentityMismatch {
		t.Fatalf("reused PID identity was accepted by the production backend: %v", err)
	}
}

func TestOrcaPreflightRequiresConfiguredCodexPolicyMapping(t *testing.T) {
	c, _ := nativeTaskFixture(t)
	stateDir := t.TempDir()
	missing := filepath.Join(stateDir, "missing-worker.exe")
	c.Host, c.Agent = "orca", "codex"
	c.PublicationPolicy = executionapi.PublicationPolicy{Publisher: executionapi.PublisherWorker}
	c.PR.Query, c.PR.Close = Command{Executable: os.Args[0]}, Command{Executable: os.Args[0]}
	terminal := &orcaworker.ProfileConfig{Executable: os.Args[0], Args: []string{"--terminal-fixture", stateDir}, Shell: "powershell.exe", WaitTimeout: "5s"}
	applyProfile := func(worker codexworker.ProfileConfig) {
		profile := testExecutionProfile(t, "terminal", "Terminal", "orca", "codex", worker, terminal)
		c.ExecutionProfiles, c.DefaultExecutionProfile = []ExecutionProfile{profile}, profile.ID
		c = c.ApplyExecutionProfile(profile)
	}
	applyProfile(codexworker.ProfileConfig{Executable: missing, WindowsSandbox: "unelevated"})
	if err := c.Preflight(context.Background(), false); err == nil || !strings.Contains(err.Error(), "missing-worker.exe") {
		t.Fatalf("native policy mapping did not require its fixed tool: %v", err)
	}
	// A fixed model actually requires the configured local tool. Probe the
	// production registration so an unrelated profile-format error cannot
	// satisfy the rejection assertion.
	applyProfile(codexworker.ProfileConfig{Executable: missing, WindowsSandbox: "unelevated", Model: "fixture-fixed-model", ReasoningEffort: "high"})
	require(t, c.ValidateProfiles())
	entry, err := c.ExecutionDirectory.Resolve(c)
	require(t, err)
	_, err = entry.Backend(c).Probe(context.Background(), entry.Target(c), executionapi.AgentSpec{Name: c.Agent})
	if err == nil || !strings.Contains(err.Error(), "missing-worker.exe") {
		t.Fatalf("fixed local tool was not required by the native backend: %v", err)
	}
}

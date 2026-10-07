package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	steerline "github.com/erranliu/SteerLine/internal/app/host"
	"github.com/erranliu/SteerLine/internal/execution"
	codexworker "github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func configuredLifecycleApp(t *testing.T) Config {
	t.Helper()
	root := t.TempDir()
	t.Setenv("STEERLINE_HOME", filepath.Join(root, "home"))
	t.Setenv("STEERLINE_APP_SERVER_FIXTURE", "1")
	profile := testExecutionProfile(t, "local", "Local", "local", "codex", codexworker.ProfileConfig{Executable: os.Args[0]}, nil)
	writeGlobalExecutionFixture(t, []ExecutionProfile{profile}, "local")
	repository := projectRepo(t)
	if output, err := exec.Command("git", "-C", repository, "remote", "add", "origin", "https://github.com/owner/repo.git").CombinedOutput(); err != nil {
		t.Fatalf("configure fixture remote: %v: %s", err, output)
	}
	input := minimalConfig(repository)
	input.PR.Query = executionbackend.Command{Executable: "git", Args: []string{"status"}}
	configPath := filepath.Join(root, "project.json")
	require(t, os.WriteFile(configPath, projectConfigJSON(t, input), 0600))
	config, err := LoadConfig(configPath)
	require(t, err)
	return config
}

func TestRunConfiguredServiceRejectsBeadsBeforeStartupWrites(t *testing.T) {
	config := configuredLifecycleApp(t)
	beads := filepath.Join(config.Repository, ".beads")
	require(t, os.Mkdir(beads, 0700))
	if _, err := os.Stat(config.RunDir); !os.IsNotExist(err) {
		t.Fatalf("fixture run directory already exists: %v", err)
	}
	err := RunConfiguredService(context.Background(), config, "manual", "", "off")
	if err == nil || !strings.Contains(err.Error(), "Beads state exists") {
		t.Fatalf("actual app service admission did not reject Beads state: %v", err)
	}
	if _, err := os.Stat(config.RunDir); !os.IsNotExist(err) {
		t.Fatalf("rejected app startup created run state: %v", err)
	}
	if info, err := os.Stat(beads); err != nil || !info.IsDir() {
		t.Fatalf("rejected app startup changed Beads state: %v", err)
	}
}

func TestRunConfiguredServiceClosesTransferredStoreOnReturn(t *testing.T) {
	config := configuredLifecycleApp(t)
	// Construction performs multiple bounded Git/native probes before the host
	// starts. This test checks store ownership across exit, not startup latency.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	created := make(chan *execution.Service, 1)
	done := make(chan error, 1)
	go func() {
		done <- runConfiguredService(ctx, config, "manual", "", "off", func(service *execution.Service) {
			created <- service
		})
	}()
	var service *execution.Service
	select {
	case service = <-created:
	case err := <-done:
		t.Fatalf("app service construction failed: %v", err)
	case <-ctx.Done():
		t.Fatal("app service construction timed out")
	}
	hostConfig, err := hostRuntimeConfig(ctx, config)
	require(t, err)
	deadline := time.Now().Add(15 * time.Second)
	for {
		connection := steerline.ConnectService(ctx, hostConfig)
		if connection.CanControl {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			<-done
			t.Fatalf("actual app service did not become ready: %+v", connection)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := service.Heartbeat(); err != nil {
		t.Fatalf("transferred SQLite store was unusable before service exit: %v", err)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("lock-owning app host returned an error: %v", err)
	}
	if err := service.Heartbeat(); err == nil {
		t.Fatal("lock-owning app host returned while its transferred SQLite store remained usable")
	}
}

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/steerline"
	"github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	"github.com/erranliu/SteerLine/internal/steerlineapp"
)

var cliPath string

func TestMain(m *testing.M) {
	repoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		fmt.Fprintln(os.Stderr, "locate repository root:", err)
		os.Exit(1)
	}
	buildDir, err := os.MkdirTemp("", "steerline-integration-build-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "create CLI build directory:", err)
		os.Exit(1)
	}
	name := "steerline-integration"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	cliPath = filepath.Join(buildDir, name)
	build := exec.Command("go", "build", "-o", cliPath, "./cmd/steerline")
	build.Dir = repoRoot
	if output, err := build.CombinedOutput(); err != nil {
		_ = os.RemoveAll(buildDir)
		fmt.Fprintf(os.Stderr, "build integration CLI once: %v\n%s", err, output)
		os.Exit(1)
	}
	code := m.Run()
	_ = os.RemoveAll(buildDir)
	os.Exit(code)
}

func TestCLIRejectsLegacyTaskAuthorityBeforeStateInitialization(t *testing.T) {
	temp := t.TempDir()
	configPath := filepath.Join(temp, "legacy.json")
	runDir := filepath.Join(temp, "state")
	writeConfig(t, configPath, map[string]any{
		"repository":     filepath.Join(temp, "repo"),
		"task_authority": "beads",
		"remote":         "origin",
		"branch":         "main",
		"run_dir":        runDir,
	})

	output, err := runCLI(t, filepath.Join(temp, "home"), "run", "-config", configPath)
	if err == nil || !strings.Contains(string(output), "task_authority must be explicitly set to native") {
		t.Fatalf("CLI admitted a retired task authority: err=%v output=%s", err, output)
	}
	for _, path := range []string{runDir, filepath.Join(runDir, "runtime.json"), filepath.Join(runDir, "steerline.db")} {
		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			t.Fatalf("rejected legacy authority created state at %s: %v", path, statErr)
		}
	}
}

func TestTaskCLIProcessUsesNativeAuthorityWithoutInitializingMissingRead(t *testing.T) {
	temp := t.TempDir()
	repo := filepath.Join(temp, "repo")
	remote := filepath.Join(temp, "origin.git")
	for _, args := range [][]string{
		{"init", "--quiet", "-b", "main", repo},
		{"init", "--quiet", "--bare", remote},
		{"-C", repo, "remote", "add", "origin", "https://github.com/owner/repo.git"},
	} {
		if output, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	configPath := filepath.Join(temp, "native.json")
	home := filepath.Join(temp, "home")
	t.Setenv("STEERLINE_HOME", home)
	if err := os.MkdirAll(home, 0700); err != nil {
		t.Fatal(err)
	}
	profilePayload, _ := codexworker.EncodeProfileConfig(codexworker.ProfileConfig{Executable: "codex", WindowsSandbox: "elevated"})
	writeConfig(t, steerlineapp.GlobalExecutionConfigPath(), map[string]any{"version": 2, "execution_profiles": []map[string]any{{"id": "local", "name": "Local Codex", "host": "local", "agent": "codex", "implementation_configs": []executionbackend.ImplementationConfigInput{{ImplementationKey: codexworker.ProfileCodecKey, CodecVersion: 1, Payload: profilePayload}}}}})
	writeConfig(t, configPath, map[string]any{
		"repository":         repo,
		"remote":             "origin",
		"branch":             "main",
		"task_authority":     "native",
		"workflow_protocol":  executionmodel.Version,
		"execution_policy":   executionbackend.ExecutionPolicy{Filesystem: executionbackend.FilesystemPolicy{ReadScope: "host", WriteScope: "workspace", GitMetadataWrite: true}, Network: "allow", Escalation: "deny"},
		"publication_policy": executionbackend.PublicationPolicy{Publisher: executionbackend.PublisherWorker},
		"pr":                 map[string]any{"kind": "command", "base_url": "https://github.com", "repository": "owner/repo", "query": map[string]any{"executable": "gh"}},
	})
	config, err := steerlineapp.LoadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	runDir := config.RunDir
	exportPath := filepath.Join(temp, "before-initialization.json")

	if output, err := runCLI(t, home, "task", "graph", "-config", configPath); err == nil {
		t.Fatalf("task graph accepted missing state: %s", output)
	}
	if output, err := runCLI(t, home, "task", "export", "-config", configPath, "-format", "json", "-output", exportPath); err == nil {
		t.Fatalf("task export accepted missing state: %s", output)
	}
	if _, err := os.Stat(exportPath); !os.IsNotExist(err) {
		t.Fatalf("failed export published a file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(runDir, "steerline.db")); !os.IsNotExist(err) {
		t.Fatalf("read commands initialized missing native state: %v", err)
	}

	store, err := steerline.OpenNativeTaskProject(context.Background(), &config)
	if err != nil {
		t.Fatal(err)
	}
	before, err := (steerline.NewQueryService(store, config.Identity())).ListNativeTasks(context.Background(), "", 100)
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}

	output, err := runCLI(t, home, "task", "list", "-config", configPath)
	if err != nil {
		t.Fatalf("native task list process failed: %v: %s", err, output)
	}
	var page steerline.NativeTaskPage
	if err = json.Unmarshal(output, &page); err != nil {
		t.Fatalf("invalid native task JSON: %v: %s", err, output)
	}
	if page.Protocol != "native-tasks/v1" || page.ProjectID != config.Identity() || page.SampledAt.IsZero() || len(page.Tasks) != 0 || page.Capabilities.DependencyReadiness != "provided" || page.DatabaseID != before.DatabaseID || page.DatabaseRevision != before.DatabaseRevision {
		t.Fatalf("unexpected native task page or read changed persistence: before=%+v after=%+v", before, page)
	}
	if _, err = os.Stat(filepath.Join(runDir, "steerline.db")); err != nil {
		t.Fatalf("native database was not persisted: %v", err)
	}

	if output, err = runCLI(t, home, "task", "show", "-task-id", "01", "-config", configPath); err == nil || !strings.Contains(string(output), "contract_mismatch") {
		t.Fatalf("noncanonical task ID was accepted: err=%v output=%s", err, output)
	}
	reader, err := steerline.OpenNativeTaskReader(context.Background(), &config)
	if err != nil {
		t.Fatalf("native authority did not survive CLI process: %v", err)
	}
	defer reader.Close()
	after, err := (steerline.NewQueryService(reader, config.Identity())).ListNativeTasks(context.Background(), "", 100)
	if err != nil || after.DatabaseID != before.DatabaseID || after.DatabaseRevision != before.DatabaseRevision {
		t.Fatalf("CLI reads changed or lost persisted task state: before=%+v after=%+v err=%v", before, after, err)
	}
}

func writeConfig(t *testing.T, path string, value any) {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
}

func runCLI(t *testing.T, home string, args ...string) ([]byte, error) {
	t.Helper()
	command := exec.Command(cliPath, args...)
	command.Env = withSteerlineHome(os.Environ(), home)
	return command.CombinedOutput()
}

func withSteerlineHome(environment []string, home string) []string {
	const key = "STEERLINE_HOME="
	result := make([]string, 0, len(environment)+1)
	for _, entry := range environment {
		if !strings.HasPrefix(strings.ToUpper(entry), key) {
			result = append(result, entry)
		}
	}
	return append(result, key+home)
}

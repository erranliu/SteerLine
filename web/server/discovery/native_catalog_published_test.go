//go:build runnercontract

package discovery

import (
	"context"
	"encoding/json"
	cfg "github.com/erranliu/SteerLine/internal/executionconfig"
	"github.com/erranliu/SteerLine/internal/localservice"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	repository "github.com/erranliu/SteerLine/web/server/executionconfig"
	"github.com/erranliu/SteerLine/web/server/nativequery"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNativeCatalogPublishedRunner(t *testing.T) {
	exe := os.Getenv("STEERRUNNER_EXE")
	if !filepath.IsAbs(exe) {
		t.Fatal("STEERRUNNER_EXE must explicitly select the published EXE")
	}
	home, data := t.TempDir(), t.TempDir()
	t.Setenv("ORCA_USER_DATA_PATH", data)
	if err := os.WriteFile(filepath.Join(data, "orca-environments.json"), []byte(`{"version":1,"environments":[{"id":"one","name":"Published contract fixture"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	raw, err := nativequery.Run(ctx, home, exe, []string{"version", "--json"})
	if err != nil {
		t.Fatal(err)
	}
	var version struct {
		Data struct {
			Version string `json:"version"`
		} `json:"data"`
	}
	if err = json.Unmarshal(raw, &version); err != nil || version.Data.Version == "" {
		t.Fatal(string(raw), err)
	}
	body, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	config := cfg.File[cfg.Pin]{Version: 3, Runner: cfg.Pin{Executable: exe, Version: version.Data.Version, SHA256: cfg.Digest(body)}, DefaultProfile: "local", Profiles: []cfg.Profile{{ID: "local", Name: "Local", Execution: runnercontract.ExecutionInput{Adapter: "codex-app-server", Target: runnercontract.ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{"access_mode":"full_access","model":"contract-model"}`)}, Workflow: runnercontract.Workflow{ID: "development", Version: 2}}}}
	if err = localservice.WritePrivateFile(home, "execution.json", config); err != nil {
		t.Fatal(err)
	}
	manager := New(repository.New(home))
	defer manager.Close()
	result, err := manager.Query(ctx, Request{Kind: "environments", Host: "orca_environment"})
	if err != nil || result.State != "ready" || len(result.Environments) != 1 || result.Environments[0].ID != "one" {
		t.Fatal(result, err)
	}
	for _, name := range []string{"state", "application-api", "logs"} {
		if _, err = os.Stat(filepath.Join(home, name)); !os.IsNotExist(err) {
			t.Fatal("metadata query initialized runtime", name, err)
		}
	}
	t.Logf("fixed Runner %s SHA256=%s consumed by Web metadata entry without a project or service", version.Data.Version, config.Runner.SHA256)
}

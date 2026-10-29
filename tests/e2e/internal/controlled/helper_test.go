package controlled

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstatepath"
)

func TestDirectControlledHelperAdvertisesGeneration(t *testing.T) {
	capture, err := os.CreateTemp(t.TempDir(), "capabilities-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer capture.Close()
	previous := os.Stdout
	os.Stdout = capture
	handled, callErr := RunnerCLI([]string{"capabilities", "--offline", "--json"})
	os.Stdout = previous
	if !handled || callErr != nil {
		t.Fatal("direct helper capabilities unavailable", handled, callErr)
	}
	if _, err = capture.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(capture)
	if err != nil {
		t.Fatal(err)
	}
	var envelope map[string]json.RawMessage
	if err = json.Unmarshal(raw, &envelope); err != nil || len(envelope) != 5 {
		t.Fatal("public CLI requires its complete five-field envelope", string(raw), err)
	}
	var caps runnerclient.Capabilities
	if err = json.Unmarshal(envelope["data"], &caps); err != nil {
		t.Fatal(err)
	}
	catalog, err := runnerclient.CompileCatalog(caps)
	if err != nil {
		t.Fatal(err)
	}
	if err = catalog.ValidateWorkflow(runnercontract.Workflow{ID: "development", Version: 1}); err != nil {
		t.Fatal("controlled delivery capability lost", err)
	}
	input := runnercontract.GenerateRunInput{Workflow: runnercontract.Workflow{ID: "generate", Version: 1}, Execution: runnercontract.ExecutionInput{Adapter: "codex-app-server", Target: runnercontract.ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{"access_mode":"read_only"}`)}}
	if err = catalog.ValidateGenerate(input); err != nil {
		t.Fatal("automatic fixture materials cannot use the advertised generation contract", err)
	}
}

func TestControlledProviderKeepsOriginalPRWhenAnotherTaskPublishes(t *testing.T) {
	first := executionmodel.PRInfo{URL: "https://github.com/e2e/project/pull/1", HeadBranch: "first", State: "open"}
	second := executionmodel.PRInfo{URL: "https://github.com/e2e/project/pull/2", HeadBranch: "second", State: "open"}
	s := State{PR: second, PRs: map[string]executionmodel.PRInfo{first.URL: first, second.URL: second}}
	for _, expected := range []executionmodel.PRInfo{first, second} {
		got, err := ProviderPR(s, expected.URL)
		if err != nil || got.URL != expected.URL || got.HeadBranch != expected.HeadBranch {
			t.Fatal("provider retargeted a fixed PR", got, err)
		}
	}
	if _, err := ProviderPR(s, "https://github.com/e2e/project/pull/3"); err == nil {
		t.Fatal("unknown PR URL reused the latest result")
	}
}

func TestSharedHostRoutesRunnerWorkspaceToOriginalProvider(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "seed"), 0700); err != nil {
		t.Fatal(err)
	}
	primary := filepath.Join(root, "provider", "state.json")
	secondary := filepath.Join(root, "provider-b", "state.json")
	first := filepath.Join(root, "home", "state", "first")
	second := filepath.Join(root, "home", "state", "second")
	if err := Write(filepath.Join(root, "manifest.json"), map[string]bool{"fixture": true}); err != nil {
		t.Fatal(err)
	}
	for _, pair := range []struct{ name, prefix, provider string }{{"SteerlineTest.json", first, primary}, {"SteerlineTestB.json", second, secondary}} {
		if err := Write(filepath.Join(root, "seed", "fixture-routes-"+pair.name), map[string]string{pair.prefix: pair.provider, runnerstatepath.Directory(pair.prefix): pair.provider}); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("STEERLINE_E2E_CONTROLLED_STATE", primary)
	for _, pair := range []struct{ dir, want string }{{filepath.Join(runnerstatepath.Directory(first), "w", "original"), primary}, {filepath.Join(runnerstatepath.Directory(second), "w", "original"), secondary}, {second + "-sibling", primary}} {
		if got := statePathForDirectory(pair.dir); got != pair.want {
			t.Fatalf("provider crossed caller path: %s => %s", pair.dir, got)
		}
	}
}

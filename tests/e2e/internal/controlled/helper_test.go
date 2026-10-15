package controlled

import (
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"os"
	"path/filepath"
	"testing"
)

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
		if err := Write(filepath.Join(root, "seed", "fixture-routes-"+pair.name), map[string]string{pair.prefix: pair.provider}); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("STEERLINE_E2E_CONTROLLED_STATE", primary)
	for _, pair := range []struct{ dir, want string }{{filepath.Join(first, "runner", "w", "original"), primary}, {filepath.Join(second, "runner", "w", "original"), secondary}, {second + "-sibling", primary}} {
		if got := statePathForDirectory(pair.dir); got != pair.want {
			t.Fatalf("provider crossed caller path: %s => %s", pair.dir, got)
		}
	}
}

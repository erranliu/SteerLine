package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	testfeedback "github.com/erranliu/SteerLine/internal/testkit/feedback"
)

func TestE2EGoDeadlineIncludesHeavyScenarioAndIndependentCleanup(t *testing.T) {
	ordinary := e2eScenario{ID: "recovery.waiting-merge-restart", Env: "controlled"}
	isolation := e2eScenario{ID: "runtime.source-build-isolation", Env: "controlled"}
	if e2ePackageGoTimeout([]e2eScenario{ordinary}) != "12m" || e2ePackageGoTimeout([]e2eScenario{isolation}) != "6m" || e2ePackageGoTimeout([]e2eScenario{ordinary, isolation}) != "18m" {
		t.Fatal("Go parent deadline clips a selected business scene or its cleanup")
	}

	if e2ePackageGoTimeout([]e2eScenario{{Env: "live"}}) != "35m" {
		t.Fatal("live package deadline changed")
	}
}

func TestE2EParallelismOnlyAllowsIndependentRuntimeScenes(t *testing.T) {
	root, err := e2eRoot()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := selectE2E(root, options{level: "L3", suite: "runtime", e2eEnv: "controlled"})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 4 || e2ePackageParallelism(entries) != 2 {
		t.Fatal("runtime isolation scenes are not selected with bounded parallelism")
	}
	for _, entry := range entries {
		if e2ePackageParallelism([]e2eScenario{entry}) != 1 {
			t.Fatal("single scene needs no parallel slot")
		}
	}
	for _, extra := range []e2eScenario{
		{ID: "runtime.unreviewed", Env: "controlled", Package: "./tests/e2e/suites/runtime"},
		{ID: entries[0].ID, Env: "native", Package: entries[0].Package},
		{ID: entries[0].ID, Env: "live", Package: entries[0].Package},
		{ID: entries[0].ID, Env: "controlled", Package: "./tests/e2e/suites/recovery"},
	} {
		mixed := append(append([]e2eScenario{}, entries...), extra)
		if e2ePackageParallelism(mixed) != 1 {
			t.Fatalf("unsafe parallel selection: %+v", extra)
		}
	}
	if _, err := selectE2E(root, options{level: "L3", suite: "runtime", e2eEnv: "controlled", scenario: "runtime.installed-debug-isolation"}); err == nil {
		t.Fatal("retired comprehensive scene remains selectable")
	}
}

func TestLevelSelectionsRejectAmbiguity(t *testing.T) {
	for _, args := range [][]string{{"-level=L4"}, {"-env=controlled"}, {"-level=L1", "-quick"}, {"-level=L1", "-suite=recovery"}, {"-level=L2", "-suite=smoke"}, {"-level=L3"}, {"-level=L2", "-workers=1"}, {"-level=L2", "-build-only=false"}, {"-level=L2", "-env=native"}, {"-level=L2", "--", "./..."}, {"-level=L2", "-level=L3"}} {
		if _, err := parseOptions(args); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	opts, err := parseOptions([]string{"-level=L1", "--", "./internal/testkit/feedback", "-count=1"})
	if err != nil || !opts.quick || opts.workers != 3 {
		t.Fatalf("L1 alias: %+v %v", opts, err)
	}
	for _, level := range []string{"L2", "L3"} {
		args := []string{"-level=" + level}
		if level == "L3" {
			args = append(args, "-suite=recovery", "-env=controlled")
		}
		opts, err := parseOptions(args)
		expected := "gated"
		if level == "L3" {
			expected = "controlled"
		}
		if err != nil || opts.e2eEnv != expected {
			t.Fatalf("%v: %v", args, err)
		}
	}
}

func TestL2GatedSelectionUsesCurrentRegisteredChain(t *testing.T) {
	root, err := e2eRoot()
	if err != nil {
		t.Fatal(err)
	}
	selected, err := selectE2E(root, options{level: "L2", e2eEnv: "gated"})
	if err != nil || len(selected) != 2 || selected[0].ID != "smoke.delivery" || selected[1].ID != "smoke.local-codex" {
		t.Fatalf("fixed current L2 selection: %+v %v", selected, err)
	}
}

func TestManifestRejectsMissingDuplicateAndUnknownSelection(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "tests", "e2e"), 0700); err != nil {
		t.Fatal(err)
	}
	entry := e2eScenario{ID: "recovery.one", Level: "L3", Suite: "recovery", Package: "./tests/e2e/suites/recovery", Test: "TestOne", Env: "controlled"}
	save := func(entries []e2eScenario) {
		raw, _ := json.Marshal(entries)
		if err := os.WriteFile(filepath.Join(root, "tests", "e2e", "suites.json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	save([]e2eScenario{entry})
	opts := options{level: "L3", suite: "recovery", e2eEnv: "controlled"}
	if entries, err := selectE2E(root, opts); err != nil || len(entries) != 1 {
		t.Fatal(err)
	}
	for _, suite := range []string{"unknown", "recovery,unknown", "recovery,recovery", "recovery,"} {
		opts.suite = suite
		if _, err := selectE2E(root, opts); err == nil {
			t.Fatalf("accepted %s", suite)
		}
	}
	opts.suite = "recovery"
	save([]e2eScenario{entry, entry})
	if _, err := selectE2E(root, opts); err == nil {
		t.Fatal("duplicate accepted")
	}
	other := entry
	other.ID = "recovery.live"
	other.Test = "TestLive"
	other.Env = "live"
	other.Chain = "local-codex"
	save([]e2eScenario{entry, other})
	if _, err := selectE2E(root, opts); err == nil {
		t.Fatal("package mixing live into controlled gate accepted")
	}
	save(nil)
	if _, err := selectE2E(root, opts); err == nil {
		t.Fatal("empty selection accepted")
	}
}

func TestE2EResultRequiresCompleteLogsAndCleanupEvidence(t *testing.T) {
	entry := e2eScenario{ID: "smoke.one", Test: "TestOne"}
	out := t.TempDir()
	dir := filepath.Join(out, "scenarios", entry.ID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	good := `{"Action":"start","Package":"p"}` + "\n" + `{"Action":"run","Package":"p","Test":"TestOne"}` + "\n" + `{"Action":"pass","Package":"p","Test":"TestOne"}` + "\n" + `{"Action":"pass","Package":"p"}` + "\n"
	good = strings.ReplaceAll(good, `"Action":`, `"Time":"2026-10-01T00:00:00Z","Elapsed":0,"Action":`)
	group := func(raw string) scopedGroup {
		return scopedGroup{ID: "e2e-1", Packages: []string{"p"}, Summary: testfeedback.Parse([]byte(raw), 0)}
	}
	if checkE2EGroup(group(good), []e2eScenario{entry}, out) == nil {
		t.Fatal("missing evidence passed")
	}
	if err := writeJSON(filepath.Join(dir, "scenario.json"), map[string]any{"id": entry.ID, "passed": true, "cleanup_complete": true, "resources": []any{map[string]string{"kind": "project"}}}); err != nil {
		t.Fatal(err)
	}
	if err := checkE2EGroup(group(good), []e2eScenario{entry}, out); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"", strings.ReplaceAll(good, "pass", "skip"), strings.ReplaceAll(good, "pass", "fail"), strings.TrimSuffix(good, "\n"), strings.ReplaceAll(good, "TestOne", "TestOther"), good + "{", good + good} {
		if checkE2EGroup(group(raw), []e2eScenario{entry}, out) == nil {
			t.Fatalf("invalid log passed: %s", raw)
		}
	}
	if err := writeJSON(filepath.Join(dir, "scenario.json"), map[string]any{"id": entry.ID, "passed": true, "cleanup_complete": false}); err != nil {
		t.Fatal(err)
	}
	if checkE2EGroup(group(good), []e2eScenario{entry}, out) == nil {
		t.Fatal("cleanup failure passed")
	}
	if err := writeJSON(filepath.Join(dir, "scenario.json", "impossible.json"), map[string]bool{"passed": true}); err == nil {
		t.Fatal("report write failure was ignored")
	}
}

func TestL2FailureStopsBeforeNativeEnvironmentCallback(t *testing.T) {
	names := []string{"controlled", "live"}
	groups := map[string][]e2eScenario{"controlled": {{Env: "controlled", ID: "gate"}}, "live": {{Env: "live", ID: "native"}}}
	for _, failure := range []error{errors.New("test failed"), errors.New("cleanup failed"), errors.New("usage/evidence missing")} {
		nativeCalls := 0
		errs, blocked := executeE2EPackages(context.Background(), names, groups, true, func(_ int, pkg string) error {
			if pkg == "controlled" {
				return failure
			}
			nativeCalls++
			return nil
		})
		if len(errs) != 1 || len(blocked) != 1 || nativeCalls != 0 {
			t.Fatalf("gate leaked: %v %v %d", errs, blocked, nativeCalls)
		}
	}
	calls := 0
	errs, blocked := executeE2EPackages(context.Background(), names, groups, true, func(_ int, _ string) error { calls++; return nil })
	if len(errs) != 0 || len(blocked) != 0 || calls != 2 {
		t.Fatal("successful gate did not run live stage")
	}
}
func TestRetiredExecutionScenesCannotBeSelected(t *testing.T) {
	root, err := e2eRoot()
	if err != nil {
		t.Fatal(err)
	}
	for _, opts := range []options{
		{level: "L3", suite: "delivery", e2eEnv: "controlled", scenario: "delivery.basic"},
		{level: "L3", suite: "delivery", e2eEnv: "live", scenario: "delivery.local-codex"},
		{level: "L3", suite: "recovery", e2eEnv: "controlled", scenario: "recovery.codex-resume"},
		{level: "L3", suite: "recovery", e2eEnv: "controlled", scenario: "recovery.terminal-environment-unknown-send"},
		{level: "L3", suite: "workspace", e2eEnv: "native"},
	} {
		if _, err := selectE2E(root, opts); err == nil {
			t.Fatalf("retired selection accepted: %+v", opts)
		}
	}
}

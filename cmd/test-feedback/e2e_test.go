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
	ordinary := e2eScenario{ID: "delivery.basic", Env: "controlled"}
	heavy := e2eScenario{ID: "runtime.installed-debug-isolation", Env: "controlled"}
	native := e2eScenario{ID: "workspace.orca-removal-restart", Env: "native"}
	if e2ePackageGoTimeout([]e2eScenario{ordinary}) != "6m" || e2ePackageGoTimeout([]e2eScenario{heavy}) != "11m" || e2ePackageGoTimeout([]e2eScenario{ordinary, heavy}) != "17m" {
		t.Fatal("Go parent deadline clips a selected business scene or its cleanup")
	}
	if e2ePackageGoTimeout([]e2eScenario{native}) != "11m" {
		t.Fatal("Go parent deadline clips native removal/restart and its second workflow")
	}
	if e2ePackageGoTimeout([]e2eScenario{{Env: "live"}}) != "35m" {
		t.Fatal("live package deadline changed")
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
func TestL3ExactScenarioSelectionRejectsWrongEnvironment(t *testing.T) {
	root, err := e2eRoot()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"delivery.local-codex"} {
		entries, err := selectE2E(root, options{level: "L3", suite: "delivery", e2eEnv: "live", scenario: id})
		if err != nil || len(entries) != 1 || entries[0].ID != id {
			t.Fatalf("independent selection %s: %v %v", id, entries, err)
		}
	}
	entries, err := selectE2E(root, options{level: "L3", suite: "runtime", e2eEnv: "live", scenario: "runtime.sandbox-git.local-codex"})
	if err != nil || len(entries) != 1 || entries[0].Chain != "local-codex" || entries[0].Package != "./tests/e2e/suites/sandbox" {
		t.Fatalf("sandbox live selection: %+v %v", entries, err)
	}
	for _, opts := range []options{
		{level: "L3", suite: "runtime", e2eEnv: "controlled", scenario: "runtime.sandbox-git.local-codex"},
		{level: "L3", suite: "runtime", e2eEnv: "native", scenario: "runtime.sandbox-git.local-codex"},
		{level: "L3", suite: "delivery", e2eEnv: "live", scenario: "delivery.orca-remote-codex"},
		{level: "L3", suite: "delivery", e2eEnv: "controlled", scenario: "delivery.local-codex"},
		{level: "L3", suite: "recovery", e2eEnv: "live", scenario: "delivery.local-codex"},
		{level: "L3", suite: "delivery", e2eEnv: "live", scenario: "delivery.local-codex,delivery.local-codex"},
	} {
		if _, err := selectE2E(root, opts); err == nil {
			t.Fatal("invalid scenario selection accepted")
		}
	}
}

func TestL3NativeWorkspaceIsExplicitAndSeparateFromL2(t *testing.T) {
	root, err := e2eRoot()
	if err != nil {
		t.Fatal(err)
	}
	opts, err := parseOptions([]string{"-level=L3", "-suite=workspace", "-env=native", "-scenario=workspace.orca-removal-restart"})
	if err != nil {
		t.Fatal(err)
	}
	selected, err := selectE2E(root, opts)
	if err != nil || len(selected) != 1 || selected[0].Env != "native" || selected[0].Chain != "" {
		t.Fatalf("native selection: %+v %v", selected, err)
	}
	for _, environment := range []string{"controlled", "live"} {
		opts.e2eEnv = environment
		if _, err := selectE2E(root, opts); err == nil {
			t.Fatal("native workspace scenario accepted under wrong environment")
		}
	}
	if _, err := parseOptions([]string{"-level=L2", "-env=native"}); err == nil {
		t.Fatal("native environment replaced the L2 model gate")
	}
}

func TestNativeWorkspaceRequiresItsOwnEvidenceWithoutModelUsage(t *testing.T) {
	entry := e2eScenario{ID: "workspace.one", Test: "TestOne", Env: "native", Level: "L3"}
	out := t.TempDir()
	dir := filepath.Join(out, "scenarios", entry.ID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	log := "{\"Time\":\"2026-10-09T00:00:00Z\",\"Action\":\"start\",\"Package\":\"p\"}\n" +
		"{\"Time\":\"2026-10-09T00:00:00Z\",\"Action\":\"run\",\"Package\":\"p\",\"Test\":\"TestOne\"}\n" +
		"{\"Time\":\"2026-10-09T00:00:01Z\",\"Action\":\"pass\",\"Package\":\"p\",\"Test\":\"TestOne\"}\n" +
		"{\"Time\":\"2026-10-09T00:00:01Z\",\"Action\":\"pass\",\"Package\":\"p\"}\n"
	log = strings.ReplaceAll(log, `"Action":`, `"Elapsed":0,"Action":`)
	g := scopedGroup{ID: "native", Packages: []string{"p"}, Summary: testfeedback.Parse([]byte(log), 0)}
	for _, tc := range []struct{ verified, usage, pass bool }{{false, false, false}, {true, true, false}, {true, false, true}} {
		evidence := map[string]any{"id": entry.ID, "passed": true, "cleanup_complete": true, "resources": []any{map[string]string{"kind": "project"}}, "native_workspace_verified": tc.verified, "usage_verified": tc.usage}
		if err := writeJSON(filepath.Join(dir, "scenario.json"), evidence); err != nil {
			t.Fatal(err)
		}
		if err := checkE2EGroup(g, []e2eScenario{entry}, out); (err == nil) != tc.pass {
			t.Fatalf("evidence=%+v result=%v", tc, err)
		}
	}
}

func TestRemoteResultRequiresPublicOutputAndHonestUsageStatus(t *testing.T) {
	entry := e2eScenario{ID: "delivery.orca-remote-codex", Test: "TestRemote", Env: "live", Level: "L3", Chain: "orca-remote-codex"}
	out := t.TempDir()
	dir := filepath.Join(out, "scenarios", entry.ID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	log := ""
	for _, event := range []struct{ action, test string }{{"start", ""}, {"run", entry.Test}, {"pass", entry.Test}, {"pass", ""}} {
		raw, err := json.Marshal(map[string]any{"Time": "2026-10-09T00:00:00Z", "Elapsed": 0, "Action": event.action, "Package": "p", "Test": event.test})
		if err != nil {
			t.Fatal(err)
		}
		log += string(raw) + "\n"
	}
	g := scopedGroup{ID: "remote", Packages: []string{"p"}, Summary: testfeedback.Parse([]byte(log), 0)}
	for _, tc := range []struct {
		name           string
		public, usage  bool
		status         string
		mergedPR, pass bool
	}{
		{"missing public output", false, false, "not_exposed_by_orca", true, false},
		{"missing usage status", true, false, "", true, false},
		{"claimed token verification", true, true, "not_exposed_by_orca", true, false},
		{"missing merge", true, false, "not_exposed_by_orca", false, false},
		{"complete public evidence", true, false, "not_exposed_by_orca", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			evidence := map[string]any{"id": entry.ID, "passed": true, "cleanup_complete": true, "resources": []any{map[string]string{"kind": "project"}}, "remote_public_output_verified": tc.public, "usage_verified": tc.usage, "remote_usage_status": tc.status}
			if tc.mergedPR {
				evidence["real_pr"] = map[string]any{"url": "https://github.com/erranliu/SteerlineTest/pull/168", "state": "merged", "repository": "erranliu/SteerlineTest", "merged": true}
			}
			if err := writeJSON(filepath.Join(dir, "scenario.json"), evidence); err != nil {
				t.Fatal(err)
			}
			if err := checkE2EGroup(g, []e2eScenario{entry}, out); (err == nil) != tc.pass {
				t.Fatalf("unexpected evidence gate result: %v", err)
			}
		})
	}
}

func TestNativeOrcaL3RequiresHonestPublicUsageCapability(t *testing.T) {
	for _, tc := range []struct {
		name, level, suite, chain, status string
		usage, completed, cleanup, pass   bool
	}{
		{"delivery", "L3", "delivery", "orca-terminal-local", "not_exposed_by_orca", false, true, true, true},
		{"recovery", "L3", "recovery", "orca-terminal-local", "not_exposed_by_orca", false, true, true, true},
		{"missing capability", "L3", "delivery", "orca-terminal-local", "", false, true, true, false},
		{"wrong capability", "L3", "delivery", "orca-terminal-local", "not_applicable", false, true, true, false},
		{"claimed tokens", "L3", "delivery", "orca-terminal-local", "not_exposed_by_orca", true, true, true, false},
		{"failed business", "L3", "delivery", "orca-terminal-local", "not_exposed_by_orca", false, false, true, false},
		{"failed cleanup", "L3", "delivery", "orca-terminal-local", "not_exposed_by_orca", false, true, false, false},
		{"local still requires tokens", "L3", "delivery", "local-codex", "not_exposed_by_orca", false, true, true, false},
		{"local tokens", "L3", "delivery", "local-codex", "not_applicable", true, true, true, true},
		{"L2 unchanged", "L2", "delivery", "orca-terminal-local", "not_exposed_by_orca", false, true, true, false},
		{"other suite unchanged", "L3", "candidates", "orca-terminal-local", "not_exposed_by_orca", false, true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entry := e2eScenario{ID: "delivery.one", Test: "TestOne", Env: "live", Level: tc.level, Suite: tc.suite, Chain: tc.chain}
			out := t.TempDir()
			dir := filepath.Join(out, "scenarios", entry.ID)
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			var log strings.Builder
			for _, event := range []struct{ action, test string }{{"start", ""}, {"run", entry.Test}, {"pass", entry.Test}, {"pass", ""}} {
				raw, err := json.Marshal(map[string]any{"Time": "2026-10-09T00:00:00Z", "Elapsed": 0, "Action": event.action, "Package": "p", "Test": event.test})
				if err != nil {
					t.Fatal(err)
				}
				log.Write(raw)
				log.WriteByte('\n')
			}
			group := scopedGroup{Packages: []string{"p"}, Summary: testfeedback.Parse([]byte(log.String()), 0)}
			evidence := map[string]any{"id": entry.ID, "passed": tc.completed, "cleanup_complete": tc.cleanup, "resources": []any{map[string]string{"kind": "project"}}, "usage_verified": tc.usage, "orca_usage_status": tc.status}
			if err := writeJSON(filepath.Join(dir, "scenario.json"), evidence); err != nil {
				t.Fatal(err)
			}
			if err := checkE2EGroup(group, []e2eScenario{entry}, out); (err == nil) != tc.pass {
				t.Fatalf("unexpected acceptance result: %v", err)
			}
		})
	}
}

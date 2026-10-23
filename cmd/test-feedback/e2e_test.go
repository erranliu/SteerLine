package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	entries, err := selectE2E(root, options{level: "L4", suite: "runtime", e2eEnv: "controlled"})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 5 || e2ePackageParallelism(entries) != 1 {
		t.Fatal("runtime maintenance must serialize the package")
	}
	entries = entries[:4]
	if e2ePackageParallelism(entries) != 2 {
		t.Fatal("independent boundaries lost bounded parallelism")
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
		{ID: "runtime.service-lifecycle", Level: "L3", Env: "controlled", Package: entries[0].Package},
	} {
		mixed := append(append([]e2eScenario{}, entries...), extra)
		if e2ePackageParallelism(mixed) != 1 {
			t.Fatalf("unsafe parallel selection: %+v", extra)
		}
	}
	flows, err := selectE2E(root, options{level: "L3", suite: "runtime", e2eEnv: "controlled"})
	if err != nil || len(flows) != 3 || e2ePackageParallelism(flows) != 2 {
		t.Fatalf("normal flows lost bounded parallelism: %v %v", flows, err)
	}
	if _, err := selectE2E(root, options{level: "L3", suite: "runtime", e2eEnv: "controlled", scenario: "runtime.installed-debug-isolation"}); err == nil {
		t.Fatal("retired comprehensive scene remains selectable")
	}
}

func TestLevelSelectionsRejectAmbiguity(t *testing.T) {
	for _, args := range [][]string{{"-level=L4"}, {"-level=L5"}, {"-level=L4", "-suite=runtime", "-env=live"}, {"-level=L4", "-suite=runtime", "-env=gated"}, {"-level=L4", "-suite=runtime", "-env=controlled", "--", "./..."}, {"-level=L3", "-suite=runtime", "-env=live"}, {"-env=controlled"}, {"-level=L1", "-quick"}, {"-level=L1", "-suite=recovery"}, {"-level=L2", "-suite=smoke"}, {"-level=L3"}, {"-level=L2", "-workers=1"}, {"-level=L2", "-build-only=false"}, {"-level=L2", "-env=native"}, {"-level=L2", "--", "./..."}, {"-level=L2", "-level=L3"}} {
		if _, err := parseOptions(args); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	opts, err := parseOptions([]string{"-level=L1", "--", "./internal/testkit/feedback", "-count=1"})
	if err != nil || !opts.quick || opts.workers != 3 {
		t.Fatalf("L1 alias: %+v %v", opts, err)
	}
	for _, level := range []string{"L2", "L3", "L4"} {
		args := []string{"-level=" + level}
		if level != "L2" {
			args = append(args, "-suite=runtime", "-env=controlled")
		}
		opts, err := parseOptions(args)
		if err != nil || opts.e2eEnv != "controlled" {
			t.Fatalf("%v: %v", args, err)
		}
	}
}

func TestL2OnlySelectsControlledSmokeAndL4OwnsRealChains(t *testing.T) {
	root, err := e2eRoot()
	if err != nil {
		t.Fatal(err)
	}
	opts, err := parseOptions([]string{"-level=L2"})
	if err != nil {
		t.Fatal(err)
	}
	selected, err := selectE2E(root, opts)
	if err != nil || len(selected) != 1 || selected[0].ID != "smoke.delivery" || selected[0].Env != "controlled" {
		t.Fatalf("fixed controlled smoke: %+v %v", selected, err)
	}
	for _, tc := range []struct{ suite, id string }{
		{"smoke", "smoke.local-codex"}, {"workflow", "workflow.local-codex-audited"}, {"testing", "testing.management-local-codex"},
	} {
		opts, err = parseOptions([]string{"-level=L4", "-suite=" + tc.suite, "-env=live", "-scenario=" + tc.id})
		if err != nil {
			t.Fatal(err)
		}
		selected, err = selectE2E(root, opts)
		if err != nil || len(selected) != 1 || selected[0].ID != tc.id {
			t.Fatalf("real chain not selectable: %v %v", selected, err)
		}
	}
	for _, args := range [][]string{
		{"-level=L2", "-env=live"}, {"-level=L2", "-env=gated"},
		{"-level=L3", "-suite=workflow", "-env=live", "-scenario=workflow.local-codex-audited"},
		{"-level=L4", "-suite=testing", "-env=live"},
		{"-level=L4", "-suite=go", "-scenario=unknown"}, {"-level=L4", "-suite=go", "-env=controlled"},
	} {
		if _, err := parseOptions(args); err == nil {
			t.Fatalf("unsafe selection accepted: %v", args)
		}
	}
}

func TestExclusiveScenariosOnlySelectAtL4(t *testing.T) {
	root, err := e2eRoot()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ suite, id string }{
		{"runtime", "runtime.project-maintenance"}, {"testing", "testing.records"},
	} {
		opts := options{level: "L3", suite: tc.suite, scenario: tc.id, e2eEnv: "controlled"}
		if _, err := selectE2E(root, opts); err == nil {
			t.Fatal("exclusive builder accepted as L3", tc.id)
		}
		opts.level = "L4"
		selected, err := selectE2E(root, opts)
		if err != nil || len(selected) != 1 || e2ePackageParallelism(selected) != 1 {
			t.Fatalf("exclusive L4 selection: %v %v", selected, err)
		}
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

func TestL3AndL4SelectDisjointRegisteredContent(t *testing.T) {
	root, err := e2eRoot()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"L3": {"recovery.waiting-merge-restart", "runtime.service-lifecycle", "runtime.dual-home-operation", "runtime.shared-runner-projects"},
		"L4": {"runtime.source-build-isolation", "runtime.path-isolation", "runtime.service-write-isolation", "runtime.dual-home-isolation", "runtime.project-maintenance"},
	}
	for level, ids := range want {
		suite := "runtime"
		if level == "L3" {
			suite = "recovery,runtime"
		}
		opts := options{level: level, suite: suite, e2eEnv: "controlled"}
		entries, err := selectE2E(root, opts)
		if err != nil || len(entries) != len(ids) {
			t.Fatalf("%s selection: %v %v", level, entries, err)
		}
		for i, entry := range entries {
			if entry.ID != ids[i] || entry.Level != level {
				t.Fatalf("wrong %s scenario: %+v", level, entry)
			}
		}
		opts.scenario = strings.Join(ids[len(ids)-2:], ",")
		if selected, err := selectE2E(root, opts); err != nil || len(selected) != 2 {
			t.Fatalf("multi-select: %v %v", selected, err)
		}
		for _, invalid := range []string{"unknown", ids[0] + "," + ids[0], ids[0] + ",", "smoke.delivery"} {
			opts.scenario = invalid
			if _, err := selectE2E(root, opts); err == nil {
				t.Fatalf("accepted invalid %s selection: %s", level, invalid)
			}
		}
		other := "L4"
		if level == "L4" {
			other = "L3"
		}
		opts.scenario = want[other][len(want[other])-1]
		if _, err := selectE2E(root, opts); err == nil {
			t.Fatal("cross-level scenario accepted")
		}
	}
}

func TestIndependentPackagesOverlapAndPreserveFailuresAndCancellation(t *testing.T) {
	names := []string{"recovery", "runtime"}
	groups := map[string][]e2eScenario{"recovery": {{ID: "recovery"}}, "runtime": {{ID: "runtime"}}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	entered := make(chan struct{}, 2)
	errs, blocked := executeE2EPackages(ctx, names, groups, 2, func(i int, _ string) error {
		entered <- struct{}{}
		if i == 0 {
			select {
			case <-entered:
			case <-ctx.Done():
				return ctx.Err()
			}
			select {
			case <-entered:
			case <-ctx.Done():
				return ctx.Err()
			}
			return errors.New("first package evidence failed")
		}
		return errors.New("second package cleanup failed")
	})
	if ctx.Err() != nil || len(errs) != 2 || errs[0] != "first package evidence failed" || errs[1] != "second package cleanup failed" || len(blocked) != 0 {
		t.Fatalf("parallel packages lost overlap/failures: %v %v", errs, blocked)
	}
	cancel()
	calls := 0
	errs, blocked = executeE2EPackages(ctx, names, groups, 2, func(int, string) error { calls++; return nil })
	if calls != 0 || len(errs) == 0 || len(blocked) != 2 {
		t.Fatalf("cancelled packages executed or lost blockers: %v %v %d", errs, blocked, calls)
	}
}

func TestScenarioContentListMatchesRegisteredIDsAndLevels(t *testing.T) {
	root, err := e2eRoot()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "SteerlineFlow", "testing", "test-list.json"))
	if err != nil {
		t.Fatal(err)
	}
	var catalogue struct{ Tests []struct{ ID, Level string } }
	if err = json.Unmarshal(raw, &catalogue); err != nil {
		t.Fatal(err)
	}
	levels := map[string]string{}
	seen := map[string]int{}
	for _, entry := range catalogue.Tests {
		levels[entry.ID] = entry.Level
	}
	for _, level := range []string{"L1", "L2", "L3", "L4"} {
		doc, err := os.ReadFile(filepath.Join(root, "SteerlineFlow", "testing", strings.ToLower(level)+".md"))
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(doc), "\n") {
			if !strings.HasPrefix(line, "| `") {
				continue
			}
			id := strings.Trim(strings.Split(line, "|")[1], " `")
			// L4 also lists unregistered standalone script paths.
			if strings.HasPrefix(id, "scripts/") {
				continue
			}
			if levels[id] != level {
				t.Fatalf("documented ID %s under %s differs from catalogue %s", id, level, levels[id])
			}
			seen[id]++
		}
	}
	for id := range levels {
		if seen[id] != 1 {
			t.Fatalf("test %s documented %d times", id, seen[id])
		}
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

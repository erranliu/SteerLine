package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestExecutionParametersPreserveActualArgvAndCommandKinds(t *testing.T) {
	t.Setenv("GOFLAGS", "")
	for _, args := range [][]string{
		{"go", "test", "-p=1", "-parallel=2", "-count=3", "-timeout=25m", "-tags=owner", "-run=^TestA$", "-skip=TestB", "./pkg"},
		{"go", "test", "./pkg", "-p", "1", "-parallel", "2", "-count", "3", "-timeout", "25m", "-tags", "owner", "-run", "^TestA$", "-skip", "TestB"},
	} {
		before := append([]string(nil), args...)
		p := executionParametersFor(args, nil)
		if p.Kind != "go_test" || p.Status != "not_executed" {
			t.Fatal(p)
		}
		for name, pair := range map[string]struct {
			got  executionParameter
			want string
		}{"package": {p.PackageParallel, "1"}, "test": {p.TestParallel, "2"}, "count": {p.Count, "3"}, "timeout": {p.Timeout, "25m"}, "tags": {p.Tags, "owner"}, "filter": {p.Filter, "^TestA$"}, "skip": {p.Skip, "TestB"}} {
			if pair.got.State != "explicit" || pair.got.Value != pair.want {
				t.Fatal(name, pair)
			}
		}
		if !reflect.DeepEqual(args, before) {
			t.Fatal("report generation changed argv")
		}
	}
	build := []string{"go", "test", "-tags=e2e", "-c", "-o", "scenario.test.exe", "./pkg"}
	for _, command := range [][]string{
		{"scenario.test.exe", "-test.count=1", "-test.parallel=2", "-test.timeout=10m", "-test.run=^TestA$"},
		{"go", "tool", "test2json", "-t", "-p", "package-name-is-not-parallelism", "scenario.test.exe", "-test.count=1", "-test.parallel=2", "-test.timeout=10m", "-test.run=^TestA$"},
	} {
		p := executionParametersFor(command, build)
		if p.Count.Value != "1" || p.TestParallel.Value != "2" || p.Timeout.Value != "10m" || p.Tags.Value != "e2e" || p.PackageParallel.State != "not_applicable" || !reflect.DeepEqual(p.BuildCommand, build) {
			t.Fatal(p)
		}
	}
	for _, value := range []string{"true", "1", "TRUE", "True", "false", "0", "FALSE", "False"} {
		p := executionParametersFor([]string{"go", "test", "-c=" + value}, nil)
		compiled := value == "true" || value == "1" || value == "TRUE" || value == "True"
		if (p.Kind == "go_test_compile") != compiled || (p.Count.State == "not_applicable") != compiled {
			t.Fatal("compile boolean semantics differ from Go", value, p)
		}
	}
	compiled := executionParametersFor(build, nil)
	if compiled.Kind != "go_test_compile" || compiled.Count.State != "not_applicable" || compiled.TestParallel.State != "not_applicable" || compiled.Tags.Value != "e2e" {
		t.Fatal(compiled)
	}
	converter := executionParametersFor([]string{"go", "tool", "test2json", "-p", "only-a-package"}, nil)
	if converter.PackageParallel.State != "unknown" || converter.Count.State != "unknown" {
		t.Fatal("converter invented a test command", converter)
	}
	for _, args := range [][]string{{"go", "test", "-count=2", "-count", "4", "-run", "-parallel=88"}, {"go", "test", "-count=4", "-run", "-args"}} {
		p := executionParametersFor(args, nil)
		if p.Count.Value != "4" || p.Count.State != "explicit" || p.TestParallel.State != "default" {
			t.Fatal("flag value or last effective value misread", p)
		}
	}
	for _, marker := range []string{"-args", "--"} {
		p := executionParametersFor([]string{"go", "test", "-p=1", "-count=1", marker, "-test.count=9", "-parallel=99"}, nil)
		if p.Count.State != "unknown" || p.TestParallel.State != "unknown" || p.PackageParallel.Value != "1" {
			t.Fatal("custom tail parsed as authoritative flags", p)
		}
	}
	custom := executionParametersFor([]string{"go", "test", "-count=1", "-custom", "-parallel=8"}, nil)
	if custom.TestParallel.State != "unknown" || custom.Count.State != "unknown" || custom.PackageParallel.State != "unknown" {
		t.Fatal("unknown arity interpreted a custom flag value", custom)
	}
	for _, flag := range []string{"-test.p=9", "-test.tags=custom"} {
		p := executionParametersFor([]string{"go", "test", "./pkg", flag}, nil)
		if p.PackageParallel.State != "unknown" || p.Tags.State != "unknown" {
			t.Fatal("test namespace treated as Go build flags", p)
		}
	}
	binaryCustom := executionParametersFor([]string{"custom.test.exe", "-p", "-test.parallel=9"}, nil)
	if binaryCustom.TestParallel.State != "unknown" {
		t.Fatal("binary custom flag treated as Go build flag", binaryCustom)
	}
	knownBool := executionParametersFor([]string{"go", "test", "-race", "-v", "-parallel=8"}, nil)
	if knownBool.TestParallel.State != "explicit" || knownBool.TestParallel.Value != "8" {
		t.Fatal("known bool consumed next flag", knownBool)
	}
	missing := executionParametersFor([]string{"go", "test", "-parallel"}, nil)
	if missing.TestParallel.State != "unknown" {
		t.Fatal(missing)
	}
	t.Setenv("GOFLAGS", "-p=7 -tags=other")
	inherited := executionParametersFor([]string{"go", "test", "-parallel=1"}, nil)
	if inherited.PackageParallel.State != "unknown" || inherited.Tags.State != "unknown" || inherited.TestParallel.Value != "1" {
		t.Fatal("unexpanded GOFLAGS invented defaults", inherited)
	}
}

func TestExecutionParameterSummaryDistinguishesMixedDefaultsAndUnknown(t *testing.T) {
	t.Setenv("GOFLAGS", "")
	first := executionParametersFor([]string{"go", "test", "-p=1", "-parallel=1"}, nil)
	second := executionParametersFor([]string{"go", "test", "-p=2", "-parallel=3"}, nil)
	finishExecutionParameters(&first, true)
	finishExecutionParameters(&second, true)
	var env environment
	summarizeExecutionParameters(&env, []scopedGroup{{ExecutionParameters: first}, {ExecutionParameters: second}})
	if env.Concurrency["go_build_package_parallel"] != "mixed" || env.Concurrency["go_test_parallel"] != "mixed" {
		t.Fatal(env)
	}
	summarizeExecutionParameters(&env, []scopedGroup{{ExecutionParameters: first}, {ExecutionParameters: first}})
	if env.Concurrency["go_build_package_parallel"] != "1" || env.Concurrency["go_test_parallel"] != "1" {
		t.Fatal(env)
	}
	web := executionParametersFor([]string{"node", "vitest.mjs", "run"}, nil)
	finishExecutionParameters(&web, true)
	summarizeExecutionParameters(&env, []scopedGroup{{ExecutionParameters: web}})
	if env.Concurrency["go_test_parallel"] != "not_applicable" {
		t.Fatal(env)
	}
	defaults := executionParametersFor([]string{"go", "test", "./pkg"}, nil)
	finishExecutionParameters(&defaults, true)
	summarizeExecutionParameters(&env, []scopedGroup{{ExecutionParameters: defaults}})
	if env.Concurrency["go_test_parallel"] != "default" || defaults.TestParallel.Value != "" {
		t.Fatal("controller CPU count substituted for child default", env, defaults)
	}
	first.Status = "not_executed"
	summarizeExecutionParameters(&env, []scopedGroup{{ExecutionParameters: first}})
	if env.Concurrency["go_test_parallel"] != "unknown" {
		t.Fatal("unexecuted group presented as executed", env)
	}
	summarizeExecutionParameters(&env, nil)
	if env.Concurrency["go_test_parallel"] != "unknown" {
		t.Fatal(env)
	}
}

func TestExecutionParameterEvidencePreservesSetupAndProcessFailures(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out := t.TempDir()
	block := filepath.Join(out, "blocked")
	if err := os.WriteFile(block, []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}
	g := runGroup(ctx, block, "group", "", []string{"go", "test", "-parallel=1"}, nil, nil, false)
	if g.Error == "" || g.ExecutionParameters.Status != "not_executed" {
		t.Fatal("setup failure lost execution boundary", g)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	g = runGroup(ctx, t.TempDir(), "empty", "", []string{self, "-test.run=^$", "-test.count=1", "-test.parallel=1"}, nil, nil, false)
	if g.ExecutionParameters.Status != "executed" || g.Summary.Passed {
		t.Fatal("zero matches became success or process execution lost", g)
	}
	g = runGroup(ctx, t.TempDir(), "missing", "", []string{filepath.Join(t.TempDir(), "missing.test.exe"), "-test.parallel=1"}, nil, nil, false)
	if g.ExecutionParameters.Status != "attempted" || g.Error == "" {
		t.Fatal("failed start mislabeled", g)
	}
	g.Command = []string{"original.test.exe", "-test.count=1"}
	build := []string{"go", "test", "-c", "-tags=actual-tag", "-o", "original.test.exe", "./pkg"}
	path := filepath.Join(t.TempDir(), "build.json")
	raw, _ := json.Marshal(build)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	attachBuildParameters(&g, path)
	if g.ExecutionParameters.Status != "attempted" || g.ExecutionParameters.Tags.Value != "actual-tag" {
		t.Fatal("build provenance guessed or execution status overwritten", g)
	}
	attachBuildParameters(&g, filepath.Join(t.TempDir(), "absent"))
	if g.Error == "" {
		t.Fatal("missing build evidence hidden")
	}
	t.Chdir(t.TempDir())
	w := runWebGroup(ctx, t.TempDir(), "web", nil)
	if w.Error == "" || w.ExecutionParameters.Kind != "web" || w.ExecutionParameters.Status != "not_executed" {
		t.Fatal("Web early return invented execution", w)
	}
}

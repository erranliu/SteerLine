package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	feedback "github.com/erranliu/SteerLine/internal/testkit/feedback"
	"github.com/erranliu/SteerLine/internal/testmanagement"
)

func TestLocalSuitesRequireExactSelection(t *testing.T) {
	for _, def := range localSuites {
		args := []string{"-level=L4", "-suite=" + def.Suite, "-scenario=" + def.ID}
		if def.Live {
			args = append(args, "-env=live")
		}
		opts, err := parseOptions(args)
		selected, ok := selectLocalSuite(opts)
		if err != nil || !ok || selected.ID != def.ID || opts.e2eEnv != def.environment() {
			t.Fatal(def.ID, opts, err)
		}
	}
	for _, args := range [][]string{
		{"-level=L4", "-suite=usage", "-scenario=usage.remote-codex"},
		{"-level=L4", "-suite=usage", "-scenario=usage.remote-codex", "-env=isolated"},
		{"-level=L3", "-suite=web", "-env=isolated", "-scenario=web.unit"},
		{"-level=L4", "-suite=web", "-env=controlled"},
		{"-level=L4", "-suite=go", "-scenario=go.http,go.storage"},
		{"-level=L4", "-suite=web", "-scenario=go.full"},
	} {
		if _, err := parseOptions(args); err == nil {
			t.Fatal("accepted ambiguous local suite", args)
		}
	}
}

func TestNativeWebEvidenceRequiresExactFilesAndExecutedAssertions(t *testing.T) {
	root := t.TempDir()
	for _, mode := range []string{"pass", "missing", "extra", "duplicate-file", "zero", "skipped", "todo", "failed", "counter", "exit", "success-false", "duplicate-assertion", "malformed"} {
		t.Run(mode, func(t *testing.T) {
			status, fileStatus, success, exit := "passed", "passed", true, 0
			if mode == "skipped" {
				status = "pending"
			}
			if mode == "todo" {
				status = "todo"
			}
			if mode == "failed" {
				status, fileStatus, success = "failed", "failed", false
			}
			if mode == "exit" {
				exit = 1
			}
			if mode == "success-false" {
				success = false
			}
			assertions := []map[string]any{{"fullName": "actual assertion", "status": status}}
			if mode == "zero" {
				assertions = nil
			}
			if mode == "duplicate-assertion" {
				assertions = append(assertions, assertions[0])
			}
			name := filepath.Join(root, "src/a.test.ts")
			if mode == "extra" {
				name = filepath.Join(root, "src/foreign.test.ts")
			}
			files := []map[string]any{{"name": name, "status": fileStatus, "assertionResults": assertions}}
			if mode == "missing" {
				files = nil
			}
			if mode == "duplicate-file" {
				files = append(files, files[0])
			}
			passed, failed, pending, todo := 1, 0, 0, 0
			if mode == "failed" {
				passed, failed = 0, 1
			}
			if mode == "skipped" {
				passed, pending = 0, 1
			}
			if mode == "todo" {
				passed, todo = 0, 1
			}
			if mode == "counter" {
				passed = 2
			}
			body, _ := json.Marshal(map[string]any{"success": success, "numTotalTests": 1, "numPassedTests": passed, "numFailedTests": failed, "numPendingTests": pending, "numTodoTests": todo, "testResults": files})
			if mode == "malformed" {
				body = []byte("{")
			}
			summary := parseWebReport(body, root, []string{"src/a.test.ts"}, exit)
			if summary.Passed != (mode == "pass") {
				t.Fatal(mode, summary)
			}
		})
	}
}

func TestFullOnlyAllowsEnumeratedCompileOnlyPackageSkips(t *testing.T) {
	report := scopedReport{NoTestPackages: []string{"empty"}}
	group := scopedGroup{Summary: feedback.Summary{SkippedPackages: []string{"empty"}}}
	if skippedCoverage(report, group) {
		t.Fatal("compile-only package is test coverage")
	}
	group.Summary.SkippedTests = 1
	if !skippedCoverage(report, group) {
		t.Fatal("actual skipped test was ignored")
	}
	group.Summary.SkippedTests, group.Summary.SkippedPackages = 0, []string{"unexpected"}
	if !skippedCoverage(report, group) {
		t.Fatal("unknown package skip was ignored")
	}
}

func TestRunnerSuiteMissingInputsWritesIncompleteWithoutStartingRunner(t *testing.T) {
	for _, key := range []string{"STEERRUNNER_EXE", "STEERRUNNER_VERSION", "STEERRUNNER_SHA256", "STEERRUNNER_NATIVE_FIXTURE"} {
		t.Setenv(key, "")
	}
	for _, def := range localSuites {
		if def.Tag == "" {
			continue
		}
		out := t.TempDir()
		if runLocalSuite(options{out: out}, def) == 0 {
			t.Fatal("missing native input passed")
		}
		raw, err := os.ReadFile(filepath.Join(out, "summary.json"))
		if err != nil {
			t.Fatal(err)
		}
		var report scopedReport
		if json.Unmarshal(raw, &report) != nil || report.Passed || len(report.Groups) != 0 || len(report.Errors) == 0 {
			t.Fatal(string(raw))
		}
	}
}

func TestManagedWriteFailureNeverPublishesCompletion(t *testing.T) {
	root := t.TempDir()
	progress := filepath.Join(root, "progress.bin")
	t.Setenv("STEERLINE_PROGRESS_FILE", progress)
	if err := os.WriteFile(progress, []byte(`{"test.l1.minimum":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	opts := options{level: "L1", out: root}
	frozen, err := freezeManagedReports(opts)
	if err != nil {
		t.Fatal(err)
	}
	opts.managed = frozen
	report := scopedReport{Scope: "quick", Complete: true, Passed: true, Groups: []scopedGroup{{Summary: feedback.Summary{Complete: true, Passed: true, Tests: []feedback.Test{{Status: "pass"}}}}}}
	if err = writeJSON(filepath.Join(root, "summary.json"), report); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(filepath.Join(root, "managed-l1.minimum.json"), 0700); err != nil {
		t.Fatal(err)
	}
	if err = writeManagedReports(opts, managedSource{}, 1); err == nil {
		t.Fatal("managed write failure was ignored")
	}
	raw, err := os.ReadFile(progress)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]bool
	if json.Unmarshal(raw, &fields) != nil || fields[testmanagement.ProgressKey("l1.minimum")] {
		t.Fatal("missing evidence left completion true", string(raw))
	}
}

func TestRegisteredExecutionRejectsInheritedGoSelection(t *testing.T) {
	for _, flags := range []string{"-skip=TestAPIRequestRecordAtomicSizeGuard", "-run=^TestOnlyOne$", "-tags=runneracceptance"} {
		t.Setenv("GOFLAGS", flags)
		out := filepath.Join(t.TempDir(), "not-created")
		if code := run([]string{"-level=L4", "-suite=go", "-scenario=go.http", "-out", out}); code != 2 {
			t.Fatal("inherited selection was accepted", flags, code)
		}
		if _, err := os.Stat(out); !os.IsNotExist(err) {
			t.Fatal("rejected selection created output", err)
		}
	}
}

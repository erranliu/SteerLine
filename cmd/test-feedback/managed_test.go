package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	feedback "github.com/erranliu/SteerLine/internal/testkit/feedback"
	"github.com/erranliu/SteerLine/internal/testmanagement"
)

func TestTMManagedResultsPreserveFailureAndNoMatch(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	_ = os.WriteFile("go.mod", []byte("module fixture\n"), 0600)
	_ = os.MkdirAll("SteerlineFlow/testing", 0700)
	d := testmanagement.Definition{ID: "l1.minimum", Name: "L1", Level: "L1", Environment: "isolated", Command: "go run ./cmd/test-feedback -level=L1", Check: "complete"}
	raw, _ := json.Marshal(testmanagement.Catalogue{Tests: []testmanagement.Definition{d}})
	_ = os.WriteFile(testmanagement.CataloguePath, raw, 0600)
	for _, tt := range []struct {
		name                    string
		pass, complete, matched bool
		want                    string
	}{{"pass", true, true, true, "passed"}, {"failure", false, true, true, "failed"}, {"incomplete", false, false, true, "incomplete"}, {"zero-match", true, true, false, "incomplete"}} {
		t.Run(tt.name, func(t *testing.T) {
			out := t.TempDir()
			summary := scopedReport{Scope: "quick", Passed: tt.pass, Complete: tt.complete}
			if tt.matched {
				summary.Groups = []scopedGroup{{Summary: feedback.Summary{Complete: true, Passed: tt.pass, Tests: []feedback.Test{}}}}
				summary.Groups[0].Summary.Tests = append(summary.Groups[0].Summary.Tests, feedback.Test{})
			}
			_ = writeJSON(filepath.Join(out, "summary.json"), summary)
			progress := filepath.Join(t.TempDir(), "progress.bin")
			t.Setenv("STEERLINE_PROGRESS_FILE", progress)
			if err := os.WriteFile(progress, []byte(`{"test.l1.minimum":true,"plan.design":true}`), 0600); err != nil {
				t.Fatal(err)
			}
			opts := options{level: "L1", out: out}
			frozen, freezeErr := freezeManagedReports(opts)
			if freezeErr != nil {
				t.Fatal(freezeErr)
			}
			reset, _ := os.ReadFile(progress)
			var progressFields map[string]bool
			_ = json.Unmarshal(reset, &progressFields)
			if progressFields["test.l1.minimum"] {
				t.Fatal("rerun did not reset previous pass")
			}
			opts.managed = frozen
			writeManagedReports(opts, managedSource{}, 1)
			progressRaw, _ := os.ReadFile(progress)
			_ = json.Unmarshal(progressRaw, &progressFields)
			if progressFields["test.l1.minimum"] != (tt.want == "passed") || !progressFields["plan.design"] {
				t.Fatal("progress differs from actual result", string(progressRaw))
			}
			body, err := os.ReadFile(filepath.Join(out, "managed-l1.minimum.json"))
			if err != nil {
				t.Fatal(err)
			}
			r, err := testmanagement.ParseReport(body)
			if err != nil || r.Results[0].Status != tt.want {
				t.Fatal(string(body), err)
			}
		})
	}
	out := t.TempDir()
	writeManagedReports(options{level: "L1", out: out, goTestArgs: []string{"./subset"}}, managedSource{}, 1)
	if _, err := os.Stat(filepath.Join(out, "managed-l1.minimum.json")); !os.IsNotExist(err) {
		t.Fatal("targeted subset claimed complete L1")
	}
}

func TestTMRegisteredCatalogueMatchesExecutableManifest(t *testing.T) {
	root, err := e2eRoot()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(testmanagement.CataloguePath)))
	if err != nil {
		t.Fatal(err)
	}
	catalogue, err := testmanagement.ParseCatalogue(raw)
	if err != nil {
		t.Fatal(err)
	}
	manifestRaw, err := os.ReadFile(filepath.Join(root, "tests/e2e/suites.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest []e2eScenario
	if err = json.Unmarshal(manifestRaw, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(catalogue.Tests) != len(manifest)+2 {
		t.Fatal("catalogue and executable manifest differ")
	}
	byID := map[string]testmanagement.Definition{}
	for _, d := range catalogue.Tests {
		byID[d.ID] = d
	}
	for _, entry := range manifest {
		d, ok := byID[entry.ID]
		if !ok || d.Level != entry.Level || d.Environment != entry.Env || !strings.Contains(d.Command, "-level="+entry.Level) {
			t.Fatal("catalogue changed level/environment/command", entry.ID)
		}
	}
}

func TestProgressE2EFinalizationRequiresVerifiedScenario(t *testing.T) {
	entry := e2eScenario{ID: "fixture.progress", Test: "TestProgress", Package: "./tests/e2e/suites/testing", Level: "L3", Env: "controlled"}
	definition := testmanagement.Definition{ID: entry.ID, Name: "Progress", Level: entry.Level, Environment: entry.Env, Command: "actual scenario", Check: "verified scenario and cleanup"}
	pkg := strings.TrimSuffix(corePackage, "/internal/app/host") + "/tests/e2e/suites/testing"
	for _, tc := range []struct {
		name                                      string
		missingResources, skipped, missingCleanup bool
	}{
		{name: "passed"},
		{name: "missing-resources", missingResources: true},
		{name: "skipped", skipped: true},
		{name: "missing-cleanup", missingCleanup: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := t.TempDir()
			path := filepath.Join(out, "scenarios", entry.ID)
			if err := os.MkdirAll(path, 0700); err != nil {
				t.Fatal(err)
			}
			resources := []map[string]any{{"kind": "owned-fixture"}}
			if tc.missingResources {
				resources = nil
			}
			if err := writeJSON(filepath.Join(path, "scenario.json"), map[string]any{"id": entry.ID, "passed": true, "cleanup_complete": !tc.missingCleanup, "resources": resources}); err != nil {
				t.Fatal(err)
			}
			group := scopedGroup{Packages: []string{entry.Package}, Summary: feedback.Summary{Passed: true, Complete: true, Packages: []feedback.Package{{Name: pkg, Status: "pass"}}, Tests: []feedback.Test{{Package: pkg, Name: entry.Test, Status: "pass"}}}}
			if tc.skipped {
				group.Summary.SkippedTests = 1
			}
			if err := writeJSON(filepath.Join(out, "summary.json"), e2eReport{Selected: []e2eScenario{entry}, Groups: []scopedGroup{group}, Cleanup: true}); err != nil {
				t.Fatal(err)
			}
			progress := filepath.Join(out, "progress.bin")
			t.Setenv("STEERLINE_PROGRESS_FILE", progress)
			if err := os.WriteFile(progress, []byte(`{"test.fixture.progress":false}`), 0600); err != nil {
				t.Fatal(err)
			}
			frozen := &managedRun{definitions: map[string]testmanagement.Definition{entry.ID: definition}, environments: map[string]string{entry.ID: entry.Env}, ids: map[string]string{entry.ID: testmanagement.NewRecordID()}}
			writeManagedReports(options{level: entry.Level, out: out, managed: frozen}, managedSource{}, 1)
			raw, err := os.ReadFile(progress)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]bool
			if err := json.Unmarshal(raw, &fields); err != nil {
				t.Fatal(err)
			}
			if fields["test.fixture.progress"] != (tc.name == "passed") {
				t.Fatal("unverified finalization completed progress", string(raw))
			}
		})
	}
}

func TestManagedGoFullCannotClaimSubsetOrSkippedCoverage(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	if err := os.WriteFile("go.mod", []byte("module fixture\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll("SteerlineFlow/testing", 0700); err != nil {
		t.Fatal(err)
	}
	d := testmanagement.Definition{ID: "go.full", Name: "Go", Level: "L4", Environment: "isolated", Command: "go run ./cmd/test-feedback -level=L4 -suite=go", Check: "complete"}
	raw, _ := json.Marshal(testmanagement.Catalogue{Tests: []testmanagement.Definition{d}})
	if err := os.WriteFile(testmanagement.CataloguePath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, scope, testStatus, want string
		complete, skipped             bool
	}{
		{"full", "full", "pass", "passed", true, false},
		{"quick-subset", "quick", "pass", "incomplete", true, false},
		{"zero-match", "full", "", "incomplete", true, false},
		{"skipped", "full", "pass", "incomplete", true, true},
		{"incomplete", "full", "pass", "incomplete", false, false},
		{"assertion-failure", "full", "fail", "failed", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := options{level: "L4", suite: "go", out: t.TempDir()}
			var err error
			opts.managed, err = freezeManagedReports(opts)
			if err != nil {
				t.Fatal(err)
			}
			group := scopedGroup{Summary: feedback.Summary{Complete: true}}
			if tc.testStatus != "" {
				group.Summary.Tests = []feedback.Test{{Status: tc.testStatus}}
			}
			if tc.skipped {
				group.Summary.SkippedTests = 1
			}
			summary := scopedReport{Scope: tc.scope, Passed: tc.testStatus == "pass", Complete: tc.complete, Groups: []scopedGroup{group}}
			if err = writeJSON(filepath.Join(opts.out, "summary.json"), summary); err != nil {
				t.Fatal(err)
			}
			writeManagedReports(opts, managedSource{}, 1)
			body, err := os.ReadFile(filepath.Join(opts.out, "managed-go.full.json"))
			if err != nil {
				t.Fatal(err)
			}
			report, err := testmanagement.ParseReport(body)
			if err != nil || report.Results[0].Status != tc.want {
				t.Fatalf("report status differs: %s %v", body, err)
			}
		})
	}
}

func TestRegisteredCommandsSelectExactlyTheirDefinitions(t *testing.T) {
	root, err := e2eRoot()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(testmanagement.CataloguePath)))
	if err != nil {
		t.Fatal(err)
	}
	catalogue, err := testmanagement.ParseCatalogue(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range catalogue.Tests {
		args := strings.Fields(d.Command)
		if len(args) < 4 || strings.Join(args[:3], " ") != "go run ./cmd/test-feedback" {
			t.Fatalf("unsupported registered command: %s", d.Command)
		}
		opts, err := parseOptions(args[3:])
		if err != nil {
			t.Fatal(d.ID, err)
		}
		if opts.level != d.Level {
			t.Fatal("registered level differs", d.ID)
		}
		frozen, err := freezeManagedReports(opts)
		if err != nil {
			t.Fatal(d.ID, err)
		}
		if len(frozen.definitions) != 1 || frozen.definitions[d.ID].ID != d.ID {
			t.Fatal("command selects a different definition", d.ID, frozen.definitions)
		}
	}
}

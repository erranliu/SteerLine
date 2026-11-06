package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	feedback "github.com/erranliu/SteerLine/internal/testkit/feedback"
	"github.com/erranliu/SteerLine/internal/testmanagement"
)

func localEvidence(t *testing.T, group localGroup, mode string) scopedGroup {
	t.Helper()
	pkg := strings.TrimSuffix(corePackage, "/internal/app/host") + "/" + strings.TrimPrefix(group.Packages[0], "./")
	if mode == "wrong-package" {
		pkg += "/foreign"
	}
	var events []feedback.Event
	add := func(action, name string) {
		elapsed := 0.01
		events = append(events, feedback.Event{Time: "2026-11-02T00:00:00Z", Action: action, Package: pkg, Test: name, Elapsed: &elapsed})
	}
	if mode != "missing-package-start" {
		add("start", "")
	}
	if mode == "duplicate-package-start" {
		add("start", "")
	}
	if mode != "zero" && mode != "missing-root" {
		add("run", "TestSelected")
		add("run", "TestSelected/child")
		if mode == "duplicate" {
			add("run", "TestSelected")
		}
		if mode != "incomplete-child" {
			action := "pass"
			if mode == "skip" {
				action = "skip"
			}
			add(action, "TestSelected/child")
		}
		action := "pass"
		if mode == "failed" {
			action = "fail"
		}
		add(action, "TestSelected")
	}
	if mode == "extra-root" || mode == "platform-extra" {
		name := "TestExtra"
		if mode == "platform-extra" {
			name = "TestProjectEnvironmentScriptReportsToolsWithoutChainChecks"
		}
		add("run", name)
		add("pass", name)
	}
	if mode == "orphan" {
		add("pass", "TestOrphan")
	}
	add("pass", "")
	var raw []byte
	for _, e := range events {
		b, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		raw = append(raw, append(b, '\n')...)
	}
	path := filepath.Join(t.TempDir(), "test.jsonl")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if mode == "missing-log" {
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
	result := scopedGroup{ID: group.ID, Packages: group.Packages, Assigned: group.Tests, StdoutLog: path, Summary: feedback.Parse(raw, 0)}
	if mode == "process-error" {
		result.Error = "original process failed"
	}
	return result
}

func TestLocalGroupsPreserveEvidenceFailuresThroughManagedAndProgress(t *testing.T) {
	for _, mode := range []string{"pass", "missing-root", "wrong-package", "extra-root", "platform-extra", "platform-missing", "duplicate", "orphan", "missing-package-start", "duplicate-package-start", "incomplete-child", "skip", "zero", "failed", "missing-log", "process-error", "report-write-failure"} {
		t.Run(mode, func(t *testing.T) {
			selected := localGroup{ID: "app", Packages: []string{"./internal/app"}, Tests: []string{"TestSelected"}}
			if mode == "platform-missing" {
				selected.Tests = append(selected.Tests, "TestProjectEnvironmentScriptReportsToolsWithoutChainChecks")
			}
			owner := localGroup{ID: "owner", Packages: []string{"./internal/executionconfig"}}
			group, other := localEvidence(t, selected, mode), localEvidence(t, owner, "pass")
			out := t.TempDir()
			progress := filepath.Join(t.TempDir(), "progress.json")
			t.Setenv("STEERLINE_PROGRESS_FILE", progress)
			if err := os.WriteFile(progress, []byte(`{"test.go.environment":true,"plan.design":true}`), 0600); err != nil {
				t.Fatal(err)
			}
			opts := options{level: "L4", suite: "go", scenario: "go.environment", out: out}
			frozen, err := freezeManagedReports(opts)
			if err != nil {
				t.Fatal(err)
			}
			opts.managed = frozen
			report := scopedReport{Scope: "local", Groups: []scopedGroup{group, other}}
			for i, definition := range []localGroup{selected, owner} {
				if err := checkLocalGroup(report.Groups[i], definition); err != nil {
					report.Errors = append(report.Errors, err.Error())
				}
			}
			if mode == "report-write-failure" {
				if err := os.Mkdir(filepath.Join(out, "summary.json"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			code := finishLocalReport(out, &report)
			if (code == 0) != (mode == "pass") {
				t.Fatal("exit code accepted invalid evidence", mode, code, report.Errors)
			}
			if err := writeManagedReports(opts, managedSource{}, 1); (err == nil) != (mode == "pass") {
				t.Fatal("managed error did not preserve evidence status", mode, err)
			}
			raw, err := os.ReadFile(filepath.Join(out, "managed-go.environment.json"))
			if err != nil {
				t.Fatal(err)
			}
			managed, err := testmanagement.ParseReport(raw)
			if err != nil || (managed.Results[0].Status == "passed") != (mode == "pass") {
				t.Fatal("managed lost failure", string(raw), err)
			}
			raw, err = os.ReadFile(progress)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]bool
			if err := json.Unmarshal(raw, &fields); err != nil {
				t.Fatal(err)
			}
			if fields["test.go.environment"] != (mode == "pass") || !fields["plan.design"] {
				t.Fatal("completion lost original failure", string(raw))
			}
		})
	}
}

func TestEnvironmentSelectionRejectsSourceAndDefinitionDrift(t *testing.T) {
	windows := environmentAppRootsForGOOS("windows")
	if len(windows) != 31 {
		t.Fatal("Windows environment root count changed", windows)
	}
	for _, goos := range []string{"windows", "linux", "darwin"} {
		t.Run("selection/"+goos, func(t *testing.T) {
			want := slices.Clone(windows)
			if goos != "windows" {
				want = slices.DeleteFunc(want, func(name string) bool { return name == "TestProjectEnvironmentScriptReportsToolsWithoutChainChecks" })
			}
			if got := environmentAppRootsForGOOS(goos); !slices.Equal(got, want) || (goos != "windows" && len(got) != 30) {
				t.Fatal("platform selection changed explicit roots", goos, got, want)
			}
		})
	}
	for _, mode := range []string{"pass", "added-root", "renamed-root", "deleted-root", "platform-renamed-root", "platform-declaration", "new-file", "owner-filter", "app-extra-root", "wrong-app-package", "duplicate-definition"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, "internal/app")
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			for _, source := range []map[string][]string{environmentAppSources, environmentAppExtra} {
				for file, names := range source {
					body := "package app\nimport \"testing\"\n"
					for i, name := range names {
						if mode == "platform-renamed-root" && file == "environment_script_windows_test.go" {
							name += "Renamed"
						}
						if file == "environment_target_test.go" && i == 0 {
							if mode == "deleted-root" {
								continue
							}
							if mode == "renamed-root" {
								name += "Renamed"
							}
						}
						body += fmt.Sprintf("func %s(t *testing.T) {}\n", name)
					}
					if mode == "added-root" && file == "environment_target_test.go" {
						body += "func TestAnother(t *testing.T) {}\n"
					}
					if err := os.WriteFile(filepath.Join(dir, file), []byte(body), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			if mode == "new-file" {
				if err := os.WriteFile(filepath.Join(dir, "environment_new_test.go"), []byte("package app\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			original := localSuites
			localSuites = slices.Clone(localSuites)
			defer func() { localSuites = original }()
			if mode == "platform-declaration" {
				previous := environmentAppSourceGOOS
				environmentAppSourceGOOS = map[string]string{"environment_script_windows_test.go": "linux"}
				defer func() { environmentAppSourceGOOS = previous }()
			}
			for i := range localSuites {
				if localSuites[i].ID != "go.environment" {
					continue
				}
				localSuites[i].Groups = slices.Clone(localSuites[i].Groups)
				switch mode {
				case "owner-filter":
					localSuites[i].Groups[0].Tests = []string{"TestSelected"}
				case "app-extra-root":
					localSuites[i].Groups[1].Tests = append(slices.Clone(environmentAppRoots()), "TestExtra")
				case "wrong-app-package":
					localSuites[i].Groups[1].Packages = []string{"./internal/taskrunner"}
				case "duplicate-definition":
					localSuites[i].Groups[1].Tests = append(slices.Clone(environmentAppRoots()), environmentAppRoots()[0])
				}
			}
			if err := checkEnvironmentAppSources(root); (err == nil) != (mode == "pass") {
				t.Fatal("source/selection drift not rejected", mode, err)
			}
		})
	}
}

func TestLocalGroupCommandsKeepOwnerUnfilteredAndHTTPComplete(t *testing.T) {
	for _, suite := range localSuites {
		if suite.ID == "go.full" || suite.ID == "web.unit" {
			continue
		}
		if err := validateLocalGroups(suite.Groups); err != nil {
			t.Fatal(suite.ID, err)
		}
		if suite.ID == "go.http" {
			if !slices.Equal(suite.Groups[0].Packages, []string{"./web/server", "./internal/product/httpapi", "./internal/product/httpclient", "./internal/product/httpfs", "./web/server/executionconfig", "./web/server/discovery", "./web/server/nativequery"}) {
				t.Fatal("HTTP owner scope changed")
			}
		}
		for _, group := range suite.Groups {
			command := localGroupCommand(suite.Tag, group)
			filtered := false
			for _, arg := range command {
				filtered = filtered || strings.HasPrefix(arg, "-run=")
			}
			if filtered != (len(group.Tests) > 0) {
				t.Fatal("owner inherited another group's filter", command)
			}
		}
	}
	if err := validateLocalGroups([]localGroup{{ID: "a", Packages: []string{"./one"}}, {ID: "b", Packages: []string{"./one"}}}); err == nil {
		t.Fatal("overlapping groups accepted")
	}
}

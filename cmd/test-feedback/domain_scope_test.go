package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/testmanagement"
)

func domainSourceFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for path, source := range appDomainSources {
		body := "package " + filepath.Base(filepath.Dir(path)) + "\n"
		if source.BuildTag != "" {
			body = "//go:build " + source.BuildTag + "\n\n" + body
		}
		if len(source.Roots) > 0 {
			body += "import \"testing\"\n"
		}
		for _, name := range source.Roots {
			body += fmt.Sprintf("func %s(t *testing.T) {}\n", name)
		}
		file := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestAppDomainSelectionPreservesPlatformOwnersAndExplicitRoots(t *testing.T) {
	for _, goos := range []string{"windows", "linux", "darwin"} {
		dep := domainGroupsForGOOS("go.deployment", goos)
		wantGroups, wantApp := 3, 10
		if goos == "windows" {
			wantGroups, wantApp = 4, 11
		}
		if len(dep) != wantGroups || len(domainRootsForGOOS("go.deployment", "internal/app", goos)) != wantApp {
			t.Fatal(goos, dep)
		}
		road := domainGroupsForGOOS("go.roadmap", goos)
		if len(road) != 3 || len(road[0].Tests) != 0 || len(road[1].Tests) != 11 || len(road[2].Tests) != 1 {
			t.Fatal(goos, road)
		}
		if !slices.Equal(dep[0].Packages, []string{"./internal/app/host"}) || len(dep[0].Tests) != 0 {
			t.Fatal("deployment host owner filtered", dep)
		}
		if goos == "windows" && (!slices.Equal(dep[1].Packages, []string{"./internal/localservice"}) || len(dep[1].Tests) != 0) {
			t.Fatal("Windows service owner changed", dep)
		}
		upgrade := dep[len(dep)-1]
		if !slices.Equal(upgrade.Tests, []string{"TestUpgradeDrainedUsesCandidateProofAndBothBusinessReaders"}) {
			t.Fatal("upgrade gate missing", upgrade)
		}
		for _, groups := range [][]localGroup{dep, road} {
			if err := validateLocalGroups(groups); err != nil {
				t.Fatal(err)
			}
			for _, g := range groups {
				if slices.Contains(g.Tests, "TestRoadmapLocalCodexPromotion") {
					t.Fatal("live root entered ordinary set")
				}
			}
		}
	}
}

func TestAppDomainSourceAndDefinitionDriftIsRejected(t *testing.T) {
	for _, mode := range []string{"pass", "new-app-file", "new-host-file", "new-upgrade-file", "missing-file", "renamed-root", "missing-root", "extra-root", "helper-root", "windows-root", "windows-name", "ordinary-tag", "live-tag-removed", "live-tag-changed", "legacy-tag", "duplicate-definition", "missing-definition", "owner-filter", "wrong-package", "extra-assignment", "definition-tag"} {
		t.Run(mode, func(t *testing.T) {
			root := domainSourceFixture(t)
			path := filepath.Join(root, "internal/app/roadmap_test.go")
			switch mode {
			case "new-app-file", "new-host-file", "new-upgrade-file":
				dir, name := "internal/app", "deployment_added_test.go"
				if mode == "new-host-file" {
					dir, name = "internal/app/host", "roadmap_added_test.go"
				}
				if mode == "new-upgrade-file" {
					dir, name = "internal/runnerclient", "upgrade_added_test.go"
				}
				if err := os.WriteFile(filepath.Join(root, dir, name), []byte("package fixture\n"), 0600); err != nil {
					t.Fatal(err)
				}
			case "missing-file":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			case "windows-name":
				from := filepath.Join(root, "internal/app/deployment_record_windows_test.go")
				if err := os.Rename(from, strings.Replace(from, "_windows_", "_linux_", 1)); err != nil {
					t.Fatal(err)
				}
			case "renamed-root", "missing-root", "extra-root", "helper-root", "windows-root", "ordinary-tag", "live-tag-removed", "live-tag-changed", "legacy-tag":
				if mode == "helper-root" {
					path = filepath.Join(root, "internal/app/roadmap_generation_test.go")
				}
				if mode == "windows-root" {
					path = filepath.Join(root, "internal/app/deployment_record_windows_test.go")
				}
				if strings.HasPrefix(mode, "live-") {
					path = filepath.Join(root, "internal/app/roadmap_live_test.go")
				}
				body, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				s := string(body)
				switch mode {
				case "renamed-root":
					s = strings.Replace(s, "TestRoadmapAdmissionAtomicNumbersSourceAndReplay", "TestRenamed", 1)
				case "missing-root":
					s = strings.Replace(s, "func TestRoadmapAdmissionAtomicNumbersSourceAndReplay(t *testing.T) {}\n", "", 1)
				case "extra-root", "helper-root":
					s += "func TestNew(t *testing.T) {}\n"
				case "windows-root":
					s = strings.Replace(s, "TestDeploymentRecordScriptMeetsRecoveryACL", "TestRenamed", 1)
				case "ordinary-tag":
					s = "//go:build windows\n\n" + s
				case "live-tag-removed":
					s = strings.Replace(s, "//go:build e2e\n\n", "", 1)
				case "live-tag-changed":
					s = strings.Replace(s, "//go:build e2e", "//go:build runneracceptance", 1)
				case "legacy-tag":
					s = "// +build windows\n\n" + s
				}
				if err := os.WriteFile(path, []byte(s), 0600); err != nil {
					t.Fatal(err)
				}
			}
			original := localSuites
			localSuites = slices.Clone(original)
			defer func() { localSuites = original }()
			for i := range localSuites {
				if localSuites[i].ID != "go.roadmap" {
					continue
				}
				localSuites[i].Groups = slices.Clone(localSuites[i].Groups)
				switch mode {
				case "duplicate-definition":
					localSuites = append(localSuites, localSuites[i])
				case "missing-definition":
					localSuites[i].ID = "removed"
				case "owner-filter":
					localSuites[i].Groups[0].Tests = []string{"TestFiltered"}
				case "wrong-package":
					localSuites[i].Groups[1].Packages = []string{"./wrong"}
				case "extra-assignment":
					localSuites[i].Groups[1].Tests = append(slices.Clone(localSuites[i].Groups[1].Tests), "TestExtra")
				case "definition-tag":
					localSuites[i].Tag = "e2e"
				}
				break
			}
			if err := checkAppDomainSources(root); (err == nil) != (mode == "pass") {
				t.Fatal(mode, err, runtime.GOOS)
			}
		})
	}
}

func TestAppDomainEvidenceFailuresReachManagedAndProgress(t *testing.T) {
	for _, id := range []string{"go.deployment", "go.roadmap"} {
		for _, mode := range []string{"pass", "missing-root", "wrong-package", "extra-root", "duplicate", "orphan", "missing-package-start", "duplicate-package-start", "incomplete-child", "skip", "zero", "failed", "missing-log", "process-error", "report-write-failure"} {
			t.Run(id+"/"+mode, func(t *testing.T) {
				definition := localGroup{ID: "selected", Packages: []string{"./internal/app"}, Tests: []string{"TestSelected"}}
				g := localEvidence(t, definition, mode)
				out, progress := t.TempDir(), filepath.Join(t.TempDir(), "progress.json")
				t.Setenv("STEERLINE_PROGRESS_FILE", progress)
				if err := os.WriteFile(progress, []byte(fmt.Sprintf(`{"test.%s":true,"plan.design":true}`, id)), 0600); err != nil {
					t.Fatal(err)
				}
				opts := options{level: "L4", suite: "go", scenario: id, out: out}
				frozen, err := freezeManagedReports(opts)
				if err != nil {
					t.Fatal(err)
				}
				opts.managed = frozen
				report := scopedReport{Scope: "local", Groups: []scopedGroup{g}}
				if err := checkLocalGroup(g, definition); err != nil {
					report.Errors = append(report.Errors, err.Error())
				}
				if mode == "report-write-failure" {
					if err := os.Mkdir(filepath.Join(out, "summary.json"), 0700); err != nil {
						t.Fatal(err)
					}
				}
				if code := finishLocalReport(out, &report); (code == 0) != (mode == "pass") {
					t.Fatal("invalid final exit", id, mode, code)
				}
				if err := writeManagedReports(opts, managedSource{}, 1); (err == nil) != (mode == "pass") {
					t.Fatal("managed failure lost", mode, err)
				}
				raw, err := os.ReadFile(filepath.Join(out, "managed-"+id+".json"))
				if err != nil {
					t.Fatal(err)
				}
				managed, err := testmanagement.ParseReport(raw)
				if err != nil || (managed.Results[0].Status == "passed") != (mode == "pass") {
					t.Fatal("managed status", mode, err)
				}
				raw, err = os.ReadFile(progress)
				if err != nil {
					t.Fatal(err)
				}
				var fields map[string]bool
				if err = json.Unmarshal(raw, &fields); err != nil {
					t.Fatal(err)
				}
				if fields["test."+id] != (mode == "pass") || !fields["plan.design"] {
					t.Fatal("progress failure lost", mode, fields)
				}
			})
		}
	}
}

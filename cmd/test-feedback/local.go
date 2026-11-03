package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Fixed reusable sets are executable definitions, not user-supplied Go flags.
type localSuite struct {
	ID, Suite string
	Groups    []localGroup
	Tag       string
	Required  []string
}

type localGroup struct {
	ID       string
	Packages []string
	Tests    []string
}

var localSuites = []localSuite{
	{ID: "go.full", Suite: "go"},
	{ID: "web.unit", Suite: "web"},
	{ID: "go.http", Suite: "go", Groups: []localGroup{{ID: "go-owner", Packages: []string{"./web/server", "./internal/product/httpapi", "./internal/product/httpclient", "./internal/product/httpfs", "./web/server/executionconfig", "./web/server/discovery", "./web/server/nativequery"}}}},
	{ID: "go.storage", Suite: "go", Groups: []localGroup{{ID: "go-owner", Packages: []string{"./internal/storage/sqlite", "./internal/pathguard"}}}},
	{ID: "go.environment", Suite: "go", Groups: []localGroup{
		{ID: "go-environment-owner", Packages: []string{"./internal/environmentadaptation", "./internal/environmentadaptation/model", "./internal/testkit/remotetest", "./internal/executionconfig"}},
		{ID: "go-environment-app", Packages: []string{"./internal/app"}, Tests: environmentAppRoots()},
	}},
	{ID: "go.runner-contract", Suite: "go", Groups: []localGroup{
		{ID: "go-runner-app", Packages: []string{"./internal/app"}, Tests: []string{"TestPublishedRunnerApplicationLifecycle"}},
		{ID: "go-runner-client", Packages: []string{"./internal/runnerclient"}, Tests: []string{"TestLegacyCheckRetirementPublishedContract", "TestRunnerFullBindingPublicCLI"}},
	}, Tag: "runneracceptance", Required: []string{"STEERRUNNER_EXE", "STEERRUNNER_VERSION", "STEERRUNNER_SHA256"}},
	{ID: "go.runner-result", Suite: "go", Groups: []localGroup{{ID: "go-owner", Packages: []string{"./internal/runnerclient"}, Tests: []string{"TestProjectEnvironmentExecutionPublishedResultRoundTrip"}}}, Tag: "runneracceptance", Required: []string{"STEERRUNNER_EXE", "STEERRUNNER_VERSION", "STEERRUNNER_SHA256", "STEERRUNNER_NATIVE_FIXTURE"}},
}

func selectLocalSuite(opts options) (localSuite, bool) {
	if opts.level != "L4" {
		return localSuite{}, false
	}
	for _, def := range localSuites {
		if opts.suite == def.Suite && (opts.scenario == def.ID || opts.scenario == "" && (def.ID == "go.full" || def.ID == "web.unit")) {
			return def, true
		}
	}
	return localSuite{}, false
}

func runLocalSuite(opts options, def localSuite) int {
	started := time.Now()
	report := scopedReport{SchemaVersion: 1, Scope: "local", Phase: opts.phase, Workers: 1, StartedAt: started.UTC().Format(time.RFC3339Nano), Groups: []scopedGroup{}, Errors: []string{}, Environment: collectEnvironment(opts.cacheCondition, opts.nativeNote, opts.out, nil)}
	for _, key := range def.Required {
		if strings.TrimSpace(os.Getenv(key)) == "" {
			return scopedSetupError(opts.out, &report, fmt.Errorf("%s requires explicit %s", def.ID, key))
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	if def.ID == "web.unit" {
		report.Groups = append(report.Groups, runWebGroup(ctx, opts.out, "web-unit", nil))
	} else {
		if err := validateLocalGroups(def.Groups); err != nil {
			return scopedSetupError(opts.out, &report, err)
		}
		if def.ID == "go.environment" {
			root, err := e2eRoot()
			if err == nil {
				err = checkEnvironmentAppSources(root)
			}
			if err != nil {
				return scopedSetupError(opts.out, &report, err)
			}
		}
		for _, selected := range def.Groups {
			g := runGroup(ctx, opts.out, selected.ID, "", localGroupCommand(def.Tag, selected), selected.Tests, selected.Packages, true)
			report.Groups = append(report.Groups, g)
			if err := checkLocalGroup(g, selected); err != nil {
				report.Errors = append(report.Errors, selected.ID+": "+err.Error())
			}
		}
	}
	report.ExecutionSecs = time.Since(started).Seconds()
	return finishLocalReport(opts.out, &report)
}

func localGroupCommand(tag string, group localGroup) []string {
	args := []string{"go", "test", "-json", "-count=1", "-timeout=25m", "-p=1", "-parallel=1"}
	if tag != "" {
		args = append(args, "-tags="+tag)
	}
	if len(group.Tests) > 0 {
		args = append(args, "-run="+exactPattern(group.Tests))
	}
	return append(args, group.Packages...)
}

func validateLocalGroups(groups []localGroup) error {
	ids, packages := map[string]bool{}, map[string]bool{}
	if len(groups) == 0 {
		return fmt.Errorf("local suite has no groups")
	}
	for _, group := range groups {
		if group.ID == "" || ids[group.ID] || len(group.Packages) == 0 || len(group.Tests) > 0 && len(group.Packages) != 1 {
			return fmt.Errorf("invalid or duplicate local group %s", group.ID)
		}
		ids[group.ID] = true
		for _, pkg := range group.Packages {
			if !strings.HasPrefix(pkg, "./") || packages[pkg] {
				return fmt.Errorf("invalid or repeated local package %s", pkg)
			}
			packages[pkg] = true
		}
		roots := map[string]bool{}
		for _, name := range group.Tests {
			if name == "" || strings.Contains(name, "/") || roots[name] {
				return fmt.Errorf("invalid or repeated assigned root %s", name)
			}
			roots[name] = true
		}
	}
	return nil
}

func checkLocalGroup(group scopedGroup, selected localGroup) error {
	if group.Error != "" {
		return fmt.Errorf("%s", group.Error)
	}
	raw, err := os.ReadFile(group.StdoutLog)
	if err != nil {
		return err
	}
	runs, terminals, packages, err := scanEvents(raw)
	if err != nil {
		return err
	}
	starts := map[string]int{}
	for _, line := range strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n") {
		var event struct{ Action, Package, Test string }
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return err
		}
		if event.Test == "" && event.Action == "start" {
			starts[event.Package]++
		}
	}
	expected, counts := map[string]bool{}, map[string]int{}
	module := strings.TrimSuffix(corePackage, "/internal/app/host")
	for _, pkg := range selected.Packages {
		expected[module+"/"+strings.TrimPrefix(pkg, "./")] = true
	}
	roots := map[string]bool{}
	for _, name := range selected.Tests {
		roots[name] = true
	}
	for key, count := range runs {
		pkg, name, _ := strings.Cut(key, "\x00")
		root, _, child := strings.Cut(name, "/")
		if !expected[pkg] || count != 1 || terminals[key] != 1 || packages[pkg] != 1 {
			return fmt.Errorf("unexpected or incomplete test %s", key)
		}
		if child && runs[pkg+"\x00"+root] != 1 {
			return fmt.Errorf("subtest without assigned root %s", key)
		}
		if len(roots) > 0 && !roots[root] {
			return fmt.Errorf("unassigned root %s", key)
		}
		counts[pkg]++
	}
	for key, count := range terminals {
		if count != 1 || runs[key] != 1 {
			return fmt.Errorf("orphan or duplicate terminal %s", key)
		}
	}
	for pkg, count := range packages {
		if !expected[pkg] || count != 1 || starts[pkg] != 1 || counts[pkg] == 0 {
			return fmt.Errorf("unexpected, empty or duplicate package %s", pkg)
		}
	}
	for pkg, count := range starts {
		if !expected[pkg] || count != 1 || packages[pkg] != 1 {
			return fmt.Errorf("orphan or duplicate package start %s", pkg)
		}
	}
	for pkg := range expected {
		if packages[pkg] != 1 || counts[pkg] == 0 {
			return fmt.Errorf("missing assigned package %s", pkg)
		}
		for root := range roots {
			if runs[pkg+"\x00"+root] != 1 {
				return fmt.Errorf("missing assigned root %s/%s", pkg, root)
			}
		}
	}
	if len(runs) == 0 {
		return fmt.Errorf("no tests matched")
	}
	return nil
}

func finishLocalReport(out string, report *scopedReport) int {
	for _, g := range report.Groups {
		if g.Error != "" {
			report.Errors = append(report.Errors, g.ID+": "+g.Error)
		}
		if !g.Summary.Passed || !g.Summary.Complete || g.Summary.TestCount == 0 || g.Summary.SkippedTests > 0 || len(g.Summary.SkippedPackages) > 0 {
			report.Errors = append(report.Errors, g.ID+": failed, empty, skipped or incomplete tests")
		}
	}
	report.Complete, report.Passed = len(report.Errors) == 0, len(report.Errors) == 0
	report.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := writeJSON(filepath.Join(out, "summary.json"), report); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	fmt.Fprintf(os.Stderr, "local test feedback: passed=%v; report %s\n", report.Passed, filepath.Join(out, "summary.json"))
	for _, err := range report.Errors {
		fmt.Fprintln(os.Stderr, err)
	}
	if !report.Passed {
		return 1
	}
	return 0
}

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Fixed reusable sets are executable definitions, not user-supplied Go flags.
type localSuite struct {
	ID, Suite string
	Packages  []string
	Tests     []string
	Tag       string
	Required  []string
}

var localSuites = []localSuite{
	{ID: "go.full", Suite: "go"},
	{ID: "web.unit", Suite: "web"},
	{ID: "go.http", Suite: "go", Packages: []string{"./web/server", "./internal/product/httpapi", "./internal/product/httpclient"}},
	{ID: "go.storage", Suite: "go", Packages: []string{"./internal/storage/sqlite", "./internal/pathguard"}},
	{ID: "go.environment", Suite: "go", Packages: []string{"./internal/environmentadaptation", "./internal/environmentadaptation/model", "./internal/testkit/remotetest", "./internal/app"}},
	{ID: "go.runner-contract", Suite: "go", Packages: []string{"./internal/app", "./internal/runnerclient"}, Tests: []string{"TestPublishedRunnerApplicationLifecycle", "TestLegacyCheckRetirementPublishedContract", "TestRunnerFullBindingPublicCLI"}, Tag: "runneracceptance", Required: []string{"STEERRUNNER_EXE", "STEERRUNNER_VERSION", "STEERRUNNER_SHA256"}},
	{ID: "go.runner-result", Suite: "go", Packages: []string{"./internal/runnerclient"}, Tests: []string{"TestProjectEnvironmentExecutionPublishedResultRoundTrip"}, Tag: "runneracceptance", Required: []string{"STEERRUNNER_EXE", "STEERRUNNER_VERSION", "STEERRUNNER_SHA256", "STEERRUNNER_NATIVE_FIXTURE"}},
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
		args := []string{"go", "test", "-json", "-count=1", "-timeout=25m", "-p=1", "-parallel=1"}
		if def.Tag != "" {
			args = append(args, "-tags="+def.Tag)
		}
		if len(def.Tests) > 0 {
			args = append(args, "-run="+exactPattern(def.Tests))
		}
		args = append(args, def.Packages...)
		g := runGroup(ctx, opts.out, "go-owner", "", args, def.Tests, def.Packages, true)
		report.Groups = append(report.Groups, g)
		// Every selected package must actually execute tests. Patterns are expanded
		// by Go itself; package/test events remain the original evidence.
		if g.Error == "" {
			raw, err := os.ReadFile(g.StdoutLog)
			if err != nil {
				report.Errors = append(report.Errors, err.Error())
			} else {
				runs, terminals, packages, err := scanEvents(raw)
				if err != nil {
					report.Errors = append(report.Errors, err.Error())
				}
				seen := map[string]bool{}
				expectedPackages := map[string]bool{}
				module := strings.TrimSuffix(corePackage, "/internal/app/host")
				for _, pkg := range def.Packages {
					expectedPackages[module+"/"+strings.TrimPrefix(pkg, "./")] = true
				}
				for key, count := range runs {
					pkg, name, _ := strings.Cut(key, "\x00")
					if count != 1 || terminals[key] != 1 {
						report.Errors = append(report.Errors, "incomplete test "+key)
					}
					if !strings.Contains(name, "/") {
						seen[name] = true
					}
					if packages[pkg] != 1 {
						report.Errors = append(report.Errors, "missing package terminal "+pkg)
					}
				}
				for pkg, count := range packages {
					if !expectedPackages[pkg] {
						report.Errors = append(report.Errors, "unexpected package "+pkg)
					}
					n := 0
					for key := range runs {
						if strings.HasPrefix(key, pkg+"\x00") {
							n++
						}
					}
					if count != 1 || n == 0 {
						report.Errors = append(report.Errors, "empty or duplicate package "+pkg)
					}
				}
				for pkg := range expectedPackages {
					if packages[pkg] != 1 {
						report.Errors = append(report.Errors, "missing assigned package "+pkg)
					}
				}
				for _, name := range def.Tests {
					if !seen[name] {
						report.Errors = append(report.Errors, "missing assigned test "+name)
					}
				}
				if len(runs) == 0 {
					report.Errors = append(report.Errors, "no tests matched")
				}
			}
		}
	}
	report.ExecutionSecs = time.Since(started).Seconds()
	return finishLocalReport(opts.out, &report)
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

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/testfeedback"
)

type e2eScenario struct {
	ID      string `json:"id"`
	Level   string `json:"level"`
	Suite   string `json:"suite"`
	Package string `json:"package"`
	Test    string `json:"test"`
	Env     string `json:"env"`
}
type e2eReport struct {
	Scope       string            `json:"scope"`
	Level       string            `json:"level"`
	Env         string            `json:"env"`
	Environment environment       `json:"environment"`
	Selected    []e2eScenario     `json:"selected_scenarios"`
	Groups      []scopedGroup     `json:"groups"`
	Builds      map[string]string `json:"build_sha256"`
	Preparation float64           `json:"preparation_seconds"`
	Execution   float64           `json:"execution_seconds"`
	Passed      bool              `json:"passed"`
	Cleanup     bool              `json:"cleanup_complete"`
	Errors      []string          `json:"errors"`
	Started     string            `json:"started_at"`
	Finished    string            `json:"finished_at"`
}

func e2eRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("cannot locate repository go.mod")
		}
		dir = parent
	}
}
func selectE2E(root string, opts options) ([]e2eScenario, error) {
	raw, err := os.ReadFile(filepath.Join(root, "tests", "e2e", "suites.json"))
	if err != nil {
		return nil, err
	}
	var entries []e2eScenario
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, err
	}
	ids, tests := map[string]bool{}, map[string]bool{}
	requested := map[string]bool{}
	if opts.level == "L3" {
		for _, name := range strings.Split(opts.suite, ",") {
			if name == "" || requested[name] {
				return nil, fmt.Errorf("invalid or repeated suite %q", name)
			}
			requested[name] = true
		}
	}
	found := map[string]bool{}
	var selected []e2eScenario
	for _, entry := range entries {
		key := entry.Package + "/" + entry.Test
		if !regexp.MustCompile(`^[a-z0-9]+(?:[.-][a-z0-9]+)*$`).MatchString(entry.ID) || !regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`).MatchString(entry.Suite) || entry.Env != "controlled" || (entry.Level != "L2" && entry.Level != "L3") || !strings.HasPrefix(entry.Package, "./tests/e2e/") || strings.Contains(entry.Package, "..") || !regexp.MustCompile(`^Test[A-Za-z0-9_]+$`).MatchString(entry.Test) {
			return nil, fmt.Errorf("invalid E2E manifest entry %+v", entry)
		}
		if ids[entry.ID] || tests[key] {
			return nil, fmt.Errorf("duplicate E2E scenario %s", entry.ID)
		}
		ids[entry.ID], tests[key] = true, true
		if entry.Level == opts.level && entry.Env == opts.e2eEnv && (opts.level == "L2" || requested[entry.Suite]) {
			selected = append(selected, entry)
			found[entry.Suite] = true
		}
	}
	for name := range requested {
		if !found[name] {
			return nil, fmt.Errorf("unknown or unsupported E2E suite %q", name)
		}
	}
	if len(selected) == 0 {
		return nil, errors.New("no E2E scenarios selected")
	}
	return selected, nil
}

func e2eEnvironment(base []string, config string) []string {
	var result []string
	for _, v := range base {
		key := strings.ToUpper(strings.SplitN(v, "=", 2)[0])
		if strings.HasPrefix(key, "STEERLINE_") || key == "GOFLAGS" {
			continue
		}
		result = append(result, v)
	}
	return append(result, "STEERLINE_E2E_RUN="+config, "GOFLAGS=")
}

func runE2E(opts options) (code int) {
	root, err := e2eRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	selected, err := selectE2E(root, opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if runtime.GOOS != "windows" {
		fmt.Fprintln(os.Stderr, "E2E currently supports Windows only")
		return 2
	}
	out, err := filepath.Abs(opts.out)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	opts.out = out
	existing, err := os.ReadDir(out)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if len(existing) > 0 {
		fmt.Fprintln(os.Stderr, "E2E output directory must be empty; use a new -out path")
		return 2
	}
	report := e2eReport{Scope: "e2e", Level: opts.level, Env: opts.e2eEnv, Selected: selected, Builds: map[string]string{}, Errors: []string{}, Started: time.Now().UTC().Format(time.RFC3339Nano), Environment: collectEnvironment(opts.cacheCondition, "controlled worker and provider; no native certification", out, nil)}
	started := time.Now()
	scratch, err := os.MkdirTemp("", "steerline-e2e-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	defer func() {
		// scratch is the literal directory created by this invocation.
		if err := os.RemoveAll(scratch); err != nil {
			report.Errors = append(report.Errors, "scratch cleanup: "+err.Error())
		} else {
			report.Cleanup = true
		}
		report.Finished = time.Now().UTC().Format(time.RFC3339Nano)
		report.Passed = len(report.Errors) == 0
		if err := writeJSON(filepath.Join(out, "summary.json"), report); err != nil {
			fmt.Fprintln(os.Stderr, err)
			report.Passed = false
			report.Errors = append(report.Errors, "write report: "+err.Error())
			code = 2
		} else if !report.Passed {
			code = 1
		}
		fmt.Fprintf(os.Stderr, "%s E2E: passed=%v; report %s\n", opts.level, report.Passed, filepath.Join(out, "summary.json"))
		for _, e := range report.Errors {
			fmt.Fprintln(os.Stderr, e)
		}
	}()
	fail := func(err error) int { report.Errors = append(report.Errors, err.Error()); return 1 }
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	ctx, cancelTimeout := context.WithTimeout(ctx, 15*time.Minute)
	defer cancelTimeout()
	buildDir := filepath.Join(out, "build")
	if err := os.MkdirAll(buildDir, 0700); err != nil {
		return fail(err)
	}
	binary, helper := filepath.Join(buildDir, "steerline.exe"), filepath.Join(buildDir, "controlled-helper.exe")
	for _, build := range []struct{ path, pkg string }{{binary, "./cmd/steerline"}, {helper, "./tests/e2e/testdata/controlled-helper"}} {
		buildCtx, stop := context.WithTimeout(ctx, 3*time.Minute)
		cmd := exec.Command("go", "build", "-o", build.path, build.pkg)
		cmd.Dir = root
		cmd.Env = e2eEnvironment(os.Environ(), "")
		var logs strings.Builder
		cmd.Stdout, cmd.Stderr = &logs, &logs
		err := runOwned(buildCtx, cmd)
		stop()
		if logErr := os.WriteFile(filepath.Join(buildDir, filepath.Base(build.path)+".log"), []byte(logs.String()), 0600); logErr != nil {
			return fail(logErr)
		}
		if err != nil {
			return fail(fmt.Errorf("build %s: %w: %s", build.pkg, err, logs.String()))
		}
		raw, err := os.ReadFile(build.path)
		if err != nil {
			return fail(err)
		}
		sum := sha256.Sum256(raw)
		report.Builds[build.path] = hex.EncodeToString(sum[:])
	}
	config, err := json.Marshal(map[string]string{"Root": root, "Binary": binary, "Helper": helper, "Output": out, "Scratch": scratch})
	if err != nil {
		return fail(err)
	}
	packages := map[string][]e2eScenario{}
	for _, e := range selected {
		packages[e.Package] = append(packages[e.Package], e)
	}
	names := make([]string, 0, len(packages))
	for pkg := range packages {
		names = append(names, pkg)
	}
	sort.Strings(names)
	// Enumerate selected packages before any scenario can create resources.
	for _, pkg := range names {
		cmd := exec.CommandContext(ctx, "go", "test", "-tags=e2e", "-list=.", pkg)
		cmd.Dir = root
		cmd.Env = e2eEnvironment(os.Environ(), string(config))
		raw, err := cmd.CombinedOutput()
		if err != nil {
			return fail(fmt.Errorf("enumerate %s: %w: %s", pkg, err, raw))
		}
		actual := []string{}
		for _, line := range strings.Split(string(raw), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "Test") {
				actual = append(actual, line)
			}
		}
		expected := []string{}
		for _, e := range packages[pkg] {
			expected = append(expected, e.Test)
		}
		sort.Strings(actual)
		sort.Strings(expected)
		if strings.Join(actual, "\n") != strings.Join(expected, "\n") {
			return fail(fmt.Errorf("manifest differs from tests in %s: actual=%v expected=%v", pkg, actual, expected))
		}
	}
	report.Preparation = time.Since(started).Seconds()
	execution := time.Now()
	for i, pkg := range names {
		tests := []string{}
		for _, e := range packages[pkg] {
			tests = append(tests, e.Test)
		}
		args := []string{"test", "-tags=e2e", "-json", "-count=1", "-p=1", "-parallel=1", "-timeout=6m", "-run=" + exactPattern(tests), pkg}
		g := scopedGroup{ID: fmt.Sprintf("e2e-%d", i+1), Command: append([]string{"go"}, args...), Assigned: tests, Packages: []string{pkg}, StdoutLog: filepath.Join(out, fmt.Sprintf("e2e-%d.jsonl", i+1)), StderrLog: filepath.Join(out, fmt.Sprintf("e2e-%d.stderr.txt", i+1))}
		stdout, err := os.Create(g.StdoutLog)
		if err != nil {
			return fail(err)
		}
		stderr, err := os.Create(g.StderrLog)
		if err != nil {
			stdout.Close()
			return fail(err)
		}
		cmd := exec.Command("go", args...)
		cmd.Dir = root
		cmd.Env = e2eEnvironment(os.Environ(), string(config))
		cmd.Stdout, cmd.Stderr = stdout, stderr
		groupStarted := time.Now()
		err = runOwned(ctx, cmd)
		g.WallSeconds = time.Since(groupStarted).Seconds()
		closeErr := errors.Join(stdout.Close(), stderr.Close())
		if closeErr != nil {
			return fail(closeErr)
		}
		if err != nil {
			g.ExitCode = 1
			g.Error = err.Error()
		}
		raw, err := os.ReadFile(g.StdoutLog)
		if err != nil {
			return fail(err)
		}
		g.Summary = testfeedback.Parse(raw, g.ExitCode)
		report.Groups = append(report.Groups, g)
		if err := checkE2EGroup(g, packages[pkg], out); err != nil {
			report.Errors = append(report.Errors, err.Error())
		}
		if ctx.Err() != nil {
			break
		}
	}
	report.Execution = time.Since(execution).Seconds()
	if len(report.Groups) != len(names) {
		report.Errors = append(report.Errors, "not all selected packages completed")
	}
	return 0
}

func checkE2EGroup(g scopedGroup, entries []e2eScenario, out string) error {
	if g.ExitCode != 0 || !g.Summary.Passed || !g.Summary.Complete || g.Summary.SkippedTests > 0 || len(g.Summary.SkippedPackages) > 0 {
		return fmt.Errorf("%s: failed/incomplete/skipped tests", g.ID)
	}
	if len(g.Packages) != 1 || len(g.Summary.Packages) != 1 {
		return errors.New("E2E package completion differs from selection")
	}
	expectedPackage := g.Packages[0]
	if strings.HasPrefix(expectedPackage, "./") {
		expectedPackage = strings.TrimSuffix(corePackage, "/internal/steerline") + "/" + strings.TrimPrefix(expectedPackage, "./")
	}
	if g.Summary.Packages[0].Name != expectedPackage || g.Summary.Packages[0].Status != "pass" {
		return errors.New("unexpected E2E package result")
	}
	expected := map[string]bool{}
	for _, e := range entries {
		expected[e.Test] = true
	}
	seen := map[string]int{}
	for _, t := range g.Summary.Tests {
		if t.Package != expectedPackage {
			return fmt.Errorf("test %s belongs to unexpected package", t.Name)
		}
		if !strings.Contains(t.Name, "/") {
			if !expected[t.Name] || t.Status != "pass" {
				return fmt.Errorf("unexpected scenario result %s", t.Name)
			}
			seen[t.Name]++
		}
	}
	for _, e := range entries {
		if seen[e.Test] != 1 {
			return fmt.Errorf("scenario %s completed %d times", e.ID, seen[e.Test])
		}
		var evidence struct {
			ID        string           `json:"id"`
			Passed    bool             `json:"passed"`
			Cleanup   bool             `json:"cleanup_complete"`
			Resources []map[string]any `json:"resources"`
		}
		raw, err := os.ReadFile(filepath.Join(out, "scenarios", e.ID, "scenario.json"))
		if err != nil {
			return fmt.Errorf("missing evidence for %s: %w", e.ID, err)
		}
		if err = json.Unmarshal(raw, &evidence); err != nil || evidence.ID != e.ID || !evidence.Passed || !evidence.Cleanup || len(evidence.Resources) == 0 {
			return fmt.Errorf("invalid or incomplete evidence for %s", e.ID)
		}
	}
	return nil
}

package main

import (
	"bytes"
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
	"sync"
	"time"

	testfeedback "github.com/erranliu/SteerLine/internal/testkit/feedback"
)

type e2eScenario struct {
	ID      string `json:"id"`
	Level   string `json:"level"`
	Suite   string `json:"suite"`
	Package string `json:"package"`
	Test    string `json:"test"`
	Env     string `json:"env"`
	Chain   string `json:"chain,omitempty"`
}
type e2eReport struct {
	Scope          string            `json:"scope"`
	Level          string            `json:"level"`
	Env            string            `json:"env"`
	Environment    environment       `json:"environment"`
	Selected       []e2eScenario     `json:"selected_scenarios"`
	Groups         []scopedGroup     `json:"groups"`
	Builds         map[string]string `json:"build_sha256"`
	Preparation    float64           `json:"preparation_seconds"`
	Execution      float64           `json:"execution_seconds"`
	Passed         bool              `json:"passed"`
	Cleanup        bool              `json:"cleanup_complete"`
	Errors         []string          `json:"errors"`
	Started        string            `json:"started_at"`
	Finished       string            `json:"finished_at"`
	Blocked        []e2eScenario     `json:"blocked_scenarios,omitempty"`
	Coverage       string            `json:"coverage"`
	PackageWorkers int               `json:"package_workers"`
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
	packageEnvs := map[string]string{}
	requested := map[string]bool{}
	if opts.level == "L3" || opts.level == "L4" {
		for _, name := range strings.Split(opts.suite, ",") {
			if name == "" || requested[name] {
				return nil, fmt.Errorf("invalid or repeated suite %q", name)
			}
			requested[name] = true
		}
	}
	found := map[string]bool{}
	wantedIDs := map[string]bool{}
	if opts.scenario != "" {
		for _, id := range strings.Split(opts.scenario, ",") {
			if id == "" || wantedIDs[id] {
				return nil, fmt.Errorf("invalid or repeated scenario %q", id)
			}
			wantedIDs[id] = true
		}
	}
	foundIDs := map[string]bool{}
	var selected []e2eScenario
	for _, entry := range entries {
		key := entry.Package + "/" + entry.Test
		if !regexp.MustCompile(`^[a-z0-9]+(?:[.-][a-z0-9]+)*$`).MatchString(entry.ID) || !regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`).MatchString(entry.Suite) || (entry.Env != "controlled" && entry.Env != "live") || (entry.Env == "live" && !supportedLiveScenario(entry)) || (entry.Env == "controlled" && entry.Chain != "") || (entry.Level != "L2" && entry.Level != "L3" && entry.Level != "L4") || !strings.HasPrefix(entry.Package, "./tests/e2e/") || strings.Contains(entry.Package, "..") || !regexp.MustCompile(`^Test[A-Za-z0-9_]+$`).MatchString(entry.Test) {
			return nil, fmt.Errorf("invalid E2E manifest entry %+v", entry)
		}
		if ids[entry.ID] || tests[key] {
			return nil, fmt.Errorf("duplicate E2E scenario %s", entry.ID)
		}
		ids[entry.ID], tests[key] = true, true
		if env, ok := packageEnvs[entry.Package]; ok && env != entry.Env {
			return nil, errors.New("one E2E package cannot mix environments")
		}
		packageEnvs[entry.Package] = entry.Env
		if entry.Level == opts.level && entry.Env == opts.e2eEnv && (opts.level == "L2" || requested[entry.Suite]) {
			found[entry.Suite] = true
			if len(wantedIDs) > 0 && !wantedIDs[entry.ID] {
				continue
			}
			selected = append(selected, entry)
			foundIDs[entry.ID] = true
		}
	}
	for id := range wantedIDs {
		if !foundIDs[id] {
			return nil, fmt.Errorf("scenario %q does not match selected level/suite/environment", id)
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
	executionModel, auditModel := "", ""
	for _, entry := range selected {
		if entry.ID == "workflow.local-codex-audited" {
			executionModel, auditModel = strings.TrimSpace(os.Getenv("STEERLINE_AUDIT_EXECUTION_MODEL")), strings.TrimSpace(os.Getenv("STEERLINE_AUDIT_MODEL"))
			if executionModel == "" || auditModel == "" || executionModel == auditModel || os.Getenv("STEERRUNNER_EXE") == "" {
				fmt.Fprintln(os.Stderr, "audited live requires explicit STEERRUNNER_EXE and distinct STEERLINE_AUDIT_EXECUTION_MODEL / STEERLINE_AUDIT_MODEL before fixture creation")
				return 2
			}
		}
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
	note, coverage := "controlled worker/provider; no model requests", "selected-"+opts.level
	if opts.level == "L2" {
		coverage = "full-L2"
	}
	if opts.e2eEnv == "live" {
		note = "real Runner/local Codex; isolated test resources and explicit external effects"
	}

	report := e2eReport{Scope: "e2e", Coverage: coverage, Level: opts.level, Env: opts.e2eEnv, Selected: selected, Builds: map[string]string{}, Errors: []string{}, Started: time.Now().UTC().Format(time.RFC3339Nano), Environment: collectEnvironment(opts.cacheCondition, note, out, nil)}
	started := time.Now()
	scratch, err := os.MkdirTemp("", "steerline-e2e-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	defer func() {
		// scratch is the literal directory created by this invocation.
		preserve := false
		for _, entry := range selected {
			var state struct {
				Cleanup bool `json:"cleanup_complete"`
			}
			raw, err := os.ReadFile(filepath.Join(out, "scenarios", entry.ID, "scenario.json"))
			if err == nil {
				if json.Unmarshal(raw, &state) != nil || !state.Cleanup {
					preserve = true
				}
			} else if len(report.Groups) > 0 && len(report.Blocked) == 0 {
				preserve = true
			}
		}
		if preserve {
			report.Errors = append(report.Errors, "unverified cleanup; preserve scratch: "+scratch)
		} else if err := os.RemoveAll(scratch); err != nil {
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
	// The parent must include every selected scene's existing body/cleanup
	// bound, as well as independent builds; it must not clip the final scene.
	budget := 10 * time.Minute
	for _, entry := range selected {
		window, err := time.ParseDuration(e2ePackageGoTimeout([]e2eScenario{entry}))
		if err != nil {
			return fail(err)
		}
		budget += window
	}
	if budget < 45*time.Minute {
		budget = 45 * time.Minute
	}
	ctx, cancelTimeout := context.WithTimeout(ctx, budget)
	defer cancelTimeout()
	buildDir := filepath.Join(out, "build")
	if err := os.MkdirAll(buildDir, 0700); err != nil {
		return fail(err)
	}
	sourceCommit, err := e2eCommandOutput(root, "git", "rev-parse", "HEAD")
	if err != nil {
		return fail(fmt.Errorf("read E2E source commit: %w", err))
	}
	status, err := e2eCommandOutput(root, "git", "status", "--porcelain")
	if err != nil {
		return fail(fmt.Errorf("read E2E source status: %w", err))
	}
	sourceDirty := "false"
	if strings.TrimSpace(status) != "" {
		sourceDirty = "true"
	}
	binary, helper := filepath.Join(buildDir, "steerline.exe"), filepath.Join(buildDir, "controlled-helper.exe")
	for _, build := range []struct{ path, pkg string }{{binary, "./cmd/steerline"}, {helper, "./tests/e2e/testdata/controlled-helper"}} {
		buildCtx, stop := context.WithTimeout(ctx, 3*time.Minute)
		args := []string{"build"}
		if build.pkg == "./cmd/steerline" {
			ldflags := "-X github.com/erranliu/SteerLine/internal/app.buildSourceCommit=" + sourceCommit + " -X github.com/erranliu/SteerLine/internal/app.buildSourceDirty=" + sourceDirty
			args = append(args, "-ldflags", ldflags)
		}
		args = append(args, "-o", build.path, build.pkg)
		cmd := exec.Command("go", args...)
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
	runnerPath := helper
	hasLive := false
	for _, entry := range selected {
		hasLive = hasLive || entry.Env == "live"
	}
	if hasLive {
		runnerPath = os.Getenv("STEERRUNNER_EXE")
		if runnerPath == "" {
			runnerPath, err = exec.LookPath("steerrunner.exe")
			if err != nil {
				return fail(errors.New("select a standalone Runner with STEERRUNNER_EXE"))
			}
		}
		runnerPath, err = filepath.Abs(runnerPath)
		if err != nil {
			return fail(err)
		}
		runnerBytes, err := os.ReadFile(runnerPath)
		if err != nil {
			return fail(err)
		}
		runnerSum := sha256.Sum256(runnerBytes)
		report.Builds[runnerPath] = hex.EncodeToString(runnerSum[:])

	}
	authHome := os.Getenv("CODEX_HOME")
	if authHome == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return fail(err)
		}
		authHome = filepath.Join(userHome, ".codex")
	}
	authHome, err = filepath.Abs(authHome)
	if err != nil {
		return fail(err)
	}
	config, err := json.Marshal(map[string]string{"ExecutionModel": executionModel, "AuditModel": auditModel, "Root": root, "Binary": binary, "Helper": helper, "Runner": runnerPath, "Output": out, "Scratch": scratch, "AuthHome": authHome, "Level": opts.level, "SourceCommit": strings.TrimSpace(sourceCommit), "SourceDirty": sourceDirty})
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
	sort.Slice(names, func(i, j int) bool {
		if packages[names[i]][0].Env != packages[names[j]][0].Env {
			return packages[names[i]][0].Env == "controlled"
		}
		return names[i] < names[j]
	})
	// Freeze every selected test binary before resource creation. Later packages
	// must not silently compile a newer checkout halfway through a long run.
	testBinaries := map[string]string{}
	for index, pkg := range names {
		testBinary := filepath.Join(buildDir, fmt.Sprintf("scenario-%d.test.exe", index+1))
		build := exec.CommandContext(ctx, "go", "test", "-tags=e2e", "-c", "-o", testBinary, pkg)
		build.Dir = root
		build.Env = e2eEnvironment(os.Environ(), string(config))
		if output, err := build.CombinedOutput(); err != nil {
			return fail(fmt.Errorf("compile %s: %w: %s", pkg, err, output))
		}
		compiled, err := os.ReadFile(testBinary)
		if err != nil {
			return fail(err)
		}
		sum := sha256.Sum256(compiled)
		report.Builds[testBinary] = hex.EncodeToString(sum[:])
		testBinaries[pkg] = testBinary
		cmd := exec.CommandContext(ctx, testBinary, "-test.list=.")
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
		var manifest []e2eScenario
		manifestRaw, err := os.ReadFile(filepath.Join(root, "tests", "e2e", "suites.json"))
		if err != nil {
			return fail(err)
		}
		if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
			return fail(err)
		}
		expected := []string{}
		for _, e := range manifest {
			if e.Package == pkg {
				expected = append(expected, e.Test)
			}
		}
		sort.Strings(actual)
		sort.Strings(expected)
		if strings.Join(actual, "\n") != strings.Join(expected, "\n") {
			return fail(fmt.Errorf("manifest differs from tests in %s: actual=%v expected=%v", pkg, actual, expected))
		}
	}
	if err := writeJSON(filepath.Join(buildDir, "frozen-binaries.json"), map[string]any{"source_commit": strings.TrimSpace(sourceCommit), "source_dirty": sourceDirty, "sha256": report.Builds}); err != nil {
		return fail(err)
	}
	report.Preparation = time.Since(started).Seconds()
	execution := time.Now()
	// Each callback owns one slot; publish ordered groups only after all workers
	// have joined, so concurrent packages never mutate the shared report slice.
	groups := make([]*scopedGroup, len(names))
	packageWorkers := 1
	if opts.level == "L3" && len(names) > 1 {
		packageWorkers = 2
	}
	report.PackageWorkers = packageWorkers
	report.Errors, report.Blocked = executeE2EPackages(ctx, names, packages, packageWorkers, func(i int, pkg string) error {
		tests := []string{}
		for _, e := range packages[pkg] {
			tests = append(tests, e.Test)
		}
		timeout := e2ePackageGoTimeout(packages[pkg])
		importPath := strings.TrimSuffix(corePackage, "/internal/app/host") + "/" + strings.TrimPrefix(pkg, "./")
		parallelism := e2ePackageParallelism(packages[pkg])
		if packageWorkers > 1 {
			parallelism = 1
		}
		args := []string{"-test.v=test2json", "-test.count=1", fmt.Sprintf("-test.parallel=%d", parallelism), "-test.timeout=" + timeout, "-test.run=" + exactPattern(tests)}
		g := scopedGroup{ID: fmt.Sprintf("e2e-%d", i+1), Command: append([]string{testBinaries[pkg]}, args...), Assigned: tests, Packages: []string{pkg}, StdoutLog: filepath.Join(out, fmt.Sprintf("e2e-%d.jsonl", i+1)), StderrLog: filepath.Join(out, fmt.Sprintf("e2e-%d.stderr.txt", i+1))}
		// Own the test binary directly. Detached external services must not keep
		// a live test2json pipe open after the test process has exited.
		rawLog := filepath.Join(out, g.ID+".raw.log")
		stdout, err := os.Create(rawLog)
		if err != nil {
			return err
		}
		stderr, err := os.Create(g.StderrLog)
		if err != nil {
			stdout.Close()
			return err
		}
		cmd := exec.Command(testBinaries[pkg], args...)
		cmd.Dir = filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(pkg, "./")))
		cmd.Env = e2eEnvironment(os.Environ(), string(config))
		cmd.Stdout, cmd.Stderr = stdout, stderr
		groupStarted := time.Now()
		g.StartedAt = groupStarted.UTC().Format(time.RFC3339Nano)
		err = runOwned(ctx, cmd)
		groupFinished := time.Now()
		g.FinishedAt = groupFinished.UTC().Format(time.RFC3339Nano)
		g.WallSeconds = groupFinished.Sub(groupStarted).Seconds()
		closeErr := errors.Join(stdout.Close(), stderr.Close())
		if closeErr != nil {
			return closeErr
		}
		if err != nil {
			g.ExitCode = 1
			g.Error = err.Error()
		}
		raw, err := os.ReadFile(rawLog)
		if err != nil {
			return err
		}
		convert := exec.CommandContext(ctx, "go", "tool", "test2json", "-t", "-p", importPath)
		convert.Stdin = bytes.NewReader(raw)
		converted, err := convert.Output()
		if err != nil {
			return fmt.Errorf("convert finite test log: %w", err)
		}
		if err := os.WriteFile(g.StdoutLog, converted, 0600); err != nil {
			return err
		}
		raw = converted
		g.Summary = testfeedback.Parse(raw, g.ExitCode)
		groups[i] = &g
		return checkE2EGroup(g, packages[pkg], out)
	})
	for _, g := range groups {
		if g != nil {
			report.Groups = append(report.Groups, *g)
		}
	}
	report.Execution = time.Since(execution).Seconds()
	for _, entry := range selected {
		if _, err := os.Stat(filepath.Join(out, "scenarios", entry.ID, "blocker.json")); err == nil {
			report.Blocked = append(report.Blocked, entry)
		}
	}
	if len(report.Groups) != len(names) {
		report.Errors = append(report.Errors, "not all selected packages completed")
	}
	return 0
}

// Only the audited runtime scenes opt into bounded Go test parallelism. Other
// suites retain their existing sequencing. Shared Web builders always serialize.
func e2ePackageParallelism(entries []e2eScenario) int {
	if len(entries) < 2 {
		return 1
	}
	for _, entry := range entries {
		if entry.Env != "controlled" || entry.Level != entries[0].Level || entry.Package != "./tests/e2e/suites/runtime" {
			return 1
		}
		switch entry.ID {
		case "runtime.service-lifecycle", "runtime.dual-home-operation", "runtime.shared-runner-projects":
			if entry.Level != "L3" {
				return 1
			}
		case "runtime.source-build-isolation", "runtime.path-isolation", "runtime.service-write-isolation", "runtime.dual-home-isolation":
			if entry.Level != "L4" {
				return 1
			}
		default:
			return 1
		}
	}
	return 2
}

func e2ePackageGoTimeout(entries []e2eScenario) string {
	if len(entries) > 0 && entries[0].Env == "live" {
		return "35m"
	}
	minutes := 0
	for _, entry := range entries {
		// Ordinary controlled/native business is 5m plus independent cleanup.
		// Heavy isolation/native restart has a 10m body; its parent must not
		// cut the selected scene at the ordinary 6m package bound.
		if entry.ID == "recovery.waiting-merge-restart" || entry.ID == "runtime.project-maintenance" {
			minutes += 12
		} else {
			minutes += 6
		}
	}
	return fmt.Sprintf("%dm", minutes)
}

func e2eCommandOutput(dir, executable string, args ...string) (string, error) {
	cmd := exec.Command(executable, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s %v: %w: %s", executable, args, err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
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
		expectedPackage = strings.TrimSuffix(corePackage, "/internal/app/host") + "/" + strings.TrimPrefix(expectedPackage, "./")
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
			ID        string                            `json:"id"`
			Passed    bool                              `json:"passed"`
			Cleanup   bool                              `json:"cleanup_complete"`
			Resources []map[string]any                  `json:"resources"`
			Live      *testfeedback.LiveDeliveryProof   `json:"live_delivery"`
			Testing   *testfeedback.TestManagementProof `json:"test_management"`
		}
		raw, err := os.ReadFile(filepath.Join(out, "scenarios", e.ID, "scenario.json"))
		if err != nil {
			return fmt.Errorf("missing evidence for %s: %w", e.ID, err)
		}
		if err = json.Unmarshal(raw, &evidence); err != nil || evidence.ID != e.ID || !evidence.Passed || !evidence.Cleanup || len(evidence.Resources) == 0 {
			return fmt.Errorf("invalid or incomplete evidence for %s", e.ID)
		}
		if e.ID == "testing.management-local-codex" {
			if evidence.Testing == nil {
				return errors.New("missing test management proof")
			}
			if err := evidence.Testing.Validate(); err != nil {
				return err
			}
			continue
		}
		if e.Env == "live" {
			if !supportedLiveScenario(e) || evidence.Live == nil || e.ID == "workflow.local-codex-audited" && evidence.Live.Audited == nil {
				return fmt.Errorf("unsupported live chain or missing delivery proof: %s", e.ID)
			}
			if err := evidence.Live.Validate(); err != nil {
				return fmt.Errorf("live delivery %s: %w", e.ID, err)
			}
		}

	}
	return nil
}

func supportedLiveScenario(e e2eScenario) bool {
	return e.Chain == "local-codex" && (e.ID == "testing.management-local-codex" && e.Level == "L4" && e.Suite == "testing" || e.ID == "smoke.local-codex" && e.Level == "L4" || e.ID == "workflow.local-codex-audited" && e.Level == "L4" && e.Suite == "workflow")
}

func executeE2EPackages(ctx context.Context, names []string, packages map[string][]e2eScenario, workers int, execute func(int, string) error) ([]string, []e2eScenario) {
	var failures []string
	var blocked []e2eScenario
	if workers > 1 {
		results := make([]error, len(names))
		started := make([]bool, len(names))
		jobs := make(chan int)
		var wg sync.WaitGroup
		for w := 0; w < workers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := range jobs {
					if ctx.Err() != nil {
						continue
					}
					started[i] = true
					results[i] = execute(i, names[i])
				}
			}()
		}
		for i := range names {
			jobs <- i
		}
		close(jobs)
		wg.Wait()
		for i, pkg := range names {
			if !started[i] {
				blocked = append(blocked, packages[pkg]...)
			} else if results[i] != nil {
				failures = append(failures, results[i].Error())
			}
		}
		if ctx.Err() != nil {
			failures = append(failures, ctx.Err().Error())
		}
		return failures, blocked
	}
	for i, pkg := range names {
		if ctx.Err() != nil {
			for _, remaining := range names[i:] {
				blocked = append(blocked, packages[remaining]...)
			}
			if ctx.Err() != nil {
				failures = append(failures, ctx.Err().Error())
			}
			break
		}
		if err := execute(i, pkg); err != nil {
			failures = append(failures, err.Error())
		}
	}
	return failures, blocked
}

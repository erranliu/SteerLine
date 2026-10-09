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
	Blocked     []e2eScenario     `json:"blocked_scenarios,omitempty"`
	Coverage    string            `json:"coverage"`
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
	if opts.level == "L3" {
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
		if !regexp.MustCompile(`^[a-z0-9]+(?:[.-][a-z0-9]+)*$`).MatchString(entry.ID) || !regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`).MatchString(entry.Suite) || (entry.Env != "controlled" && entry.Env != "live" && entry.Env != "native") || (entry.Env == "live" && entry.Chain != "local-codex" && entry.Chain != "orca-codex" && entry.Chain != "orca-remote-codex") || (entry.Env == "native" && (entry.Level != "L3" || entry.Chain != "")) || (entry.Level != "L2" && entry.Level != "L3") || !strings.HasPrefix(entry.Package, "./tests/e2e/") || strings.Contains(entry.Package, "..") || !regexp.MustCompile(`^Test[A-Za-z0-9_]+$`).MatchString(entry.Test) {
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
		if entry.Level == opts.level && (entry.Env == opts.e2eEnv || opts.e2eEnv == "gated") && (opts.level == "L2" || requested[entry.Suite]) {
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
	if opts.e2eEnv == "gated" {
		chains := map[string]bool{}
		for _, e := range selected {
			if e.Env == "controlled" {
				chains["controlled"] = true
			} else {
				chains[e.Chain] = true
			}
		}
		if !chains["controlled"] || !chains["local-codex"] || !chains["orca-codex"] {
			return nil, errors.New("L2 requires controlled gate and both live chains")
		}
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
	note, coverage := "controlled worker/provider; no model requests", "selected-L3"
	if opts.e2eEnv == "native" {
		note = "native Orca workspace operations; controlled worker/provider; no model requests"
	}
	if opts.e2eEnv == "live" {
		note = "native gpt-6-luna/medium; loopback provider"
		for _, scenario := range selected {
			if scenario.Chain == "orca-remote-codex" {
				note = "native gpt-6-luna/medium; selected providers; remote Orca uses real SteerlineTest PRs"
				break
			}
		}
		if opts.suite == "candidates" {
			note = "native gpt-6-luna/medium; exact multi-profile admission; real SteerlineTest PRs"
		}
	}
	if opts.level == "L2" {
		coverage = "gate-only"
	}
	if opts.e2eEnv == "gated" {
		note = "controlled gate, then native gpt-6-luna/medium on both chains with real SteerlineTest GitHub PRs"
		coverage = "full-L2"
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
	ctx, cancelTimeout := context.WithTimeout(ctx, 45*time.Minute)
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
	candidateConfig := os.Getenv("STEERLINE_CANDIDATE_ACCEPTANCE_CONFIG")
	if candidateConfig != "" {
		candidateConfig, err = filepath.Abs(candidateConfig)
		if err != nil {
			return fail(err)
		}
	}
	remoteConfig := os.Getenv("STEERLINE_REMOTE_ACCEPTANCE_CONFIG")
	if remoteConfig != "" {
		remoteConfig, err = filepath.Abs(remoteConfig)
		if err != nil {
			return fail(err)
		}
	}
	config, err := json.Marshal(map[string]string{"Root": root, "Binary": binary, "Helper": helper, "Output": out, "Scratch": scratch, "AuthHome": authHome, "Level": opts.level, "CandidateConfig": candidateConfig, "RemoteConfig": remoteConfig, "SourceCommit": strings.TrimSpace(sourceCommit), "SourceDirty": sourceDirty})
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
	report.Preparation = time.Since(started).Seconds()
	execution := time.Now()
	report.Errors, report.Blocked = executeE2EPackages(ctx, names, packages, opts.e2eEnv == "gated", func(i int, pkg string) error {
		tests := []string{}
		for _, e := range packages[pkg] {
			tests = append(tests, e.Test)
		}
		// Each controlled scenario owns a 5m business deadline and cleanup.
		// A package selecting several independent scenarios needs their sum.
		timeout := fmt.Sprintf("%dm", 6*len(packages[pkg]))
		if packages[pkg][0].Env == "live" {
			timeout = "35m"
		}
		args := []string{"test", "-tags=e2e", "-json", "-count=1", "-p=1", "-parallel=1", "-timeout=" + timeout, "-run=" + exactPattern(tests), pkg}
		g := scopedGroup{ID: fmt.Sprintf("e2e-%d", i+1), Command: append([]string{"go"}, args...), Assigned: tests, Packages: []string{pkg}, StdoutLog: filepath.Join(out, fmt.Sprintf("e2e-%d.jsonl", i+1)), StderrLog: filepath.Join(out, fmt.Sprintf("e2e-%d.stderr.txt", i+1))}
		stdout, err := os.Create(g.StdoutLog)
		if err != nil {
			return err
		}
		stderr, err := os.Create(g.StderrLog)
		if err != nil {
			stdout.Close()
			return err
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
			return closeErr
		}
		if err != nil {
			g.ExitCode = 1
			g.Error = err.Error()
		}
		raw, err := os.ReadFile(g.StdoutLog)
		if err != nil {
			return err
		}
		g.Summary = testfeedback.Parse(raw, g.ExitCode)
		report.Groups = append(report.Groups, g)
		return checkE2EGroup(g, packages[pkg], out)
	})
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
			ID                         string           `json:"id"`
			Passed                     bool             `json:"passed"`
			Cleanup                    bool             `json:"cleanup_complete"`
			Resources                  []map[string]any `json:"resources"`
			RemotePublicOutputVerified bool             `json:"remote_public_output_verified"`
			RemoteUsageStatus          string           `json:"remote_usage_status"`
			UsageVerified              bool             `json:"usage_verified"`
			OrcaUsageStatus            string           `json:"orca_usage_status"`
			NativeWorkspaceVerified    bool             `json:"native_workspace_verified"`
			RealPR                     *struct {
				URL, State, Repository string
				Merged                 bool
			} `json:"real_pr,omitempty"`
		}
		raw, err := os.ReadFile(filepath.Join(out, "scenarios", e.ID, "scenario.json"))
		if err != nil {
			return fmt.Errorf("missing evidence for %s: %w", e.ID, err)
		}
		if err = json.Unmarshal(raw, &evidence); err != nil || evidence.ID != e.ID || !evidence.Passed || !evidence.Cleanup || len(evidence.Resources) == 0 {
			return fmt.Errorf("invalid or incomplete evidence for %s", e.ID)
		}
		// Task 84's L3 native Orca acceptance records the public interface's
		// unavailable usage capability. Keep other suites and L2's gate intact.
		orcaPublicUsage := e.Env == "live" && e.Level == "L3" && e.Chain == "orca-codex" && (e.Suite == "delivery" || e.Suite == "recovery")
		if orcaPublicUsage && (evidence.UsageVerified || evidence.OrcaUsageStatus != "not_exposed_by_orca") {
			return fmt.Errorf("native Orca public usage capability missing or token verification claimed for %s", e.ID)
		}
		if e.Env == "live" && e.Chain != "orca-remote-codex" && !orcaPublicUsage && !evidence.UsageVerified {
			return fmt.Errorf("native token usage not verified for %s", e.ID)
		}
		if e.Chain == "orca-remote-codex" && (!evidence.RemotePublicOutputVerified || evidence.UsageVerified || evidence.RemoteUsageStatus != "not_exposed_by_orca") {
			return fmt.Errorf("remote public output evidence missing or private usage claimed for %s", e.ID)
		}
		if e.Env == "native" && (!evidence.NativeWorkspaceVerified || evidence.UsageVerified || evidence.RealPR != nil) {
			return fmt.Errorf("native no-model workspace evidence missing or mixed with model/PR evidence for %s", e.ID)
		}
		if (e.Level == "L2" && e.Env == "live" || e.Chain == "orca-remote-codex") && (evidence.RealPR == nil || !evidence.RealPR.Merged || evidence.RealPR.State != "merged" || evidence.RealPR.Repository != "erranliu/SteerlineTest" || !regexp.MustCompile(`^https://github.com/erranliu/SteerlineTest/pull/[1-9][0-9]*$`).MatchString(evidence.RealPR.URL)) {
			return fmt.Errorf("real merged SteerlineTest PR evidence missing for %s", e.ID)
		}
	}
	return nil
}

// The gate includes execution, evidence validation and cleanup; even a cleanup
// failure prevents the callback from ever constructing a native environment.
func executeE2EPackages(ctx context.Context, names []string, packages map[string][]e2eScenario, gated bool, execute func(int, string) error) ([]string, []e2eScenario) {
	var failures []string
	var blocked []e2eScenario
	for i, pkg := range names {
		if ctx.Err() != nil || (gated && packages[pkg][0].Env == "live" && len(failures) > 0) {
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

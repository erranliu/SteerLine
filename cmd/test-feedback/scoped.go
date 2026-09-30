package main

import (
	"bufio"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/erranliu/SteerLine/internal/testfeedback"
)

const corePackage = "github.com/erranliu/SteerLine/internal/steerline"

var quickTests = []string{"TestWorkerFlowClaimNeedsNoProjectSteps", "TestWorkerFlowProviderDeliveryRejectsWrongPRAndClosedUnmerged"}
var quickPackages = []string{corePackage, corePackage + "/taskgraph", corePackage + "/taskview", corePackage + "/workerflow", corePackage + "/orca_remote_worker"}

//go:embed testdata/core-test-weights.json
var weightFile embed.FS

type scopedGroup struct {
	ID          string               `json:"id"`
	Command     []string             `json:"command"`
	Assigned    []string             `json:"assigned_tests,omitempty"`
	Packages    []string             `json:"assigned_packages,omitempty"`
	StdoutLog   string               `json:"stdout_json_log"`
	StderrLog   string               `json:"stderr_log"`
	WallSeconds float64              `json:"wall_seconds"`
	ExitCode    int                  `json:"exit_code"`
	Error       string               `json:"error,omitempty"`
	Summary     testfeedback.Summary `json:"summary"`
}

type scopedReport struct {
	SchemaVersion   int           `json:"schema_version"`
	Scope           string        `json:"scope"`
	Phase           string        `json:"phase"`
	StartedAt       string        `json:"started_at"`
	FinishedAt      string        `json:"finished_at"`
	PreparationSecs float64       `json:"preparation_seconds"`
	ExecutionSecs   float64       `json:"execution_seconds"`
	Environment     environment   `json:"environment"`
	CoreTests       []string      `json:"enumerated_core_tests,omitempty"`
	AllPackages     []string      `json:"enumerated_packages"`
	WeightSource    string        `json:"weight_source,omitempty"`
	Groups          []scopedGroup `json:"groups"`
	Complete        bool          `json:"complete"`
	Passed          bool          `json:"passed"`
	Errors          []string      `json:"errors"`
}

func runScoped(opts options) int {
	if opts.workers == 3 {
		if switches := activeRealSwitches(); len(switches) > 0 {
			fmt.Fprintf(os.Stderr, "parallel full suite refuses native acceptance switches %s; use -workers=1\n", strings.Join(switches, ", "))
			return 2
		}
	}
	started := time.Now()
	report := scopedReport{SchemaVersion: 1, Phase: opts.phase, StartedAt: started.UTC().Format(time.RFC3339Nano), Complete: true, Errors: []string{}, Groups: []scopedGroup{}}
	if opts.quick {
		report.Scope = "quick"
	} else {
		report.Scope = "full"
		report.WeightSource = "testdata/core-test-weights.json (serial baseline 543f0d4; top-level elapsed, floor 0.05s)"
	}
	report.Environment = collectEnvironment(opts.cacheCondition, opts.nativeNote, opts.out, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Minute)
	defer cancel()
	packages, err := listPackages(ctx)
	if err != nil {
		return scopedSetupError(opts.out, &report, fmt.Errorf("list packages: %w", err))
	}
	if opts.quick {
		report.AllPackages = append([]string(nil), quickPackages...)
	} else {
		report.AllPackages = packages
	}
	var binary, coreDir string
	if opts.workers == 3 {
		binary, coreDir, err = prepareCore(ctx, opts.out)
		if err != nil {
			return scopedSetupError(opts.out, &report, err)
		}
		report.CoreTests, err = listCoreTests(ctx, binary, coreDir)
		if err != nil {
			return scopedSetupError(opts.out, &report, err)
		}
	}
	report.PreparationSecs = time.Since(started).Seconds()
	execution := time.Now()
	if opts.quick {
		report.CoreTests = append([]string(nil), quickTests...)
		args := []string{"go", "test", "-json", "-count=1", "-timeout=45m", "-p", "1", "-parallel", "1", "-run=" + exactPattern(quickTests), corePackage}
		report.Groups = append(report.Groups, runGroup(ctx, opts.out, "quick-core", "", args, quickTests, []string{corePackage}, false))
		args = append([]string{"go", "test", "-json", "-count=1", "-timeout=45m", "-p", "1", "-parallel", "1"}, quickPackages[1:]...)
		report.Groups = append(report.Groups, runGroup(ctx, opts.out, "quick-rules", "", args, nil, quickPackages[1:], false))
		if len(opts.goTestArgs) > 0 {
			args = append([]string{"go", "test"}, opts.goTestArgs...)
			if !hasJSONFlag(args) {
				args = append(args, "-json")
			}
			if flagValue(args, "-count", "") == "" {
				args = append(args, "-count=1")
			}
			report.Groups = append(report.Groups, runGroup(ctx, opts.out, "quick-targeted", "", args, nil, nil, false))
		}
	} else {
		weights, e := loadWeights()
		if e != nil {
			return scopedSetupError(opts.out, &report, e)
		}
		assignments := assignTests(report.CoreTests, weights, 3)
		groups := make([]scopedGroup, 3)
		var wg sync.WaitGroup
		for i := range groups {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				id := fmt.Sprintf("core-%d", i+1)
				args := []string{"go", "tool", "test2json", "-t", "-p", corePackage, binary, "-test.v=test2json", "-test.count=1", "-test.parallel=1", "-test.timeout=45m", "-test.run=" + exactPattern(assignments[i])}
				groups[i] = runGroup(ctx, opts.out, id, coreDir, args, assignments[i], []string{corePackage}, true)
			}(i)
		}
		wg.Wait()
		report.Groups = append(report.Groups, groups...)
		others := make([]string, 0, len(packages)-1)
		for _, pkg := range packages {
			if pkg != corePackage {
				others = append(others, pkg)
			}
		}
		if len(others) > 0 {
			args := append([]string{"go", "test", "-json", "-count=1", "-timeout=45m", "-p", "1", "-parallel", "1"}, others...)
			report.Groups = append(report.Groups, runGroup(ctx, opts.out, "other-packages", "", args, nil, others, false))
		}
	}
	report.ExecutionSecs = time.Since(execution).Seconds()
	report.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	report.Errors = append(report.Errors, checkScopedCoverage(report)...)
	report.Complete = len(report.Errors) == 0
	report.Passed = report.Complete
	if err := writeJSON(filepath.Join(opts.out, "summary.json"), report); err != nil {
		fmt.Fprintf(os.Stderr, "write scoped report: %v\n", err)
		return 2
	}
	fmt.Fprintf(os.Stderr, "%s test feedback: passed=%v, preparation=%.1fs, execution=%.1fs; report %s\n", report.Scope, report.Passed, report.PreparationSecs, report.ExecutionSecs, filepath.Join(opts.out, "summary.json"))
	for _, reason := range report.Errors {
		fmt.Fprintln(os.Stderr, "incomplete:", reason)
	}
	if !report.Passed {
		return 1
	}
	return 0
}

func scopedSetupError(out string, report *scopedReport, err error) int {
	report.Errors = append(report.Errors, err.Error())
	report.Complete = false
	report.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	_ = writeJSON(filepath.Join(out, "summary.json"), report)
	fmt.Fprintln(os.Stderr, err)
	return 2
}

func listPackages(ctx context.Context) ([]string, error) {
	output, err := exec.CommandContext(ctx, "go", "list", "./...").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("go list: %w: %s", err, output)
	}
	packages := strings.Fields(string(output))
	if len(packages) == 0 {
		return nil, fmt.Errorf("go list returned no packages")
	}
	found := false
	for _, pkg := range packages {
		if pkg == corePackage {
			found = true
		}
	}
	if !found {
		return nil, fmt.Errorf("core package missing from go list")
	}
	return packages, nil
}

func prepareCore(ctx context.Context, out string) (string, string, error) {
	dirBytes, err := exec.CommandContext(ctx, "go", "list", "-f", "{{.Dir}}", "./internal/steerline").CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("locate core: %w: %s", err, dirBytes)
	}
	dir := strings.TrimSpace(string(dirBytes))
	binary, err := filepath.Abs(filepath.Join(out, "core.test"))
	if err != nil {
		return "", "", err
	}
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	output, err := exec.CommandContext(ctx, "go", "test", "-c", "-o", binary, "./internal/steerline").CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("compile core: %w: %s", err, output)
	}
	return binary, dir, nil
}

func listCoreTests(ctx context.Context, binary, dir string) ([]string, error) {
	cmd := exec.CommandContext(ctx, binary, "-test.list=.")
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("list core tests: %w: %s", err, output)
	}
	var names []string
	seen := map[string]bool{}
	for _, line := range strings.Split(string(output), "\n") {
		name := strings.TrimSpace(line)
		if !strings.HasPrefix(name, "Test") && !strings.HasPrefix(name, "Example") && !strings.HasPrefix(name, "Fuzz") {
			continue
		}
		if seen[name] {
			return nil, fmt.Errorf("duplicate enumerated core test %s", name)
		}
		seen[name] = true
		names = append(names, name)
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("no core tests enumerated")
	}
	sort.Strings(names)
	return names, nil
}

func loadWeights() (map[string]float64, error) {
	data, err := weightFile.ReadFile("testdata/core-test-weights.json")
	if err != nil {
		return nil, err
	}
	var weights map[string]float64
	if err := json.Unmarshal(data, &weights); err != nil {
		return nil, err
	}
	return weights, nil
}

func assignTests(names []string, weights map[string]float64, workers int) [][]string {
	ordered := append([]string(nil), names...)
	weight := func(name string) float64 {
		w, ok := weights[name]
		if !ok {
			return 1
		}
		if w < .05 {
			return .05
		}
		return w
	}
	sort.Slice(ordered, func(i, j int) bool {
		a, b := weight(ordered[i]), weight(ordered[j])
		if a == b {
			return ordered[i] < ordered[j]
		}
		return a > b
	})
	groups := make([][]string, workers)
	loads := make([]float64, workers)
	for _, name := range ordered {
		best := 0
		for i := 1; i < workers; i++ {
			if loads[i] < loads[best] {
				best = i
			}
		}
		groups[best] = append(groups[best], name)
		loads[best] += weight(name)
	}
	return groups
}

func exactPattern(names []string) string {
	quoted := make([]string, len(names))
	for i, name := range names {
		quoted[i] = regexp.QuoteMeta(name)
	}
	return "^(" + strings.Join(quoted, "|") + ")$"
}

func runGroup(ctx context.Context, out, id, dir string, command, assigned, packages []string, isolated bool) scopedGroup {
	group := scopedGroup{ID: id, Command: command, Assigned: append([]string(nil), assigned...), Packages: append([]string(nil), packages...), ExitCode: 127}
	groupDir := filepath.Join(out, id)
	group.StdoutLog = filepath.Join(groupDir, "test.jsonl")
	group.StderrLog = filepath.Join(groupDir, "stderr.txt")
	if err := os.MkdirAll(groupDir, 0700); err != nil {
		group.Error = err.Error()
		return group
	}
	stdout, err := os.Create(group.StdoutLog)
	if err != nil {
		group.Error = err.Error()
		return group
	}
	defer stdout.Close()
	stderr, err := os.Create(group.StderrLog)
	if err != nil {
		group.Error = err.Error()
		return group
	}
	defer stderr.Close()
	cmd := exec.Command(command[0], command[1:]...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Stdout, cmd.Stderr = stdout, io.MultiWriter(stderr, os.Stderr)
	if isolated {
		// Git worktree tests create deeply nested paths. Keep each worker's
		// writable root short on Windows while preserving process isolation.
		root, err := os.MkdirTemp("", "sl-test-")
		if err != nil {
			group.Error = err.Error()
			return group
		}
		defer os.RemoveAll(root)
		extraEnv := map[string]string{"STEERLINE_HOME": filepath.Join(root, "home"), "TMP": filepath.Join(root, "tmp"), "TEMP": filepath.Join(root, "tmp"), "TMPDIR": filepath.Join(root, "tmp")}
		for _, key := range []string{"STEERLINE_HOME", "TMP", "TEMP", "TMPDIR"} {
			if path := extraEnv[key]; path != "" {
				if err := os.MkdirAll(path, 0700); err != nil {
					group.Error = err.Error()
					return group
				}
			}
		}
		env := os.Environ()
		for key, value := range extraEnv {
			env = append(env, key+"="+value)
		}
		cmd.Env = env
	}
	begin := time.Now()
	err = runOwned(ctx, cmd)
	group.WallSeconds = time.Since(begin).Seconds()
	group.ExitCode = 0
	if err != nil {
		group.Error = err.Error()
		group.ExitCode = 127
		if exit, ok := err.(*exec.ExitError); ok {
			group.ExitCode = exit.ExitCode()
		}
	}
	if err := stdout.Close(); err != nil && group.Error == "" {
		group.Error = err.Error()
	}
	raw, err := os.ReadFile(group.StdoutLog)
	if err != nil {
		group.Error += "; read log: " + err.Error()
	}
	group.Summary = testfeedback.Parse(raw, group.ExitCode)
	return group
}

func activeRealSwitches() []string {
	set := map[string]bool{}
	for _, name := range enabledNativeSwitches() {
		set[name] = true
	}
	for _, kv := range os.Environ() {
		name, value, ok := strings.Cut(kv, "=")
		if !ok || value == "" || value == "0" || strings.EqualFold(value, "false") {
			continue
		}
		for _, prefix := range []string{"STEERLINE_REAL_", "STEERLINE_NATIVE_", "STEERLINE_LOCAL_CAPABILITY_", "STEERLINE_LIVE_", "STEERLINE_RUN_WINDOW_", "STEERLINE_WINDOW_PROBE_", "STEERLINE_CODEX_HISTORY_NATIVE_"} {
			if strings.HasPrefix(name, prefix) {
				set[name] = true
			}
		}
	}
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func checkScopedCoverage(report scopedReport) []string {
	var failures []string
	seenPackages := map[string]int{}
	seenCore := map[string]int{}
	for _, group := range report.Groups {
		if group.Error != "" {
			failures = append(failures, group.ID+": "+group.Error)
		}
		if report.Scope == "quick" && (group.Summary.SkippedTests != 0 || len(group.Summary.SkippedPackages) != 0) {
			failures = append(failures, group.ID+": quick coverage contains skipped tests or packages")
		}
		if group.ID == "quick-targeted" && group.Summary.TestCount == 0 {
			failures = append(failures, "quick-targeted: no tests matched the explicit selection")
		}
		if !group.Summary.Passed {
			failures = append(failures, group.ID+": "+group.Summary.Status+" "+strings.Join(group.Summary.IncompleteReasons, "; "))
		}
		raw, err := os.ReadFile(group.StdoutLog)
		if err != nil {
			failures = append(failures, group.ID+": read log: "+err.Error())
			continue
		}
		runs, terminals, packages, scanErr := scanEvents(raw)
		if scanErr != nil {
			failures = append(failures, group.ID+": "+scanErr.Error())
			continue
		}
		assigned := map[string]bool{}
		for _, name := range group.Assigned {
			assigned[name] = true
		}
		for name, count := range runs {
			if terminals[name] != count || count != 1 {
				failures = append(failures, fmt.Sprintf("%s: test %s run=%d terminal=%d", group.ID, name, count, terminals[name]))
			}
		}
		for name, count := range terminals {
			if runs[name] != count {
				failures = append(failures, fmt.Sprintf("%s: unmatched terminal %s", group.ID, name))
			}
		}
		if len(group.Assigned) > 0 {
			for _, name := range group.Assigned {
				key := corePackage + "\x00" + name
				if runs[key] != 1 || terminals[key] != 1 {
					failures = append(failures, fmt.Sprintf("%s: missing assigned test %s", group.ID, name))
				}
			}
			if strings.HasPrefix(group.ID, "core-") || group.ID == "quick-core" {
				for key := range runs {
					pkg, test, _ := strings.Cut(key, "\x00")
					if pkg != corePackage || (!strings.Contains(test, "/") && !assigned[test]) {
						failures = append(failures, group.ID+": unexpected test "+key)
					}
				}
			}
		}
		for pkg, count := range packages {
			if count != 1 {
				failures = append(failures, fmt.Sprintf("%s: package %s has %d terminal events", group.ID, pkg, count))
			}
			if group.ID == "other-packages" || group.ID == "quick-core" || group.ID == "quick-rules" {
				seenPackages[pkg] += count
			}
		}
		if strings.HasPrefix(group.ID, "core-") {
			if packages[corePackage] != 1 {
				failures = append(failures, group.ID+": core package has no terminal")
			}
			for key, count := range runs {
				pkg, test, _ := strings.Cut(key, "\x00")
				if pkg == corePackage && !strings.Contains(test, "/") {
					seenCore[test] += count
				}
			}
		}
	}
	if report.Scope == "full" {
		for _, name := range report.CoreTests {
			if seenCore[name] != 1 {
				failures = append(failures, fmt.Sprintf("core test %s ran %d times", name, seenCore[name]))
			}
		}
		if len(seenCore) != len(report.CoreTests) {
			failures = append(failures, "core top-level test set differs from enumeration")
		}
		for _, pkg := range report.AllPackages {
			if pkg != corePackage && seenPackages[pkg] != 1 {
				failures = append(failures, fmt.Sprintf("package %s completed %d times", pkg, seenPackages[pkg]))
			}
		}
		if len(seenPackages) != len(report.AllPackages)-1 {
			failures = append(failures, "other package set differs from enumeration")
		}
	} else {
		for _, pkg := range quickPackages {
			if seenPackages[pkg] != 1 {
				failures = append(failures, fmt.Sprintf("quick package %s completed %d times", pkg, seenPackages[pkg]))
			}
		}
		if len(seenPackages) != len(quickPackages) {
			failures = append(failures, "quick package set differs")
		}
	}
	return failures
}

func scanEvents(raw []byte) (map[string]int, map[string]int, map[string]int, error) {
	runs, terminals, packages := map[string]int{}, map[string]int{}, map[string]int{}
	scanner := bufio.NewScanner(strings.NewReader(string(raw)))
	scanner.Buffer(make([]byte, 64*1024), 32<<20)
	for scanner.Scan() {
		var event testfeedback.Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, nil, nil, err
		}
		if event.Package == "" || event.Action == "" {
			return nil, nil, nil, fmt.Errorf("event lacks package or action")
		}
		if event.Test == "" {
			if event.Action == "pass" || event.Action == "fail" || event.Action == "skip" {
				packages[event.Package]++
			}
			continue
		}
		key := event.Package + "\x00" + event.Test
		if event.Action == "run" {
			runs[key]++
		}
		if event.Action == "pass" || event.Action == "fail" || event.Action == "skip" {
			terminals[key]++
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, nil, err
	}
	if len(raw) == 0 || raw[len(raw)-1] != '\n' {
		return nil, nil, nil, fmt.Errorf("empty or truncated JSON log")
	}
	return runs, terminals, packages, nil
}

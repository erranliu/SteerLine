package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	testfeedback "github.com/erranliu/SteerLine/internal/testkit/feedback"
)

type options struct {
	out            string
	phase          string
	cacheCondition string
	nativeNote     string
	buildOnly      bool
	summarizeLog   string
	exitCode       int
	goTestArgs     []string
	quick          bool
	workers        int
	level          string
	suite          string
	e2eEnv         string
	scenario       string
	managed        *managedRun
}

type environment struct {
	SourceCommit   string            `json:"source_commit"`
	Worktree       string            `json:"worktree_status"`
	GoVersion      string            `json:"go_version"`
	GoEnv          map[string]string `json:"go_env"`
	OS             string            `json:"os"`
	Architecture   string            `json:"architecture"`
	Machine        map[string]any    `json:"machine"`
	Concurrency    map[string]string `json:"concurrency"`
	Cache          string            `json:"build_cache_condition"`
	NativeSwitches []string          `json:"enabled_native_acceptance_switches"`
	NativeNote     string            `json:"native_acceptance_note"`
}

type runReport struct {
	SchemaVersion int                  `json:"schema_version"`
	Scope         string               `json:"scope"`
	Phase         string               `json:"phase"`
	Mode          string               `json:"mode"`
	Command       []string             `json:"command"`
	StartedAt     string               `json:"started_at"`
	FinishedAt    string               `json:"finished_at"`
	WallSeconds   float64              `json:"wall_seconds"`
	Environment   environment          `json:"environment"`
	ProcessError  string               `json:"process_error,omitempty"`
	Summary       testfeedback.Summary `json:"summary"`
	StdoutLog     string               `json:"stdout_json_log"`
	StderrLog     string               `json:"stderr_log"`
}

type summaryOnlyReport struct {
	SchemaVersion   int                  `json:"schema_version"`
	SourceJSONLog   string               `json:"source_json_log"`
	SummarizedAt    string               `json:"summarized_at"`
	CommandExitCode int                  `json:"command_exit_code"`
	Summary         testfeedback.Summary `json:"summary"`
}

func main() {
	code := run(os.Args[1:])
	os.Exit(code)
}

func run(args []string) int {
	if len(args) > 0 && args[0] == "record" {
		return recordReports(args[1:])
	}
	parsed, err := parseOptions(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if parsed.summarizeLog == "" {
		existing, readErr := os.ReadDir(parsed.out)
		if readErr != nil && !os.IsNotExist(readErr) {
			fmt.Fprintf(os.Stderr, "inspect report directory: %v\n", readErr)
			return 2
		}
		if len(existing) != 0 {
			fmt.Fprintln(os.Stderr, "output directory must be empty; use a new -out path")
			return 2
		}
	}
	if err = os.MkdirAll(parsed.out, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "create report directory: %v\n", err)
		return 2
	}
	if parsed.level != "" && !parsed.buildOnly && parsed.summarizeLog == "" && len(parsed.goTestArgs) == 0 {
		before, sequence := observeManagedSource(), managedReportSequence()
		parsed.managed, err = freezeManagedReports(parsed)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		defer writeManagedReports(parsed, before, sequence)
	}
	if parsed.summarizeLog != "" {
		return summarizeExisting(parsed)
	}
	if parsed.level == "L4" && parsed.suite == "go" {
		parsed.workers = 3
		return runScoped(parsed)
	}
	if parsed.quick || parsed.workers == 3 {
		return runScoped(parsed)
	}
	if parsed.level == "L2" || parsed.level == "L3" || parsed.level == "L4" {
		return runE2E(parsed)
	}

	goArgs := parsed.goTestArgs
	mode := "tests"
	scope := "targeted"
	if parsed.buildOnly {
		mode = "build-only"
		goArgs = []string{"./...", "-run=^$", "-count=1", "-timeout", "20m", "-json"}
	} else if len(goArgs) == 0 {
		scope = "full"
		goArgs = []string{"./...", "-json", "-count=1", "-timeout", "45m", "-p", "1", "-parallel", "1"}
	} else if !hasJSONFlag(goArgs) {
		goArgs = append(goArgs, "-json")
	}
	commandArgs := append([]string{"test"}, goArgs...)
	command := append([]string{"go"}, commandArgs...)

	stdoutPath := filepath.Join(parsed.out, "go-test.jsonl")
	stderrPath := filepath.Join(parsed.out, "go-test.stderr.txt")
	stdoutFile, err := os.Create(stdoutPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create JSON log: %v\n", err)
		return 2
	}
	stderrFile, err := os.Create(stderrPath)
	if err != nil {
		stdoutFile.Close()
		fmt.Fprintf(os.Stderr, "create stderr log: %v\n", err)
		return 2
	}

	env := collectEnvironment(parsed.cacheCondition, parsed.nativeNote, parsed.out, goArgs)
	startedAt := time.Now().UTC()
	stopwatch := time.Now()
	cmd := exec.Command("go", commandArgs...)
	// Keep the machine-readable stream byte-for-byte in the report directory.
	// Printing every JSON event on the terminal makes large suites noisy and
	// needlessly adds output-pipe cost to measured wall time.
	cmd.Stdout = stdoutFile
	cmd.Stderr = io.MultiWriter(stderrFile, os.Stderr)
	err = cmd.Run()
	wallSeconds := time.Since(stopwatch).Seconds()
	finishedAt := time.Now().UTC()
	stdoutCloseErr := stdoutFile.Close()
	stderrCloseErr := stderrFile.Close()
	if stdoutCloseErr != nil && err == nil {
		err = stdoutCloseErr
	}
	if stderrCloseErr != nil && err == nil {
		err = stderrCloseErr
	}

	exitCode := 0
	processError := ""
	if err != nil {
		processError = err.Error()
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 127
		}
	}
	raw, readErr := os.ReadFile(stdoutPath)
	if readErr != nil {
		fmt.Fprintf(os.Stderr, "read JSON log: %v\n", readErr)
		return 2
	}
	summary := testfeedback.Parse(raw, exitCode)
	if processError != "" && len(raw) == 0 {
		summary.Complete = false
		summary.Status = "incomplete"
		summary.Passed = false
		summary.IncompleteReasons = append(summary.IncompleteReasons, "could not start or finish go test: "+processError)
	}
	report := runReport{
		SchemaVersion: 1,
		Scope:         scope,
		Phase:         parsed.phase,
		Mode:          mode,
		Command:       command,
		StartedAt:     startedAt.Format(time.RFC3339Nano),
		FinishedAt:    finishedAt.Format(time.RFC3339Nano),
		WallSeconds:   wallSeconds,
		Environment:   env,
		ProcessError:  processError,
		Summary:       summary,
		StdoutLog:     filepath.ToSlash(stdoutPath),
		StderrLog:     filepath.ToSlash(stderrPath),
	}
	if err = writeJSON(filepath.Join(parsed.out, "summary.json"), report); err != nil {
		fmt.Fprintf(os.Stderr, "write report: %v\n", err)
		return 2
	}
	fmt.Fprintf(os.Stderr, "test feedback: %s (%.3fs wall); report %s\n", summary.Status, wallSeconds, filepath.ToSlash(filepath.Join(parsed.out, "summary.json")))
	for _, failed := range summary.Failures {
		fmt.Fprintf(os.Stderr, "failed test: %s %s\n", failed.Package, failed.Name)
	}
	for _, failed := range summary.FailedPackages {
		fmt.Fprintf(os.Stderr, "failed package: %s\n", failed)
	}
	for _, reason := range summary.IncompleteReasons {
		fmt.Fprintf(os.Stderr, "incomplete: %s\n", reason)
	}
	if exitCode != 0 {
		return exitCode
	}
	if !summary.Passed {
		return 1
	}
	return 0
}

func parseOptions(args []string) (options, error) {
	separator := len(args)
	for index, arg := range args {
		if arg == "--" {
			separator = index
			break
		}
	}
	toolArgs := args[:separator]
	goArgs := []string(nil)
	if separator < len(args) {
		goArgs = append(goArgs, args[separator+1:]...)
	}
	flags := flag.NewFlagSet("test-feedback", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var opts options
	flags.StringVar(&opts.out, "out", "", "directory for original logs and summary.json")
	flags.StringVar(&opts.phase, "phase", "run", "label for this measurement")
	flags.StringVar(&opts.cacheCondition, "cache-condition", "as found; no explicit cache clear", "describe Go build cache state")
	flags.StringVar(&opts.nativeNote, "native-acceptance", "native acceptance not explicitly enabled", "record native acceptance setup")
	flags.BoolVar(&opts.buildOnly, "build-only", false, "run all packages with -run=^$ to measure build/precompile time")
	flags.StringVar(&opts.summarizeLog, "summarize-log", "", "summarize an existing go test -json log without running tests")
	flags.IntVar(&opts.exitCode, "exit-code", 0, "exit code from the command that produced -summarize-log")
	flags.BoolVar(&opts.quick, "quick", false, "run the fixed minimal closed loop and pure rule packages")
	flags.IntVar(&opts.workers, "workers", 1, "suite processes: 1 or 3; L1/quick defaults to 3")
	flags.StringVar(&opts.level, "level", "", "test level: L1, L2, L3 or L4")
	flags.StringVar(&opts.suite, "suite", "", "comma-separated L3/L4 suites")
	flags.StringVar(&opts.e2eEnv, "env", "", "test environment: controlled (L2 default), live or isolated (L4 Go)")
	flags.StringVar(&opts.scenario, "scenario", "", "comma-separated L3/L4 scenario IDs")
	if err := flags.Parse(toolArgs); err != nil {
		return opts, fmt.Errorf("test-feedback options: %w", err)
	}
	if len(flags.Args()) != 0 {
		return opts, fmt.Errorf("Go test arguments must follow --")
	}
	seenOptions := map[string]bool{}
	for _, arg := range toolArgs {
		if strings.HasPrefix(arg, "-") {
			key := strings.TrimLeft(strings.SplitN(arg, "=", 2)[0], "-")
			if flags.Lookup(key) != nil {
				if seenOptions[key] {
					return opts, fmt.Errorf("duplicate -%s", key)
				}
				seenOptions[key] = true
			}
		}
	}
	switch opts.level {
	case "":
		if seenOptions["suite"] || seenOptions["env"] || seenOptions["scenario"] {
			return opts, fmt.Errorf("-suite/-env require -level")
		}
	case "L1":
		if seenOptions["quick"] || seenOptions["suite"] || seenOptions["env"] || seenOptions["scenario"] {
			return opts, fmt.Errorf("L1 does not accept quick, suite or env")
		}
		opts.quick = true
	case "L2", "L3", "L4":
		for _, key := range []string{"quick", "workers", "build-only", "summarize-log"} {
			if seenOptions[key] {
				return opts, fmt.Errorf("E2E cannot combine -%s", key)
			}
		}
		if len(goArgs) > 0 {
			return opts, fmt.Errorf("E2E does not accept raw Go arguments")
		}
		if opts.level == "L2" && (seenOptions["suite"] || seenOptions["scenario"]) {
			return opts, fmt.Errorf("L2 has a fixed smoke selection")
		}
		if opts.level != "L2" && opts.suite == "" {
			return opts, fmt.Errorf("%s requires -suite", opts.level)
		}
		if opts.e2eEnv == "" && opts.level == "L2" {
			opts.e2eEnv = "controlled"
		}
		localGo := opts.level == "L4" && opts.suite == "go" && (opts.scenario == "" || opts.scenario == "go.full")
		if localGo && opts.e2eEnv == "" {
			opts.e2eEnv = "isolated"
		}
		live := opts.level == "L4" && opts.e2eEnv == "live" && (opts.suite == "smoke" && opts.scenario == "smoke.local-codex" ||
			opts.suite == "workflow" && opts.scenario == "workflow.local-codex-audited" ||
			opts.suite == "testing" && opts.scenario == "testing.management-local-codex")
		if localGo {
			if opts.e2eEnv != "isolated" {
				return opts, fmt.Errorf("L4 go.full requires isolated environment")
			}
		} else if opts.suite == "go" || opts.e2eEnv != "controlled" && !live {
			return opts, fmt.Errorf("unsupported test selection/environment %q", opts.e2eEnv)
		}

	default:
		return opts, fmt.Errorf("invalid test level %q", opts.level)
	}
	if opts.buildOnly && len(goArgs) != 0 {
		return opts, fmt.Errorf("-build-only cannot be combined with Go test arguments")
	}
	if opts.summarizeLog != "" && (opts.buildOnly || len(goArgs) != 0) {
		return opts, fmt.Errorf("-summarize-log cannot be combined with Go test arguments or -build-only")
	}
	if opts.workers != 1 && opts.workers != 3 {
		return opts, fmt.Errorf("-workers must be 1 or 3")
	}
	if opts.quick && !seenOptions["workers"] {
		opts.workers = 3
	}
	if opts.quick && (opts.buildOnly || opts.summarizeLog != "") {
		return opts, fmt.Errorf("-quick cannot be combined with build-only or summarize-log")
	}
	if opts.quick && len(goArgs) > 0 {
		for _, arg := range goArgs {
			if arg == "./..." {
				return opts, fmt.Errorf("-quick additional Go arguments must select a focused package or test, not ./...")
			}
		}
		if count := flagValue(goArgs, "-count", "1"); count != "1" {
			return opts, fmt.Errorf("-quick additional tests require -count=1")
		}
	}
	if !opts.quick && opts.workers == 3 && (opts.buildOnly || opts.summarizeLog != "" || len(goArgs) != 0) {
		return opts, fmt.Errorf("-workers=3 only runs the complete deterministic suite")
	}
	if opts.out == "" {
		opts.out = filepath.Join(os.TempDir(), "steerline-test-feedback", time.Now().UTC().Format("20060102T150405.000000000Z"))
	}
	opts.goTestArgs = goArgs
	return opts, nil
}

func summarizeExisting(opts options) int {
	raw, err := os.ReadFile(opts.summarizeLog)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read existing JSON log: %v\n", err)
		return 2
	}
	summary := testfeedback.Parse(raw, opts.exitCode)
	report := summaryOnlyReport{
		SchemaVersion:   1,
		SourceJSONLog:   filepath.ToSlash(opts.summarizeLog),
		SummarizedAt:    time.Now().UTC().Format(time.RFC3339Nano),
		CommandExitCode: opts.exitCode,
		Summary:         summary,
	}
	if err = writeJSON(filepath.Join(opts.out, "summary.json"), report); err != nil {
		fmt.Fprintf(os.Stderr, "write summary: %v\n", err)
		return 2
	}
	fmt.Fprintf(os.Stderr, "test feedback: %s; report %s\n", summary.Status, filepath.ToSlash(filepath.Join(opts.out, "summary.json")))
	if opts.exitCode != 0 {
		return opts.exitCode
	}
	if !summary.Passed {
		return 1
	}
	return 0
}

func hasJSONFlag(args []string) bool {
	for _, arg := range args {
		if arg == "-json" || arg == "-json=true" {
			return true
		}
	}
	return false
}

func collectEnvironment(cacheCondition, nativeNote, out string, goArgs []string) environment {
	commit := commandOutput("git", "rev-parse", "HEAD")
	status := worktreeStatus(out)
	goEnv := map[string]string{}
	envOutput := commandOutput("go", "env", "GOOS", "GOARCH", "GOFLAGS", "GOCACHE", "GOTOOLCHAIN")
	values := strings.Split(strings.TrimSuffix(envOutput, "\n"), "\n")
	keys := []string{"GOOS", "GOARCH", "GOFLAGS", "GOCACHE", "GOTOOLCHAIN"}
	for index, key := range keys {
		if index < len(values) {
			goEnv[key] = values[index]
		}
	}
	concurrency := map[string]string{
		"GOMAXPROCS_environment":    os.Getenv("GOMAXPROCS"),
		"runtime_GOMAXPROCS":        fmt.Sprint(runtime.GOMAXPROCS(0)),
		"go_test_parallel":          flagValue(goArgs, "-parallel", fmt.Sprintf("default %d", runtime.GOMAXPROCS(0))),
		"go_build_package_parallel": flagValue(goArgs, "-p", "Go default"),
	}
	if concurrency["GOMAXPROCS_environment"] == "" {
		concurrency["GOMAXPROCS_environment"] = "unset"
	}
	machine := map[string]any{
		"host_name":            commandOutput("hostname"),
		"logical_cpu_count":    runtime.NumCPU(),
		"processor_identifier": os.Getenv("PROCESSOR_IDENTIFIER"),
	}
	nativeSwitches := enabledNativeSwitches()
	return environment{
		SourceCommit:   strings.TrimSpace(commit),
		Worktree:       status,
		GoVersion:      strings.TrimSpace(commandOutput("go", "version")),
		GoEnv:          goEnv,
		OS:             runtime.GOOS,
		Architecture:   runtime.GOARCH,
		Machine:        machine,
		Concurrency:    concurrency,
		Cache:          cacheCondition,
		NativeSwitches: nativeSwitches,
		NativeNote:     nativeNote,
	}
}

func worktreeStatus(outDir string) string {
	rootBytes, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "unavailable: git rev-parse --show-toplevel: " + err.Error()
	}
	root := strings.TrimSpace(string(rootBytes))
	statusBytes, err := exec.Command("git", "status", "--short", "--untracked-files=all", "-z").Output()
	if err != nil {
		return "unavailable: git status: " + err.Error()
	}
	status := filterGeneratedRunOutput(statusBytes, generatedRunOutputPaths(root, outDir))
	if status == "" {
		return "clean (this run's logs and summary excluded)"
	}
	return status
}

func generatedRunOutputPaths(repoRoot, outDir string) map[string]struct{} {
	paths := map[string]struct{}{}
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		return paths
	}
	out, err := filepath.Abs(outDir)
	if err != nil {
		return paths
	}
	rel, err := filepath.Rel(root, out)
	if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return paths
	}
	for _, name := range []string{"go-test.jsonl", "go-test.stderr.txt", "summary.json"} {
		paths[filepath.ToSlash(filepath.Join(rel, name))] = struct{}{}
	}
	return paths
}

func filterGeneratedRunOutput(status []byte, generatedPaths map[string]struct{}) string {
	var kept []string
	entries := bytes.Split(status, []byte{0})
	for index := 0; index < len(entries); index++ {
		entry := entries[index]
		if len(entry) == 0 {
			continue
		}
		if len(entry) < 4 || entry[2] != ' ' {
			kept = append(kept, string(entry))
			continue
		}
		code := string(entry[:2])
		path := strings.ReplaceAll(string(entry[3:]), "\\", "/")
		if strings.ContainsAny(code, "RC") && index+1 < len(entries) && len(entries[index+1]) > 0 {
			kept = append(kept, code+" "+path+" -> "+string(entries[index+1]))
			index++
			continue
		}
		if _, isGeneratedOutput := generatedPaths[path]; isGeneratedOutput {
			continue
		}
		kept = append(kept, code+" "+path)
	}
	return strings.Join(kept, "\n")
}

func enabledNativeSwitches() []string {
	known := []string{"STEERLINE_DEBUG_TEST_ROOT", "STEERLINE_DEBUG_SECOND_TEST_ROOT"}

	enabled := []string{}
	for _, name := range known {
		if value := os.Getenv(name); value != "" && value != "0" && !strings.EqualFold(value, "false") {
			enabled = append(enabled, name)
		}
	}
	sort.Strings(enabled)
	return enabled
}

func flagValue(args []string, name, fallback string) string {
	prefix := name + "="
	for index, arg := range args {
		if strings.HasPrefix(arg, prefix) {
			return strings.TrimPrefix(arg, prefix)
		}
		if arg == name && index+1 < len(args) {
			return args[index+1]
		}
	}
	return fallback
}

func commandOutput(name string, args ...string) string {
	output, err := exec.Command(name, args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0600)
}

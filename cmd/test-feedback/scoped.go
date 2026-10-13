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

	testfeedback "github.com/erranliu/SteerLine/internal/testkit/feedback"
)

const corePackage = "github.com/erranliu/SteerLine/internal/app/host"

var quickTests = []string{"TestTM08ControlIdentity", "TestTM08Heartbeat", "TestTM08Shutdown", "TestServiceLifetimeCloseOnRunError"}

const executionPackage = "github.com/erranliu/SteerLine/internal/execution"

// Full partitions the execution business tests after their move out of the host facade.
const fullCorePackage = executionPackage

// Recovery, multi-turn and candidate orchestration belong to directed regression
// or Full. L1 keeps one claim plus the immutable identity and delivery rules.
var quickExecutionTests = []string{"TestWorkerFlowClaimNeedsNoProjectSteps", "TestPS2MandatoryLifecycleCapabilitiesKeepFixedIdentity", "TestTaskStatusAndDeliveryEvidence", "TestDeliveryGateRequiresSelectedWorker", "TestRestartPreflightRetainsOriginalPolicyRoots", "TestProjectPolicyFrozenSnapshotRejectsDamageAndPreservesAuthority"}

const boundaryPackage = "github.com/erranliu/SteerLine/tests/integration"

var quickBoundaryTests = []string{"TestCD67CoreBoundary", "TestTM04ExecutionWiring", "TestTM07StorageWiring"}

var quickBoundarySubtests = []string{"pure_ownership_all_tags_production_and_tests", "negative_source_fixtures", "repository_all_tags_and_production_graphs", "console_negative_all_tags_production_and_tests", "claim_transaction_failure_and_original_receipt"}

func quickBoundaryPattern() string {
	// Go matches each slash-separated test level independently. Keep all static
	// guards, but only the claim transaction subtest of the long wiring suite.
	return exactPattern(quickBoundaryTests) + "/" + exactPattern(quickBoundarySubtests)
}

var quickBoundaryAssigned = []string{
	"TestCD67CoreBoundary",
	"TestCD67CoreBoundary/pure_ownership_all_tags_production_and_tests",
	"TestCD67CoreBoundary/pure_ownership_all_tags_production_and_tests/product/query/taskgraph",
	"TestCD67CoreBoundary/pure_ownership_all_tags_production_and_tests/task",
	"TestCD67CoreBoundary/pure_ownership_all_tags_production_and_tests/task/model",
	"TestCD67CoreBoundary/pure_ownership_all_tags_production_and_tests/execution/model",
	"TestCD67CoreBoundary/pure_ownership_all_tags_production_and_tests/execution/backend",
	"TestCD67CoreBoundary/pure_ownership_all_tags_production_and_tests/execution/conversation",
	"TestCD67CoreBoundary/pure_ownership_all_tags_production_and_tests/product/api",
	"TestCD67CoreBoundary/pure_ownership_all_tags_production_and_tests/storage/ports",
	"TestCD67CoreBoundary/negative_source_fixtures",
	"TestCD67CoreBoundary/negative_source_fixtures/mixed_case_text_in_other_os_test",
	"TestCD67CoreBoundary/negative_source_fixtures/mixed_case_filename",
	"TestCD67CoreBoundary/negative_source_fixtures/direct_import_behind_tag",
	"TestCD67CoreBoundary/negative_source_fixtures/transitive_helper_edge",
	"TestCD67CoreBoundary/negative_source_fixtures/nested_generated_name_is_not_an_exclusion",
	"TestCD67CoreBoundary/negative_source_fixtures/top_level_output_name_is_not_an_exclusion",
	"TestCD67CoreBoundary/negative_source_fixtures/reverse_implementation_edge",
	"TestCD67CoreBoundary/negative_source_fixtures/reverse_storage_edge",
	"TestCD67CoreBoundary/negative_source_fixtures/concrete_storage_edge",
	"TestCD67CoreBoundary/negative_source_fixtures/transitive_storage_edge",
	"TestCD67CoreBoundary/negative_source_fixtures/reverse_client_subpackage",
	"TestCD67CoreBoundary/repository_all_tags_and_production_graphs",
	"TestCD67CoreBoundary/console_negative_all_tags_production_and_tests",
	"TestCD67CoreBoundary/console_negative_all_tags_production_and_tests/transitive_test_helper",
	"TestTM07StorageWiring",
	"TestTM04ExecutionWiring",
	"TestTM04ExecutionWiring/claim_transaction_failure_and_original_receipt",
}

var quickRulePackages = append(append([]string{}, taskRulePackages...), []string{
	"github.com/erranliu/SteerLine/internal/product/api",
	"github.com/erranliu/SteerLine/internal/product/client",
	"github.com/erranliu/SteerLine/internal/product/query",
	"github.com/erranliu/SteerLine/internal/product/query/taskgraph",
	"github.com/erranliu/SteerLine/internal/storage/ports",
	"github.com/erranliu/SteerLine/internal/execution/scheduler",
	"github.com/erranliu/SteerLine/internal/execution/model",
	"github.com/erranliu/SteerLine/internal/execution/backend",
	executionPackage + "/reconstruction",
	"github.com/erranliu/SteerLine/internal/storage/sqlite",
	"github.com/erranliu/SteerLine/internal/console", "github.com/erranliu/SteerLine/internal/console/activityrender",
	"github.com/erranliu/SteerLine/internal/testkit/boundary/e2e",
}...)

var quickOwnerCases = []struct {
	ID, Package string
	Tests       []string
}{
	{"quick-execution", executionPackage, quickExecutionTests},
	{"quick-execution-boundary", "github.com/erranliu/SteerLine/internal/testkit/boundary", []string{"TestTM04ExecutionBoundary", "TestSharedFixturesRemainTestOnly", "TestTM06AdapterBoundary", "TestTM06ContractValueClosure", "TestTM05ProductQueryBoundary", "TestTM07StorageBoundary", "TestTM08AppHostBoundary"}},
	{"quick-owner-orca-terminal", "github.com/erranliu/SteerLine/internal/execution/adapters/orcatransport", []string{"TestTerminalLifecycleReplyReuseAndExactStop", "TestTerminalUnknownSubmissionOnlyObservesOriginal", "TestTerminalLocalAndRemoteUseIdenticalReturnAndNativePorts", "TestTerminalClippedScreenDoesNotBlockNativeFinal"}},
	{"quick-owner-codec", "github.com/erranliu/SteerLine/internal/execution/adapters/codex", []string{"TestCD63CodecIdentityAndVersion", "TestCD62NativeFactsAndQuestionRoute", "TestHistoryMixedTurnRetainsOnlyCompleteFinal", "TestHistoryAdapterRejectsOversizedNativeCursor", "TestAppServerLaunchPreservesConfiguredArgs", "TestAppServerPendingFeedbackRecoveryUsesNewTurnReceipt"}},
	{"quick-owner-identity", "github.com/erranliu/SteerLine/internal/execution/adapters/protocol/orca", []string{"TestBoundTerminalRejectsRuntimeAndIncarnationReplacement", "TestTerminalCloseRequiresExactRuntimeHandleAndPtyProof"}},
	{"quick-owner-codexapp", "github.com/erranliu/SteerLine/internal/execution/adapters/protocol/codexapp", []string{"TestAskToolResponseUsesNativeAnswerEnvelope", "TestCodexAppCallCancellationInterruptsBlockedWrite", "TestCodexSocketHandshakeHonorsCancellationWithoutDeadline", "TestCodexSocketLoopbackHandshakeAndMaskedJSON"}},
	{"quick-owner-workspace", "github.com/erranliu/SteerLine/internal/execution/workspace", []string{"TestLocalWorkspacePrepareInspectResourcesAndCleanup", "TestPrepareRejectsHeadChangedByCheckoutHook"}},
	{"quick-owner-publication", "github.com/erranliu/SteerLine/internal/execution/publication", []string{"TestPublicationPolicyValidatesProviderValues", "TestPRRequiresExactIdentityAndMergeEvidence", "TestUnsupportedProviderDoesNoNetworkOperation"}},
	{"quick-owner-app", "github.com/erranliu/SteerLine/internal/app", []string{"TestNativeProjectInitializesOnceAndRejectsMismatchedBinding", "TestProjectConfigDecoderPreservesStrictSingleJSONValueAndBOM", "TestRunConfiguredServiceRejectsBeadsBeforeStartupWrites", "TestAppConfigProjectsOwnerInputsAndSelectedProfile", "TestCM71ProductDependencyBoundary", "TestPS2FixedWindowsProfilesFreezeAndRestore", "TestOrcaNativeProfileKeepsFrozenAuthority", "TestOrcaPreflightDoesNotProbeAgentExecutable", "TestProjectPolicyFrozenSnapshotRejectsDamageAndPreservesAuthority", "TestNativeOrcaAgentSelectionUsesFrozenProfile"}},
}

var quickPackages = func() []string {
	p := append([]string{corePackage}, quickRulePackages...)
	p = append(p, boundaryPackage)
	for _, owner := range quickOwnerCases {
		p = append(p, owner.Package)
	}
	return p
}()

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
	StartedAt   string               `json:"started_at,omitempty"`
	FinishedAt  string               `json:"finished_at,omitempty"`
	ExitCode    int                  `json:"exit_code"`
	Error       string               `json:"error,omitempty"`
	Summary     testfeedback.Summary `json:"summary"`
}

type scopedReport struct {
	SchemaVersion      int           `json:"schema_version"`
	Scope              string        `json:"scope"`
	Phase              string        `json:"phase"`
	StartedAt          string        `json:"started_at"`
	FinishedAt         string        `json:"finished_at"`
	PreparationSecs    float64       `json:"preparation_seconds"`
	ExecutionSecs      float64       `json:"execution_seconds"`
	Workers            int           `json:"workers"`
	Environment        environment   `json:"environment"`
	CorePackage        string        `json:"core_package"`
	CoreTests          []string      `json:"enumerated_core_tests,omitempty"`
	IntegrationPackage string        `json:"integration_package,omitempty"`
	IntegrationTests   []string      `json:"enumerated_integration_tests,omitempty"`
	AllPackages        []string      `json:"enumerated_packages"`
	WeightSource       string        `json:"weight_source,omitempty"`
	Groups             []scopedGroup `json:"groups"`
	Complete           bool          `json:"complete"`
	Passed             bool          `json:"passed"`
	Errors             []string      `json:"errors"`
}

func runScoped(opts options) int {
	if opts.workers == 3 {
		if switches := activeRealSwitches(); len(switches) > 0 {
			fmt.Fprintf(os.Stderr, "parallel suite refuses native acceptance switches %s; use -workers=1\n", strings.Join(switches, ", "))
			return 2
		}
	}
	started := time.Now()
	report := scopedReport{SchemaVersion: 1, Phase: opts.phase, Workers: opts.workers, StartedAt: started.UTC().Format(time.RFC3339Nano), Complete: true, Errors: []string{}, Groups: []scopedGroup{}}
	if opts.quick {
		report.Scope = "quick"
		report.CorePackage = corePackage
	} else {
		report.Scope = "full"
		report.CorePackage = fullCorePackage
		report.WeightSource = "testdata/core-test-weights.json (serial baseline 543f0d4; top-level elapsed, floor 0.05s)"
	}
	report.Environment = collectEnvironment(opts.cacheCondition, opts.nativeNote, opts.out, nil)
	// Keep room for the bounded multi-turn integration fixtures as well as
	// each worker's core partition; missing terminal events still fail coverage.
	timeout := 60 * time.Minute
	if !opts.quick {
		timeout = 75 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var packages []string
	var err error
	if opts.quick {
		report.AllPackages = append([]string(nil), quickPackages...)
	} else {
		packages, err = listPackages(ctx)
		if err != nil {
			return scopedSetupError(opts.out, &report, fmt.Errorf("list packages: %w", err))
		}
		report.AllPackages = packages
	}
	var binary, coreDir, integrationBinary, integrationDir string
	if !opts.quick && opts.workers == 3 {
		binary, coreDir, err = prepareCore(ctx, opts.out)
		if err != nil {
			return scopedSetupError(opts.out, &report, err)
		}
		report.CoreTests, err = listCoreTests(ctx, binary, coreDir)
		if err != nil {
			return scopedSetupError(opts.out, &report, err)
		}
		report.IntegrationPackage = boundaryPackage
		integrationBinary, integrationDir, err = prepareTestPackage(ctx, opts.out, boundaryPackage, "integration")
		if err != nil {
			return scopedSetupError(opts.out, &report, err)
		}
		report.IntegrationTests, err = listCoreTests(ctx, integrationBinary, integrationDir)
		if err != nil {
			return scopedSetupError(opts.out, &report, err)
		}
	}
	report.PreparationSecs = time.Since(started).Seconds()
	execution := time.Now()
	if opts.quick {
		report.CoreTests = append([]string(nil), quickTests...)
		report.Groups = runQuickGroups(opts.workers, quickInvocations(opts), func(inv scopedInvocation) scopedGroup {
			return runGroup(ctx, opts.out, inv.ID, "", inv.Command, inv.Assigned, inv.Packages, true)
		})
	} else {
		weights, e := loadWeights()
		if e != nil {
			return scopedSetupError(opts.out, &report, e)
		}
		assignments := assignTests(report.CoreTests, weights, 3)
		integrationAssignments := assignTests(report.IntegrationTests, weights, 3)
		integrationGroups := make([]scopedGroup, 3)
		others := make([]string, 0, len(packages)-1)
		for _, pkg := range packages {
			if pkg != fullCorePackage && pkg != boundaryPackage {
				others = append(others, pkg)
			}
		}
		groups := runFullGroups(len(assignments), func(i int) scopedGroup {
			id := fmt.Sprintf("core-%d", i+1)
			args := []string{"go", "tool", "test2json", "-t", "-p", fullCorePackage, binary, "-test.v=test2json", "-test.count=1", "-test.parallel=1", "-test.timeout=45m", "-test.run=" + exactPattern(assignments[i])}
			core := runGroup(ctx, opts.out, id, coreDir, args, assignments[i], []string{fullCorePackage}, true)
			id = fmt.Sprintf("integration-%d", i+1)
			args = []string{"go", "tool", "test2json", "-t", "-p", boundaryPackage, integrationBinary, "-test.v=test2json", "-test.count=1", "-test.parallel=1", "-test.timeout=45m", "-test.run=" + exactPattern(integrationAssignments[i])}
			integrationGroups[i] = runGroup(ctx, opts.out, id, integrationDir, args, integrationAssignments[i], []string{boundaryPackage}, true)
			return core
		}, func() scopedGroup {
			args := append([]string{"go", "test", "-json", "-count=1", "-timeout=45m", "-p", "1", "-parallel", "1"}, others...)
			return runGroup(ctx, opts.out, "other-packages", "", args, nil, others, false)
		})
		report.Groups = append(report.Groups, groups...)
		report.Groups = append(report.Groups, integrationGroups...)
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

type scopedInvocation struct {
	ID       string
	Command  []string
	Assigned []string
	Packages []string
}

func quickInvocations(opts options) []scopedInvocation {
	var invocations []scopedInvocation
	add := func(id, pattern string, assigned, packages []string) {
		args := []string{"go", "test", "-json", "-count=1", "-timeout=3m", "-p=1", "-parallel=1"}
		if pattern != "" {
			args = append(args, "-run="+pattern)
		}
		invocations = append(invocations, scopedInvocation{id, append(args, packages...), assigned, packages})
	}
	add("quick-core", exactPattern(quickTests), quickTests, []string{corePackage})
	add("quick-rules", "", nil, quickRulePackages)
	for _, owner := range quickOwnerCases {
		add(owner.ID, exactPattern(owner.Tests), owner.Tests, []string{owner.Package})
	}
	add("quick-boundary", quickBoundaryPattern(), quickBoundaryAssigned, []string{boundaryPackage})
	if len(opts.goTestArgs) > 0 {
		args := append([]string{"go", "test"}, opts.goTestArgs...)
		if !hasJSONFlag(args) {
			args = append(args, "-json")
		}
		for _, flag := range []string{"-count", "-p", "-parallel"} {
			if flagValue(args, flag, "") == "" {
				args = append(args, flag+"=1")
			}
		}
		// An explicit integration selection is additional regression, never a
		// replacement for the fixed L1 package/test coverage.
		invocations = append(invocations, scopedInvocation{ID: "quick-targeted", Command: args})
	}
	return invocations
}

func runQuickGroups(workers int, invocations []scopedInvocation, run func(scopedInvocation) scopedGroup) []scopedGroup {
	groups := make([]scopedGroup, len(invocations))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				groups[index] = run(invocations[index])
			}
		}()
	}
	for index := range invocations {
		jobs <- index
	}
	close(jobs)
	wg.Wait()
	return groups
}

// The remaining packages have their own serial Go command. Starting it alongside
// the three execution workers keeps both owners within the same Full deadline.
func runFullGroups(workers int, runCore func(int) scopedGroup, runOthers func() scopedGroup) []scopedGroup {
	groups := make([]scopedGroup, workers+1)
	var wg sync.WaitGroup
	wg.Add(workers + 1)
	for i := 0; i < workers; i++ {
		go func(i int) {
			defer wg.Done()
			groups[i] = runCore(i)
		}(i)
	}
	go func() {
		defer wg.Done()
		groups[workers] = runOthers()
	}()
	wg.Wait()
	return groups
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
	for _, required := range []string{corePackage, fullCorePackage, boundaryPackage} {
		found := false
		for _, pkg := range packages {
			if pkg == required {
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("test owner package missing from go list: %s", required)
		}
	}
	return packages, nil
}

func prepareCore(ctx context.Context, out string) (string, string, error) {
	return prepareTestPackage(ctx, out, fullCorePackage, "core")
}

func prepareTestPackage(ctx context.Context, out, pkg, name string) (string, string, error) {
	dirBytes, err := exec.CommandContext(ctx, "go", "list", "-f", "{{.Dir}}", pkg).CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("locate core: %w: %s", err, dirBytes)
	}
	dir := strings.TrimSpace(string(dirBytes))
	binary, err := filepath.Abs(filepath.Join(out, name+".test"))
	if err != nil {
		return "", "", err
	}
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	output, err := exec.CommandContext(ctx, "go", "test", "-c", "-o", binary, pkg).CombinedOutput()
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
	group.StartedAt = begin.UTC().Format(time.RFC3339Nano)
	err = runOwned(ctx, cmd)
	finished := time.Now()
	group.FinishedAt = finished.UTC().Format(time.RFC3339Nano)
	group.WallSeconds = finished.Sub(begin).Seconds()
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
	if report.Scope == "full" && report.CorePackage != fullCorePackage {
		failures = append(failures, "full core package differs from the execution test owner")
	}
	if report.Scope == "full" && (report.IntegrationPackage != boundaryPackage || len(report.IntegrationTests) == 0) {
		failures = append(failures, "full integration partition differs from the integration test owner")
	}
	seenPackages := map[string]int{}
	seenCore := map[string]int{}
	seenIntegration := map[string]int{}
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
				pkg := corePackage
				if len(group.Packages) == 1 {
					pkg = group.Packages[0]
				}
				key := pkg + "\x00" + name
				if runs[key] != 1 || terminals[key] != 1 {
					failures = append(failures, fmt.Sprintf("%s: missing assigned test %s", group.ID, name))
				}
			}
			if strings.HasPrefix(group.ID, "core-") || strings.HasPrefix(group.ID, "integration-") || group.ID == "quick-core" || group.ID == "quick-boundary" {
				expectedPackage := corePackage
				if strings.HasPrefix(group.ID, "core-") {
					expectedPackage = fullCorePackage
				}
				if strings.HasPrefix(group.ID, "integration-") || group.ID == "quick-boundary" {
					expectedPackage = boundaryPackage
				}
				for key := range runs {
					pkg, test, _ := strings.Cut(key, "\x00")
					checkName := !strings.Contains(test, "/") || (group.ID == "quick-boundary" && strings.Count(test, "/") == 1)
					if pkg != expectedPackage || (checkName && !assigned[test]) {
						failures = append(failures, group.ID+": unexpected test "+key)
					}
				}
			}
		}
		if group.ID == "quick-boundary" && (packages[boundaryPackage] != 1 || len(packages) != 1) {
			failures = append(failures, "quick-boundary: package set differs")
		}
		for pkg, count := range packages {
			if count != 1 {
				failures = append(failures, fmt.Sprintf("%s: package %s has %d terminal events", group.ID, pkg, count))
			}
			if group.ID == "other-packages" || group.ID == "quick-core" || group.ID == "quick-rules" || (group.ID == "quick-boundary" || strings.HasPrefix(group.ID, "quick-owner-")) || group.ID == "quick-execution" || group.ID == "quick-execution-boundary" {
				seenPackages[pkg] += count
			}
		}
		if strings.HasPrefix(group.ID, "core-") {
			if packages[fullCorePackage] != 1 || len(packages) != 1 {
				failures = append(failures, group.ID+": core package has no terminal")
			}
			for key, count := range runs {
				pkg, test, _ := strings.Cut(key, "\x00")
				if pkg == fullCorePackage && !strings.Contains(test, "/") {
					seenCore[test] += count
				}
			}
		}
		if strings.HasPrefix(group.ID, "integration-") {
			if packages[boundaryPackage] != 1 || len(packages) != 1 {
				failures = append(failures, group.ID+": integration package has no unique terminal")
			}
			for key, count := range runs {
				pkg, test, _ := strings.Cut(key, "\x00")
				if pkg == boundaryPackage && !strings.Contains(test, "/") {
					seenIntegration[test] += count
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
		for _, name := range report.IntegrationTests {
			if seenIntegration[name] != 1 {
				failures = append(failures, fmt.Sprintf("integration test %s ran %d times", name, seenIntegration[name]))
			}
		}
		if len(seenIntegration) != len(report.IntegrationTests) {
			failures = append(failures, "integration top-level test set differs from enumeration")
		}
		for _, pkg := range report.AllPackages {
			if pkg != fullCorePackage && pkg != boundaryPackage && seenPackages[pkg] != 1 {
				failures = append(failures, fmt.Sprintf("package %s completed %d times", pkg, seenPackages[pkg]))
			}
		}
		if len(seenPackages) != len(report.AllPackages)-2 {
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

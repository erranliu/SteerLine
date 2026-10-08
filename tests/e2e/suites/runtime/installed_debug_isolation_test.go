//go:build e2e

package runtime

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/tests/e2e/internal/testenv"
)

type serviceStart struct {
	Status       string `json:"status"`
	LauncherPath string `json:"launcher_path"`
	ConfigPath   string `json:"config_path"`
}

func TestInstalledDebugIsolation(t *testing.T) {
	s := testenv.NewScenario(t, "runtime.installed-debug-isolation")
	rootPart := filepath.Join("中文 路径", "有空格的现场")
	installed := s.NewProjectAt("installed", rootPart, "")
	debug := s.NewProjectAt("debug", rootPart, "")
	if installed.Repo == debug.Repo || installed.Home == debug.Home {
		t.Fatal("controlled setup did not create independent project clones and homes")
	}
	unrelatedCWD := filepath.Join(s.Root, "无关 cwd", "深层目录")
	s.Must(os.MkdirAll(unrelatedCWD, 0700))
	installed.Driver.WorkingDirectory = unrelatedCWD
	debug.Driver.WorkingDirectory = unrelatedCWD

	if _, err := installed.Driver.Run(s.Context, "install"); err != nil {
		t.Fatal(err)
	}
	if _, err := installed.Driver.Run(s.Context, "launchers"); err != nil {
		t.Fatal(err)
	}
	installedExe := filepath.Join(installed.Home, "bin", "steerline.exe")
	installed.Executable, installed.Driver.Binary = installedExe, installedExe
	installedHash := fileSHA256(t, installedExe)
	installed.CreateTasks()
	installed.StartService()
	installedBefore, err := installed.Snapshot()
	s.Must(err)
	installedIdentity := assertServiceIdentity(t, installed, installedExe, s.Run.SourceCommit, s.Run.SourceDirty)
	evidence := map[string]any{
		"source_commit": s.Run.SourceCommit,
		"source_dirty":  s.Run.SourceDirty,
		"installed": map[string]any{
			"executable": installedExe, "home": installed.Home, "config": installed.Config, "repository": installed.Repo,
			"sha256_before": installedHash, "service_instance_id": serviceInstance(installedBefore), "startup_diagnostics": installedIdentity,
		},
		"debug": map[string]any{"home": debug.Home, "config": debug.Config, "repository": debug.Repo},
	}
	writeRuntimeEvidence(t, s.Output, evidence)

	powershell := powershellExecutable(t)
	debugScript := filepath.Join(s.Run.Root, "scripts", "debug-runtime.ps1")
	if _, err := runDebugScript(s, debug, powershell, debugScript, installed.Home, "build"); err != nil {
		t.Fatal(err)
	}
	if _, err := runDebugScript(s, debug, powershell, debugScript, installed.Home, "install"); err != nil {
		t.Fatal(err)
	}
	if _, err := runDebugScript(s, debug, powershell, debugScript, installed.Home, "launchers"); err != nil {
		t.Fatal(err)
	}
	if _, err := runDebugScript(s, debug, powershell, debugScript, installed.Home, "launchers"); err != nil {
		t.Fatal(err)
	}
	debugExe := filepath.Join(debug.Home, "bin", "steerline.exe")
	debug.Executable, debug.Driver.Binary = debugExe, debugExe
	if fileSHA256(t, installedExe) != installedHash {
		t.Fatal("debug build or launcher generation changed the installed executable")
	}
	installedAfterFirstBuild := mustSnapshot(t, installed)
	assertProjectFactsEqual(t, installedBefore, installedAfterFirstBuild, "debug build changed the running installed project")
	assertSameServiceInstance(t, installedBefore, installedAfterFirstBuild)
	installedEvidence := evidence["installed"].(map[string]any)
	installedEvidence["sha256_after_first_debug_build"] = fileSHA256(t, installedExe)
	installedEvidence["service_instance_id_after_first_debug_build"] = serviceInstance(installedAfterFirstBuild)
	writeRuntimeEvidence(t, s.Output, evidence)

	cliOutput, err := runDebugScript(s, debug, powershell, debugScript, installed.Home, "cli", "home")
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, string(cliOutput), s.Run.SourceCommit)
	assertContains(t, string(cliOutput), s.Run.SourceDirty)
	productCLI, err := debug.Driver.Run(s.Context, "home", "-config", debug.Config)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, string(productCLI), debug.Home)
	assertContains(t, string(productCLI), debugExe)
	assertContains(t, string(productCLI), s.Run.SourceCommit)
	assertContains(t, string(productCLI), s.Run.SourceDirty)
	assertContains(t, string(productCLI), debug.Config)
	assertRuntimePathsInsideHome(t, debug)
	assertGeneratedLaunchers(t, debug.Home)
	debugEvidence := evidence["debug"].(map[string]any)
	debugEvidence["executable"] = debugExe
	debugEvidence["cli_home_output"] = map[string]string{"home": debug.Home, "executable": debugExe, "config": debug.Config, "source_commit": s.Run.SourceCommit, "source_dirty": s.Run.SourceDirty}
	debugEvidence["executable_sha256"] = fileSHA256(t, debugExe)
	writeRuntimeEvidence(t, s.Output, evidence)

	startJSON, err := debug.Driver.Run(s.Context, "start-service", "-config", debug.Config)
	if err != nil {
		t.Fatal(err)
	}
	var requested serviceStart
	if err := json.Unmarshal(startJSON, &requested); err != nil {
		t.Fatalf("decode debug service entry: %v: %s", err, startJSON)
	}
	if requested.Status != "window-required" || requested.LauncherPath != filepath.Join(debug.Home, "service-window.bat") || requested.ConfigPath != debug.Config {
		t.Fatalf("console/service entry did not stay in debug home: %+v", requested)
	}

	debug.CreateTasks()
	debug.StartService()
	debugIdentity := assertServiceIdentity(t, debug, debugExe, s.Run.SourceCommit, s.Run.SourceDirty)
	debugEvidence["startup_diagnostics"] = debugIdentity
	debugHashBeforeRunningBuild := fileSHA256(t, debugExe)
	buildOutput, buildErr := runDebugScript(s, debug, powershell, debugScript, installed.Home, "build")
	if buildErr == nil || !strings.Contains(strings.ToLower(string(buildOutput)), "debug executable is still running") {
		t.Fatalf("debug build was allowed while its service executable was running: err=%v output=%s", buildErr, buildOutput)
	}
	if fileSHA256(t, debugExe) != debugHashBeforeRunningBuild {
		t.Fatal("rejected build changed the running debug executable")
	}
	debugEvidence["build_while_debug_service_running_rejected"] = string(buildOutput)
	debugBefore, err := debug.Snapshot()
	s.Must(err)
	debugEvidence["service_instance_id"] = serviceInstance(debugBefore)
	writeRuntimeEvidence(t, s.Output, evidence)
	if debugBefore.Page.ProjectID == installedBefore.Page.ProjectID || debugBefore.Page.DatabaseID == installedBefore.Page.DatabaseID {
		t.Fatal("installed and debug homes shared a project or database identity")
	}
	requestsBeforeStaleTarget := controlRequestCount(t, debug)
	staleTargetErr := debug.Driver.ControlTargeted(s.Context, s.Run.Helper, "start_task", debug.Tasks["a"].ID, serviceInstance(debugBefore)+"-stale")
	if staleTargetErr == nil || !strings.Contains(staleTargetErr.Error(), "stale_instance") {
		t.Fatalf("write targeting a stale service instance was not refused: %v", staleTargetErr)
	}
	if count := controlRequestCount(t, debug); count != requestsBeforeStaleTarget {
		t.Fatalf("stale-instance rejection published a control request: before=%d after=%d", requestsBeforeStaleTarget, count)
	}
	debugEvidence["stale_instance_write_rejected"] = staleTargetErr.Error()
	writeRuntimeEvidence(t, s.Output, evidence)

	staleConfig := writeConfigVariant(t, debug, "Stale.json", func(config map[string]any) {
		config["heartbeat"] = "250ms"
	})
	staleDriver := debug.Driver
	staleDriver.Config = staleConfig
	requestsBeforeStaleConfig := controlRequestCount(t, debug)
	staleWriteErr := staleDriver.Control(s.Context, s.Run.Helper, "start_task", debug.Tasks["a"].ID)
	if staleWriteErr == nil {
		t.Fatal("a write command using a changed configuration reached the running service")
	}
	debugEvidence["changed_config_write_rejected"] = staleWriteErr.Error()
	if count := controlRequestCount(t, debug); count != requestsBeforeStaleConfig {
		t.Fatalf("changed-config rejection published a control request: before=%d after=%d", requestsBeforeStaleConfig, count)
	}
	writeRuntimeEvidence(t, s.Output, evidence)
	debugAfterRejectedWrite := mustSnapshot(t, debug)
	if len(debugAfterRejectedWrite.Domain.WorkflowRuns) != 0 {
		t.Fatal("rejected wrong-configuration write created a workflow")
	}

	mirror := s.NewHomeForProject(debug, "same-repository-other-home", debugExe)
	lockCommand := mirror.Command(debugExe, "run", "-config", mirror.Config, "-dispatch", "manual", "-console", "off")
	lockOutput, lockErr := s.RunCommandFor(lockCommand, "same-repository-second-writer", 10*time.Second)
	if lockErr == nil || !strings.Contains(string(lockOutput), "repository already locked") {
		t.Fatalf("same-clone second home did not preserve repository lock: err=%v output=%s", lockErr, lockOutput)
	}
	debugEvidence["cross_home_second_writer_rejected"] = string(lockOutput)
	writeRuntimeEvidence(t, s.Output, evidence)

	installed.Claim("a")
	installedOpen := installed.WaitPhase("waiting_pr_merge")
	installed.Record("waiting-pr-merge", installedOpen)
	debug.Claim("a")
	debugOpen := debug.WaitPhase("waiting_pr_merge")
	debug.Record("waiting-pr-merge", debugOpen)
	if installedOpen.Helper.Dispatches != 1 || debugOpen.Helper.Dispatches != 1 || installedOpen.Page.ProjectID == debugOpen.Page.ProjectID {
		t.Fatal("writes or controlled worker state crossed between install and debug clones")
	}
	installed.Merge()
	debug.Merge()
	installedDone := installed.WaitPhase("completed")
	debugDone := debug.WaitPhase("completed")
	installed.Record("completed", installedDone)
	debug.Record("completed", debugDone)
	installed.StopService()
	debug.StopService()

	if _, err := runDebugScript(s, debug, powershell, debugScript, installed.Home, "build"); err != nil {
		t.Fatal(err)
	}
	if _, err := runDebugScript(s, debug, powershell, debugScript, installed.Home, "install"); err != nil {
		t.Fatal(err)
	}
	if _, err := runDebugScript(s, debug, powershell, debugScript, installed.Home, "launchers"); err != nil {
		t.Fatal(err)
	}
	if _, err := runDebugScript(s, debug, powershell, debugScript, installed.Home, "launchers"); err != nil {
		t.Fatal(err)
	}
	if fileSHA256(t, installedExe) != installedHash {
		t.Fatal("debug stop/start preparation changed the installed executable SHA-256")
	}
	installedAfterBuild, err := installed.Snapshot()
	s.Must(err)
	assertProjectFactsEqual(t, installedDone, installedAfterBuild, "debug rebuild changed installed task facts")
	assertSameServiceInstance(t, installedBefore, installedAfterBuild)
	installedEvidence["sha256_after_debug_restart_preparation"] = fileSHA256(t, installedExe)
	installedEvidence["service_instance_id_after_debug_restart_preparation"] = serviceInstance(installedAfterBuild)
	debug.StartService()
	debugAfterRestartIdentity := assertServiceIdentity(t, debug, debugExe, s.Run.SourceCommit, s.Run.SourceDirty)
	debugAfterRestart, err := debug.Snapshot()
	s.Must(err)
	assertProjectFactsEqual(t, debugDone, debugAfterRestart, "same-build debug restart changed task facts")
	if serviceInstance(debugDone) == "" || serviceInstance(debugAfterRestart) == "" || serviceInstance(debugDone) == serviceInstance(debugAfterRestart) {
		t.Fatalf("same-build debug restart did not publish a new service instance: before=%s after=%s", serviceInstance(debugDone), serviceInstance(debugAfterRestart))
	}
	debugEvidence["executable_sha256_after_rebuild"] = fileSHA256(t, debugExe)
	debugEvidence["service_instance_id_after_restart"] = serviceInstance(debugAfterRestart)
	debugEvidence["restart_startup_diagnostics"] = debugAfterRestartIdentity
	if fileSHA256(t, installedExe) != installedHash {
		t.Fatal("debug restart changed installed executable SHA-256")
	}
	writeRuntimeEvidence(t, s.Output, evidence)

	assertScriptRejects(t, s, debug, powershell, debugScript, installed.Home, installed.Home, installed.Config, "overlap", "home-overlap")
	assertHomeJunctionRejected(t, s, debug, powershell, debugScript, installed)
	assertEscapedPathRejected(t, s, debug, powershell, debugScript, installed.Home, "run_dir", "run-dir-escape")
	assertEscapedPathRejected(t, s, debug, powershell, debugScript, installed.Home, "worktree_root", "worktree-root-escape")
	assertMissingDebugExecutableRejected(t, s, debug, powershell, debugScript, installed.Home)
	debug.StopService()
	installed.StopService()

}

func assertHomeJunctionRejected(t *testing.T, s *testenv.Scenario, project *testenv.Project, powershell, script string, installed *testenv.Project) {
	t.Helper()
	alias := filepath.Join(s.Root, "installed-home-alias")
	encode := func(value string) string { return base64.StdEncoding.EncodeToString([]byte(value)) }
	createScript := "$alias=[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('" + encode(alias) + "')); $target=[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('" + encode(installed.Home) + "')); New-Item -ItemType Junction -Path $alias -Target $target | Out-Null"
	if _, err := s.RunCommand(project.Command(powershell, "-NoProfile", "-Command", createScript), "create-installed-home-junction"); err != nil {
		t.Fatal(err)
	}
	installedHash := fileSHA256(t, filepath.Join(installed.Home, "bin", "steerline.exe"))
	assertScriptRejects(t, s, project, powershell, script, installed.Home, alias, installed.Config, "overlap", "home-junction-overlap")
	if fileSHA256(t, filepath.Join(installed.Home, "bin", "steerline.exe")) != installedHash {
		t.Fatal("junction alias rejection changed the isolated installed executable")
	}
	removeScript := "$alias=[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('" + encode(alias) + "')); [IO.Directory]::Delete($alias, $false)"
	if _, err := s.RunCommand(project.Command(powershell, "-NoProfile", "-Command", removeScript), "remove-installed-home-junction"); err != nil {
		t.Fatal(err)
	}
}

func powershellExecutable(t *testing.T) string {
	t.Helper()
	for _, name := range []string{"powershell.exe", "pwsh.exe", "pwsh"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	t.Fatal("Windows PowerShell is required for runtime.installed-debug-isolation")
	return ""
}

func runDebugScript(s *testenv.Scenario, project *testenv.Project, powershell, script, installedHome, operation string, args ...string) ([]byte, error) {
	commandArgs := []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script,
		"-DebugHome", project.Home, "-InstalledHome", installedHome, "-Config", project.Config, "-Operation", operation}
	if len(args) > 0 {
		commandArgs = append(commandArgs, "-Arguments", args[0])
	}
	return s.RunCommand(project.Command(powershell, commandArgs...), "debug-runtime-"+operation+"-"+filepath.Base(project.Home)+"-"+time.Now().Format("150405.000000000"))
}

func assertScriptRejects(t *testing.T, s *testenv.Scenario, project *testenv.Project, powershell, script, installedHome, debugHome, config, expected, name string) {
	t.Helper()
	cmd := project.Command(powershell, "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script,
		"-DebugHome", debugHome, "-InstalledHome", installedHome, "-Config", config, "-Operation", "cli", "-Arguments", "home")
	output, err := s.RunCommand(cmd, name)
	if err == nil || !strings.Contains(strings.ToLower(string(output)), strings.ToLower(expected)) {
		t.Fatalf("debug runtime accepted invalid %s: err=%v output=%s", name, err, output)
	}
}

func assertEscapedPathRejected(t *testing.T, s *testenv.Scenario, project *testenv.Project, powershell, script, installedHome, field, name string) {
	t.Helper()
	escape := filepath.Join(s.Root, "escaped "+field)
	config := writeConfigVariant(t, project, name+".json", func(value map[string]any) {
		value[field] = escape
	})
	output, err := runDebugScriptWithConfig(s, project, powershell, script, installedHome, config, name)
	if err == nil || !strings.Contains(strings.ToLower(string(output)), strings.ToLower(field)) {
		t.Fatalf("debug runtime accepted %s outside its home: err=%v output=%s", field, err, output)
	}
}

func runDebugScriptWithConfig(s *testenv.Scenario, project *testenv.Project, powershell, script, installedHome, config, name string) ([]byte, error) {
	cmd := project.Command(powershell, "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script,
		"-DebugHome", project.Home, "-InstalledHome", installedHome, "-Config", config, "-Operation", "cli", "-Arguments", "home")
	return s.RunCommand(cmd, name)
}

func assertMissingDebugExecutableRejected(t *testing.T, s *testenv.Scenario, project *testenv.Project, powershell, script, installedHome string) {
	t.Helper()
	missingHome := filepath.Join(s.Root, "missing debug executable")
	if err := os.MkdirAll(filepath.Join(missingHome, "etc"), 0700); err != nil {
		t.Fatal(err)
	}
	config := writeConfigVariantAt(t, project.Config, filepath.Join(missingHome, "etc", "SteerLine.json"), func(value map[string]any) {
		value["worktree_root"] = filepath.Join(missingHome, "worktrees")
	})
	cmd := project.Command(powershell, "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script,
		"-DebugHome", missingHome, "-InstalledHome", installedHome, "-Config", config, "-Operation", "cli", "-Arguments", "home")
	output, err := s.RunCommand(cmd, "missing-debug-executable")
	if err == nil || !strings.Contains(strings.ToLower(string(output)), "missing debug executable") {
		t.Fatalf("debug runtime silently fell back when its executable was missing: err=%v output=%s", err, output)
	}
}

func writeConfigVariant(t *testing.T, project *testenv.Project, name string, change func(map[string]any)) string {
	t.Helper()
	return writeConfigVariantAt(t, project.Config, filepath.Join(project.Home, "etc", name), change)
}

func writeConfigVariantAt(t *testing.T, source, target string, change func(map[string]any)) string {
	t.Helper()
	raw, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	change(config)
	raw, err = json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	return target
}

func assertServiceIdentity(t *testing.T, project *testenv.Project, executable, commit, dirty string) []string {
	t.Helper()
	log := latestServiceLog(t, project)
	for _, want := range []string{
		"runtime executable=" + executable,
		"runtime home=" + project.Home,
		"runtime config=" + project.Config,
		"runtime source_commit=" + commit + " source_dirty=" + dirty,
		"runtime repository=" + strings.ToLower(project.Repo),
	} {
		assertContains(t, string(log), want)
	}
	return runtimeDiagnosticLines(log)
}

func runtimeDiagnosticLines(log []byte) []string {
	var lines []string
	for _, line := range strings.Split(string(log), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "runtime ") {
			lines = append(lines, line)
		}
	}
	return lines
}

func serviceInstance(snapshot testenv.Snapshot) string {
	if snapshot.Runtime == nil {
		return ""
	}
	return snapshot.Runtime.Service.InstanceID
}

func controlRequestCount(t *testing.T, project *testenv.Project) int {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(filepath.Dir(project.Database), "control", "requests", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	return len(files)
}

func assertSameServiceInstance(t *testing.T, before, after testenv.Snapshot) {
	t.Helper()
	if serviceInstance(before) == "" || serviceInstance(before) != serviceInstance(after) {
		t.Fatalf("debug maintenance changed the active installed service instance: before=%s after=%s", serviceInstance(before), serviceInstance(after))
	}
}

func writeRuntimeEvidence(t *testing.T, directory string, evidence map[string]any) {
	t.Helper()
	raw, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "runtime-identities.json"), append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
}

func latestServiceLog(t *testing.T, project *testenv.Project) []byte {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(filepath.Dir(project.Database), "service", "startups", "*", "service.log"))
	if err != nil || len(files) == 0 {
		t.Fatalf("service startup log missing for %s: %v", project.Name, err)
	}
	sort.Slice(files, func(i, j int) bool {
		left, _ := os.Stat(files[i])
		right, _ := os.Stat(files[j])
		return left.ModTime().Before(right.ModTime())
	})
	raw, err := os.ReadFile(files[len(files)-1])
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func assertGeneratedLaunchers(t *testing.T, home string) {
	t.Helper()
	console, err := os.ReadFile(filepath.Join(home, "console.bat"))
	if err != nil {
		t.Fatal(err)
	}
	service, err := os.ReadFile(filepath.Join(home, "service-window.bat"))
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []struct {
		data []byte
		want string
	}{
		{console, `set "STEERLINE_HOME=%SHOME%"`},
		{console, `set "CODEX_HOME=%SHOME%\codex"`},
		{console, `%SHOME%\bin\steerline.exe`},
		{service, `set "STEERLINE_HOME=%SHOME%"`},
		{service, `set "CODEX_HOME=%SHOME%\codex"`},
		{service, `%SHOME%\bin\steerline.exe`},
	} {
		if !strings.Contains(string(value.data), value.want) {
			t.Fatalf("launcher did not bind to its own home: missing %q", value.want)
		}
	}
}

func assertRuntimePathsInsideHome(t *testing.T, project *testenv.Project) {
	t.Helper()
	for _, path := range []string{filepath.Dir(project.Database), filepath.Join(project.Home, "worktrees")} {
		rel, err := filepath.Rel(project.Home, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			t.Fatalf("runtime path escaped home: home=%s path=%s rel=%s err=%v", project.Home, path, rel, err)
		}
	}
}

func assertProjectFactsEqual(t *testing.T, before, after testenv.Snapshot, message string) {
	t.Helper()
	if before.Page.ProjectID != after.Page.ProjectID || before.Page.DatabaseID != after.Page.DatabaseID || len(before.Domain.WorkflowRuns) != len(after.Domain.WorkflowRuns) || len(before.Domain.Executions) != len(after.Domain.Executions) {
		t.Fatal(message)
	}
	for i := range before.Domain.WorkflowRuns {
		left, right := before.Domain.WorkflowRuns[i], after.Domain.WorkflowRuns[i]
		if left.WorkflowRunID != right.WorkflowRunID || left.TaskID != right.TaskID || left.Phase != right.Phase {
			t.Fatal(message)
		}
	}
	for i := range before.Page.Tasks {
		left, right := before.Page.Tasks[i], after.Page.Tasks[i]
		if left.TaskID != right.TaskID || left.Status != right.Status {
			t.Fatal(message)
		}
	}
}

func fileSHA256(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func assertContains(t *testing.T, value, expected string) {
	t.Helper()
	if !strings.Contains(value, expected) {
		t.Fatalf("output does not contain %q:\n%s", expected, value)
	}
}

func mustSnapshot(t *testing.T, project *testenv.Project) testenv.Snapshot {
	t.Helper()
	snapshot, err := project.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

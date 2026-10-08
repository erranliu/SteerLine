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
	if _, err := runDebugScript(s, debug, powershell, debugScript, installed.Home, "launchers"); err != nil {
		t.Fatal(err)
	}
	if _, err := runDebugScript(s, debug, powershell, debugScript, installed.Home, "launchers"); err != nil {
		t.Fatal(err)
	}
	debugExe := filepath.Join(s.Run.Root, "bin", "steerline.exe")
	debug.Executable, debug.Driver.Binary = debugExe, debugExe
	debugEvidence := evidence["debug"].(map[string]any)
	assertDefaultStateAliasRejected(t, s, debug, installed, debugExe, powershell, debugScript, debugEvidence)
	assertDefaultUIAliasRejected(t, s, debug, installed, debugExe, powershell, debugScript, debugEvidence)
	if _, err := os.Stat(filepath.Join(debug.Home, "bin", "steerline.exe")); !os.IsNotExist(err) {
		t.Fatalf("debug runtime unexpectedly copied the source exe into its home: %v", err)
	}
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
	assertGeneratedLaunchers(t, debug.Home, debugExe)
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
	debugEvidence["start_service_window_required"] = requested

	debug.CreateTasks()
	serviceCommand := debug.Command(powershell, "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", debugScript,
		"-DebugHome", debug.Home, "-InstalledHome", installed.Home, "-Config", debug.Config, "-Operation", "service")
	debug.StartServiceViaVisibleConsole(serviceCommand, "debug-operation-service-window-1")
	debugIdentity := assertServiceIdentity(t, debug, debugExe, s.Run.SourceCommit, s.Run.SourceDirty)
	debugEvidence["startup_diagnostics"] = debugIdentity
	debugSnapshot, err := debug.Snapshot()
	s.Must(err)
	if debugSnapshot.Runtime == nil || debugSnapshot.Runtime.Service.PID <= 0 || debugSnapshot.Runtime.Service.ProcessIdentity == nil ||
		debugSnapshot.Runtime.Service.ProcessIdentity.PID != debugSnapshot.Runtime.Service.PID ||
		!strings.EqualFold(debugSnapshot.Runtime.Service.ProcessIdentity.Executable, debugExe) || debugSnapshot.Runtime.Service.StartupID == nil {
		var identity any
		var servicePID int
		if debugSnapshot.Runtime != nil {
			identity = debugSnapshot.Runtime.Service.ProcessIdentity
			servicePID = debugSnapshot.Runtime.Service.PID
		}
		t.Fatalf("debug service-window child identity is missing or wrong: pid=%d identity=%+v want_executable=%s", servicePID, identity, debugExe)
	}
	debugEvidence["service_window_entry"] = map[string]any{
		"entry":        "debug-runtime.ps1 -Operation service -> generated service-window.bat",
		"launcher_pid": debug.ServiceWindow.Cmd.Process.Pid,
		"service_pid":  debugSnapshot.Runtime.Service.PID,
		"executable":   debugSnapshot.Runtime.Service.ProcessIdentity.Executable,
		"home":         debug.Home, "config": debug.Config, "instance_id": serviceInstance(debugSnapshot),
		"startup_log": filepath.Join(filepath.Dir(debug.Database), "service", "startups", *debugSnapshot.Runtime.Service.StartupID, "service.log"),
	}
	consoleBatch := filepath.Join(debug.Home, "console.bat")
	consoleCommand := debug.Command("cmd.exe", "/d", "/c", "call", consoleBatch, "--steerline-console-child", debug.Config)
	consoleOutput, consoleErr := s.RunPseudoConsole(consoleCommand, "debug-runtime-console", s.Run.SourceCommit)
	if consoleErr != nil {
		t.Fatalf("run generated debug console.bat under ConPTY: %v\n%s", consoleErr, consoleOutput)
	}
	for _, expected := range []string{debugExe, debug.Home, debug.Config, s.Run.SourceCommit, s.Run.SourceDirty} {
		assertContains(t, string(consoleOutput), expected)
	}
	debugEvidence["console_runtime_identity"] = map[string]string{"entry": "generated console.bat --steerline-console-child via Windows ConPTY", "executable": debugExe, "home": debug.Home, "config": debug.Config, "source_commit": s.Run.SourceCommit, "source_dirty": s.Run.SourceDirty, "transcript": filepath.Join(s.Output, "debug-runtime-console.log")}
	writeRuntimeEvidence(t, s.Output, evidence)
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
	requestsAfterStaleTarget := controlRequestCount(t, debug)
	if count := requestsAfterStaleTarget; count != requestsBeforeStaleTarget {
		t.Fatalf("stale-instance rejection published a control request: before=%d after=%d", requestsBeforeStaleTarget, count)
	}
	debugEvidence["stale_instance_write_rejected"] = staleTargetErr.Error()
	debugEvidence["stale_instance_request_counts"] = map[string]int{"before": requestsBeforeStaleTarget, "after": requestsAfterStaleTarget}
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
	requestsAfterStaleConfig := controlRequestCount(t, debug)
	if count := requestsAfterStaleConfig; count != requestsBeforeStaleConfig {
		t.Fatalf("changed-config rejection published a control request: before=%d after=%d", requestsBeforeStaleConfig, count)
	}
	debugEvidence["changed_config_request_counts"] = map[string]int{"before": requestsBeforeStaleConfig, "after": requestsAfterStaleConfig}
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
	requestsBeforeClaim := controlRequestFiles(t, debug)
	debug.Claim("a")
	debugEvidence["normal_control_publication"] = assertControlPublication(t, debug, requestsBeforeClaim, debugBefore)
	writeRuntimeEvidence(t, s.Output, evidence)
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
	assertServiceStopReceipt(t, debug.ServiceStopOutput)
	debugEvidence["operation_service_stop"] = map[string]any{"launcher_pid": debug.ServiceWindowPID, "owned_console_stopped": debug.ServiceWindowStopped, "receipt": string(debug.ServiceStopOutput)}
	writeRuntimeEvidence(t, s.Output, evidence)

	if _, err := runDebugScript(s, debug, powershell, debugScript, installed.Home, "build"); err != nil {
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
	serviceBatchCommand := debug.Command("cmd.exe", "/d", "/c", "call", filepath.Join(debug.Home, "service-window.bat"), debug.Config)
	debug.StartServiceViaVisibleConsole(serviceBatchCommand, "debug-generated-service-window-batch-2")
	debugAfterRestartIdentity := assertServiceIdentity(t, debug, debugExe, s.Run.SourceCommit, s.Run.SourceDirty)
	debugAfterRestart, err := debug.Snapshot()
	s.Must(err)
	if debugAfterRestart.Runtime == nil || debugAfterRestart.Runtime.Service.PID <= 0 || debugAfterRestart.Runtime.Service.ProcessIdentity == nil ||
		debugAfterRestart.Runtime.Service.ProcessIdentity.PID != debugAfterRestart.Runtime.Service.PID ||
		!strings.EqualFold(debugAfterRestart.Runtime.Service.ProcessIdentity.Executable, debugExe) || debugAfterRestart.Runtime.Service.StartupID == nil {
		var identity any
		var servicePID int
		if debugAfterRestart.Runtime != nil {
			identity = debugAfterRestart.Runtime.Service.ProcessIdentity
			servicePID = debugAfterRestart.Runtime.Service.PID
		}
		t.Fatalf("direct service-window batch child identity is missing or wrong: pid=%d identity=%+v want_executable=%s", servicePID, identity, debugExe)
	}
	assertProjectFactsEqual(t, debugDone, debugAfterRestart, "same-build debug restart changed task facts")
	if serviceInstance(debugDone) == "" || serviceInstance(debugAfterRestart) == "" || serviceInstance(debugDone) == serviceInstance(debugAfterRestart) {
		t.Fatalf("same-build debug restart did not publish a new service instance: before=%s after=%s", serviceInstance(debugDone), serviceInstance(debugAfterRestart))
	}
	debugEvidence["executable_sha256_after_rebuild"] = fileSHA256(t, debugExe)
	debugEvidence["service_instance_id_after_restart"] = serviceInstance(debugAfterRestart)
	debugEvidence["restart_startup_diagnostics"] = debugAfterRestartIdentity
	debugEvidence["generated_service_window_batch"] = map[string]any{
		"entry":        "cmd.exe /c call debug-home\\service-window.bat <project-config>",
		"launcher_pid": debug.ServiceWindow.Cmd.Process.Pid,
		"service_pid":  debugAfterRestart.Runtime.Service.PID,
		"executable":   debugAfterRestart.Runtime.Service.ProcessIdentity.Executable,
		"home":         debug.Home, "config": debug.Config, "instance_id": serviceInstance(debugAfterRestart),
		"startup_log":         filepath.Join(filepath.Dir(debug.Database), "service", "startups", *debugAfterRestart.Runtime.Service.StartupID, "service.log"),
		"startup_diagnostics": debugAfterRestartIdentity,
	}
	if fileSHA256(t, installedExe) != installedHash {
		t.Fatal("debug restart changed installed executable SHA-256")
	}
	writeRuntimeEvidence(t, s.Output, evidence)

	assertScriptRejects(t, s, debug, powershell, debugScript, installed.Home, installed.Home, installed.Config, "overlap", "home-overlap")
	assertHomeJunctionRejected(t, s, debug, powershell, debugScript, installed)
	assertEscapedPathRejected(t, s, debug, powershell, debugScript, installed.Home, "run_dir", "run-dir-escape")
	assertEscapedPathRejected(t, s, debug, powershell, debugScript, installed.Home, "worktree_root", "worktree-root-escape")
	debug.StopService()
	installed.StopService()
	assertServiceStopReceipt(t, debug.ServiceStopOutput)
	debugEvidence["generated_service_window_batch_stop"] = map[string]any{"launcher_pid": debug.ServiceWindowPID, "owned_console_stopped": debug.ServiceWindowStopped, "receipt": string(debug.ServiceStopOutput)}
	writeRuntimeEvidence(t, s.Output, evidence)
	assertDirectBuildOutputIsolation(t, s, debug, installed, powershell, evidence)
	assertMissingSourceExecutableRejected(t, s, debug, powershell, debugScript, installed.Home)

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

func assertDefaultStateAliasRejected(t *testing.T, s *testenv.Scenario, project, installed *testenv.Project, executable, powershell, script string, evidence map[string]any) {
	t.Helper()
	config := writeConfigVariant(t, project, "DefaultState.json", func(value map[string]any) {
		delete(value, "run_dir")
		delete(value, "worktree_root")
	})
	homeOutput, err := project.Driver.Run(s.Context, "home", "-config", config)
	s.Must(err)
	defaultRunDir := ""
	for _, line := range strings.Split(string(homeOutput), "\n") {
		if strings.HasPrefix(line, "run_dir") {
			defaultRunDir = strings.TrimSpace(strings.TrimPrefix(line, "run_dir"))
		}
	}
	if defaultRunDir == "" || !filepath.IsAbs(defaultRunDir) {
		t.Fatalf("default state path was not reported: %s", homeOutput)
	}
	installedState := filepath.Join(installed.Home, "state")
	sentinel := filepath.Join(installedState, "task83-state-alias-sentinel.txt")
	if err := os.WriteFile(sentinel, []byte("installed-state-sentinel"), 0600); err != nil {
		t.Fatal(err)
	}
	sentinelHash := fileSHA256(t, sentinel)
	aliasedRunDir := filepath.Join(installedState, filepath.Base(defaultRunDir))
	if _, err := os.Stat(aliasedRunDir); !os.IsNotExist(err) {
		t.Fatalf("aliased default state already exists: %s: %v", aliasedRunDir, err)
	}
	invalidBatch := filepath.Join(s.Root, "invalid-default-state-batch.json")
	if err := os.WriteFile(invalidBatch, []byte("{}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(project.Home, "state")
	var scriptOutput, productOutput []byte
	withJunction(t, s, project, powershell, alias, installedState, "default-state-alias", func() {
		var scriptErr error
		scriptOutput, scriptErr = runDebugScriptWithConfig(s, project, powershell, script, installed.Home, config, "default-state-script-alias")
		if scriptErr == nil || !strings.Contains(strings.ToLower(string(scriptOutput)), "writable path must resolve inside its home") {
			t.Fatalf("debug runtime accepted default state junction: err=%v output=%s", scriptErr, scriptOutput)
		}
		productCommand := project.Command(executable, "task", "validate", "-config", config, "-file", invalidBatch)
		var productErr error
		productOutput, productErr = s.RunCommand(productCommand, "default-state-native-alias")
		if productErr == nil || !strings.Contains(strings.ToLower(string(productOutput)), "state root must resolve inside steerline_home") {
			t.Fatalf("native task entry did not reject default state junction before initialization: err=%v output=%s", productErr, productOutput)
		}
		if _, err := os.Stat(aliasedRunDir); !os.IsNotExist(err) {
			t.Fatalf("rejected task validation created aliased state directory %s: %v", aliasedRunDir, err)
		}
		if actual := fileSHA256(t, sentinel); actual != sentinelHash {
			t.Fatalf("rejected task validation changed installed state sentinel: before=%s after=%s", sentinelHash, actual)
		}
	})
	evidence["default_state_alias_rejected"] = map[string]any{
		"alias": alias, "installed_state": installedState, "default_run_dir": defaultRunDir,
		"aliased_run_dir": aliasedRunDir, "installed_sentinel": sentinel, "installed_sentinel_sha256": sentinelHash,
		"script_refusal": string(scriptOutput), "native_task_validate_refusal": string(productOutput),
		"default_database_directory_created": false,
	}
}

func assertDefaultUIAliasRejected(t *testing.T, s *testenv.Scenario, project, installed *testenv.Project, executable, powershell, script string, evidence map[string]any) {
	t.Helper()
	installedUI := filepath.Join(installed.Home, "ui")
	if err := os.MkdirAll(installedUI, 0700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(installedUI, "task83-ui-alias-sentinel.txt")
	if err := os.WriteFile(sentinel, []byte("installed-ui-sentinel"), 0600); err != nil {
		t.Fatal(err)
	}
	sentinelHash := fileSHA256(t, sentinel)
	views := filepath.Join(installedUI, "views")
	if _, err := os.Stat(views); !os.IsNotExist(err) {
		t.Fatalf("installed UI views already exist before alias test: %s: %v", views, err)
	}
	alias := filepath.Join(project.Home, "ui")
	var scriptOutput, productOutput []byte
	withJunction(t, s, project, powershell, alias, installedUI, "default-ui-alias", func() {
		var scriptErr error
		scriptOutput, scriptErr = runDebugScript(s, project, powershell, script, installed.Home, "console")
		if scriptErr == nil || !strings.Contains(strings.ToLower(string(scriptOutput)), "writable path must resolve inside its home") {
			t.Fatalf("debug runtime accepted default UI junction: err=%v output=%s", scriptErr, scriptOutput)
		}
		productCommand := project.Command(executable, "console", "-config", project.Config)
		var productErr error
		productOutput, productErr = s.RunPseudoConsole(productCommand, "default-ui-native-console-alias", "UI_ALIAS_READY")
		if productErr == nil || !strings.Contains(strings.ToLower(string(productOutput)), "ui views directory resolves outside application home") {
			t.Fatalf("native console did not reject UI junction before creating views: err=%v output=%s", productErr, productOutput)
		}
		if _, err := os.Stat(views); !os.IsNotExist(err) {
			t.Fatalf("rejected console created installed UI views directory: %v", err)
		}
		if actual := fileSHA256(t, sentinel); actual != sentinelHash {
			t.Fatalf("rejected console changed installed UI sentinel: before=%s after=%s", sentinelHash, actual)
		}
	})
	evidence["default_ui_alias_rejected"] = map[string]any{
		"alias": alias, "installed_ui": installedUI, "installed_sentinel": sentinel, "installed_sentinel_sha256": sentinelHash,
		"script_refusal": string(scriptOutput), "native_console_refusal": string(productOutput),
		"installed_views_directory_created": false,
	}
}

func withJunction(t *testing.T, s *testenv.Scenario, project *testenv.Project, powershell, alias, target, name string, body func()) {
	t.Helper()
	encode := func(value string) string { return base64.StdEncoding.EncodeToString([]byte(value)) }
	create := "$alias=[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('" + encode(alias) + "')); $target=[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('" + encode(target) + "')); New-Item -ItemType Junction -Path $alias -Target $target | Out-Null"
	if _, err := s.RunCommand(project.Command(powershell, "-NoProfile", "-Command", create), "create-"+name); err != nil {
		t.Fatalf("create %s junction: %v", name, err)
	}
	defer func() {
		remove := "$alias=[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('" + encode(alias) + "')); [IO.Directory]::Delete($alias, $false)"
		if _, err := s.RunCommand(project.Command(powershell, "-NoProfile", "-Command", remove), "remove-"+name); err != nil {
			t.Errorf("remove %s junction: %v", name, err)
		}
	}()
	body()
}

func assertDirectBuildOutputIsolation(t *testing.T, s *testenv.Scenario, project, installed *testenv.Project, powershell string, evidence map[string]any) {
	t.Helper()
	sourceRoot := filepath.Join(s.Root, "direct-build-source")
	if _, err := os.Stat(sourceRoot); !os.IsNotExist(err) {
		t.Fatalf("direct build source clone already exists: %s", sourceRoot)
	}
	cloneCommand := exec.Command("git", "clone", "--local", "--no-hardlinks", s.Run.Root, sourceRoot)
	cloneCommand.Dir = s.Root
	cloneOutput, err := s.RunCommand(cloneCommand, "clone-source-for-direct-build")
	if err != nil {
		t.Fatalf("clone source for direct build tests: %v: %s", err, cloneOutput)
	}
	s.RecordResource("source-build-clone", sourceRoot, 0)
	headCommand := exec.Command("git", "-C", sourceRoot, "rev-parse", "HEAD")
	headOutput, err := s.RunCommand(headCommand, "direct-build-source-head")
	if err != nil || strings.TrimSpace(string(headOutput)) != s.Run.SourceCommit {
		t.Fatalf("source build clone HEAD mismatch: head=%s want=%s err=%v", strings.TrimSpace(string(headOutput)), s.Run.SourceCommit, err)
	}

	installedExe := filepath.Join(installed.Home, "bin", "steerline.exe")
	installedBin := filepath.Join(installed.Home, "bin")
	installedExeSHA := fileSHA256(t, installedExe)
	installedDBSHA := fileSHA256(t, installed.Database)
	installedBinBefore := directoryFileSnapshot(t, installedBin)
	installedBefore := mustSnapshot(t, installed)
	assertInstallUnchanged := func(label string) {
		t.Helper()
		if current := fileSHA256(t, installedExe); current != installedExeSHA {
			t.Fatalf("%s changed installed executable SHA-256: before=%s after=%s", label, installedExeSHA, current)
		}
		if current := fileSHA256(t, installed.Database); current != installedDBSHA {
			t.Fatalf("%s changed installed SQLite SHA-256: before=%s after=%s", label, installedDBSHA, current)
		}
		if current := directoryFileSnapshot(t, installedBin); strings.Join(current, "\n") != strings.Join(installedBinBefore, "\n") {
			t.Fatalf("%s changed installed bin directory contents: before=%v after=%v", label, installedBinBefore, current)
		}
		after := mustSnapshot(t, installed)
		assertProjectFactsEqual(t, installedBefore, after, label+" changed installed task facts")
		assertSameServiceInstance(t, installedBefore, after)
	}
	buildScript := filepath.Join(sourceRoot, "build.ps1")
	runBuild := func(name string) ([]byte, error) {
		cmd := project.Command(powershell, "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", buildScript)
		return s.RunCommand(cmd, name)
	}
	cloneBin := filepath.Join(sourceRoot, "bin")
	if _, err := os.Stat(cloneBin); !os.IsNotExist(err) {
		t.Fatalf("fresh source clone unexpectedly contains bin output: %s: %v", cloneBin, err)
	}
	var junctionOutput []byte
	withJunction(t, s, project, powershell, cloneBin, installedBin, "direct-build-bin-junction", func() {
		var buildErr error
		junctionOutput, buildErr = runBuild("direct-build-bin-junction")
		if buildErr == nil || !strings.Contains(strings.ToLower(string(junctionOutput)), "build output directory must resolve inside source tree") {
			t.Fatalf("direct build accepted source/bin junction into installed/bin: err=%v output=%s", buildErr, junctionOutput)
		}
		assertInstallUnchanged("rejected direct build through bin junction")
	})
	if _, err := os.Stat(cloneBin); !os.IsNotExist(err) {
		t.Fatalf("source/bin junction was not removed after test: %s: %v", cloneBin, err)
	}

	if err := os.MkdirAll(cloneBin, 0700); err != nil {
		t.Fatal(err)
	}
	hardLink := filepath.Join(cloneBin, "steerline.exe")
	encode := func(value string) string { return base64.StdEncoding.EncodeToString([]byte(value)) }
	linkScript := "$link=[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('" + encode(hardLink) + "')); $target=[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('" + encode(installedExe) + "')); New-Item -ItemType HardLink -Path $link -Target $target | Out-Null"
	if _, err := s.RunCommand(project.Command(powershell, "-NoProfile", "-Command", linkScript), "create-direct-build-output-hardlink"); err != nil {
		t.Fatalf("create output hard link: %v", err)
	}
	hardLinkOutput, hardLinkErr := runBuild("direct-build-output-hardlink")
	if hardLinkErr == nil || !strings.Contains(strings.ToLower(string(hardLinkOutput)), "build output file must not be a hard link") {
		t.Fatalf("direct build accepted a hard-linked output exe: err=%v output=%s", hardLinkErr, hardLinkOutput)
	}
	assertInstallUnchanged("rejected direct build through output hard link")
	if err := os.Remove(hardLink); err != nil {
		t.Fatalf("remove direct build output hard link: %v", err)
	}
	if err := os.Remove(cloneBin); err != nil {
		t.Fatalf("remove empty source bin directory: %v", err)
	}

	positiveOutput, positiveErr := runBuild("direct-build-ordinary-source-output")
	if positiveErr != nil {
		t.Fatalf("ordinary direct source build failed: %v: %s", positiveErr, positiveOutput)
	}
	cloneExe := filepath.Join(cloneBin, "steerline.exe")
	cloneSHA := fileSHA256(t, cloneExe)
	identityCommand := project.Command(cloneExe, "home", "-config", project.Config)
	identityOutput, identityErr := s.RunCommand(identityCommand, "direct-build-source-home-identity")
	if identityErr != nil {
		t.Fatalf("read direct build source identity: %v: %s", identityErr, identityOutput)
	}
	for _, expected := range []string{cloneExe, s.Run.SourceCommit, "dirty=false"} {
		assertContains(t, string(identityOutput), expected)
	}
	assertInstallUnchanged("ordinary direct source build")
	evidence["direct_source_build"] = map[string]any{
		"source_clone": sourceRoot, "source_commit": strings.TrimSpace(string(headOutput)),
		"source_bin_junction_rejected": string(junctionOutput), "output_hardlink_rejected": string(hardLinkOutput),
		"ordinary_build_output": string(positiveOutput), "source_executable": cloneExe,
		"source_executable_sha256": cloneSHA, "source_home_identity": string(identityOutput),
		"installed_executable_sha256_before_after": installedExeSHA, "installed_database_sha256_before_after": installedDBSHA,
		"installed_service_instance_id_before_after": serviceInstance(installedBefore),
		"installed_bin_entries_unchanged":            true, "installed_task_facts_unchanged": true,
	}
	writeRuntimeEvidence(t, s.Output, evidence)
}

func directoryFileSnapshot(t *testing.T, directory string) []string {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := make([]string, 0, len(entries))
	for _, entry := range entries {
		path := filepath.Join(directory, entry.Name())
		if entry.IsDir() {
			snapshot = append(snapshot, entry.Name()+"=directory")
			continue
		}
		snapshot = append(snapshot, entry.Name()+"="+fileSHA256(t, path))
	}
	sort.Strings(snapshot)
	return snapshot
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

func assertMissingSourceExecutableRejected(t *testing.T, s *testenv.Scenario, project *testenv.Project, powershell, script, installedHome string) {
	t.Helper()
	sourceExecutable := filepath.Join(s.Run.Root, "bin", "steerline.exe")
	missingExecutable := sourceExecutable + ".task83-missing"
	if _, err := os.Stat(missingExecutable); !os.IsNotExist(err) {
		t.Fatalf("source executable backup path already exists: %s", missingExecutable)
	}
	before := fileSHA256(t, sourceExecutable)
	if err := os.Rename(sourceExecutable, missingExecutable); err != nil {
		t.Fatalf("hide stopped source executable for fail-closed check: %v", err)
	}
	restored := false
	defer func() {
		if !restored {
			if err := os.Rename(missingExecutable, sourceExecutable); err != nil {
				t.Errorf("restore source executable after fail-closed check: %v", err)
			}
		}
	}()
	cmd := project.Command(powershell, "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script,
		"-DebugHome", project.Home, "-InstalledHome", installedHome, "-Config", project.Config, "-Operation", "cli", "-Arguments", "home")
	output, err := s.RunCommand(cmd, "missing-source-executable")
	if err == nil || !strings.Contains(strings.ToLower(string(output)), "missing source build executable") {
		t.Fatalf("debug runtime did not reject a missing source executable: err=%v output=%s", err, output)
	}
	if err := os.Rename(missingExecutable, sourceExecutable); err != nil {
		t.Fatalf("restore source executable after fail-closed check: %v", err)
	}
	restored = true
	if actual := fileSHA256(t, sourceExecutable); actual != before {
		t.Fatalf("source executable changed during fail-closed check: before=%s after=%s", before, actual)
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

func assertServiceStopReceipt(t *testing.T, output []byte) {
	t.Helper()
	var receipt struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(output, &receipt); err != nil || receipt.Status != "stopped" {
		t.Fatalf("service-window stop did not return an applied stopped receipt: status=%q err=%v output=%s", receipt.Status, err, output)
	}
}

func controlRequestCount(t *testing.T, project *testenv.Project) int {
	t.Helper()
	return len(controlRequestFiles(t, project))
}

func controlRequestFiles(t *testing.T, project *testenv.Project) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(filepath.Dir(project.Database), "control", "requests", "*", "request.json"))
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func assertControlPublication(t *testing.T, project *testenv.Project, before []string, snapshot testenv.Snapshot) map[string]any {
	t.Helper()
	after := controlRequestFiles(t, project)
	// Claim uses the normal product client for resume_dispatch and start_task.
	// Observing both immutable requests proves the rejection counter is live.
	if len(after) != len(before)+2 {
		t.Fatalf("normal claim did not publish two observable control requests: before=%d after=%d", len(before), len(after))
	}
	existing := make(map[string]bool, len(before))
	for _, path := range before {
		existing[path] = true
	}
	observed := make([]map[string]any, 0, 2)
	actions := make(map[string]bool)
	for _, path := range after {
		if existing[path] {
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var request struct {
			RequestID          string `json:"request_id"`
			RepositoryIdentity string `json:"repository_identity"`
			InstanceID         string `json:"instance_id"`
			Action             string `json:"action"`
			Payload            struct {
				TaskID string `json:"task_id"`
			} `json:"payload"`
		}
		if err := json.Unmarshal(raw, &request); err != nil {
			t.Fatalf("decode published control request %s: %v", path, err)
		}
		id, idErr := hex.DecodeString(request.RequestID)
		if idErr != nil || len(id) != 16 || request.RequestID != filepath.Base(filepath.Dir(path)) ||
			request.InstanceID != serviceInstance(snapshot) || request.RepositoryIdentity != snapshot.Runtime.Repository.Identity {
			t.Fatalf("published request has wrong file or runtime identity: path=%s request=%+v", path, request)
		}
		if (request.Action != "resume_dispatch" && request.Action != "start_task") || actions[request.Action] ||
			(request.Action == "start_task" && request.Payload.TaskID != project.Tasks["a"].ID) {
			t.Fatalf("normal claim published an unexpected request: %+v", request)
		}
		actions[request.Action] = true
		observed = append(observed, map[string]any{"path": path, "sha256": fileSHA256(t, path), "request": request})
	}
	return map[string]any{"before": len(before), "after": len(after), "requests": observed}
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

func assertGeneratedLaunchers(t *testing.T, home, executable string) {
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
		{console, `set "BIN=` + executable + `"`},
		{service, `set "STEERLINE_HOME=%SHOME%"`},
		{service, `set "CODEX_HOME=%SHOME%\codex"`},
		{service, `set "BIN=` + executable + `"`},
	} {
		if !strings.Contains(string(value.data), value.want) {
			t.Fatalf("launcher did not bind to its own home: missing %q", value.want)
		}
	}
	for _, data := range [][]byte{console, service} {
		if strings.Contains(string(data), `set "BIN=%SHOME%\bin\steerline.exe"`) {
			t.Fatal("debug launcher retained a home-bin fallback")
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

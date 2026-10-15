//go:build e2e

package runtime

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/erranliu/SteerLine/tests/e2e/internal/testenv"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

type serviceStart struct {
	Status       string `json:"status"`
	LauncherPath string `json:"launcher_path"`
	ConfigPath   string `json:"config_path"`
}

func TestInstalledDebugIsolation(t *testing.T) {
	s := testenv.NewScenario(t, "runtime.installed-debug-isolation")
	part := filepath.Join("中文 路径", "有空格的现场")
	installed := s.NewProjectAt("installed", part, "")
	debug := s.NewProjectAt("debug", part, "")
	cwd := filepath.Join(s.Root, "无关 cwd")
	s.Must(os.MkdirAll(cwd, 0700))
	installed.Driver.WorkingDirectory = cwd
	debug.Driver.WorkingDirectory = cwd
	_, err := installed.Driver.Run(s.Context, "install")
	s.Must(err)
	installedExe := filepath.Join(installed.Home, "bin", "steerline.exe")
	installed.Executable = installedExe
	installed.Driver.Binary = installedExe
	installedHash := fileSHA256(t, installedExe)
	installed.CreateTasks()
	installedBefore := mustSnapshot(t, installed)
	powershell := powershellExecutable(t)
	script := filepath.Join(s.Run.Root, "scripts", "debug-runtime.ps1")
	_, err = runDebugScript(s, debug, powershell, script, installed.Home, "build")
	s.Must(err)
	_, err = runDebugScript(s, debug, powershell, script, installed.Home, "launchers")
	s.Must(err)
	debugExe := filepath.Join(s.Run.Root, "bin", "steerline.exe")
	debug.Executable = debugExe
	debug.Driver.Binary = debugExe
	evidence := map[string]any{"source_commit": s.Run.SourceCommit, "source_dirty": s.Run.SourceDirty, "installed_executable": installedExe, "debug_executable": debugExe, "installed_sha256": installedHash}
	assertDefaultStateAliasRejected(t, s, debug, installed, debugExe, powershell, script, evidence)
	assertGeneratedLaunchers(t, debug.Home, debugExe)
	debug.CreateTasks()
	before := mustSnapshot(t, debug)
	if before.Page.ProjectID == installedBefore.Page.ProjectID || before.Page.DatabaseID == installedBefore.Page.DatabaseID {
		t.Fatal("independent homes shared project/database")
	}
	if before.Runtime.Service.PID == installedBefore.Runtime.Service.PID {
		t.Fatal("independent homes shared host process")
	}
	_, runningBuildErr := runDebugScript(s, debug, powershell, script, installed.Home, "build")
	if runningBuildErr == nil {
		t.Fatal("source build replaced a running host")
	}
	if fileSHA256(t, installedExe) != installedHash {
		t.Fatal("source build changed installation")
	}
	unchanged := mustSnapshot(t, installed)
	assertProjectFactsEqual(t, installedBefore, unchanged, "debug build changed installed facts")
	assertSameServiceInstance(t, installedBefore, unchanged)
	count := controlRequestCount(t, debug)
	if err = debug.Driver.ControlTargeted(s.Context, s.Run.Helper, "start_task", debug.Tasks["a"].ID, serviceInstance(before)+"-stale"); err == nil {
		t.Fatal("stale project instance accepted")
	}
	if controlRequestCount(t, debug) != count {
		t.Fatal("stale target published input")
	}
	mirror := s.NewHomeForProject(debug, "same-repository-other-home", debugExe)
	mirror.StartHost()
	s.Wait("cross-home repository conflict", func() (bool, error) {
		status, err := mirror.Driver.HostStatus(s.Context)
		if err != nil {
			return false, err
		}
		for _, project := range status.Projects {
			if project.RuntimeState == "failed" {
				if !strings.Contains(project.Diagnostic, "repository already locked") {
					t.Fatal(project.Diagnostic)
				}
				return true, nil
			}
		}
		return false, nil
	})
	if _, err = os.Stat(mirror.Database); !os.IsNotExist(err) {
		t.Fatal("conflicting home initialized database")
	}
	mirror.StopService()
	installed.Claim("a")
	debug.Claim("a")
	installedOpen := installed.WaitPhase("waiting_pr_merge")
	debugOpen := debug.WaitPhase("waiting_pr_merge")
	debug.StopService()
	_, err = runDebugScript(s, debug, powershell, script, installed.Home, "build")
	s.Must(err)
	_, err = runDebugScript(s, debug, powershell, script, installed.Home, "launchers")
	s.Must(err)
	assertSameServiceInstance(t, installedBefore, mustSnapshot(t, installed))
	assertSharedSQLiteRejected(t, s, debug, installed, powershell, script, evidence)
	assertHomeJunctionRejected(t, s, debug, powershell, script, installed)
	assertDirectBuildOutputIsolation(t, s, debug, installed, powershell, evidence)
	assertMissingSourceExecutableRejected(t, s, debug, powershell, script, installed.Home)
	debug.StartService()
	after := mustSnapshot(t, debug)
	if serviceInstance(before) == serviceInstance(after) {
		t.Fatal("host restart retained runtime instance")
	}
	if len(after.Domain.WorkflowRuns) != len(debugOpen.Domain.WorkflowRuns) || after.Helper.Dispatches != debugOpen.Helper.Dispatches {
		t.Fatal("restart produced new business input")
	}
	installed.Record("waiting-merge", installedOpen)
	debug.Record("after-restart", after)
	installed.Merge()
	debug.Merge()
	s.Wait("independent deliveries", func() (bool, error) {
		a, e := installed.Snapshot()
		if e != nil {
			return false, e
		}
		b, e := debug.Snapshot()
		return e == nil && len(a.Domain.Deliveries) == 1 && len(b.Domain.Deliveries) == 1, e
	})
	installed.StopService()
	debug.StopService()
	evidence["installed_instance"] = serviceInstance(installedBefore)
	evidence["debug_before"] = serviceInstance(before)
	evidence["debug_after"] = serviceInstance(after)
	evidence["independent_deliveries"] = true
	writeRuntimeEvidence(t, s.Output, evidence)
}

func assertSharedSQLiteRejected(t *testing.T, s *testenv.Scenario, project, installed *testenv.Project, powershell, script string, evidence map[string]any) {
	t.Helper()
	invalidBatch := filepath.Join(s.Root, "invalid-shared-sqlite-batch.json")
	s.Must(os.WriteFile(invalidBatch, []byte("{}\n"), 0600))
	installedDBBefore := fileSHA256(t, installed.Database)
	results := []map[string]any{}
	evidence["sqlite_shared_hardlinks"] = results
	quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }
	for _, suffix := range []string{"", "-wal", "-shm"} {
		name := "database" + suffix
		func() {
			path := project.Database + suffix
			backup := filepath.Join(s.Root, "original-"+name)
			hadOriginal := false
			if _, err := os.Stat(path); err == nil {
				s.Must(os.Rename(path, backup))
				hadOriginal = true
			} else if !os.IsNotExist(err) {
				t.Fatal(err)
			}
			linked := false
			defer func() {
				if linked {
					if err := os.Remove(path); err != nil {
						t.Errorf("remove owned %s hard link: %v", name, err)
					}
				}
				if hadOriginal {
					if err := os.Rename(backup, path); err != nil {
						t.Errorf("restore owned %s: %v", name, err)
					}
				}
			}()
			// The main DB is a copy of the actual initialized native database.
			// Empty WAL/SHM installed sentinels reproduce SQLite's writable
			// sidecars without synthesizing any task or authority state.
			var content []byte
			if suffix == "" {
				var err error
				content, err = os.ReadFile(backup)
				s.Must(err)
			}
			sentinel := filepath.Join(installed.Home, "temp", "task83-shared-"+name)
			s.Must(os.WriteFile(sentinel, content, 0600))
			s.Must(os.Link(sentinel, path))
			linked = true
			before := sqliteFileSnapshot(t, project.Database)
			sentinelHash := fileSHA256(t, sentinel)
			sentinelInfo, err := os.Stat(sentinel)
			s.Must(err)
			result := map[string]any{"file": path, "installed_sentinel": sentinel, "sentinel_size_before": sentinelInfo.Size(), "sentinel_sha256_before": sentinelHash, "sqlite_before": before}
			results = append(results, result)
			evidence["sqlite_shared_hardlinks"] = results
			checkRefusal := func(entry string, command *exec.Cmd) {
				output, commandErr := s.RunCommand(command, "sqlite-shared-"+name+"-"+entry)
				after := sqliteFileSnapshot(t, project.Database)
				info, statErr := os.Stat(sentinel)
				s.Must(statErr)
				hash := fileSHA256(t, sentinel)
				result[entry] = map[string]any{"output": string(output), "error": fmt.Sprint(commandErr), "sentinel_size_after": info.Size(), "sentinel_sha256_after": hash, "sqlite_after": after}
				writeSQLiteEvidence(t, s.Output, results)
				if commandErr == nil || !strings.Contains(strings.ToLower(string(output)), "sqlite state file must not be a hard link") {
					t.Fatalf("%s accepted shared SQLite %s before open: err=%v output=%s", entry, name, commandErr, output)
				}
				if info.Size() != sentinelInfo.Size() || hash != sentinelHash || !reflect.DeepEqual(before, after) {
					t.Fatalf("%s changed shared %s or native SQLite before refusal: sentinel=%d/%s before=%v after=%v", entry, name, info.Size(), hash, before, after)
				}
			}
			checkRefusal("product_cli", project.Command(project.Executable, "check", "-config", project.Config))
			wrapper := "& " + quote(script) + " -DebugHome " + quote(project.Home) + " -InstalledHome " + quote(installed.Home) + " -Config " + quote(project.Config) + " -Operation cli -Arguments @('task','validate','-file'," + quote(invalidBatch) + "); exit $LASTEXITCODE"
			checkRefusal("debug_wrapper", project.Command(powershell, "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", wrapper))
		}()
	}
	if fileSHA256(t, installed.Database) != installedDBBefore {
		t.Fatal("SQLite hardlink negatives changed the installed project's actual database")
	}
	evidence["sqlite_installed_database_sha256_before"] = installedDBBefore
	evidence["sqlite_installed_database_sha256_after"] = fileSHA256(t, installed.Database)
	// Restore every original SQLite file before exercising the ordinary reader.
	// The malformed batch must now reach the real business validation error.
	output, err := s.RunCommand(project.Command(project.Executable, "check", "-config", project.Config), "sqlite-single-link-normal-validation")
	if err != nil {
		t.Fatalf("normal native SQLite did not remain usable: err=%v output=%s", err, output)
	}
	evidence["sqlite_single_link_validation"] = string(output)
}

func sqliteFileSnapshot(t *testing.T, database string) map[string]any {
	t.Helper()
	result := map[string]any{}
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		path := database + suffix
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			result[path] = "missing"
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		result[path] = map[string]any{"size": info.Size(), "sha256": fileSHA256(t, path)}
	}
	return result
}

func writeSQLiteEvidence(t *testing.T, output string, value any) {
	t.Helper()
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(output, "sqlite-shared-hardlinks.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
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
		productCommand := project.Command(executable, "check", "-config", config)
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
	// Keep the temporary clone path short enough that git status can inspect
	// every tracked file on Windows, including deeply nested acceptance fixtures.
	sourceRoot := filepath.Join(s.Root, "source-build")
	if _, err := os.Stat(sourceRoot); !os.IsNotExist(err) {
		t.Fatalf("direct build source clone already exists: %s", sourceRoot)
	}
	// Use the same isolated Git configuration as the direct build command, so
	// checkout line endings and the later source-dirty probe agree.
	cloneCommand := project.Command("git", "clone", "--local", "--no-hardlinks", s.Run.Root, sourceRoot)
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
	sourceStatus := func(name string) string {
		output, statusErr := s.RunCommand(project.Command("git", "-C", sourceRoot, "status", "--porcelain"), "direct-build-source-status-"+name)
		if statusErr != nil {
			t.Fatalf("read direct build source status %s: %v: %s", name, statusErr, output)
		}
		return string(output)
	}
	statusAfterClone := sourceStatus("after-clone")
	if strings.TrimSpace(statusAfterClone) != "" {
		t.Fatalf("fresh direct build source clone is unexpectedly dirty: %s", statusAfterClone)
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
	statusBeforeBuild := sourceStatus("before-positive-build")
	if strings.TrimSpace(statusBeforeBuild) != "" {
		t.Fatalf("direct build refusal checks changed the source clone: %s", statusBeforeBuild)
	}

	positiveOutput, positiveErr := runBuild("direct-build-ordinary-source-output")
	if positiveErr != nil {
		t.Fatalf("ordinary direct source build failed: %v: %s", positiveErr, positiveOutput)
	}
	statusAfterBuild := sourceStatus("after-positive-build")
	cloneExe := filepath.Join(cloneBin, "steerline.exe")
	rebuildOutput, rebuildErr := runBuild("direct-build-ordinary-source-rebuild")
	if rebuildErr != nil {
		t.Fatalf("ordinary direct source rebuild failed: %v: %s", rebuildErr, rebuildOutput)
	}
	statusAfterRebuild := sourceStatus("after-positive-rebuild")
	cloneSHA := fileSHA256(t, cloneExe)
	identityCommand := project.Command(cloneExe, "home", "-config", project.Config)
	identityOutput, identityErr := s.RunCommand(identityCommand, "direct-build-source-home-identity")
	if identityErr != nil {
		t.Fatalf("read direct build source identity: %v: %s", identityErr, identityOutput)
	}
	for _, expected := range []string{cloneExe, s.Run.SourceCommit} {
		assertContains(t, string(identityOutput), expected)
	}
	sourceDirty := ""
	for _, line := range strings.Split(string(identityOutput), "\n") {
		if strings.HasPrefix(line, "source dirty") {
			sourceDirty = strings.TrimSpace(strings.TrimPrefix(line, "source dirty"))
		}
	}
	if sourceDirty != "false" {
		t.Fatalf("direct source build identity reports dirty=%q, want false:\n%s", sourceDirty, identityOutput)
	}
	if strings.TrimSpace(statusAfterBuild) != "" || strings.TrimSpace(statusAfterRebuild) != "" {
		t.Fatalf("ordinary direct source build changed source status: after_build=%s after_rebuild=%s", statusAfterBuild, statusAfterRebuild)
	}
	assertInstallUnchanged("ordinary direct source build")
	evidence["direct_source_build"] = map[string]any{
		"source_clone": sourceRoot, "source_commit": strings.TrimSpace(string(headOutput)),
		"git_status_after_clone": statusAfterClone, "git_status_before_positive_build": statusBeforeBuild,
		"git_status_after_positive_build": statusAfterBuild, "git_status_after_positive_rebuild": statusAfterRebuild,
		"source_bin_junction_rejected": string(junctionOutput), "output_hardlink_rejected": string(hardLinkOutput),
		"ordinary_build_output": string(positiveOutput), "ordinary_rebuild_output": string(rebuildOutput),
		"source_executable": cloneExe, "source_bin_entries_after_rebuild": directoryFileSnapshot(t, cloneBin),
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

func serviceInstance(snapshot testenv.Snapshot) string {
	if snapshot.Runtime == nil {
		return ""
	}
	return snapshot.Runtime.Service.InstanceID
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

func assertGeneratedLaunchers(t *testing.T, home, executable string) {
	t.Helper()
	serve, err := os.ReadFile(filepath.Join(home, "serve.bat"))
	if err != nil {
		t.Fatal(err)
	}
	service, err := os.ReadFile(filepath.Join(home, "stop.bat"))
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []struct {
		data []byte
		want string
	}{
		{serve, `set "STEERLINE_HOME=%SHOME%"`},
		{service, `set "STEERLINE_HOME=%SHOME%"`},
		{service, `set "CODEX_HOME=%SHOME%\codex"`},
		{service, `set "BIN=` + executable + `"`},
	} {
		if !strings.Contains(string(value.data), value.want) {
			t.Fatalf("launcher did not bind to its own home: missing %q", value.want)
		}
	}
	if strings.Contains(string(service), `set "BIN=%SHOME%\bin\steerline.exe"`) {
		t.Fatal("debug launcher retained a home-bin fallback")
	}
}

func assertProjectFactsEqual(t *testing.T, before, after testenv.Snapshot, message string) {
	t.Helper()
	if before.Page.ProjectID != after.Page.ProjectID || before.Page.DatabaseID != after.Page.DatabaseID || len(before.Domain.WorkflowRuns) != len(after.Domain.WorkflowRuns) || len(before.Domain.Executions) != len(after.Domain.Executions) {
		t.Fatal(message)
	}
	for i := range before.Domain.WorkflowRuns {
		left, right := before.Domain.WorkflowRuns[i], after.Domain.WorkflowRuns[i]
		if left.WorkflowRunID != right.WorkflowRunID || left.TaskID != right.TaskID || left.Status != right.Status {
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

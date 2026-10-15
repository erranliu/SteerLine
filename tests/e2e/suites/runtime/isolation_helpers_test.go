//go:build e2e

package runtime

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/tests/e2e/internal/testenv"
)

func powershellExecutable(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("powershell.exe")
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func assertSharedSQLiteRejected(t *testing.T, s *testenv.Scenario, project, installed *testenv.Project, evidence map[string]any) {
	t.Helper()
	installedDBBefore := fileSHA256(t, installed.Database)
	results := []map[string]any{}
	evidence["sqlite_shared_hardlinks"] = results
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
			sentinel := filepath.Join(installed.Home, "temp", "isolation-shared-"+name)
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

		}()
	}
	if fileSHA256(t, installed.Database) != installedDBBefore {
		t.Fatal("SQLite hardlink negatives changed the installed project's actual database")
	}
	evidence["sqlite_installed_database_sha256_before"] = installedDBBefore
	evidence["sqlite_installed_database_sha256_after"] = fileSHA256(t, installed.Database)
	// Restore every original SQLite file before exercising the ordinary reader.
	// The restored database must reach the ordinary local configuration check.
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

func writeConfigVariant(t *testing.T, project *testenv.Project, name string, change func(map[string]any)) string {
	t.Helper()
	return writeConfigVariantAt(t, project.Config, filepath.Join(project.Root, name), change)
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

func assertProjectFactsEqual(t *testing.T, before, after testenv.Snapshot, message string) {
	t.Helper()
	if before.Page.ProjectID != after.Page.ProjectID || before.Page.DatabaseID != after.Page.DatabaseID || len(before.Page.Tasks) != len(after.Page.Tasks) || len(before.Runner.Candidates) != len(after.Runner.Candidates) {
		t.Fatal(message)
	}
	for i := range before.Page.Tasks {
		left, right := before.Page.Tasks[i], after.Page.Tasks[i]
		if left.TaskID != right.TaskID || left.Status != right.Status || left.Version != right.Version {
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

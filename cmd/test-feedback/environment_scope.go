package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
)

// This is the fixed selection, not a name-pattern discovery mechanism. Source
// parsing only checks for drift; execution always uses these explicit roots.
var environmentAppSources = map[string][]string{
	"environment_adaptation_test.go":     {"TestProjectEnvironmentAdmissionDeclaredBasisAndPersistentRevocation", "TestProjectEnvironmentAdmissionReservationAndConcurrentStart", "TestProjectEnvironmentAdmissionAcrossProjectsUsesPublicTarget"},
	"environment_decoupling_test.go":     {"TestEnvironmentDetectionNeedsNoProjectCatalogueOrPlatformCopies", "TestEnvironmentStableBasisExcludesUnrelatedCommitAndModel", "TestEnvironmentQualificationIgnoresProjectTestsAndBusinessDocuments", "TestEnvironmentRuntimeDependenciesBindBasisWithoutExpandingReadingReferences", "TestEnvironmentReadFailureDoesNotPersistRevocation", "TestEnvironmentKnownDeletedDependencyPermanentlyRevokes", "TestEnvironmentStartRejectsPendingOrSupersededBaseline"},
	"environment_lifecycle_test.go":      {"TestProjectEnvironmentSameBuildStartupStops", "TestProjectEnvironmentRejectsForeignNamespace"},
	"environment_materials_test.go":      {"TestEnvironmentMaterialQueryDoesNotExposeExecutionSecrets", "TestEnvironmentHistoryFiltersBeforeTwentyAndKeepsRemovedTarget"},
	"environment_readiness_test.go":      {"TestEnvironmentConfigurationQueryDoesNotTouchProject", "TestEnvironmentUnqualifiedQueryDoesNotReadDependencyBasis", "TestProjectEnvironmentConfigurationSurvivesProjectFailures", "TestProjectEnvironmentReadinessRefreshRejectsBeforeMaterialPreparation", "TestProjectEnvironmentReadinessInvalidationUnknownPreservesAppliedReceipt", "TestProjectEnvironmentReadinessLocalAndModelGrouping", "TestProjectEnvironmentReadinessLocalTargetOccupancyAcrossContexts"},
	"environment_script_windows_test.go": {"TestProjectEnvironmentScriptReportsToolsWithoutChainChecks"},
	"environment_target_test.go":         {"TestEnvironmentContextAndGlobalTargetBoundaries"},
	"environment_workflow_test.go":       {"TestProjectEnvironmentWorkflowPublicCommandsAndSingleInvocation"},
}

var environmentAppExtra = map[string][]string{
	"developer_test.go":           {"TestDeveloperConfigurationOptions", "TestDeveloperConfigurationLoad", "TestDeveloperReadinessService"},
	"project_maintenance_test.go": {"TestPMReservationRejectsPendingEnvironmentObservation", "TestPMSGenerationFreshnessAndCandidateVersions", "TestPMSNativeConfigurationPrecheckSnapshot"},
	"shared_environment_test.go":  {"TestSharedEnvironmentMaterialsAndIndependentQualifications"},
}

// Match Go's platform file selection without hiding source drift on other hosts.
var environmentAppSourceGOOS = map[string]string{
	"environment_script_windows_test.go": "windows",
}

func environmentAppRoots() []string {
	return environmentAppRootsForGOOS(runtime.GOOS)
}

func environmentAppRootsForGOOS(goos string) []string {
	var roots []string
	for _, source := range []map[string][]string{environmentAppSources, environmentAppExtra} {
		for file, names := range source {
			if target := environmentAppSourceGOOS[file]; target != "" && target != goos {
				continue
			}
			roots = append(roots, names...)
		}
	}
	sort.Strings(roots)
	return roots
}

func checkEnvironmentAppSources(root string) error {
	for file, goos := range environmentAppSourceGOOS {
		if _, ok := environmentAppSources[file]; !ok || goos != "windows" || !strings.HasSuffix(file, "_windows_test.go") {
			return fmt.Errorf("invalid environment source platform %s: %s", file, goos)
		}
	}
	for file := range environmentAppSources {
		if strings.HasSuffix(file, "_windows_test.go") && environmentAppSourceGOOS[file] != "windows" {
			return fmt.Errorf("missing environment source platform %s", file)
		}
	}
	dir := filepath.Join(root, "internal/app")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "environment") && strings.HasSuffix(entry.Name(), "_test.go") {
			if _, ok := environmentAppSources[entry.Name()]; !ok {
				return fmt.Errorf("unregistered environment test source %s", entry.Name())
			}
		}
	}
	seen := map[string]bool{}
	for _, sources := range []map[string][]string{environmentAppSources, environmentAppExtra} {
		for file, expected := range sources {
			source, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, file), nil, 0)
			if err != nil {
				return err
			}
			var actual []string
			for _, decl := range source.Decls {
				if f, ok := decl.(*ast.FuncDecl); ok && f.Recv == nil && strings.HasPrefix(f.Name.Name, "Test") && f.Name.Name != "TestMain" {
					actual = append(actual, f.Name.Name)
				}
			}
			for _, name := range expected {
				if seen[name] || !slices.Contains(actual, name) {
					return fmt.Errorf("missing or duplicate environment root %s in %s", name, file)
				}
				seen[name] = true
			}
			if _, wholeFile := environmentAppSources[file]; wholeFile {
				a, b := slices.Clone(actual), slices.Clone(expected)
				sort.Strings(a)
				sort.Strings(b)
				if !slices.Equal(a, b) {
					return fmt.Errorf("environment root selection drift in %s: actual %v, assigned %v", file, a, b)
				}
			}
		}
	}
	for _, def := range localSuites {
		if def.ID != "go.environment" {
			continue
		}
		if len(def.Groups) != 2 || !slices.Equal(def.Groups[0].Packages, []string{"./internal/environmentadaptation", "./internal/environmentadaptation/model", "./internal/testkit/remotetest", "./internal/executionconfig"}) || len(def.Groups[0].Tests) != 0 || !slices.Equal(def.Groups[1].Packages, []string{"./internal/app"}) || !slices.Equal(def.Groups[1].Tests, environmentAppRoots()) {
			return fmt.Errorf("environment groups differ from fixed owner/app selection")
		}
		return validateLocalGroups(def.Groups)
	}
	return fmt.Errorf("environment suite is missing")
}

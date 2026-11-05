package main

import (
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"sort"
	"strings"
)

type domainTestSource struct {
	Suite, GOOS, BuildTag string
	Roots                 []string
}

// Execution uses these explicit roots. Parsing validates drift on every host,
// including excluded live sources and files for another platform.
var appDomainSources = map[string]domainTestSource{
	"internal/app/deployment_discard_test.go":          {Suite: "go.deployment", Roots: []string{"TestDeploymentDiscardRuns"}},
	"internal/app/deployment_prepare_test.go":          {Suite: "go.deployment", Roots: []string{"TestDeploymentPreparation", "TestDeploymentPreparationPackage"}},
	"internal/app/deployment_recovery_test.go":         {Suite: "go.deployment", Roots: []string{"TestDeploymentRecoveryReadiness", "TestDeploymentInterruptedRecovery"}},
	"internal/app/deployment_record_windows_test.go":   {Suite: "go.deployment", GOOS: "windows", Roots: []string{"TestDeploymentRecordScriptMeetsRecoveryACL"}},
	"internal/app/application_verify_stop_test.go":     {Suite: "go.deployment", Roots: []string{"TestZeroProjectOfflineVerificationRequiresAbsentRunner", "TestDeploymentRechecksFailedReceiptWithoutRestartOrRewrite", "TestDeploymentRestartCandidateBindingsPreserveOriginalState", "TestDeploymentCandidateVerificationKeepsOriginalBinding"}},
	"internal/app/runner_preset_test.go":               {Suite: "go.deployment", Roots: []string{"TestRunnerPresetFreeze"}},
	"internal/runnerclient/upgrade_test.go":            {Suite: "go.deployment", Roots: []string{"TestUpgradeDrainedUsesCandidateProofAndBothBusinessReaders"}},
	"internal/app/roadmap_test.go":                     {Suite: "go.roadmap", Roots: []string{"TestRoadmapAdmissionAtomicNumbersSourceAndReplay", "TestRoadmapSourceAndMissingDocumentCapability", "TestRoadmapUnknownReceiptKeepsBusinessEffectPending", "TestRoadmapPublicationRejectsAnotherInFlightGeneration", "TestRoadmapGeneratedIndexCannotOverwriteAnotherSource"}},
	"internal/app/roadmap_publication_test.go":         {Suite: "go.roadmap", Roots: []string{"TestRoadmapPublicationAndAdmissionWithRealGit", "TestRoadmapPublicationAndAdmissionWithCRLFCheckout"}},
	"internal/app/roadmap_preparation_test.go":         {Suite: "go.roadmap", Roots: []string{"TestRoadmapStaticInitializationAndPublication", "TestRoadmapStaticInitializationRejectsGitOperation", "TestRoadmapInitializationPendingReceiptReconcilesWithoutWriting", "TestRoadmapPublishedGuideMustBeValidUTF8AndBounded"}},
	"internal/app/roadmap_generation_test.go":          {Suite: "go.roadmap"},
	"internal/app/roadmap_live_test.go":                {Suite: "go.roadmap", BuildTag: "e2e", Roots: []string{"TestRoadmapLocalCodexPromotion", "TestRoadmapPreparedSteerlineTestLocalCodexPromotion"}},
	"internal/app/host/native_state_preflight_test.go": {Suite: "go.roadmap", Roots: []string{"TestRoadmapPublicationStateCanReopenWithoutRelaxingDirectoryGuard"}},
}

func domainRootsForGOOS(suite, dir, goos string) []string {
	var roots []string
	for path, source := range appDomainSources {
		if source.Suite == suite && filepath.ToSlash(filepath.Dir(path)) == dir && source.BuildTag == "" && (source.GOOS == "" || source.GOOS == goos) {
			roots = append(roots, source.Roots...)
		}
	}
	sort.Strings(roots)
	return roots
}

func domainGroupsForGOOS(suite, goos string) []localGroup {
	if suite == "go.deployment" {
		groups := []localGroup{{ID: "go-deployment-host", Packages: []string{"./internal/app/host"}}}
		if goos == "windows" {
			groups = append(groups, localGroup{ID: "go-deployment-service", Packages: []string{"./internal/localservice"}})
		}
		return append(groups,
			localGroup{ID: "go-deployment-app", Packages: []string{"./internal/app"}, Tests: domainRootsForGOOS(suite, "internal/app", goos)},
			localGroup{ID: "go-deployment-upgrade", Packages: []string{"./internal/runnerclient"}, Tests: domainRootsForGOOS(suite, "internal/runnerclient", goos)})
	}
	return []localGroup{
		{ID: "go-roadmap-owner", Packages: []string{"./internal/roadmap"}},
		{ID: "go-roadmap-app", Packages: []string{"./internal/app"}, Tests: domainRootsForGOOS(suite, "internal/app", goos)},
		{ID: "go-roadmap-host", Packages: []string{"./internal/app/host"}, Tests: domainRootsForGOOS(suite, "internal/app/host", goos)},
	}
}

func checkAppDomainSources(root string) error {
	for _, dir := range []string{"internal/app", "internal/app/host", "internal/runnerclient"} {
		entries, err := os.ReadDir(filepath.Join(root, dir))
		if err != nil {
			return err
		}
		for _, entry := range entries {
			name := entry.Name()
			if strings.HasSuffix(name, "_test.go") && ((dir == "internal/runnerclient" && strings.HasPrefix(name, "upgrade")) || (dir != "internal/runnerclient" && (strings.HasPrefix(name, "deployment") || strings.HasPrefix(name, "roadmap")))) {
				if _, ok := appDomainSources[dir+"/"+name]; !ok {
					return fmt.Errorf("unregistered App domain test source %s/%s", dir, name)
				}
			}
		}
	}
	seen := map[string]bool{}
	for path, expected := range appDomainSources {
		if strings.HasSuffix(path, "_windows_test.go") != (expected.GOOS == "windows") || (expected.GOOS != "" && expected.GOOS != "windows") {
			return fmt.Errorf("App domain source platform mismatch: %s", path)
		}
		f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, path), nil, parser.ParseComments)
		if err != nil {
			return err
		}
		var tags []string
		for _, group := range f.Comments {
			if group.Pos() >= f.Package {
				continue
			}
			for _, c := range group.List {
				if constraint.IsGoBuild(c.Text) {
					expr, err := constraint.Parse(c.Text)
					if err != nil {
						return fmt.Errorf("invalid domain build constraint in %s: %w", path, err)
					}
					tags = append(tags, expr.String())
				} else if constraint.IsPlusBuild(c.Text) {
					return fmt.Errorf("unexpected legacy domain build constraint: %s", path)
				}
			}
		}
		var wantTags []string
		if expected.BuildTag != "" {
			wantTags = []string{expected.BuildTag}
		}
		if !slices.Equal(tags, wantTags) {
			return fmt.Errorf("App domain build constraint drift in %s: %v", path, tags)
		}
		var actual []string
		for _, decl := range f.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && strings.HasPrefix(fn.Name.Name, "Test") && fn.Name.Name != "TestMain" {
				key := filepath.ToSlash(filepath.Dir(path)) + ":" + fn.Name.Name
				if seen[key] {
					return fmt.Errorf("duplicate App domain root %s", key)
				}
				seen[key] = true
				actual = append(actual, fn.Name.Name)
			}
		}
		want := slices.Clone(expected.Roots)
		sort.Strings(want)
		sort.Strings(actual)
		if !slices.Equal(actual, want) {
			return fmt.Errorf("App domain root drift in %s: actual %v, assigned %v", path, actual, want)
		}
	}
	for _, id := range []string{"go.deployment", "go.roadmap"} {
		found := false
		for _, def := range localSuites {
			if def.ID != id {
				continue
			}
			if found || def.Tag != "" || len(def.Required) != 0 || !reflect.DeepEqual(def.Groups, domainGroupsForGOOS(id, runtime.GOOS)) {
				return fmt.Errorf("App domain definition drift: %s", id)
			}
			found = true
			if err := validateLocalGroups(def.Groups); err != nil {
				return err
			}
		}
		if !found {
			return fmt.Errorf("missing App domain suite %s", id)
		}
	}
	return nil
}

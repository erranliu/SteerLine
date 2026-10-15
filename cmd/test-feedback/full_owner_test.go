package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	testfeedback "github.com/erranliu/SteerLine/internal/testkit/feedback"
)

func TestFullOtherPackagesStartBeforeExecutionWorkersFinish(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	entered := make(chan int, 3)
	release := make(chan struct{})
	groups := runFullGroups(3, func(i int) scopedGroup {
		entered <- i
		select {
		case <-release:
		case <-ctx.Done():
		}
		return scopedGroup{ID: fmt.Sprintf("core-%d", i+1)}
	}, func() scopedGroup {
		for i := 0; i < 3; i++ {
			select {
			case <-entered:
			case <-ctx.Done():
			}
		}
		close(release)
		return scopedGroup{ID: "other-packages"}
	})
	if ctx.Err() != nil {
		t.Fatal("other packages waited for the execution workers to finish")
	}
	if len(groups) != 4 || groups[3].ID != "other-packages" {
		t.Fatal("Full lost the separately reported remaining package group")
	}
	for i := 0; i < 3; i++ {
		if groups[i].ID != fmt.Sprintf("core-%d", i+1) {
			t.Fatal("parallel Full changed the execution group report order")
		}
	}
}

func TestPrepareFullCoreEnumeratesExecutionBusinessTests(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	binary, dir, err := prepareCore(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(dir) != "execution" {
		t.Fatalf("Full still compiles the old host facade: %s", dir)
	}
	names, err := listCoreTests(ctx, binary, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(names, "TestCandidateResumeErrorDoesNotStarveSibling") || !slices.Contains(names, "TestWorkerFlowClaimNeedsNoProjectSteps") {
		t.Fatal("Full partition omitted the migrated execution business tests")
	}
}

func TestFullPackageEnumerationIncludesTestkitOwners(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("missing source location")
	}
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(original); err != nil {
			t.Error(err)
		}
	}()
	if err := os.Chdir(filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))); err != nil {
		t.Fatal(err)
	}
	packages, err := listPackages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"github.com/erranliu/SteerLine/internal/testkit/boundary",
		"github.com/erranliu/SteerLine/internal/testkit/boundary/e2e",
		"github.com/erranliu/SteerLine/internal/testkit/feedback",
		"github.com/erranliu/SteerLine/internal/testkit/process",
		"github.com/erranliu/SteerLine/internal/testkit/protocol",
	}
	for _, owner := range want {
		if !slices.Contains(packages, owner) {
			t.Errorf("Full package enumeration omitted testkit owner %s", owner)
		}
	}
}

func TestFullCoverageRequiresExecutionPartitionsAndCompleteOtherOwners(t *testing.T) {
	for _, mode := range []string{"complete", "old partition owner", "missing test", "duplicate test", "execution repeated in others", "host omitted", "wrong reported owner", "missing integration test", "duplicate integration test", "integration repeated in others", "wrong integration owner"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			report := scopedReport{Scope: "full", CorePackage: fullCorePackage,
				CoreTests: []string{"TestA", "TestB", "TestC"}, IntegrationPackage: boundaryPackage,
				IntegrationTests: []string{"TestIA", "TestIB", "TestIC"}, AllPackages: []string{fullCorePackage, boundaryPackage, corePackage, "task"}}
			if mode == "wrong reported owner" {
				report.CorePackage = corePackage
			}
			addGroup := func(id string, assigned, packages []string, raw string) {
				path := filepath.Join(root, id+".jsonl")
				if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
					t.Fatal(err)
				}
				report.Groups = append(report.Groups, scopedGroup{ID: id, Assigned: assigned, Packages: packages,
					StdoutLog: path, Summary: testfeedback.Parse([]byte(raw), 0)})
			}
			for i, assigned := range report.CoreTests {
				pkg, name := fullCorePackage, assigned
				if mode == "old partition owner" {
					pkg = corePackage
				}
				if mode == "duplicate test" && i == 1 {
					name = "TestA"
				}
				raw := packageEvent("start", pkg)
				if mode != "missing test" || i != 1 {
					raw += strings.ReplaceAll(event("run", name)+event("pass", name), corePackage, pkg)
				}
				raw += packageEvent("pass", pkg)
				addGroup(fmt.Sprintf("core-%d", i+1), []string{assigned}, []string{pkg}, raw)
			}
			for i, assigned := range report.IntegrationTests {
				pkg, name := boundaryPackage, assigned
				if mode == "wrong integration owner" {
					pkg = corePackage
				}
				if mode == "duplicate integration test" && i == 1 {
					name = "TestIA"
				}
				raw := packageEvent("start", pkg)
				if mode != "missing integration test" || i != 1 {
					raw += strings.ReplaceAll(event("run", name)+event("pass", name), corePackage, pkg)
				}
				raw += packageEvent("pass", pkg)
				addGroup(fmt.Sprintf("integration-%d", i+1), []string{assigned}, []string{pkg}, raw)
			}
			others := []string{corePackage, "task"}
			if mode == "host omitted" {
				others = []string{"task"}
			}
			if mode == "execution repeated in others" {
				others = append(others, fullCorePackage)
			}
			if mode == "integration repeated in others" {
				others = append(others, boundaryPackage)
			}
			var raw string
			for _, pkg := range others {
				raw += packageEvent("start", pkg) + packageEvent("pass", pkg)
			}
			addGroup("other-packages", nil, others, raw)
			problems := checkScopedCoverage(report)
			if (len(problems) == 0) != (mode == "complete") {
				t.Fatalf("coverage mode %q: %v", mode, problems)
			}
		})
	}
}

func TestPrepareIntegrationPartitionEnumeratesBusinessWiring(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	binary, dir, err := prepareTestPackage(ctx, t.TempDir(), boundaryPackage, "integration")
	if err != nil {
		t.Fatal(err)
	}
	names, err := listCoreTests(ctx, binary, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(names, "TestCandidateClientWiring") || !slices.Contains(names, "TestTM04ExecutionWiring") {
		t.Fatal("Full integration partition omitted current business owners")
	}
}

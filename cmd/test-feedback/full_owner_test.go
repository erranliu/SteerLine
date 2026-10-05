package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/testfeedback"
)

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

func TestFullCoverageRequiresExecutionPartitionsAndCompleteOtherOwners(t *testing.T) {
	for _, mode := range []string{"complete", "old partition owner", "missing test", "duplicate test", "execution repeated in others", "host omitted", "wrong reported owner"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			report := scopedReport{Scope: "full", CorePackage: fullCorePackage,
				CoreTests: []string{"TestA", "TestB", "TestC"}, AllPackages: []string{fullCorePackage, corePackage, "task"}}
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
			others := []string{corePackage, "task"}
			if mode == "host omitted" {
				others = []string{"task"}
			}
			if mode == "execution repeated in others" {
				others = append(others, fullCorePackage)
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

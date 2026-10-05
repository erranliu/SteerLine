package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/testfeedback"
)

func TestAssignTestsCoversNewNamesOnceAndBalancesLongCases(t *testing.T) {
	names := []string{"TestTiny", "TestLongA", "TestNew", "TestLongB"}
	groups := assignTests(names, map[string]float64{"TestLongA": 10, "TestLongB": 9, "TestTiny": 0}, 3)
	if len(groups) != 3 || len(groups[0]) == 0 || len(groups[1]) == 0 || len(groups[2]) == 0 {
		t.Fatalf("unbalanced groups: %v", groups)
	}
	seen := map[string]int{}
	for _, group := range groups {
		for _, name := range group {
			seen[name]++
		}
	}
	for _, name := range names {
		if seen[name] != 1 {
			t.Fatalf("%s assigned %d times: %v", name, seen[name], groups)
		}
	}
	longGroup := map[string]int{}
	for index, group := range groups {
		for _, name := range group {
			longGroup[name] = index
		}
	}
	if longGroup["TestLongA"] == longGroup["TestLongB"] {
		t.Fatalf("long cases share allocation: %v", groups)
	}
}

func TestOwnedProcessHelper(t *testing.T) {
	mode := os.Getenv("STEERLINE_TEST_OWNED_HELPER")
	if mode == "" {
		return
	}
	marker := os.Getenv("STEERLINE_TEST_OWNED_MARKER")
	if mode == "child" {
		time.Sleep(1200 * time.Millisecond)
		if err := os.WriteFile(marker, []byte("survived"), 0600); err != nil {
			t.Fatal(err)
		}
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestOwnedProcessHelper$")
	cmd.Env = append(os.Environ(), "STEERLINE_TEST_OWNED_HELPER=child")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(marker+".started", []byte("started"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestRunOwnedKillsDescendantsOnCancellation(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "descendant")
	cmd := exec.Command(os.Args[0], "-test.run=^TestOwnedProcessHelper$")
	cmd.Env = append(os.Environ(), "STEERLINE_TEST_OWNED_HELPER=parent", "STEERLINE_TEST_OWNED_MARKER="+marker)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- runOwned(ctx, cmd) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(marker + ".started"); err == nil {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("helper stopped before starting descendant: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			cancel()
			<-done
			t.Fatal("helper did not start descendant")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	if err := <-done; err != context.Canceled {
		t.Fatalf("cancelled process result = %v", err)
	}
	time.Sleep(1400 * time.Millisecond)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("owned descendant survived cancellation: %v", err)
	}
}

func TestScopedCoverageRejectsMissingDuplicateAndTruncatedEvents(t *testing.T) {
	for _, tc := range []struct {
		name, events string
		wantPass     bool
	}{
		{"complete", eventsFor("run", "pass", "pass"), true},
		{"missing", eventsFor("run", "", "pass"), false},
		{"duplicate", eventsFor("run", "pass", "pass") + event("run", "TestA") + event("pass", "TestA"), false},
		{"skip", eventsFor("run", "skip", "pass"), false},
		{"truncated", strings.TrimSuffix(eventsFor("run", "pass", "pass"), "\n"), false},
		{"invalid", "{broken}\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "events.jsonl")
			if err := os.WriteFile(path, []byte(tc.events), 0600); err != nil {
				t.Fatal(err)
			}
			var rulesRaw []byte
			for _, pkg := range quickRulePackages {
				rulesRaw = append(rulesRaw, packageEvent("start", pkg)...)
				rulesRaw = append(rulesRaw, packageEvent("pass", pkg)...)
			}
			rulesPath := filepath.Join(root, "rules.jsonl")
			if err := os.WriteFile(rulesPath, rulesRaw, 0600); err != nil {
				t.Fatal(err)
			}
			group := scopedGroup{ID: "quick-core", Assigned: []string{"TestA"}, Packages: []string{corePackage}, StdoutLog: path, Summary: testfeedback.Parse([]byte(tc.events), 0)}
			rules := scopedGroup{ID: "quick-rules", Packages: quickRulePackages, StdoutLog: rulesPath, Summary: testfeedback.Parse(rulesRaw, 0)}
			var boundaryRaw []byte
			boundaryRaw = append(boundaryRaw, packageEvent("start", boundaryPackage)...)
			for _, name := range quickBoundaryAssigned {
				boundaryRaw = append(boundaryRaw, []byte(strings.ReplaceAll(event("run", name), corePackage, boundaryPackage))...)
				boundaryRaw = append(boundaryRaw, []byte(strings.ReplaceAll(event("pass", name), corePackage, boundaryPackage))...)
			}
			boundaryRaw = append(boundaryRaw, packageEvent("pass", boundaryPackage)...)
			boundaryPath := filepath.Join(root, "boundary.jsonl")
			if err := os.WriteFile(boundaryPath, boundaryRaw, 0600); err != nil {
				t.Fatal(err)
			}
			boundary := scopedGroup{ID: "quick-boundary", Assigned: quickBoundaryAssigned, Packages: []string{boundaryPackage}, StdoutLog: boundaryPath, Summary: testfeedback.Parse(boundaryRaw, 0)}
			report := scopedReport{Scope: "quick", Groups: append([]scopedGroup{group, rules, boundary}, syntheticQuickOwners(t, root)...), AllPackages: quickPackages}
			problems := checkScopedCoverage(report)
			if tc.wantPass && len(problems) != 0 {
				t.Fatalf("complete stream added errors: %v", problems)
			}
			if !tc.wantPass && len(problems) == 0 {
				t.Fatalf("invalid stream was not identified: %v", problems)
			}
		})
	}
}

func TestScopedOptionsAndNativeSwitchGate(t *testing.T) {
	for _, args := range [][]string{{"-quick", "-workers=3"}, {"-workers=3", "--", "./internal/app/host"}, {"-workers=4"}, {"-quick", "--", "./..."}, {"-quick", "--", "./internal/app/host", "-count=2"}} {
		if _, err := parseOptions(args); err == nil {
			t.Fatalf("accepted invalid options %v", args)
		}
	}
	if _, err := parseOptions([]string{"-quick", "--", "./internal/task/model", "-run=^TestExample$"}); err != nil {
		t.Fatalf("focused quick extension rejected: %v", err)
	}
	t.Setenv("STEERLINE_REAL_SAMPLE", "1")
	if got := activeRealSwitches(); !containsSwitch(got, "STEERLINE_REAL_SAMPLE") {
		t.Fatalf("native switches=%v", got)
	}
}

func containsSwitch(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func event(action, test string) string {
	part := ""
	if test != "" {
		part = fmt.Sprintf(`,"Test":%q`, test)
	}
	return fmt.Sprintf(`{"Time":"2026-09-28T00:00:00Z","Action":%q,"Package":%q%s,"Elapsed":0.1}`+"\n", action, corePackage, part)
}

func packageEvent(action, pkg string) string {
	return fmt.Sprintf(`{"Time":"2026-09-28T00:00:00Z","Action":%q,"Package":%q,"Elapsed":0.1}`+"\n", action, pkg)
}

func eventsFor(run, terminal, pkgTerminal string) string {
	stream := event("start", "")
	if run != "" {
		stream += event(run, "TestA")
	}
	if terminal != "" {
		stream += event(terminal, "TestA")
	}
	if pkgTerminal != "" {
		stream += event(pkgTerminal, "")
	}
	return stream
}

func TestQuickBoundaryCoverageRequiresEachAssignedEntry(t *testing.T) {
	for _, tc := range []struct {
		name, omitted string
		duplicate     bool
		wantFailure   bool
	}{
		{name: "complete"},
		{name: "missing_parent", omitted: "TestCD67CoreBoundary", wantFailure: true},
		{name: "missing_recovery_gate", omitted: "TestCD67AdditionalImplementation/supported_terminal_recovery_and_original_identity", wantFailure: true},
		{name: "missing_claim_rejection", omitted: "TestCD67AdditionalImplementation/remote_claim_requires_permission_evidence", wantFailure: true},
		{name: "duplicate", duplicate: true, wantFailure: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := packageEvent("start", boundaryPackage)
			for _, name := range quickBoundaryAssigned {
				if name == tc.omitted {
					continue
				}
				raw += strings.ReplaceAll(event("run", name)+event("pass", name), corePackage, boundaryPackage)
			}
			if tc.duplicate {
				raw += strings.ReplaceAll(event("run", quickBoundaryTests[0])+event("pass", quickBoundaryTests[0]), corePackage, boundaryPackage)
			}
			raw += packageEvent("pass", boundaryPackage)
			path := filepath.Join(t.TempDir(), "boundary.jsonl")
			if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
			group := scopedGroup{ID: "quick-boundary", Assigned: quickBoundaryAssigned, Packages: []string{boundaryPackage}, StdoutLog: path, Summary: testfeedback.Parse([]byte(raw), 0)}
			problems := checkScopedCoverage(scopedReport{Scope: "quick", Groups: []scopedGroup{group}})
			boundaryFailure := false
			for _, problem := range problems {
				if strings.HasPrefix(problem, "quick-boundary:") {
					boundaryFailure = true
				}
			}
			if boundaryFailure != tc.wantFailure {
				t.Fatalf("boundary coverage failure=%t, want %t: %v", boundaryFailure, tc.wantFailure, problems)
			}
		})
	}
}

func syntheticQuickOwners(t *testing.T, root string) []scopedGroup {
	t.Helper()
	var groups []scopedGroup
	for _, owner := range quickOwnerCases {
		raw := packageEvent("start", owner.Package)
		for _, name := range owner.Tests {
			raw += strings.ReplaceAll(event("run", name)+event("pass", name), corePackage, owner.Package)
		}
		raw += packageEvent("pass", owner.Package)
		path := filepath.Join(root, owner.ID+".jsonl")
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		groups = append(groups, scopedGroup{ID: owner.ID, Assigned: owner.Tests, Packages: []string{owner.Package}, StdoutLog: path, Summary: testfeedback.Parse([]byte(raw), 0)})
	}
	return groups
}

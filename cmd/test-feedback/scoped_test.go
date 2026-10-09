package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	testfeedback "github.com/erranliu/SteerLine/internal/testkit/feedback"
)

func TestQuickWorkersDefaultAndExplicitSelections(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want int
	}{
		{[]string{"-level=L1"}, 3},
		{[]string{"-quick"}, 3},
		{[]string{"-level=L1", "-workers=1"}, 1},
		{[]string{"-quick", "-workers=3"}, 3},
		{[]string{"-level=L1", "-workers=3", "--", "./tests/integration", "-run=^TestOne$"}, 3},
		{nil, 1},
	} {
		opts, err := parseOptions(tc.args)
		if err != nil || opts.workers != tc.want {
			t.Fatalf("%v: workers=%d want=%d err=%v", tc.args, opts.workers, tc.want, err)
		}
	}
}

func TestQuickGroupsBoundConcurrencyAndPreserveReportOrder(t *testing.T) {
	for _, workers := range []int{1, 3} {
		t.Run(fmt.Sprint(workers), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			invocations := make([]scopedInvocation, 7)
			for i := range invocations {
				invocations[i].ID = fmt.Sprint(i)
			}
			entered := make(chan string, len(invocations))
			release := make(chan struct{})
			defer close(release)
			var active, peak atomic.Int32
			done := make(chan []scopedGroup, 1)
			go func() {
				done <- runQuickGroups(workers, invocations, func(inv scopedInvocation) scopedGroup {
					count := active.Add(1)
					defer active.Add(-1)
					for old := peak.Load(); count > old; old = peak.Load() {
						if peak.CompareAndSwap(old, count) {
							break
						}
					}
					entered <- inv.ID
					select {
					case <-release:
					case <-ctx.Done():
					}
					return scopedGroup{ID: inv.ID}
				})
			}()
			for i := 0; i < workers; i++ {
				select {
				case <-entered:
				case <-ctx.Done():
					t.Fatal("workers did not start concurrently")
				}
			}
			// Release one job at a time; idle slots may take later jobs while
			// earlier jobs remain blocked, exercising both reuse and ordering.
			for i := range invocations {
				select {
				case release <- struct{}{}:
				case <-ctx.Done():
					t.Fatalf("job %d did not finish", i)
				}
			}
			select {
			case groups := <-done:
				if peak.Load() != int32(workers) || len(groups) != len(invocations) {
					t.Fatalf("peak=%d groups=%d", peak.Load(), len(groups))
				}
				for i, group := range groups {
					if group.ID != invocations[i].ID {
						t.Fatalf("report order changed: %v", groups)
					}
				}
			case <-ctx.Done():
				t.Fatal("scheduler did not collect all results")
			}
		})
	}
}

func TestQuickIntegrationSelectionKeepsClaimAndGuardsWithoutLongFlows(t *testing.T) {
	parts := strings.Split(quickBoundaryPattern(), "/")
	top, sub := regexp.MustCompile(parts[0]), regexp.MustCompile(parts[1])
	for _, name := range quickBoundaryAssigned {
		levels := strings.Split(name, "/")
		if !top.MatchString(levels[0]) || (len(levels) > 1 && !sub.MatchString(levels[1])) {
			t.Fatalf("required coverage excluded: %s", name)
		}
	}
	for _, name := range []string{"TestTM04ExecutionWiring/intent_binding_and_two_original_turns", "TestTM04ExecutionWiring/original_request_ack_and_manual_recovery", "TestCD66FixedProfileRecovery/changed_config_keeps_native_original_identity_and_manual_gate", "TestCD67AdditionalImplementation/neutral_fixed_input_restart_and_fresh_manual_answer"} {
		levels := strings.Split(name, "/")
		if top.MatchString(levels[0]) && sub.MatchString(levels[1]) {
			t.Fatalf("long flow still selected: %s", name)
		}
	}
}

func TestQuickTargetedIntegrationDoesNotReplaceFixedCoverage(t *testing.T) {
	opts := options{goTestArgs: []string{"./tests/integration", "-run=^TestSomeRegression$"}}
	invocations := quickInvocations(opts)
	boundary, targeted := invocations[len(invocations)-2], invocations[len(invocations)-1]
	if boundary.ID != "quick-boundary" || targeted.ID != "quick-targeted" || len(boundary.Assigned) != len(quickBoundaryAssigned) {
		t.Fatal("directed regression replaced fixed coverage")
	}
	if flagValue(boundary.Command, "-run", "") != quickBoundaryPattern() || flagValue(targeted.Command, "-run", "") != "^TestSomeRegression$" {
		t.Fatalf("selection lost: %v / %v", boundary.Command, targeted.Command)
	}
}

func TestQuickTargetedCoverageRejectsZeroMatches(t *testing.T) {
	for _, matched := range []bool{false, true} {
		t.Run(fmt.Sprint(matched), func(t *testing.T) {
			raw := packageEvent("start", boundaryPackage)
			if matched {
				raw += strings.ReplaceAll(event("run", "TestDirected")+event("pass", "TestDirected"), corePackage, boundaryPackage)
			}
			raw += packageEvent("pass", boundaryPackage)
			path := filepath.Join(t.TempDir(), "targeted.jsonl")
			if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
			group := scopedGroup{ID: "quick-targeted", StdoutLog: path, Summary: testfeedback.Parse([]byte(raw), 0)}
			problems := checkScopedCoverage(scopedReport{Scope: "quick", Groups: []scopedGroup{group}})
			rejected := containsSwitch(problems, "quick-targeted: no tests matched the explicit selection")
			if rejected == matched {
				t.Fatalf("matched=%t problems=%v", matched, problems)
			}
		})
	}
}

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
	for _, args := range [][]string{{"-workers=3", "--", "./internal/app/host"}, {"-workers=4"}, {"-quick", "--", "./..."}, {"-quick", "--", "./internal/app/host", "-count=2"}} {
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
		unexpected    bool
		wantFailure   bool
	}{
		{name: "complete"},
		{name: "missing_parent", omitted: "TestCD67CoreBoundary", wantFailure: true},
		{name: "missing_claim_transaction", omitted: "TestTM04ExecutionWiring/claim_transaction_failure_and_original_receipt", wantFailure: true},
		{name: "missing_storage_wiring", omitted: "TestTM07StorageWiring", wantFailure: true},
		{name: "duplicate", duplicate: true, wantFailure: true},
		{name: "unexpected_recovery", unexpected: true, wantFailure: true},
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
			if tc.unexpected {
				name := "TestTM04ExecutionWiring/original_request_ack_and_manual_recovery"
				raw += strings.ReplaceAll(event("run", name)+event("pass", name), corePackage, boundaryPackage)
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

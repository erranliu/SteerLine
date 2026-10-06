package feedback

import (
	"strings"
	"testing"
)

func TestParseKeepsNestedDurationsSeparateAndUsesTimestampWall(t *testing.T) {
	input := strings.Join([]string{
		`{"Time":"2026-09-26T00:00:00Z","Action":"start","Package":"example/a"}`,
		`{"Time":"2026-09-26T00:00:00Z","Action":"run","Package":"example/a","Test":"TestParent"}`,
		`{"Time":"2026-09-26T00:00:00Z","Action":"run","Package":"example/a","Test":"TestParent/child"}`,
		`{"Time":"2026-09-26T00:00:01Z","Action":"pass","Package":"example/a","Test":"TestParent/child","Elapsed":1}`,
		`{"Time":"2026-09-26T00:00:02Z","Action":"pass","Package":"example/a","Test":"TestParent","Elapsed":2}`,
		`{"Time":"2026-09-26T00:00:02Z","Action":"pass","Package":"example/a","Elapsed":2}`,
		"",
	}, "\n")
	summary := Parse([]byte(input), 0)
	if !summary.Passed || !summary.Complete {
		t.Fatalf("valid nested log was not successful: %+v", summary)
	}
	if summary.TestCount != 2 || summary.PackageCount != 1 {
		t.Fatalf("unexpected event counts: %+v", summary)
	}
	if summary.TestEventWallSeconds != 2 {
		t.Fatalf("test event wall = %v, want 2 seconds", summary.TestEventWallSeconds)
	}
	if len(summary.Packages) != 1 || summary.Packages[0].ElapsedSeconds == nil || *summary.Packages[0].ElapsedSeconds != 2 {
		t.Fatalf("package elapsed = %+v, want 2 seconds", summary.Packages)
	}
	if len(summary.SlowestTests) != 2 || summary.SlowestTests[0].Name != "TestParent" {
		t.Fatalf("slowest nested tests are wrong: %+v", summary.SlowestTests)
	}
}

func TestParseDoesNotSumParallelPackageTimesAsWallTime(t *testing.T) {
	input := strings.Join([]string{
		`{"Time":"2026-09-26T00:00:00Z","Action":"start","Package":"example/a"}`,
		`{"Time":"2026-09-26T00:00:00Z","Action":"start","Package":"example/b"}`,
		`{"Time":"2026-09-26T00:00:00Z","Action":"run","Package":"example/a","Test":"TestA"}`,
		`{"Time":"2026-09-26T00:00:00Z","Action":"run","Package":"example/b","Test":"TestB"}`,
		`{"Time":"2026-09-26T00:00:03Z","Action":"pass","Package":"example/a","Test":"TestA","Elapsed":3}`,
		`{"Time":"2026-09-26T00:00:04Z","Action":"pass","Package":"example/a","Elapsed":3}`,
		`{"Time":"2026-09-26T00:00:04Z","Action":"pass","Package":"example/b","Test":"TestB","Elapsed":4}`,
		`{"Time":"2026-09-26T00:00:05Z","Action":"pass","Package":"example/b","Elapsed":4}`,
		"",
	}, "\n")
	summary := Parse([]byte(input), 0)
	if !summary.Passed {
		t.Fatalf("parallel package log was not successful: %+v", summary)
	}
	if summary.TestEventWallSeconds != 5 {
		t.Fatalf("parallel package event wall = %v, want 5 seconds", summary.TestEventWallSeconds)
	}
	if len(summary.Packages) != 2 {
		t.Fatalf("parallel package summaries = %+v, want 2 packages", summary.Packages)
	}
	packageElapsed := 0.0
	for _, pkg := range summary.Packages {
		if pkg.ElapsedSeconds == nil {
			t.Fatalf("package elapsed missing: %+v", pkg)
		}
		packageElapsed += *pkg.ElapsedSeconds
	}
	if packageElapsed != 7 || packageElapsed == summary.TestEventWallSeconds {
		t.Fatalf("parallel package elapsed sum = %v; event wall = %v", packageElapsed, summary.TestEventWallSeconds)
	}
}

func TestParseReportsFailuresSkipsAndProcessExit(t *testing.T) {
	input := strings.Join([]string{
		`{"Time":"2026-09-26T00:00:00Z","Action":"start","Package":"example/a"}`,
		`{"Time":"2026-09-26T00:00:00Z","Action":"run","Package":"example/a","Test":"TestFail"}`,
		`{"Time":"2026-09-26T00:00:01Z","Action":"fail","Package":"example/a","Test":"TestFail","Elapsed":1}`,
		`{"Time":"2026-09-26T00:00:01Z","Action":"run","Package":"example/a","Test":"TestSkip"}`,
		`{"Time":"2026-09-26T00:00:01Z","Action":"skip","Package":"example/a","Test":"TestSkip","Elapsed":0}`,
		`{"Time":"2026-09-26T00:00:01Z","Action":"fail","Package":"example/a","Elapsed":1}`,
		`{"Time":"2026-09-26T00:00:02Z","Action":"skip","Package":"example/no-tests","Elapsed":0}`,
		"",
	}, "\n")
	summary := Parse([]byte(input), 1)
	if summary.Passed || summary.Status != "failed" || !summary.Complete {
		t.Fatalf("failed test log has wrong status: %+v", summary)
	}
	if summary.FailedTests != 1 || summary.SkippedTests != 1 || len(summary.Failures) != 1 || len(summary.Skipped) != 1 {
		t.Fatalf("failure/skip counts are wrong: %+v", summary)
	}
	if summary.PackageCount != 2 || len(summary.FailedPackages) != 1 || len(summary.SkippedPackages) != 1 {
		t.Fatalf("failed or skipped package status was lost: %+v", summary)
	}
}

func TestParseRejectsMalformedTruncatedAndMissingTerminalEvents(t *testing.T) {
	for name, input := range map[string][]byte{
		"malformed":           []byte("{not json}\n"),
		"truncated":           []byte(`{"Time":"2026-09-26T00:00:00Z","Action":"start","Package":"example/a"}`),
		"missing package end": []byte("{\"Time\":\"2026-09-26T00:00:00Z\",\"Action\":\"start\",\"Package\":\"example/a\"}\n{\"Time\":\"2026-09-26T00:00:00Z\",\"Action\":\"run\",\"Package\":\"example/a\",\"Test\":\"TestPending\"}\n"),
	} {
		t.Run(name, func(t *testing.T) {
			summary := Parse(input, 0)
			if summary.Passed || summary.Complete || summary.Status != "incomplete" || len(summary.IncompleteReasons) == 0 {
				t.Fatalf("incomplete log was accepted: %+v", summary)
			}
		})
	}
}

func TestParseTreatsProcessFailureWithoutTerminalTestsAsIncomplete(t *testing.T) {
	summary := Parse(nil, 2)
	if summary.Passed || summary.Complete || summary.Status != "incomplete" {
		t.Fatalf("missing process output was accepted: %+v", summary)
	}
}

func TestParseToolchainBuildEventsPreservesActualFailures(t *testing.T) {
	for _, failed := range []bool{false, true} {
		lines := []string{`{"ImportPath":"example/pkg [example/pkg.test]","Action":"build-output","Output":"compiler diagnostic"}`}
		code, status := 0, "pass"
		if failed {
			lines = append(lines, `{"ImportPath":"example/pkg [example/pkg.test]","Action":"build-fail"}`)
			code, status = 1, "fail"
		}
		lines = append(lines, `{"Time":"2026-10-02T00:00:00Z","Action":"start","Package":"example/pkg"}`,
			`{"Time":"2026-10-02T00:00:01Z","Action":"`+status+`","Package":"example/pkg","Elapsed":1}`, "")
		summary := Parse([]byte(strings.Join(lines, "\n")), code)
		if !summary.Complete || summary.Passed == failed || summary.PackageCount != 1 || (len(summary.BuildFailures) > 0) != failed {
			t.Fatalf("build events changed completion or hid failure: %+v", summary)
		}
	}
	if got := Parse([]byte(`{"Action":"build-output"}`+"\n"), 0); got.Passed || got.Complete {
		t.Fatal("malformed build event accepted")
	}
}

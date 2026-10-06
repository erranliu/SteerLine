package feedback

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

const maxJSONLine = 32 << 20

// Event is one event from go test -json.
type Event struct {
	ImportPath string   `json:"ImportPath,omitempty"`
	Time       string   `json:"Time"`
	Action     string   `json:"Action"`
	Package    string   `json:"Package"`
	Test       string   `json:"Test,omitempty"`
	Elapsed    *float64 `json:"Elapsed,omitempty"`
}

type Package struct {
	Name           string   `json:"package"`
	Status         string   `json:"status"`
	ElapsedSeconds *float64 `json:"elapsed_seconds,omitempty"`
}

type Test struct {
	Package        string   `json:"package"`
	Name           string   `json:"name"`
	Status         string   `json:"status"`
	ElapsedSeconds *float64 `json:"elapsed_seconds,omitempty"`
}

type Summary struct {
	Status               string    `json:"status"`
	Passed               bool      `json:"passed"`
	Complete             bool      `json:"complete"`
	CommandExitCode      int       `json:"command_exit_code"`
	PackageCount         int       `json:"package_count"`
	PassedPackages       int       `json:"passed_packages"`
	BuildFailures        []string  `json:"build_failures,omitempty"`
	FailedPackages       []string  `json:"failed_packages"`
	SkippedPackages      []string  `json:"skipped_packages"`
	TestCount            int       `json:"test_count"`
	PassedTests          int       `json:"passed_tests"`
	FailedTests          int       `json:"failed_tests"`
	SkippedTests         int       `json:"skipped_tests"`
	Packages             []Package `json:"packages"`
	Tests                []Test    `json:"tests"`
	SlowestTests         []Test    `json:"slowest_tests"`
	Failures             []Test    `json:"failures"`
	Skipped              []Test    `json:"skipped"`
	IncompleteReasons    []string  `json:"incomplete_reasons"`
	TestEventWallSeconds float64   `json:"test_event_wall_seconds"`
	HasTestEventWall     bool      `json:"has_test_event_wall"`
}

// Parse summarizes a go test -json stream. Package and test elapsed values stay
// per-item because packages may run concurrently and parent tests overlap their
// children. TestEventWallSeconds is based on event timestamps.
func Parse(raw []byte, commandExitCode int) Summary {
	s := Summary{
		CommandExitCode:   commandExitCode,
		Complete:          true,
		FailedPackages:    []string{},
		SkippedPackages:   []string{},
		Packages:          []Package{},
		Tests:             []Test{},
		SlowestTests:      []Test{},
		Failures:          []Test{},
		Skipped:           []Test{},
		IncompleteReasons: []string{},
	}
	startedPackages := map[string]bool{}
	finishedPackages := map[string]bool{}
	pendingTests := map[string]int{}
	var eventTimes []time.Time
	addIncomplete := func(reason string) {
		s.Complete = false
		for _, existing := range s.IncompleteReasons {
			if existing == reason {
				return
			}
		}
		s.IncompleteReasons = append(s.IncompleteReasons, reason)
	}
	if len(raw) > 0 && raw[len(raw)-1] != '\n' {
		addIncomplete("JSON log does not end with a newline; the final record may be truncated")
	}

	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 64*1024), maxJSONLine)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var event Event
		if err := json.Unmarshal(line, &event); err != nil {
			addIncomplete(fmt.Sprintf("invalid JSON at line %d: %v", lineNumber, err))
			continue
		}
		if event.Action == "build-output" || event.Action == "build-fail" {
			if event.ImportPath == "" || event.Package != "" || event.Test != "" {
				addIncomplete(fmt.Sprintf("invalid build event at line %d", lineNumber))
			} else if event.Action == "build-fail" {
				s.BuildFailures = append(s.BuildFailures, event.ImportPath)
			}
			continue
		}
		if event.Package == "" || event.Action == "" {
			addIncomplete(fmt.Sprintf("missing package or action at line %d", lineNumber))
			continue
		}
		if event.Time == "" {
			addIncomplete(fmt.Sprintf("missing event timestamp at line %d", lineNumber))
		} else if at, err := time.Parse(time.RFC3339Nano, event.Time); err != nil {
			addIncomplete(fmt.Sprintf("invalid event timestamp at line %d: %v", lineNumber, err))
		} else {
			eventTimes = append(eventTimes, at)
		}

		if event.Test == "" {
			if event.Action == "start" {
				startedPackages[event.Package] = true
			}
			if isTerminal(event.Action) {
				finishedPackages[event.Package] = true
				var elapsed *float64
				if event.Elapsed == nil {
					addIncomplete(fmt.Sprintf("package %s ended without elapsed time", event.Package))
				} else {
					value := *event.Elapsed
					elapsed = &value
				}
				s.Packages = append(s.Packages, Package{Name: event.Package, Status: event.Action, ElapsedSeconds: elapsed})
				switch event.Action {
				case "pass":
					s.PassedPackages++
				case "fail":
					s.FailedPackages = append(s.FailedPackages, event.Package)
				case "skip":
					s.SkippedPackages = append(s.SkippedPackages, event.Package)
				}
			}
			continue
		}

		key := event.Package + "\x00" + event.Test
		if event.Action == "run" {
			pendingTests[key]++
			continue
		}
		if !isTerminal(event.Action) {
			continue
		}
		if pendingTests[key] == 0 {
			addIncomplete(fmt.Sprintf("test %s in %s ended without a matching run event", event.Test, event.Package))
		} else {
			pendingTests[key]--
		}
		var elapsed *float64
		if event.Elapsed == nil {
			addIncomplete(fmt.Sprintf("test %s in %s ended without elapsed time", event.Test, event.Package))
		} else {
			value := *event.Elapsed
			elapsed = &value
		}
		test := Test{Package: event.Package, Name: event.Test, Status: event.Action, ElapsedSeconds: elapsed}
		s.Tests = append(s.Tests, test)
		switch event.Action {
		case "pass":
			s.PassedTests++
		case "fail":
			s.FailedTests++
			s.Failures = append(s.Failures, test)
		case "skip":
			s.SkippedTests++
			s.Skipped = append(s.Skipped, test)
		}
	}
	if err := scanner.Err(); err != nil {
		addIncomplete("could not read complete JSON log: " + err.Error())
	}
	for pkg := range startedPackages {
		if !finishedPackages[pkg] {
			addIncomplete("package " + pkg + " has no terminal event")
		}
	}
	for key, count := range pendingTests {
		if count > 0 {
			parts := strings.SplitN(key, "\x00", 2)
			addIncomplete(fmt.Sprintf("test %s in %s has %d unmatched run event(s)", parts[1], parts[0], count))
		}
	}
	if len(startedPackages) == 0 && len(finishedPackages) == 0 {
		addIncomplete("JSON log contains no package start or terminal event")
	}
	if commandExitCode != 0 && s.Complete && s.FailedTests == 0 && len(s.FailedPackages) == 0 && len(s.BuildFailures) == 0 {
		addIncomplete(fmt.Sprintf("go test process exited with code %d without a terminal test failure", commandExitCode))
	}

	s.PackageCount = len(finishedPackages)
	s.TestCount = len(s.Tests)
	if len(eventTimes) >= 2 {
		sort.Slice(eventTimes, func(i, j int) bool { return eventTimes[i].Before(eventTimes[j]) })
		s.TestEventWallSeconds = eventTimes[len(eventTimes)-1].Sub(eventTimes[0]).Seconds()
		s.HasTestEventWall = true
	}
	sort.Slice(s.Packages, func(i, j int) bool {
		left, right := elapsed(s.Packages[i].ElapsedSeconds), elapsed(s.Packages[j].ElapsedSeconds)
		if left == right {
			return s.Packages[i].Name < s.Packages[j].Name
		}
		return left > right
	})
	sort.Strings(s.FailedPackages)
	sort.Strings(s.SkippedPackages)
	s.Tests = sortSlowest(s.Tests)
	s.Failures = sortSlowest(s.Failures)
	s.Skipped = sortSlowest(s.Skipped)
	if len(s.Tests) > 20 {
		s.SlowestTests = append([]Test(nil), s.Tests[:20]...)
	} else if len(s.Tests) > 0 {
		s.SlowestTests = append([]Test(nil), s.Tests...)
	} else {
		s.SlowestTests = []Test{}
	}
	s.Passed = commandExitCode == 0 && s.Complete && s.FailedTests == 0 && len(s.FailedPackages) == 0 && len(s.BuildFailures) == 0
	s.Status = "passed"
	if !s.Complete {
		s.Status = "incomplete"
	} else if commandExitCode != 0 || s.FailedTests > 0 || len(s.FailedPackages) > 0 || len(s.BuildFailures) > 0 {
		s.Status = "failed"
	}
	return s
}

func isTerminal(action string) bool {
	return action == "pass" || action == "fail" || action == "skip"
}

func elapsed(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func sortSlowest(tests []Test) []Test {
	sort.Slice(tests, func(i, j int) bool {
		left, right := elapsed(tests[i].ElapsedSeconds), elapsed(tests[j].ElapsedSeconds)
		if left == right {
			if tests[i].Package == tests[j].Package {
				return tests[i].Name < tests[j].Name
			}
			return tests[i].Package < tests[j].Package
		}
		return left > right
	})
	return tests
}

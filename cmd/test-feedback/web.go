package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	feedback "github.com/erranliu/SteerLine/internal/testkit/feedback"
)

var quickWebFiles = []string{"src/shared/api/client.test.ts", "src/features/runs/state.test.ts"}

type vitestReport struct {
	Success         bool `json:"success"`
	NumTotalTests   int  `json:"numTotalTests"`
	NumPassedTests  int  `json:"numPassedTests"`
	NumFailedTests  int  `json:"numFailedTests"`
	NumPendingTests int  `json:"numPendingTests"`
	NumTodoTests    int  `json:"numTodoTests"`
	TestResults     []struct {
		Name       string `json:"name"`
		Status     string `json:"status"`
		Assertions []struct {
			FullName string `json:"fullName"`
			Status   string `json:"status"`
		} `json:"assertionResults"`
	} `json:"testResults"`
}

func webTestFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(filepath.Join(root, "src"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && (strings.HasSuffix(path, ".test.ts") || strings.HasSuffix(path, ".test.tsx")) {
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(files)
	if err == nil && len(files) == 0 {
		err = fmt.Errorf("no Web test files")
	}
	return files, err
}

// Parse native Vitest evidence directly; no synthetic Go test events are emitted.
func parseWebReport(raw []byte, root string, expected []string, exit int) feedback.Summary {
	s := feedback.Summary{Status: "incomplete", CommandExitCode: exit, Complete: true}
	fail := func(reason string) { s.Complete = false; s.IncompleteReasons = append(s.IncompleteReasons, reason) }
	var report vitestReport
	if err := json.Unmarshal(raw, &report); err != nil {
		fail("read native Vitest JSON: " + err.Error())
		return s
	}
	want, seen := map[string]bool{}, map[string]bool{}
	for _, file := range expected {
		want[filepath.ToSlash(filepath.Clean(file))] = true
	}
	if len(want) == 0 {
		fail("empty Web selection")
	}
	for _, file := range report.TestResults {
		rel, err := filepath.Rel(root, file.Name)
		name := filepath.ToSlash(rel)
		if err != nil || !want[name] || seen[name] {
			fail("unexpected or duplicate Web file: " + file.Name)
		}
		seen[name] = true
		if len(file.Assertions) == 0 {
			fail("Web file has zero assertions: " + name)
		}
		tests := map[string]bool{}
		for _, a := range file.Assertions {
			if a.FullName == "" || tests[a.FullName] {
				fail("missing or duplicate assertion identity: " + name + "/" + a.FullName)
			}
			tests[a.FullName] = true
			test := feedback.Test{Package: name, Name: a.FullName, Status: a.Status}
			switch a.Status {
			case "passed":
				test.Status = "pass"
				s.PassedTests++
			case "failed":
				test.Status = "fail"
				s.FailedTests++
				s.Failures = append(s.Failures, test)
			default:
				test.Status = "skip"
				s.SkippedTests++
				s.Skipped = append(s.Skipped, test)
				fail("non-executed Web assertion: " + a.Status)
			}
			s.Tests = append(s.Tests, test)
		}
		status := "pass"
		if file.Status != "passed" {
			status = "fail"
			s.FailedPackages = append(s.FailedPackages, name)
		} else {
			s.PassedPackages++
		}
		s.Packages = append(s.Packages, feedback.Package{Name: name, Status: status})
	}
	for name := range want {
		if !seen[name] {
			fail("missing Web file: " + name)
		}
	}
	s.TestCount, s.PackageCount = len(s.Tests), len(s.Packages)
	if s.TestCount == 0 || report.NumTotalTests != s.TestCount || report.NumPassedTests != s.PassedTests || report.NumFailedTests != s.FailedTests || report.NumPendingTests+report.NumTodoTests != s.SkippedTests {
		fail("native Vitest counters differ or zero matches")
	}
	s.Passed = s.Complete && report.Success && exit == 0 && s.FailedTests == 0 && s.SkippedTests == 0 && len(s.FailedPackages) == 0
	if s.Complete {
		s.Status = "failed"
		if s.Passed {
			s.Status = "passed"
		}
	}
	return s
}

func runWebGroup(ctx context.Context, out, id string, files []string) scopedGroup {
	g := scopedGroup{ID: id, ExitCode: 127}
	g.ExecutionParameters = executionParametersFor([]string{"node"}, nil)
	root, err := e2eRoot()
	if err != nil {
		g.Error = err.Error()
		return g
	}
	web := filepath.Join(root, "web")
	if files == nil {
		files, err = webTestFiles(web)
		if err != nil {
			g.Error = err.Error()
			return g
		}
	}
	g.Packages = append([]string{}, files...)
	dir, err := filepath.Abs(filepath.Join(out, id))
	if err != nil {
		g.Error = err.Error()
		return g
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		g.Error = err.Error()
		return g
	}
	g.StdoutLog, g.StderrLog = filepath.Join(dir, "vitest.json"), filepath.Join(dir, "stderr.txt")
	stdout, err := os.Create(filepath.Join(dir, "stdout.txt"))
	if err != nil {
		g.Error = err.Error()
		return g
	}
	defer stdout.Close()
	stderr, err := os.Create(g.StderrLog)
	if err != nil {
		g.Error = err.Error()
		return g
	}
	defer stderr.Close()
	cli := filepath.Join(web, "node_modules", "vitest", "vitest.mjs")
	if _, err = os.Stat(cli); err != nil {
		g.Error = "Web dependencies unavailable; run npm --prefix web ci: " + err.Error()
		return g
	}
	args := append([]string{cli, "run", "--reporter=json", "--outputFile=" + g.StdoutLog}, files...)
	g.Command = append([]string{"node"}, args...)
	g.ExecutionParameters = executionParametersFor(g.Command, nil)
	cmd := exec.Command("node", args...)
	cmd.Dir, cmd.Stdout, cmd.Stderr = web, stdout, stderr
	started := time.Now()
	g.StartedAt = started.UTC().Format(time.RFC3339Nano)
	err = runOwned(ctx, cmd)
	finishExecutionParameters(&g.ExecutionParameters, cmd.ProcessState != nil)
	g.WallSeconds, g.FinishedAt = time.Since(started).Seconds(), time.Now().UTC().Format(time.RFC3339Nano)
	g.ExitCode = 0
	if err != nil {
		g.Error = err.Error()
		g.ExitCode = 127
		if exit, ok := err.(*exec.ExitError); ok {
			g.ExitCode = exit.ExitCode()
		}
	}
	raw, readErr := os.ReadFile(g.StdoutLog)
	if readErr != nil {
		g.Error += "; Vitest report unavailable: " + readErr.Error()
	}
	g.Summary = parseWebReport(raw, web, files, g.ExitCode)
	return g
}

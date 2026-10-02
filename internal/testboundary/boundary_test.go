package testboundary

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

const module = "github.com/erranliu/SteerLine"

func under(path, prefix string) bool { return path == prefix || strings.HasPrefix(path, prefix+"/") }

func testOnly(path string) bool {
	for _, suffix := range []string{"/tests", "/cmd/test-feedback", "/internal/testfeedback", "/internal/testprocess", "/internal/testboundary", "/internal/steerline/testprotocol"} {
		if under(path, module+suffix) {
			return true
		}
	}
	return false
}

type goPackage struct {
	ImportPath                         string
	Imports, TestImports, XTestImports []string
}

func list(t *testing.T, args ...string) []goPackage {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", append([]string{"list", "-json"}, args...)...)
	cmd.Dir = filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	raw, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v: %s", err, stderr.String())
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var packages []goPackage
	for {
		var p goPackage
		err := decoder.Decode(&p)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		packages = append(packages, p)
	}
	if len(packages) == 0 {
		t.Fatal("dependency inventory is empty")
	}
	return packages
}

func violations(graph map[string][]string, roots []string) []string {
	var failures []string
	var queue [][]string
	for _, root := range roots {
		queue = append(queue, []string{root})
	}
	seen := map[string]bool{}
	for len(queue) > 0 {
		path := queue[0]
		queue = queue[1:]
		node := path[len(path)-1]
		if seen[node] {
			continue
		}
		seen[node] = true
		if testOnly(node) {
			failures = append(failures, strings.Join(path, " -> "))
			continue
		}
		for _, dep := range graph[node] {
			next := append(append([]string(nil), path...), dep)
			queue = append(queue, next)
		}
	}
	sort.Strings(failures)
	return failures
}

func TestProductionHasNoTestDependencies(t *testing.T) {
	var roots []string
	for _, p := range list(t, "./cmd/steerline", "./internal/steerline/...") {
		if !testOnly(p.ImportPath) {
			roots = append(roots, p.ImportPath)
		}
	}
	graph := map[string][]string{}
	for _, p := range list(t, append([]string{"-deps"}, roots...)...) {
		graph[p.ImportPath] = p.Imports
	}
	if failures := violations(graph, roots); len(failures) > 0 {
		t.Fatalf("production imports test tooling:\n%s", strings.Join(failures, "\n"))
	}
}

func e2eAllowed(source, dependency string) bool {
	production := module + "/internal/steerline"
	if !under(dependency, production) {
		return true
	}
	for _, adapter := range []string{"driver", "native", "evidence"} {
		if under(source, module+"/tests/e2e/internal/"+adapter) {
			return true
		}
	}
	return dependency == production+"/contracts" || dependency == production+"/task"
}

func TestE2EScenariosUseTestAdapters(t *testing.T) {
	var failures []string
	for _, p := range list(t, "-tags=e2e", "./tests/e2e/...", "./tests/e2e/testdata/controlled-helper") {
		imports := append(append(append([]string(nil), p.Imports...), p.TestImports...), p.XTestImports...)
		for _, dep := range imports {
			if !e2eAllowed(p.ImportPath, dep) {
				failures = append(failures, fmt.Sprintf("%s -> %s", p.ImportPath, dep))
			}
		}
	}
	sort.Strings(failures)
	if len(failures) > 0 {
		t.Fatalf("E2E implementation imports must stay in test adapters:\n%s", strings.Join(failures, "\n"))
	}
}

func TestBoundaryRulesRejectTransitiveLeaksAndDirectImplementationImports(t *testing.T) {
	app, lib, fake := module+"/cmd/steerline", module+"/internal/steerline/query", module+"/tests/e2e/internal/controlled"
	graph := map[string][]string{app: {lib}, lib: {fake}}
	if got := violations(graph, []string{app}); len(got) != 1 || got[0] != app+" -> "+lib+" -> "+fake {
		t.Fatalf("transitive dependency was not reported: %v", got)
	}
	if got := violations(map[string][]string{app: {lib}, lib: {"fmt"}}, []string{app}); len(got) != 0 {
		t.Fatalf("production-only graph rejected: %v", got)
	}
	for _, source := range []string{"/tests/e2e/internal/testenv", "/tests/e2e/live", "/tests/e2e/smoke", "/tests/e2e/suites/recovery"} {
		if e2eAllowed(module+source, module+"/internal/steerline/codexworker") || e2eAllowed(module+source, module+"/internal/steerline") {
			t.Fatalf("implementation leak accepted from %s", source)
		}
		if !e2eAllowed(module+source, module+"/internal/steerline/contracts") {
			t.Fatal("data contract rejected")
		}
	}
	for _, adapter := range []string{"driver", "native", "evidence"} {
		if !e2eAllowed(module+"/tests/e2e/internal/"+adapter, module+"/internal/steerline/codexworker") {
			t.Fatal("test adapter rejected")
		}
	}
}

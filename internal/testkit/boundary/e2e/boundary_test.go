package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

const module = "github.com/erranliu/SteerLine"

func under(path, prefix string) bool { return path == prefix || strings.HasPrefix(path, prefix+"/") }

func testOnly(path string) bool {
	for _, suffix := range []string{"/tests", "/cmd/test-feedback", "/internal/testkit"} {
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

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("missing source location")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../../../.."))
}

func walkGoSources(t *testing.T, root string, visit func(path, importer string, file *ast.File)) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") {
			return nil
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		visit(path, module+"/"+filepath.ToSlash(rel), parsed)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func list(t *testing.T, args ...string) []goPackage {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", append([]string{"list", "-json"}, args...)...)
	cmd.Dir = repositoryRoot(t)
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
	for _, p := range list(t, "./cmd/steerline", "./internal/app/...", "./internal/execution/...") {
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

func toolTestkitAllowed(importer, dependency string) bool {
	process := module + "/internal/testkit/process"
	remote := module + "/internal/testkit/remotetest"
	feedback := module + "/internal/testkit/feedback"
	allowed := map[string]map[string]bool{
		module + "/cmd/test-feedback": {
			feedback: true,
			remote:   true,
			process:  true,
		},
		// Controlled external CLI fixtures share protocol behavior with integration.
		module + "/tests/e2e/internal/controlled": {process: true, module + "/internal/testkit/protocol": true},
		// The debug driver publishes the test-management proof consumed by feedback.
		module + "/tests/e2e/internal/driver":  {process: true, feedback: true},
		module + "/tests/e2e/internal/testenv": {process: true, feedback: true, remote: true},
	}
	return allowed[importer][dependency]
}

func TestProductionSourcesUseOnlyOwnedTestkitSupport(t *testing.T) {
	var failures []string
	walkGoSources(t, repositoryRoot(t), func(path, importer string, file *ast.File) {
		if strings.HasSuffix(path, "_test.go") {
			return
		}
		for _, spec := range file.Imports {
			dependency, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Errorf("%s: invalid import: %v", path, err)
				continue
			}
			if under(dependency, module+"/tests") && !(under(importer, module+"/tests/e2e") && under(dependency, module+"/tests/e2e/internal")) {
				failures = append(failures, filepath.ToSlash(path)+" -> "+dependency)
			}
			if under(dependency, module+"/internal/testkit") && !toolTestkitAllowed(importer, dependency) {
				failures = append(failures, filepath.ToSlash(path)+" -> "+dependency)
			}
		}
	})
	if len(failures) > 0 {
		sort.Strings(failures)
		t.Fatalf("production source has a testkit/tests dependency outside its named tool owner:\n%s", strings.Join(failures, "\n"))
	}
}

func TestRemoteTestSupportHasOnlyNamedToolOwners(t *testing.T) {
	dependency := module + "/internal/testkit/remotetest"
	for _, owner := range []string{"/cmd/test-feedback", "/tests/e2e/internal/testenv"} {
		if !toolTestkitAllowed(module+owner, dependency) {
			t.Fatal("named tool owner rejected", owner)
		}
	}
	for _, owner := range []string{"/cmd/steerline", "/internal/app", "/internal/product/query", "/tests/e2e/live", "/tests/e2e/internal/driver", "/cmd/test-feedback/unnamed"} {
		if toolTestkitAllowed(module+owner, dependency) {
			t.Fatal("unnamed owner accepted", owner)
		}
	}
	for _, owner := range []string{"/cmd/test-feedback", "/tests/e2e/internal/testenv"} {
		if toolTestkitAllowed(module+owner, module+"/internal/testkit/unowned") {
			t.Fatal("arbitrary support package accepted", owner)
		}
	}
}

func TestIntegrationDoesNotImportE2EInternal(t *testing.T) {
	root := repositoryRoot(t)
	integration := filepath.Join(root, "tests", "integration")
	var failures []string
	walkGoSources(t, integration, func(path, importer string, file *ast.File) {
		for _, spec := range file.Imports {
			dependency, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Errorf("%s: invalid import: %v", path, err)
				continue
			}
			if under(dependency, module+"/tests/e2e/internal") {
				failures = append(failures, filepath.ToSlash(path)+" -> "+dependency)
			}
		}
	})
	if len(failures) > 0 {
		sort.Strings(failures)
		t.Fatalf("tests/integration imports E2E internal packages:\n%s", strings.Join(failures, "\n"))
	}
}

func e2eAllowed(source, dependency string) bool {
	if under(dependency, module+"/tests/e2e/internal/native") {
		return false
	}
	for _, retired := range []string{"adapters", "workflow", "reliable", "conversation"} {
		if under(dependency, module+"/internal/execution/"+retired) {
			return false
		}
	}
	if under(dependency, "github.com/erranliu/SteerRunner") {
		return false
	}
	production := module + "/internal/app/host"
	executionResources := []string{
		module + "/internal/execution/adapters",
		module + "/internal/execution/workspace",
		module + "/internal/execution/publication",
		module + "/internal/execution/process",
	}
	restricted := under(dependency, production) || under(dependency, module+"/internal/app")
	for _, root := range executionResources {
		restricted = restricted || under(dependency, root)
	}
	if !restricted {
		return true
	}
	for _, adapter := range []string{"driver", "evidence"} {
		if under(source, module+"/tests/e2e/internal/"+adapter) {
			return true
		}
	}
	for _, valueOwner := range []string{module + "/internal/execution/model", module + "/internal/execution/reconstruction", module + "/internal/product/api", module + "/internal/task/model"} {
		if dependency == valueOwner || under(dependency, valueOwner) {
			return true
		}
	}
	return false
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
	app, lib, fake := module+"/cmd/steerline", module+"/internal/product/query", module+"/tests/e2e/internal/controlled"
	graph := map[string][]string{app: {lib}, lib: {fake}}
	if got := violations(graph, []string{app}); len(got) != 1 || got[0] != app+" -> "+lib+" -> "+fake {
		t.Fatalf("transitive dependency was not reported: %v", got)
	}
	if got := violations(map[string][]string{app: {lib}, lib: {"fmt"}}, []string{app}); len(got) != 0 {
		t.Fatalf("production-only graph rejected: %v", got)
	}
	for _, source := range []string{"/tests/e2e/internal/testenv", "/tests/e2e/live", "/tests/e2e/smoke", "/tests/e2e/suites/recovery"} {
		if e2eAllowed(module+source, module+"/internal/execution/adapters/codex") || e2eAllowed(module+source, module+"/internal/app/host") || e2eAllowed(module+source, module+"/internal/app") {
			t.Fatalf("implementation leak accepted from %s", source)
		}
		if !e2eAllowed(module+source, module+"/internal/product/api") {
			t.Fatal("product value contract rejected")
		}
	}
	for _, adapter := range []string{"driver", "evidence"} {
		if e2eAllowed(module+"/tests/e2e/internal/"+adapter, module+"/internal/execution/adapters/codex") || e2eAllowed(module+"/tests/e2e/internal/"+adapter, "github.com/erranliu/SteerRunner/internal/app") {
			t.Fatal("private execution owner accepted by test tooling")
		}
	}
}

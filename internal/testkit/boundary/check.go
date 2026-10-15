// Package boundary checks source and dependency boundaries.
// It lives outside the core so the checker cannot weaken the boundary it checks.
package boundary

import (
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const module = "github.com/erranliu/SteerLine"

const core = module + "/internal/app/host"
const execution = module + "/internal/execution"
const sharedFixtures = module + "/internal/testkit/fixtures"

var platformText = regexp.MustCompile(`(?i)codex|orca`)

const productQuery = module + "/internal/product/query"
const console = module + "/internal/console"
const storageSQLite = module + "/internal/storage/sqlite"

var executionAdapters = execution + "/adapters"

// Report contains deterministic source diagnostics, including the full path
// through a helper when a forbidden dependency is transitive.
type Report struct {
	PureFiles      map[string]int `json:"pure_files"`
	ConsoleFiles   int            `json:"console_files"`
	CoreFiles      int            `json:"core_files"`
	ExecutionFiles int            `json:"execution_files"`
	SourcePackages int            `json:"source_packages"`
	Dependencies   map[string]int `json:"production_dependencies"`
	Violations     []string       `json:"violations"`
}

// PureImportsAllowed encodes the ownership matrix for definitions already
// migrated by #73. Transitional aggregations are checked by the existing guards.
func PureImportsAllowed(owner, dependency string) bool {
	if dependency == "database/sql" {
		return false
	}
	if !strings.Contains(strings.Split(dependency, "/")[0], ".") {
		return true
	}
	switch owner {
	case module + "/internal/product/query/taskgraph":
		return dependency == module+"/internal/task/model" || dependency == module+"/internal/execution/model" || dependency == module+"/internal/product/api" || dependency == module+"/internal/storage/ports"
	case module + "/internal/task":
		return dependency == module+"/internal/task/model"
	case module + "/internal/execution/model":
		return dependency == module+"/internal/execution/backend" || dependency == execution+"/agentlaunch"
	case module + "/internal/execution/conversation":
		return dependency == module+"/internal/execution/model" || dependency == module+"/internal/execution/backend"
	case module + "/internal/runnerstate":
		return dependency == module+"/internal/runnercontract"
	case module + "/internal/storage/ports":
		return dependency == module+"/internal/runnercontract" || dependency == module+"/internal/runnerstate" || dependency == module+"/internal/task/model" || dependency == module+"/internal/execution/model" || dependency == module+"/internal/execution/backend" || dependency == module+"/internal/product/api"
	}
	return false
}

var purePackages = []string{module + "/internal/runnercontract", module + "/internal/runnerstate", module + "/internal/product/query/taskgraph", module + "/internal/task", module + "/internal/task/model", module + "/internal/execution/model", module + "/internal/execution/backend", module + "/internal/execution/conversation", execution + "/agentlaunch", module + "/internal/product/api", module + "/internal/storage/ports"}

func PureReconstructionImportAllowed(dependency string) bool {
	if !strings.Contains(strings.Split(dependency, "/")[0], ".") {
		return true
	}
	return dependency == module+"/internal/task/model" || dependency == module+"/internal/execution/model" || dependency == module+"/internal/execution/backend" || dependency == module+"/internal/storage/ports"
}

func purePackage(pkg string) bool {
	for _, owner := range purePackages {
		if pkg == owner {
			return true
		}
	}
	return false
}

func implementation(pkg string) bool {
	for _, path := range []string{executionAdapters} {
		if pkg == path || strings.HasPrefix(pkg, path+"/") {
			return true
		}
	}
	return false
}

func forbidden(pkg string) bool {
	return implementation(pkg) || pkg == module+"/internal/app" || strings.HasPrefix(pkg, module+"/internal/app/") || pkg == module+"/internal/testkit" || strings.HasPrefix(pkg, module+"/internal/testkit/") || pkg == module+"/tests/integration" || strings.HasPrefix(pkg, module+"/tests/integration/")
}

func consoleForbidden(pkg string) bool {
	if pkg == console || pkg == console+"/activityrender" || pkg == module+"/internal/product/api" {
		return false
	}
	if pkg == core || pkg == execution || pkg == "database/sql" || pkg == "modernc.org/sqlite" || strings.HasPrefix(pkg, "modernc.org/sqlite/") || forbidden(pkg) {
		return true
	}
	if strings.HasPrefix(pkg, module+"/internal/") && pkg != module+"/internal/product/api" {
		return true
	}
	for _, p := range []string{module + "/internal/storage/sqlite", productQuery, execution + "/reconstruction", execution + "/workspace", execution + "/publication", execution + "/process", module + "/internal/product/client"} {
		if pkg == p || strings.HasPrefix(pkg, p+"/") {
			return true
		}
	}
	return false
}

func httpConsumerPackage(pkg string) bool {
	for _, suffix := range []string{"/internal/consoleapp", "/internal/projectconnection", "/internal/pathguard", "/internal/localservice", "/internal/product/applicationclient", "/internal/product/applicationcontract", "/internal/product/httpclient", "/internal/product/httpcontract", "/internal/product/httpfs"} {
		if pkg == module+suffix {
			return true
		}
	}
	return false
}

func hostForbidden(pkg string) bool {
	if pkg == execution {
		return true
	}
	if pkg == module+"/internal/task/model" {
		return false
	}
	for _, prefix := range []string{
		"database/sql",
		"modernc.org/sqlite",
		storageSQLite,
		executionAdapters,
		execution + "/process",
		execution + "/workspace",
		execution + "/publication",
		module + "/internal/task",
		module + "/internal/product/query",
		module + "/internal/console",
		module + "/internal/app",
	} {
		if prefix == module+"/internal/task" && (pkg == prefix+"/model" || strings.HasPrefix(pkg, prefix+"/model/")) {
			continue
		}
		if pkg == prefix || strings.HasPrefix(pkg, prefix+"/") {
			return true
		}
	}
	return false
}

// The execution application consumes task/model and injected ports. Task
// implementations, old-host business access and concrete implementations are
// forbidden even through a helper or an inactive OS/build-tag file.
func executionForbidden(pkg string) bool {
	if pkg == core || pkg == module+"/internal/task" || forbidden(pkg) || pkg == "database/sql" || strings.HasPrefix(pkg, "modernc.org/sqlite") {
		return true
	}
	for _, owner := range []string{module + "/internal/storage/sqlite", console, productQuery, module + "/internal/product/client"} {
		if pkg == owner || strings.HasPrefix(pkg, owner+"/") {
			return true
		}
	}
	return strings.HasPrefix(pkg, module+"/internal/task/") && pkg != module+"/internal/task/model"
}

// Adapter implementations may consume backend capabilities and pure model values,
// dedicated protocol packages and execution resource helpers. They cannot
// depend on application use cases, task implementations, storage, SQL, app or
// product code.
func adapterForbidden(pkg string) bool {
	if pkg == module+"/internal/testkit" || strings.HasPrefix(pkg, module+"/internal/testkit/") {
		return true
	}
	if pkg == "database/sql" || strings.HasPrefix(pkg, "modernc.org/sqlite") {
		return true
	}
	// Agent launch configuration is a pure execution value,
	// not application use cases. Their dependencies are still traversed.
	if pkg == execution+"/agentlaunch" {
		return false
	}
	if pkg == execution || strings.HasPrefix(pkg, execution+"/") {
		for _, allowed := range []string{
			execution + "/backend", execution + "/model", execution + "/conversation", execution + "/adapters",
			execution + "/workspace", execution + "/publication", execution + "/process",
		} {
			if pkg == allowed || strings.HasPrefix(pkg, allowed+"/") {
				return false
			}
		}
		return true
	}
	if pkg == module+"/internal/task" || strings.HasPrefix(pkg, module+"/internal/task/") {
		return pkg != module+"/internal/task/model" && !strings.HasPrefix(pkg, module+"/internal/task/model/")
	}
	// Adapter source must use execution-owned values and must not depend on
	// public product DTOs through an unrelated helper package.
	for _, allowed := range []string{module + "/internal/product/api"} {
		if pkg == allowed || strings.HasPrefix(pkg, allowed+"/") {
			return false
		}
	}
	if pkg == core || strings.HasPrefix(pkg, core+"/") {
		return true
	}
	return pkg == console || strings.HasPrefix(pkg, console+"/") || strings.HasPrefix(pkg, module+"/internal/storage/") ||
		(pkg != module+"/internal/product/api" && strings.HasPrefix(pkg, module+"/internal/product/")) ||
		strings.HasPrefix(pkg, module+"/internal/app") ||
		strings.HasPrefix(pkg, module+"/internal/product/client") ||
		strings.HasPrefix(pkg, module+"/tests/integration")
}

func adapterProductValue(pkg string) bool {
	for _, pure := range []string{module + "/internal/product/api"} {
		if pkg == pure || strings.HasPrefix(pkg, pure+"/") {
			return true
		}
	}
	return false
}

func adapterDirectForbidden(pkg string) bool {
	return adapterForbidden(pkg) || adapterProductValue(pkg)
}

// Track both routes to a value package: a shorter permitted contract route
// must not hide a forbidden route through an unrelated helper.
func inspectAdapterGraph(graph map[string][]string, seed string, reject func([]string)) {
	type entry struct {
		path []string
		pkg  string
	}
	seen := map[string]bool{seed: true}
	queue := []entry{{path: []string{seed}, pkg: seed}}
	for len(queue) != 0 {
		current := queue[0]
		queue = queue[1:]
		for _, next := range graph[current.pkg] {
			if seen[next] {
				continue
			}
			seen[next] = true
			path := append(append([]string(nil), current.path...), next)
			if adapterForbidden(next) || adapterProductValue(next) {
				reject(path)
			}
			queue = append(queue, entry{path: path, pkg: next})
		}
	}
}

func productQueryImportAllowed(dependency string) bool {
	if dependency == "database/sql" {
		return false
	}
	if !strings.Contains(strings.Split(dependency, "/")[0], ".") {
		return true
	}
	for _, allowed := range []string{module + "/internal/runnercontract", module + "/internal/runnerstate", module + "/internal/product/api", productQuery + "/taskgraph", module + "/internal/task/model", module + "/internal/execution/model", module + "/internal/execution/backend", module + "/internal/storage/ports"} {
		if dependency == allowed {
			return true
		}
	}
	return false
}

func storageSQLiteImportAllowed(dependency string) bool {
	if !strings.HasPrefix(dependency, module+"/internal/") {
		return true
	}
	for _, allowed := range []string{
		module + "/internal/runnercontract", module + "/internal/runnerstate",
		module + "/internal/execution/backend", module + "/internal/execution/model",
		module + "/internal/product/api", productQuery,
		module + "/internal/storage/ports", module + "/internal/task/model",
	} {
		if dependency == allowed {
			return true
		}
	}
	return false
}

func sqliteProductQuerySelectorAllowed(file *ast.File) []string {
	allowed := map[string]bool{"CandidateSummary": true, "RunnerHistory": true, "MaxEventPageBytes": true, "NativeTaskProtocol": true, "ReadySnapshot": true, "SetTaskPageCursor": true}
	aliases := map[string]bool{}
	for _, spec := range file.Imports {
		dependency, err := strconv.Unquote(spec.Path.Value)
		if err != nil || dependency != productQuery {
			continue
		}
		alias := "query"
		if spec.Name != nil {
			alias = spec.Name.Name
		}
		aliases[alias] = true
	}
	var violations []string
	ast.Inspect(file, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		identifier, ok := selector.X.(*ast.Ident)
		if ok && aliases[identifier.Name] && !allowed[selector.Sel.Name] {
			violations = append(violations, selector.Sel.Name)
		}
		return true
	})
	return violations
}

// ScanSource parses production and test Go files regardless of GOOS or build tags.
func ScanSource(root string) (Report, error) {
	report := Report{Dependencies: map[string]int{}, PureFiles: map[string]int{}}
	graph := map[string][]string{}
	productionGraph := map[string][]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			// Git metadata is not source. Every other directory participates,
			// including names commonly used for generated output or helpers.
			if filepath.Dir(path) == filepath.Clean(root) && entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") {
			return nil
		}
		rel, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		pkg := module + "/" + filepath.ToSlash(rel)
		if pkg == core {
			report.CoreFiles++
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if platformText.MatchString(entry.Name()) || platformText.Match(data) {
				report.Violations = append(report.Violations, "platform text: "+filepath.ToSlash(rel)+"/"+entry.Name())
			}
		}
		if pkg == console {
			report.ConsoleFiles++
		}
		if pkg == execution || pkg == module+"/internal/taskrunner" {
			report.ExecutionFiles++
		}
		if purePackage(pkg) {
			report.PureFiles[pkg]++
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		if _, ok := graph[pkg]; !ok {
			graph[pkg] = nil
		}
		for _, spec := range parsed.Imports {
			value, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			// Shared adapters are available only to these two test owners. Keep
			// their dependency edge so transitive forbidden imports remain visible.
			if value == sharedFixtures && !(strings.HasSuffix(entry.Name(), "_test.go") && (pkg == core || pkg == execution)) {
				report.Violations = append(report.Violations, "test fixture dependency: "+filepath.ToSlash(path)+" -> "+value)
			}
			graph[pkg] = append(graph[pkg], value)
			if (pkg == execution+"/workflow" || pkg == execution+"/reliable") && !strings.HasSuffix(entry.Name(), "_test.go") && (strings.HasPrefix(value, module+"/") || value == "database/sql" || strings.HasPrefix(value, "modernc.org/sqlite")) {
				allowed := pkg == execution+"/workflow" && value == execution+"/reliable" || pkg == execution+"/reliable" && value == execution+"/backend"
				if !allowed {
					report.Violations = append(report.Violations, "three-layer direct dependency: "+filepath.ToSlash(path)+" -> "+value)
				}
			}

			if pkg == productQuery && !strings.HasSuffix(entry.Name(), "_test.go") && !productQueryImportAllowed(value) {
				report.Violations = append(report.Violations, "product query forbidden dependency: "+filepath.ToSlash(path)+" -> "+value)
			}
			if pkg == storageSQLite && !strings.HasSuffix(entry.Name(), "_test.go") && !storageSQLiteImportAllowed(value) {
				report.Violations = append(report.Violations, "storage sqlite forbidden dependency: "+filepath.ToSlash(path)+" -> "+value)
			}
			if pkg == storageSQLite && !strings.HasSuffix(entry.Name(), "_test.go") && value == productQuery {
				for _, selector := range sqliteProductQuerySelectorAllowed(parsed) {
					report.Violations = append(report.Violations, "storage sqlite product query application call: "+filepath.ToSlash(path)+" -> "+selector)
				}
			}

			if implementation(pkg) && !strings.HasSuffix(entry.Name(), "_test.go") && adapterDirectForbidden(value) {
				report.Violations = append(report.Violations, "execution adapter forbidden dependency: "+filepath.ToSlash(path)+" -> "+value)
			}
			if purePackage(pkg) && !PureImportsAllowed(pkg, value) {
				report.Violations = append(report.Violations, "pure ownership dependency: "+filepath.ToSlash(path)+" -> "+value)
			}
			if pkg == module+"/internal/execution/reconstruction" && !PureReconstructionImportAllowed(value) {
				report.Violations = append(report.Violations, "execution reconstruction dependency: "+filepath.ToSlash(path)+" -> "+value)
			}
			if pkg == execution && !strings.HasSuffix(entry.Name(), "_test.go") && value == productQuery {
				report.Violations = append(report.Violations, "execution product read dependency: "+filepath.ToSlash(path)+" -> "+value)
			}
			if !strings.HasSuffix(entry.Name(), "_test.go") {
				productionGraph[pkg] = append(productionGraph[pkg], value)
			}
			if pkg == core && !strings.HasSuffix(entry.Name(), "_test.go") && (value == "database/sql" || value == module+"/internal/storage/sqlite" || strings.HasPrefix(value, "modernc.org/sqlite")) {
				report.Violations = append(report.Violations, "concrete storage dependency: "+filepath.ToSlash(path)+" -> "+value)
			}
			if pkg == core && !strings.HasSuffix(entry.Name(), "_test.go") && hostForbidden(value) {
				report.Violations = append(report.Violations, "app host forbidden dependency: "+filepath.ToSlash(path)+" -> "+value)
			}
		}
		return nil
	})
	if err != nil {
		return report, err
	}
	report.SourcePackages = len(graph)
	for pkg := range graph {
		sort.Strings(graph[pkg])
	}
	for pkg := range productionGraph {
		sort.Strings(productionGraph[pkg])
	}
	visitHostGraph(productionGraph, core, func(path []string) {
		last := path[len(path)-1]
		if last == "database/sql" || last == storageSQLite || strings.HasPrefix(last, storageSQLite+"/") || strings.HasPrefix(last, "modernc.org/sqlite") {
			report.Violations = append(report.Violations, "concrete storage dependency: "+strings.Join(path, " -> "))
			return
		}
		if hostForbidden(last) {
			report.Violations = append(report.Violations, "app host forbidden dependency: "+strings.Join(path, " -> "))
		}
	})
	visitGraph(productionGraph, execution, func(path []string) {
		if executionForbidden(path[len(path)-1]) {
			report.Violations = append(report.Violations, "execution forbidden dependency: "+strings.Join(path, " -> "))
		}
	})
	for _, seed := range []string{execution + "/workflow", execution + "/reliable"} {
		visitGraph(productionGraph, seed, func(path []string) {
			edge := path[len(path)-1]
			if !strings.HasPrefix(edge, module+"/") && edge != "database/sql" && !strings.HasPrefix(edge, "modernc.org/sqlite") {
				return
			}
			allowed := edge == execution+"/backend" || seed == execution+"/workflow" && edge == execution+"/reliable"
			if !allowed {
				report.Violations = append(report.Violations, "three-layer dependency: "+strings.Join(path, " -> "))
			}
		})
	}
	for pkg := range productionGraph {
		if !strings.HasPrefix(pkg, executionAdapters+"/") {
			continue
		}
		inspectAdapterGraph(productionGraph, pkg, func(path []string) {
			report.Violations = append(report.Violations, "execution adapter forbidden dependency: "+strings.Join(path, " -> "))
		})
	}
	for _, seed := range []string{module + "/internal/storage/sqlite", module + "/internal/storage/ports"} {
		visitGraph(graph, seed, func(path []string) {
			last := path[len(path)-1]
			if last == core || last == execution || forbidden(last) || last == module+"/internal/product/client" || strings.HasPrefix(last, module+"/internal/product/client/") {
				report.Violations = append(report.Violations, "reverse storage dependency: "+strings.Join(path, " -> "))
			}
		})
	}
	for _, seed := range append([]string{core, execution}, purePackages...) {
		visitGraph(graph, seed, func(path []string) {
			if forbidden(path[len(path)-1]) && !((seed == core || seed == execution) && path[len(path)-1] == sharedFixtures) {
				report.Violations = append(report.Violations, "forbidden dependency: "+strings.Join(path, " -> "))
			}
		})
	}
	visitGraph(graph, console, func(path []string) {
		// HTTP integration fixtures may exercise the real transport. Production
		// views still receive interfaces and must not construct their own adapter.
		if consoleForbidden(path[len(path)-1]) && !httpConsumerPackage(path[len(path)-1]) {
			report.Violations = append(report.Violations, "console forbidden dependency: "+strings.Join(path, " -> "))
		}
	})
	visitGraph(productionGraph, console, func(path []string) {
		if consoleForbidden(path[len(path)-1]) {
			report.Violations = append(report.Violations, "console production forbidden dependency: "+strings.Join(path, " -> "))
		}
	})
	for _, suffix := range []string{"/internal/consoleapp", "/internal/projectconnection", "/internal/pathguard", "/internal/localservice", "/internal/product/applicationclient", "/internal/product/applicationcontract", "/internal/product/httpclient", "/internal/product/httpcontract", "/internal/product/httpfs"} {
		visitGraph(productionGraph, module+suffix, func(path []string) {
			last := path[len(path)-1]
			if !httpConsumerPackage(last) && consoleForbidden(last) {
				report.Violations = append(report.Violations, "HTTP consumer forbidden dependency: "+strings.Join(path, " -> "))
			}
		})
	}
	for pkg := range graph {
		if !implementation(pkg) {
			continue
		}
		visitGraph(graph, pkg, func(path []string) {
			if path[len(path)-1] == core || path[len(path)-1] == execution {
				report.Violations = append(report.Violations, "reverse dependency: "+strings.Join(path, " -> "))
			}
		})
	}
	sort.Strings(report.Violations)
	return report, nil
}

func visitGraph(graph map[string][]string, seed string, inspect func([]string)) {
	seen := map[string]bool{seed: true}
	queue := [][]string{{seed}}
	for len(queue) != 0 {
		path := queue[0]
		queue = queue[1:]
		for _, next := range graph[path[len(path)-1]] {
			if seen[next] {
				continue
			}
			seen[next] = true
			chain := append(append([]string(nil), path...), next)
			inspect(chain)
			queue = append(queue, chain)
		}
	}
}

// visitHostGraph traverses every production dependency below the app host.
// Narrow value and storage ports remain subject to the same transitive check as
// direct imports so an owner split cannot hide a forbidden implementation.
func visitHostGraph(graph map[string][]string, seed string, inspect func([]string)) {
	seen := map[string]bool{seed: true}
	queue := [][]string{{seed}}
	for len(queue) > 0 {
		path := queue[0]
		queue = queue[1:]
		for _, next := range graph[path[len(path)-1]] {
			if seen[next] {
				continue
			}
			seen[next] = true
			chain := append(append([]string(nil), path...), next)
			inspect(chain)
			queue = append(queue, chain)
		}
	}
}

// Check adds Windows and Linux production graphs to the all-tags source scan.
// An unavailable go list is an error, never a skipped or passing boundary.
func Check(ctx context.Context, root string) (Report, error) {
	report, err := ScanSource(root)
	if err != nil {
		return report, err
	}
	productionRoots := map[string]bool{}
	for _, owner := range []string{core, module + "/internal/app", module + "/internal/taskrunner", module + "/internal/runnerbridge", module + "/internal/runnerclient", module + "/internal/task/model", module + "/internal/execution/model", module + "/internal/execution/backend", module + "/internal/product/api", productQuery, module + "/internal/product/query/taskgraph", module + "/internal/storage/ports"} {
		productionRoots[owner] = true
	}

	for _, target := range []string{"windows", "linux"} {
		for _, consoleGraph := range []bool{false, true} {

			cmd := exec.CommandContext(ctx, "go", "list", "-deps", "-json", "./internal/app/host", "./internal/app", "./internal/taskrunner", "./internal/runnerbridge", "./internal/runnerclient", "./internal/task/model", "./internal/execution/model", "./internal/execution/backend", "./internal/product/api", "./internal/product/query", "./internal/product/query/taskgraph", "./internal/storage/ports")
			if consoleGraph {
				cmd = exec.CommandContext(ctx, "go", "list", "-deps", "-json", "./internal/console")
			}
			cmd.Dir = root
			for _, value := range os.Environ() {
				key, _, _ := strings.Cut(value, "=")
				if !strings.EqualFold(key, "GOOS") && !strings.EqualFold(key, "GOARCH") {
					cmd.Env = append(cmd.Env, value)
				}
			}
			cmd.Env = append(cmd.Env, "GOOS="+target, "GOARCH=amd64")
			data, err := cmd.Output()
			if err != nil {
				return report, fmt.Errorf("%s production dependencies: %w", target, err)
			}
			decoder := json.NewDecoder(strings.NewReader(string(data)))
			standards := map[string]bool{}
			closures := map[string][]string{}
			for decoder.More() {
				var pkg struct {
					ImportPath string
					Standard   bool
					Imports    []string
					Deps       []string
					Error      *struct{ Err string }
					DepsErrors []struct{ Err string }
				}
				if err := decoder.Decode(&pkg); err != nil {
					return report, err
				}
				if pkg.Error != nil || len(pkg.DepsErrors) != 0 {
					return report, fmt.Errorf("%s dependency has errors: %s", target, pkg.ImportPath)
				}
				graphName := target
				if consoleGraph {
					graphName += "-console"
				}
				report.Dependencies[graphName]++
				standards[pkg.ImportPath] = pkg.Standard
				if pkg.ImportPath == module+"/internal/execution/reconstruction" {
					for _, dep := range pkg.Imports {
						if !PureReconstructionImportAllowed(dep) {
							report.Violations = append(report.Violations, graphName+" reconstruction dependency: "+pkg.ImportPath+" -> "+dep)
						}
					}
				}
				if purePackage(pkg.ImportPath) {
					for _, dep := range pkg.Imports {
						if !PureImportsAllowed(pkg.ImportPath, dep) {
							report.Violations = append(report.Violations, graphName+" pure ownership dependency: "+pkg.ImportPath+" -> "+dep)
						}
					}
					closures[pkg.ImportPath] = pkg.Deps
				}
				if pkg.ImportPath == execution {
					for _, dep := range append(append([]string{}, pkg.Imports...), pkg.Deps...) {
						if executionForbidden(dep) {
							report.Violations = append(report.Violations, graphName+" execution forbidden dependency: "+execution+" -> "+dep)
						}
					}
				}
				if pkg.ImportPath == productQuery {
					for _, dep := range pkg.Imports {
						if !productQueryImportAllowed(dep) {
							report.Violations = append(report.Violations, graphName+" product query dependency: "+productQuery+" -> "+dep)
						}
						if dep == "database/sql" || strings.HasPrefix(dep, "modernc.org/sqlite") {
							report.Violations = append(report.Violations, graphName+" product query concrete storage dependency: "+productQuery+" -> "+dep)
						}
					}
				}
				bad := !productionRoots[pkg.ImportPath] && (pkg.ImportPath == module+"/internal/testkit" || strings.HasPrefix(pkg.ImportPath, module+"/internal/testkit/") || pkg.ImportPath == module+"/tests/integration" || strings.HasPrefix(pkg.ImportPath, module+"/tests/integration/"))
				if consoleGraph {
					bad = consoleForbidden(pkg.ImportPath)
				}
				if bad {
					report.Violations = append(report.Violations, graphName+" production dependency: "+pkg.ImportPath)
				}
			}
			for owner, deps := range closures {
				for _, dep := range deps {
					if !standards[dep] && !purePackage(dep) || dep == "database/sql" {
						report.Violations = append(report.Violations, target+" pure transitive dependency: "+owner+" -> "+dep)
					}
				}
			}
		}
		adapterCmd := exec.CommandContext(ctx, "go", "list", "-deps", "-json", "./internal/execution/adapters/...")
		adapterCmd.Dir = root
		for _, value := range os.Environ() {
			key, _, _ := strings.Cut(value, "=")
			if !strings.EqualFold(key, "GOOS") && !strings.EqualFold(key, "GOARCH") {
				adapterCmd.Env = append(adapterCmd.Env, value)
			}
		}
		adapterCmd.Env = append(adapterCmd.Env, "GOOS="+target, "GOARCH=amd64")
		adapterData, err := adapterCmd.Output()
		if err != nil {
			return report, fmt.Errorf("%s adapter dependencies: %w", target, err)
		}
		adapterDecoder := json.NewDecoder(strings.NewReader(string(adapterData)))
		adapterGraph := map[string][]string{}
		for adapterDecoder.More() {
			var pkg struct {
				ImportPath string
				Imports    []string
				Deps       []string
				Error      *struct{ Err string }
				DepsErrors []struct{ Err string }
			}
			if err := adapterDecoder.Decode(&pkg); err != nil {
				return report, err
			}
			if pkg.Error != nil {
				return report, fmt.Errorf("%s adapter package %s: %s", target, pkg.ImportPath, pkg.Error.Err)
			}
			if len(pkg.DepsErrors) > 0 {
				return report, fmt.Errorf("%s adapter package %s: %s", target, pkg.ImportPath, pkg.DepsErrors[0].Err)
			}
			adapterGraph[pkg.ImportPath] = pkg.Imports
			report.Dependencies[target+"-adapters"]++
		}
		for pkg := range adapterGraph {
			if implementation(pkg) {
				inspectAdapterGraph(adapterGraph, pkg, func(path []string) {
					report.Violations = append(report.Violations, target+" adapter forbidden dependency: "+strings.Join(path, " -> "))
				})
			}
		}
	}

	sort.Strings(report.Violations)
	return report, nil
}

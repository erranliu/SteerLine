// Package coreboundary checks the source and dependency boundaries of the core.
// It lives outside the core so the checker cannot weaken the boundary it checks.
package coreboundary

import (
	"context"
	"encoding/json"
	"fmt"
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

const core = module + "/internal/steerline"

var platformText = regexp.MustCompile(`(?i)codex|orca`)

// Report contains deterministic source diagnostics, including the full path
// through a helper when a forbidden dependency is transitive.
type Report struct {
	PureFiles      map[string]int `json:"pure_files"`
	ConsoleFiles   int            `json:"console_files"`
	CoreFiles      int            `json:"core_files"`
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
	case module + "/internal/steerline/taskview":
		return dependency == module+"/internal/task/model" || dependency == module+"/internal/execution/model"
	case module + "/internal/task":
		return dependency == module+"/internal/task/model"
	case module + "/internal/execution/model":
		return dependency == module+"/internal/execution/backend"
	case module + "/internal/storage/ports":
		return dependency == module+"/internal/task/model" || dependency == module+"/internal/execution/model" || dependency == module+"/internal/execution/backend" || dependency == module+"/internal/product/api"
	}
	return false
}

var purePackages = []string{module + "/internal/steerline/taskview", module + "/internal/task", module + "/internal/task/model", module + "/internal/execution/model", module + "/internal/execution/backend", module + "/internal/product/api", module + "/internal/storage/ports"}

func purePackage(pkg string) bool {
	for _, owner := range purePackages {
		if pkg == owner {
			return true
		}
	}
	return false
}

func implementation(pkg string) bool {
	for _, path := range []string{core + "/codexapp", core + "/codexworker", core + "/orcaworker", core + "/orca_remote_worker", core + "/processhost"} {
		if pkg == path || strings.HasPrefix(pkg, path+"/") {
			return true
		}
	}
	return false
}

func forbidden(pkg string) bool {
	return implementation(pkg) || pkg == module+"/internal/steerlineapp" || strings.HasPrefix(pkg, module+"/internal/steerlineapp/") || pkg == module+"/internal/steerlineintegration" || strings.HasPrefix(pkg, module+"/internal/steerlineintegration/")
}

func consoleForbidden(pkg string) bool {
	if pkg == core || pkg == "database/sql" || pkg == "modernc.org/sqlite" || strings.HasPrefix(pkg, "modernc.org/sqlite/") || forbidden(pkg) {
		return true
	}
	for _, p := range []string{core + "/sqlite", core + "/persistence", core + "/query", core + "/projection", core + "/workspace", core + "/publication", module + "/internal/steerlineclient"} {
		if pkg == p || strings.HasPrefix(pkg, p+"/") {
			return true
		}
	}
	return false
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
		if pkg == core+"/console" {
			report.ConsoleFiles++
		}
		if purePackage(pkg) {
			report.PureFiles[pkg]++
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
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
			graph[pkg] = append(graph[pkg], value)
			if purePackage(pkg) && !PureImportsAllowed(pkg, value) {
				report.Violations = append(report.Violations, "pure ownership dependency: "+filepath.ToSlash(path)+" -> "+value)
			}
			if !strings.HasSuffix(entry.Name(), "_test.go") {
				productionGraph[pkg] = append(productionGraph[pkg], value)
			}
			if pkg == core && !strings.HasSuffix(entry.Name(), "_test.go") && (value == "database/sql" || value == core+"/sqlite" || strings.HasPrefix(value, "modernc.org/sqlite")) {
				report.Violations = append(report.Violations, "concrete storage dependency: "+filepath.ToSlash(path)+" -> "+value)
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
	visitGraph(productionGraph, core, func(path []string) {
		last := path[len(path)-1]
		if last == "database/sql" || last == core+"/sqlite" || strings.HasPrefix(last, core+"/sqlite/") || strings.HasPrefix(last, "modernc.org/sqlite") {
			report.Violations = append(report.Violations, "concrete storage dependency: "+strings.Join(path, " -> "))
		}
	})
	for _, seed := range []string{core + "/sqlite", core + "/persistence"} {
		visitGraph(graph, seed, func(path []string) {
			last := path[len(path)-1]
			if last == core || forbidden(last) || last == module+"/internal/steerlineclient" || strings.HasPrefix(last, module+"/internal/steerlineclient/") || last == core+"/console" || strings.HasPrefix(last, core+"/console/") {
				report.Violations = append(report.Violations, "reverse storage dependency: "+strings.Join(path, " -> "))
			}
		})
	}
	for _, seed := range append([]string{core, core + "/contracts"}, purePackages...) {
		visitGraph(graph, seed, func(path []string) {
			if forbidden(path[len(path)-1]) {
				report.Violations = append(report.Violations, "forbidden dependency: "+strings.Join(path, " -> "))
			}
		})
	}
	visitGraph(graph, core+"/console", func(path []string) {
		if consoleForbidden(path[len(path)-1]) {
			report.Violations = append(report.Violations, "console forbidden dependency: "+strings.Join(path, " -> "))
		}
	})
	for pkg := range graph {
		if !implementation(pkg) {
			continue
		}
		visitGraph(graph, pkg, func(path []string) {
			if path[len(path)-1] == core {
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

// Check adds Windows and Linux production graphs to the all-tags source scan.
// An unavailable go list is an error, never a skipped or passing boundary.
func Check(ctx context.Context, root string) (Report, error) {
	report, err := ScanSource(root)
	if err != nil {
		return report, err
	}

	for _, target := range []string{"windows", "linux"} {
		for _, consoleGraph := range []bool{false, true} {

			cmd := exec.CommandContext(ctx, "go", "list", "-deps", "-json", "./internal/steerline", "./internal/steerline/contracts", "./internal/task/model", "./internal/execution/model", "./internal/execution/backend", "./internal/product/api", "./internal/storage/ports")
			if consoleGraph {
				cmd = exec.CommandContext(ctx, "go", "list", "-deps", "-json", "./internal/steerline/console")
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
				if purePackage(pkg.ImportPath) {
					for _, dep := range pkg.Imports {
						if !PureImportsAllowed(pkg.ImportPath, dep) {
							report.Violations = append(report.Violations, graphName+" pure ownership dependency: "+pkg.ImportPath+" -> "+dep)
						}
					}
					closures[pkg.ImportPath] = pkg.Deps
				}
				bad := forbidden(pkg.ImportPath) || pkg.ImportPath == core+"/sqlite" || pkg.ImportPath == "database/sql" || strings.HasPrefix(pkg.ImportPath, "modernc.org/sqlite")
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
	}

	sort.Strings(report.Violations)
	return report, nil
}

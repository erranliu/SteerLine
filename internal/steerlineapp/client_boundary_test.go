package steerlineapp

import (
	"encoding/json"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestCM71ProductDependencyBoundary(t *testing.T) {
	const base = "github.com/erranliu/SteerLine/internal/steerline"
	// Parse all source files, independent of host or build tags.
	for _, dir := range []string{"../steerline/clientcontracts", "../steerlineclient", "../product/api"} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, entry.Name()), nil, parser.ImportsOnly)
			if err != nil {
				t.Fatal(err)
			}
			for _, spec := range file.Imports {
				path, err := strconv.Unquote(spec.Path.Value)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(path, "/steerlineapp") || strings.Contains(path, "/sqlite") || strings.Contains(path, "/console") {
					t.Fatalf("%s depends on concrete construction: %s", entry.Name(), path)
				}
				if strings.HasSuffix(dir, "api") && strings.Contains(path, ".") {
					t.Fatalf("product/api imports non-scalar owner %s", path)
				}
				if strings.HasSuffix(dir, "clientcontracts") && strings.HasPrefix(path, "github.com/erranliu/SteerLine/") {
					allowed := false
					for _, owner := range []string{"contracts", "task", "taskview"} {
						if path == base+"/"+owner {
							allowed = true
						}
					}
					for _, owner := range []string{"execution/backend", "execution/model", "product/api", "storage/ports", "task/model"} {
						if path == "github.com/erranliu/SteerLine/internal/"+owner {
							allowed = true
						}
					}
					if !allowed {
						t.Fatalf("pure contract %s imports %s", entry.Name(), path)
					}
				}
			}
		}
	}
	output, err := exec.Command("go", "list", "-deps", "-json", "../steerline/clientcontracts").Output()
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(output)))
	count := 0
	for decoder.More() {
		var pkg struct{ ImportPath string }
		if err := decoder.Decode(&pkg); err != nil {
			t.Fatal(err)
		}
		count++
		path := pkg.ImportPath
		if path == base || path == "database/sql" || strings.Contains(path, "/sqlite") || strings.Contains(path, "/steerlineapp") || strings.Contains(path, "/steerlineclient") || strings.Contains(path, "/codex") || strings.Contains(path, "/orca") || strings.Contains(path, "/execution/process") || strings.Contains(path, "/execution/publication") || strings.Contains(path, "/execution/workspace") {
			t.Fatalf("pure product dependency reached %s", path)
		}
	}
	if count == 0 {
		t.Fatal("dependency guard matched no packages")
	}
}

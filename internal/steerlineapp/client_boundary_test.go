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
	const module = "github.com/erranliu/SteerLine"
	owners := []struct {
		dir   string
		allow []string
	}{
		{"../product/api", nil},
		{"../product/client", []string{module + "/internal/product/api"}},
		{"../console", []string{module + "/internal/product/api"}},
	}
	for _, owner := range owners {
		entries, err := os.ReadDir(owner.dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(owner.dir, entry.Name()), nil, parser.ImportsOnly)
			if err != nil {
				t.Fatal(err)
			}
			for _, spec := range file.Imports {
				path, err := strconv.Unquote(spec.Path.Value)
				if err != nil {
					t.Fatal(err)
				}
				if strings.HasPrefix(path, module+"/internal/") && !contains(owner.allow, path) {
					t.Fatalf("%s depends on an unowned internal package: %s", entry.Name(), path)
				}
			}
		}
	}

	for _, owner := range []string{"../product/api", "../product/client", "../console"} {
		output, err := exec.Command("go", "list", "-deps", "-json", owner).Output()
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
			if path == "database/sql" || strings.HasPrefix(path, "modernc.org/sqlite") || strings.HasPrefix(path, module+"/internal/steerline/") || path == module+"/internal/steerline" {
				t.Fatalf("product-owned package %s reached concrete storage or the old center: %s", owner, path)
			}
		}
		if count == 0 {
			t.Fatalf("dependency guard matched no packages for %s", owner)
		}
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

package ports

import (
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestTM01StoragePortDependencies(t *testing.T) {
	files, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, file := range files {
		if !strings.HasSuffix(file.Name(), ".go") || strings.HasSuffix(file.Name(), "_test.go") {
			continue
		}
		count++
		parsed, err := parser.ParseFile(token.NewFileSet(), file.Name(), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range parsed.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			allowed := false
			for _, owner := range []string{"task/model", "execution/model", "execution/backend", "product/api"} {
				if path == "github.com/erranliu/SteerLine/internal/"+owner {
					allowed = true
				}
			}
			if path == "database/sql" || strings.Contains(path, ".") && !allowed {
				t.Errorf("port depends on non-value owner: %s", path)
			}

		}
	}
	if count == 0 {
		t.Fatal("no production write contracts scanned")
	}
}

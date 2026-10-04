package persistence

import (
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestCM69PurePortDependencies(t *testing.T) {
	files, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	const base = "github.com/erranliu/SteerLine/internal/steerline/"
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
			// CM70's application-owned Connection composes the existing query
			// read contracts. Keep that exception out of the narrow write ports.
			connectionReadPort := file.Name() == "connections.go" && path == base+"query"
			if path == "database/sql" || strings.Contains(path, ".") && path != base+"contracts" && path != "github.com/erranliu/SteerLine/internal/execution/backend" && path != "github.com/erranliu/SteerLine/internal/execution/model" && path != "github.com/erranliu/SteerLine/internal/storage/ports" && path != "github.com/erranliu/SteerLine/internal/task/model" && path != base+"task" && !connectionReadPort {
				t.Errorf("port depends on non-value owner: %s", path)
			}
		}
	}
	if count == 0 {
		t.Fatal("no production write contracts scanned")
	}
}

package app

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"strings"
	"testing"
)

func TestTUIHTTPBoundary(t *testing.T) {
	for _, owner := range []string{"../consoleapp", "../product/httpclient", "../product/httpcontract", "../product/httpfs"} {
		output, err := exec.Command("go", "list", "-deps", "-json", owner).Output()
		if err != nil {
			t.Fatalf("%s: %v", owner, err)
		}
		decoder := json.NewDecoder(strings.NewReader(string(output)))
		count := 0
		for decoder.More() {
			var pkg struct{ ImportPath string }
			if err = decoder.Decode(&pkg); err != nil {
				t.Fatal(err)
			}
			count++
			name := pkg.ImportPath
			if name == "database/sql" || strings.HasPrefix(name, "modernc.org/sqlite") || strings.HasPrefix(name, "github.com/erranliu/SteerLine/internal/") && !(strings.HasPrefix(name, "github.com/erranliu/SteerLine/internal/console") || strings.HasPrefix(name, "github.com/erranliu/SteerLine/internal/product/api") || strings.HasPrefix(name, "github.com/erranliu/SteerLine/internal/product/httpclient") || strings.HasPrefix(name, "github.com/erranliu/SteerLine/internal/product/httpcontract") || strings.HasPrefix(name, "github.com/erranliu/SteerLine/internal/product/httpfs")) {
				t.Fatalf("HTTP consumer reaches backend owner: %s", name)
			}
		}
		if count == 0 {
			t.Fatal("empty dependency closure")
		}
	}
	file, err := parser.ParseFile(token.NewFileSet(), "console.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "PrepareConsole" {
			continue
		}
		found = true
		calls := 0
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if ok {
				owner, ok := selector.X.(*ast.Ident)
				if !ok || owner.Name != "consoleapp" || selector.Sel.Name != "Prepare" {
					t.Fatal("console entry no longer delegates to isolated HTTP assembly")
				}
				calls++
			}
			return true
		})
		if calls != 1 {
			t.Fatalf("assembly calls %d", calls)
		}
	}
	if !found {
		t.Fatal("actual console assembly not checked")
	}
}

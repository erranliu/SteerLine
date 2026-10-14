package httpclient

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestHTTPClientCapabilityBoundary(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("test source location unavailable")
	}
	root := filepath.Dir(source)
	fset := token.NewFileSet()
	console, err := parser.ParseFile(fset, filepath.Join(root, "console.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := parser.ParseFile(fset, filepath.Join(root, "request_journal.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	fields := map[string]bool{}
	for _, declaration := range console.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.TYPE {
			continue
		}
		for _, spec := range general.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec.Name.Name != "Console" {
				continue
			}
			structure, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				t.Fatal("Console is not a struct")
			}
			for _, field := range structure.Fields.List {
				for _, name := range field.Names {
					fields[name.Name] = true
				}
			}
		}
	}
	if !fields["journal"] || !fields["observer"] {
		t.Fatal("Console must depend on explicit journal and stop observer capabilities")
	}

	fileOperations := map[string]bool{"ReadFile": true, "WriteFile": true, "OpenFile": true, "Rename": true, "Link": true, "Remove": true}
	for _, declaration := range console.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Body == nil {
			continue
		}
		ast.Inspect(function.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if selector.Sel.Name == "observeExit" {
				t.Errorf("Console.%s directly calls the platform observer", function.Name.Name)
			}
			owner, isIdentifier := selector.X.(*ast.Ident)
			if isIdentifier && owner.Name == "os" && fileOperations[selector.Sel.Name] {
				t.Errorf("Console.%s directly performs journal file IO through os.%s", function.Name.Name, selector.Sel.Name)
			}
			return true
		})
	}

	forbiddenImports := []string{"internal/app", "internal/execution", "internal/storage", "internal/product/httpapi", "internal/product/query"}
	var output bytes.Buffer
	command := exec.Command("go", "list", "-deps", "-f={{.ImportPath}}", ".", "../../consoleapp")
	command.Dir = root
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Run(); err != nil {
		t.Fatalf("inspect HTTP consumer dependency closure: %v\n%s", err, output.String())
	}
	for _, imported := range strings.Fields(output.String()) {
		for _, forbidden := range forbiddenImports {
			if imported == forbidden || strings.Contains(imported, "/"+forbidden) || strings.HasSuffix(imported, forbidden) {
				t.Errorf("HTTP client/consoleapp closure reaches forbidden capability %q", imported)
			}
		}
	}

	journalImportsFiles := false
	for _, imported := range journal.Imports {
		if imported.Path != nil && imported.Path.Value == `"os"` {
			journalImportsFiles = true
		}
	}
	if !journalImportsFiles {
		t.Fatal("requestJournal must own client journal file access")
	}
}

package app

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
)

func TestProjectLifecycleOwnershipBoundary(t *testing.T) {
	retired := map[string]bool{"StartService": true, "StartConfiguredService": true, "RunConfiguredService": true, "runConfiguredService": true, "ServiceLifetime": true, "ServiceFactory": true, "serviceLocalAPI": true, "executionServiceLifetime": true, "newRunnerServiceWithTaskStore": true, "findStopRequestCached": true}
	for _, dir := range []string{".", "host"} {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		require(t, err)
		for _, name := range files {
			if len(name) >= 8 && name[len(name)-8:] == "_test.go" {
				continue
			}
			file, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
			require(t, err)
			for _, decl := range file.Decls {
				switch node := decl.(type) {
				case *ast.FuncDecl:
					if retired[node.Name.Name] {
						t.Error("retired lifecycle implementation", name, node.Name.Name)
					}
					if node.Name.Name == "newRunnerService" || node.Name.Name == "Close" && name == "application_directory.go" || node.Name.Name == "VerifyApplicationStopped" {
						ast.Inspect(node.Body, func(n ast.Node) bool {
							call, ok := n.(*ast.CallExpr)
							if !ok {
								return true
							}
							if selector, ok := call.Fun.(*ast.SelectorExpr); ok {
								if node.Name.Name == "newRunnerService" && (selector.Sel.Name == "Close" || selector.Sel.Name == "StartService") {
									t.Error("factory owns resource", name, selector.Sel.Name)
								}
								if node.Name.Name == "Close" && selector.Sel.Name == "Start" {
									t.Error("Close starts directory")
								}
							}
							if id, ok := call.Fun.(*ast.Ident); ok && node.Name.Name == "VerifyApplicationStopped" && id.Name == "NewApplicationDirectory" {
								t.Error("offline verification creates active directory")
							}
							return true
						})
					}
				case *ast.GenDecl:
					for _, spec := range node.Specs {
						if typ, ok := spec.(*ast.TypeSpec); ok && retired[typ.Name.Name] {
							t.Error("retired owner type", name, typ.Name.Name)
						}
					}
				}
			}
		}
	}
	actor, err := parser.ParseFile(token.NewFileSet(), "../taskrunner/actor.go", nil, 0)
	require(t, err)
	for _, decl := range actor.Decls {
		if method, ok := decl.(*ast.FuncDecl); ok && method.Recv != nil && (method.Name.Name == "Close" || method.Name.Name == "shutdown") {
			t.Error("actor still owns resources", method.Name.Name)
		}
	}
}

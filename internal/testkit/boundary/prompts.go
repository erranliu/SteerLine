package boundary

import (
	"go/ast"
	"go/token"
	"strings"
)

// These are protocol adaptation points, not a catalogue of factual producers.
// New sinks require a deliberate source-rule and consumer-test change.
func promptSinkAllowed(file, function string) bool {
	switch file + ":" + function {
	case "internal/testmanagement/generation.go:Generate",
		"internal/roadmap/generation.go:Parse",
		"internal/roadmap/documents.go:GenerateDocuments",
		"internal/app/environment_adaptation.go:prepareGeneration",
		"internal/environmentadaptation/service.go:appendRound",
		"internal/runnerclient/prompt_context.go:WithTestProgressContext":
		return true
	}
	return false
}

func promptField(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		return v.Sel.Name
	case *ast.IndexExpr:
		return promptField(v.X)
	}
	return ""
}

// Inspect direct syntax and one local initializer only. No call graph, taint
// tracking or natural-language filtering: indirect facts remain review's job.
func directPromptText(e ast.Expr, locals map[string]ast.Expr) bool {
	if id, ok := e.(*ast.Ident); ok {
		if init := locals[id.Name]; init != nil {
			e = init
		}
	}
	switch v := e.(type) {
	case *ast.BasicLit:
		return v.Kind == token.STRING
	case *ast.BinaryExpr:
		return true
	case *ast.CallExpr:
		if sel, ok := v.Fun.(*ast.SelectorExpr); ok {
			if owner, ok := sel.X.(*ast.Ident); ok {
				return owner.Name == "strings" || owner.Name == "fmt"
			}
		}
	case *ast.CompositeLit:
		for _, elt := range v.Elts {
			if kv, ok := elt.(*ast.KeyValueExpr); ok && directPromptText(kv.Value, locals) {
				return true
			}
		}
	}
	return false
}

func checkPromptSource(report *Report, file string, parsed *ast.File) {
	// Test fixtures are not product send paths; arbitrary new helper directories
	// participate, including inactive platform/build-tag source files.
	if strings.HasPrefix(file, "internal/testkit/") || strings.HasPrefix(file, "tests/") {
		return
	}
	violate := func(reason string) {
		report.Violations = append(report.Violations, "prompt boundary: "+file+": "+reason)
	}
	for _, group := range parsed.Comments {
		for _, c := range group.List {
			if strings.HasPrefix(c.Text, "//go:embed ") && !strings.HasPrefix(file, "internal/prompts/") && (strings.Contains(c.Text, ".md") || strings.Contains(c.Text, "prompt")) {
				violate("fixed prompt templates must be embedded by internal/prompts")
			}
		}
	}
	for _, decl := range parsed.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			ast.Inspect(decl, func(n ast.Node) bool {
				if kv, ok := n.(*ast.KeyValueExpr); ok && (promptField(kv.Key) == "Instruction" || promptField(kv.Key) == "PromptOverrides") {
					violate("unregistered package-level instruction write")
				}
				return true
			})
			continue
		}
		if fn.Body == nil {
			continue
		}
		locals := map[string]ast.Expr{}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if a, ok := n.(*ast.AssignStmt); ok {
				for i, lhs := range a.Lhs {
					if id, ok := lhs.(*ast.Ident); ok && i < len(a.Rhs) {
						locals[id.Name] = a.Rhs[i]
					}
				}
			}
			if v, ok := n.(*ast.ValueSpec); ok {
				for i, name := range v.Names {
					if i < len(v.Values) {
						locals[name.Name] = v.Values[i]
					}
				}
			}
			return true
		})
		check := func(field string, value ast.Expr) {
			if field != "Instruction" && field != "PromptOverrides" {
				return
			}
			if !promptSinkAllowed(file, fn.Name.Name) {
				violate("unregistered instruction write in " + fn.Name.Name)
				return
			}
			if directPromptText(value, locals) {
				violate("inline or transformed fixed instruction in " + fn.Name.Name)
			}
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			switch v := n.(type) {
			case *ast.KeyValueExpr:
				check(promptField(v.Key), v.Value)
			case *ast.AssignStmt:
				for i, lhs := range v.Lhs {
					if i < len(v.Rhs) {
						check(promptField(lhs), v.Rhs[i])
					}
				}
			}
			return true
		})
	}
}

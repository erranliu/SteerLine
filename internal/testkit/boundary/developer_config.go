package boundary

import (
	"go/ast"
	"strconv"
	"strings"
)

// Keep raw options in their definitions, configuration validation/editing and
// the application capability mapping. Business consumers use domain policies.
func developerConfigOwner(file string) bool {
	if strings.HasPrefix(file, "internal/developerconfig/") {
		return true
	}
	switch file {
	case "internal/executionconfig/config.go", "internal/app/runner_config.go",
		"internal/app/capabilities.go", "web/server/executionconfig/store.go":
		return true
	}
	return false
}

func checkDeveloperConfigSource(report *Report, file string, parsed *ast.File) {
	if developerConfigOwner(file) {
		return
	}
	for _, spec := range parsed.Imports {
		dependency, _ := strconv.Unquote(spec.Path.Value)
		if dependency == module+"/internal/developerconfig" || strings.HasPrefix(dependency, module+"/internal/developerconfig/") {
			report.Violations = append(report.Violations, "developer configuration boundary: "+file+" -> "+dependency)
		}
	}
	// Imports alone miss access through the already composed app Config or a
	// shared configuration DTO, including access moved into a helper package.
	ast.Inspect(parsed, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if ok && (selector.Sel.Name == "Developer" || selector.Sel.Name == "SkipEnvironmentReadiness") {
			report.Violations = append(report.Violations, "developer configuration boundary: "+file+" -> "+selector.Sel.Name)
		}
		return true
	})
}

package boundary

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"
)

// Configuration construction and admission consume opaque Runner values.
// Frozen historical display and trusted deployment/test assembly retain their
// existing owners and are deliberately outside this editing/admission boundary.
func checkNativeConfigurationSource(report *Report, path string, file *ast.File) {
	scoped := strings.HasPrefix(path, "internal/app/") || strings.HasPrefix(path, "internal/environmentadaptation/") || strings.HasPrefix(path, "internal/storage/sqlite/environment") || path == "internal/runnerclient/content.go" || strings.HasPrefix(path, "web/server/executionconfig/") || strings.HasPrefix(path, "web/server/discovery/")
	if !scoped {
		return
	}
	ast.Inspect(file, func(node ast.Node) bool {
		if literal, ok := node.(*ast.BasicLit); ok && literal.Kind == token.STRING {
			value, err := strconv.Unquote(literal.Value)
			if err == nil && (value == "agent_launch" || value == "runtime_executable" || value == "runtime_sha256" || value == "transport" || strings.Contains(value, `json:"agent_launch`) || strings.Contains(value, `json:"runtime_executable`)) {
				report.Violations = append(report.Violations, "native configuration mapping outside Runner: "+path)
			}
		}
		if identifier, ok := node.(*ast.Ident); ok {
			switch identifier.Name {
			case "GenerationExecution", "useNativeOrcaExecutor", "nativePrograms", "programsUnchanged", "validHelper":
				report.Violations = append(report.Violations, "retired native configuration mapping: "+path+" -> "+identifier.Name)
			}
		}
		return true
	})
}

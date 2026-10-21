package boundary

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestWebExecutionConfigOwnership(t *testing.T) {
	for _, tc := range []struct {
		name, path, dep string
		allowed         bool
	}{
		{"pure_web_values", "web/server/executionconfig/edge_plan9.go", "internal/executionconfig", true},
		{"no_app_in_query", "web/server/discovery/edge_plan9.go", "internal/app", false},
		{"no_runner_private", "web/server/discovery/edge_windows.go", "github.com/erranliu/SteerRunner/internal/execution/adapters/protocol/orca", false},
		{"no_runner_client", "web/server/executionconfig/edge.go", "internal/runnerclient", false},
		{"no_SQL_in_query", "web/server/discovery/edge.go", "database/sql", false},
		{"no_reverse_query", "internal/app/edge_plan9.go", "web/server/discovery", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			dep := tc.dep
			if strings.HasPrefix(dep, "internal/") || strings.HasPrefix(dep, "web/") {
				dep = module + "/" + dep
			}
			writeCallerSource(t, root, tc.path, "package fixture\nimport _ \""+dep+"\"\n")
			if tc.dep == "internal/executionconfig" {
				writeCallerSource(t, root, filepath.Join(tc.dep, "value.go"), "package fixture\n")
			}
			report, err := ScanSource(root)
			if err != nil {
				t.Fatal(err)
			}
			if (len(report.Violations) == 0) != tc.allowed {
				t.Fatalf("allowed=%v: %v", tc.allowed, report.Violations)
			}
		})
	}
}

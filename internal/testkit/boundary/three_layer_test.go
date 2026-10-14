package boundary

import (
	"os"
	"path/filepath"
	"testing"
)

func TestThreeLayerDependencyBoundary(t *testing.T) {
	for _, tc := range []struct {
		owner, edge string
		allowed     bool
	}{
		{"workflow", "reliable", true}, {"workflow", "backend", false}, {"workflow", "adapters/codex", false}, {"workflow", "model", false},
		{"reliable", "backend", true}, {"reliable", "workflow", false}, {"reliable", "model", false}, {"reliable", "reconstruction", false}, {"reliable", "", false},
		{"adapters/fake", "reliable", false}, {"adapters/fake", "workflow", false},
	} {
		t.Run(tc.owner+"_"+tc.edge, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "internal/execution", tc.owner, "edge.go")
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			edge := execution
			if tc.edge != "" {
				edge += "/" + tc.edge
			}
			if err := os.WriteFile(path, []byte("package fixture\nimport _ \""+edge+"\"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			report, err := ScanSource(root)
			if err != nil {
				t.Fatal(err)
			}
			if (len(report.Violations) == 0) != tc.allowed {
				t.Fatalf("allowed=%v violations=%v", tc.allowed, report.Violations)
			}
		})
	}
	// SQL remains forbidden even though database/sql is a standard package.
	rootSQL := t.TempDir()
	pathSQL := filepath.Join(rootSQL, "internal/execution/reliable/sql.go")
	if err := os.MkdirAll(filepath.Dir(pathSQL), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pathSQL, []byte("package reliable\nimport _ \"database/sql\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	sqlReport, err := ScanSource(rootSQL)
	if err != nil || len(sqlReport.Violations) == 0 {
		t.Fatal("SQL entered reliability owner")
	}
	// An unrelated helper cannot hide the dependency, including inactive source.
	root := t.TempDir()
	for _, file := range []struct{ path, source string }{
		{"internal/execution/reliable/edge_plan9.go", "//go:build never\n\npackage reliable\nimport _ \"" + module + "/internal/helper\"\n"},
		{"internal/helper/edge.go", "package helper\nimport _ \"" + execution + "/workflow\"\n"},
	} {
		path := filepath.Join(root, file.path)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(file.source), 0600); err != nil {
			t.Fatal(err)
		}
	}
	report, err := ScanSource(root)
	if err != nil || len(report.Violations) == 0 {
		t.Fatalf("hidden edge was allowed: %v %+v", err, report)
	}
}
func TestThreeLayerRepositoryBoundary(t *testing.T) {
	report := checkRepositoryBoundary(t)
	if len(report.Violations) != 0 {
		t.Fatal(report.Violations)
	}
}

package coreboundary

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestTM04ExecutionBoundary(t *testing.T) {
	for _, fixture := range []struct{ name, path, source, helper string }{
		{"task_implementation_in_inactive_tag", "edge_plan9.go", "//go:build never_enabled\n\npackage execution\nimport _ \"" + module + "/internal/task\"\n", ""},
		{"old_host_in_other_os", "edge_freebsd.go", "package execution\nimport _ \"" + core + "\"\n", ""},
		{"concrete_storage_in_helper", "edge.go", "package execution\nimport _ \"" + module + "/internal/helper\"\n", "package helper\nimport _ \"" + core + "/sqlite\"\n"},
		{"adapter_in_other_os_test", "edge_linux_test.go", "package execution\nimport _ \"" + core + "/codexworker\"\n", ""},
		{"product_query_in_execution", "query_edge.go", "package execution\nimport _ \"" + core + "/query\"\n", ""},
		{"old_projection_in_execution", "projection_edge.go", "package execution\nimport _ \"" + core + "/projection\"\n", ""},
		{"adapter_in_pure_reconstruction", "pure_edge.go", "package reconstruction\nimport _ \"" + core + "/codexapp\"\n", ""},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			root := t.TempDir()
			write := func(path, source string) {
				t.Helper()
				path = filepath.Join(root, path)
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(source), 0600); err != nil {
					t.Fatal(err)
				}
			}
			write(filepath.Join("internal/execution", fixture.path), fixture.source)
			if fixture.helper != "" {
				write("internal/helper/edge_darwin.go", fixture.helper)
			}
			report, err := ScanSource(root)
			if err != nil {
				t.Fatal(err)
			}
			if len(report.Violations) == 0 {
				t.Fatalf("forbidden execution fixture escaped: %+v", report)
			}
		})
	}
	t.Run("repository_all_tags_windows_linux", func(t *testing.T) {
		_, file, _, ok := runtime.Caller(0)
		if !ok {
			t.Fatal("source location unavailable")
		}
		report, err := Check(context.Background(), filepath.Clean(filepath.Join(filepath.Dir(file), "../..")))
		if err != nil {
			t.Fatal(err)
		}
		if report.ExecutionFiles == 0 || report.Dependencies["windows"] == 0 || report.Dependencies["linux"] == 0 {
			t.Fatalf("empty execution boundary coverage: %+v", report)
		}
		if len(report.Violations) != 0 {
			t.Fatal(strings.Join(report.Violations, "\n"))
		}
	})
}

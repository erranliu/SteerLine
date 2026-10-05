package steerlineintegration

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/coreboundary"
)

func TestCD67CoreBoundary(t *testing.T) {
	t.Run("pure_ownership_all_tags_production_and_tests", func(t *testing.T) {
		for _, owner := range []string{"product/query/taskgraph", "task", "task/model", "execution/model", "execution/backend", "product/api", "storage/ports"} {
			t.Run(owner, func(t *testing.T) {
				for _, dependency := range []string{"database/sql", "github.com/erranliu/SteerLine/internal/steerline/contracts", "github.com/erranliu/SteerLine/internal/product/query", "github.com/erranliu/SteerLine/internal/steerline/sqlite", "github.com/erranliu/SteerLine/internal/execution/adapters/codex", "github.com/erranliu/SteerLine/internal/steerlineapp", "github.com/erranliu/SteerLine/internal/helper", "example.org/implementation"} {
					for _, filename := range []string{"edge_plan9.go", "edge_linux_test.go"} {
						root := t.TempDir()
						dir := filepath.Join(root, "internal", owner)
						require(t, os.MkdirAll(dir, 0700))
						require(t, os.WriteFile(filepath.Join(dir, filename), []byte("//go:build never_enabled\n\npackage fixture\nimport _ \""+dependency+"\"\n"), 0600))
						report, err := coreboundary.ScanSource(root)
						require(t, err)
						if !strings.Contains(strings.Join(report.Violations, "\n"), "pure ownership dependency") {
							t.Fatalf("%s fixture escaped %s: %+v", owner, dependency, report)
						}
					}
				}
			})
		}
		// A model importing another pure domain is still forbidden.
		for _, edge := range [][2]string{{"task", "storage/ports"}, {"task", "execution/model"}, {"task/model", "execution/backend"}, {"execution/model", "task/model"}, {"execution/backend", "execution/model"}, {"product/api", "execution/backend"}, {"storage/ports", "steerline/persistence"}} {
			root := t.TempDir()
			dir := filepath.Join(root, "internal", edge[0])
			require(t, os.MkdirAll(dir, 0700))
			require(t, os.WriteFile(filepath.Join(dir, "edge_test.go"), []byte("package fixture\nimport _ \"github.com/erranliu/SteerLine/internal/"+edge[1]+"\"\n"), 0600))
			report, err := coreboundary.ScanSource(root)
			require(t, err)
			if len(report.Violations) == 0 {
				t.Fatalf("cross-domain pure edge escaped: %v", edge)
			}
		}
	})
	t.Run("negative_source_fixtures", func(t *testing.T) {
		for _, fixture := range []struct {
			name, filename, source, helper, helperPath, implementation string
			want                                                       string
		}{
			{name: "mixed_case_text_in_other_os_test", filename: "neutral_linux_test.go", source: "//go:build linux\n\npackage steerline\n// OrCa forbidden\n", want: "platform text"},
			{name: "mixed_case_filename", filename: "CoDeX_windows_test.go", source: "package steerline\n", want: "platform text"},
			{name: "direct_import_behind_tag", filename: "neutral_plan9.go", source: "//go:build plan9\n\npackage steerline\nimport _ \"github.com/erranliu/SteerLine/internal/steerlineapp\"\n", want: "forbidden dependency"},
			{name: "transitive_helper_edge", filename: "neutral.go", source: "package steerline\nimport _ \"github.com/erranliu/SteerLine/internal/helper\"\n", helper: "package helper\nimport _ \"github.com/erranliu/SteerLine/internal/steerlineintegration\"\n", want: "forbidden dependency"},
			{name: "nested_generated_name_is_not_an_exclusion", filename: "neutral.go", source: "package steerline\nimport _ \"github.com/erranliu/SteerLine/internal/helper/bin\"\n", helperPath: "internal/helper/bin/edge_freebsd.go", helper: "package bin\nimport _ \"github.com/erranliu/SteerLine/internal/steerlineapp\"\n", want: "forbidden dependency"},
			{name: "top_level_output_name_is_not_an_exclusion", filename: "neutral.go", source: "package steerline\nimport _ \"github.com/erranliu/SteerLine/bin/helper\"\n", helperPath: "bin/helper/edge_freebsd.go", helper: "package helper\nimport _ \"github.com/erranliu/SteerLine/internal/steerlineapp\"\n", want: "forbidden dependency"},
			{name: "reverse_implementation_edge", filename: "neutral.go", source: "package steerline\n", implementation: "package codexworker\nimport _ \"github.com/erranliu/SteerLine/internal/helper\"\n", helper: "package helper\nimport _ \"github.com/erranliu/SteerLine/internal/steerline\"\n", want: "reverse dependency"},
			{name: "reverse_storage_edge", filename: "neutral.go", source: "package steerline\n", helperPath: "internal/steerline/sqlite/edge_freebsd.go", helper: "package sqlite\nimport _ \"github.com/erranliu/SteerLine/internal/steerline\"\n", want: "reverse storage dependency"},
			{name: "concrete_storage_edge", filename: "neutral_freebsd.go", source: "package steerline\nimport _ \"database/sql\"\n", want: "concrete storage dependency"},
			{name: "transitive_storage_edge", filename: "neutral_plan9.go", source: "//go:build never_enabled\n\npackage steerline\nimport _ \"github.com/erranliu/SteerLine/internal/helper\"\n", helper: "//go:build never_enabled\n\npackage helper\nimport _ \"github.com/erranliu/SteerLine/internal/steerline/sqlite\"\n", want: "concrete storage dependency"},
			{name: "reverse_client_subpackage", filename: "sqlite/neutral_plan9.go", source: "//go:build never_enabled\n\npackage sqlite\nimport _ \"github.com/erranliu/SteerLine/internal/steerlineclient/nested\"\n", want: "reverse storage dependency"},
		} {
			t.Run(fixture.name, func(t *testing.T) {
				root := t.TempDir()
				write := func(path, source string) {
					t.Helper()
					require(t, os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0700))
					require(t, os.WriteFile(filepath.Join(root, path), []byte(source), 0600))
				}
				write("internal/steerline/"+fixture.filename, fixture.source)
				if fixture.helper != "" {
					path := fixture.helperPath
					if path == "" {
						path = "internal/helper/edge_freebsd.go"
					}
					write(path, fixture.helper)
				}
				if fixture.implementation != "" {
					write("internal/execution/adapters/codex/edge_darwin.go", fixture.implementation)
				}
				report, err := coreboundary.ScanSource(root)
				require(t, err)
				if !strings.Contains(strings.Join(report.Violations, "\n"), fixture.want) {
					t.Fatalf("fixture escaped guard: %+v", report)
				}
			})
		}
	})

	t.Run("console_negative_all_tags_production_and_tests", func(t *testing.T) {
		for _, dependency := range []string{"github.com/erranliu/SteerLine/internal/steerline", "database/sql", "modernc.org/sqlite", "github.com/erranliu/SteerLine/internal/steerline/sqlite", "github.com/erranliu/SteerLine/internal/steerline/persistence", "github.com/erranliu/SteerLine/internal/execution/adapters/codex", "github.com/erranliu/SteerLine/internal/execution/adapters/orcaremote", "github.com/erranliu/SteerLine/internal/execution/process", "github.com/erranliu/SteerLine/internal/steerlineclient", "github.com/erranliu/SteerLine/internal/product/client", "github.com/erranliu/SteerLine/internal/product/query", "github.com/erranliu/SteerLine/internal/execution/workspace", "github.com/erranliu/SteerLine/internal/execution/publication", "github.com/erranliu/SteerLine/internal/steerlineapp"} {
			t.Run(dependency, func(t *testing.T) {
				for _, file := range []string{"edge_plan9.go", "edge_linux_test.go"} {
					root := t.TempDir()
					dir := filepath.Join(root, "internal/console")
					require(t, os.MkdirAll(dir, 0700))
					require(t, os.WriteFile(filepath.Join(dir, file), []byte("//go:build never_enabled\n\npackage console\nimport _ \""+dependency+"\"\n"), 0600))
					report, err := coreboundary.ScanSource(root)
					require(t, err)
					if !strings.Contains(strings.Join(report.Violations, "\n"), "console forbidden dependency") {
						t.Fatalf("console fixture escaped: %+v", report)
					}
				}
			})
		}
		t.Run("transitive_test_helper", func(t *testing.T) {
			root := t.TempDir()
			for path, source := range map[string]string{"internal/console/edge_darwin_test.go": "package console\nimport _ \"github.com/erranliu/SteerLine/internal/helper\"\n", "internal/helper/edge_freebsd.go": "package helper\nimport _ \"github.com/erranliu/SteerLine/internal/product/client\"\n"} {
				file := filepath.Join(root, path)
				require(t, os.MkdirAll(filepath.Dir(file), 0700))
				require(t, os.WriteFile(file, []byte(source), 0600))
			}
			report, err := coreboundary.ScanSource(root)
			require(t, err)
			if !strings.Contains(strings.Join(report.Violations, "\n"), "console forbidden dependency") {
				t.Fatalf("helper fixture escaped: %+v", report)
			}
		})
	})
	t.Run("repository_all_tags_and_production_graphs", func(t *testing.T) {
		_, file, _, ok := runtime.Caller(0)
		if !ok {
			t.Fatal("missing source location")
		}
		root := filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
		report, err := coreboundary.Check(context.Background(), root)
		require(t, err)
		raw, err := json.Marshal(report)
		require(t, err)
		t.Log(string(raw))
		for _, owner := range []string{"product/query/taskgraph", "task", "task/model", "execution/model", "execution/backend", "product/api", "storage/ports"} {
			if report.PureFiles["github.com/erranliu/SteerLine/internal/"+owner] == 0 {
				t.Fatalf("empty migrated owner scan: %s", owner)
			}
		}
		if report.CoreFiles == 0 || report.ConsoleFiles == 0 || report.Dependencies["windows-console"] == 0 || report.Dependencies["linux-console"] == 0 || report.Dependencies["windows"] == 0 || report.Dependencies["linux"] == 0 {
			t.Fatal("empty boundary scan")
		}
		if len(report.Violations) != 0 {
			t.Fatalf("core boundary violations:\n%s", strings.Join(report.Violations, "\n"))
		}
	})
}

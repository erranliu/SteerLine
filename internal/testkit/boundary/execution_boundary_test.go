package boundary

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestTM04ExecutionBoundary(t *testing.T) {
	for _, fixture := range []struct{ name, path, source, helper string }{
		{"task_implementation_in_inactive_tag", "edge_plan9.go", "//go:build never_enabled\n\npackage execution\nimport _ \"" + module + "/internal/task\"\n", ""},
		{"old_host_in_other_os", "edge_freebsd.go", "package execution\nimport _ \"" + core + "\"\n", ""},
		{"concrete_storage_in_helper", "edge.go", "package execution\nimport _ \"" + module + "/internal/helper\"\n", "package helper\nimport _ \"" + module + "/internal/storage/sqlite\"\n"},
		{"adapter_in_other_os_test", "edge_linux_test.go", "package execution\nimport _ \"" + execution + "/adapters/codex\"\n", ""},
		{"product_query_in_execution", "query_edge.go", "package execution\nimport _ \"" + core + "/query\"\n", ""},
		{"old_projection_in_execution", "projection_edge.go", "package execution\nimport _ \"" + core + "/projection\"\n", ""},
		{"product_query_in_helper", "helper_query.go", "package execution\nimport _ \"" + module + "/internal/helper\"\n", "package helper\nimport _ \"" + core + "/query\"\n"},
		{"new_product_query_in_helper", "helper_product_query.go", "package execution\nimport _ \"" + module + "/internal/helper\"\n", "package helper\nimport _ \"" + module + "/internal/product/query\"\n"},
		{"adapter_in_pure_reconstruction", "reconstruction/pure_edge.go", "package reconstruction\nimport _ \"" + execution + "/adapters/protocol/codexapp\"\n", ""},
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
		report, err := Check(context.Background(), filepath.Clean(filepath.Join(filepath.Dir(file), "../../..")))
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

func TestTM08AppHostBoundary(t *testing.T) {
	t.Run("transitive_execution_workspace", func(t *testing.T) {
		graph := map[string][]string{
			core:      {execution},
			execution: {execution + "/workspace"},
		}
		var found []string
		visitHostGraph(graph, core, func(path []string) {
			if hostForbidden(path[len(path)-1]) {
				found = append(found, strings.Join(path, " -> "))
			}
		})
		want := core + " -> " + execution + " -> " + execution + "/workspace"
		for _, path := range found {
			if path == want {
				return
			}
		}
		t.Fatalf("host closure skipped its execution-to-workspace dependency: got %v, want %s", found, want)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../../.."))
	report, err := Check(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	for _, violation := range report.Violations {
		if strings.Contains(violation, "app host forbidden dependency") {
			t.Fatalf("host implementation crossed its injected boundary: %s", violation)
		}
	}
}

// Console tests and inactive-tag helpers must not regain internal query or
// execution reconstruction access when the execution package moves.
func TestTM04ConsoleQueryBoundary(t *testing.T) {
	for _, dependency := range []string{core + "/query", core + "/projection", execution + "/reconstruction"} {
		t.Run(strings.TrimPrefix(dependency, module+"/internal/"), func(t *testing.T) {
			root := t.TempDir()
			files := map[string]string{
				"internal/console/edge_linux_test.go": "package console\nimport _ \"" + module + "/internal/helper\"\n",
				"internal/helper/query_plan9.go":      "//go:build never_enabled\n\npackage helper\nimport _ \"" + dependency + "\"\n",
			}
			for name, source := range files {
				path := filepath.Join(root, name)
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(source), 0600); err != nil {
					t.Fatal(err)
				}
			}
			report, err := ScanSource(root)
			if err != nil {
				t.Fatal(err)
			}
			if len(report.Violations) == 0 {
				t.Fatalf("console helper reached internal query: %+v", report)
			}
		})
	}
}

func TestTM06AdapterBoundary(t *testing.T) {
	for _, dependency := range []string{
		module + "/internal/execution",
		module + "/internal/task",
		module + "/internal/storage/ports",
		"database/sql",
		module + "/internal/storage/sqlite",
		module + "/internal/testkit/protocol",
		module + "/internal/app",
		console,
		console,
		console + "/activityrender",
		module + "/internal/product/api",
		productQuery + "/taskgraph",
	} {
		t.Run(strings.TrimPrefix(dependency, module+"/internal/"), func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "internal", "execution", "adapters", "codex", "edge_plan9.go")
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			source := "//go:build never_enabled\n\npackage codexworker\nimport _ \"" + dependency + "\"\n"
			if err := os.WriteFile(path, []byte(source), 0600); err != nil {
				t.Fatal(err)
			}
			report, err := ScanSource(root)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(strings.Join(report.Violations, "\n"), "execution adapter forbidden dependency") {
				t.Fatalf("adapter dependency escaped guard: %s: %+v", dependency, report)
			}
		})
	}
	t.Run("transitive_helper", func(t *testing.T) {
		root := t.TempDir()
		files := map[string]string{
			"internal/execution/adapters/orca/edge.go": "package orcaworker\nimport _ \"github.com/erranliu/SteerLine/internal/helper\"\n",
			"internal/helper/edge_freebsd.go":          "//go:build never_enabled\n\npackage helper\nimport _ \"github.com/erranliu/SteerLine/internal/storage/sqlite\"\n",
		}
		for name, source := range files {
			path := filepath.Join(root, name)
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(source), 0600); err != nil {
				t.Fatal(err)
			}
		}
		report, err := ScanSource(root)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(strings.Join(report.Violations, "\n"), "execution adapter forbidden dependency") {
			t.Fatalf("adapter transitive dependency escaped guard: %+v", report)
		}
	})
	t.Run("backend_resources_and_protocol_are_allowed", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, "internal", "execution", "adapters", "codex", "edge.go")
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		source := "package codexworker\nimport (_ \"" + execution + "/backend\"; _ \"" + execution + "/model\"; _ \"" + execution + "/conversation\"; _ \"" + execution + "/workspace\"; _ \"" + execution + "/adapters/protocol/codexapp\")\n"
		if err := os.WriteFile(path, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
		report, err := ScanSource(root)
		if err != nil {
			t.Fatal(err)
		}
		for _, violation := range report.Violations {
			if strings.Contains(violation, "execution adapter forbidden dependency") {
				t.Fatalf("allowed adapter dependencies were rejected: %s", violation)
			}
		}
	})
}

// Adapter imports remain closed over execution-owned values. A helper must not
// let product DTOs or product query implementations bypass that boundary.
func TestTM06ContractValueClosure(t *testing.T) {
	for _, dependency := range []string{module + "/internal/product/api", productQuery + "/taskgraph"} {
		for _, withModel := range []bool{false, true} {
			name := strings.TrimPrefix(dependency, module+"/internal/")
			if withModel {
				name += "/alongside_execution_model_path"
			}
			t.Run(name, func(t *testing.T) {
				root := t.TempDir()
				source := "package codexworker\nimport _ \"" + module + "/internal/helper\"\n"
				files := map[string]string{
					"internal/helper/edge.go":            "package helper\nimport _ \"" + module + "/internal/otherhelper\"\n",
					"internal/otherhelper/edge_plan9.go": "//go:build never_enabled\n\npackage otherhelper\nimport _ \"" + dependency + "\"\n",
				}
				if withModel {
					source += "import _ \"" + execution + "/model\"\n"
					files["internal/execution/model/edge.go"] = "package model\nimport _ \"" + dependency + "\"\n"
				}
				files["internal/execution/adapters/codex/edge.go"] = source
				for name, source := range files {
					path := filepath.Join(root, name)
					if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, []byte(source), 0600); err != nil {
						t.Fatal(err)
					}
				}
				report, err := ScanSource(root)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(strings.Join(report.Violations, "\n"), "execution adapter forbidden dependency") {
					t.Fatalf("unrelated helper escaped contract-value boundary: %+v", report)
				}
				// Exercise the actual Windows/Linux go-list graphs as well as
				// the inactive-tag scan, including competing legal/illegal paths.
				if err := os.Remove(filepath.Join(root, "internal/otherhelper/edge_plan9.go")); err != nil {
					t.Fatal(err)
				}
				files = map[string]string{
					"go.mod":                       "module " + module + "\n\ngo 1.23\n",
					"internal/otherhelper/edge.go": "package otherhelper\nimport _ \"" + dependency + "\"\n",
				}
				for _, pkg := range []string{"internal/app", "internal/app/host", "internal/execution", "internal/task/model", "internal/execution/model", "internal/execution/backend", "internal/product/api", "internal/storage/ports", "internal/product/query", "internal/product/query/taskgraph", "internal/console"} {
					path := filepath.Join(root, pkg, "empty.go")
					if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, []byte("package "+filepath.Base(pkg)+"\n"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				for name, source := range files {
					if err := os.WriteFile(filepath.Join(root, name), []byte(source), 0600); err != nil {
						t.Fatal(err)
					}
				}
				report, err = Check(context.Background(), root)
				if err != nil {
					t.Fatal(err)
				}
				for _, target := range []string{"windows", "linux"} {
					if !strings.Contains(strings.Join(report.Violations, "\n"), target+" adapter forbidden dependency") {
						t.Fatalf("%s production helper escaped boundary: %+v", target, report)
					}
				}
			})
		}
	}
}

func TestTM05ProductQueryBoundary(t *testing.T) {
	for _, dependency := range []string{execution, console, executionAdapters + "/codex", "database/sql", module + "/internal/helper"} {
		t.Run(dependency, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "internal/product/query/edge_plan9.go")
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("//go:build never_enabled\n\npackage query\nimport _ \""+dependency+"\"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			report, err := ScanSource(root)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(strings.Join(report.Violations, "\n"), "product query forbidden dependency") {
				t.Fatalf("inactive query import escaped: %+v", report)
			}
		})
	}
}

func TestTM07StorageBoundary(t *testing.T) {
	for _, fixture := range []struct {
		name, dir, source, want string
	}{
		{"adapter_dependency", "internal/storage/sqlite", "package sqlite\nimport _ \"" + module + "/internal/execution/adapters/codex\"\n", "storage sqlite forbidden dependency"},
		{"application_dependency", "internal/storage/sqlite", "package sqlite\nimport _ \"" + module + "/internal/app\"\n", "storage sqlite forbidden dependency"},
		{"query_use_case", "internal/storage/sqlite", "package sqlite\nimport productquery \"" + productQuery + "\"\nvar _ = productquery.NewService\n", "storage sqlite product query application call"},
		{"storage_sqlite_in_reconstruction", "internal/execution/reconstruction", "package reconstruction\nimport _ \"" + storageSQLite + "\"\n", "execution reconstruction dependency"},
		{"storage_ports_in_reconstruction", "internal/execution/reconstruction", "package reconstruction\nimport _ \"" + module + "/internal/storage/ports\"\n", ""},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, filepath.FromSlash(fixture.dir), "edge_plan9.go")
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("//go:build never_enabled\n\n"+fixture.source), 0600); err != nil {
				t.Fatal(err)
			}
			report, err := ScanSource(root)
			if err != nil {
				t.Fatal(err)
			}
			if fixture.want == "" {
				if len(report.Violations) != 0 {
					t.Fatalf("pure storage port was rejected: %+v", report)
				}
				return
			}
			if !strings.Contains(strings.Join(report.Violations, "\n"), fixture.want) {
				t.Fatalf("storage dependency escaped guard: %+v", report)
			}
		})
	}
}

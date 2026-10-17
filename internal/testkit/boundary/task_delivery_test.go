package boundary

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTaskDeliveryOwnership(t *testing.T) {
	for _, tc := range []struct {
		name, path, source, diagnostic string
	}{
		{"task owner", "internal/task/model/delivery.go", "package model\ntype TaskDeliveryRecord struct{}", ""},
		{"Runner profile public facts", "internal/execution/model/runner_profile.go", "package model\nimport _ \"" + module + "/internal/runnercontract\"", ""},
		{"checkpoint public build pin", "internal/execution/reconstruction/runtime_types.go", "package reconstruction\nimport _ \"" + module + "/internal/runnercontract\"", ""},
		{"old owner copy", "internal/execution/model/edge_plan9.go", "//go:build never_enabled\n\npackage model\ntype ProjectTaskAuthority struct{}", "task delivery definition outside task/model"},
		{"old owner alias", "internal/execution/model/edge_freebsd.go", "package model\nimport t \"" + module + "/internal/task/model\"\ntype TaskDeliveryReceipt = t.TaskDeliveryReceipt", "task delivery definition outside task/model"},
		{"helper alias", "internal/helper/edge.go", "package helper\nimport t \"" + module + "/internal/task/model\"\ntype TaskDeliveryEvidence = t.TaskDeliveryEvidence", "task delivery definition outside task/model"},
		{"validation forwarding", "internal/execution/model/edge.go", "package model\nfunc ValidateDelivery() {}", "task delivery definition outside task/model"},
		{"validation variable forwarding", "internal/helper/edge.go", "package helper\nvar ValidateDelivery = func() {}", "task delivery definition outside task/model"},
		{"task cannot import execution", "internal/task/model/edge_linux.go", "package model\nimport _ \"" + execution + "/model\"", "pure ownership dependency"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, filepath.FromSlash(tc.path))
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(tc.source), 0600); err != nil {
				t.Fatal(err)
			}
			report, err := ScanSource(root)
			if err != nil {
				t.Fatal(err)
			}
			if tc.diagnostic == "" {
				if len(report.Violations) != 0 {
					t.Fatal(report.Violations)
				}
			} else if !strings.Contains(strings.Join(report.Violations, "\n"), tc.diagnostic) {
				t.Fatalf("missing %q: %v", tc.diagnostic, report.Violations)
			}
		})
	}
	for _, file := range []string{
		"internal/storage/ports/native_task.go", "internal/storage/ports/task_graph.go", "internal/storage/ports/transactions.go",
		"internal/storage/sqlite/connections.go", "internal/storage/sqlite/reader.go", "internal/storage/sqlite/graph.go",
		"internal/storage/sqlite/sqlite_native_authority.go", "internal/storage/sqlite/runner_dispatch.go", "internal/storage/sqlite/runner_snapshot.go",
		"internal/storage/sqlite/sqlite_write_ports.go",
		"internal/product/query/taskgraph/model.go", "internal/app/task_command_store.go",
		"internal/taskrunner/service.go", "internal/taskrunner/control.go",
	} {
		t.Run(file, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, filepath.FromSlash(file))
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("//go:build never_enabled\n\npackage fixture\nimport old \""+execution+"/model\"\nvar _ old.WorkerStateFields"), 0600); err != nil {
				t.Fatal(err)
			}
			report, err := ScanSource(root)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(strings.Join(report.Violations, "\n"), "task facts execution dependency") {
				t.Fatal("execution import escaped task fact boundary", report.Violations)
			}
		})
	}
	if report := checkRepositoryBoundary(t); len(report.Violations) != 0 {
		t.Fatal(strings.Join(report.Violations, "\n"))
	}
}

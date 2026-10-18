package boundary

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHostCheckpointOwnership(t *testing.T) {
	for _, tc := range []struct{ name, file, source, diagnostic string }{
		{"host value", "internal/hostcontract/checkpoint.go", "package hostcontract\ntype ServiceCheckpoint struct{}", ""},
		{"public Runner pin", "internal/hostcontract/checkpoint.go", "package hostcontract\nimport _ \"" + module + "/internal/runnercontract\"", ""},
		{"old checkpoint copy", "internal/storage/ports/edge_plan9.go", "//go:build never_enabled\n\npackage ports\ntype ServiceCheckpoint struct{}", "host contract definition outside hostcontract"},
		{"old checkpoint alias", "internal/storage/ports/edge_freebsd.go", "package ports\nimport h \"" + hostContract + "\"\ntype ServiceCheckpoint = h.ServiceCheckpoint", "host contract definition outside hostcontract"},
		{"old state alias", "internal/execution/reconstruction/edge.go", "package reconstruction\nimport h \"" + hostContract + "\"\ntype ServiceState = h.ServiceState", "host contract definition outside hostcontract"},
		{"old receipt", "internal/execution/model/edge.go", "package model\ntype ControlReceipt struct{}", "host contract definition outside hostcontract"},
		{"old process value", "internal/execution/backend/edge.go", "package backend\ntype ProcessIdentity struct{}", "host contract definition outside hostcontract"},
		{"storage regression", "internal/storage/sqlite/sqlite_store_checkpoint.go", "package sqlite\nimport _ \"" + execution + "/model\"", "host checkpoint execution dependency"},
		{"reconstruction storage regression", "internal/execution/reconstruction/edge.go", "package reconstruction\nimport _ \"" + module + "/internal/storage/ports\"", "execution reconstruction dependency"},
	} {
		t.Run(tc.name, func(t *testing.T) { checkHostFixture(t, tc.file, tc.source, tc.diagnostic) })
	}
	for _, dep := range []string{"database/sql", execution + "/model", execution + "/backend", module + "/internal/app/host", module + "/internal/storage/ports", module + "/internal/product/api", module + "/internal/helper"} {
		t.Run(dep, func(t *testing.T) {
			checkHostFixture(t, "internal/hostcontract/edge_plan9.go", "//go:build never_enabled\n\npackage hostcontract\nimport _ \""+dep+"\"", "pure ownership dependency")
		})
	}
	if report := checkRepositoryBoundary(t); len(report.Violations) != 0 {
		t.Fatal(strings.Join(report.Violations, "\n"))
	}
}

func checkHostFixture(t *testing.T, name, source, diagnostic string) {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	report, err := ScanSource(root)
	if err != nil {
		t.Fatal(err)
	}
	if diagnostic == "" {
		if len(report.Violations) != 0 {
			t.Fatal(report.Violations)
		}
	} else if !strings.Contains(strings.Join(report.Violations, "\n"), diagnostic) {
		t.Fatalf("missing %s: %v", diagnostic, report.Violations)
	}
}

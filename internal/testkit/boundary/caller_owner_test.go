package boundary

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeCallerSource(t *testing.T, root, file, source string) {
	t.Helper()
	path := filepath.Join(root, file)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestCallerOwnerBoundary(t *testing.T) {
	for _, owner := range []string{"processwindow", "runnerclient", "runnerbridge", "runnerbinding"} {
		for _, dep := range []string{"database/sql", storageSQLite, module + "/internal/product/api", module + "/internal/app", module + "/internal/taskrunner"} {
			t.Run(owner+"/"+dep, func(t *testing.T) {
				root := t.TempDir()
				writeCallerSource(t, root, "internal/"+owner+"/edge_plan9.go", "//go:build never_enabled\n\npackage "+owner+"\nimport _ \""+dep+"\"\n")
				report, err := ScanSource(root)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(strings.Join(report.Violations, "\n"), "caller owner dependency") {
					t.Fatal(report.Violations)
				}
			})
		}
		t.Run(owner+"/transitive_inactive_helper", func(t *testing.T) {
			root := t.TempDir()
			writeCallerSource(t, root, "internal/"+owner+"/edge.go", "package "+owner+"\nimport _ \""+module+"/internal/helper\"\n")
			writeCallerSource(t, root, "internal/helper/edge_plan9.go", "//go:build never_enabled\n\npackage helper\nimport _ \"database/sql\"\n")
			report, err := ScanSource(root)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(strings.Join(report.Violations, "\n"), "caller owner transitive dependency: "+module+"/internal/"+owner+" -> "+module+"/internal/helper -> database/sql") {
				t.Fatal(report.Violations)
			}
		})
	}
	t.Run("window_helper_is_an_HTTP_infrastructure_dependency", func(t *testing.T) {
		root := t.TempDir()
		writeCallerSource(t, root, "internal/projectconnection/config.go", "package projectconnection\nimport _ \""+module+"/internal/processwindow\"\n")
		writeCallerSource(t, root, "internal/processwindow/window_windows.go", "package processwindow\nimport _ \"golang.org/x/sys/windows\"\n")
		report, err := ScanSource(root)
		if err != nil || len(report.Violations) > 0 {
			t.Fatal(err, report.Violations)
		}
	})
	if report := checkRepositoryBoundary(t); len(report.Violations) > 0 {
		t.Fatal(strings.Join(report.Violations, "\n"))
	}
}

func TestTakeoverContractOwnership(t *testing.T) {
	for _, name := range []string{"TakeoverReport", "TakeoverItem", "PendingStop", "ServiceStartInput"} {
		for _, owner := range []string{"runnercontract", "runnerclient", "app"} {
			t.Run(owner+"/"+name, func(t *testing.T) {
				root := t.TempDir()
				writeCallerSource(t, root, "internal/"+owner+"/edge_plan9.go", "//go:build never_enabled\n\npackage "+owner+"\ntype "+name+" struct{}\n")
				report, err := ScanSource(root)
				if err != nil {
					t.Fatal(err)
				}
				bad := strings.Contains(strings.Join(report.Violations, "\n"), "takeover contract definition outside runnercontract")
				if bad != (owner != "runnercontract") {
					t.Fatal(report.Violations)
				}
			})
		}
	}
}

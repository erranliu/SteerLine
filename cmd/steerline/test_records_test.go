package main

import (
	"encoding/json"
	"github.com/erranliu/SteerLine/internal/testmanagement"
	tm "github.com/erranliu/SteerLine/internal/testmanagement/model"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTRRegisterCLIArgumentsAndOfflineReportRetained(t *testing.T) {
	for _, args := range [][]string{{}, {"-project", "p"}, {"-project", "p", "-report-dir", "relative"}, {"-home", "unused"}} {
		if err := runTestRegister(args); err == nil {
			t.Fatal("invalid registration accepted", args)
		}
	}
	t.Setenv("STEERLINE_HOME", t.TempDir())
	dir := t.TempDir()
	d := tm.Definition{ID: "rules", Name: "Rules", Level: "L1", Environment: "isolated", Command: "test", Check: "actual"}
	report := tm.Report{ID: strings.Repeat("a", 32), Sequence: 1, StartedAt: time.Now(), FinishedAt: time.Now(), Definition: d, Command: "test", Repository: dir, Selection: []string{"rules"}, Environment: d.Environment, Definitions: map[string]string{d.ID: testmanagement.Digest(d)}, Results: []tm.Result{{TestID: d.ID, Status: "failed", Message: "actual failure", Log: "summary.json"}}, Evidence: tm.ReportEvidence{Path: "summary.json"}}
	raw, _ := json.Marshal(report)
	path := filepath.Join(dir, "managed-rules.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := runTestRegister([]string{"-project", "fixture", "-report-dir", dir}); err == nil {
		t.Fatal("offline registration reported success")
	}
	kept, err := os.ReadFile(path)
	if err != nil || string(kept) != string(raw) {
		t.Fatal("registration changed original execution", err)
	}
	receipt, err := os.ReadFile(filepath.Join(dir, "registration.json"))
	if err != nil || !strings.Contains(string(receipt), `"status": "unknown"`) || !strings.Contains(string(receipt), report.ID) {
		t.Fatal(string(receipt), err)
	}
}

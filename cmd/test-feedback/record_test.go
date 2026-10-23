package main

import (
	"encoding/json"
	"github.com/erranliu/SteerLine/internal/testmanagement"
	m "github.com/erranliu/SteerLine/internal/testmanagement/model"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTRProducerRepositoryExportAndSourceIsolation(t *testing.T) {
	repo := t.TempDir()
	t.Chdir(repo)
	git := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("git", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s %v", args, out, err)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "--initial-branch=main")
	git("config", "user.name", "history")
	git("config", "user.email", "history@example.invalid")
	if err := os.WriteFile("code.txt", []byte("one"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("-c", "commit.gpgsign=false", "commit", "-qm", "code")
	before := observeManagedSource()
	if !before.clean {
		t.Fatal(before)
	}
	d := testmanagement.Definition{ID: "l1.minimum", Name: "minimum", Level: "L1", Environment: "isolated", Command: "test", Check: "actual"}
	at := time.Date(2026, 10, 23, 1, 0, 0, 0, time.UTC)
	summary := []byte(`{"passed":false}`)
	r := testmanagement.Report{ID: strings.Repeat("a", 32), Sequence: 1, StartedAt: at, FinishedAt: at.Add(time.Second), Definition: d, Command: "test", Repository: managedGitValue("rev-parse", "--path-format=absolute", "--git-common-dir"), Environment: d.Environment, Selection: []string{d.ID}, Definitions: map[string]string{d.ID: testmanagement.Digest(d)}, BeforeCommit: before.commit, AfterCommit: before.commit, BeforeClean: true, AfterClean: true, Evidence: m.ReportEvidence{Path: "summary.json", Bytes: len(summary), SHA256: testmanagement.ContentDigest(summary)}, Results: []testmanagement.Result{{TestID: d.ID, Status: "failed", Message: "actual assertion failed", Log: "summary.json"}}}
	dir := t.TempDir()
	write := func() {
		t.Helper()
		if err := testmanagement.WriteRecordJSON(filepath.Join(dir, "managed-"+d.ID+".json"), r); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "summary.json"), summary, 0600); err != nil {
		t.Fatal(err)
	}
	write()
	name, err := exportReports(dir)
	if err != nil || filepath.Base(name) != before.commit+".json" {
		t.Fatal(name, err)
	}
	if source := observeManagedSource(); !source.clean {
		t.Fatal("history-only changes made source dirty", source)
	}
	if _, err = exportReports(dir); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(name)
	var file testmanagement.RecordFile
	if err != nil || json.Unmarshal(raw, &file) != nil || len(file.Runs) != 1 {
		t.Fatal(string(raw), err)
	}
	r.Results[0].Status = "passed"
	r.FinishedAt = r.FinishedAt.Add(time.Hour)
	r.StartedAt = r.StartedAt.Add(time.Hour)
	write()
	if _, err = exportReports(dir); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(name)
	file, _ = testmanagement.ParseRecordFile(raw)
	if len(file.Runs) != 2 || file.Runs[0].Results[0].Status != "failed" {
		t.Fatal("failure lost", file)
	}
	r.BeforeClean = false
	write()
	if _, err = exportReports(dir); err == nil {
		t.Fatal("dirty source exported")
	}
	r.BeforeClean = true
	r.Repository = "other-repository"
	write()
	if _, err = exportReports(dir); err == nil {
		t.Fatal("cross-repository report exported")
	}
	if err = os.WriteFile("code.txt", []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if observeManagedSource().clean {
		t.Fatal("source edit ignored")
	}
}

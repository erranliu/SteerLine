package main

import (
	"encoding/json"
	feedback "github.com/erranliu/SteerLine/internal/testkit/feedback"
	"github.com/erranliu/SteerLine/internal/testmanagement"
	"os"
	"path/filepath"
	"testing"
)

func TestTRProducerFreezesDefinitionAndIDsBeforeExecution(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	_ = os.WriteFile("go.mod", []byte("module fixture\n"), 0600)
	_ = os.MkdirAll("SteerlineFlow/testing", 0700)
	d := testmanagement.Definition{ID: "l1.minimum", Name: "before", Level: "L1", Environment: "isolated", Command: "test rules", Check: "actual rules"}
	catalog, _ := json.Marshal(testmanagement.Catalogue{Tests: []testmanagement.Definition{d}})
	_ = os.WriteFile(testmanagement.CataloguePath, catalog, 0600)
	opts := options{level: "L1", out: t.TempDir()}
	frozen, e := freezeManagedReports(opts)
	if e != nil {
		t.Fatal(e)
	}
	opts.managed = frozen
	d.Name = "after"
	catalog, _ = json.Marshal(testmanagement.Catalogue{Tests: []testmanagement.Definition{d}})
	_ = os.WriteFile(testmanagement.CataloguePath, catalog, 0600)
	summary := scopedReport{Scope: "quick", Complete: true, Passed: true, Groups: []scopedGroup{{Summary: feedback.Summary{Complete: true, Passed: true, Tests: []feedback.Test{{Status: "pass"}}}}}}
	if e = writeJSON(filepath.Join(opts.out, "summary.json"), summary); e != nil {
		t.Fatal(e)
	}
	writeManagedReports(opts, managedSource{}, 1)
	raw, e := os.ReadFile(filepath.Join(opts.out, "managed-l1.minimum.json"))
	if e != nil {
		t.Fatal(e)
	}
	r, e := testmanagement.ParseReport(raw)
	if e != nil || r.Definition.Name != "before" || r.ID != frozen.ids["l1.minimum"] || r.Results[0].Status != "passed" {
		t.Fatal(string(raw), e)
	}
	other, e := freezeManagedReports(opts)
	if e != nil || other.ids["l1.minimum"] == r.ID {
		t.Fatal("rerun reused execution identity", e)
	}
	d.Environment = "controlled"
	catalog, _ = json.Marshal(testmanagement.Catalogue{Tests: []testmanagement.Definition{d}})
	_ = os.WriteFile(testmanagement.CataloguePath, catalog, 0600)
	if _, err := freezeManagedReports(opts); err == nil {
		t.Fatal("environment mismatch was discovered after execution")
	}
}
func TestTRProducerRejectsRemovedRegistrationOptions(t *testing.T) {
	for _, args := range [][]string{{"-record-project=x"}, {"-level=L1", "-record-project=x", "--", "./subset"}, {"-level=L1", "-rerun-of=x=bad", "-build-only"}} {
		if _, e := parseOptions(args); e == nil {
			t.Fatal("removed database registration option accepted", args)
		}
	}
}

func TestTRProducerUnknownGitDoesNotBecomeFakeCommit(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	source := observeManagedSource()
	if source.commit != "" || source.clean {
		t.Fatal("Git failure became source identity", source)
	}
	if branch := managedGitValue("branch", "--show-current"); branch != "" {
		t.Fatal(branch)
	}
}

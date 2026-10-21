package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	feedback "github.com/erranliu/SteerLine/internal/testkit/feedback"
	"github.com/erranliu/SteerLine/internal/testmanagement"
)

func TestTMManagedResultsPreserveFailureAndNoMatch(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	_ = os.WriteFile("go.mod", []byte("module fixture\n"), 0600)
	_ = os.MkdirAll("docs/testing", 0700)
	d := testmanagement.Definition{ID: "l1.minimum", Name: "L1", Level: "L1", Environment: "isolated", Command: "go run ./cmd/test-feedback -level=L1", Check: "complete"}
	raw, _ := json.Marshal(testmanagement.Catalogue{Tests: []testmanagement.Definition{d}})
	_ = os.WriteFile(testmanagement.CataloguePath, raw, 0600)
	for _, tt := range []struct {
		name                    string
		pass, complete, matched bool
		want                    string
	}{{"pass", true, true, true, "passed"}, {"failure", false, true, true, "failed"}, {"incomplete", false, false, true, "incomplete"}, {"zero-match", true, true, false, "incomplete"}} {
		t.Run(tt.name, func(t *testing.T) {
			out := t.TempDir()
			summary := scopedReport{Scope: "quick", Passed: tt.pass, Complete: tt.complete}
			if tt.matched {
				summary.Groups = []scopedGroup{{Summary: feedback.Summary{Complete: true, Passed: tt.pass, Tests: []feedback.Test{}}}}
				summary.Groups[0].Summary.Tests = append(summary.Groups[0].Summary.Tests, feedback.Test{})
			}
			_ = writeJSON(filepath.Join(out, "summary.json"), summary)
			opts := options{level: "L1", out: out}
			frozen, freezeErr := freezeManagedReports(opts, 1)
			if freezeErr != nil {
				t.Fatal(freezeErr)
			}
			opts.managed = frozen
			writeManagedReports(opts, managedSource{}, 1)
			body, err := os.ReadFile(filepath.Join(out, "managed-l1.minimum.json"))
			if err != nil {
				t.Fatal(err)
			}
			r, err := testmanagement.ParseReport(body)
			if err != nil || r.Results[0].Status != tt.want {
				t.Fatal(string(body), err)
			}
		})
	}
	out := t.TempDir()
	writeManagedReports(options{level: "L1", out: out, goTestArgs: []string{"./subset"}}, managedSource{}, 1)
	if _, err := os.Stat(filepath.Join(out, "managed-l1.minimum.json")); !os.IsNotExist(err) {
		t.Fatal("targeted subset claimed complete L1")
	}
}

func TestTMRegisteredCatalogueMatchesExecutableManifest(t *testing.T) {
	root, err := e2eRoot()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(testmanagement.CataloguePath)))
	if err != nil {
		t.Fatal(err)
	}
	catalogue, err := testmanagement.ParseCatalogue(raw)
	if err != nil {
		t.Fatal(err)
	}
	manifestRaw, err := os.ReadFile(filepath.Join(root, "tests/e2e/suites.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest []e2eScenario
	if err = json.Unmarshal(manifestRaw, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(catalogue.Tests) != len(manifest)+1 {
		t.Fatal("catalogue and executable manifest differ")
	}
	byID := map[string]testmanagement.Definition{}
	for _, d := range catalogue.Tests {
		byID[d.ID] = d
	}
	for _, entry := range manifest {
		d, ok := byID[entry.ID]
		if !ok || d.Level != entry.Level || d.Environment != entry.Env || !strings.Contains(d.Command, "-level="+entry.Level) {
			t.Fatal("catalogue changed level/environment/command", entry.ID)
		}
	}
}

package projectmaintenance

import (
	"encoding/json"
	"errors"
	pm "github.com/erranliu/SteerLine/internal/projectmaintenance/model"
	testmanagement "github.com/erranliu/SteerLine/internal/testmanagement/model"
	"strings"
	"testing"
)

func materialFixture() map[string][]byte {
	c, _ := json.Marshal(testmanagement.Catalogue{Tests: []testmanagement.Definition{{ID: "one", Name: "one", Level: "L1", Environment: "isolated", Command: "test", Check: "check"}, {ID: "two", Name: "two", Level: "L1", Environment: "isolated", Command: "other", Check: "other check"}}})
	return map[string][]byte{"docs/task.plan.md": []byte("# Plan\nTest one\n"), testmanagement.RulesPath: []byte("# Testing\nRules\n"), testmanagement.CataloguePath: c, "docs/task.tests.json": []byte(`{"tests":["one"]}`)}
}
func TestPMRulesSameThreeEntrypoints(t *testing.T) {
	files := materialFixture()
	reads := 0
	read := func(p string) ([]byte, error) {
		reads++
		if b, ok := files[p]; ok {
			return b, nil
		}
		return nil, errors.New("missing")
	}
	var original Item
	for _, mode := range []string{"initialize", "baseline", "local"} {
		a, e := Evaluate(mode, "commit", []string{"docs/task.plan.md"}, read, nil)
		if e != nil {
			t.Fatal(e)
		}
		i := a.Items[0]
		if i.Status != "needs_review" {
			t.Fatal(i)
		}
		if original.Inputs != "" && i.Inputs != original.Inputs {
			t.Fatal("separate rule paths")
		}
		original = i
	}
	if reads != 12 {
		t.Fatal("unexpected model or extra discovery", reads)
	}
	ack := map[string]pm.Approval{"docs/task.plan.md": {Revision: 1, Inputs: original.Inputs, Output: original.Output}}
	for p, b := range files {
		files[p] = []byte(strings.ReplaceAll(string(b), "\n", "\r\n"))
	}
	a, e := Evaluate("baseline", "new-commit", []string{"docs/task.plan.md"}, read, ack)
	if e != nil || a.Items[0].Status != "ready" {
		t.Fatal("CRLF or commit caused invalidation", a, e)
	}
	// Even an unselected definition is part of the actual complete model input.
	files[testmanagement.CataloguePath] = []byte(strings.ReplaceAll(string(files[testmanagement.CataloguePath]), "other check", "changed unselected definition"))
	a, _ = Evaluate("baseline", "newer", []string{"docs/task.plan.md"}, read, ack)
	if a.Items[0].Status != "needs_review" {
		t.Fatal("ignored part of actual generator input")
	}
}
func TestPMRulesMissingInvalidAndEmpty(t *testing.T) {
	files := materialFixture()
	read := func(p string) ([]byte, error) {
		b, ok := files[p]
		if !ok {
			return nil, errors.New("missing " + p)
		}
		return b, nil
	}
	delete(files, "docs/task.tests.json")
	a, e := Evaluate("initialize", "commit", []string{"docs/task.plan.md"}, read, nil)
	if e != nil || a.Items[0].Status != "invalid" {
		t.Fatal(a, e)
	}
	files["docs/task.tests.json"] = []byte(`{"tests":[]}`)
	a, _ = Evaluate("local", "draft", []string{"docs/task.plan.md"}, read, nil)
	if a.Items[0].Status != "invalid" {
		t.Fatal("unexplained empty arrangement accepted")
	}
	files["docs/task.plan.md"] = []byte("# Plan\n无需自动测试\n")
	a, _ = Evaluate("local", "draft", []string{"docs/task.plan.md"}, read, nil)
	if a.Items[0].Status != "needs_review" {
		t.Fatal(a)
	}
	a, e = Evaluate("initialize", "commit", nil, read, nil)
	if e != nil || len(a.Items) != 1 || a.Items[0].Plan != "" {
		t.Fatal("initialization invented a Task", a, e)
	}
	if _, e = Evaluate("local", "draft", []string{"../escape.plan.md"}, read, nil); e == nil {
		t.Fatal("unsafe path accepted")
	}
}

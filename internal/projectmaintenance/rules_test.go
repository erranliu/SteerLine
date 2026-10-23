package projectmaintenance

import (
	"errors"
	tm "github.com/erranliu/SteerLine/internal/testmanagement/model"
	"testing"
)

func TestPMSMaterialInputsDoNotRequireArrangement(t *testing.T) {
	files := map[string][]byte{"SteerlineFlow/plans/task.plan.md": []byte("# Plan\nTest one\n"), tm.RulesPath: []byte("# Rules\n"), tm.CataloguePath: []byte(`{"tests":[{"id":"one","name":"One","level":"L1","environment":"isolated","command":"test","check":"pass"}]}`)}
	read := func(path string) ([]byte, error) {
		v, ok := files[path]
		if !ok {
			return nil, errors.New("missing")
		}
		return v, nil
	}
	sources, e := GenerationSources("SteerlineFlow/plans/task.plan.md", read)
	if e != nil || len(sources) != 3 {
		t.Fatal(sources, e)
	}
	before := sources[0].TextDigest
	files["SteerlineFlow/plans/task.plan.md"] = []byte("# Plan\nchanged\n")
	sources, e = GenerationSources("SteerlineFlow/plans/task.plan.md", read)
	if e != nil || sources[0].TextDigest == before {
		t.Fatal("Plan update was ignored", e)
	}
	files[tm.CataloguePath] = []byte(`{"tests":[{"id":"unknown"}]}`)
	if _, e = GenerationSources("SteerlineFlow/plans/task.plan.md", read); e == nil {
		t.Fatal("invalid catalogue accepted")
	}
	files[tm.RulesPath] = []byte{255}
	if _, e = GenerationSources("SteerlineFlow/plans/task.plan.md", read); e == nil {
		t.Fatal("non UTF-8 accepted")
	}
	if _, e = GenerationSources("../escape.plan.md", read); e == nil {
		t.Fatal("invalid path accepted")
	}
}

func TestPMSDoesNotReadMaterialsFromLegacyDocs(t *testing.T) {
	files := map[string][]byte{
		"SteerlineFlow/plans/task.plan.md": []byte("# Plan\nTest one\n"),
		"docs/testing.md":                  []byte("# Legacy rules\n"),
		"docs/testing/test-list.json":      []byte(`{"tests":[]}`),
	}
	var requested []string
	read := func(path string) ([]byte, error) {
		requested = append(requested, path)
		if raw, ok := files[path]; ok {
			return raw, nil
		}
		return nil, errors.New("missing")
	}
	if _, err := GenerationSources("SteerlineFlow/plans/task.plan.md", read); err == nil {
		t.Fatal("legacy-only materials accepted")
	}
	for _, path := range requested {
		if path == "docs/testing.md" || path == "docs/testing/test-list.json" {
			t.Fatalf("read legacy material %s", path)
		}
	}
}

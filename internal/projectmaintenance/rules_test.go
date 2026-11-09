package projectmaintenance

import (
	"errors"
	tm "github.com/erranliu/SteerLine/internal/testmanagement/model"
	"reflect"
	"testing"
)

func arrangementFixture() (map[string][]byte, Reader, []byte) {
	protocol := []byte("# Fixed protocol\n")
	files := map[string][]byte{
		tm.ProjectEntryPath: []byte(`{"version":1,"requirements":["quality/policy.md"],"testing_catalogue":"quality/cases.json"}`),
		"task.plan.md":      []byte("# Plan\nTest one\n"), "task.tests.json": []byte(`{"tests":["one"]}`),
		"quality/policy.md": []byte("Run one\n"), tm.ProtocolPath: protocol,
		"quality/cases.json": []byte(`{"tests":[{"id":"one","name":"One","level":"L1","environment":"isolated","command":"test","check":"pass"}]}`),
	}
	read := func(path string) ([]byte, error) {
		if b, ok := files[path]; ok {
			return b, nil
		}
		return nil, errors.New("missing " + path)
	}
	return files, read, protocol
}
func TestPMSCommittedArrangementAndRelevantDefinitions(t *testing.T) {
	files, read, protocol := arrangementFixture()
	first, basis, err := Arrangement("task.plan.md", "first", read, protocol)
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != "ready" || len(first.Definitions) != 1 || first.Definitions[0].ID != "one" {
		t.Fatal(first)
	}
	files["quality/cases.json"] = []byte(`{"tests":[{"id":"other","name":"Other","level":"L4","environment":"isolated","command":"other","check":"pass"},{"id":"one","name":"One","level":"L1","environment":"isolated","command":"test","check":"pass"}]}`)
	next, nextBasis, err := Arrangement("task.plan.md", "next", read, protocol)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(basis, nextBasis) || first.Digest != next.Digest || next.Commit != "next" {
		t.Fatal("unselected entry changed material", first, next)
	}
	files["task.tests.json"] = []byte(`{"tests":["other","one"]}`)
	changed, _, err := Arrangement("task.plan.md", "next", read, protocol)
	if err != nil || changed.Digest == first.Digest || changed.Definitions[0].ID != "other" {
		t.Fatal("selection/order lost", changed, err)
	}
}
func TestPMSInvalidArrangementsAndMissingFiles(t *testing.T) {
	for _, raw := range []string{`{}`, `{"tests":null}`, `{"tests":["missing"]}`, `{"tests":["one","one"]}`, `{"tests":[],"extra":true}`, `{"tests":[],"tests":["one"]}`, `{"tests":[]}`} {
		t.Run(raw, func(t *testing.T) {
			files, read, protocol := arrangementFixture()
			files["task.tests.json"] = []byte(raw)
			if _, _, err := Arrangement("task.plan.md", "c", read, protocol); err == nil {
				t.Fatal("invalid arrangement accepted")
			}
		})
	}
	files, read, protocol := arrangementFixture()
	delete(files, "task.tests.json")
	if _, _, err := Arrangement("task.plan.md", "c", read, protocol); err == nil {
		t.Fatal("missing sidecar accepted")
	}
	if _, _, err := Arrangement("../task.plan.md", "c", read, protocol); err == nil {
		t.Fatal("invalid plan path accepted")
	}
	files["task.tests.json"] = []byte(`{"tests":[]}`)
	files["task.plan.md"] = []byte("# Plan\n无需自动测试：仅文档\n")
	if got, _, err := Arrangement("task.plan.md", "c", read, protocol); err != nil || got.Status != "ready" || len(got.Definitions) != 0 {
		t.Fatal(got, err)
	}
}
func TestPMSFixedProtocolAndProjectSources(t *testing.T) {
	files, read, protocol := arrangementFixture()
	files[tm.ProtocolPath] = []byte("# Fixed protocol\r\n")
	if _, _, err := Arrangement("task.plan.md", "c", read, protocol); err != nil {
		t.Fatal(err)
	}
	for _, expected := range [][]byte{nil, []byte("other"), []byte("# Fixed protocol\n ")} {
		if _, _, err := Arrangement("task.plan.md", "c", read, expected); err == nil {
			t.Fatal("different protocol accepted")
		}
	}
	files["quality/policy.md"] = []byte{255}
	if _, _, err := Arrangement("task.plan.md", "c", read, protocol); err == nil {
		t.Fatal("invalid UTF-8 accepted")
	}
	delete(files, tm.ProjectEntryPath)
	if _, _, err := Arrangement("task.plan.md", "c", read, protocol); err == nil {
		t.Fatal("missing project entry accepted")
	}
}

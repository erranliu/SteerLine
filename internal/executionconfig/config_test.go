package executionconfig

import (
	"encoding/json"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"testing"
)

func TestExecutionConfigValues(t *testing.T) {
	p := Profile{ID: "local", Name: "本机", Execution: runnercontract.ExecutionInput{Adapter: "codex-app-server", Target: runnercontract.ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{"model":"example","reasoning_effort":"high"}`)}, Workflow: runnercontract.Workflow{ID: "development", Version: 2}}
	if err := Validate(3, "local", []Profile{p}); err != nil {
		t.Fatal(err)
	}
	for _, profiles := range [][]Profile{nil, {p, p}} {
		if Validate(3, "local", profiles) == nil {
			t.Fatal("invalid directory accepted")
		}
	}
	q := p
	q.Name = "renamed"
	if p.Digest() != q.Digest() {
		t.Fatal("name changed execution parameter digest")
	}
	for _, raw := range []string{`{"version":3,"version":3}`, `{"version":3} {}`, `{"version":3,"unknown":1}`} {
		var c File[Pin]
		if Decode([]byte(raw), &c) == nil {
			t.Fatal("ambiguous JSON accepted", raw)
		}
	}
}

package app

import (
	"encoding/json"
	"strings"
	"testing"

	roadmap "github.com/erranliu/SteerLine/internal/roadmap/model"
	tm "github.com/erranliu/SteerLine/internal/testmanagement/model"
)

func roadmapFutureTestPreview(t *testing.T) {
	t.Helper()
	r := roadmap.Record{Analysis: &roadmap.Analysis{Coordination: "Reuse the declared owners and confirmed dependency handoffs.", Tasks: []roadmap.Node{{Key: "tools", ProvidesTests: []string{"new"}}, {Key: "sample", Proposed: roadmap.Dependencies{Local: []string{"tools"}}}}}, Documents: []roadmap.Documents{
		{Key: "tools", Tests: []string{"old"}, ProvidesTests: []string{"new"}},
		{Key: "sample", Tests: []string{"old", "new"}},
	}}
	catalogue := tm.Catalogue{Tests: []tm.Definition{{ID: "old", Command: "test-old"}}}
	before, err := roadmapTestPreview(r, r.Documents[1], catalogue)
	require(t, err)
	raw, _ := json.Marshal(before)
	if !strings.Contains(string(raw), `"pending_tests":["new"]`) || strings.Contains(string(raw), "test-new") {
		t.Fatal(string(raw))
	}
	catalogue.Tests = append(catalogue.Tests, tm.Definition{ID: "new", Command: "test-new"})
	after, err := roadmapTestPreview(r, r.Documents[1], catalogue)
	require(t, err)
	raw, _ = json.Marshal(after)
	if !strings.Contains(string(raw), "test-new") || !strings.Contains(string(raw), `"pending_tests":[]`) {
		t.Fatal(string(raw))
	}
}

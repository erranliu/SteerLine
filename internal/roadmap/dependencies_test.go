package roadmap

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/roadmap/model"
)

func TestRoadmapMixedFormalDependencies(t *testing.T) {
	source := sampleSource()
	graph := []model.Existing{{ID: "8", Title: "调度", Status: "open"}, {ID: "12", Title: "接口", Predecessors: []string{"8"}}}
	a := sampleAnalysis()
	a.Tasks[1].Original.Formal = []string{"12"}
	a.Tasks[1].Proposed.Formal = []string{"12"}
	raw, _ := json.Marshal(a)
	got, err := model.DecodeAnalysis(raw, source, graph)
	if err != nil || len(got.Tasks) != 2 || got.Tasks[0].ExistingID != "" ||
		strings.Join(got.Tasks[1].Proposed.Local, ",") != "Alpha" ||
		strings.Join(got.Tasks[1].Proposed.Formal, ",") != "12" {
		t.Fatal("formal predecessor became a draft task or mixed references were lost", got, err)
	}
	// Existing tasks need not finish before a roadmap can be confirmed.
	r := model.Record{State: "review", Source: source, Analysis: &got, GraphDigest: model.GraphDigest(graph)}
	graph[0].Status = "done"
	if _, err := model.Confirm(r, source, graph, "confirm"); err != nil {
		t.Fatal("runtime progress invalidated dependency confirmation", err)
	}
	graph[0].Status = "discarded"
	if _, err := model.Confirm(r, source, graph, "confirm"); err == nil || !strings.Contains(err.Error(), "#8") {
		t.Fatal("discarded predecessor was confirmed", err)
	}
}

func TestRoadmapDependencyValidationAndAliases(t *testing.T) {
	graph := []model.Existing{{ID: "8", Title: "调度"}, {ID: "12", Title: "接口", Predecessors: []string{"8"}}}
	for _, tc := range []struct {
		name, want string
		change     func(*model.Analysis)
	}{
		{"missing-original-formal", "原依赖", func(a *model.Analysis) { a.Tasks[0].Original.Formal = []string{"999"} }},
		{"missing-proposed-formal", "#999", func(a *model.Analysis) { a.Tasks[0].Proposed.Formal = []string{"999"} }},
		{"missing-original-local", "原依赖", func(a *model.Analysis) { a.Tasks[0].Original.Local = []string{"unknown"} }},
		{"self-original", "内部依赖无效", func(a *model.Analysis) { a.Tasks[0].Original.Local = []string{"Alpha"} }},
		{"noncanonical-id", "编号无效", func(a *model.Analysis) { a.Tasks[0].Proposed.Formal = []string{"08"} }},
		{"prefixed-id", "编号无效", func(a *model.Analysis) { a.Tasks[0].Proposed.Formal = []string{"#8"} }},
		{"overflow-id", "编号无效", func(a *model.Analysis) { a.Tasks[0].Proposed.Formal = []string{"9223372036854775808"} }},
		{"negative-id", "编号无效", func(a *model.Analysis) { a.Tasks[0].Proposed.Formal = []string{"-1"} }},
		{"changed-without-reason", "说明理由", func(a *model.Analysis) { a.Tasks[0].Proposed.Formal = []string{"12"} }},
		{"local-cycle", "存在环", func(a *model.Analysis) {
			a.Tasks[0].Original.Local = []string{"Beta"}
			a.Tasks[0].Proposed.Local = []string{"Beta"}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := sampleAnalysis()
			tc.change(&a)
			raw, _ := json.Marshal(a)
			if _, err := model.DecodeAnalysis(raw, sampleSource(), graph); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}

	a := sampleAnalysis()
	a.Tasks[0].ExistingID = "8"
	a.Tasks[0].Original = model.Dependencies{}
	a.Tasks[0].Proposed = model.Dependencies{}
	a.Tasks[1].Original.Local = []string{"Alpha", "Alpha"}
	a.Tasks[1].Original.Formal = []string{"8"}
	a.Tasks[1].Proposed = a.Tasks[1].Original
	raw, _ := json.Marshal(a)
	got, err := model.DecodeAnalysis(raw, sampleSource(), graph)
	if err != nil || len(got.Tasks[1].Proposed.Local) != 0 || strings.Join(got.Tasks[1].Proposed.Formal, ",") != "8" || len(got.Tasks[1].Original.Local) != 2 {
		t.Fatal("aliases were not deduplicated or original references changed", got, err)
	}
	// A formal predecessor graph is part of cycle validation, even when its
	// nodes are not copied into the roadmap analysis.
	graph[0].Predecessors = []string{"12"}
	raw, _ = json.Marshal(sampleAnalysis())
	if _, err := model.DecodeAnalysis(raw, sampleSource(), graph); err == nil || !strings.Contains(err.Error(), "存在环") {
		t.Fatal("formal DAG cycle accepted", err)
	}
}

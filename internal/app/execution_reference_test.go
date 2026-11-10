package app

import (
	"encoding/json"
	"strings"
	"testing"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/product/api"
	roadmap "github.com/erranliu/SteerLine/internal/roadmap/model"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func TestExecutionReferencesPreserveSelectedRunWithoutPrivateBindings(t *testing.T) {
	run := &runnercontract.Run{Execution: runnercontract.Execution{AttemptID: "original-attempt"}}
	ref := api.ExecutionReference{RequestID: "original-request", RunID: "original-run", AttemptID: "original-attempt"}
	r := roadmap.Record{RequestID: ref.RequestID, RunID: ref.RunID, Projection: run, Runner: runnercontract.BuildPin{StateDir: "private-state"}}
	projected := publicRoadmap(r)
	if projected.Execution != ref {
		t.Fatalf("wrong roadmap reference: %+v", projected.Execution)
	}
	raw, err := json.Marshal(projected)
	if err != nil || strings.Contains(string(raw), "private-state") {
		t.Fatalf("private binding exposed: %s %v", raw, err)
	}
	detection := env.Record{ID: ref.RequestID, Rounds: []env.Round{{RunID: "older-run"}, {RunID: ref.RunID, Run: run}}}
	if got := environmentExecutionReference(detection); got != ref {
		t.Fatalf("wrong detection reference: %+v", got)
	}
	detection.Rounds = nil
	if got := environmentExecutionReference(detection); got.RunID != "" || got.AttemptID != "" {
		t.Fatalf("invented unstarted run: %+v", got)
	}
}

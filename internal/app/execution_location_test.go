package app

import (
	"encoding/json"
	"testing"

	"github.com/erranliu/SteerLine/internal/hostcontract"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	ports "github.com/erranliu/SteerLine/internal/storage/ports"
	"github.com/erranliu/SteerLine/internal/taskrunner"
)

func TestExecutionLocationPresentation(t *testing.T) {
	config := Config{Repository: "C:/项目/repo"}
	if projectPresentation(config).ExecutionLocation.Runner != nil {
		t.Fatal("unconfigured Runner was exposed as enabled")
	}
	config.RunnerExecution.Runner.Executable = "C:/工具/steerrunner.exe"
	location := projectPresentation(config).ExecutionLocation
	if location.Repository != config.Repository || location.Runner == nil || location.Runner.Executable != config.RunnerExecution.Runner.Executable {
		t.Fatalf("lost configured execution location: %+v", location)
	}
}

func TestExecutionLocationCandidateProjection(t *testing.T) {
	// The saved directory must survive all display projections, including when
	// no public Runner observation has arrived yet. Each candidate owns its path.
	candidates := []runnerstate.Candidate{
		{ID: "candidate-a", TaskID: "1", StateDir: "C:/slr/first", RunID: "runner-a", Projection: &runnerclient.Run{Execution: runnerclient.Execution{AttemptID: "attempt-a"}}},
		{ID: "candidate-b", TaskID: "1", StateDir: "D:/slr/second"},
	}
	runtime := taskrunner.Project(ports.RunnerSnapshot{Candidates: candidates}, hostcontract.ServiceCheckpoint{})
	page := productServicePage(&runtime)
	for i, candidate := range candidates {
		got := page.Runs[i]
		if got.RunID != candidate.ID || got.ExecutionID != candidate.RunID || got.RunnerStateDir != candidate.StateDir {
			t.Fatalf("candidate location was lost or rebound: %+v", got)
		}
		encoded, err := json.Marshal(got)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		if err := json.Unmarshal(encoded, &fields); err != nil || fields["runner_state_dir"] != candidate.StateDir {
			t.Fatalf("location missing from public JSON: %s (%v)", encoded, err)
		}
	}
	if page.Runs[0].AttemptID != "attempt-a" || page.Runs[1].AttemptID != "" {
		t.Fatal("attempt identities crossed candidate boundaries")
	}
}

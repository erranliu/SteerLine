package query

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func TestSampledHistoryAndCandidateProjection(t *testing.T) {
	at := time.Now().UTC()
	candidate := runnerstate.Candidate{ID: "candidate", TaskID: "7", RunID: "run", Number: 2, ProfileID: "profile", CreatedAt: at, Participation: "active", SubmitState: "accepted", StateDir: "private-runner-directory",
		Submit:     runnercontract.Request{RequestID: "private-original-request", Input: json.RawMessage(`{"workflow":{"id":"development","version":1},"execution":{"adapter":"codex-app-server","target":{"kind":"local"},"settings":{"private":"private-profile-setting"}}}`)},
		Projection: &runnercontract.Run{RunID: "run", Revision: 9, Status: "waiting_input", Step: "implement", Execution: runnercontract.Execution{AttemptID: "attempt"}, Question: &runnercontract.Question{ID: "question"}}}
	tasks := &storageports.NativeTaskPage{ProjectID: "project", DatabaseID: strings.Repeat("a", 32), DatabaseRevision: 4, AuthorityVersion: 2, GraphRevision: 3, SampledAt: at, HasMore: true,
		Tasks: []storageports.NativeTaskDetail{{TaskRecord: taskmodel.TaskRecord{TaskID: "7", Title: "task\u202etext"}, Candidates: []runnerstate.Candidate{candidate}}}}
	view := storageports.ProjectView{SampledAt: at, Tasks: tasks, SelectedTask: &tasks.Tasks[0], Candidates: []runnerstate.Candidate{candidate}}
	before, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	page := ProjectPage(view, "7", 0, api.ObservationSnapshot{}, api.ServiceConnection{}, nil, nil)
	if page.History == nil || len(page.History.WorkflowRuns) != 1 || len(page.History.Attempts) != 1 || len(page.History.Decisions) != 1 || page.SelectedNative == nil || len(page.SelectedNative.WorkflowRuns) != 1 {
		t.Fatalf("sampled candidate facts were omitted: %+v", page)
	}
	history, detail := page.History.WorkflowRuns[0], page.SelectedNative.WorkflowRuns[0]
	if history.WorkflowRunID != "candidate" || detail.WorkflowRunID != "candidate" || history.WorkflowStage != "implement" || history.CandidateNumber != 2 || history.ExecutionProfile.ID != "profile" || history.ExecutionProfile.Agent != "codex-app-server" || history.ExecutionProfile.Host != "local" || page.History.Attempts[0].AttemptID != "attempt" || page.History.Decisions[0].DecisionID != "question" || page.History.Decisions[0].Version != 9 || !page.FullReadAt.Equal(at) {
		t.Fatal("identity, profile, stage or sample time changed", page.History)
	}
	var cursor TaskCursor
	rawCursor, err := base64.RawURLEncoding.DecodeString(page.NativeTasks.NextCursor)
	if err != nil || json.Unmarshal(rawCursor, &cursor) != nil || cursor.Offset != 1 || cursor.DatabaseRevision != 4 || cursor.GraphRevision != 3 || page.NativeTasks.Protocol != NativeTaskProtocol {
		t.Fatal("project page lost its sampled continuation", page.NativeTasks)
	}
	encoded, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"private-runner-directory", "private-original-request", "private-profile-setting"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("private/bidi source escaped the public projection: %s", encoded)
		}
	}
	taskJSON, err := json.Marshal(page.NativeTasks)
	var decoded api.TaskPage
	if err != nil || bytes.Contains(taskJSON, []byte("\u202e")) || json.Unmarshal(taskJSON, &decoded) != nil || decoded.Tasks[0].Title != tasks.Tasks[0].Title {
		t.Fatal("public task page lost terminal-safe bidi roundtrip", err)
	}
	page.History.WorkflowRuns[0].ExecutionProfile.ID = "changed"
	page.SelectedNative.WorkflowRuns[0].ExecutionProfile.ID = "changed"
	after, err := json.Marshal(view)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("product projection mutated sampled storage facts", err)
	}
}

func TestSampledHistoryDistinguishesEmptyAndFailedReads(t *testing.T) {
	view := storageports.ProjectView{Candidates: []runnerstate.Candidate{}, Tasks: &storageports.NativeTaskPage{}}
	page := ProjectPage(view, "", 0, api.ObservationSnapshot{}, api.ServiceConnection{}, nil, nil)
	if page.History == nil || len(page.History.WorkflowRuns) != 0 || page.HistoryError != "" {
		t.Fatal("empty successful history became unavailable", page)
	}
	view.Candidates = []runnerstate.Candidate{{ID: "partial-row"}}
	view.HistoryError = errors.New("candidate read failed")
	page = ProjectPage(view, "", 0, api.ObservationSnapshot{}, api.ServiceConnection{}, nil, nil)
	if page.History != nil || page.HistoryError != "candidate read failed" || page.NativeTasks == nil || !page.FullReadAt.IsZero() {
		t.Fatal("failed history invented success or hid usable task data", page)
	}
}

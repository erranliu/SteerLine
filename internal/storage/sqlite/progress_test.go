package sqlite

import (
	"context"
	"github.com/erranliu/SteerLine/internal/testmanagement/model"
	"testing"
)

func TestProgressCASAndOriginalCandidateBinding(t *testing.T) {
	s, p, tasks := nativeAuditStore(t)
	ctx := context.Background()
	_, err := s.db.ExecContext(ctx, `INSERT INTO runner_candidates(project_identity,candidate_id,task_id,revision,request_id,run_id,payload) VALUES(?,?,?,1,?,?,?)`, p, "c", tasks[0].TaskID, "submit", "run", []byte(`{"run_id":"run","input_digest":"input"}`))
	require(t, err)
	r := model.Progress{Project: p, CandidateID: "c", RunID: "run", InputDigest: "input", Revision: 1, Completed: map[string]bool{"test.one": true}, ParsedVersion: 3}
	r.Snapshot.Version = 3
	require(t, s.SaveProgress(ctx, r, 0))
	if s.SaveProgress(ctx, r, 0) == nil {
		t.Fatal("duplicate write")
	}
	r.Revision++
	require(t, s.SaveProgress(ctx, r, 1))
	if s.SaveProgress(ctx, r, 1) == nil {
		t.Fatal("stale CAS")
	}
	for _, kind := range []string{"run", "input", "version"} {
		changed := r
		changed.Revision++
		switch kind {
		case "run":
			changed.RunID = "foreign"
		case "input":
			changed.InputDigest = "foreign"
		case "version":
			changed.Snapshot.Version = 2
		}
		if s.SaveProgress(ctx, changed, 2) == nil {
			t.Fatal("changed binding accepted", kind)
		}
	}
	got, err := s.Progress(ctx, p, "c")
	require(t, err)
	if got.Revision != 2 || !got.Completed["test.one"] {
		t.Fatal(got)
	}
}

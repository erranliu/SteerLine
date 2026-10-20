package sqlite

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/testmanagement/model"
)

func TestTMEvidenceImmutableCASAndCandidateAssociation(t *testing.T) {
	s, project, tasks := nativeAuditStore(t)
	ctx := context.Background()
	raw := []byte(`{"run_id":"original-run","input_digest":"input"}`)
	_, err := s.db.ExecContext(ctx, `INSERT INTO runner_candidates(project_identity,candidate_id,task_id,revision,request_id,run_id,payload) VALUES(?,?,?,1,?,?,?)`, project, "candidate", tasks[0].TaskID, "submit", "original-run", raw)
	require(t, err)
	r := model.Evidence{Project: project, CandidateID: "candidate", RunID: "original-run", InputDigest: "input", Revision: 1, Collections: []model.Collection{{Path: "one/managed-L1.json", Request: runnercontract.Request{Protocol: runnercontract.Protocol, RequestID: "collect-original", Input: json.RawMessage(`{"expected_revision":1,"relative_path":"one/managed-L1.json"}`)}, State: "unknown"}}, Artifacts: []model.EvidenceArtifact{{Metadata: runnercontract.Artifact{RunID: "original-run", ArtifactID: "artifact"}, Content: []byte("immutable")}}}
	require(t, s.SaveEvidence(ctx, r, 0))
	if err = s.SaveEvidence(ctx, r, 0); err == nil {
		t.Fatal("duplicate initial write")
	}
	r.Revision++
	r.Collections[0].State = "completed"
	require(t, s.SaveEvidence(ctx, r, 1))
	if err = s.SaveEvidence(ctx, r, 1); err == nil {
		t.Fatal("stale revision accepted")
	}
	for _, kind := range []string{"run", "input", "request", "body", "remove"} {
		b, _ := json.Marshal(r)
		var changed model.Evidence
		require(t, json.Unmarshal(b, &changed))
		changed.Revision++
		switch kind {
		case "run":
			changed.RunID = "foreign"
		case "input":
			changed.InputDigest = "foreign"
		case "request":
			changed.Collections[0].Request.RequestID = "replacement"
		case "body":
			changed.Artifacts[0].Content = []byte("changed")
		case "remove":
			changed.Artifacts = nil
		}
		if s.SaveEvidence(ctx, changed, 2) == nil {
			t.Fatalf("accepted %s rewrite", kind)
		}
	}
	got, err := s.Evidence(ctx, project, "candidate")
	require(t, err)
	if got.Revision != 2 || string(got.Artifacts[0].Content) != "immutable" {
		t.Fatal(got)
	}
}

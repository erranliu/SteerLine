package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	ma "github.com/erranliu/SteerLine/internal/mainagent/model"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func TestMaterialAnalysisStoreBindingRejectionAndRestart(t *testing.T) {
	ctx := context.Background()
	file := filepath.Join(t.TempDir(), "state.db")
	s, err := OpenSQLiteStore(file)
	require(t, err)
	r := ma.Record{Project: "p", ID: strings.Repeat("a", 32), Revision: 1, CreatedAt: time.Now().UTC(), Context: ma.Context{CandidateID: strings.Repeat("b", 32)}, Input: ma.Start{Kind: "clarification", Note: "original question"}, Submission: "pending", State: "queued", Request: runnercontract.Request{RequestID: "original", Input: json.RawMessage(`{"frozen":"original"}`)}}
	require(t, s.SaveAnalysis(ctx, r, 0))
	r.Revision++
	r.Submission, r.State = "started", "unknown"
	require(t, s.SaveAnalysis(ctx, r, 1))
	rejected := r
	rejected.Revision++
	rejected.Submission, rejected.State = "rejected", "rejected"
	require(t, s.SaveAnalysis(ctx, rejected, 2))
	require(t, s.Close())
	s, err = OpenSQLiteStore(file)
	require(t, err)
	defer s.Close()
	got, err := s.Analysis(ctx, "p", r.ID)
	require(t, err)
	if !got.Settled() || got.State != "rejected" {
		t.Fatal("definite rejection did not settle", got)
	}
	if _, err = s.Analysis(ctx, "other", r.ID); err == nil {
		t.Fatal("analysis leaked across projects")
	}
	for _, mutate := range []func(*ma.Record){func(v *ma.Record) { v.Context.CandidateID = "replacement" }, func(v *ma.Record) { v.Request.Input = json.RawMessage(`{"frozen":"changed"}`) }, func(v *ma.Record) { v.Submission = "pending" }} {
		bad := got
		bad.Revision++
		mutate(&bad)
		if err = s.SaveAnalysis(ctx, bad, got.Revision); err == nil {
			t.Fatal("immutable analysis association changed")
		}
	}
}
func TestMaterialAnalysisSummariesAreBoundedAndCandidateScoped(t *testing.T) {
	s, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	require(t, err)
	defer s.Close()
	ctx := context.Background()
	for i := 0; i < 55; i++ {
		candidate := "wanted"
		if i == 54 {
			candidate = "other"
		}
		r := ma.Record{Project: "p", ID: fmt.Sprintf("%032x", i+1), Revision: 1, CreatedAt: time.Unix(int64(i), 0).UTC(), Context: ma.Context{CandidateID: candidate, Prior: json.RawMessage(`{"large":"not returned"}`)}, Input: ma.Start{Note: fmt.Sprint(i), Kind: "conflict"}, State: "review"}
		require(t, s.SaveAnalysis(ctx, r, 0))
	}
	rows, more, err := s.AnalysisSummaries(ctx, "p", "wanted")
	require(t, err)
	if !more || len(rows) != 50 || rows[0].Note != "53" || rows[len(rows)-1].Note != "4" {
		t.Fatal("wrong summary scope/order/boundary", len(rows), more)
	}
	foreign, _, err := s.AnalysisSummaries(ctx, "other", "wanted")
	require(t, err)
	if len(foreign) != 0 {
		t.Fatal("cross-project summaries returned")
	}
}

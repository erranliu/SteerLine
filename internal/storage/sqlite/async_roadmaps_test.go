package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/product/query"
	roadmap "github.com/erranliu/SteerLine/internal/roadmap/model"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/storage/ports"
)

func TestAsyncActivityRoadmapUsesCurrentPublishAdmitOrResumeCommand(t *testing.T) {
	ctx := context.Background()
	s, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "domain.db"))
	require(t, err)
	defer s.Close()
	index := 0
	for _, state := range []string{"publishing", "admission-unknown"} {
		for _, action := range []string{"roadmap_publish", "roadmap_admit", "roadmap_resume"} {
			for _, instance := range []string{"i", "old"} {
				index++
				id := fmt.Sprintf("%032x", index)
				r := roadmap.Record{Project: "p", ID: id, RequestID: id, Revision: 1, State: state, Submission: "started", CreatedAt: time.Now().UTC(), Source: roadmap.Source{ID: id}, Projection: &runnercontract.Run{StopStatus: "stopped"}, Publication: &roadmap.Publication{RequestID: "original-" + id}}
				require(t, s.SaveRoadmap(ctx, r, 0))
				for n, status := range []string{"outcome_unknown", "pending"} {
					payload, _ := json.Marshal(map[string]any{"action": action, "instance_id": instance})
					request := fmt.Sprintf("command-%s-%d", id, n)
					_, _, err = s.EnqueueCommand(ctx, ports.CommandRecord{ProjectIdentity: "p", CommandID: request, Kind: "roadmap_command", TargetID: id, IdempotencyKey: request, RequestedBy: "fixture", Payload: payload, Status: status, CreatedAt: time.Now().UTC().Add(time.Duration(n) * time.Second)})
					require(t, err)
				}
				links, err := query.ReadAsyncActivityLinks(ctx, s, "p", "i", query.AsyncActivityOptions{RequestIDs: []string{id}})
				require(t, err)
				a := links.Links[id]
				want := "waiting"
				if instance == "i" {
					want = "running"
				}
				if a.Execution.RequestID != fmt.Sprintf("command-%s-1", id) || a.Execution.Action != action || a.Execution.RunID != "" {
					t.Fatalf("operation reference differs: %+v", a.Execution)
				}
				if a.State != want || a.NeedsAttention != (instance != "i") {
					t.Fatalf("%s %s %s: %+v", state, action, instance, a)
				}
			}
		}
	}
}

func TestAsyncActivityRoadmapFactsFiltersLinksAndReopen(t *testing.T) {
	t.Run("batch settlement", testRoadmapActivityBatchSettlement)
	t.Run("batch native projection", testRoadmapActivityNativeProjection)
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "domain.db")
	s, err := OpenSQLiteStore(path)
	require(t, err)
	t.Cleanup(func() { _ = s.Close() })
	index := 0
	for _, purpose := range []string{"", "documents"} {
		for _, state := range []string{"queued", "unknown", "accepted", "rejected", "failed", "invalid", "cancelled", "blocked", "review", "confirmed", "documents-blocked", "documents-review", "publishing", "awaiting-merge", "awaiting-admission", "admission-unknown", "admitted", "future_state", ""} {
			for _, stopped := range []bool{false, true} {
				index++
				id := fmt.Sprintf("%032x", index)
				r := roadmap.Record{Project: "p", ID: id, RequestID: id, Revision: 1, Purpose: purpose, State: state, Submission: "started", CreatedAt: time.Now().UTC(), Source: roadmap.Source{ID: id, Path: "draft.md"}, Materials: []runnercontract.Document{{Path: "private-material"}}}
				if index == 1 {
					r.Source.Body = strings.Repeat("large source body", 200000)
				}
				if state == "queued" {
					r.Submission = "pending"
				}
				if state == "cancelled" {
					r.Cancelled = true
				}
				if stopped {
					r.Projection = &runnercontract.Run{Status: "completed", StopStatus: "stopped", UpdatedAt: time.Now().UTC()}
				}
				if strings.HasPrefix(state, "awaiting-") || state == "publishing" || state == "admission-unknown" || state == "admitted" {
					r.Publication = &roadmap.Publication{RequestID: "publish-" + id, State: "pr"}
					r.AdmissionRequest = "admit-" + id
				}
				require(t, s.SaveRoadmap(ctx, r, 0))
			}
		}
	}
	assertAsyncActivityFilterContract(t, s, "p", "roadmap")
	id := fmt.Sprintf("%032x", 1)
	facts, more, err := s.ReadAsyncActivityFacts(ctx, "p", ports.AsyncActivityRead{Source: "roadmap", Limit: 500, RequestIDs: []string{id}})
	require(t, err)
	if more || len(facts) != 1 || len(facts[0].Payload) > 3000 || strings.Contains(string(facts[0].Payload), "large source body") || strings.Contains(string(facts[0].Payload), "private-material") {
		t.Fatalf("unbounded or missing roadmap facts: %d %v", len(facts), more)
	}
	require(t, s.Close())
	s, err = OpenSQLiteStore(path)
	require(t, err)
	links, err := query.ReadAsyncActivityLinks(ctx, s, "p", "new-instance", query.AsyncActivityOptions{RequestIDs: []string{id}})
	require(t, err)
	if links.Links[id].SourceID != id {
		t.Fatalf("roadmap lost after reopen: %+v", links)
	}
	for _, request := range []string{"publish-" + fmt.Sprintf("%032x", index-6), "admit-" + fmt.Sprintf("%032x", index-6)} {
		links, err = query.ReadAsyncActivityLinks(ctx, s, "p", "i", query.AsyncActivityOptions{RequestIDs: []string{request}})
		require(t, err)
		if links.Links[request].Source != "roadmap" {
			t.Fatalf("missing publication/admission link: %s %+v", request, links)
		}
	}
}

func testRoadmapActivityBatchSettlement(t *testing.T) {
	ctx := context.Background()
	s, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	require(t, err)
	defer s.Close()
	id := strings.Repeat("a", 32)
	r := roadmap.Record{ID: id, Project: "p", RequestID: id, Revision: 1, Purpose: "documents", State: "cancelled", Cancelled: true, CreatedAt: time.Now().UTC(), Source: roadmap.Source{ID: "source"}}
	for i := 0; i < 2; i++ {
		r.Generations = append(r.Generations, roadmap.Generation{ID: fmt.Sprintf("%032x", i+1), Key: fmt.Sprint(i), CreatedAt: r.CreatedAt, State: "accepted", Submission: "started", Request: runnercontract.Request{RequestID: fmt.Sprint(i), Input: json.RawMessage(`{"secret":"private input"}`)}, RunID: fmt.Sprint(i), Projection: &runnercontract.Run{Revision: 1, Status: "running", StopStatus: "active"}})
	}
	require(t, s.SaveRoadmap(ctx, r, 0))
	for i := 0; i <= 2; i++ {
		links, err := query.ReadAsyncActivityLinks(ctx, s, "p", "i", query.AsyncActivityOptions{RequestIDs: []string{id}})
		require(t, err)
		a := links.Links[id]
		want := "waiting"
		if i == 2 {
			want = "cancelled"
		}
		if a.State != want || a.Execution.RunID != "" || a.Execution.AttemptID != "" {
			t.Fatalf("incorrect batch settlement %+v", a)
		}
		facts, _, err := s.ReadAsyncActivityFacts(ctx, "p", ports.AsyncActivityRead{Source: "roadmap", Limit: 10, RequestIDs: []string{id}})
		require(t, err)
		for _, f := range facts {
			if strings.Contains(string(f.Payload), "private input") {
				t.Fatal("activity includes original request")
			}
		}
		if i < 2 {
			r.Generations[i].Projection = &runnercontract.Run{Revision: 2, Status: "stopped", StopStatus: "stopped"}
			r.Revision++
			require(t, s.SaveRoadmap(ctx, r, r.Revision-1))
		}
	}
}

func testRoadmapActivityNativeProjection(t *testing.T) {
	ctx := context.Background()
	s, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	require(t, err)
	defer s.Close()
	id := strings.Repeat("a", 32)
	r := roadmap.Record{ID: id, RequestID: id, Project: "p", Revision: 1, Purpose: "documents", State: "accepted", CreatedAt: time.Now().UTC(), Source: roadmap.Source{ID: "source"}, Generations: []roadmap.Generation{{ID: strings.Repeat("b", 32), Key: "T1", CreatedAt: time.Now().UTC(), State: "accepted", Submission: "started", Request: runnercontract.Request{RequestID: "original", Input: json.RawMessage(`{"secret":"private request"}`)}, RunID: "original", Projection: &runnercontract.Run{Status: "running", ResumeRequired: true}}}}
	require(t, s.SaveRoadmap(ctx, r, 0))
	links, err := query.ReadAsyncActivityLinks(ctx, s, "p", "i", query.AsyncActivityOptions{RequestIDs: []string{id}})
	require(t, err)
	a := links.Links[id]
	if a.State != "waiting" || !a.NeedsAttention {
		t.Fatalf("SQL dropped native resume flag: %+v", a)
	}
	facts, _, err := s.ReadAsyncActivityFacts(ctx, "p", ports.AsyncActivityRead{Source: "roadmap", RequestIDs: []string{id}, Limit: 10})
	require(t, err)
	for _, f := range facts {
		if strings.Contains(string(f.Payload), "private request") {
			t.Fatal("activity exposed execution request")
		}
	}
}

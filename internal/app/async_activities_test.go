package app

import (
	"context"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	"github.com/erranliu/SteerLine/internal/product/query"
	roadmap "github.com/erranliu/SteerLine/internal/roadmap/model"
	"github.com/erranliu/SteerLine/internal/storage/ports"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
)

func assertRoadmapActivityState(t *testing.T, store *sqlite.SQLiteStore, project, request, state string, attention bool, observation string) {
	t.Helper()
	links, err := query.ReadAsyncActivityLinks(context.Background(), store, project, "fixture", query.AsyncActivityOptions{RequestIDs: []string{request}})
	if err != nil {
		t.Fatal(err)
	}
	a := links.Links[request]
	if a.State != state || a.NeedsAttention != attention || a.Observation != observation {
		t.Fatalf("roadmap activity mismatch: %+v (want %s attention=%v observation=%s)", a, state, attention, observation)
	}
}

func TestAsyncActivityReadUsesPublicIdentityAndNoActor(t *testing.T) {
	ctx := context.Background()
	s, err := sqlite.OpenSQLiteStore(filepath.Join(t.TempDir(), "domain.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	domain := `c:\owned\repo\.git`
	at := time.Now().UTC()
	_, _, err = s.EnqueueCommand(ctx, ports.CommandRecord{ProjectIdentity: domain, CommandID: "check", Kind: "project_maintenance", TargetID: domain, IdempotencyKey: "check", RequestedBy: "test", Payload: []byte(`{"action":"maintenance_check","instance_id":"instance"}`), Status: "pending", CreatedAt: at, UpdatedAt: at})
	if err != nil {
		t.Fatal(err)
	}
	p := &runtimeProjectPort{config: Config{identity: domain}, instance: "instance", store: s}
	read, err := p.Read(ctx, "/activities", url.Values{"request_ids": {"check"}})
	if err != nil {
		t.Fatal(err)
	}
	page := read.(api.AsyncActivities)
	public := string(httpcontract.ProjectID(domain))
	if page.ProjectID != public || len(page.Items) != 1 || page.Items[0].ProjectID != public || page.Links["check"].ProjectID != public {
		t.Fatalf("domain identity leaked into browser scope: %+v", page)
	}
	original, _, err := s.LookupCommand(ctx, domain, "check")
	if err != nil || original.Status != "pending" {
		t.Fatalf("observation reconciled original command: %+v %v", original, err)
	}
	if _, err = p.Read(ctx, "/activities", url.Values{"limit": {"0"}}); err == nil {
		t.Fatal("invalid limit accepted")
	}
	if _, err = p.Read(ctx, "/activities", url.Values{"filter": {"recent", "ongoing"}}); err == nil {
		t.Fatal("repeated query accepted")
	}
	linked, err := p.Read(ctx, "/activities", url.Values{"mode": {"links"}, "request_ids": {"check"}})
	if err != nil {
		t.Fatal(err)
	}
	links := linked.(api.AsyncActivityLinks)
	if len(links.Links) != 1 || links.ProjectID != public || links.Links["check"].ProjectID != public {
		t.Fatalf("association-only scope differs: %+v", links)
	}
	for _, values := range []url.Values{
		{"mode": {"bad"}}, {"mode": {"links"}},
		{"mode": {"links"}, "request_ids": {"check"}, "cursor": {"any"}},
		{"mode": {"links"}, "request_ids": {"check"}, "filter": {"ongoing"}},
		{"mode": {"links"}, "request_ids": {"check"}, "limit": {"50"}},
	} {
		if _, err = p.Read(ctx, "/activities", values); err == nil {
			t.Fatalf("ambiguous links query accepted: %v", values)
		}
	}
}

func TestAsyncActivityReadIncludesRoadmapDocumentsWithoutActor(t *testing.T) {
	ctx := context.Background()
	s, err := sqlite.OpenSQLiteStore(filepath.Join(t.TempDir(), "domain.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	id := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	r := roadmap.Record{Project: "p", ID: id, RequestID: id, Revision: 1, Purpose: "documents", State: "queued", Submission: "pending", Source: roadmap.Source{ID: "source", Path: "draft.md"}, CreatedAt: time.Now().UTC()}
	if err = s.SaveRoadmap(ctx, r, 0); err != nil {
		t.Fatal(err)
	}
	p := &runtimeProjectPort{config: Config{identity: "p"}, instance: "i", store: s}
	read, err := p.Read(ctx, "/activities", url.Values{})
	if err != nil {
		t.Fatal(err)
	}
	page := read.(api.AsyncActivities)
	if !page.CountsKnown || len(page.Items) != 1 || page.Items[0].Kind != "roadmap_documents" || page.Items[0].ProjectID != string(httpcontract.ProjectID("p")) {
		t.Fatalf("documents missing: %+v", page)
	}
	linked, err := p.Read(ctx, "/activities", url.Values{"mode": {"links"}, "request_ids": {id}})
	if err != nil {
		t.Fatal(err)
	}
	if linked.(api.AsyncActivityLinks).Links[id].SourceID != id {
		t.Fatal("roadmap request not linked")
	}
	stored, err := s.Roadmap(ctx, "p", id)
	if err != nil || stored.Revision != 1 || stored.State != "queued" {
		t.Fatalf("GET advanced roadmap: %+v %v", stored, err)
	}
}

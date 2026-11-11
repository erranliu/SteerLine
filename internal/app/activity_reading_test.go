package app

import (
	"context"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/product/query"
	roadmap "github.com/erranliu/SteerLine/internal/roadmap/model"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
)

func TestActivityReadingRoutesAreLocalAndValidateExactScope(t *testing.T) {
	ctx := context.Background()
	s, err := sqlite.OpenSQLiteStore(filepath.Join(t.TempDir(), "domain.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	id := strings.Repeat("a", 32)
	r := roadmap.Record{Project: "p", ID: id, RequestID: id, Revision: 1, State: "queued", Submission: "pending", Source: roadmap.Source{ID: id, Path: "draft.md", Body: strings.Repeat("draft", 1000)}}
	if err = s.SaveRoadmap(ctx, r, 0); err != nil {
		t.Fatal(err)
	}
	p := &runtimeProjectPort{config: Config{identity: "p"}, store: s, instance: "i"}
	list, err := p.Read(ctx, "/activities", url.Values{})
	if err != nil {
		t.Fatal(err)
	}
	activity := list.(api.AsyncActivities).Items[0]
	root := "/activities/" + activity.ID
	if _, err = p.Read(ctx, root, url.Values{}); err != nil {
		t.Fatal(err)
	}
	if _, err = p.Read(ctx, root+"/changes", url.Values{}); err != nil {
		t.Fatal(err)
	}
	read, err := p.Read(ctx, root+"/interactions", url.Values{"limit": {"1"}})
	if err != nil {
		t.Fatal(err)
	}
	page := read.(api.ReadingPage)
	if len(page.Items) != 1 {
		t.Fatal("missing draft")
	}
	item := page.Items[0]
	if _, err = p.Read(ctx, root+"/body", url.Values{"item": {item.ID}, "sha256": {item.SHA256}, "offset": {"0"}}); err != nil {
		t.Fatal(err)
	}
	for _, v := range []url.Values{{"limit": {"0"}}, {"limit": {"1", "2"}}, {"unknown": {"true"}}} {
		if _, err = p.Read(ctx, root+"/interactions", v); err == nil {
			t.Fatalf("bad query accepted: %v", v)
		}
	}
	other := &runtimeProjectPort{config: Config{identity: "other"}, store: s, instance: "i"}
	if _, err = other.Read(ctx, root+"/interactions", url.Values{}); err == nil {
		t.Fatal("cross-project route accepted")
	}
	stored, err := s.Roadmap(ctx, "p", id)
	if err != nil || stored.Revision != 1 || stored.State != "queued" {
		t.Fatal("GET advanced owner")
	}
	if _, _, err = query.DecodeActivityID("p", activity.ID); err != nil {
		t.Fatal(err)
	}
}

package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"testing"

	"github.com/erranliu/SteerLine/internal/product/httpclient"
	pm "github.com/erranliu/SteerLine/internal/projectmaintenance/model"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

func TestTaskReadinessWithoutDocumentReads(t *testing.T) {
	ctx := context.Background()
	store := &pmsStore{materials: map[string]pm.Material{
		"101": {TaskID: "101", Commit: "original", Status: "accepted"},
	}}
	// No repository files, Git command or process host exist. Status reads must
	// still work and must not depend on the full-document query succeeding.
	m := newProjectMaintenance(Config{}, store)
	m.observation.Commit = "original"
	p := &runtimeProjectPort{config: Config{runtimeMaintenance: m}}
	read := func() taskPreparation {
		t.Helper()
		value, err := p.Read(ctx, "/maintenance/tasks/101", url.Values{"view": {"status"}})
		if err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil || fields["files"] != nil {
			t.Fatalf("status query included documents: %s, %v", raw, err)
		}
		return value.(taskPreparation)
	}
	if got := read(); got.Status != "accepted" || got.Commit != "original" {
		t.Fatal(got)
	}
	store.materials["101"] = pm.Material{TaskID: "101", Commit: "original", Status: "ready"}
	if got := read(); got.Status != "ready" {
		t.Fatal(got)
	}
	m.observation.Commit = "newer"
	if got := read(); got.Status != "unknown" {
		t.Fatal("old material remained ready after the remote changed", got)
	}
	m.observation.Commit = "original"
	m.observation.Diagnostic = "remote unavailable"
	if got := read(); got.Status != "unknown" {
		t.Fatal("failed observation kept cached readiness", got)
	}
	if _, err := p.Read(ctx, "/maintenance/tasks/missing", url.Values{"view": {"status"}}); !errors.Is(err, storageports.ErrNotFound) {
		t.Fatal("missing task reused another task's readiness", err)
	}
}

func TestTaskReadinessRejectsUnsupportedQueries(t *testing.T) {
	p := &runtimeProjectPort{}
	for _, query := range []url.Values{
		{"view": {"status", "status"}},
		{"view": {"documents"}},
		{"view": {"status"}, "extra": {"true"}},
		{"page": {"1"}},
	} {
		_, err := p.Read(context.Background(), "/maintenance/tasks/101", query)
		var rejected *httpclient.ClientError
		if !errors.As(err, &rejected) || rejected.Status != 400 || rejected.Code != "invalid_query" {
			t.Fatalf("unsupported query %v was not rejected: %v", query, err)
		}
	}
}

func TestTaskReadinessKnownTaskWithoutMaterialIsUnprepared(t *testing.T) {
	m, store := pmsFixture(t)
	m.config.Git.Executable = "unavailable-git"
	got, err := m.taskReadiness(context.Background(), "101")
	if err != nil || got.Status != "unprepared" || got.Diagnostic == "" || len(store.materials) != 0 {
		t.Fatal("a read performed preparation or treated a missing material as an unknown task", got, err)
	}
}

package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	rs "github.com/erranliu/SteerLine/internal/runnerstate"
	"path/filepath"
	"testing"
	"time"
)

func TestEnvironmentCheckStoreCASAndIsolation(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	db, err := OpenSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	f := struct {
		Input        runnercontract.CheckInput
		Capabilities runnercontract.CheckCapabilities
	}{Input: runnercontract.CheckInput{Publication: &runnercontract.Publication{Repository: "fixture", Settings: json.RawMessage(`{"opaque_revision":9007199254740992}`)}}, Capabilities: runnercontract.CheckCapabilities{Schema: json.RawMessage(`{ "type": "object" }`)}}
	r := rs.EnvironmentCheckRecord{ID: "check", Project: "project", Revision: 1, Input: f.Input, Capabilities: runnercontract.CheckCapabilities(f.Capabilities), CreatedAt: time.Now().UTC(), Operations: []rs.EnvironmentCheckOperation{{RequestID: "request", Original: json.RawMessage(`{ "a": 1 }`), Request: runnercontract.Request{Protocol: runnercontract.Protocol, RequestID: "request", Input: json.RawMessage(`{ "x": 2 }`)}, Status: "outcome_unknown"}}}
	if err = db.SaveCheck(ctx, r, 0); err != nil {
		t.Fatal(err)
	}
	if err = db.SaveCheck(ctx, r, 0); err == nil {
		t.Fatal("duplicate create accepted")
	}
	r.Revision++
	r.CheckID = "native"
	r.Operations[0].Status = "applied"
	if err = db.SaveCheck(ctx, r, 1); err != nil {
		t.Fatal("JSON whitespace caused false immutable conflict", err)
	}
	if err = db.SaveCheck(ctx, r, 1); err == nil {
		t.Fatal("stale CAS accepted")
	}
	r.Revision++
	r.StateDir = "other-runner-directory"
	if err = db.SaveCheck(ctx, r, 2); err == nil {
		t.Fatal("immutable Runner directory changed")
	}
	r.StateDir = ""
	r.Input.Publication = nil
	if err = db.SaveCheck(ctx, r, 2); err == nil {
		t.Fatal("publication removed")
	}
	r.Input.Publication = &runnercontract.Publication{Repository: "fixture", Settings: json.RawMessage(`{"opaque_revision":9007199254740993}`)}
	if err = db.SaveCheck(ctx, r, 2); err == nil {
		t.Fatal("immutable public settings rounded through float64")
	}
	if _, err = db.CheckRequest(ctx, "other", "request"); !errors.Is(err, rs.ErrEnvironmentCheckNotFound) {
		t.Fatal("scope leak", err)
	}
	var tasks, candidates int
	if err = db.db.QueryRow(`SELECT count(*) FROM tasks`).Scan(&tasks); err != nil {
		t.Fatal(err)
	}
	if err = db.db.QueryRow(`SELECT count(*) FROM runner_candidates`).Scan(&candidates); err != nil {
		t.Fatal(err)
	}
	if tasks != 0 || candidates != 0 {
		t.Fatal("check produced business work")
	}
	_ = db.Close()
	db, err = OpenSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	got, err := db.CheckRequest(ctx, "project", "request")
	if err != nil || got.CheckID != "native" || got.Revision != 2 {
		t.Fatal("restart association lost", got, err)
	}
}

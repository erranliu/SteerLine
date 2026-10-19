package environmentcheck_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/erranliu/SteerLine/internal/runnerclient"
)

func TestSharedRunnerChecksUseSavedExternalIdentity(t *testing.T) {
	ctx := context.Background()
	a, ar, _ := setup(t)
	b, br, _ := setup(t)
	a.StateDir = "shared"
	b.StateDir = "shared"
	b.Project = "other-project"
	ar.callErr = &runnerclient.Failure{Unknown: true, Code: "request_outcome_unknown"}
	br.callErr = ar.callErr
	first, err := a.Submit(ctx, start())
	if err != nil {
		t.Fatal(err)
	}
	other := start()
	other.TargetID = b.Project
	second, err := b.Submit(ctx, other)
	if err != nil {
		t.Fatal(err)
	}
	x, y := first.Operations[0].Request.RequestID, second.Operations[0].Request.RequestID
	if x == y || x == start().RequestID || y == start().RequestID {
		t.Fatal("checks share a product/external ID")
	}
	if _, err = a.Submit(ctx, start()); err != nil {
		t.Fatal(err)
	}
	if ar.starts != 1 || len(ar.inspected) != 1 || ar.inspected[0] != "request-id:"+x {
		t.Fatal("unknown request was resent or looked up by product ID", ar.starts, ar.inspected)
	}
	a.StateDir = "other"
	if _, err = a.Detail(ctx, first.ID); err == nil {
		t.Fatal("check rebound to another service directory")
	}
}

func TestSharedRunnerRestartVerifiesChecksReadOnly(t *testing.T) {
	for _, kind := range []string{"bound", "unknown_start", "unknown_absent", "wrong_check", "wrong_input", "wrong_command", "offline", "other_directory"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			s, r, store := setup(t)
			s.StateDir = "original-directory"
			unknown := kind == "unknown_start" || kind == "unknown_absent" || kind == "wrong_command"
			if unknown {
				r.callErr = &runnerclient.Failure{Unknown: true, Code: "request_outcome_unknown"}
			}
			record, err := s.Submit(ctx, start())
			if err != nil {
				t.Fatal(err)
			}
			saved, err := store.Check(ctx, s.Project, record.ID)
			if err != nil {
				t.Fatal(err)
			}
			before, _ := json.Marshal(saved)
			switch kind {
			case "unknown_absent":
				r.inspectErr = &runnerclient.Failure{Code: "request_not_found"}
			case "wrong_check":
				r.f.Inspection.Check.CheckID = "other-check"
			case "wrong_input":
				r.f.Inspection.Check.Input.Repository.URL = "other-repository"
			case "wrong_command":
				r.f.Inspection.Command.Command = "environment-check stop"
			case "offline":
				r.inspectErr = errors.New("offline")
			case "other_directory":
				s.StateDir = "other-directory"
			}
			err = s.VerifyBindings(ctx)
			pass := kind == "bound" || kind == "unknown_start" || kind == "unknown_absent"
			if (err == nil) != pass {
				t.Fatal("startup binding", err)
			}
			if kind == "bound" && !reflect.DeepEqual(r.inspected, []string{"check-id:" + record.CheckID}) {
				t.Fatal(r.inspected)
			}
			if unknown && !reflect.DeepEqual(r.inspected, []string{"request-id:" + record.Operations[0].Request.RequestID}) {
				t.Fatal(r.inspected)
			}
			after, err := store.Check(ctx, s.Project, record.ID)
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(after)
			if string(before) != string(raw) || r.starts != 1 || r.stops != 0 {
				t.Fatal("startup changed saved facts or dispatched input")
			}
		})
	}
}

func TestSharedRunnerCheckStopRejectsOtherDirectory(t *testing.T) {
	ctx := context.Background()
	s, r, _ := setup(t)
	s.StateDir = "original"
	if _, err := s.Submit(ctx, start()); err != nil {
		t.Fatal(err)
	}
	r.f.Inspection.Check.CleanupStatus = "pending"
	r.f.Inspection.Check.AllowedActions = []string{"stop"}
	record, err := s.Detail(ctx, start().RequestID)
	if err != nil {
		t.Fatal(err)
	}
	s.StateDir = "other"
	if _, err := s.Submit(ctx, stop(record.Report.Revision)); err == nil || r.stops != 0 {
		t.Fatal("stop crossed Runner directories", err)
	}
}

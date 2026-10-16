package environmentcheck_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	ec "github.com/erranliu/SteerLine/internal/environmentcheck"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	"github.com/erranliu/SteerLine/internal/testkit/checkfixture"
)

type runner struct {
	f                         checkfixture.Fixture
	starts, stops, admissions int
	inspected                 []string
	callErr, inspectErr       error
	inspectErrors             map[string]error
	before                    func(runnerclient.Request)
}

func (r *runner) EnvironmentCheckCatalog(context.Context) (*runnerclient.Catalog, error) {
	return &runnerclient.Catalog{Capabilities: runnerclient.Capabilities{ResourceDigest: r.f.ResourceDigest, EnvironmentCheck: &r.f.Capabilities}}, nil
}
func (r *runner) StartEnvironmentCheck(_ context.Context, q runnerclient.Request) (runnerclient.CheckReceipt, error) {
	r.starts++
	if r.before != nil {
		r.before(q)
	}
	return r.f.Receipt, r.callErr
}
func (r *runner) StopEnvironmentCheck(_ context.Context, q runnerclient.Request) (runnerclient.CheckReceipt, error) {
	r.stops++
	if r.before != nil {
		r.before(q)
	}
	receipt := r.f.Receipt
	receipt.CommandID = "stop-command"
	return receipt, r.callErr
}
func (r *runner) InspectEnvironmentCheck(_ context.Context, selector, id string) (runnerclient.CheckInspection, error) {
	r.inspected = append(r.inspected, selector+":"+id)
	if err := r.inspectErrors[id]; err != nil {
		return runnerclient.CheckInspection{}, err
	}
	if r.inspectErr != nil {
		return runnerclient.CheckInspection{}, r.inspectErr
	}
	b, _ := json.Marshal(r.f.Inspection)
	var v runnerclient.CheckInspection
	_ = json.Unmarshal(b, &v)
	if selector == "check-id" {
		v.Command = nil
		return v, nil
	}
	v.Command.RequestID = id
	if id != strings.Repeat("1", 32) {
		v.Command.Command = "environment-check stop"
		v.Command.CommandID = "stop-command"
	}
	return v, nil
}
func setup(t *testing.T) (*ec.Service, *runner, *sqlite.SQLiteStore) {
	t.Helper()
	store, err := sqlite.OpenSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	r := &runner{f: checkfixture.Load()}
	s := &ec.Service{Project: "fixture-project", Store: store, Selection: runnerclient.Selection{Executable: "fixture.exe", Version: "draft-fixture", SHA256: strings.Repeat("d", 64)}, Profiles: []ec.Profile{{ID: "fixture", Name: "Fixture", Execution: r.f.Input.Execution}}}
	s.Admit = func(context.Context) error { r.admissions++; return nil }
	s.Open = func(context.Context, runnerclient.Selection) (ec.Runner, error) { return r, nil }
	s.ResolveInput = func(_ context.Context, e runnerclient.ExecutionInput) (runnerclient.CheckInput, error) {
		input := r.f.Input
		input.Execution = e
		return input, nil
	}
	return s, r, store
}
func start() ec.Command {
	return ec.Command{RequestID: strings.Repeat("1", 32), Action: ec.StartAction, TargetID: "fixture-project", Payload: json.RawMessage(`{"profile_id":"fixture","access_mode":"sandbox"}`), Original: json.RawMessage(`{"request_id":"11111111111111111111111111111111","action":"start"}`)}
}
func stop(revision uint64) ec.Command {
	return ec.Command{RequestID: strings.Repeat("2", 32), Action: ec.StopAction, TargetID: strings.Repeat("1", 32), ExpectedRevision: revision, Payload: json.RawMessage(`{}`), Original: json.RawMessage(`{"request_id":"22222222222222222222222222222222","action":"stop"}`)}
}

func TestEnvironmentCheckDurableUnknownAndRestart(t *testing.T) {
	ctx := context.Background()
	s, r, store := setup(t)
	q := start()
	r.callErr = &runnerclient.Failure{Code: "request_outcome_unknown", Unknown: true, Exit: 6}
	r.before = func(request runnerclient.Request) {
		saved, err := store.CheckRequest(ctx, s.Project, request.RequestID)
		if err != nil || saved.Operations[0].Status != "outcome_unknown" || request.RequestID != q.RequestID {
			t.Fatal("mutation preceded durable intent", saved, err)
		}
	}
	rec, err := s.Submit(ctx, q)
	if err != nil || rec.Revision != 2 || rec.CheckID != "" {
		t.Fatal(rec, err)
	}
	if _, err = s.Submit(ctx, stop(1)); err == nil || r.stops != 0 {
		t.Fatal("guessed stop allowed", err)
	}
	other := q
	other.RequestID = strings.Repeat("4", 32)
	other.Original = json.RawMessage(`{"action":"another-start"}`)
	prepared := r.admissions
	if _, err = s.Submit(ctx, other); err == nil || r.starts != 1 || r.admissions != prepared {
		t.Fatal("unresolved check allowed a replacement service/check", err)
	}
	// Recreated caller uses only the stored original pin/request.
	restarted := &ec.Service{Project: s.Project, Store: store, Selection: runnerclient.Selection{Executable: "other.exe"}, Open: func(_ context.Context, p runnerclient.Selection) (ec.Runner, error) {
		if p != s.Selection {
			t.Fatal("changed pinned Runner")
		}
		return r, nil
	}}
	rec, err = restarted.Submit(ctx, q)
	if err != nil || r.starts != 1 || rec.Report == nil || rec.Report.CapabilityStatus != "failed" {
		t.Fatal(rec, err, r.starts)
	}
	if rec.Input.Publication == nil || rec.Input.Publication.Repository != "fixture/repo" || rec.Input.Repository.BaseBranch != "main" {
		t.Fatal("project publication omitted", rec.Input)
	}
	if rec.Report.InputDigest != strings.Repeat("c", 64) {
		t.Fatal("authoritative digest was recomputed")
	}
	changed := q
	changed.Original = json.RawMessage(`{"action":"other"}`)
	if _, err = restarted.Submit(ctx, changed); err == nil {
		t.Fatal("immutable request changed")
	}
	if _, err = store.Check(ctx, "other-project", rec.ID); !errors.Is(err, ec.ErrNotFound) {
		t.Fatal("cross-project read", err)
	}
	r.inspectErr = errors.New("offline")
	cached, err := restarted.Detail(ctx, rec.ID)
	if err != nil || cached.Report.UpdatedAt != rec.Report.UpdatedAt || cached.Diagnostic == "" {
		t.Fatal("offline cache lost", cached, err)
	}
}
func TestEnvironmentCheckSnapshotGuards(t *testing.T) {
	for _, kind := range []string{"lower", "same-conflict", "digest", "missing", "context", "publication", "command", "manifest"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			s, r, _ := setup(t)
			_, _ = s.Submit(ctx, start())
			r.f.Inspection.Check.Revision = 2
			good, err := s.Detail(ctx, start().RequestID)
			if err != nil || good.Report == nil {
				t.Fatal(good, err)
			}
			switch kind {
			case "lower":
				r.f.Inspection.Check.Revision = 1
			case "same-conflict":
				r.f.Inspection.Check.Status = "changed"
			case "digest":
				r.f.Inspection.Check.Revision++
				r.f.Inspection.Check.InputDigest = strings.Repeat("f", 64)
			case "missing":
				r.f.Inspection.Check.Items = nil
			case "context":
				r.f.Inspection.Check.Context.Repository.URL = "other"
			case "publication":
				r.f.Inspection.Check.CapabilityStatus = "passed"
				for i := range r.f.Inspection.Check.Items {
					r.f.Inspection.Check.Items[i].Status = "passed"
				}
			case "command":
				r.f.Inspection.Command.Command = "environment-check stop"
			case "manifest":
				r.f.Inspection.Check.Revision++
				r.f.Inspection.Check.RequiredChecks = []string{"permissions"}
			}
			got, err := s.Request(ctx, start().RequestID)
			if err != nil || got.Report.Revision != 2 || got.Report.Status != good.Report.Status || got.Diagnostic == "" {
				t.Fatal("untrusted snapshot replaced cache", got, err)
			}
		})
	}
}
func TestEnvironmentCheckStopIdentityAndConcurrency(t *testing.T) {
	ctx := context.Background()
	s, r, store := setup(t)
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.Submit(ctx, start())
			if err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if r.starts != 1 {
		t.Fatal("duplicate dispatch", r.starts)
	}
	r.f.Inspection.Check.Resources.Status = "pending"
	r.f.Inspection.Check.Controls.CanStop = true
	r.f.Inspection.Check.Revision++
	rec, _ := s.Detail(ctx, start().RequestID)
	r.before = func(q runnerclient.Request) {
		saved, err := store.CheckRequest(ctx, s.Project, q.RequestID)
		if err != nil || saved.Operations[len(saved.Operations)-1].Status != "outcome_unknown" {
			t.Fatal("stop not saved", err)
		}
	}
	stopped, err := s.Submit(ctx, stop(rec.Report.Revision))
	if err != nil || r.stops != 1 || len(stopped.Operations) != 2 {
		t.Fatal(stopped, err)
	}
	repeated := stop(rec.Report.Revision)
	repeated.RequestID = strings.Repeat("3", 32)
	repeated.Original = json.RawMessage(`{"action":"another-stop"}`)
	if _, err = s.Submit(ctx, repeated); err == nil || r.stops != 1 {
		t.Fatal("same revision redispatched", err)
	}
	r.f.Inspection.Check.Revision++
	_, err = s.Request(ctx, stop(1).RequestID)
	if err != nil || r.inspected[len(r.inspected)-1] != "request-id:"+stop(1).RequestID {
		t.Fatal("stop queried as start", err, r.inspected)
	}
	if _, err = s.Submit(ctx, stop(rec.Report.Revision)); err != nil || r.stops != 1 {
		t.Fatal("duplicate stop replayed", err)
	}
}
func TestEnvironmentCheckCapabilityAndRejection(t *testing.T) {
	ctx := context.Background()
	s, r, _ := setup(t)
	_ = s.Catalog(ctx)
	_, _ = s.List(ctx)
	if r.admissions != 0 || r.starts != 0 {
		t.Fatal("read started service")
	}
	r.f.Capabilities.Supported = nil
	if _, err := s.Submit(ctx, start()); err == nil || r.starts != 0 {
		t.Fatal("missing capability dispatched", err)
	}
	r.f = checkfixture.Load()
	r.callErr = &runnerclient.Failure{Code: "invalid_request", Exit: 2}
	rec, err := s.Submit(ctx, start())
	if err != nil || rec.Operations[0].Status != "rejected" {
		t.Fatal(rec, err)
	}
	_, _ = s.Detail(ctx, rec.ID)
	if len(r.inspected) != 0 {
		t.Fatal("known rejection sent again")
	}
}

func TestEnvironmentCheckStopRecoveryDoesNotDependOnStartHistory(t *testing.T) {
	ctx := context.Background()
	s, r, _ := setup(t)
	_, err := s.Submit(ctx, start())
	if err != nil {
		t.Fatal(err)
	}
	r.f.Inspection.Check.Resources.Status = "pending"
	r.f.Inspection.Check.Controls.CanStop = true
	current, err := s.Detail(ctx, start().RequestID)
	if err != nil || current.Report == nil {
		t.Fatal(current, err)
	}
	r.callErr = &runnerclient.Failure{Code: "request_outcome_unknown", Unknown: true, Exit: 6}
	if _, err = s.Submit(ctx, stop(current.Report.Revision)); err != nil {
		t.Fatal(err)
	}
	r.inspectErrors = map[string]error{start().RequestID: errors.New("old start receipt unavailable")}
	r.f.Inspection.Check.Revision++
	r.f.Inspection.Check.Resources.Status = "settled"
	r.f.Inspection.Check.Controls.CanStop = false
	r.inspected = nil
	recovered, err := s.Request(ctx, stop(1).RequestID)
	if err != nil || recovered.Diagnostic != "" || recovered.Operations[1].Status != "applied" || recovered.Report.Resources.Status != "settled" || len(r.inspected) != 1 || r.inspected[0] != "request-id:"+stop(1).RequestID {
		t.Fatal("old start blocked original stop recovery", recovered, err, r.inspected)
	}
	r.inspected = nil
	for range 3 {
		recovered, err = s.Detail(ctx, start().RequestID)
		if err != nil || recovered.Diagnostic != "" {
			t.Fatal(recovered, err)
		}
	}
	if len(r.inspected) != 3 {
		t.Fatal("history commands polled with each detail", r.inspected)
	}
	for _, selector := range r.inspected {
		if selector != "check-id:check-fixture" {
			t.Fatal("detail queried unrelated historic command", r.inspected)
		}
	}
	if r.starts != 1 || r.stops != 1 {
		t.Fatal("query replayed a mutation", r.starts, r.stops)
	}
}

func TestEnvironmentCheckUnknownDetailQueriesOriginalRequest(t *testing.T) {
	ctx := context.Background()
	s, r, _ := setup(t)
	r.callErr = &runnerclient.Failure{Code: "request_outcome_unknown", Unknown: true, Exit: 6}
	_, err := s.Submit(ctx, start())
	if err != nil {
		t.Fatal(err)
	}
	row, err := s.Detail(ctx, start().RequestID)
	if err != nil || row.CheckID == "" || len(r.inspected) != 1 || r.inspected[0] != "request-id:"+start().RequestID {
		t.Fatal(row, err, r.inspected)
	}
	_, err = s.Detail(ctx, row.ID)
	if err != nil || r.inspected[len(r.inspected)-1] != "check-id:check-fixture" || r.starts != 1 {
		t.Fatal("binding did not switch to read-only check snapshot", err, r.inspected)
	}
}

func TestEnvironmentCheckPreservesRunnerVerdictAndFrozenManifest(t *testing.T) {
	ctx := context.Background()
	s, r, _ := setup(t)
	_, err := s.Submit(ctx, start())
	if err != nil {
		t.Fatal(err)
	}
	r.f.Inspection.Check.CapabilityStatus = "unverified"
	for i := range r.f.Inspection.Check.Items {
		r.f.Inspection.Check.Items[i].Status = "passed"
	}
	row, err := s.Detail(ctx, start().RequestID)
	if err != nil || row.Report == nil || row.Report.CapabilityStatus != "unverified" {
		t.Fatal("caller recomputed native verdict", row, err)
	}
	// Ordering is not policy; the published required set remains the same.
	r.f.Inspection.Check.Revision++
	required := r.f.Inspection.Check.RequiredChecks
	required[0], required[1] = required[1], required[0]
	row, err = s.Detail(ctx, row.ID)
	if err != nil || row.Diagnostic != "" || row.Report.Revision != 2 {
		t.Fatal(row, err)
	}
}

func TestEnvironmentCheckImmutableLargeJSONNumbers(t *testing.T) {
	ctx := context.Background()
	s, r, _ := setup(t)
	q := start()
	q.Original = json.RawMessage(`{"opaque_revision":9007199254740992}`)
	if _, err := s.Submit(ctx, q); err != nil {
		t.Fatal(err)
	}
	q.Original = json.RawMessage(`{"opaque_revision":9007199254740993}`)
	if _, err := s.Submit(ctx, q); err == nil || r.starts != 1 {
		t.Fatal("immutable integers rounded to the same envelope", err, r.starts)
	}
}

func TestEnvironmentCheckAdmissionFailureDoesNotStartOrReplaceRunner(t *testing.T) {
	ctx := context.Background()
	s, r, _ := setup(t)
	s.Admit = func(context.Context) error { return errors.New("original Runner unavailable") }
	if _, err := s.Submit(ctx, start()); err == nil || r.starts != 0 {
		t.Fatal("start bypassed original instance admission", err, r.starts)
	}
	s.Admit = func(context.Context) error { return nil }
	_, err := s.Submit(ctx, start())
	if err != nil {
		t.Fatal(err)
	}
	r.f.Inspection.Check.Resources.Status = "pending"
	r.f.Inspection.Check.Controls.CanStop = true
	row, err := s.Detail(ctx, start().RequestID)
	if err != nil || row.Report == nil {
		t.Fatal(row, err)
	}
	s.Admit = func(context.Context) error { return errors.New("original Runner unavailable") }
	if _, err = s.Submit(ctx, stop(row.Report.Revision)); err == nil || r.stops != 0 {
		t.Fatal("stop bypassed original instance admission", err)
	}
	r.inspected = nil
	if _, err = s.Request(ctx, start().RequestID); err != nil || len(r.inspected) != 1 {
		t.Fatal("original read incorrectly required new input admission", err)
	}
}

type failStore struct {
	ec.Store
	failAt uint64
}

func (s *failStore) SaveCheck(ctx context.Context, r ec.Record, expected uint64) error {
	if r.Revision == s.failAt {
		return errors.New("injected commit failure")
	}
	return s.Store.SaveCheck(ctx, r, expected)
}
func TestEnvironmentCheckCommitWindows(t *testing.T) {
	for _, revision := range []uint64{1, 2} {
		t.Run(string(rune('0'+revision)), func(t *testing.T) {
			ctx := context.Background()
			s, r, store := setup(t)
			s.Store = &failStore{Store: store, failAt: revision}
			if _, err := s.Submit(ctx, start()); err == nil {
				t.Fatal("injected failure lost")
			}
			if revision == 1 {
				if r.starts != 0 {
					t.Fatal("unsaved intent dispatched")
				}
				return
			}
			saved, err := store.CheckRequest(ctx, s.Project, start().RequestID)
			if err != nil || saved.CheckID != "" || saved.Operations[0].Status != "outcome_unknown" {
				t.Fatal("uncertain intent not retained", saved, err)
			}
			s.Store = store
			got, err := s.Submit(ctx, start())
			if err != nil || got.CheckID != "check-fixture" || r.starts != 1 {
				t.Fatal("commit failure replayed external success", got, err)
			}
		})
	}
}

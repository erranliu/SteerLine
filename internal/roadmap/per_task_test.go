package roadmap

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/roadmap/model"
	"github.com/erranliu/SteerLine/internal/runnerbinding"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

type batchPeer struct {
	*roadmapRunner
	runs     map[string]runnercontract.Run
	requests map[string]string
	stopped  []string
}

func (f *batchPeer) Submit(ctx context.Context, q runnercontract.Request) (runnerclient.Receipt, error) {
	receipt, err := f.roadmapRunner.Submit(ctx, q)
	run := f.run
	run.RunID = q.RequestID
	run.InputDigest = model.Digest(q.Input)
	run.Status = "running"
	run.StopStatus = "running"
	run.Execution.AttemptID = "attempt-" + q.RequestID
	run.Execution.SessionID = "session-" + q.RequestID
	f.runs[run.RunID] = run
	f.requests[q.RequestID] = run.RunID
	receipt.RunID = run.RunID
	return receipt, err
}
func (f *batchPeer) Inspect(_ context.Context, selector, id string) (runnerclient.Inspection, error) {
	if selector == "request-id" {
		id = f.requests[id]
	}
	run, ok := f.runs[id]
	if !ok {
		return runnerclient.Inspection{}, nil
	}
	return runnerclient.Inspection{Run: &run, Command: &runnerclient.Command{Command: "submit", RequestID: id, RunID: &run.RunID}}, nil
}
func (f *batchPeer) Call(_ context.Context, command string, args []string, _ *runnercontract.Request) (runnerclient.Response, error) {
	if command != "stop" {
		return runnerclient.Response{}, fmt.Errorf("unexpected command")
	}
	run := f.runs[args[1]]
	run.StopStatus = "stopped"
	run.Revision++
	f.runs[run.RunID] = run
	f.stopped = append(f.stopped, run.RunID)
	raw, _ := json.Marshal(runnerclient.Receipt{RunID: run.RunID, CommandID: "stop", ReceiptStatus: "accepted"})
	return runnerclient.Response{Data: raw}, nil
}
func batchSetup(t *testing.T, count, limit int) (*Service, *memoryRoadmaps, *batchPeer, model.Record) {
	t.Helper()
	s, store, peer, parent := documentSetup(t)
	a := *parent.Analysis
	a.Tasks = append([]model.Node(nil), a.Tasks...)
	for len(a.Tasks) < count {
		n := a.Tasks[0]
		n.ProvidesTests = []string{}
		n.Key = fmt.Sprintf("task-%d", len(a.Tasks))
		n.Evidence = n.Key
		parent.Source.Body += "\n" + n.Key
		a.Tasks = append(a.Tasks, n)
	}
	a.Tasks = a.Tasks[:count]
	parent.Analysis = &a
	parent.Source.Digest = model.Digest([]byte(parent.Source.Body))
	parent.ConfirmedDigest = model.ConfirmationDigest(parent)
	store.rows[parent.ID] = model.StoreValue(parent)
	s.Source = func(context.Context, string) (model.Source, error) { return parent.Source, nil }
	b := &batchPeer{roadmapRunner: peer, runs: map[string]runnercontract.Run{}, requests: map[string]string{}}
	s.Runner = b
	s.Queue = runnerbinding.NewContentQueue(limit)
	return s, store, b, parent
}
func observeBatch(t *testing.T, s *Service, id string) model.Record {
	t.Helper()
	r, e := s.Observe(context.Background(), id)
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func finishGeneration(t *testing.T, s *Service, f *batchPeer, r model.Record, g model.Generation, failed bool) {
	t.Helper()
	run := f.runs[g.RunID]
	run.Revision++
	run.Status = "completed"
	run.StopStatus = "stopped"
	if failed {
		run.Status = "failed"
		run.ContentResult = &runnercontract.ContentResult{Status: "failed", Diagnostic: "invalid_json: invalid character after top-level value"}
	} else {
		view := r.Execution(g)
		out, source := documentResult(view)
		source.LocalPath = s.Repository
		selected := []model.Documents{}
		for _, d := range out.Documents {
			if d.Key == g.Key {
				selected = append(selected, d)
			}
		}
		out.Documents = selected
		raw, e := json.Marshal(out)
		if e != nil {
			t.Fatal(e)
		}
		run.RepositorySource = source
		run.ContentResult = &runnercontract.ContentResult{Status: "succeeded", JSON: raw, RepositorySource: source, FileMaterials: run.Materials, Source: runnercontract.Final{Verified: true, NativeOutcome: "succeeded", CommandID: "command", TurnID: "turn"}}
	}
	f.runs[run.RunID] = run
}
func TestRoadmapPerTaskConcurrencyAndRetry(t *testing.T) {
	s, store, f, parent := batchSetup(t, 6, 5)
	ctx := context.Background()
	r, e := s.GenerateDocuments(ctx, parent.ID, strings.Repeat("d", 32), parent.Revision)
	if e != nil {
		t.Fatal(e)
	}
	if len(r.Generations) != 6 || r.RunID != "" || len(f.runs) != 0 {
		t.Fatal("batch was not atomically queued", r)
	}
	var shared []runnercontract.Document
	for _, g := range r.Generations {
		var input runnercontract.GenerateRunInput
		if err := json.Unmarshal(g.Request.Input, &input); err != nil {
			t.Fatal(err)
		}
		var schema map[string]any
		if err := json.Unmarshal(input.Generate.OutputSchema, &schema); err != nil {
			t.Fatal(err)
		}
		documents := schema["oneOf"].([]any)[0].(map[string]any)["properties"].(map[string]any)["documents"].(map[string]any)
		entries := documents["prefixItems"].([]any)
		if len(entries) != 1 || documents["maxItems"] != float64(1) || entries[0].(map[string]any)["properties"].(map[string]any)["local_key"].(map[string]any)["const"] != g.Key {
			t.Fatal("schema still requests a batch")
		}
		if shared == nil {
			shared = input.Generate.Materials
		} else if !reflect.DeepEqual(shared, input.Generate.Materials) {
			t.Fatal("siblings use different planning/source snapshots")
		}
	}
	r = observeBatch(t, s, r.ID)
	if len(f.runs) != 5 {
		t.Fatalf("want five simultaneously active sessions, got %d", len(f.runs))
	}
	var first model.Generation
	for _, g := range r.Generations {
		if g.RunID != "" {
			first = g
			break
		}
	}
	finishGeneration(t, s, f, r, first, false)
	r = observeBatch(t, s, r.ID)
	r = observeBatch(t, s, r.ID)
	if len(f.runs) != 6 {
		t.Fatal("released slot did not dispatch sixth task")
	}
	failedKey := ""
	var failedInput runnercontract.GenerateRunInput
	for _, g := range r.Generations {
		if g.ID != first.ID {
			failedKey = g.Key
			if err := json.Unmarshal(g.Request.Input, &failedInput); err != nil {
				t.Fatal(err)
			}
			finishGeneration(t, s, f, r, g, true)
			break
		}
	}
	r = observeBatch(t, s, r.ID)
	// A settled failure can be retried while siblings remain running.
	prepare := s.Prepare
	s.Prepare = func(ctx context.Context) (runnercontract.ExecutionInput, *runnerclient.Catalog, error) {
		execution, catalog, err := prepare(ctx)
		execution.Settings = json.RawMessage(`{"access_mode":"read_only","model":"new-steer-agent","reasoning_effort":"medium"}`)
		return execution, catalog, err
	}
	retryID := strings.Repeat("e", 32)
	r, e = s.RetryDocuments(ctx, r.ID, failedKey, retryID, r.Revision)
	if e != nil {
		t.Fatal(e)
	}
	if len(r.Generations) != 7 {
		t.Fatal("retry did not append one execution")
	}
	var retriedInput runnercontract.GenerateRunInput
	if err := json.Unmarshal(r.Generations[6].Request.Input, &retriedInput); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(retriedInput.Execution.Settings), "new-steer-agent") ||
		!reflect.DeepEqual(retriedInput.Generate, failedInput.Generate) {
		t.Fatal("retry did not use the current settings with the original generation materials")
	}
	replay, e := s.RetryDocuments(ctx, r.ID, failedKey, retryID, r.Revision)
	if e != nil || len(replay.Generations) != 7 {
		t.Fatal("retry was not idempotent", e)
	}
	if _, e = s.RetryDocuments(ctx, r.ID, first.Key, retryID, r.Revision); e == nil {
		t.Fatal("same request changed task")
	}
	r = observeBatch(t, s, r.ID)
	for _, g := range r.CurrentGenerations() {
		if g.ID != first.ID {
			finishGeneration(t, s, f, r, g, false)
		}
	}
	r = observeBatch(t, s, r.ID)
	if r.State != "documents-review" || len(r.Documents) != 6 || !r.DocumentsVerified || !r.ObservationComplete() {
		t.Fatalf("incomplete adoption: %s %s", r.State, r.Diagnostic)
	}
	for _, g := range r.CurrentGenerations() {
		if g.RunID == "" || g.Projection == nil || g.Projection.Execution.AttemptID == "" || g.Projection.Execution.SessionID == "" {
			t.Fatal("missing execution identity")
		}
	}
	original := r.Documents[0].Spec.Body
	r.Documents[0].Spec.Body = "# Reviewed edit"
	r, e = s.save(ctx, r)
	if e != nil {
		t.Fatal(e)
	}
	r = observeBatch(t, s, r.ID)
	if r.Documents[0].Spec.Body == original {
		t.Fatal("observation overwrote review edit")
	}
	saved, _ := store.Roadmap(ctx, s.Project, r.ID)
	if len(saved.Generations) != 7 || !reflect.DeepEqual(saved.Generations[0].Request, r.Generations[0].Request) {
		t.Fatal("private execution did not survive storage")
	}
}
func TestRoadmapPerTaskStopCoversEveryRun(t *testing.T) {
	t.Run("cancel-after-queue-admission", func(t *testing.T) {
		s, _, f, parent := batchSetup(t, 2, 5)
		ctx := context.Background()
		r, err := s.GenerateDocuments(ctx, parent.ID, strings.Repeat("d", 32), parent.Revision)
		if err != nil {
			t.Fatal(err)
		}
		s.Queue = &stopAtAdmission{Queue: s.Queue, stop: func() {
			if _, err := s.Stop(ctx, r.ID, strings.Repeat("e", 32)); err != nil {
				t.Fatal(err)
			}
		}}
		r = observeBatch(t, s, r.ID)
		if len(f.runs) != 0 || !r.Cancelled || !r.Settled() {
			t.Fatal("cancelled unsent task started or retained phantom obligation")
		}
		for _, g := range r.Generations {
			if g.Submission != "pending" {
				t.Fatal("cancel created an unknown unsent request")
			}
		}
	})

	s, _, f, parent := batchSetup(t, 6, 5)
	ctx := context.Background()
	r, e := s.GenerateDocuments(ctx, parent.ID, strings.Repeat("d", 32), parent.Revision)
	if e != nil {
		t.Fatal(e)
	}
	r = observeBatch(t, s, r.ID)
	r, e = s.Stop(ctx, r.ID, strings.Repeat("e", 32))
	if e != nil {
		t.Fatal(e)
	}
	r = observeBatch(t, s, r.ID)
	r = observeBatch(t, s, r.ID)
	if len(f.runs) != 5 || len(f.stopped) != 5 || !r.Settled() {
		t.Fatal("stop missed sessions or dispatched waiting task", len(f.runs), len(f.stopped), r.State)
	}
}
func TestRoadmapPerTaskRetryPreservesFIFOAndRejectsHistory(t *testing.T) {
	s, _, f, parent := batchSetup(t, 1, 1)
	ctx := context.Background()
	r, e := s.GenerateDocuments(ctx, parent.ID, strings.Repeat("d", 32), parent.Revision)
	if e != nil {
		t.Fatal(e)
	}
	r = observeBatch(t, s, r.ID)
	finishGeneration(t, s, f, r, r.Generations[0], true)
	r = observeBatch(t, s, r.ID)
	waiting := time.Now().UTC().Add(-time.Millisecond)
	s.Queue.UpdateContent("other", "waiting", waiting, false, false)
	r, e = s.RetryDocuments(ctx, r.ID, r.Generations[0].Key, strings.Repeat("e", 32), r.Revision)
	if e != nil {
		t.Fatal(e)
	}
	newest := r.Generations[len(r.Generations)-1]
	if !newest.CreatedAt.After(waiting) {
		t.Fatal("retry reused old timestamp")
	}
	// Rebuild admission from original persisted creation times.
	s.Queue = runnerbinding.NewContentQueue(1)
	s.Queue.UpdateContent("other", "waiting", waiting, false, false)
	s.track(r)
	r = observeBatch(t, s, r.ID)
	if len(f.runs) != 1 || !s.Queue.AcquireContent("other", "waiting") {
		t.Fatal("retry jumped ahead after reopening")
	}
	s.Queue.UpdateContent("other", "waiting", waiting, true, true)
	r = observeBatch(t, s, r.ID)
	finishGeneration(t, s, f, r, r.Generations[len(r.Generations)-1], true)
	r = observeBatch(t, s, r.ID)
	if _, e = s.GenerateDocuments(ctx, parent.ID, strings.Repeat("f", 32), parent.Revision); e != nil {
		t.Fatal(e)
	}
	if _, e = s.RetryDocuments(ctx, r.ID, r.Generations[0].Key, strings.Repeat("a", 32), r.Revision); e == nil {
		t.Fatal("historical batch retried alongside newer batch")
	}
}

// Explicitly interleave durable cancellation between quota acquisition and send intent.
type stopAtAdmission struct {
	Queue
	stop func()
}

func (q *stopAtAdmission) AcquireContent(project, id string) bool {
	ok := q.Queue.AcquireContent(project, id)
	if ok && q.stop != nil {
		stop := q.stop
		q.stop = nil
		stop()
	}
	return ok
}

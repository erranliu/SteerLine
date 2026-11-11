package refactoring

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/erranliu/SteerLine/internal/billing"
	"github.com/erranliu/SteerLine/internal/prompts"
	decision "github.com/erranliu/SteerLine/internal/steeragent/maintenance"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	m "github.com/erranliu/SteerLine/internal/refactoring/model"
	"github.com/erranliu/SteerLine/internal/runnerbinding"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	rc "github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
)

// Public Runner fixture: no access to Runner implementation or private state.
type maintenanceRunner struct {
	noChanges  bool
	t          *testing.T
	dir        string
	requests   []rc.Request
	runs       map[string]*rc.Run
	files      map[string]rc.ResultFile
	latest     string
	reads      int
	unknown    bool
	hold       bool
	failWrite  bool
	rejectStop bool
	stops      int
}

func (f *maintenanceRunner) Submit(_ context.Context, req rc.Request) (runnerclient.Receipt, error) {
	f.requests = append(f.requests, req)
	id := fmt.Sprintf("run-%d", len(f.requests))
	var in rc.RunInput
	if e := json.Unmarshal(req.Input, &in); e != nil {
		f.t.Fatal(e)
	}
	run := &rc.Run{RunID: id, Revision: 2, InputDigest: m.Digest(req.Input), Workflow: in.Workflow, Correlation: in.Correlation, StopStatus: "stopped", Status: "completed", UpdatedAt: time.Now().Add(-time.Minute), Final: &rc.Final{Verified: true, NativeOutcome: "succeeded", CommandID: "command-" + id, TurnID: "turn-" + id}}
	if in.Workflow.ID == "generate" {
		var g rc.GenerateRunInput
		_ = json.Unmarshal(req.Input, &g)
		var ctx struct {
			Mode            string
			Round           int
			PR              *rc.PRFact
			ReportCandidate *m.Result `json:"report_candidate"`
		}
		_ = json.Unmarshal([]byte(*g.Generate.Materials[1].Content), &ctx)
		d := map[string]string{"kind": "execute", "prompt": "执行当前有限切片并验证，保留原分支。", "summary": "已调查", "question": ""}
		if ctx.Mode == "audit" {
			d["kind"] = "accept"
			d["prompt"] = ""
		}
		if ctx.Mode == "finish" {
			d["prompt"] = "交付已有成果，更新原PR和同源报告。"
		}
		data, _ := json.Marshal(d)
		if g.Generate.Repository != nil {
			run.RepositorySource = &rc.RepositorySource{LocalPath: g.Generate.Repository.LocalPath, Commit: g.Generate.Repository.Commit, Tree: "tree"}
		} else {
			run.Execution.Workspace = g.Generate.Directory.Path
		}
		for _, doc := range g.Generate.Materials {
			path := filepath.Join(f.dir, "materials", doc.Path)
			run.Materials = append(run.Materials, rc.Material{Kind: "file", Path: doc.Path, Digest: m.Digest([]byte(*doc.Content)), SnapshotPath: path, ExecutionLocation: path})
		}
		run.ContentResult = &rc.ContentResult{Status: "succeeded", JSON: data, Source: *run.Final, FileMaterials: run.Materials, RepositorySource: run.RepositorySource}
		f.reads++
	} else {
		if in.ReuseWorktreeFromRunID != f.latest {
			f.t.Fatalf("source=%s latest=%s", in.ReuseWorktreeFromRunID, f.latest)
		}
		if f.latest == "" {
			run.WorkspaceOriginRunID = id
		} else {
			run.WorkspaceOriginRunID = f.runs[f.latest].WorkspaceOriginRunID
		}
		f.latest = id
		run.ReuseWorktreeFromRunID = in.ReuseWorktreeFromRunID
		run.WorkspaceIdentity = "workspace-identity"
		run.Execution = rc.Execution{Workspace: filepath.Join(f.dir, "worktree"), Branch: "codex/maintenance", AttemptID: id, SessionID: "session-" + id}
		seedBytes, _ := base64.StdEncoding.DecodeString(in.ResultFile.InitialContentBase64)
		var seed struct {
			ActionID    string `json:"action_id"`
			OperationID string `json:"operation_id"`
			Context     facts  `json:"context"`
		}
		_ = json.Unmarshal(seedBytes, &seed)
		value := m.Result{ActionID: seed.ActionID, OperationID: seed.OperationID, Outcome: "done", Summary: "切片完成", Head: strings.Repeat("a", 40), Base: strings.Repeat("b", 40)}
		if seed.Context.Finish {
			value.PR = "https://example.test/repo/pull/1"
			value.Report = "收益已验证；冲突审计：保持。费用按冻结快照。"
			if f.noChanges {
				value.PR = ""
				value.NoChanges = true
				value.Report = "没有有效改动，交付审计报告。"
			}
		}
		data, _ := json.Marshal(value)
		encoded := base64.StdEncoding.EncodeToString(data)
		f.files[id] = rc.ResultFile{Version: 1, CollectionStatus: "closed", ContentBase64: &encoded, SHA256: m.Digest(data), SizeBytes: len(data)}
		if f.failWrite {
			run.Final.NativeOutcome = "failed"
			run.Status = "failed"
		}
	}
	if f.hold {
		run.StopStatus = "running"
	}
	f.runs[id] = run
	if f.unknown {
		f.unknown = false
		return runnerclient.Receipt{}, &runnerclient.Failure{Code: "transport", Unknown: true}
	}
	return runnerclient.Receipt{RunID: id, ReceiptStatus: "accepted"}, nil
}
func (f *maintenanceRunner) Inspect(_ context.Context, selector, id string) (runnerclient.Inspection, error) {
	if selector == "request-id" {
		for i, req := range f.requests {
			if req.RequestID == id {
				run := f.runs[fmt.Sprintf("run-%d", i+1)]
				return runnerclient.Inspection{Run: run, Command: &rc.Command{Command: "submit", RequestID: id, RunID: &run.RunID}}, nil
			}
		}
	}
	if run := f.runs[id]; run != nil {
		return runnerclient.Inspection{Run: run}, nil
	}
	return runnerclient.Inspection{}, errors.New("missing")
}
func (f *maintenanceRunner) Call(_ context.Context, command string, args []string, _ *rc.Request) (runnerclient.Response, error) {
	if command == "workspace-git" {
		commit, clean := strings.Repeat("a", 40), false
		body, _ := json.Marshal(rc.WorkspaceGit{RunID: args[1], Commit: &commit, Clean: &clean, ObservedAt: time.Now(), Verified: true})
		return runnerclient.Response{Data: body}, nil
	}
	f.stops++
	if f.rejectStop {
		f.rejectStop = false
		return runnerclient.Response{}, &runnerclient.Failure{Code: "stale_revision"}
	}
	f.runs[args[1]].StopStatus = "stopped"
	return runnerclient.Response{}, nil
}
func (f *maintenanceRunner) ResultFile(_ context.Context, id string) (rc.ResultFile, error) {
	return f.files[id], nil
}

func fixture(t *testing.T) (*Service, *maintenanceRunner, m.Record) {
	t.Helper()
	dir := t.TempDir()
	store, e := sqlite.OpenSQLiteStore(filepath.Join(dir, "db.sqlite"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { store.Close() })
	f := &maintenanceRunner{t: t, dir: dir, runs: map[string]*rc.Run{}, files: map[string]rc.ResultFile{}}
	s := &Service{Project: "project", Store: store, Runner: f, Queue: runnerbinding.NewContentQueue(1), Prepare: func(context.Context, m.Start) (m.Frozen, error) {
		return m.Frozen{Repository: rc.Repository{LocalPath: dir, BaseRevision: strings.Repeat("b", 40)}, Rules: "完整规则", Runner: rc.BuildPin{StateDir: dir}}, nil
	}, Verify: func(_ context.Context, _ m.Record, v m.Result) (*rc.PRFact, error) {
		return &rc.PRFact{URL: v.PR, HeadRevision: v.Head, HeadBranch: "codex/maintenance", State: "open"}, nil
	}}
	cmd := m.Command{ID: m.ID(), Action: "refactoring_start", Target: s.Project, Expected: 1, Raw: json.RawMessage(`{"start":true}`)}
	r, e := s.Command(context.Background(), cmd, &m.Start{Direction: "simplify", Profile: "local", Strategy: "cautious", Budget: m.Budget{Rounds: 2}})
	if e != nil {
		t.Fatal(e)
	}
	return s, f, r
}
func tickUntil(t *testing.T, s *Service, id string, check func(m.Record) bool) m.Record {
	t.Helper()
	for i := 0; i < 100; i++ {
		r, e := s.Store.Refactoring(context.Background(), s.Project, id)
		if e != nil {
			t.Fatal(e)
		}
		if check(r) {
			return r
		}
		if e = s.Tick(context.Background()); e != nil {
			t.Fatal(e)
		}
	}
	r, _ := s.Store.Refactoring(context.Background(), s.Project, id)
	t.Fatalf("did not settle: phase=%s diagnostic=%s ops=%+v", r.Phase, r.Diagnostic, r.Operations)
	return r
}
func control(t *testing.T, s *Service, r m.Record, action string) m.Record {
	t.Helper()
	c := m.Command{ID: m.ID(), Action: action, Target: r.ID, Expected: r.Revision, Raw: json.RawMessage(`{"action":"` + action + `"}`)}
	out, e := s.Command(context.Background(), c, nil)
	if e != nil {
		t.Fatal(e)
	}
	return out
}

func TestMaintenanceBudgetDeliveryAndOriginalPR(t *testing.T) {
	s, f, r := fixture(t)
	r = tickUntil(t, s, r.ID, func(r m.Record) bool { return r.Phase == "delivered" })
	if r.Round != 2 || len(r.Reports) != 1 || r.PR == nil {
		t.Fatal(r)
	}
	writes := 0
	for _, req := range f.requests {
		var in rc.RunInput
		_ = json.Unmarshal(req.Input, &in)
		if in.Workflow.ID == "execute" {
			writes++
		}
	}
	if writes != 3 {
		t.Fatalf("two rounds and publication: got %d", writes)
	}
	for _, o := range r.Operations {
		if o.Purpose == "select" || o.Purpose == "audit" || o.Purpose == "finish" {
			if !strings.Contains(o.Feedback, `"kind"`) {
				t.Fatal("completed decision feedback missing from history", o)
			}
		}
	}
	originalPR := r.PR.URL
	originalOwner := r.LatestOwner
	r = control(t, s, r, "refactoring_conflict")
	r = tickUntil(t, s, r.ID, func(r m.Record) bool { return r.Phase == "delivered" })
	if len(r.Reports) != 2 || r.PR.URL != originalPR || r.Round != 2 || r.LatestOwner == originalOwner {
		t.Fatal("conflict must update original action/PR", r)
	}
	if len(f.requests) == 0 || r.Reports[0].AsOf.IsZero() {
		t.Fatal("missing frozen report snapshot")
	}
}
func TestMaintenanceUnknownSubmitStopAndFinish(t *testing.T) {
	s, f, r := fixture(t)
	f.unknown = true
	f.hold = true
	r = tickUntil(t, s, r.ID, func(r m.Record) bool { return len(r.Operations) > 0 && r.Operations[0].RunID != "" })
	if len(f.requests) != 1 {
		t.Fatal("unknown submit resent")
	}
	f.rejectStop = true
	r = control(t, s, r, "refactoring_stop")
	r = tickUntil(t, s, r.ID, func(r m.Record) bool { return r.Phase == "stopped" })
	if f.stops != 2 || len(f.requests) != 1 {
		t.Fatal("stop rejection must reconcile original without resubmit", f.stops, len(f.requests))
	}
	f.hold = false
	r = control(t, s, r, "refactoring_finish")
	r = tickUntil(t, s, r.ID, func(r m.Record) bool { return r.Phase == "delivered" })
	if r.Round != 0 || len(r.Reports) != 1 {
		t.Fatal(r)
	}
}

func TestMaintenanceStoppedWriterRemainsSourceForDelivery(t *testing.T) {
	s, f, r := fixture(t)
	r = tickUntil(t, s, r.ID, func(r m.Record) bool { return r.Phase == "execute" })
	f.hold = true
	r = tickUntil(t, s, r.ID, func(r m.Record) bool { return r.LatestOwner != "" })
	owner := r.LatestOwner
	r = control(t, s, r, "refactoring_stop")
	r = tickUntil(t, s, r.ID, func(r m.Record) bool { return r.Phase == "stopped" })
	if r.LatestOwner != owner || r.Workspace == "" || r.Round != 1 {
		t.Fatal("stopped writer ownership lost", r)
	}
	f.hold = false
	r = control(t, s, r, "refactoring_finish")
	r = tickUntil(t, s, r.ID, func(r m.Record) bool { return r.Phase == "delivered" })
	if r.Round != 1 {
		t.Fatal("delivery started another maintenance round")
	}
	found := false
	for _, request := range f.requests {
		var input rc.RunInput
		_ = json.Unmarshal(request.Input, &input)
		if input.Workflow.ID == "execute" && input.ReuseWorktreeFromRunID == owner {
			found = true
		}
	}
	if !found {
		t.Fatal("delivery did not reuse latest stopped writer")
	}
}

func TestMaintenanceReplaysOriginalReceipt(t *testing.T) {
	s, f, r := fixture(t)
	r = tickUntil(t, s, r.ID, func(r m.Record) bool { return r.Phase == "delivered" })
	count := len(f.requests)
	command := m.Command{ID: r.ID, Action: "refactoring_start", Target: s.Project, Expected: 1, Raw: json.RawMessage(`{"start":true}`)}
	original, e := s.Command(context.Background(), command, nil)
	if e != nil || original.ID != r.ID || original.Revision != 1 || original.Project != s.Project || len(f.requests) != count {
		t.Fatal("receipt changed or replay submitted", original, e)
	}
	command.Raw = json.RawMessage(`{"start":false}`)
	if _, e = s.Command(context.Background(), command, nil); e == nil {
		t.Fatal("changed original request accepted")
	}
}

func TestMaintenanceNoChangesDeliversReportWithoutPR(t *testing.T) {
	s, f, r := fixture(t)
	f.noChanges = true
	r = control(t, s, r, "refactoring_finish")
	r = tickUntil(t, s, r.ID, func(r m.Record) bool { return r.Phase == "delivered" })
	if r.PR != nil || len(r.Reports) != 1 || r.Reports[0].PR != "" || r.Round != 0 {
		t.Fatal("no changes created PR or new round", r)
	}
}
func TestMaintenanceConcurrentTicksDoNotOrphanPending(t *testing.T) {
	s, _, r := fixture(t)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _ = s.Tick(context.Background()) }()
	}
	wg.Wait()
	r = tickUntil(t, s, r.ID, func(r m.Record) bool { return r.Phase == "delivered" })
	for _, o := range r.Operations {
		if !o.Settled() {
			t.Fatal("orphan operation", o)
		}
	}
}
func TestMaintenanceRejectsForeignDecisionSource(t *testing.T) {
	for _, change := range []string{"commit", "result-commit", "final", "material"} {
		t.Run(change, func(t *testing.T) {
			s, f, r := fixture(t)
			r = tickUntil(t, s, r.ID, func(r m.Record) bool { return len(r.Operations) > 0 && r.Operations[0].RunID != "" })
			run := f.runs[r.Operations[0].RunID]
			switch change {
			case "commit":
				run.RepositorySource.Commit = "foreign"
			case "result-commit":
				copy := *run.ContentResult.RepositorySource
				copy.Commit = "foreign"
				run.ContentResult.RepositorySource = &copy
			case "final":
				run.ContentResult.Source.TurnID = "foreign"
			case "material":
				run.Materials[0].Digest = "foreign"
			}
			if e := s.Tick(context.Background()); e == nil {
				t.Fatal("foreign evidence accepted")
			}
			if len(f.requests) != 1 {
				t.Fatal("advanced despite foreign source")
			}
		})
	}
	t.Run("directory", func(t *testing.T) {
		s, f, r := fixture(t)
		r = tickUntil(t, s, r.ID, func(r m.Record) bool {
			if len(r.Operations) == 0 {
				return false
			}
			o := r.Operations[len(r.Operations)-1]
			return o.Purpose == "audit" && o.RunID != ""
		})
		o := r.Operations[len(r.Operations)-1]
		f.runs[o.RunID].Execution.Workspace = filepath.Join(t.TempDir(), "foreign")
		if e := s.Tick(context.Background()); e == nil {
			t.Fatal("foreign directory accepted")
		}
	})
}
func TestMaintenanceRetriesFailedDecisionOutputOnce(t *testing.T) {
	s, f, r := fixture(t)
	r = tickUntil(t, s, r.ID, func(r m.Record) bool {
		if len(r.Operations) == 0 {
			return false
		}
		last := r.Operations[len(r.Operations)-1]
		return last.Purpose == "audit" && last.RunID != ""
	})
	first := r.Operations[len(r.Operations)-1]
	run := f.runs[first.RunID]
	run.Status = "failed"
	run.Final = nil
	run.ContentResult = &rc.ContentResult{Status: "failed", Diagnostic: "human_input_required"}
	r = tickUntil(t, s, r.ID, func(r m.Record) bool {
		last := r.Operations[len(r.Operations)-1]
		return last.ID != first.ID && last.RunID != ""
	})
	second := r.Operations[len(r.Operations)-1]
	if second.Purpose != "audit" || second.Round != first.Round || second.Request.RequestID == first.Request.RequestID || !r.Operations[len(r.Operations)-2].Settled() {
		t.Fatal("failed audit did not start one fresh decision in the same round", r)
	}
	run = f.runs[second.RunID]
	run.Status = "failed"
	run.Final = nil
	run.ContentResult = &rc.ContentResult{Status: "failed", Diagnostic: "invalid_json: extra data"}
	submissions := len(f.requests)
	r = tickUntil(t, s, r.ID, func(r m.Record) bool { return r.Phase == "blocked" })
	if !strings.Contains(r.Diagnostic, "invalid_json: extra data") || r.Active() || r.Question != nil || !r.Operations[len(r.Operations)-1].Settled() {
		t.Fatal("failed audit did not settle with its own diagnostic", r)
	}
	if err := s.Tick(context.Background()); err != nil || len(f.requests) != submissions {
		t.Fatal("advanced after failed decision", err, len(f.requests))
	}
}

func TestMaintenanceFailedPublicationCannotBecomeCandidate(t *testing.T) {
	s, f, r := fixture(t)
	r = tickUntil(t, s, r.ID, func(r m.Record) bool { return r.Phase == "execute" && r.WriteKind == "publish" })
	f.failWrite = true
	r = tickUntil(t, s, r.ID, func(r m.Record) bool { return r.Phase == "audit" })
	if r.Candidate != nil || r.Phase == "delivered" {
		t.Fatal("failed native execution became delivery", r)
	}
}
func TestMaintenanceRecoverExcludesOfflineTimeAndDoesNotResubmit(t *testing.T) {
	s, f, r := fixture(t)
	base := time.Now().UTC()
	s.Now = func() time.Time { return base }
	r, _ = s.update(context.Background(), r.ID, func(r *m.Record) error { r.ActiveSince = &base; return nil })
	s.Now = func() time.Time { return base.Add(24 * time.Hour) }
	if e := s.Recover(context.Background()); e != nil {
		t.Fatal(e)
	}
	r = tickUntil(t, s, r.ID, func(r m.Record) bool { return r.Phase == "stopped" })
	if r.ElapsedNanos != 0 || len(f.requests) != 0 {
		t.Fatal("offline charged or pending sent", r.ElapsedNanos, len(f.requests))
	}
}

func TestMaintenanceActualPromptFileHandoff(t *testing.T) {
	s, _, r := fixture(t)
	rules, e := prompts.MaintenanceRules()
	if e != nil {
		t.Fatal(e)
	}
	r.Frozen.Rules = string(rules)
	read, e := s.prepareOperation(r)
	if e != nil {
		t.Fatal(e)
	}
	var g rc.GenerateRunInput
	if e = json.Unmarshal(read.Request.Input, &g); e != nil {
		t.Fatal(e)
	}
	golden, e := os.ReadFile("../prompts/testdata/maintenance-decision.golden.md")
	if e != nil {
		t.Fatal(e)
	}
	if g.Generate.Instruction != string(golden) || g.Generate.Repository == nil || g.Generate.Repository.Commit != r.Frozen.Repository.BaseRevision || g.Generate.Directory != nil || len(g.Generate.Materials) != 2 || *g.Generate.Materials[0].Content != string(rules) {
		t.Fatal("actual read-only wire differs")
	}
	r.Workspace = filepath.Join(t.TempDir(), "original-worktree")
	r.LatestOwner = "latest-failed-owner"
	before := "0.75"
	head, clean := strings.Repeat("c", 40), false
	git := &rc.WorkspaceGit{RunID: r.LatestOwner, Commit: &head, Clean: &clean, ObservedAt: time.Now(), Verified: true}
	r.Operations = []m.Operation{{State: "complete", Cost: m.Cost{Amount: &before, Complete: true}, Run: &rc.Run{RunID: r.LatestOwner, Workflow: rc.Workflow{ID: "execute"}, Execution: rc.Execution{Branch: "codex/original"}}, WorkspaceGit: git}}
	r.Phase = "audit"
	read, e = s.prepareOperation(r)
	if e != nil {
		t.Fatal(e)
	}
	g = rc.GenerateRunInput{}
	_ = json.Unmarshal(read.Request.Input, &g)
	if g.Generate.Directory == nil || g.Generate.Directory.Path != r.Workspace || g.Generate.Repository != nil {
		t.Fatal("audit lost original dirty workspace")
	}
	var current facts
	if e = json.Unmarshal([]byte(*g.Generate.Materials[1].Content), &current); e != nil || current.Repository.LocalPath != "" || current.ManagedWorkspace != r.Workspace || current.WorkspaceGit == nil || !current.WorkspaceGit.Verified || current.WorkspaceGit.RunID != r.LatestOwner || current.WorkspaceGit.Commit == nil || *current.WorkspaceGit.Commit != head || current.WorkspaceGit.Clean == nil || *current.WorkspaceGit.Clean {
		t.Fatal("audit did not receive the verified original workspace evidence", e, current)
	}
	frozenAt := time.Date(2026, 11, 11, 4, 0, 0, 0, time.UTC)
	now := frozenAt
	s.Now = func() time.Time { return now }
	r.Operations[0].RunID = r.LatestOwner
	r.Operations[0].Cost.Complete = false
	r.Operations[0].Cost.Diagnostic = "pending usage"
	r.Phase = "finish"
	finish, e := s.prepareOperation(r)
	if e != nil {
		t.Fatal(e)
	}
	decisionUsage := "0.50"
	finish.State, finish.Cost = "complete", m.Cost{Amount: &decisionUsage, Complete: true}
	r.Operations = append(r.Operations, finish)
	r.Operations[0].Cost.Complete = true
	now = now.Add(time.Minute)
	r.Phase = "execute"
	r.WriteKind = "publish"
	r.PendingPrompt = "用户原文\r\n保持 CRLF 与结尾空格  "
	write, e := s.prepareOperation(r)
	if e != nil {
		t.Fatal(e)
	}
	var input rc.RunInput
	_ = json.Unmarshal(write.Request.Input, &input)
	golden, e = os.ReadFile("../prompts/testdata/maintenance-execute.golden.md")
	if e != nil {
		t.Fatal(e)
	}
	if input.Workflow.PromptOverrides["execute"] != string(golden) || input.ReuseWorktreeFromRunID != r.LatestOwner {
		t.Fatal("writable handoff changed fixed body/source")
	}
	raw, e := base64.StdEncoding.DecodeString(input.ResultFile.InitialContentBase64)
	if e != nil {
		t.Fatal(e)
	}
	var seed struct {
		Instruction, Rules string
		Context            facts
	}
	if e = json.Unmarshal(raw, &seed); e != nil {
		t.Fatal(e)
	}
	if seed.Instruction != r.PendingPrompt || seed.Rules != string(rules) || seed.Context.ManagedBranch != "codex/original" || !seed.Context.AsOf.Equal(frozenAt) || !write.ReportAsOf.Equal(frozenAt) || seed.Context.Amount == nil || *seed.Context.Amount != "0.750000000000000000" || seed.Context.CostComplete || len(seed.Context.CostIssues) != 1 || !strings.Contains(seed.Context.CostIssues[0], "pending usage") {
		t.Fatal("complete original file lost bytes/snapshot")
	}
	late := "1.00"
	write.State, write.Cost = "complete", m.Cost{Amount: &late, Complete: true}
	r.Operations = append(r.Operations, write)
	now = now.Add(time.Minute)
	r.Phase = "audit"
	read, e = s.prepareOperation(r)
	if e != nil {
		t.Fatal(e)
	}
	var audit struct {
		Amount   string
		Feedback []struct {
			ReferenceCost string    `json:"report_reference_cost"`
			AsOf          time.Time `json:"report_as_of"`
		}
	}
	if e = json.Unmarshal(read.Request.Input, &g); e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal([]byte(*g.Generate.Materials[1].Content), &audit); e != nil {
		t.Fatal(e)
	}
	if len(audit.Feedback) != 3 || audit.Amount != "2.250000000000000000" || audit.Feedback[2].ReferenceCost != "0.750000000000000000" || !audit.Feedback[2].AsOf.Equal(frozenAt) {
		t.Fatal("publish audit lost the frozen report cost beside later usage", audit)
	}
	t.Logf("new caller bodies: decision=%d execute=%d rules=%d seed=%d bytes", len(g.Generate.Instruction), len(input.Workflow.PromptOverrides["execute"]), len(rules), len(raw))
}
func TestMaintenanceConflictCannotEraseOriginalPR(t *testing.T) {
	s, _, r := fixture(t)
	r.PR = &rc.PRFact{URL: "https://example.test/original"}
	r.Phase = "verifying"
	r.Candidate = &m.Result{Outcome: "done", NoChanges: true, Report: "本轮没有改动"}
	old := r.Revision
	r.Revision++
	if e := s.Store.SaveRefactoring(context.Background(), r, old, nil); e != nil {
		t.Fatal(e)
	}
	if e := s.settleCandidate(context.Background(), r.ID); e != nil {
		t.Fatal(e)
	}
	saved, _ := s.Store.Refactoring(context.Background(), s.Project, r.ID)
	if saved.PR == nil || saved.PR.URL != r.PR.URL || saved.Phase != "blocked" {
		t.Fatal("original PR lost")
	}
}

func TestMaintenanceDollarGapAndExhaustionFinishWithoutQuestion(t *testing.T) {
	for _, full := range []bool{false, true} {
		t.Run(fmt.Sprint(full), func(t *testing.T) {
			s, f, r := fixture(t)
			r, _ = s.update(context.Background(), r.ID, func(r *m.Record) error {
				r.Input.Budget = m.Budget{Dollars: "0.1"}
				r.Frozen.Pricing = &billing.Policy{}
				return nil
			})
			amount := "0.2"
			s.ReadCost = func(context.Context, m.Record, m.Operation) (m.Cost, error) {
				return m.Cost{Amount: &amount, Complete: full, ObservedAt: s.now(), Diagnostic: map[bool]string{false: "registered parser uncovered"}[full]}, nil
			}
			r = tickUntil(t, s, r.ID, func(r m.Record) bool { return r.Phase == "delivered" })
			if r.Round != 0 || r.Question != nil {
				t.Fatal("budget created new round or human wait", r)
			}
			calls := len(f.requests)
			cost := r.Reports[0].Cost
			asOf := r.Reports[0].AsOf
			future := s.now().Add(time.Minute)
			s.Now = func() time.Time { return future }
			amount = "0.3"
			if e := s.Tick(context.Background()); e != nil {
				t.Fatal(e)
			}
			saved, _ := s.Store.Refactoring(context.Background(), s.Project, r.ID)
			if len(f.requests) != calls || len(saved.Reports) != 1 || !saved.Reports[0].AsOf.Equal(asOf) || *saved.Reports[0].Cost != *cost {
				t.Fatal("late fees regenerated delivery")
			}
		})
	}
	s, _, r := fixture(t)
	r.Round = 2
	if e := s.applyDecision(&r, "select", decision.Decision{Kind: "ask", Question: "追加预算？"}); e != nil {
		t.Fatal(e)
	}
	if r.Phase != "finish" || r.Question != nil {
		t.Fatal("exhausted selection must finish")
	}
}
func TestMaintenanceInvalidReportSourcePreservesCandidate(t *testing.T) {
	for _, kind := range []string{"branch-base", "short-head"} {
		t.Run(kind, func(t *testing.T) {
			s, _, r := fixture(t)
			r.Phase = "verifying"
			r.Candidate = &m.Result{Outcome: "done", NoChanges: true, Report: "已有成果", Head: strings.Repeat("a", 40), Base: strings.Repeat("b", 40)}
			if kind == "branch-base" {
				r.Candidate.Base = "main"
			} else {
				r.Candidate.Head = "abcdef1"
			}
			old := r.Revision
			r.Revision++
			if e := s.Store.SaveRefactoring(context.Background(), r, old, nil); e != nil {
				t.Fatal(e)
			}
			if e := s.settleCandidate(context.Background(), r.ID); e != nil {
				t.Fatal(e)
			}
			saved, _ := s.Store.Refactoring(context.Background(), s.Project, r.ID)
			if saved.Phase != "blocked" || saved.Candidate == nil || len(saved.Reports) != 0 {
				t.Fatal("invalid source stuck or overwrote evidence", saved)
			}
		})
	}
}

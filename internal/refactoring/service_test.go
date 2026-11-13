package refactoring

import (
	"context"
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
	noChanges            bool
	prBody               string
	baseOverride         string
	headOverride         string
	askMode              string
	t                    *testing.T
	dir                  string
	requests             []rc.Request
	runs                 map[string]*rc.Run
	latest               string
	reads                int
	unknown              bool
	hold                 bool
	failWrite            bool
	malformedPublication bool
	rejectStop           bool
	stops                int
}

func (f *maintenanceRunner) Submit(_ context.Context, req rc.Request) (runnerclient.Receipt, error) {
	f.requests = append(f.requests, req)
	id := fmt.Sprintf("run-%d", len(f.requests))
	var in rc.RunInput
	if e := json.Unmarshal(req.Input, &in); e != nil {
		f.t.Fatal(e)
	}
	run := &rc.Run{RunID: id, Revision: 2, InputDigest: m.Digest(req.Input), Workflow: in.Workflow, Correlation: in.Correlation, StopStatus: "stopped", Status: "completed", UpdatedAt: time.Now().Add(-time.Minute), Final: &rc.Final{Verified: true, NativeOutcome: "succeeded", CommandID: "command-" + id, AttemptID: id, SessionID: "session-" + id, TurnID: "turn-" + id}}
	if in.Workflow.ID == "generate" {
		var g rc.GenerateRunInput
		_ = json.Unmarshal(req.Input, &g)
		var ctx decisionContext
		_ = json.Unmarshal([]byte(*g.Generate.Materials[1].Content), &ctx)
		d := decision.Decision{Kind: "execute", Prompt: "执行当前有限切片并验证，保留原分支。", Summary: "已调查"}
		switch ctx.Mode {
		case "audit":
			d.Kind, d.Prompt = "accept", ""
		case "first":
			if f.noChanges {
				d.Kind, d.Prompt = "finish", ""
			}
		case "finish":
			d.Prompt = "交付已有成果并返回PR。"
		}
		if f.askMode == ctx.Mode && len(ctx.Answers) == 0 {
			d.Kind, d.Prompt = "ask", ""
			d.Question, _ = json.Marshal(m.QuestionBody{Prompt: "如何处理？", Facts: []string{"已有接口"}, Attempts: []string{}, Options: []m.QuestionOption{{ID: "keep", Label: "保留", Impact: "保持行为"}}, RecommendedOptionID: "keep", RecommendationReason: "易验证"})
		}
		data, _ := json.Marshal(d)
		if ctx.Mode == "report" {
			data, _ = json.Marshal(decision.Review{Recommendation: "merge", Report: "收益已验证。" + ctx.PR.HeadRevision + ctx.PR.BaseRevision})
		}
		if g.Generate.Repository != nil || (ctx.Mode == "first") != (g.Generate.Directory != nil) {
			f.t.Fatalf("invalid decision source: %+v", g.Generate)
		}
		run.Execution.Workspace = filepath.Join(f.dir, "runs", id, "content")
		if g.Generate.Directory != nil {
			if g.Generate.Directory.Access != "read_only" {
				f.t.Fatal("non-read-only investigation")
			}
			run.Execution.Workspace = g.Generate.Directory.Path
		}
		for _, doc := range g.Generate.Materials {
			path := filepath.Join(f.dir, "materials", doc.Path)
			run.Materials = append(run.Materials, rc.Material{Kind: "file", Path: doc.Path, Digest: m.Digest([]byte(*doc.Content)), SnapshotPath: path, ExecutionLocation: path})
		}
		run.ContentResult = &rc.ContentResult{Status: "succeeded", JSON: data, Source: *run.Final, FileMaterials: run.Materials}
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
		run.Execution = rc.Execution{Workspace: filepath.Join(f.dir, "worktree"), Branch: "codex/maintenance", AttemptID: id, SessionID: "session-" + id, TurnID: "turn-" + id}
		if in.ResultFile != nil {
			f.t.Fatal("maintenance writer requested a result file")
		}
		value := m.Result{ActionID: in.Correlation["steerline_refactoring"], OperationID: in.Correlation["refactoring_operation"], Outcome: "done", Summary: "切片完成", Head: strings.Repeat("a", 40), Base: strings.Repeat("b", 40)}
		purpose := strings.TrimPrefix(in.Title, "重构与维护 · ")
		if purpose == "publish" || purpose == "conflict" {
			value.PR = "https://example.test/repo/pull/1"
			f.prBody = ""
		}
		if purpose == "conflict" && f.baseOverride != "" {
			value.Base = f.baseOverride
		}
		data, _ := json.Marshal(value)
		if purpose == "publish" && f.malformedPublication {
			var fields map[string]any
			if err := json.Unmarshal(data, &fields); err != nil {
				f.t.Fatal(err)
			}
			fields["pr"] = map[string]any{"number": 818, "url": value.PR, "state": "OPEN", "draft": true}
			data, _ = json.Marshal(fields)
		}
		run.Final.Text = string(data)
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
	run := f.runs[args[1]]
	run.StopStatus = "stopped"
	return runnerclient.Response{}, nil
}

func fixture(t *testing.T) (*Service, *maintenanceRunner, m.Record) {
	t.Helper()
	dir := t.TempDir()
	store, e := sqlite.OpenSQLiteStore(filepath.Join(dir, "db.sqlite"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { store.Close() })
	f := &maintenanceRunner{t: t, dir: dir, runs: map[string]*rc.Run{}}
	s := &Service{Project: "project", Store: store, Runner: f, Queue: runnerbinding.NewContentQueue(1), InvestigationPath: func(string) (string, error) { return filepath.Join(dir, "fixed"), nil }, Investigate: func(context.Context, m.Record, string) error { return nil }, Prepare: func(context.Context, m.Start) (m.Frozen, error) {
		direction, err := readBuiltInDirection("simplify")
		return m.Frozen{Direction: direction, Repository: rc.Repository{LocalPath: dir, BaseRevision: strings.Repeat("b", 40)}, FirstRules: "首轮规则", FollowRules: "后续规则", ReportRules: "报告规则", Strategy: "策略正文", Runner: rc.BuildPin{StateDir: dir}}, err
	}, ReviewDiff: func(context.Context, m.Record, rc.PRFact) (string, error) {
		return "diff --git a/example b/example", nil
	}, Verify: func(_ context.Context, _ m.Record, v m.Result) (*rc.PRFact, string, error) {
		base := strings.Repeat("b", 40)
		if f.baseOverride != "" {
			base = f.baseOverride
		}
		head := strings.Repeat("a", 40)
		if f.headOverride != "" {
			head = f.headOverride
		}
		return &rc.PRFact{URL: v.PR, HeadRevision: head, BaseRevision: base, HeadBranch: "codex/maintenance", State: "open"}, "SUCCESS", nil
	}}
	cmd := m.Command{ID: m.ID(), Action: "refactoring_start", Target: s.Project, Expected: 1, Raw: json.RawMessage(`{"start":true}`)}
	direction, e := readBuiltInDirection("simplify")
	if e != nil {
		t.Fatal(e)
	}
	r, e := s.Command(context.Background(), cmd, &m.Start{Direction: direction.ID, DirectionRevision: direction.Revision, Profile: "local", Strategy: "cautious", Budget: m.Budget{Rounds: 2}})
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
	t.Fatalf("did not settle: phase=%s diagnostic=%s operations=%d", r.Phase, r.Diagnostic, len(r.Operations))
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
	r = tickUntil(t, s, r.ID, func(r m.Record) bool { return len(r.Operations) == 1 && r.Operations[0].RunID != "" })
	run := f.runs[r.Operations[0].RunID]
	// The verified Final can supply feedback while the structured result
	// independently supplies the decision.
	run.Final.Text = "原生反馈仅供展示"
	run.ContentResult.Source = *run.Final
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
	if r.Operations[0].Feedback != "原生反馈仅供展示" {
		t.Fatal("original Final feedback missing from history")
	}
	for _, o := range r.Operations[1:] {
		if o.Purpose == "select" || o.Purpose == "audit" || o.Purpose == "finish" {
			if !strings.Contains(o.Feedback, `"kind"`) {
				t.Fatal("completed decision feedback missing from history", o)
			}
		}
	}
	originalPR := r.PR.URL
	originalOwner := r.LatestOwner
	f.baseOverride = strings.Repeat("c", 40)
	r = control(t, s, r, "refactoring_conflict")
	op, err := s.prepareOperation(r, m.ID(), "", m.ReviewEvidence{})
	var in rc.GenerateRunInput
	if err != nil || json.Unmarshal(op.Request.Input, &in) != nil {
		t.Fatalf("conflict decision request unavailable: %v", err)
	}
	var facts decisionContext
	if err := json.Unmarshal([]byte(*in.Generate.Materials[1].Content), &facts); err != nil {
		t.Fatal(err)
	}
	if in.Generate.Directory != nil || !facts.ContinueConflict || facts.Mode != "finish" ||
		facts.PR == nil || facts.PR.URL != originalPR || facts.PR.BaseRevision != f.baseOverride || facts.PR.HeadRevision != r.Reports[0].Head {
		t.Fatalf("conflict did not freeze the newly verified version of the original PR: %+v", facts)
	}
	r = tickUntil(t, s, r.ID, func(r m.Record) bool { return r.Phase == "delivered" })
	if len(r.Reports) != 2 || r.PR.URL != originalPR || r.Round != 2 || r.LatestOwner == originalOwner ||
		r.Reports[0].Base == f.baseOverride || r.Reports[1].Base != f.baseOverride || f.prBody != "" {
		t.Fatal("conflict must update original action/PR", r)
	}
	if len(f.requests) == 0 {
		t.Fatal("missing frozen report snapshot")
	}
}

func TestMaintenanceNeverAdoptsDecisionTextWithoutStructuredResult(t *testing.T) {
	for _, change := range []string{"missing result", "empty JSON", "invalid decision JSON"} {
		t.Run(change, func(t *testing.T) {
			s, f, r := fixture(t)
			r = tickUntil(t, s, r.ID, func(r m.Record) bool { return len(r.Operations) == 1 && r.Operations[0].RunID != "" })
			run := f.runs[r.Operations[0].RunID]
			text := string(run.ContentResult.JSON)
			run.Final.Text = text
			run.ContentResult.Source = *run.Final
			run.ContentResult.Text = text
			diagnostic := "缺少结构化决策结果 ContentResult.JSON"
			switch change {
			case "missing result":
				run.ContentResult = nil
			case "empty JSON":
				run.ContentResult.JSON = nil
			case "invalid decision JSON":
				run.ContentResult.JSON = json.RawMessage(`{"kind":"execute","summary":"已调查","prompt":""}`)
				diagnostic = "执行决定缺少提示词"
			}

			r = tickUntil(t, s, r.ID, func(r m.Record) bool { return r.Phase == "blocked" })
			if !strings.Contains(r.Diagnostic, diagnostic) || r.Round != 0 || r.PendingPrompt != "" || r.Active() {
				t.Fatalf("unusable structured result advanced the action: %+v", r)
			}
			if len(r.Operations) != 1 || !r.Operations[0].Settled() || r.Operations[0].Feedback != text {
				t.Fatalf("the original read-only operation should settle and keep feedback: %+v", r.Operations)
			}
			if err := s.Tick(context.Background()); err != nil || len(f.requests) != 1 || f.latest != "" {
				t.Fatalf("unusable structured result submitted another operation: %v, %d requests, latest write %q", err, len(f.requests), f.latest)
			}
		})
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
	if r.Round != 0 || len(r.Reports) != 0 {
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

func TestMaintenanceNoChangesSkipsReview(t *testing.T) {
	s, f, r := fixture(t)
	f.noChanges = true
	r = tickUntil(t, s, r.ID, func(r m.Record) bool { return r.Phase == "delivered" })
	if r.PR != nil || len(r.Reports) != 0 || r.Round != 0 ||
		len(r.Operations) != 1 || r.Operations[0].Purpose != "first" {
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
	for _, change := range []string{"repository", "result-repository", "final", "material", "result-material"} {
		t.Run(change, func(t *testing.T) {
			s, f, r := fixture(t)
			r = tickUntil(t, s, r.ID, func(r m.Record) bool { return len(r.Operations) > 0 && r.Operations[0].RunID != "" })
			run := f.runs[r.Operations[0].RunID]
			switch change {
			case "repository":
				run.RepositorySource = &rc.RepositorySource{Commit: "foreign"}
			case "result-repository":
				run.ContentResult.RepositorySource = &rc.RepositorySource{Commit: "foreign"}
			case "final":
				run.ContentResult.Source.TurnID = "foreign"
			case "material":
				run.Materials[0].Digest = "foreign"
			case "result-material":
				run.ContentResult.FileMaterials = append([]rc.Material(nil), run.Materials...)
				run.ContentResult.FileMaterials[0].Digest = "foreign"
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
		f.runs[o.RunID].Execution.Workspace = r.Workspace
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
	r = tickUntil(t, s, r.ID, func(r m.Record) bool { return r.Phase == "blocked" })
	if r.Candidate != nil || r.Phase == "delivered" {
		t.Fatal("failed native execution became delivery", r)
	}
	c := m.Command{ID: m.ID(), Action: "refactoring_finish", Target: r.ID, Expected: r.Revision, Raw: json.RawMessage(`{}`)}
	if _, err := s.Command(context.Background(), c, nil); err == nil {
		t.Fatal("uncertain publication allowed another publish")
	}
}

func TestMaintenancePublicationRejectsPRObjectWithFieldDiagnostic(t *testing.T) {
	s, f, r := fixture(t)
	f.malformedPublication = true
	r = tickUntil(t, s, r.ID, func(r m.Record) bool { return r.Phase == "blocked" })
	o := r.Operations[len(r.Operations)-1]
	if o.Purpose != "publish" || o.Result != nil || r.Candidate != nil || r.PR != nil ||
		!strings.Contains(o.Feedback, "最终回复字段类型无效：pr") {
		t.Fatalf("invalid PR result was accepted or its field was hidden: %q", o.Feedback)
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
	r.Frozen.Direction.Prompt = "# 自定义方向\r\n只整理给定范围  \r\n"
	for _, phase := range []string{"first", "audit", "report"} {
		r.Phase = phase
		dir := ""
		ruleName := "followup"
		if phase == "first" {
			dir = filepath.Join(t.TempDir(), "pinned")
			ruleName = "first"
		}
		if phase == "report" {
			ruleName = "report"
		}
		rules, e := prompts.MaintenanceRules(ruleName)
		if e != nil {
			t.Fatal(e)
		}
		switch phase {
		case "first":
			r.Frozen.FirstRules = string(rules)
		case "report":
			r.Frozen.ReportRules = string(rules)
		default:
			r.Frozen.FollowRules = string(rules)
		}
		op, e := s.prepareOperation(r, m.ID(), dir, m.ReviewEvidence{Diff: "fixture diff"})
		if e != nil {
			t.Fatal(e)
		}
		var input rc.GenerateRunInput
		if e = json.Unmarshal(op.Request.Input, &input); e != nil {
			t.Fatal(e)
		}
		g := input.Generate
		golden, e := os.ReadFile("../prompts/testdata/maintenance-decision.golden.md")
		if e != nil || g.Instruction != string(golden) || g.Repository != nil || (g.Directory != nil) != (phase == "first") || *g.Materials[0].Content != string(rules) {
			t.Fatal("wrong decision request", phase, e)
		}
		if phase == "first" && (g.Directory.Path != dir || g.Directory.Access != "read_only") {
			t.Fatal(g.Directory)
		}
		if phase == "report" && (len(g.Materials) != 3 || g.Materials[2].Path != "maintenance/pr.diff" || *g.Materials[2].Content != "fixture diff") || phase != "report" && (len(g.Materials) != 3 || *g.Materials[2].Content != r.Frozen.Strategy) {
			t.Fatal("wrong strategy material", phase)
		}
		var facts decisionContext
		if e = json.Unmarshal([]byte(*g.Materials[1].Content), &facts); e != nil || facts.DirectionPrompt != r.Frozen.Direction.Prompt || facts.Repository.LocalPath != "" || facts.CanStartRound != (phase == "first") {
			t.Fatal("wrong decision context", phase, e)
		}
	}
	r.Phase, r.WriteKind, r.PendingPrompt = "execute", "implement", "用户原文\r\n保持 CRLF 与结尾空格  "
	op, e := s.prepareOperation(r, m.ID(), "", m.ReviewEvidence{})
	if e != nil {
		t.Fatal(e)
	}
	var input rc.RunInput
	if e = json.Unmarshal(op.Request.Input, &input); e != nil {
		t.Fatal(e)
	}
	body := input.Workflow.PromptOverrides["execute"]
	if input.ResultFile != nil || !strings.Contains(body, `用户原文\r\n保持 CRLF 与结尾空格  `) || strings.ContainsAny(body, "\r\n") || strings.Contains(body, "direction_prompt") ||
		!strings.Contains(body, `"operation_id":"`+op.ID+`"`) {
		t.Fatal("wrong writable prompt or context", body)
	}

}

func TestMaintenanceConflictCannotEraseOriginalPR(t *testing.T) {
	s, _, r := fixture(t)
	r.PR = &rc.PRFact{URL: "https://example.test/original"}
	r.Phase = "verifying"
	r.Candidate = &m.Result{Outcome: "done", NoChanges: true}
	old := r.Revision
	r.Revision++
	if e := s.Store.SaveRefactoring(context.Background(), r, old, nil); e != nil {
		t.Fatal(e)
	}
	if e := s.verifyDelivery(context.Background(), r); e != nil {
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
			future := s.now().Add(time.Minute)
			s.Now = func() time.Time { return future }
			amount = "0.3"
			if e := s.Tick(context.Background()); e != nil {
				t.Fatal(e)
			}
			saved, _ := s.Store.Refactoring(context.Background(), s.Project, r.ID)
			if len(f.requests) != calls || len(saved.Reports) != 0 {
				t.Fatal("late fees regenerated delivery")
			}
		})
	}
	s, _, r := fixture(t)
	r.Round = 2
	if e := s.applyDecision(&r, "select", decision.Decision{Kind: "ask", Summary: "追加预算？"}); e != nil {
		t.Fatal(e)
	}
	if r.Phase != "verifying" || r.Question != nil {
		t.Fatal("exhausted selection must finish")
	}
}
func TestMaintenanceInvalidReportSourcePreservesCandidate(t *testing.T) {
	for _, kind := range []string{"branch-base", "short-head"} {
		t.Run(kind, func(t *testing.T) {
			s, _, r := fixture(t)
			r.Phase = "verifying"
			r.Candidate = &m.Result{Outcome: "done", NoChanges: true, Head: strings.Repeat("a", 40), Base: strings.Repeat("b", 40)}
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
			if e := s.verifyDelivery(context.Background(), r); e != nil {
				t.Fatal(e)
			}
			saved, _ := s.Store.Refactoring(context.Background(), s.Project, r.ID)
			if saved.Phase != "blocked" || saved.Candidate == nil || len(saved.Reports) != 0 {
				t.Fatal("invalid source stuck or overwrote evidence", saved)
			}
		})
	}
}

func TestMaintenanceAnswerPersistsAndResumesOriginalPurpose(t *testing.T) {
	for _, mode := range []string{"first", "audit"} {
		t.Run(mode, func(t *testing.T) {
			s, f, r := fixture(t)
			f.askMode = mode
			r = tickUntil(t, s, r.ID, func(r m.Record) bool { return r.Phase == "waiting_input" })
			ctx := context.Background()
			before := len(f.requests)
			if err := s.Close(ctx); err != nil {
				t.Fatal(err)
			}
			if err := s.Recover(ctx); err != nil {
				t.Fatal(err)
			}
			if err := s.Tick(ctx); err != nil {
				t.Fatal(err)
			}
			waiting, err := s.Store.Refactoring(ctx, s.Project, r.ID)
			if err != nil || waiting.Question.ID != r.Question.ID || waiting.ActiveSince != nil || len(f.requests) != before {
				t.Fatal("waiting question or paused clock changed", waiting.Question, err)
			}
			cmd := m.Command{ID: m.ID(), Action: "refactoring_answer", Target: r.ID, Expected: waiting.Revision, Raw: json.RawMessage(`{"answer":true}`), QuestionID: r.Question.ID, OptionID: "other"}
			if _, err = s.Command(ctx, cmd, nil); err == nil {
				t.Fatal("accepted an option from another question")
			}
			cmd.OptionID = "keep"
			cmd.Note = "原说明\r\n "
			var wg sync.WaitGroup
			out := make(chan error, 2)
			for range 2 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					receipt, e := s.Command(ctx, cmd, nil)
					if e == nil && receipt.ID != r.ID {
						e = errors.New("incorrect replay identity")
					}
					out <- e
				}()
			}
			wg.Wait()
			close(out)
			for err := range out {
				if err != nil {
					t.Fatal(err)
				}
			}
			resumed, err := s.Store.Refactoring(ctx, s.Project, r.ID)
			if err != nil || resumed.Phase != mode || len(resumed.Answers) != 1 || resumed.Answers[0].Note != cmd.Note {
				t.Fatal("answer did not restore original decision", resumed, err)
			}
		})
	}
}

func TestMaintenanceVersionDriftDiscardsStaleInFlightReview(t *testing.T) {
	for _, changed := range []string{"head", "base"} {
		t.Run(changed, func(t *testing.T) {
			s, f, r := fixture(t)
			r = tickUntil(t, s, r.ID, func(r m.Record) bool {
				if len(r.Operations) == 0 {
					return false
				}
				last := r.Operations[len(r.Operations)-1]
				return last.Purpose == "report" && last.RunID != ""
			})
			head, base := r.PR.HeadRevision, r.PR.BaseRevision
			if changed == "base" {
				base = strings.Repeat("c", 40)
				f.baseOverride = base
			} else {
				head = strings.Repeat("d", 40)
				f.headOverride = head
			}
			r = tickUntil(t, s, r.ID, func(r m.Record) bool { return r.Phase == "delivered" })
			if len(r.Reports) != 1 || r.Reports[0].Head != head || r.Reports[0].Base != base || r.Reports[0].PR != r.PR.URL || f.prBody != "" {
				t.Fatal("stale review was adopted or PR changed", r.Reports)
			}
			publishes := 0
			for _, op := range r.Operations {
				if op.Purpose == "publish" {
					publishes++
				}
			}
			if publishes != 1 {
				t.Fatal("PR version drift triggered a publish", publishes)
			}
		})
	}
}

func TestMaintenanceMissingPRDiffDoesNotInvokeWriter(t *testing.T) {
	s, f, r := fixture(t)
	s.ReviewDiff = func(context.Context, m.Record, rc.PRFact) (string, error) { return "", errors.New("fixture missing") }
	r = tickUntil(t, s, r.ID, func(r m.Record) bool { return r.Phase == "delivered" })
	if r.PR == nil || len(r.Reports) != 1 || r.Reports[0].Recommendation != "insufficient_evidence" || f.prBody != "" {
		t.Fatal("missing diff did not preserve PR and evidence gap")
	}
	for _, op := range r.Operations {
		if op.Purpose == "report" || op.Purpose == "sync" {
			t.Fatal("unexpected dispatch", op.Purpose)
		}
	}
}

func TestMaintenanceFinishCancelsUnsentImplementation(t *testing.T) {
	s, f, r := fixture(t)
	r = tickUntil(t, s, r.ID, func(r m.Record) bool {
		return r.Phase == "execute" && len(r.Operations) > 0 && r.Operations[len(r.Operations)-1].Purpose == "implement"
	})
	before := len(f.requests)
	r = control(t, s, r, "refactoring_finish")
	r = tickUntil(t, s, r.ID, func(r m.Record) bool { return r.Phase == "delivered" })
	if r.Round != 0 || r.PR != nil || len(f.requests) != before {
		t.Fatal("finish sent an unsent implementation", r)
	}
}

func TestMaintenanceAuditUsesPreviousAcceptedWorkspaceHead(t *testing.T) {
	s, _, r := fixture(t)
	r.Round, r.Phase = 1, "audit"
	if got := s.context(r, time.Now()).RoundStart; got != r.Frozen.Repository.BaseRevision {
		t.Fatal("first round baseline differs", got)
	}
	head := strings.Repeat("a", 40)
	clean := true
	r.Round = 2
	r.Operations = []m.Operation{
		{ID: "accepted-workspace", Purpose: "implement", Round: 1, WorkspaceGit: &rc.WorkspaceGit{Commit: &head, Clean: &clean, Verified: true}},
		{Purpose: "audit", Round: 1, AcceptedOperation: "accepted-workspace"},
		{Purpose: "implement", Round: 2},
	}
	if got := s.context(r, time.Now()).RoundStart; got != head {
		t.Fatal("second round must use the prior accepted workspace HEAD", got)
	}
	r.Phase, r.WriteKind, r.PendingPrompt = "execute", "repair", "只补查本轮差异"
	op, err := s.prepareOperation(r, m.ID(), "", m.ReviewEvidence{})
	if err != nil {
		t.Fatal(err)
	}
	var in rc.RunInput
	if err := json.Unmarshal(op.Request.Input, &in); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(in.Workflow.PromptOverrides["execute"], `"round_start":"`+head+`"`) {
		t.Fatal("repair request lacks the same round baseline")
	}

	r.Phase = "audit"
	clean = false
	if got := s.context(r, time.Now()).RoundStart; got != "" {
		t.Fatal("dirty workspace HEAD omitted accepted changes", got)
	}
	clean = true
	r.Operations[0].WorkspaceGit.Clean = nil
	if got := s.context(r, time.Now()).RoundStart; got != "" {
		t.Fatal("workspace cleanliness was not verified", got)
	}
	r.Operations[0].WorkspaceGit.Clean = &clean
	r.Operations[0].WorkspaceGit.Verified = false
	if got := s.context(r, time.Now()).RoundStart; got != "" {
		t.Fatal("unverified baseline was supplied", got)
	}
}

func TestMaintenanceContextBoundsLargeHistory(t *testing.T) {
	s, _, r := fixture(t)
	r.Phase = "audit"
	large := strings.Repeat("old evidence to exclude ", 3000)
	for i := range 500 {
		r.Operations = append(r.Operations, m.Operation{ID: m.ID(), Round: i + 1, State: "complete", Purpose: "implement", Feedback: large, Run: &rc.Run{Status: "failed", Final: &rc.Final{NativeOutcome: "failed", Verified: true}}, Result: &m.Result{Outcome: "done", Summary: large}})
	}
	r.Operations = append(r.Operations, m.Operation{ID: m.ID(), Round: 500, State: "complete", Purpose: "repair", Run: &rc.Run{Status: "completed", Final: &rc.Final{NativeOutcome: "succeeded", Verified: true}}, Result: &m.Result{Outcome: "done", Summary: "latest bounded evidence"}})
	f := s.context(r, s.now())
	if len(f.History) > 24 || len(f.Feedback) != 2 || f.Feedback[0].Result.Summary != "latest bounded evidence" {
		t.Fatal("history or current evidence not bounded", len(f.History), len(f.Feedback))
	}
	if _, err := s.prepareOperation(r, m.ID(), "", m.ReviewEvidence{}); err != nil {
		t.Fatal(err)
	}
	if r.Operations[0].Feedback != large {
		t.Fatal("context projection rewrote original evidence")
	}
}

func TestMaintenanceRefreshesDeliveredReviewOnVersionChange(t *testing.T) {
	s, f, r := fixture(t)
	r = tickUntil(t, s, r.ID, func(r m.Record) bool { return r.Phase == "delivered" })
	first := r.Reports[0]
	r, err := s.update(context.Background(), r.ID, func(r *m.Record) error {
		r.Frozen.Pricing = &billing.Policy{}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	s.ReadCost = func(context.Context, m.Record, m.Operation) (m.Cost, error) { return m.Cost{}, nil }
	later := s.now().Add(2 * time.Minute)
	s.Now = func() time.Time { return later }
	f.baseOverride = strings.Repeat("c", 40)
	if err := s.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	r = tickUntil(t, s, r.ID, func(r m.Record) bool { return r.Phase == "delivered" })
	if len(r.Reports) != 2 || r.Reports[0] != first || r.Reports[1].Base != f.baseOverride || r.Reports[1].PR != first.PR {
		t.Fatal("original review not retained alongside new version", r.Reports)
	}
}

func TestMaintenanceRunnerInputRequiresSteerAgentDecision(t *testing.T) {
	s, _, r := fixture(t)
	r.Round = 1
	r.Operations = []m.Operation{{
		ID: m.ID(), State: "complete", Purpose: "implement",
		Run:    &rc.Run{Status: "completed", Final: &rc.Final{Verified: true, NativeOutcome: "succeeded"}},
		Result: &m.Result{Outcome: "needs_input", Question: "是否保留接口？"},
	}}
	s.applyExecution(&r, r.Operations[0])
	f := s.context(r, s.now())
	if r.Phase != "audit" || r.Question != nil || len(f.Feedback) != 1 || f.Feedback[0].Result.Question != r.Operations[0].Result.Question {
		t.Fatal("Runner input bypassed SteerAgent or was omitted from its context")
	}
	r.Operations = append(r.Operations, m.Operation{ID: m.ID(), Purpose: "audit", State: "complete"})
	ask := decision.Decision{Kind: "ask", Summary: "范围取舍", Question: json.RawMessage(`{"prompt":"如何处理？","facts":["有外部调用"],"attempts":[],"options":[{"id":"keep","label":"保留","impact":"兼容"}],"recommended_option_id":"keep","recommendation_reason":"保持原行为"}`)}
	if err := s.applyDecision(&r, "audit", ask); err != nil || r.Phase != "waiting_input" || r.Question == nil || r.Question.Mode != "audit" {
		t.Fatal("SteerAgent question did not enter the original audit wait", r.Phase, r.Question, err)
	}
}

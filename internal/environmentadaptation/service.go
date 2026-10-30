package environmentadaptation

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	envmodel "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/prompts"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

type Store interface {
	Environments(context.Context, string) ([]envmodel.Record, error)
	Environment(context.Context, string, string) (envmodel.Record, error)
	SaveEnvironment(context.Context, envmodel.Record, uint64) error
	StartEnvironment(context.Context, envmodel.Record, uint64) error
}
type Runner interface {
	Call(context.Context, string, []string, *runnerclient.Request) (runnerclient.Response, error)
	Inspect(context.Context, string, string, bool) (runnerclient.Inspection, error)
	ProgressFile(context.Context, string) (runnercontract.ProgressFile, error)
	ResultFile(context.Context, string) (runnercontract.ResultFile, error)
}
type Service struct {
	Project           string
	Store             Store
	Runner            Runner
	Build             runnercontract.BuildPin
	Prepare           func(context.Context, string) (envmodel.Prepared, error)
	PrepareGeneration func(context.Context, string) (runnercontract.GenerateRunInput, error)
	Admit             func(context.Context) error
	WithTarget        func(context.Context, string, string, func() error) error
	AcquireGeneration func(envmodel.Record) bool
	ReleaseGeneration func(envmodel.Record)
	Now               func() time.Time
	mu                sync.Mutex
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
func (s *Service) save(ctx context.Context, r *envmodel.Record) error {
	old := r.Revision
	r.Revision++
	if e := s.Store.SaveEnvironment(ctx, *r, old); e != nil {
		r.Revision = old
		return e
	}
	return nil
}
func (s *Service) original(ctx context.Context, id string, raw json.RawMessage) (envmodel.Record, error) {
	a, e := runnercontract.CanonicalJSON(raw)
	if e != nil {
		return envmodel.Record{}, e
	}
	rows, e := s.Store.Environments(ctx, s.Project)
	if e != nil {
		return envmodel.Record{}, e
	}
	for _, r := range rows {
		for _, q := range r.Requests {
			if q.ID == id {
				b, e := runnercontract.CanonicalJSON(q.Original)
				if e != nil {
					return r, e
				}
				if string(a) != string(b) {
					return r, errors.New("request_conflict: immutable environment request differs")
				}
				return r, nil
			}
		}
	}
	return envmodel.Record{}, envmodel.ErrNotFound
}
func (s *Service) Start(ctx context.Context, id string, original json.RawMessage, in envmodel.Start, generation bool) (envmodel.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !envmodel.IDPattern.MatchString(id) || !json.Valid(original) || in.Profile == "" {
		return envmodel.Record{}, errors.New("invalid environment request/profile")
	}
	if old, e := s.original(ctx, id, original); !errors.Is(e, envmodel.ErrNotFound) {
		return old, e
	}
	if s.Admit != nil {
		if e := s.Admit(ctx); e != nil {
			return envmodel.Record{}, e
		}
	}
	r := envmodel.Record{ID: id, Project: s.Project, Profile: in.Profile, Build: s.Build, CreatedAt: s.now(), State: "queued", Requests: []envmodel.Request{{ID: id, Original: original, Action: "start"}}, Rounds: []envmodel.Round{}}
	if generation {
		if s.PrepareGeneration == nil {
			return r, errors.New("environment generation unavailable")
		}
		input, e := s.PrepareGeneration(ctx, in.Profile)
		if e != nil {
			return r, e
		}
		if input.Generate.Repository == nil || len(input.Generate.Materials) != 0 {
			return r, errors.New("材料准备不足：环境生成必须使用固定仓库文件，不能 inline")
		}
		input.Correlation = map[string]string{"environment_activity": id, "steerline_project": s.Project}
		r.Kind = "generation"
		r.Execution = input.Execution
		raw, _ := json.Marshal(input)
		r.Rounds = []envmodel.Round{{Number: 1, Request: runnercontract.Request{Protocol: runnercontract.Protocol, RequestID: envmodel.ID(), Input: raw}, Submission: "pending", Deadline: s.now().Add(time.Hour)}}
		if e := s.save(ctx, &r); e != nil {
			return r, e
		}
		return r, nil
	}
	if in.TimeoutSeconds <= 0 || int64(in.TimeoutSeconds) > int64(^uint64(0)>>1)/int64(time.Second) {
		return r, errors.New("round_timeout_seconds must be positive and fit a duration")
	}
	prepared, e := s.Prepare(ctx, in.Profile)
	if e != nil {
		return r, e
	}
	r.Kind = "adaptation"
	r.Prepared = prepared
	r.Execution = prepared.Input.Execution
	r.TimeoutSeconds = in.TimeoutSeconds
	if _, err := envmodel.Key(r.Execution); err != nil {
		return r, errors.New("environment adaptation requires a supported execution profile")
	}
	create := func() error {
		rows, e := s.Store.Environments(ctx, s.Project)
		if e != nil {
			return e
		}
		for _, old := range rows {
			if old.Kind == "adaptation" && envmodel.Target(old.Execution) == envmodel.Target(r.Execution) && !old.Settled() {
				return errors.New("original environment activity remains unsettled")
			}
		}
		if err := s.appendRound(&r); err != nil {
			return err
		}
		r.Revision = 1
		return s.Store.StartEnvironment(ctx, r, in.ExpectedCurrentRevision)
	}
	if s.WithTarget != nil {
		e = s.WithTarget(ctx, envmodel.Target(r.Execution), r.Project+"\n"+r.ID, create)
	} else {
		e = create()
	}
	return r, e
}
func (s *Service) appendRound(r *envmodel.Record) error {
	n := len(r.Rounds) + 1
	input := r.Prepared.Input
	input.NamePrefix = "sl-env"
	input.Title = "环境适配: " + r.Profile
	input.Correlation = map[string]string{"steerline_project": r.Project, "environment_activity": r.ID, "environment_round": fmt.Sprint(n)}
	var previous json.RawMessage
	if n > 1 {
		previous = PreviousFacts(*r, r.Rounds[n-2])
	}
	initial, err := json.Marshal(envmodel.RoundFile{envmodel.Report{Version: envmodel.ReportVersion, ActivityID: r.ID, Round: n, Commit: r.Prepared.Commit, ScriptDigest: r.Prepared.ScriptDigest, Checks: []envmodel.Check{}, L2: []envmodel.Item{}}, previous})
	if err != nil {
		return err
	}
	if len(initial) > runnercontract.MaxProgressBytes {
		r.State = "failed"
		return errors.New("材料准备不足：完整前轮报告与本轮身份超过结果文件上限；未创建下一 Run")
	}
	input.ResultFile = &runnercontract.ProgressFileInput{InitialContentBase64: base64.StdEncoding.EncodeToString(initial)}
	progress := map[string]bool{"pr": false}
	for _, id := range r.Prepared.L2 {
		progress["test."+id] = false
	}
	raw, _ := json.Marshal(progress)
	input.ProgressFile = &runnercontract.ProgressFileInput{InitialContentBase64: base64.StdEncoding.EncodeToString(raw)}
	prepare, err := prompts.EnvironmentPrepare()
	if err != nil {
		return err
	}
	publish, err := prompts.EnvironmentPublish()
	if err != nil {
		return err
	}
	input.Workflow.PromptOverrides = map[string]string{"implement.execute": prepare, "publish": publish}
	body, _ := json.Marshal(input)
	r.Rounds = append(r.Rounds, envmodel.Round{Number: n, Request: runnercontract.Request{Protocol: runnercontract.Protocol, RequestID: envmodel.ID(), Input: body}, Submission: "pending", Deadline: s.now().Add(time.Duration(r.TimeoutSeconds) * time.Second)})
	r.State = "running"
	return nil
}
func (s *Service) Stop(ctx context.Context, id, request string, revision uint64, original json.RawMessage) (envmodel.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !envmodel.IDPattern.MatchString(request) || !json.Valid(original) {
		return envmodel.Record{}, errors.New("invalid stop request")
	}
	if old, e := s.original(ctx, request, original); !errors.Is(e, envmodel.ErrNotFound) {
		return old, e
	}
	r, e := s.Store.Environment(ctx, s.Project, id)
	if e != nil {
		return r, e
	}
	if r.Revision != revision {
		return r, errors.New("stale_revision: refresh environment activity")
	}
	if r.Settled() {
		return r, errors.New("environment activity already settled")
	}
	r.Requests = append(r.Requests, envmodel.Request{ID: request, Original: original, Action: "stop"})
	r.Cancelled = true
	r.State = "stopping"
	e = s.save(ctx, &r)
	return r, e
}
func (s *Service) Observe(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, e := s.Store.Environments(ctx, s.Project)
	if e != nil {
		return e
	}
	var all []error
	for _, r := range rows {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if r.Settled() {
			continue
		}
		if e := s.observe(ctx, &r); e != nil {
			r.Diagnostic = e.Error()
			all = append(all, e)
			if saveErr := s.save(ctx, &r); saveErr != nil {
				all = append(all, saveErr)
			}
		}
	}
	return errors.Join(all...)
}
func (s *Service) observe(ctx context.Context, r *envmodel.Record) error {
	if len(r.Rounds) == 0 {
		return errors.New("environment activity has no original operation")
	}
	a := &r.Rounds[len(r.Rounds)-1]
	if a.Submission == "pending" {
		if r.Cancelled {
			a.Settled = true
			r.State = "cancelled"
			return s.finish(ctx, r)
		}
		if !s.now().Before(a.Deadline) {
			a.Settled = true
			a.TimedOut = true
			r.State = "failed"
			r.Diagnostic = "轮次尚未发送即超时"
			return s.finish(ctx, r)
		}
		if r.Kind == "generation" && s.AcquireGeneration != nil && !s.AcquireGeneration(*r) {
			return nil
		}
		if s.Admit != nil {
			if e := s.Admit(ctx); e != nil {
				return e
			}
		}
		if !s.now().Before(a.Deadline) {
			a.Settled = true
			a.TimedOut = true
			r.State = "failed"
			r.Diagnostic = "轮次尚未发送即超时"
			return s.finish(ctx, r)
		}
		a.Submission = "unknown"
		if e := s.save(ctx, r); e != nil {
			return e
		}
		_, e := s.Runner.Call(ctx, "submit", nil, &a.Request)
		if e != nil {
			var f *runnerclient.Failure
			if errors.As(e, &f) && !f.Unknown && f.Exit != 4 {
				a.Submission = "rejected"
				a.Diagnostic = e.Error()
				a.Settled = true
				return s.next(ctx, r)
			}
			return e
		}
		a.Submission = "accepted"
		return s.save(ctx, r)
	}
	selector, id := "run-id", a.RunID
	if id == "" {
		selector, id = "request-id", a.Request.RequestID
	}
	v, e := s.Runner.Inspect(ctx, selector, id, false)
	if e != nil {
		if a.Run == nil || !(r.Cancelled || !s.now().Before(a.Deadline) || a.Stop != nil) {
			return e
		}
		// Only the already verified original binding may be used for cleanup;
		// a read outage cannot suspend cancellation or extend its deadline.
		v.Run = a.Run
		a.Diagnostic = "original Run read unavailable; preserving its stop obligation: " + e.Error()
	}
	if v.Run == nil {
		if v.Command != nil && (v.Command.ReceiptStatus == "rejected" || v.Command.ActionStatus == "rejected") {
			a.Submission = "rejected"
			a.Settled = true
			return s.next(ctx, r)
		}
		return errors.New("original environment submission remains unverified")
	}
	run := v.Run
	if a.RunID != "" && a.RunID != run.RunID || a.InputDigest != "" && a.InputDigest != run.InputDigest || run.InputDigest == "" {
		return errors.New("environment Run binding differs")
	}
	if a.Run != nil && run.Revision < a.Run.Revision {
		run = a.Run // Keep the trusted binding; a stale read must not suspend its deadline.
	}
	if run.Correlation["environment_activity"] != r.ID || run.Correlation["steerline_project"] != r.Project || r.Kind == "adaptation" && run.Correlation["environment_round"] != fmt.Sprint(a.Number) {
		return errors.New("environment Run correlation differs")
	}
	a.RunID, a.InputDigest, a.Run = run.RunID, run.InputDigest, run
	a.Submission = "accepted"
	if r.Kind == "generation" {
		if result := run.ContentResult; result != nil && (result.Status != "succeeded" || !result.Source.Verified || result.Source.NativeOutcome != "succeeded" || result.Source.CommandID == "" || result.Source.TurnID == "") {
			a.Diagnostic = "材料生成或原生结果来源未核实：" + result.Diagnostic
		}
		if result := run.ContentResult; result != nil && result.Status == "succeeded" && result.Source.Verified && result.Source.NativeOutcome == "succeeded" && result.Source.CommandID != "" && result.Source.TurnID != "" {
			var g envmodel.Generated
			if e := envmodel.Decode(run.ContentResult.JSON, &g); e != nil {
				a.Diagnostic = e.Error()
			} else if _, e := envmodel.ParseManifest([]byte(g.Document)); e != nil {
				a.Diagnostic = e.Error()
			} else if strings.TrimSpace(g.Script) == "" || g.Gaps == nil {
				a.Diagnostic = "生成脚本为空"
			} else {
				var frozen runnercontract.GenerateRunInput
				if e := json.Unmarshal(a.Request.Input, &frozen); e != nil {
					return e
				}
				var sourceErr error
				if frozen.Generate.Repository != nil {
					left, _ := json.Marshal(result.RepositorySource)
					right, _ := json.Marshal(run.RepositorySource)
					if string(left) != string(right) {
						sourceErr = errors.New("环境生成 Run/result 的固定来源不一致")
					} else {
						sourceErr = runnerclient.VerifyRepositoryFiles(result.RepositorySource, *frozen.Generate.Repository, nil)
					}
				}
				if sourceErr != nil {
					a.Diagnostic = sourceErr.Error()
				} else {
					r.Generation = &g
				}
			}
		}
	} else {
		if p, e := s.Runner.ProgressFile(ctx, a.RunID); e == nil {
			if _, e = runnerclient.ValidateProgressFile(p, a.RunID); e == nil {
				if a.Progress == nil || p.Version >= a.Progress.Version && p.SHA256 == a.Progress.SHA256 || p.Version > a.Progress.Version {
					a.Progress = &p
				}
			}
		}
	}
	due := !s.now().Before(a.Deadline)
	if due {
		a.TimedOut = true
	}
	terminal := run.Closed || run.Status == "pr_ready" || run.Status == "completed" || run.Status == "stopped" || run.Status == "failed" || run.Status == "cancelled" || run.StopStatus == "stopped" || r.Kind == "adaptation" && (run.Status == "waiting_input" || run.Status == "blocked") && run.Final != nil && run.Final.Verified
	stop := r.Cancelled || due || terminal || r.Generation != nil || r.Kind == "generation" && run.ContentResult != nil
	if stop && run.StopStatus != "stopped" {
		if due {
			a.Diagnostic = "单轮期限已到，等待原执行停止核实"
		}
		if a.Stop == nil {
			body, _ := json.Marshal(map[string]uint64{"expected_revision": run.Revision})
			a.Stop = &runnercontract.Request{Protocol: runnercontract.Protocol, RequestID: envmodel.ID(), Input: body}
			a.StopState = "pending"
		}
		if a.StopState == "rejected" {
			a.RejectedStops = append(a.RejectedStops, *a.Stop)
			body, _ := json.Marshal(map[string]uint64{"expected_revision": run.Revision})
			a.Stop = &runnercontract.Request{Protocol: runnercontract.Protocol, RequestID: envmodel.ID(), Input: body}
			a.StopState = "pending"
		}
		if a.StopState == "pending" {
			a.StopState = "unknown"
			if e := s.save(ctx, r); e != nil {
				return e
			}
			_, e := s.Runner.Call(ctx, "stop", []string{"--run-id", a.RunID}, a.Stop)
			if e != nil {
				var f *runnerclient.Failure
				if errors.As(e, &f) && !f.Unknown && f.Exit != 4 {
					a.StopState = "rejected"
				}
				return e
			}
			a.StopState = "accepted"
		} else {
			v, e := s.Runner.Inspect(ctx, "request-id", a.Stop.RequestID, false)
			if e != nil {
				return e
			}
			if v.Command != nil && (v.Command.ReceiptStatus == "rejected" || v.Command.ActionStatus == "rejected" || v.Command.ActionStatus == "failed") {
				a.StopState = "rejected"
			}
		}
	}
	if run.StopStatus == "stopped" {
		if r.Kind == "adaptation" {
			r.State = "unverified"
			// Only query persisted result facts after stopping. A sealed success or
			// failure is permanent, including across application restarts.
			if a.Result == nil || a.Result.CollectionStatus != "closed" {
				p, e := s.Runner.ResultFile(ctx, a.RunID)
				if e != nil {
					a.Diagnostic = e.Error()
					return e
				}
				if _, e := runnerclient.ValidateResultFile(p, a.RunID); e != nil {
					a.Diagnostic = e.Error()
					return e
				}
				a.Result = &p
			}
			raw, e := runnerclient.ValidateResultFile(*a.Result, a.RunID)
			if e != nil {
				return e
			}
			if a.Result.CollectionStatus != "closed" {
				a.Diagnostic = "原Run已停止，最终结果尚未封存：" + a.Result.CollectionStatus
				return errors.New(a.Diagnostic)
			}
			a.Report = nil
			if a.Result.Version == 0 {
				a.Diagnostic = "最终结果采集失败（同一Run不再采集）：" + a.Result.Diagnostic
			} else {
				a.Report, e = envmodel.ParseReport(raw, *r, *a)
				if e != nil {
					a.Diagnostic = e.Error()
				} else if !a.Report.Completed {
					a.Diagnostic = "验收脚本未完成，不能判定环境通过"
					if run.Diagnostic != "" {
						a.Diagnostic += "：" + run.Diagnostic
					}
				} else {
					a.Diagnostic = a.Report.Message
				}
			}
		}
		a.Settled = true
		return s.next(ctx, r)
	}
	return s.save(ctx, r)
}

func (s *Service) next(ctx context.Context, r *envmodel.Record) error {
	a := r.Rounds[len(r.Rounds)-1]
	r.Diagnostic = a.Diagnostic
	// Before publish, the existing development workflow proves no PR stage
	// was dispatched. A stopped/cancelled prepare need not invent a report.
	beforePR := a.Run != nil && a.Run.PR == nil && (a.Run.Phase == "implement" || strings.HasPrefix(a.Run.Step, "implement.")) && (a.Report == nil || a.Report.PR.URL == "" && !a.Report.PR.OutcomeUnknown)
	safePR := a.Report != nil && a.Report.Completed && !a.Report.PR.OutcomeUnknown && (a.Report.PR.URL == "" || a.Report.PR.CleanupCompleted)
	if (a.Report != nil && a.Report.PR.URL != "" || a.Run != nil && a.Run.PR != nil) && !verifiedPublication(r.Prepared.Input.Publication, a) {
		safePR = false
	}
	if !safePR && (r.Cancelled || a.TimedOut) && beforePR {
		safePR = true
	}
	if r.Kind == "adaptation" && a.Submission != "rejected" && !safePR {
		r.State = "unverified"
		if r.Diagnostic == "" {
			r.Diagnostic = "原报告/PR未核实，须先处理原资源；禁止开始新活动"
		}
	} else if r.Cancelled {
		r.State = "cancelled"
	} else if r.Kind == "generation" {
		if r.Generation != nil && !a.TimedOut {
			r.State = "generated"
		} else {
			r.State = "failed"
		}
	} else if !a.TimedOut && a.Report != nil && a.Run != nil && a.Report.Passed(r.Prepared.Manifest.ValidationBase, a.Run.Execution.Branch) {
		r.State = "ready"
		r.Diagnostic = ""
	} else if a.Submission == "rejected" {
		r.State = "failed"
	} else if len(r.Rounds) < envmodel.MaxRounds {
		if err := s.appendRound(r); err != nil {
			r.Diagnostic = err.Error()
		}
	} else {
		r.State = "failed"
	}
	return s.finish(ctx, r)
}

// The script reports publication; the original Runner observation independently
// binds it to the frozen repository and exact revision. Missing facts stay
// unverified rather than opening another round that could publish a second PR.
func verifiedPublication(target runnercontract.Publication, a envmodel.Round) bool {
	if a.Report == nil || a.Run == nil || a.Run.PR == nil {
		return false
	}
	p, fact := a.Report.PR, a.Run.PR
	return fact.QueryError == "" && (fact.State == "open" || fact.Merged || fact.State == "merged") && target.Provider != "" && fact.Provider == target.Provider && target.Repository != "" && fact.Repository == target.Repository && fact.HeadRepository == target.Repository && fact.URL == p.URL && fact.BaseBranch == target.BaseBranch && p.Base == target.BaseBranch && fact.HeadBranch == a.Run.Execution.Branch && p.Head == fact.HeadBranch && p.HeadRevision != "" && fact.HeadRevision == p.HeadRevision
}
func (s *Service) finish(ctx context.Context, r *envmodel.Record) error {
	e := s.save(ctx, r)
	if e == nil && r.Kind == "generation" && r.Settled() && s.ReleaseGeneration != nil {
		s.ReleaseGeneration(*r)
	}
	return e
}

// Close persists cancellation before issuing native stops. It never clears facts.
func (s *Service) Close(ctx context.Context) error {
	s.mu.Lock()
	rows, e := s.Store.Environments(ctx, s.Project)
	if e == nil {
		for _, r := range rows {
			if !r.Settled() {
				r.Cancelled = true
				r.State = "stopping"
				if err := s.save(ctx, &r); err != nil {
					e = errors.Join(e, err)
				}
			}
		}
	}
	s.mu.Unlock()
	if e != nil {
		return e
	}
	return s.Observe(ctx)
}

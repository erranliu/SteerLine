package environmentadaptation

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/erranliu/SteerLine/internal/executionconfig"
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
	Inspect(context.Context, string, string) (runnerclient.Inspection, error)
	ResultFile(context.Context, string) (runnercontract.ResultFile, error)
}
type Service struct {
	Project    string
	Store      Store
	Runner     Runner
	Build      runnercontract.BuildPin
	Prepare    func(context.Context, string) (envmodel.Prepared, error)
	Admit      func(context.Context) error
	WithTarget func(context.Context, string, string, string, func() error) error
	Now        func() time.Time
	mu         sync.Mutex
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
func (s *Service) Start(ctx context.Context, id string, original json.RawMessage, in envmodel.Start) (envmodel.Record, error) {
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
	r := envmodel.Record{FormatVersion: 1, ID: id, Project: s.Project, Profile: in.Profile, Build: s.Build, CreatedAt: s.now(), State: "queued", Requests: []envmodel.Request{{ID: id, Original: original, Action: "start"}}, Rounds: []envmodel.Round{}}
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
	key, keyErr := envmodel.Key(r.Execution, r.Prepared.Identity)
	if keyErr != nil || key != in.EnvironmentKey {
		return r, errors.New("selected environment and execution profile differ")
	}
	r.TimeoutSeconds = in.TimeoutSeconds
	if _, err := envmodel.Key(r.Execution, r.Prepared.Identity); err != nil {
		return r, errors.New("environment adaptation requires a supported execution profile")
	}
	create := func() error {
		rows, e := s.Store.Environments(ctx, s.Project)
		if e != nil {
			return e
		}
		for _, old := range rows {
			if old.Kind == "adaptation" && executionconfig.Target(old.Execution) == executionconfig.Target(r.Execution) && !old.Settled() {
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
		e = s.WithTarget(ctx, executionconfig.Target(r.Execution), r.Project+"\n"+r.ID, r.Prepared.Commit, create)
	} else {
		e = create()
	}
	return r, e
}
func (s *Service) appendRound(r *envmodel.Record) error {
	if len(r.Rounds) != 0 {
		return errors.New("detection is a single invocation")
	}
	input := r.Prepared.Input
	if err := runnercontract.ValidateExecutionIdentity(r.Execution, r.Prepared.Identity); err != nil {
		return err
	}
	input.Execution = r.Prepared.DerivedExecution
	if input.Execution.Adapter != r.Execution.Adapter || input.Execution.Target != r.Execution.Target || len(input.Execution.Settings) == 0 {
		return errors.New("derived execution differs from the selected environment")
	}
	input.NamePrefix = "sl-env"
	input.Title = "项目环境检测: " + r.Profile
	input.Correlation = map[string]string{"steerline_project": r.Project, "environment_activity": r.ID, "environment_round": "1"}
	initial, err := json.Marshal(envmodel.InitialReport(*r, 1))
	if err != nil {
		return err
	}
	input.ResultFile = &runnercontract.ProgressFileInput{InitialContentBase64: base64.StdEncoding.EncodeToString(initial)}
	input.ProgressFile = nil
	instruction, err := prompts.EnvironmentInvoke(r.Prepared.Manifest.Interpreter, r.Prepared.Manifest.Script)
	if err != nil {
		return err
	}
	input.Workflow.PromptOverrides = map[string]string{"execute": instruction}
	body, err := json.Marshal(input)
	if err != nil {
		return err
	}
	r.Rounds = []envmodel.Round{{Number: 1, Request: runnercontract.Request{Protocol: runnercontract.Protocol, RequestID: envmodel.ID(), Input: body}, Submission: "pending", Deadline: s.now().Add(time.Duration(r.TimeoutSeconds) * time.Second)}}
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
			// Persist the actual cause even if the external call exhausted its
			// budget or the project stopped. This only extends the local write.
			saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			if saveErr := s.save(saveCtx, &r); saveErr != nil {
				all = append(all, saveErr)
			}
			cancel()
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
			return s.save(ctx, r)
		}
		if !s.now().Before(a.Deadline) {
			a.Settled = true
			a.TimedOut = true
			r.State = "failed"
			r.Diagnostic = "轮次尚未发送即超时"
			return s.save(ctx, r)
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
			return s.save(ctx, r)
		}
		a.Submission = "unknown"
		if e := s.save(ctx, r); e != nil {
			// No external call crossed this failed persistence boundary.
			a.Submission = "pending"
			return e
		}
		e := ctx.Err()
		unattempted := e != nil
		if !unattempted {
			_, e = s.Runner.Call(ctx, "submit", nil, &a.Request)
		}
		saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if e != nil {
			a.SubmissionDiagnostic = e.Error()
			var f *runnerclient.Failure
			if unattempted || errors.As(e, &f) && !f.Unknown && f.Exit != 4 {
				a.Submission = "rejected"
				a.Diagnostic = e.Error()
				a.Settled = true
				return s.next(saveCtx, r)
			}
			return e
		}
		a.Submission = "accepted"
		return s.save(saveCtx, r)
	}
	selector, id := "run-id", a.RunID
	if id == "" {
		selector, id = "request-id", a.Request.RequestID
	}
	v, e := s.Runner.Inspect(ctx, selector, id)
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
	due := !s.now().Before(a.Deadline)
	if due {
		a.TimedOut = true
	}
	terminal := run.Closed || run.Status == "completed" || run.Status == "stopped" || run.Status == "failed" || run.Status == "cancelled" || run.StopStatus == "stopped" || r.Kind == "adaptation" && (run.Status == "waiting_input" || run.Status == "blocked") && run.Final != nil && run.Final.Verified
	stop := r.Cancelled || due || terminal
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
			v, e := s.Runner.Inspect(ctx, "request-id", a.Stop.RequestID)
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
	if r.Cancelled {
		r.State = "cancelled"
	} else if !a.TimedOut && a.Report != nil && a.Report.Passed() && envmodel.TrustedFinal(a) {
		r.State = "ready"
	} else {
		r.State = "failed"
		if r.Diagnostic == "" {
			r.Diagnostic = "项目检测未就绪或原调用终态未核实"
		}
	}
	return s.save(ctx, r)
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

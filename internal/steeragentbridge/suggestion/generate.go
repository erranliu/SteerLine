package suggestionbridge

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/erranliu/SteerLine/internal/runnerbinding"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/steeragent/suggestion"
)

func token() string {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b[:])
}

func (s *Service) generateRequest(ctx context.Context, p suggestion.Prepared) (*runnercontract.Request, error) {
	if s.Prepare == nil || s.Runner == nil {
		return nil, errors.New("Runner generate 未配置")
	}
	execution, catalog, err := s.Prepare(ctx)
	if err != nil {
		return nil, err
	}
	input := runnercontract.GenerateRunInput{Title: "后续对话提示词建议", Correlation: map[string]string{"steerline_project": s.Project, "steerline_advisory": p.RequestID}, Workflow: runnercontract.Workflow{ID: "generate", Version: 1}, Execution: execution, Generate: runnercontract.GenerateInput{Instruction: p.Text, Materials: []runnercontract.Document{}}}
	if err = catalog.ValidateGenerate(input); err != nil {
		return nil, err
	}
	body, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	return &runnercontract.Request{Protocol: runnercontract.Protocol, RequestID: token(), Input: body}, nil
}

func Content(r Record) runnerbinding.Content {
	var request runnercontract.Request
	if r.RunnerRequest != nil {
		request = *r.RunnerRequest
	}
	var revision uint64
	if r.Projection != nil {
		revision = r.Projection.Revision
	}
	return runnerbinding.Content{StateDir: r.Runner.StateDir, Request: request, RunID: r.RunID, InputDigest: r.RunnerDigest, Revision: revision, Project: r.Project, ObjectKey: "steerline_advisory", ObjectID: r.ID, WorkflowID: "generate", WorkflowVersion: 1, RequireDigest: true, Unsubmitted: r.Submission != "started", Rejected: r.Submission == "rejected"}
}

// Every external call happens outside the state gate; cancellation can persist
// while submission/inspection is in flight. Updates always reload that decision.
func (s *Service) update(ctx context.Context, id string, apply func(*Record) error) (Record, error) {
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	if e := s.enter(saveCtx); e != nil {
		return Record{}, e
	}
	defer s.leave()
	r, e := s.load(saveCtx, id)
	if e != nil {
		return r, e
	}
	if e = apply(&r); e != nil {
		return r, e
	}
	return s.save(saveCtx, r)
}
func (s *Service) advance(ctx context.Context, id string) error {
	r, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if r.Complete() {
		return nil
	}
	if !runnerbinding.SamePath(r.Runner.StateDir, s.RunnerBuild.StateDir) {
		return errors.New("原生成 Runner namespace 不同")
	}
	if r.Submission == "started" {
		return s.observe(ctx, r)
	}
	if s.Admit != nil {
		if err = s.Admit(ctx); err != nil {
			return err
		}
	}
	if err = s.enter(ctx); err != nil {
		return err
	}
	r, err = s.load(ctx, id)
	if err != nil {
		s.leave()
		return err
	}
	if r.Complete() || r.Cancelled {
		s.leave()
		return nil
	}
	if s.Queue != nil && !s.Queue.AcquireContent(s.Project, "advisory:"+r.ID) {
		s.leave()
		return nil
	}
	before := r
	r.Submission, r.State, r.Settled = "started", "unknown", false
	r, err = s.save(ctx, r)
	s.leave()
	if err != nil {
		s.track(before)
		return err
	}
	receipt, callErr := s.Runner.Submit(ctx, *r.RunnerRequest)
	_, err = s.update(ctx, id, func(current *Record) error {
		if callErr == nil && receipt.RunID != "" && receipt.CommandID != "" && receipt.ReceiptStatus == "accepted" {
			current.RunID = receipt.RunID
			if !current.Cancelled {
				current.State = "running"
			}
		} else {
			current.Diagnostic = "原生成提交结果待核实"
			if callErr != nil {
				current.Diagnostic = callErr.Error()
			}
			var failure *runnerclient.Failure
			if errors.As(callErr, &failure) && !failure.Unknown {
				current.Submission = "rejected"
				if current.Cancelled {
					finish(current, "cancelled", current.Diagnostic)
				} else {
					finish(current, "failed", current.Diagnostic)
				}
			}
		}
		return nil
	})
	return err
}
func (s *Service) observe(ctx context.Context, r Record) error {
	binding := Content(r)
	selector, id := binding.Selector()
	view, err := s.Runner.Inspect(ctx, selector, id)
	if err != nil {
		return err
	}
	if err = runnerbinding.ValidateContent(binding, view); err != nil {
		return err
	}
	run := view.Run
	current, err := s.update(ctx, r.ID, func(current *Record) error {
		// Another operation can cancel this job, but never replace the original Run.
		if err := runnerbinding.ValidateContent(Content(*current), view); err != nil {
			return err
		}
		current.RunID, current.RunnerDigest, current.Projection = run.RunID, run.InputDigest, run
		if current.Cancelled {
			current.State = "cancelling"
			if run.StopStatus == "stopped" {
				finish(current, "cancelled", "本次提示词生成已取消")
			}
		} else if current.State != "failed" {
			current.State, current.Diagnostic = "running", run.Diagnostic
			result := run.ContentResult
			if result != nil && result.Status == "succeeded" {
				source := result.Source
				if run.Final == nil || *run.Final != source || source.Text != result.Text || source.AttemptID == "" || source.Source == "" || source.ItemID == "" || !source.Verified || source.NativeOutcome != "succeeded" || source.CommandID == "" || source.SessionID == "" || source.TurnID == "" || source.AttemptID != run.Execution.AttemptID || source.SessionID != run.Execution.SessionID || source.TurnID != run.Execution.TurnID || result.RepositorySource != nil || len(result.FileMaterials) != 0 || run.RepositorySource != nil || len(run.Materials) != 0 {
					current.State, current.Diagnostic = "failed", "生成结果完整来源未核实"
				} else {
					suggestion, e := suggestion.Accept(suggestion.Prepared{RequestID: current.ID, Text: current.PreparedText, Digest: current.InputDigest}, result.Text)
					if e != nil {
						current.State, current.Diagnostic = "failed", e.Error()
					} else {
						current.Suggestion = &suggestion
						if run.Status == "completed" && run.StopStatus == "stopped" {
							finish(current, "ready", "")
						} else {
							current.State = "settling"
						}
					}
				}
			} else if result != nil && result.Status != "succeeded" || run.Status == "failed" || run.Status == "stopped" || run.Status == "completed" {
				current.State, current.Diagnostic = "failed", "Runner 未返回经核实的完整提示词"
			}
		}
		if current.State == "failed" && run.StopStatus == "stopped" {
			finish(current, "failed", current.Diagnostic)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if current.Complete() || !current.Cancelled && current.State != "failed" {
		return nil
	}
	if current.StopRequest != nil && current.StopState == "unknown" {
		state, readErr := InspectOriginalStop(ctx, current, run, s.Runner.Inspect)
		if readErr != nil {
			return readErr
		}
		if state == "unknown" {
			return nil
		}
		originalID := current.StopRequest.RequestID
		current, err = s.update(ctx, r.ID, func(c *Record) error {
			if c.StopRequest == nil || c.StopRequest.RequestID != originalID {
				return errors.New("original advisory stop changed during inspection")
			}
			if !c.Complete() {
				c.StopState = state
			}
			return nil
		})
		if err != nil || current.Complete() {
			return err
		}
	}
	if !runnerbinding.NeedsContentStop(current.StopRequest, current.StopState, run.StopStatus) {
		return nil
	}
	current, err = s.update(ctx, r.ID, func(c *Record) error {
		if c.Complete() {
			return nil
		}
		c.StopRequest = runnerbinding.ContentStop(token(), run.Revision)
		c.StopState = "unknown"
		return nil
	})
	if err != nil || current.Complete() {
		return err
	}
	response, callErr := s.Runner.Call(ctx, "stop", []string{"--run-id", run.RunID}, current.StopRequest)
	_, err = s.update(ctx, r.ID, func(c *Record) error {
		var receipt runnerclient.Receipt
		if callErr == nil && json.Unmarshal(response.Data, &receipt) == nil && receipt.RunID == run.RunID && receipt.CommandID != "" && receipt.ReceiptStatus == "accepted" {
			c.StopState = "accepted"
		} else {
			c.Diagnostic = "原生成停止结果待核实"
			var failure *runnerclient.Failure
			if errors.As(callErr, &failure) && !failure.Unknown && failure.Code == "stale_revision" {
				c.StopState = "rejected"
			}
		}
		return nil
	})
	return err
}

package environmentadaptation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	envmodel "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func setCheckoutStep(r *envmodel.Record, a *envmodel.Round, step string) error {
	var input runnercontract.RunInput
	if err := json.Unmarshal(a.Request.Input, &input); err != nil {
		return err
	}
	if input.EnvironmentCheck == nil || input.Correlation == nil || input.Execution.Target.Kind != "orca_environment" {
		return errors.New("invalid frozen checkout preparation")
	}
	input.Title = "环境检测仓库准备: " + r.Profile
	input.Correlation["checkout_preparation"] = step
	input.ResultFile = &runnercontract.ProgressFileInput{InitialContentBase64: "e30="}
	if step == "fast_forward" {
		input.FastForward = true
		input.EnvironmentCheck = nil
	} else {
		input.EnvironmentCheck.VerifyOnly = true
	}
	body, err := json.Marshal(input)
	if err != nil {
		return err
	}
	a.Checkout = &envmodel.CheckoutPreparation{Step: step, Request: runnercontract.Request{Protocol: runnercontract.Protocol, RequestID: envmodel.ID(), Input: body}}
	return nil
}

func (s *Service) observeCheckout(ctx context.Context, r *envmodel.Record, a *envmodel.Round) error {
	p := a.Checkout
	due := !s.now().Before(a.Deadline)
	stop := r.Cancelled || due
	if !p.Dispatched {
		if stop {
			return s.finishCheckout(ctx, r, a, "环境检测仓库准备已取消或超时", due)
		}
		if s.Admit != nil {
			if err := s.Admit(ctx); err != nil {
				return err
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if !s.now().Before(a.Deadline) {
			return s.finishCheckout(ctx, r, a, "环境检测仓库准备超时", true)
		}
		p.Dispatched = true
		if err := s.save(ctx, r); err != nil {
			return err
		}
		err := ctx.Err()
		unattempted := err != nil
		if !unattempted {
			_, err = s.Runner.Call(ctx, "submit", nil, &p.Request)
		}
		if err == nil {
			return nil
		}
		var failure *runnerclient.Failure
		if unattempted || errors.As(err, &failure) && !failure.Unknown && failure.Exit != 4 {
			saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			return s.finishCheckout(saveCtx, r, a, err.Error(), false)
		}
		return err
	}

	selector, id := "request-id", p.Request.RequestID
	if p.Run != nil {
		selector, id = "run-id", p.Run.RunID
	}
	view, err := s.Runner.Inspect(ctx, selector, id)
	if err != nil {
		if !stop || p.Run == nil {
			return err
		}
		view.Run = p.Run
	}
	if view.Run == nil {
		if selector == "request-id" && view.Command != nil && view.Command.IsSubmitAdmission(id) {
			if view.Command.SubmitRejected() {
				return s.finishCheckout(ctx, r, a, view.Command.Error.Error(), due)
			}
			if view.Command.SubmitPreparing() {
				return nil
			}
		}
		return errors.New("原 checkout 准备请求尚未核实")
	}
	run := view.Run
	if run.RunID == "" || run.InputDigest == "" ||
		p.Run != nil && (p.Run.RunID != run.RunID || p.Run.InputDigest != run.InputDigest) ||
		run.Correlation["steerline_project"] != r.Project || run.Correlation["environment_activity"] != r.ID ||
		run.Correlation["checkout_preparation"] != p.Step {
		return errors.New("checkout 准备 Run 与原请求不一致")
	}
	if p.Run != nil && run.Revision <= p.Run.Revision {
		run = p.Run
	} else {
		p.Run = run
		if err := s.save(ctx, r); err != nil {
			return err
		}
	}
	if stop && run.StopStatus != "stopped" {
		return s.stopCheckout(ctx, r, p, run)
	}
	if run.StopStatus != "stopped" {
		return nil
	}
	if stop {
		return s.finishCheckout(ctx, r, a, "环境检测仓库准备已取消或超时", due)
	}
	if !envmodel.TrustedFinal(envmodel.Round{Run: run}) {
		diagnostic := run.Diagnostic
		if diagnostic == "" {
			diagnostic = "远端 checkout 准备失败"
		}
		return s.finishCheckout(ctx, r, a, diagnostic, false)
	}
	if p.Step == "fast_forward" {
		if err := setCheckoutStep(r, a, "verify_after_sync"); err != nil {
			return err
		}
		return s.save(ctx, r)
	}
	file, err := s.Runner.ResultFile(ctx, run.RunID)
	if err != nil {
		return err
	}
	raw, err := runnerclient.ValidateResultFile(file, run.RunID)
	if err != nil {
		return err
	}
	if file.CollectionStatus != "closed" {
		return nil
	}
	var result struct {
		Matches *bool `json:"matches"`
	}
	if err := envmodel.Decode(raw, &result); err != nil || result.Matches == nil {
		return s.finishCheckout(ctx, r, a, "远端检测材料核验结果无效", false)
	}
	if !*result.Matches {
		if p.Step != "verify" {
			return s.finishCheckout(ctx, r, a, "远端 checkout 快进后检测材料仍与冻结基线不同", false)
		}
		if err := setCheckoutStep(r, a, "fast_forward"); err != nil {
			return err
		}
	} else {
		a.Checkout = nil
	}
	r.ObservationDiagnostic, r.Diagnostic = "", ""
	return s.save(ctx, r)
}

func (s *Service) stopCheckout(ctx context.Context, r *envmodel.Record, p *envmodel.CheckoutPreparation, run *runnercontract.Run) error {
	if p.Stop != nil {
		view, err := s.Runner.Inspect(ctx, "request-id", p.Stop.RequestID)
		if err != nil {
			return err
		}
		if view.Command == nil || (view.Command.ReceiptStatus != "rejected" && view.Command.ActionStatus != "rejected" && view.Command.ActionStatus != "failed") {
			return nil
		}
	}
	body := json.RawMessage(fmt.Sprintf("{\"expected_revision\":%d}", run.Revision))
	p.Stop = &runnercontract.Request{Protocol: runnercontract.Protocol, RequestID: envmodel.ID(), Input: body}
	if err := s.save(ctx, r); err != nil {
		return err
	}
	_, err := s.Runner.Call(ctx, "stop", []string{"--run-id", run.RunID}, p.Stop)
	var failure *runnerclient.Failure
	if errors.As(err, &failure) && !failure.Unknown && failure.Exit != 4 {
		p.Stop = nil
		saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		return errors.Join(err, s.save(saveCtx, r))
	}
	return err
}

func (s *Service) finishCheckout(ctx context.Context, r *envmodel.Record, a *envmodel.Round, diagnostic string, timedOut bool) error {
	a.Diagnostic, a.TimedOut, a.Settled = diagnostic, timedOut, true
	return s.next(ctx, r)
}

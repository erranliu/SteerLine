package environmentadaptation_test

import (
	"context"
	"encoding/json"
	"testing"

	envmodel "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func TestRemoteCheckoutPreparation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		steps []struct{ operation, result string }
		ready bool
	}{
		{name: "matching checkout", steps: []struct{ operation, result string }{{"verify", `{"matches":true}`}}, ready: true},
		{name: "fast-forward then verify", steps: []struct{ operation, result string }{
			{"verify", `{"matches":false}`}, {"fast_forward", `{}`}, {"verify_after_sync", `{"matches":true}`},
		}, ready: true},
		{name: "materials still differ", steps: []struct{ operation, result string }{
			{"verify", `{"matches":false}`}, {"fast_forward", `{}`}, {"verify_after_sync", `{"matches":false}`},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, remote, _, _ := setup(t)
			prepare := s.Prepare
			s.Prepare = func(ctx context.Context, profile string) (envmodel.Prepared, error) {
				p, err := prepare(ctx, profile)
				p.SyncRemoteCheckout = true
				return p, err
			}
			r := start(t, s)
			for i, step := range tc.steps {
				round := read(t, s, r.ID).Rounds[0]
				p := round.Checkout
				if p == nil || p.Step != step.operation || p.Request.RequestID == round.Request.RequestID {
					t.Fatalf("unexpected preparation: %+v", p)
				}
				var input runnercontract.RunInput
				if err := json.Unmarshal(p.Request.Input, &input); err != nil {
					t.Fatal(err)
				}
				if step.operation == "fast_forward" {
					if !input.FastForward || input.EnvironmentCheck != nil {
						t.Fatal("fast-forward request includes detection")
					}
				} else if input.FastForward || input.EnvironmentCheck == nil || !input.EnvironmentCheck.VerifyOnly {
					t.Fatal("verify request can execute detection")
				}
				observe(t, s)
				if remote.submits != i+1 || remote.run.Correlation["checkout_preparation"] != step.operation {
					t.Fatalf("unexpected submit: %+v", remote.run)
				}
				setReport(remote, envmodel.Report{})
				remote.run.StopStatus = "stopped"
				remote.body = []byte(step.result)
				observe(t, s)
			}
			observe(t, s)
			got := read(t, s, r.ID)
			if tc.ready {
				if remote.submits != len(tc.steps)+1 || got.Rounds[0].Checkout != nil || remote.run.Correlation["checkout_preparation"] != "" {
					t.Fatalf("detection did not start: %+v", got)
				}
			} else if remote.submits != len(tc.steps) || got.State != "failed" {
				t.Fatalf("mismatch did not block detection: %+v", got)
			}
		})
	}
}

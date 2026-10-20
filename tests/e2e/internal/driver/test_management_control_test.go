package driver

import (
	"context"
	"testing"
	"time"

	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/product/httpclient"
)

type tmReceiptFailurePort struct {
	submits int
	query   string
	err     error
}

func (p *tmReceiptFailurePort) SubmitControl(context.Context, api.ControlInput) (api.ControlRequest, error) {
	p.submits++
	return api.ControlRequest{}, p.err
}

func (p *tmReceiptFailurePort) WaitControl(_ context.Context, id string, _ time.Duration) (*api.ControlReceipt, error) {
	p.query = id
	return &api.ControlReceipt{RequestID: id, Status: "applied"}, nil
}

func TestTMControlReceiptFailureQueriesOriginalWithoutResubmit(t *testing.T) {
	for _, code := range []string{"request_receipt_persist_failed", "stale_revision"} {
		t.Run(code, func(t *testing.T) {
			p := &tmReceiptFailurePort{err: &httpclient.ClientError{Status: 503, Code: code}}
			r, err := tmControl(context.Background(), p, t.TempDir(), "original", api.ControlInput{RequestID: "original-request", Action: api.StartTask})
			if p.submits != 1 {
				t.Fatal("control was resubmitted", p.submits)
			}
			if code == "request_receipt_persist_failed" {
				if err != nil || r.Status != "applied" || p.query != "original-request" {
					t.Fatal("original accepted request was not reconciled", r, err, p.query)
				}
			} else if err == nil || p.query != "" {
				t.Fatal("known rejection was treated as an unknown receipt", r, err, p.query)
			}
		})
	}
}

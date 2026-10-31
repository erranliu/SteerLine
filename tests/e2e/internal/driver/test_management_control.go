package driver

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/product/httpclient"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
)

type tmControlPort interface {
	SubmitControl(context.Context, api.ControlInput) (api.ControlRequest, error)
	WaitControl(context.Context, string, time.Duration) (*api.ControlReceipt, error)
}

// An outcome-file failure does not establish the business result. Query its ID;
// never manufacture another request or submit the action a second time.
func tmControl(ctx context.Context, port tmControlPort, out, name string, input api.ControlInput) (*api.ControlReceipt, error) {
	if err := controlled.Write(filepath.Join(out, name+"-request.json"), input); err != nil {
		return nil, err
	}
	submitErr, err := submitOriginalControl(ctx, port, input)
	if submitErr != nil {
		if logErr := controlled.Write(filepath.Join(out, name+"-submit-error.json"), map[string]string{"request_id": input.RequestID, "error": submitErr.Error()}); logErr != nil {
			return nil, errors.Join(submitErr, logErr)
		}
	}
	if err != nil {
		return nil, err
	}
	wait, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	receipt, err := port.WaitControl(wait, input.RequestID, time.Second)
	if err != nil {
		return nil, errors.Join(submitErr, err)
	}
	if receipt == nil || receipt.RequestID != input.RequestID {
		return nil, fmt.Errorf("original control receipt differs: %+v", receipt)
	}
	return receipt, controlled.Write(filepath.Join(out, name+".json"), receipt)
}

// Only a failure to persist the transport outcome permits original-ID lookup.
// Return the original submission error separately so reconciliation never erases
// its evidence. No caller may submit the action again after this boundary.
func submitOriginalControl(ctx context.Context, port tmControlPort, input api.ControlInput) (submitErr, err error) {
	request, submitErr := port.SubmitControl(ctx, input)
	if submitErr != nil {
		var failure *httpclient.ClientError
		if !errors.As(submitErr, &failure) || failure.Code != "request_receipt_persist_failed" {
			return submitErr, submitErr
		}
	} else if request.RequestID != input.RequestID {
		return nil, errors.New("control submission changed the original request ID")
	}
	return submitErr, nil
}

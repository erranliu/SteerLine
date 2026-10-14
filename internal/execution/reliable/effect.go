package reliable

import (
	"context"
	"encoding/json"
	"errors"
	b "github.com/erranliu/SteerLine/internal/execution/backend"
)

type EffectResult struct {
	Request               b.LifecycleRequest
	Receipt               b.LifecycleReceipt
	Observer              b.LifecycleObserver
	Err                   error
	Uncertain, Reconciled bool
}

// Invoke receives an already committed immutable intent. An existing intent
// has only the reconciliation route, even when acceptance was never observed.
func Invoke(ctx context.Context, port b.LifecyclePort, reconciler b.LifecycleReconcilePort, request b.LifecycleRequest, reconcile bool) EffectResult {
	result := EffectResult{Request: request.Clone(), Reconciled: reconcile}
	if !reconcile {
		if err := QualifyAction(ctx, port, request); err != nil {
			result.Receipt = b.LifecycleReceipt{Operation: request.Operation, Disposition: b.OperationRejected, Diagnostic: err.Error()}
			result.Err = err
			return result
		}
	}
	if reconcile {
		result.Receipt, result.Observer, result.Err = reconciler.Reconcile(ctx, request.Clone())
	} else {
		result.Receipt, result.Observer, result.Err = port.Execute(ctx, request.Clone())
	}
	result.Uncertain = !reconcile && (result.Receipt.Disposition == b.OperationUnknown || result.Err != nil && result.Receipt.Disposition != b.OperationRejected)
	result.Receipt = result.Receipt.Clone()
	return result
}

// ReconciledResult retains partial native successes if the read-only follow-up
// loses its receipt. The initial observer has already been released by owner.
func ReconciledResult(result EffectResult, original *EffectResult) EffectResult {
	if original == nil {
		return result
	}
	result.Uncertain = true
	validationErr := result.Receipt.ValidateFor(result.Request)
	if result.Err == nil && validationErr == nil && result.Receipt.Disposition != b.OperationUnknown {
		return result
	}
	if result.Observer != nil {
		_ = result.Observer.Close(context.Background())
	}
	fallback := *original
	fallback.Observer = nil
	fallback.Err = errors.Join(original.Err, result.Err, validationErr)
	fallback.Reconciled = true
	return fallback
}

func Outcome(receipt b.LifecycleReceipt, err error) (string, string) {
	status, diagnostic := string(receipt.Disposition), receipt.Diagnostic
	if err != nil {
		diagnostic = err.Error()
		if receipt.Disposition != b.OperationRejected {
			status = "outcome_unknown"
		}
	}
	return status, diagnostic
}

func ReconciliationRequest(result EffectResult) (b.LifecycleRequest, bool) {
	if !result.Uncertain || result.Reconciled {
		return b.LifecycleRequest{}, false
	}
	request := result.Request.Clone()
	if request.Operation.Action == string(b.LifecyclePrepareWorkspace) && len(result.Receipt.Operation.Receipt) > 0 && result.Receipt.ValidateFor(request) == nil {
		request.Operation.Receipt, _ = json.Marshal(result.Receipt)
	}
	return request, true
}

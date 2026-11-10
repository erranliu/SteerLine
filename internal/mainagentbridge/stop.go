package mainagentbridge

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/erranliu/SteerLine/internal/runnerbinding"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

// InspectOriginalStop only reads the original public request. Run must have
// been verified first: absence at a newer monotonic revision proves that the
// old CAS envelope can never be accepted, even if its reply was lost.
func InspectOriginalStop(ctx context.Context, r Record, run *runnercontract.Run, inspect func(context.Context, string, string) (runnercontract.Inspection, error)) (string, error) {
	var original struct {
		ExpectedRevision uint64 `json:"expected_revision"`
	}
	if r.StopRequest == nil || r.StopRequest.RequestID == "" || run == nil || run.Revision == 0 || json.Unmarshal(r.StopRequest.Input, &original) != nil || original.ExpectedRevision == 0 {
		return "", errors.New("original advisory stop envelope is unverified")
	}
	view, err := inspect(ctx, "request-id", r.StopRequest.RequestID)
	if err != nil {
		var failure *runnerclient.Failure
		if !errors.As(err, &failure) || failure.Unknown || failure.Code != "request_not_found" {
			return "", err
		}
		if original.ExpectedRevision > run.Revision {
			return "", errors.New("original advisory stop revision exceeds Run revision")
		}
		if original.ExpectedRevision < run.Revision {
			return "rejected", nil
		}
		return "unknown", nil
	}
	binding := Content(r)
	binding.RunID, binding.InputDigest, binding.Revision = run.RunID, run.InputDigest, run.Revision
	if err := runnerbinding.ValidateContent(binding, view); err != nil {
		return "", err
	}
	command := view.Command
	if command == nil || command.CommandID == "" || command.RequestID != r.StopRequest.RequestID || command.Command != "stop" || command.RunID == nil || *command.RunID != run.RunID || command.ReceiptStatus != "accepted" {
		return "", errors.New("original advisory stop receipt identity is unverified")
	}
	// Acceptance keeps the original envelope, including an unresolved native
	// action. It is never a resource-settlement proof or permission to retry.
	return "accepted", nil
}

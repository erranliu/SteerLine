package reliable

import (
	"errors"
	b "github.com/erranliu/SteerLine/internal/execution/backend"
)

// OriginalWorkspace permits read-only inspection of an uncommitted resource
// receipt only against its immutable, durably saved original preparation.
func OriginalWorkspace(snapshot b.LifecycleSnapshot) (b.WorkspaceIntent, error) {
	for _, op := range snapshot.Operations {
		if op.Action != string(b.LifecyclePrepareWorkspace) || op.LifecycleIdentity == nil || op.LifecycleIdentity.ExecutionID != snapshot.Session.ExecutionID || op.LifecycleIdentity.Ref.AttemptID != snapshot.Session.AttemptID {
			continue
		}
		candidate, err := b.CandidateForOperation(op)
		if err != nil {
			return b.WorkspaceIntent{}, err
		}
		if candidate.Input.Workspace == nil || *candidate.Input.Workspace != snapshot.WorkspacePlan || Digest(op.Request) != op.PayloadDigest {
			return b.WorkspaceIntent{}, errors.New("workspace inspection differs from its original durable intent")
		}
		return *candidate.Input.Workspace, nil
	}
	return b.WorkspaceIntent{}, errors.New("workspace inspection has no matching committed resource or preparation intent")
}
